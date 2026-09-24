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
	PhoneHref     template.URL
	Email         string
	EmailHref     template.URL
	PickupAddress string
	Hours         string
	Requisites    string
	Features      []site.Feature
	Delivery      []site.DeliveryOption
	CustomSteps   []site.CustomStep
	CustomCases   []site.CustomCase
}

var siteView = siteData{
	Brand:         site.Brand,
	Tagline:       site.Tagline,
	City:          site.City,
	Warranty:      site.Warranty,
	Telegram:      site.Telegram,
	TelegramTag:   site.TelegramTag,
	Phone:         site.Phone,
	PhoneHref:     template.URL(site.PhoneHref),
	Email:         site.Email,
	EmailHref:     template.URL(site.EmailHref),
	PickupAddress: site.PickupAddress,
	Hours:         site.Hours,
	Requisites:    site.Requisites,
	Features:      site.Features,
	Delivery:      site.Delivery,
	CustomSteps:   site.CustomSteps,
	CustomCases:   site.CustomCases,
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

// categoryFilter — таб фильтра каталога на странице /catalog, со счётчиком
// изделий в категории.
type categoryFilter struct {
	Slug   string
	Title  string
	URL    string
	Active bool
	Count  int
}

// catalogView — полная витрина страницы /catalog: активный фильтр и список
// изделий. Сортировки и сворачивания больше нет — раздел 8 ТЗ убрал их как
// не нужные при 18 позициях.
type catalogView struct {
	Heading        string
	CountLabel     string
	ActiveCategory string
	Categories     []categoryFilter
	Products       []catalog.Product
}

// catalogTeaser — подборка хитов на главной: без фильтров и сортировки,
// с одной ссылкой на полный каталог.
type catalogTeaser struct {
	Heading    string
	TotalLabel string
	MoreURL    string
	Products   []catalog.Product
}

// indexPage — главная страница.
type indexPage struct {
	pageBase
	Hero    catalog.Product
	Catalog catalogTeaser
	Contact contactForm
}

// catalogPage — отдельная страница каталога.
type catalogPage struct {
	pageBase
	Catalog catalogView
}

// productPage — страница изделия.
type productPage struct {
	pageBase
	Product      catalog.Product
	Related      []catalog.Product
	BackURL      string
	TelegramHref string
}

// orderForm — значения полей формы заявки, которые возвращаются пользователю
// при ошибке валидации.
type orderForm struct {
	Name    string
	Contact string
	Product string
	Message string
}

// contactForm — состояние формы заявки: используется и на отдельной
// странице /order, и как встроенный блок в конце главной (раздел 6.8 ТЗ).
// Несёт свою копию Site, чтобы общий шаблон формы не зависел от того, что
// именно сейчас исполняется как корень шаблона.
type contactForm struct {
	Site      siteData
	Form      orderForm
	Errors    map[string]string
	Products  []catalog.Product
	Submitted bool
	Failed    bool
	OrderID   string
}

// orderPage — страница заявки.
type orderPage struct {
	pageBase
	Contact contactForm
}

// errorPage — страница ошибки (404, 500).
type errorPage struct {
	pageBase
	Code    int
	Heading string
	Message string
}

// staticPage — страница со статичным текстом (политика конфиденциальности).
type staticPage struct {
	pageBase
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
	HasPhoto      bool           `json:"hasPhoto"`
	URL           string         `json:"url"`
	Telegram      string         `json:"telegram"`
	Specs         []catalog.Spec `json:"specs"`
	OptionNames   []string       `json:"optionNames"`
	Variants      []variantDTO   `json:"variants"`
}

// variantDTO — исполнение изделия в JSON API: значения параметров в порядке
// optionNames и цена с готовой подписью («1 400 ₽» или XXX).
type variantDTO struct {
	Values     []string `json:"values"`
	Price      int      `json:"price"`
	PriceLabel string   `json:"priceLabel"`
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
		Price:         p.SortPrice(),
		OldPrice:      p.OldPrice,
		From:          p.From,
		InStock:       p.InStock,
		PriceLabel:    p.PriceLabel(),
		OldPriceLabel: p.OldPriceLabel(),
		Image:         p.Image,
		Alt:           p.Alt,
		HasPhoto:      p.HasPhoto(),
		URL:           "/product/" + p.Slug,
		Telegram:      site.Telegram,
		Specs:         p.Specs,
		OptionNames:   p.OptionNames,
		Variants:      toVariantDTOs(p.Variants),
	}
}

func toVariantDTOs(list []catalog.Variant) []variantDTO {
	out := make([]variantDTO, 0, len(list))
	for _, v := range list {
		out = append(out, variantDTO{Values: v.Values, Price: v.Price, PriceLabel: v.PriceLabel()})
	}
	return out
}

func toDTOs(list []catalog.Product) []productDTO {
	out := make([]productDTO, 0, len(list))
	for _, p := range list {
		out = append(out, toDTO(p))
	}
	return out
}
