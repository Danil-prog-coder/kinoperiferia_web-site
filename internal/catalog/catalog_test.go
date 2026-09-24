package catalog

import (
	"strconv"
	"testing"
)

func TestByCategorySplitsWholeCatalog(t *testing.T) {
	total := 0
	for _, c := range Categories {
		if c.Slug == "all" {
			continue
		}
		n := len(ByCategory(c.Slug))
		if n == 0 {
			t.Errorf("категория %q пуста", c.Slug)
		}
		total += n
	}
	if total != Count() {
		t.Errorf("сумма по категориям = %d, в каталоге %d", total, Count())
	}
}

func TestByCategoryAllAndUnknown(t *testing.T) {
	if got := len(ByCategory("")); got != Count() {
		t.Errorf("пустой slug вернул %d изделий, ожидалось %d", got, Count())
	}
	if got := len(ByCategory("all")); got != Count() {
		t.Errorf("slug all вернул %d изделий, ожидалось %d", got, Count())
	}
	if got := len(ByCategory("нет-такой")); got != 0 {
		t.Errorf("неизвестный slug вернул %d изделий, ожидалось 0", got)
	}
}

func TestAllReturnsCopy(t *testing.T) {
	list := All()
	list[0].Name = "испорчено"
	if All()[0].Name == "испорчено" {
		t.Fatal("All вернул ссылку на внутренние данные — каталог можно испортить извне")
	}
}

func TestSlugsAndArticlesAreUnique(t *testing.T) {
	slugs := map[string]bool{}
	arts := map[string]bool{}
	for _, p := range All() {
		if slugs[p.Slug] {
			t.Errorf("slug %q повторяется", p.Slug)
		}
		if arts[p.Art] {
			t.Errorf("артикул %q повторяется", p.Art)
		}
		slugs[p.Slug] = true
		arts[p.Art] = true

		if !IsCategory(p.Category) {
			t.Errorf("%s: неизвестная категория %q", p.Slug, p.Category)
		}
		// Фотографий ещё нет, но если ссылка появилась — alt обязателен.
		if p.Image != "" && p.Alt == "" {
			t.Errorf("%s: есть изображение без alt", p.Slug)
		}
	}
}

func TestVariantsMatchOptionNames(t *testing.T) {
	for _, p := range All() {
		if !p.HasVariants() {
			if len(p.OptionNames) > 0 {
				t.Errorf("%s: объявлены параметры без исполнений", p.Slug)
			}
			continue
		}
		if len(p.OptionNames) == 0 {
			t.Errorf("%s: исполнения без названий колонок", p.Slug)
		}
		if p.Price != 0 {
			t.Errorf("%s: у изделия с исполнениями цена берётся из них, поле Price должно быть нулевым", p.Slug)
		}
		for i, v := range p.Variants {
			if len(v.Values) != len(p.OptionNames) {
				t.Errorf("%s: исполнение %d описано %d значениями, колонок %d",
					p.Slug, i, len(v.Values), len(p.OptionNames))
			}
			for j, value := range v.Values {
				if value == "" {
					t.Errorf("%s: исполнение %d, пустое значение в колонке %q", p.Slug, i, p.OptionNames[j])
				}
			}
		}
	}
}

func TestBySlug(t *testing.T) {
	p, ok := BySlug("cinesaddle")
	if !ok {
		t.Fatal("cinesaddle не найден")
	}
	if got := p.MinPrice(); got != 15500 {
		t.Errorf("цена синесэдла = %d, ожидалось 15500", got)
	}
	if _, ok := BySlug("нет-такого"); ok {
		t.Error("несуществующий slug найден")
	}
}

func TestSortPriceKeepsPricelessLast(t *testing.T) {
	for _, mode := range []string{SortPriceAsc, SortPriceDesc} {
		list := Sort(All(), mode)
		if last := list[len(list)-1]; last.SortPrice() != 0 {
			t.Errorf("%s: последним оказалось %q с ценой %d, ожидалось изделие без цены",
				mode, last.Slug, last.SortPrice())
		}

		prev := -1
		for _, p := range list {
			price := p.SortPrice()
			if price == 0 {
				continue
			}
			if prev >= 0 {
				if mode == SortPriceAsc && price < prev {
					t.Errorf("%s: %d идёт после %d", mode, price, prev)
				}
				if mode == SortPriceDesc && price > prev {
					t.Errorf("%s: %d идёт после %d", mode, price, prev)
				}
			}
			prev = price
		}
	}
}

