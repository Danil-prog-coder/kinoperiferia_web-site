package server

import (
	"encoding/json"
	"html/template"

	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/catalog"
	"github.com/danil-prog-coder/kinoperiferia_web-site/internal/site"
)

// marshalJSONLD печатает структуру для блока <script type="application/ld+json">.
// Ошибку кодирования гасим пустой строкой: разметка Schema.org — украшение для
// поисковиков, ронять из-за неё страницу нельзя.
func (s *Server) marshalJSONLD(v any) template.JS {
	b, err := json.Marshal(v)
	if err != nil {
		s.log.Error("не удалось собрать JSON-LD", "err", err)
		return ""
	}
	return template.JS(b)
}

func (s *Server) organizationJSONLD() template.JS {
	return s.marshalJSONLD(map[string]any{
		"@context": "https://schema.org",
		"@type":    "Organization",
		"name":     site.Brand,
		"url":      s.canonical("/"),
		"address": map[string]any{
			"@type":           "PostalAddress",
			"addressLocality": site.City,
			"addressCountry":  "RU",
		},
		"telephone": site.PhoneHref[len("tel:"):],
		"email":     site.Email,
		"sameAs":    []string{site.Telegram},
	})
}

func (s *Server) productJSONLD(p catalog.Product) template.JS {
	data := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "Product",
		"name":        p.Name,
		"sku":         p.Art,
		"description": p.Detail,
		"category":    p.CategoryTitle(),
		"brand": map[string]any{
			"@type": "Brand",
			"name":  site.Brand,
		},
	}
	// Фотографии может не быть: пустое поле image поисковики считают ошибкой,
	// поэтому лучше его не выводить вовсе.
	if p.HasPhoto() {
		data["image"] = p.Image
	}

	// Цены может не быть ни у одного исполнения — тогда блока offers нет:
	// предложение без цены разметке Schema.org не нужно.
	if minPrice := p.MinPrice(); minPrice > 0 {
		availability := "https://schema.org/InStock"
		if !p.InStock {
			availability = "https://schema.org/OutOfStock"
		}
		url := s.canonical("/product/" + p.Slug)

		if maxPrice := p.MaxPrice(); maxPrice > minPrice {
			data["offers"] = map[string]any{
				"@type":         "AggregateOffer",
				"lowPrice":      minPrice,
				"highPrice":     maxPrice,
				"offerCount":    p.PricedVariants(),
				"priceCurrency": "RUB",
				"availability":  availability,
				"url":           url,
			}
		} else {
			data["offers"] = map[string]any{
				"@type":         "Offer",
				"price":         minPrice,
				"priceCurrency": "RUB",
				"availability":  availability,
				"url":           url,
			}
		}
	}

	return s.marshalJSONLD(data)
}
