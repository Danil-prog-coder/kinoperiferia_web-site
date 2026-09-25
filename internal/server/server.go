// Package server собирает HTTP-приложение «Кинопериферии»: маршруты, рендеринг
// шаблонов, JSON API и приём заявок.
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/catalog"
	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/orders"
	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/site"
)

// Options — настройки приложения.
type Options struct {
	// BaseURL — публичный адрес сайта, нужен для canonical, Open Graph и sitemap.
	BaseURL string
	// Orders принимает заявки с формы. Может быть nil — тогда форма недоступна.
	Orders *orders.Store
	Logger *slog.Logger
}

// Server — HTTP-приложение.
type Server struct {
	mux       *http.ServeMux
	templates map[string]*template.Template
	assets    assetVersions
	orders    *orders.Store
	baseURL   string
	log       *slog.Logger
	limiter   *rateLimiter
}

// New собирает приложение: разбирает шаблоны, считает версии статики и
// регистрирует маршруты.
func New(templatesFS, staticFS fs.FS, opts Options) (*Server, error) {
	log := opts.Logger
	if log == nil {
		log = slog.Default()
	}

	assets, err := buildAssetVersions(staticFS)
	if err != nil {
		return nil, fmt.Errorf("версии статики: %w", err)
	}

	s := &Server{
		mux:     http.NewServeMux(),
		assets:  assets,
		orders:  opts.Orders,
		baseURL: strings.TrimRight(opts.BaseURL, "/"),
		log:     log,
		limiter: newRateLimiter(10, time.Minute),
	}

	if s.templates, err = parseTemplates(templatesFS, assets); err != nil {
		return nil, fmt.Errorf("шаблоны: %w", err)
	}

	s.routes(staticFS)
	return s, nil
}

// ServeHTTP реализует http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// pages перечисляет шаблоны страниц; каждый объявляет блок "content".
var pages = []string{"index", "catalog", "product", "order", "error", "privacy"}

func parseTemplates(fsys fs.FS, assets assetVersions) (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"asset": assets.URL,
	}
	out := make(map[string]*template.Template, len(pages))
	for _, name := range pages {
		t, err := template.New(name).Funcs(funcs).ParseFS(fsys,
			"base.gohtml", "partials.gohtml", name+".gohtml")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[name] = t
	}
	return out, nil
}

func (s *Server) routes(staticFS fs.FS) {
	s.mux.Handle("GET /static/", staticHandler(staticFS))

	s.mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux.HandleFunc("GET /catalog", s.handleCatalog)
	s.mux.HandleFunc("GET /product/{slug}", s.handleProduct)
	s.mux.HandleFunc("GET /order", s.handleOrderForm)
	s.mux.HandleFunc("POST /order", s.handleOrderSubmit)
	s.mux.HandleFunc("GET /privacy", s.handlePrivacy)

	s.mux.HandleFunc("GET /api/products", s.handleAPIProducts)
	s.mux.HandleFunc("GET /api/products/{slug}", s.handleAPIProduct)

	s.mux.HandleFunc("GET /sitemap.xml", s.handleSitemap)
	s.mux.HandleFunc("GET /robots.txt", s.handleRobots)
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Всё остальное — 404 с фирменной страницей.
	s.mux.HandleFunc("/", s.handleNotFound)
}

// ── Базовые данные страницы ────────────────────────────────────────────

func (s *Server) base(r *http.Request, nav, title, desc string) pageBase {
	return pageBase{
		Title:       title,
		Description: desc,
		Canonical:   s.canonical(r.URL.Path),
		Nav:         nav,
		Year:        time.Now().Year(),
		Site:        siteView,
	}
}

func (s *Server) canonical(path string) string {
	if s.baseURL == "" {
		return path
	}
	return s.baseURL + path
}

// ── Витрина ────────────────────────────────────────────────────────────

// featuredCount — сколько хитов показывает главная: сетка 4×2 без фильтров
// и сортировки. Полный список — только на /catalog (раздел 6.4, 8 ТЗ).
const featuredCount = 8

// buildHomeCatalog собирает подборку хитов для главной.
func (s *Server) buildHomeCatalog() catalogTeaser {
	all := catalog.All()
	return catalogTeaser{
		Heading:    "Каталог",
		TotalLabel: "Все " + countLabel(len(all), true) + " →",
		MoreURL:    "/catalog",
		Products:   catalog.Featured(featuredCount),
	}
}