func TestSortUnknownModeKeepsOrder(t *testing.T) {
	list := Sort(All(), "чепуха")
	for i, p := range All() {
		if list[i].Slug != p.Slug {
			t.Fatalf("порядок изменился на позиции %d: %q вместо %q", i, list[i].Slug, p.Slug)
		}
	}
}

func TestFormatRub(t *testing.T) {
	cases := map[int]string{
		0:       "0 ₽",
		900:     "900 ₽",
		1500:    "1 500 ₽",
		35000:   "35 000 ₽",
		1234567: "1 234 567 ₽",
	}
	for in, want := range cases {
		if got := FormatRub(in); got != want {
			t.Errorf("FormatRub(%d) = %q, ожидалось %q", in, got, want)
		}
	}
}

func TestPriceLabels(t *testing.T) {
	cases := []struct {
		p    Product
		want string
	}{
		{Product{Price: 17000}, "17 000 ₽"},
		{Product{Price: 35000, From: true}, "от 35 000 ₽"},
		{Product{}, PriceUnknown},
		{withVariants(15500, 15500), "15 500 ₽"},
		{withVariants(300, 1400), "от 300 ₽"},
		{withVariants(1500, 0), "от 1 500 ₽"},
		{withVariants(0, 0), PriceUnknown},
	}
	for _, c := range cases {
		if got := c.p.PriceLabel(); got != c.want {
			t.Errorf("PriceLabel() = %q, ожидалось %q", got, c.want)
		}
	}

	if got := (Product{OldPrice: 2500}).OldPriceLabel(); got != "2 500 ₽" {
		t.Errorf("OldPriceLabel() = %q", got)
	}
	if got := (Product{}).OldPriceLabel(); got != "" {
		t.Errorf("OldPriceLabel() без скидки = %q, ожидалась пустая строка", got)
	}
}

func TestTagFallsBackWhenKickerDuplicatesCategory(t *testing.T) {
	p := Product{Category: "camera", Kicker: "Для камеры"}
	if got := p.Tag(); got != "Изделие каталога" {
		t.Errorf("Tag() = %q, ожидалось «Изделие каталога»", got)
	}
	p.Kicker = "Sale"
	if got := p.Tag(); got != "Sale" {
		t.Errorf("Tag() = %q, ожидалось «Sale»", got)
	}
}

// withVariants собирает изделие с исполнениями по списку цен: 0 означает, что
// цена этого исполнения ещё не согласована.
func withVariants(prices ...int) Product {
	p := Product{OptionNames: []string{"Размер"}}
	for i, price := range prices {
		p.Variants = append(p.Variants, Variant{Values: []string{strconv.Itoa(i)}, Price: price})
	}
	return p
}

func TestVariantPriceLabel(t *testing.T) {
	if got := (Variant{Price: 1400}).PriceLabel(); got != "1 400 ₽" {
		t.Errorf("PriceLabel() = %q", got)
	}
	if got := (Variant{}).PriceLabel(); got != PriceUnknown {
		t.Errorf("PriceLabel() без цены = %q, ожидалось %q", got, PriceUnknown)
	}
}

func TestMinMaxAndUnknownPrice(t *testing.T) {
	p := withVariants(1400, 300, 0)
	if got := p.MinPrice(); got != 300 {
		t.Errorf("MinPrice() = %d, ожидалось 300", got)
	}
	if got := p.MaxPrice(); got != 1400 {
		t.Errorf("MaxPrice() = %d, ожидалось 1400", got)
	}
	if got := p.PricedVariants(); got != 2 {
		t.Errorf("PricedVariants() = %d, ожидалось 2", got)
	}
	if !p.HasUnknownPrice() {
		t.Error("HasUnknownPrice() = false, хотя у одного исполнения цены нет")
	}

	if withVariants(1400, 300).HasUnknownPrice() {
		t.Error("HasUnknownPrice() = true, хотя цены есть у всех исполнений")
	}

	single := Product{Price: 15500}
	if got := single.MinPrice(); got != 15500 {
		t.Errorf("MinPrice() изделия без исполнений = %d", got)
	}
	if single.HasUnknownPrice() {
		t.Error("HasUnknownPrice() = true у изделия с ценой")
	}
	if !(Product{}).HasUnknownPrice() {
		t.Error("HasUnknownPrice() = false у изделия без цены")
	}
}

func TestHasPhoto(t *testing.T) {
	if (Product{}).HasPhoto() {
		t.Error("HasPhoto() = true без ссылки на фотографию")
	}
	if !(Product{Image: "https://example.test/p.jpg"}).HasPhoto() {
		t.Error("HasPhoto() = false при заполненном Image")
	}
}
