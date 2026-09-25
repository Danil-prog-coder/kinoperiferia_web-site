package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/catalog"
	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/orders"
)

func newTestServer(t *testing.T, store *orders.Store) *Server {
	t.Helper()
	s, err := New(
		os.DirFS(filepath.Join("..", "..", "web", "templates")),
		os.DirFS(filepath.Join("..", "..", "web", "static")),
		Options{BaseURL: "https://kinoperiferia.example", Orders: store},
	)
	if err != nil {
		t.Fatalf("сервер не собрался: %v", err)
	}
	return s
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRoutesReturnExpectedStatus(t *testing.T) {
	s := newTestServer(t, orders.NewStore("", nil, nil))

	cases := []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/catalog", http.StatusOK},
		{"/catalog?cat=light", http.StatusOK},
		{"/catalog?cat=неизвестно&sort=мусор", http.StatusOK},
		{"/product/cinesaddle", http.StatusOK},
		{"/product/нет-такого", http.StatusNotFound},
		{"/order", http.StatusOK},
		{"/privacy", http.StatusOK},
		{"/api/products", http.StatusOK},
		{"/api/products/sunhood", http.StatusOK},
		{"/api/products/нет-такого", http.StatusNotFound},
		{"/sitemap.xml", http.StatusOK},
		{"/robots.txt", http.StatusOK},
		{"/healthz", http.StatusOK},
		{"/static/css/style.css", http.StatusOK},
		{"/такой-страницы-нет", http.StatusNotFound},
	}

	for _, c := range cases {
		if rec := get(t, s, c.path); rec.Code != c.want {
			t.Errorf("GET %s = %d, ожидалось %d", c.path, rec.Code, c.want)
		}
	}
}

