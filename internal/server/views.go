package server

import (
	"html/template"

	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/catalog"
	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/site"
)

// siteData отдаёт шаблонам контакты и текстовые блоки из пакета site.
type siteData struct {
	Brand       string
	Tagline     string
	City        string
	Warranty    string
	Telegram    string
	TelegramTag string
	Phone       string
	// html/template пропускает в href только http, https, mailto и
	// относительные адреса, а схему tel: заменяет на #ZgotmplZ. Схемы здесь
	// заданы константами в коде, поэтому помечаем их как доверенные.
	PhoneHref   template.URL
	Email       string
	EmailHref   template.URL
	Features    []site.Feature
	Delivery    []site.DeliveryOption
	CustomSteps []site.CustomStep
	TickerItems []string
}

var siteView = siteData{
	Brand:       site.Brand,
	Tagline:     site.Tagline,
	City:        site.City,
	Warranty:    site.Warranty,
	Telegram:    site.Telegram,
	TelegramTag: site.TelegramTag,
	Phone:       site.Phone,
	PhoneHref:   template.URL(site.PhoneHref),
	Email:       site.Email,
	EmailHref:   template.URL(site.EmailHref),
	Features:    site.Features,
	Delivery:    site.Delivery,
	CustomSteps: site.CustomSteps,
	TickerItems: site.TickerItems,
}

// pageBase — общие для всех страниц данные: мета-теги, навигация, контакты.
type pageBase struct {
	Title       string
	Description string
	Canonical   string
	OGType      string
	OGImage     string
	JSONLD      template.JS
	Nav         string
	Year        int
	Site        siteData
}

// categoryFilter — чип фильтра каталога.
type categoryFilter struct {
	Slug   string
	Title  string
	URL    string
	Active bool
}

// sortOption — пункт выпадающего списка сортировки.
type sortOption struct {
	Value    string
	Title    string
	Selected bool
}

// catalogView — состояние витрины: активный фильтр, сортировка и список изделий.
type catalogView struct {
	Heading        string
	CountLabel     string
	Action         string
	ActiveCategory string
	Categories     []categoryFilter
	SortOptions    []sortOption
	Products       []catalog.Product
}

// indexPage — главная страница.
type indexPage struct {
	pageBase
	Hero    catalog.Product
	Catalog catalogView
}

// catalogPage — отдельная страница каталога.
type catalogPage struct {
	pageBase
	Catalog catalogView
}

// productPage — страница изделия.
type productPage struct {
	pageBase
	Product catalog.Product
	Related []catalog.Product
	BackURL string
}

// orderForm — значения полей формы заявки, которые возвращаются пользователю
// при ошибке валидации.
type orderForm struct {
	Name    string
	Contact string
	Product string
	Message string
}

// orderPage — страница заявки.
type orderPage struct {
	pageBase
	Form      orderForm
	Errors    map[string]string
	Products  []catalog.Product
	Submitted bool
	Failed    bool
	OrderID   string
}

// errorPage — страница ошибки (404, 500).
type errorPage struct {
	pageBase
	Code    int
	Heading string
	Message string
}

// productDTO — представление изделия в JSON API. Включает готовые к выводу
// подписи, чтобы клиенту не пришлось повторять форматирование цен.
type productDTO struct {
	Slug          string         `json:"slug"`
	Art           string         `json:"art"`
	Name          string         `json:"name"`
	Category      string         `json:"category"`
	CategoryTitle string         `json:"categoryTitle"`
	Kicker        string         `json:"kicker"`
	Tag           string         `json:"tag"`
	Desc          string         `json:"desc"`
	Detail        string         `json:"detail"`
	Price         int            `json:"price"`
	OldPrice      int            `json:"oldPrice"`
	From          bool           `json:"from"`
	InStock       bool           `json:"inStock"`
	PriceLabel    string         `json:"priceLabel"`
	OldPriceLabel string         `json:"oldPriceLabel"`
	Image         string         `json:"image"`
	Alt           string         `json:"alt"`
	URL           string         `json:"url"`
	Telegram      string         `json:"telegram"`
	Specs         []catalog.Spec `json:"specs"`
}

func toDTO(p catalog.Product) productDTO {
	return productDTO{
		Slug:          p.Slug,
		Art:           p.Art,
		Name:          p.Name,
		Category:      p.Category,
		CategoryTitle: p.CategoryTitle(),
		Kicker:        p.Kicker,
		Tag:           p.Tag(),
		Desc:          p.Desc,
		Detail:        p.Detail,
		Price:         p.Price,
		OldPrice:      p.OldPrice,
		From:          p.From,
		InStock:       p.InStock,
		PriceLabel:    p.PriceLabel(),
		OldPriceLabel: p.OldPriceLabel(),
		Image:         p.Image,
		Alt:           p.Alt,
		URL:           "/product/" + p.Slug,
		Telegram:      site.Telegram,
		Specs:         p.Specs,
	}
}

func toDTOs(list []catalog.Product) []productDTO {
	out := make([]productDTO, 0, len(list))
	for _, p := range list {
		out = append(out, toDTO(p))
	}
	return out
}
