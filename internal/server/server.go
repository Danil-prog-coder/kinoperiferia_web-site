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
var pages = []string{"index", "catalog", "product", "order", "error"}

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

var sortTitles = []struct{ Value, Title string }{
	{catalog.SortDefault, "По умолчанию"},
	{catalog.SortPriceAsc, "Сначала дешевле"},
	{catalog.SortPriceDesc, "Сначала дороже"},
	{catalog.SortName, "По названию"},
}

// buildCatalogView собирает состояние витрины из query-параметров cat и sort.
// Неизвестные значения молча заменяются значениями по умолчанию: ссылки на
// каталог часто приходят извне, и падать из-за мусора в query не нужно.
func (s *Server) buildCatalogView(r *http.Request, action, heading string) catalogView {
	cat := r.URL.Query().Get("cat")
	if cat == "all" || !catalog.IsCategory(cat) {
		cat = ""
	}

	sortMode := r.URL.Query().Get("sort")
	if !isKnownSort(sortMode) {
		sortMode = catalog.SortDefault
	}

	list := catalog.Sort(catalog.ByCategory(cat), sortMode)

	filters := make([]categoryFilter, 0, len(catalog.Categories))
	for _, c := range catalog.Categories {
		q := url.Values{}
		if c.Slug != "all" {
			q.Set("cat", c.Slug)
		}
		if sortMode != catalog.SortDefault {
			q.Set("sort", sortMode)
		}
		href := action
		if len(q) > 0 {
			href += "?" + q.Encode()
		}
		active := c.Slug == cat || (cat == "" && c.Slug == "all")
		filters = append(filters, categoryFilter{Slug: c.Slug, Title: c.Title, URL: href, Active: active})
	}

	options := make([]sortOption, 0, len(sortTitles))
	for _, o := range sortTitles {
		options = append(options, sortOption{Value: o.Value, Title: o.Title, Selected: o.Value == sortMode})
	}

	return catalogView{
		Heading:        heading,
		CountLabel:     countLabel(len(list), cat == ""),
		Action:         action,
		ActiveCategory: cat,
		Categories:     filters,
		SortOptions:    options,
		Products:       list,
	}
}

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
		Catalog: s.buildCatalogView(r, "/", "Готовые изделия"),
	}
	page.OGImage = hero.Image
	page.JSONLD = s.organizationJSONLD()

	s.render(w, r, "index", http.StatusOK, page)
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	view := s.buildCatalogView(r, "/catalog", "Каталог изделий")

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

func (s *Server) orderPageBase(r *http.Request) orderPage {
	return orderPage{
		pageBase: s.base(r, "order", "Заявка — "+site.Brand,
			"Оставьте заявку на изделие «Кинопериферии»: готовую позицию из каталога или пошив под ваш сетап."),
		Errors:   map[string]string{},
		Products: catalog.All(),
	}
}

func (s *Server) handleOrderForm(w http.ResponseWriter, r *http.Request) {
	page := s.orderPageBase(r)
	if slug := r.URL.Query().Get("product"); slug != "" {
		if _, ok := catalog.BySlug(slug); ok {
			page.Form.Product = slug
		}
	}
	s.render(w, r, "order", http.StatusOK, page)
}

func (s *Server) handleOrderSubmit(w http.ResponseWriter, r *http.Request) {
	page := s.orderPageBase(r)

	// Форма открыта всем, поэтому тело запроса ограничиваем до разбора.
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		page.Failed = true
		s.render(w, r, "order", http.StatusBadRequest, page)
		return
	}

	page.Form = orderForm{
		Name:    r.PostFormValue("name"),
		Contact: r.PostFormValue("contact"),
		Product: r.PostFormValue("product"),
		Message: r.PostFormValue("message"),
	}

	// Ловушка для ботов: поле скрыто от людей, заполнить его может только робот.
	// Отвечаем как при успехе, чтобы не подсказывать спамеру, что он отсеян.
	if strings.TrimSpace(r.PostFormValue("company")) != "" {
		page.Submitted = true
		page.OrderID = "KP-0000-000"
		s.render(w, r, "order", http.StatusOK, page)
		return
	}

	if s.orders == nil {
		page.Failed = true
		s.render(w, r, "order", http.StatusServiceUnavailable, page)
		return
	}

	if !s.limiter.allow(clientIP(r)) {
		page.Failed = true
		page.Errors["message"] = "Слишком много заявок подряд. Попробуйте через минуту или напишите в Telegram."
		s.render(w, r, "order", http.StatusTooManyRequests, page)
		return
	}

	// Изделие принимаем только из каталога: это защищает от произвольного
	// текста в поле, которое пользователь видит как выпадающий список.
	if page.Form.Product != "" {
		if _, ok := catalog.BySlug(page.Form.Product); !ok {
			page.Form.Product = ""
		}
	}

	order, err := s.orders.Submit(orders.Order{
		Name:    page.Form.Name,
		Contact: page.Form.Contact,
		Product: page.Form.Product,
		Message: page.Form.Message,
	})
	if err != nil {
		var verr *orders.ValidationError
		if errors.As(err, &verr) {
			page.Errors = verr.Fields
			page.Failed = true
			s.render(w, r, "order", http.StatusUnprocessableEntity, page)
			return
		}
		// Сохранить заявку не удалось — показываем отказ, чтобы человек написал
		// в Telegram, а не считал, что его услышали.
		s.log.Error("заявку не удалось принять", "err", err)
		page.Failed = true
		s.render(w, r, "order", http.StatusInternalServerError, page)
		return
	}

	s.log.Info("новая заявка", "id", order.ID, "product", order.Product)
	page.Submitted = true
	page.OrderID = order.ID
	s.render(w, r, "order", http.StatusOK, page)
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
	paths := []string{"/", "/catalog", "/order"}
	for _, c := range catalog.Categories {
		if c.Slug == "all" {
			continue
		}
		paths = append(paths, "/catalog?cat="+c.Slug)
	}
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