func TestIndexRendersFeaturedCatalogAndContacts(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/").Body.String()

	for _, want := range []string{
		"Все для СЪЕМОК", "Каталог", "Все 17 товаров →", "Показать больше",
		`class="hero__copy"`,
		"https://t.me/suvorov_dmitry", "Voros@list.ru",
		// html/template кодирует «+» в тексте и атрибутах как &#43; — браузер
		// разбирает его обратно, поэтому проверяем закодированный вид.
		"915 267-02-43", `href="tel:&#43;79152670243"`, `href="mailto:Voros@list.ru"`,
		"Расскажите, что нужно сшить", `enctype="multipart/form-data"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("на главной нет %q", want)
		}
	}

	for _, p := range catalog.Featured(featuredCount) {
		if !strings.Contains(body, p.Name) {
			t.Errorf("на главной нет хита %q", p.Name)
		}
	}
}

func TestCatalogFilterLimitsProducts(t *testing.T) {
	s := newTestServer(t, nil)
	body := get(t, s, "/catalog?cat=light").Body.String()

	if !strings.Contains(body, "6 в подборке") {
		t.Error("не показан счётчик подборки")
	}
	if !strings.Contains(body, "Флоппи флаг 120 × 120") {
		t.Error("в подборке «Для света» нет флоппи")
	}
	if strings.Contains(body, "Сандбег") {
		t.Error("в подборке «Для света» оказалось изделие другой категории")
	}
}

func TestCatalogShowsFullListWithoutRevealToggle(t *testing.T) {
	s := newTestServer(t, nil)

	body := get(t, s, "/catalog").Body.String()
	if strings.Contains(body, "Посмотреть все") {
		t.Error("на /catalog остался переключатель «Посмотреть все» — раздел 8 ТЗ убрал его")
	}
	if strings.Contains(body, `id="reveal-catalog"`) {
		t.Error("на /catalog остался CSS-переключатель раскрытия")
	}
	for _, p := range catalog.All() {
		if !strings.Contains(body, p.Name) {
			t.Errorf("на /catalog нет изделия %q — список должен показываться целиком", p.Name)
		}
	}
	if !strings.Contains(body, `data-filter="light"`) {
		t.Error("нет таба категории «Для света»")
	}
}

func TestCatalogUnknownFilterFallsBackToWholeCatalog(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/catalog?cat=выдумка").Body.String()
	if !strings.Contains(body, "17 товаров") {
		t.Error("неизвестная категория не свелась к полному каталогу")
	}
}

func TestAPIProductsSortByPrice(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/api/products?sort=price-asc").Body.String()

	var payload struct {
		Products []productDTO `json:"products"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	for i := 1; i < len(payload.Products); i++ {
		prev, cur := payload.Products[i-1].Price, payload.Products[i].Price
		if prev == 0 {
			continue
		}
		if cur != 0 && cur < prev {
			t.Errorf("сортировка по возрастанию цены нарушена на позиции %d: %d после %d", i, cur, prev)
		}
	}
}

func TestProductPageShowsVariantsAndRelated(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/product/soty-titan").Body.String()

	for _, want := range []string{
		"Соты для Titan",
		"Исполнения и цены",
		"от 300 ₽",
		"1 400 ₽",
		"120 см",
		"Из той же категории",
		`"@type":"Product"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице изделия нет %q", want)
		}
	}
}

func TestProductPageShowsUnknownPriceAsXXX(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/product/sunhood").Body.String()

	if !strings.Contains(body, catalog.PriceUnknown) {
		t.Error("цена без согласования показана не как XXX")
	}
	if !strings.Contains(body, "цену этого исполнения ещё согласовываем") {
		t.Error("нет пояснения к XXX")
	}
	if strings.Contains(body, "нет в наличии") {
		t.Error("у позиции в наличии показана пометка об отсутствии")
	}
}

func TestProductPageWithoutPhotoShowsPlaceholder(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/product/cinesaddle").Body.String()

	if !strings.Contains(body, "Фото скоро") {
		t.Error("вместо отсутствующей фотографии нет плашки")
	}
	if strings.Contains(body, `<img class="product__img"`) {
		t.Error("выведен пустой тег изображения")
	}
}

func TestCanonicalAndSitemapUseBaseURL(t *testing.T) {
	s := newTestServer(t, nil)

	if body := get(t, s, "/catalog").Body.String(); !strings.Contains(body, `href="https://kinoperiferia.example/catalog"`) {
		t.Error("canonical не использует BASE_URL")
	}

	sitemap := get(t, s, "/sitemap.xml").Body.String()
	if !strings.Contains(sitemap, "<loc>https://kinoperiferia.example/product/cinesaddle</loc>") {
		t.Error("в sitemap нет страницы изделия")
	}
	if strings.Contains(sitemap, "cat=camera&sort") || strings.Contains(sitemap, "&cat") {
		t.Error("в sitemap неэкранированный амперсанд")
	}
	if strings.Contains(sitemap, "?cat=") {
		t.Error("в sitemap остались отфильтрованные варианты /catalog?cat=… — их canonical всегда указывает на /catalog")
	}
	for _, p := range catalog.All() {
		if !strings.Contains(sitemap, "/product/"+p.Slug) {
			t.Errorf("в sitemap нет %q", p.Slug)
		}
	}
}

func TestAPIProductsFilterAndShape(t *testing.T) {
	s := newTestServer(t, nil)
	rec := get(t, s, "/api/products?cat=camera")

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q", ct)
	}

	var payload struct {
		Count    int          `json:"count"`
		Category string       `json:"category"`
		Products []productDTO `json:"products"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if payload.Category != "camera" || payload.Count != len(payload.Products) {
		t.Errorf("неверная сводка: %+v", payload)
	}
	for _, p := range payload.Products {
		if p.Category != "camera" {
			t.Errorf("в подборке изделие категории %q", p.Category)
		}
		if p.PriceLabel == "" || p.URL == "" || p.Telegram == "" {
			t.Errorf("в DTO не хватает полей: %+v", p)
		}
	}
}

func TestAPISingleProduct(t *testing.T) {
	rec := get(t, newTestServer(t, nil), "/api/products/soty-titan")

	var p productDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	// price — наименьшая известная цена исполнений, по ней же идёт сортировка.
	if p.Price != 300 || p.PriceLabel != "от 300 ₽" {
		t.Errorf("неверная цена: %+v", p)
	}
	if p.HasPhoto {
		t.Error("у изделия без фотографии hasPhoto = true")
	}
	if len(p.OptionNames) != 2 || len(p.Variants) != 12 {
		t.Errorf("исполнения переданы не полностью: %d колонок, %d строк", len(p.OptionNames), len(p.Variants))
	}
	if first := p.Variants[0]; first.Price != 1400 || first.PriceLabel != "1 400 ₽" {
		t.Errorf("первое исполнение: %+v", first)
	}
	if len(p.Specs) == 0 {
		t.Error("нет характеристик")
	}
}

func TestAPIMarksUnknownPriceAsXXX(t *testing.T) {
	rec := get(t, newTestServer(t, nil), "/api/products/sumka-mehanika")

	var p productDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if p.Price != 0 || p.PriceLabel != catalog.PriceUnknown {
		t.Errorf("цена без согласования: %+v", p)
	}
	for _, v := range p.Variants {
		if v.PriceLabel != catalog.PriceUnknown {
			t.Errorf("исполнение %v: подпись цены %q", v.Values, v.PriceLabel)
		}
	}
}

func postOrder(t *testing.T, s *Server, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/order", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "203.0.113.7:12345"
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestOrderSubmitAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.jsonl")
	s := newTestServer(t, orders.NewStore(path, nil, nil))

	rec := postOrder(t, s, url.Values{
		"name":    {"Дмитрий"},
		"contact": {"@suvorov_dmitry"},
		"product": {"sunhood"},
		"message": {"нужен козырёк"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("код ответа %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Заявка принята") {
		t.Error("нет подтверждения на странице")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("заявка не сохранена: %v", err)
	}
	if !strings.Contains(string(data), "sunhood") {
		t.Errorf("в файле нет выбранного изделия: %s", data)
	}
}

func TestOrderSubmitShowsFieldErrors(t *testing.T) {
	s := newTestServer(t, orders.NewStore("", nil, nil))
	rec := postOrder(t, s, url.Values{"name": {""}, "contact": {""}})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("код ответа %d, ожидался 422", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Укажите, как к вам обращаться") {
		t.Error("нет ошибки по имени")
	}
	if !strings.Contains(body, "Оставьте Telegram, телефон или почту") {
		t.Error("нет ошибки по контакту")
	}
}

func TestOrderSubmitKeepsEnteredValues(t *testing.T) {
	s := newTestServer(t, orders.NewStore("", nil, nil))
	rec := postOrder(t, s, url.Values{"name": {"Дмитрий"}, "contact": {""}, "message": {"чехол 4×5,65"}})

	body := rec.Body.String()
	if !strings.Contains(body, `value="Дмитрий"`) {
		t.Error("введённое имя потеряно после ошибки")
	}
	if !strings.Contains(body, "чехол 4×5,65") {
		t.Error("введённое сообщение потеряно после ошибки")
	}
}

func TestOrderSubmitDropsUnknownProduct(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.jsonl")
	s := newTestServer(t, orders.NewStore(path, nil, nil))

	postOrder(t, s, url.Values{
		"name":    {"А"},
		"contact": {"@a"},
		"product": {"<script>alert(1)</script>"},
	})

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "script") {
		t.Errorf("в заявку попало изделие вне каталога: %s", data)
	}
}

func TestOrderHoneypotLooksLikeSuccessButSavesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.jsonl")
	s := newTestServer(t, orders.NewStore(path, nil, nil))

	rec := postOrder(t, s, url.Values{
		"name":    {"Бот"},
		"contact": {"spam"},
		"company": {"spam corp"},
	})

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Заявка принята") {
		t.Error("ловушка выдала себя другим ответом")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("спам-заявка сохранена")
	}
}

func TestOrderRateLimited(t *testing.T) {
	s := newTestServer(t, orders.NewStore("", nil, nil))

	var lastCode int
	for i := 0; i < 12; i++ {
		lastCode = postOrder(t, s, url.Values{"name": {"А"}, "contact": {"@a"}}).Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Errorf("последний ответ %d, ожидался 429", lastCode)
	}
}

func TestOrderFormPreselectsProductFromQuery(t *testing.T) {
	body := get(t, newTestServer(t, orders.NewStore("", nil, nil)), "/order?product=sunhood").Body.String()
	if !strings.Contains(body, `value="sunhood" selected`) {
		t.Error("изделие из адреса не подставлено в форму")
	}

	body = get(t, newTestServer(t, orders.NewStore("", nil, nil)), "/order?product=выдумка").Body.String()
	if strings.Contains(body, "выдумка") {
		t.Error("несуществующее изделие попало в форму")
	}
}

func TestOrderUnavailableWithoutStore(t *testing.T) {
	s := newTestServer(t, nil)
	rec := postOrder(t, s, url.Values{"name": {"А"}, "contact": {"@a"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("код ответа %d, ожидался 503", rec.Code)
	}
}

func TestOrderFormHasFileUploadAndConsent(t *testing.T) {
	body := get(t, newTestServer(t, orders.NewStore("", nil, nil)), "/order").Body.String()
	if !strings.Contains(body, `type="file"`) || !strings.Contains(body, `name="files"`) {
		t.Error("в форме нет поля загрузки файлов")
	}
	if !strings.Contains(body, `name="consent"`) {
		t.Error("в форме нет чекбокса согласия")
	}
	if !strings.Contains(body, `href="/privacy"`) {
		t.Error("чекбокс согласия не ссылается на политику конфиденциальности")
	}
}

func TestStaticAssetsAreVersionedAndCached(t *testing.T) {
	s := newTestServer(t, nil)

	body := get(t, s, "/").Body.String()
	if !strings.Contains(body, "/static/css/style.css?v=") {
		t.Fatal("ссылка на стили без версии")
	}

	rec := get(t, s, "/static/css/style.css?v=abc")
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("версионированный файл без долгого кеша: %q", cc)
	}

	rec = get(t, s, "/static/css/style.css")
	if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
		t.Errorf("файл без версии отдан как неизменяемый: %q", cc)
	}
}

func TestStylesReflectRedesign(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/static/css/style.css").Body.String()

	for _, want := range []string{
		`--accent: #f2c200`,
		`grid-template-columns: repeat(4, minmax(0, 1fr))`,
		`var(--ease)`,
		`--fs-display: 88px`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("в стилях нет %q", want)
		}
	}

	for _, unwanted := range []string{
		"Prata", "Cormorant", "IBM Plex Sans Condensed",
		"linear-gradient(90deg", "reveal__toggle", "kpEnter",
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("в стилях остался старый декор %q, который должен быть убран по ТЗ", unwanted)
		}
	}
}