// buildFullCatalog собирает состояние страницы /catalog из query-параметра
// cat. Неизвестное значение молча заменяется пустым: ссылки на каталог часто
// приходят извне, и падать из-за мусора в query не нужно. Сортировки на этой
// странице больше нет — при 18 позициях она не нужна (раздел 8 ТЗ).
func (s *Server) buildFullCatalog(r *http.Request) catalogView {
	cat := r.URL.Query().Get("cat")
	if cat == "all" || !catalog.IsCategory(cat) {
		cat = ""
	}

	list := catalog.ByCategory(cat)

	all := catalog.All()
	counts := map[string]int{"all": len(all)}
	for _, p := range all {
		counts[p.Category]++
	}

	filters := make([]categoryFilter, 0, len(catalog.Categories))
	for _, c := range catalog.Categories {
		href := "/catalog"
		if c.Slug != "all" {
			href += "?" + (url.Values{"cat": {c.Slug}}).Encode()
		}
		active := c.Slug == cat || (cat == "" && c.Slug == "all")
		filters = append(filters, categoryFilter{Slug: c.Slug, Title: c.Title, URL: href, Active: active, Count: counts[c.Slug]})
	}

	return catalogView{
		Heading:        "Каталог изделий",
		CountLabel:     countLabel(len(list), cat == ""),
		ActiveCategory: cat,
		Categories:     filters,
		Products:       list,
	}
}

// isKnownSort проверяет параметр sort у JSON API — в разметке страниц
// сортировки больше нет, но API её сохраняет для внешних интеграций.
func isKnownSort(mode string) bool {
	switch mode {
	case catalog.SortDefault, catalog.SortPriceAsc, catalog.SortPriceDesc, catalog.SortName:
		return true
	}
	return false
}

// countLabel склоняет «товар» по числу изделий: 1 товар, 2 товара, 5 товаров.
func countLabel(n int, whole bool) string {
	if !whole {
		return fmt.Sprintf("%d в подборке", n)
	}
	return fmt.Sprintf("%d %s", n, plural(n, "товар", "товара", "товаров"))
}

func plural(n int, one, few, many string) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}

// ── Обработчики страниц ────────────────────────────────────────────────

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	hero, ok := catalog.BySlug("cinesaddle")
	if !ok {
		all := catalog.All()
		hero = all[0]
	}

	page := indexPage{
		pageBase: s.base(r, "home",
			site.Brand+" — аксессуары для кинопроизводства, "+site.City,
			"Синесэдлы, сумки, ложементы и текстиль для света. Собственное производство в Москве, "+
				"пошив под конкретный сетап, гарантия 1 год."),
		Hero:    hero,
		Catalog: s.buildHomeCatalog(),
		Contact: s.contactFormBase(),
	}
	page.OGImage = hero.Image
	page.JSONLD = s.organizationJSONLD()

	s.render(w, r, "index", http.StatusOK, page)
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	view := s.buildFullCatalog(r)

	title := "Каталог — " + site.Brand
	desc := "Все изделия «Кинопериферии»: седла для камеры, сумки и кейсы, текстиль для света и оснастка площадки."
	if view.ActiveCategory != "" {
		name := catalog.CategoryTitle(view.ActiveCategory)
		view.Heading = name
		title = name + " — каталог " + site.Brand
		desc = "Изделия «Кинопериферии» в категории «" + name + "»: цены, характеристики и заказ в Telegram."
	}

	page := catalogPage{
		pageBase: s.base(r, "catalog", title, desc),
		Catalog:  view,
	}
	if len(view.Products) > 0 {
		page.OGImage = view.Products[0].Image
	}
	s.render(w, r, "catalog", http.StatusOK, page)
}

func (s *Server) handleProduct(w http.ResponseWriter, r *http.Request) {
	p, ok := catalog.BySlug(r.PathValue("slug"))
	if !ok {
		s.handleNotFound(w, r)
		return
	}

	related := make([]catalog.Product, 0, 3)
	for _, other := range catalog.ByCategory(p.Category) {
		if other.Slug == p.Slug {
			continue
		}
		related = append(related, other)
		if len(related) == 3 {
			break
		}
	}

	page := productPage{
		pageBase: s.base(r, "catalog",
			p.Name+" — "+p.PriceLabel()+" — "+site.Brand,
			p.Detail),
		Product: p,
		Related: related,
		BackURL: "/catalog?cat=" + p.Category,
	}
	page.OGType = "product"
	page.OGImage = p.Image
	page.JSONLD = s.productJSONLD(p)

	s.render(w, r, "product", http.StatusOK, page)
}

// ── Заявка ─────────────────────────────────────────────────────────────

// maxOrderBody — потолок тела запроса заявки: 5 файлов по 10 МБ плюс запас
// на текстовые поля и служебные части multipart-разметки.
const maxOrderBody = orders.MaxFiles*orders.MaxFileSize + 1<<20

