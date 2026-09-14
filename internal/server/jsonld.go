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
		"image":       p.Image,
		"category":    p.CategoryTitle(),
		"brand": map[string]any{
			"@type": "Brand",
			"name":  site.Brand,
		},
	}

	if p.Price > 0 {
		availability := "https://schema.org/InStock"
		if !p.InStock {
			availability = "https://schema.org/OutOfStock"
		}
		data["offers"] = map[string]any{
			"@type":         "Offer",
			"price":         p.Price,
			"priceCurrency": "RUB",
			"availability":  availability,
			"url":           s.canonical("/product/" + p.Slug),
		}
	}

	return s.marshalJSONLD(data)
}
