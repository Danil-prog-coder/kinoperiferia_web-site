package catalog

import "testing"

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
		if p.Image == "" || p.Alt == "" {
			t.Errorf("%s: нет изображения или alt", p.Slug)
		}
	}
}

func TestBySlug(t *testing.T) {
	p, ok := BySlug("cinesaddle")
	if !ok {
		t.Fatal("cinesaddle не найден")
	}
	if p.Price != 17000 {
		t.Errorf("цена Cinesaddle = %d, ожидалось 17000", p.Price)
	}
	if _, ok := BySlug("нет-такого"); ok {
		t.Error("несуществующий slug найден")
	}
}

func TestSortPriceKeepsPricelessLast(t *testing.T) {
	for _, mode := range []string{SortPriceAsc, SortPriceDesc} {
		list := Sort(All(), mode)
		if last := list[len(list)-1]; last.Price != 0 {
			t.Errorf("%s: последним оказалось %q с ценой %d, ожидалось изделие без цены",
				mode, last.Slug, last.Price)
		}

		prev := -1
		for _, p := range list {
			if p.Price == 0 {
				continue
			}
			if prev >= 0 {
				if mode == SortPriceAsc && p.Price < prev {
					t.Errorf("%s: %d идёт после %d", mode, p.Price, prev)
				}
				if mode == SortPriceDesc && p.Price > prev {
					t.Errorf("%s: %d идёт после %d", mode, p.Price, prev)
				}
			}
			prev = p.Price
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
		{Product{}, "—"},
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