func (s *Server) contactFormBase() contactForm {
	return contactForm{
		Site:     siteView,
		Errors:   map[string]string{},
		Products: catalog.All(),
	}
}

func (s *Server) orderPageBase(r *http.Request) orderPage {
	return orderPage{
		pageBase: s.base(r, "order", "Заявка — "+site.Brand,
			"Оставьте заявку на изделие «Кинопериферии»: готовую позицию из каталога или пошив под ваш сетап."),
		Contact: s.contactFormBase(),
	}
}

func (s *Server) handleOrderForm(w http.ResponseWriter, r *http.Request) {
	page := s.orderPageBase(r)
	if slug := r.URL.Query().Get("product"); slug != "" {
		if _, ok := catalog.BySlug(slug); ok {
			page.Contact.Form.Product = slug
		}
	}
	s.render(w, r, "order", http.StatusOK, page)
}

func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	page := staticPage{
		pageBase: s.base(r, "", "Политика конфиденциальности — "+site.Brand,
			"Как «Кинопериферия» обрабатывает персональные данные, оставленные в заявке."),
	}
	s.render(w, r, "privacy", http.StatusOK, page)
}

func (s *Server) handleOrderSubmit(w http.ResponseWriter, r *http.Request) {
	page := s.orderPageBase(r)

	// Форма может прийти как обычная (без вложений) или как multipart с
	// файлами — ParseMultipartForm сама разбирает оба случая; тело запроса
	// ограничиваем заранее, чтобы не читать в память лишнее.
	r.Body = http.MaxBytesReader(w, r.Body, maxOrderBody)
	if err := r.ParseMultipartForm(orders.MaxFileSize); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		page.Contact.Failed = true
		page.Contact.Errors["files"] = "Файлы не загрузились — попробуйте меньшего размера или без них."
		s.render(w, r, "order", http.StatusBadRequest, page)
		return
	}

	page.Contact.Form = orderForm{
		Name:    r.PostFormValue("name"),
		Contact: r.PostFormValue("contact"),
		Product: r.PostFormValue("product"),
		Message: r.PostFormValue("message"),
	}

	// Ловушка для ботов: поле скрыто от людей, заполнить его может только робот.
	// Отвечаем как при успехе, чтобы не подсказывать спамеру, что он отсеян.
	if strings.TrimSpace(r.PostFormValue("company")) != "" {
		page.Contact.Submitted = true
		page.Contact.OrderID = "KP-0000-000"
		s.render(w, r, "order", http.StatusOK, page)
		return
	}

	if s.orders == nil {
		page.Contact.Failed = true
		s.render(w, r, "order", http.StatusServiceUnavailable, page)
		return
	}

	if !s.limiter.allow(clientIP(r)) {
		page.Contact.Failed = true
		page.Contact.Errors["message"] = "Слишком много заявок подряд. Попробуйте через минуту или напишите в Telegram."
		s.render(w, r, "order", http.StatusTooManyRequests, page)
		return
	}

	// Изделие принимаем только из каталога: это защищает от произвольного
	// текста в поле, которое пользователь видит как выпадающий список.
	if page.Contact.Form.Product != "" {
		if _, ok := catalog.BySlug(page.Contact.Form.Product); !ok {
			page.Contact.Form.Product = ""
		}
	}

	// Корзина приходит скрытым полем items — JSON-массив, который собирает
	// JS на странице заявки. Каждую позицию сверяем с каталогом по тем же
	// причинам, что и одиночное изделие выше.
	items := parseOrderItems(r.PostFormValue("items"))

	var headers []*multipart.FileHeader
	if r.MultipartForm != nil {
		headers = r.MultipartForm.File["files"]
	}
	if err := orders.ValidateFiles(headers); err != nil {
		var verr *orders.ValidationError
		errors.As(err, &verr)
		page.Contact.Errors = verr.Fields
		page.Contact.Failed = true
		s.render(w, r, "order", http.StatusUnprocessableEntity, page)
		return
	}
	files, err := orders.ReadFiles(headers)
	if err != nil {
		s.log.Error("не удалось прочитать вложения заявки", "err", err)
		page.Contact.Failed = true
		page.Contact.Errors["files"] = "Не удалось прочитать файлы, попробуйте ещё раз."
		s.render(w, r, "order", http.StatusInternalServerError, page)
		return
	}

	order, err := s.orders.Submit(orders.Order{
		Name:    page.Contact.Form.Name,
		Contact: page.Contact.Form.Contact,
		Product: page.Contact.Form.Product,
		Message: page.Contact.Form.Message,
		Items:   items,
	}, files)
	if err != nil {
		var verr *orders.ValidationError
		if errors.As(err, &verr) {
			page.Contact.Errors = verr.Fields
			page.Contact.Failed = true
			s.render(w, r, "order", http.StatusUnprocessableEntity, page)
			return
		}
		// Сохранить заявку не удалось — показываем отказ, чтобы человек написал
		// в Telegram, а не считал, что его услышали.
		s.log.Error("заявку не удалось принять", "err", err)
		page.Contact.Failed = true
		s.render(w, r, "order", http.StatusInternalServerError, page)
		return
	}

	s.log.Info("новая заявка", "id", order.ID, "product", order.Product, "items", len(order.Items), "files", len(order.Files))
	page.Contact.Submitted = true
	page.Contact.OrderID = order.ID
	s.render(w, r, "order", http.StatusOK, page)
}