func TestOrderPageIsNotCached(t *testing.T) {
	rec := get(t, newTestServer(t, orders.NewStore("", nil, nil)), "/order")
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control страницы заявки = %q, ожидался no-store", cc)
	}
}

func TestHealthReportsCatalogSize(t *testing.T) {
	rec := get(t, newTestServer(t, nil), "/healthz")

	var payload struct {
		Status   string `json:"status"`
		Products int    `json:"products"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if payload.Status != "ok" || payload.Products != catalog.Count() {
		t.Errorf("неожиданный ответ: %+v", payload)
	}
}

func TestRobotsMentionsSitemap(t *testing.T) {
	body := get(t, newTestServer(t, nil), "/robots.txt").Body.String()
	if !strings.Contains(body, "Sitemap: https://kinoperiferia.example/sitemap.xml") {
		t.Errorf("robots.txt без sitemap: %q", body)
	}
}

func TestNotFoundRendersBrandedPage(t *testing.T) {
	rec := get(t, newTestServer(t, nil), "/нет-такой-страницы")
	body := rec.Body.String()

	if !strings.Contains(body, "Страницы нет") || !strings.Contains(body, "Кино") {
		t.Error("404 отдана без фирменной страницы")
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestPlural(t *testing.T) {
	cases := map[int]string{1: "товар", 2: "товара", 4: "товара", 5: "товаров",
		11: "товаров", 12: "товаров", 21: "товар", 22: "товара", 25: "товаров", 101: "товар"}
	for n, want := range cases {
		if got := plural(n, "товар", "товара", "товаров"); got != want {
			t.Errorf("plural(%d) = %q, ожидалось %q", n, got, want)
		}
	}
}

func TestClientIPPrefersForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	if got := clientIP(req); got != "10.0.0.1" {
		t.Errorf("clientIP = %q, ожидалось 10.0.0.1", got)
	}

	req.Header.Set("X-Forwarded-For", "203.0.113.5, 70.41.3.18")
	if got := clientIP(req); got != "203.0.113.5" {
		t.Errorf("clientIP = %q, ожидалось 203.0.113.5", got)
	}
}

func TestNoTemplateActionsLeakIntoOutput(t *testing.T) {
	s := newTestServer(t, orders.NewStore("", nil, nil))
	for _, path := range []string{"/", "/catalog", "/product/cinesaddle", "/order", "/privacy", "/нет-страницы"} {
		body, _ := io.ReadAll(get(t, s, path).Body)
		if strings.Contains(string(body), "{{") || strings.Contains(string(body), "<no value>") {
			t.Errorf("%s: в разметке остались части шаблона", path)
		}
	}
}

// Схема tel: не входит в список разрешённых в html/template: без явного
// template.URL ссылка на телефон превращается в #ZgotmplZ и перестаёт работать.
func TestContactLinksKeepTheirSchemes(t *testing.T) {
	s := newTestServer(t, nil)
	for _, path := range []string{"/", "/catalog", "/product/cinesaddle", "/order"} {
		body := get(t, s, path).Body.String()
		if strings.Contains(body, "ZgotmplZ") {
			t.Errorf("%s: ссылка отфильтрована html/template", path)
		}
		if !strings.Contains(body, `href="tel:&#43;79152670243"`) {
			t.Errorf("%s: нет рабочей ссылки на телефон", path)
		}
	}
}