// parseOrderItems разбирает корзину, присланную скрытым полем формы как
// JSON-массив. Некорректный JSON и позиции с неизвестным slug молча
// отбрасываются — то же правило, что и для одиночного поля product: со
// страницы приходят только реальные slug'и каталога, всё остальное — либо
// баг на клиенте, либо попытка подделать запрос.
func parseOrderItems(raw string) []orders.OrderItem {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var in []struct {
		Slug    string `json:"slug"`
		Variant string `json:"variant"`
		Qty     int    `json:"qty"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil
	}
	out := make([]orders.OrderItem, 0, len(in))
	for _, it := range in {
		p, ok := catalog.BySlug(strings.TrimSpace(it.Slug))
		if !ok {
			continue
		}
		out = append(out, orders.OrderItem{
			Slug:    p.Slug,
			Name:    p.Name,
			Variant: it.Variant,
			Qty:     it.Qty,
		})
	}
	return out
}

// ── JSON API ───────────────────────────────────────────────────────────

func (s *Server) handleAPIProducts(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("cat")
	if cat == "all" || !catalog.IsCategory(cat) {
		cat = ""
	}
	sortMode := r.URL.Query().Get("sort")
	if !isKnownSort(sortMode) {
		sortMode = catalog.SortDefault
	}
	list := catalog.Sort(catalog.ByCategory(cat), sortMode)

	s.writeJSON(w, http.StatusOK, map[string]any{
		"count":    len(list),
		"category": cat,
		"sort":     sortMode,
		"products": toDTOs(list),
	})
}

func (s *Server) handleAPIProduct(w http.ResponseWriter, r *http.Request) {
	p, ok := catalog.BySlug(r.PathValue("slug"))
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "изделие не найдено"})
		return
	}
	s.writeJSON(w, http.StatusOK, toDTO(p))
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		s.log.Error("не удалось записать JSON", "err", err)
	}
}

// ── Служебные маршруты ─────────────────────────────────────────────────

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	// Отфильтрованные /catalog?cat=… не входят: их canonical всегда указывает
	// на голый /catalog (раздел 8 ТЗ), отдельная индексация им не нужна.
	paths := []string{"/", "/catalog", "/order", "/privacy"}
	for _, p := range catalog.All() {
		paths = append(paths, "/product/"+p.Slug)
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, p := range paths {
		b.WriteString("  <url><loc>")
		xmlEscape(&b, s.canonical(p))
		b.WriteString("</loc></url>\n")
	}
	b.WriteString("</urlset>\n")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte(b.String()))
}

func xmlEscape(b *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	body := "User-agent: *\nAllow: /\nDisallow: /api/\n"
	if s.baseURL != "" {
		body += "Sitemap: " + s.baseURL + "/sitemap.xml\n"
	}
	_, _ = w.Write([]byte(body))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"products": catalog.Count(),
	})
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	page := errorPage{
		pageBase: s.base(r, "", "Страница не найдена — "+site.Brand,
			"Такой страницы на сайте «Кинопериферии» нет."),
		Code:    http.StatusNotFound,
		Heading: "Страницы нет",
		Message: "Ссылка устарела или в адресе опечатка. Каталог изделий на месте — начните с него.",
	}
	s.render(w, r, "error", http.StatusNotFound, page)
}

// ── Рендеринг ──────────────────────────────────────────────────────────

// render выполняет шаблон в буфер и только потом пишет ответ: если шаблон
// упадёт на середине, пользователь не получит полстраницы с кодом 200.
func (s *Server) render(w http.ResponseWriter, r *http.Request, name string, status int, data any) {
	t, ok := s.templates[name]
	if !ok {
		s.log.Error("неизвестный шаблон", "name", name)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		s.log.Error("ошибка шаблона", "name", name, "path", r.URL.Path, "err", err)
		http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if status == http.StatusOK && name != "order" {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
