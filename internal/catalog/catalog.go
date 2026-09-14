// Package catalog содержит данные каталога «Кинопериферии» и операции над ними:
// фильтрацию по категориям, сортировку и поиск изделия по slug.
package catalog

import (
	"sort"
	"strconv"
	"strings"
)

// Category — раздел каталога. Slug используется в URL, Title — в интерфейсе.
type Category struct {
	Slug  string
	Title string
}

// Categories перечислены в том порядке, в котором выводятся фильтры.
var Categories = []Category{
	{Slug: "all", Title: "Все"},
	{Slug: "camera", Title: "Для камеры"},
	{Slug: "bags", Title: "Сумки и кейсы"},
	{Slug: "light", Title: "Для света"},
	{Slug: "set", Title: "Для площадки"},
}

// Product — изделие каталога.
type Product struct {
	Slug     string `json:"slug"`
	Art      string `json:"art"`      // артикул вида KP—01
	Name     string `json:"name"`     // название изделия
	Category string `json:"category"` // slug категории
	Kicker   string `json:"kicker"`   // подпись на плашке карточки
	Desc     string `json:"desc"`     // короткое описание в карточке
	Detail   string `json:"detail"`   // развёрнутое описание на странице изделия
	Price    int    `json:"price"`    // цена в рублях, 0 — цены нет
	OldPrice int    `json:"oldPrice"` // старая цена в рублях, 0 — скидки нет
	From     bool   `json:"from"`     // цена «от»
	InStock  bool   `json:"inStock"`
	Image    string `json:"image"`
	Alt      string `json:"alt"`
	Specs    []Spec `json:"specs"`
}

// Spec — строка таблицы характеристик изделия.
type Spec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// baseSpecs повторяются у всех изделий: производство и условия одинаковые.
func baseSpecs(extra ...Spec) []Spec {
	s := []Spec{
		{Name: "Материалы", Value: "Cordura, фурнитура YKK"},
		{Name: "Производство", Value: "Москва, собственный цех"},
		{Name: "Гарантия", Value: "1 год"},
		{Name: "Получение", Value: "Самовывоз / доставка / ТК"},
	}
	return append(s, extra...)
}

// products — каталог в порядке вывода на витрине. Артикулы KP—01…KP—12
// закреплены за позициями и не зависят от фильтра.
var products = []Product{
	{
		Slug: "cinesaddle", Art: "KP—01", Name: "Cinesaddle", Category: "camera",
		Kicker: "Для камеры",
		Desc:   "Тихий наполнитель, съёмный чехол, поясная фиксация.",
		Detail: "Опора для съёмки с рук, капота, штатива или любой неровной поверхности. " +
			"Наполнитель не шуршит в кадре, чехол снимается для стирки, поясная фиксация " +
			"позволяет носить седло на себе между дублями.",
		Price: 17000, InStock: true,
		Image: "https://static.tildacdn.com/tild6562-3831-4536-b265-333132623033/photo_2024-10-06_11-.jpg",
		Alt:   "Чёрный Cinesaddle Кинопериферия",
		Specs: baseSpecs(Spec{Name: "Чехол", Value: "Съёмный, стирается"}),
	},
	{
		Slug: "sumka-mehanika", Art: "KP—02", Name: "Сумка механика", Category: "bags",
		Kicker: "Индивидуально",
		Desc:   "Размер и наполнение под комплект заказчика.",
		Detail: "Шьём под конкретный набор инструмента и расходников. Габариты, число " +
			"отделений и раскладку карманов согласуем до пошива — по фотографиям и размерам " +
			"вашего комплекта.",
		Price: 35000, From: true, InStock: true,
		Image: "https://static.tildacdn.com/tild3162-3936-4038-b730-653463353563/photo_2024-10-06_11-.jpg",
		Alt:   "Сумка механика с жёлтыми перегородками",
		Specs: baseSpecs(Spec{Name: "Срок", Value: "Согласуем при заказе"}),
	},
	{
		Slug: "napolnenie-v-keys", Art: "KP—03", Name: "Наполнение в кейс", Category: "bags",
		Kicker: "Под ваш кейс",
		Desc:   "Переставные перегородки и съёмные карманы.",
		Detail: "Тканевое наполнение вместо поролона: перегородки переставляются под новую " +
			"оптику или монитор, карманы снимаются. Изготавливаем по внутренним размерам " +
			"вашего кейса.",
		Price: 15000, From: true, InStock: true,
		Image: "https://static.tildacdn.com/tild6638-6538-4637-a666-316238646664/photo_2024-10-06_11-.jpg",
		Alt:   "Жёлтое тканевое наполнение в кейс",
		Specs: baseSpecs(Spec{Name: "Изготовление", Value: "По размерам кейса"}),
	},
	{
		Slug: "sunhood", Art: "KP—04", Name: "Sunhood", Category: "camera",
		Kicker: "Для монитора",
		Desc:   "Плотная фиксация и съёмный удлинитель.",
		Detail: "Козырёк на операторский монитор: убирает засветку на солнце, садится плотно " +
			"и не болтается на ходу. Удлинитель снимается, когда нужен компактный профиль.",
		Price: 6500, InStock: true,
		Image: "https://static.tildacdn.com/tild3335-6432-4534-b436-363635303562/IMG_3662.jpg",
		Alt:   "Козырёк Sunhood на операторском мониторе",
		Specs: baseSpecs(Spec{Name: "Удлинитель", Value: "Съёмный"}),
	},
	{
		Slug: "cinesaddle-podlokotnik", Art: "KP—05", Name: "Cinesaddle «Подлокотник»", Category: "camera",
		Kicker: "Для камеры",
		Desc:   "Устойчивая статика и меньшая нагрузка на руки.",
		Detail: "Версия седла под опору предплечьями: камера стоит стабильнее на длинных " +
			"статичных планах, руки устают заметно меньше.",
		Price: 17500, InStock: true,
		Image: "https://static.tildacdn.com/tild3231-3162-4566-b761-366530623731/photo_2024-10-06_11-.jpg",
		Alt:   "Оператор снимает с опорой Cinesaddle Подлокотник",
		Specs: baseSpecs(Spec{Name: "Сценарий", Value: "Статика, съёмка с рук"}),
	},
	{
		Slug: "meshok-s-drobyu", Art: "KP—06", Name: "Мешок с дробью", Category: "set",
		Kicker: "12 кг",
		Desc:   "Две ручки, два кольца и защищённая молния.",
		Detail: "Противовес для стоек и журавлей: стальная дробь, две ручки для переноски, " +
			"два кольца для подвеса, молния закрыта клапаном от пыли и зацепов.",
		Price: 3900, InStock: true,
		Image: "https://static.tildacdn.com/tild3362-6230-4166-a138-396339643032/photo_2024-10-06_11-.jpg",
		Alt:   "Чёрно-жёлтый мешок со стальной дробью",
		Specs: baseSpecs(Spec{Name: "Вес", Value: "12 кг"}, Spec{Name: "Наполнитель", Value: "Стальная дробь"}),
	},
	{
		Slug: "soty-led-tube-2ft", Art: "KP—07", Name: "Соты для LED tube", Category: "light",
		Kicker: "2 фута",
		Desc:   "Три плотности и быстросъёмное крепление.",
		Detail: "Сотовые насадки на двухфутовые светодиодные трубки. Три плотности под разный " +
			"угол отсечки, крепление ставится и снимается без инструмента.",
		Price: 5000, InStock: true,
		Image: "https://static.tildacdn.com/tild6661-6161-4535-a462-393639356265/photo_2024-10-06_11-.jpg",
		Alt:   "Текстильные соты для светодиодной трубки 2 фута",
		Specs: baseSpecs(Spec{Name: "Длина", Value: "2 фута"}, Spec{Name: "Плотность", Value: "Три варианта"}),
	},
	{
		Slug: "soty-led-tube-4ft", Art: "KP—08", Name: "Соты для LED tube", Category: "light",
		Kicker: "4 фута",
		Desc:   "Для четырёхфутовых трубок Astera и аналогов.",
		Detail: "Тот же тип сот под четырёхфутовые трубки Astera и совместимые модели. " +
			"Плотность подбираем под задачу — от мягкой отсечки до жёсткого контроля луча.",
		Price: 6000, InStock: true,
		Image: "https://static.tildacdn.com/tild3632-3832-4231-a530-663132316639/photo_2024-10-06_11-.jpg",
		Alt:   "Текстильные соты для светодиодной трубки 4 фута",
		Specs: baseSpecs(Spec{Name: "Длина", Value: "4 фута"}, Spec{Name: "Совместимость", Value: "Astera и аналоги"}),
	},
	{
		Slug: "floppy-120", Art: "KP—09", Name: "Флоппи 120 × 120", Category: "light",
		Kicker: "Флаг",
		Desc:   "Съёмный светонепроницаемый текстиль.",
		Detail: "Флаг для отсечки света: светонепроницаемый текстиль снимается с рамы, " +
			"поэтому его удобно возить отдельно и менять при износе.",
		Price: 15000, InStock: true,
		Image: "https://static.tildacdn.com/tild3862-6436-4064-b239-386263636664/photo.jpeg",
		Alt:   "Чёрный флоппи 120 на 120 сантиметров",
		Specs: baseSpecs(Spec{Name: "Размер", Value: "120 × 120 см"}, Spec{Name: "Текстиль", Value: "Съёмный"}),
	},
	{
		Slug: "organizer", Art: "KP—10", Name: "Органайзер", Category: "set",
		Kicker: "Органайзер",
		Desc:   "Для расходников, кабелей и мелкого оборудования.",
		Detail: "Компактный органайзер на молнии под скотчи, стяжки, переходники и кабели. " +
			"Помещается в сумку механика и в тележку.",
		Price: 2000, InStock: true,
		Image: "https://static.tildacdn.com/tild3965-3761-4936-b437-383531646132/photo_2024-10-06_11-.jpg",
		Alt:   "Компактный прозрачный органайзер на молнии",
		Specs: baseSpecs(Spec{Name: "Застёжка", Value: "Молния YKK"}),
	},
	{
		Slug: "chehol-filtry", Art: "KP—11", Name: "Чехол для фильтров 4 × 5,65", Category: "camera",
		Kicker: "Sale",
		Desc:   "Защита от царапин, пыли и лёгких ударов.",
		Detail: "Мягкий чехол под светофильтры формата 4 × 5,65 дюйма. Бережёт стекло от " +
			"царапин и пыли в сумке и смягчает случайные удары.",
		Price: 1500, OldPrice: 2500, InStock: true,
		Image: "https://static.tildacdn.com/tild3539-3036-4861-a363-333263663331/photo.jpeg",
		Alt:   "Жёлтый защитный чехол для светофильтров",
		Specs: baseSpecs(Spec{Name: "Формат", Value: "4 × 5,65 дюйма"}),
	},
	{
		Slug: "skladnaya-telezhka", Art: "KP—12", Name: "Складная тележка", Category: "set",
		Kicker: "Нет в наличии",
		Desc:   "Полки, съёмные крючки и складная рама.",
		Detail: "Тележка для переброски оборудования по площадке: две полки, съёмные крючки " +
			"под кабели, рама складывается для перевозки. Сроки уточняйте в Telegram.",
		InStock: false,
		Image:   "https://static.tildacdn.com/tild3964-6364-4865-a463-303465653639/photo_2024-10-06_11-.jpg",
		Alt:     "Складная тележка с двумя полками для оборудования",
		Specs:   baseSpecs(Spec{Name: "Полки", Value: "Две"}, Spec{Name: "Рама", Value: "Складная"}),
	},
}

// All возвращает копию каталога, чтобы вызывающий код не менял исходные данные.
func All() []Product {
	out := make([]Product, len(products))
	copy(out, products)
	return out
}

// Count — число изделий в каталоге.
func Count() int { return len(products) }

// ByCategory возвращает изделия категории. Пустой slug и "all" дают весь каталог;
// неизвестный slug — пустой список.
func ByCategory(slug string) []Product {
	if slug == "" || slug == "all" {
		return All()
	}
	out := make([]Product, 0, len(products))
	for _, p := range products {
		if p.Category == slug {
			out = append(out, p)
		}
	}
	return out
}

// BySlug ищет изделие по slug.
func BySlug(slug string) (Product, bool) {
	for _, p := range products {
		if p.Slug == slug {
			return p, true
		}
	}
	return Product{}, false
}

// CategoryTitle возвращает название категории по slug.
func CategoryTitle(slug string) string {
	for _, c := range Categories {
		if c.Slug == slug {
			return c.Title
		}
	}
	return ""
}

// IsCategory сообщает, известен ли slug категории.
func IsCategory(slug string) bool { return CategoryTitle(slug) != "" }

// Сортировки, доступные в каталоге.
const (
	SortDefault   = "default"
	SortPriceAsc  = "price-asc"
	SortPriceDesc = "price-desc"
	SortName      = "name"
)

// Sort сортирует список на месте и возвращает его же. Изделия без цены всегда
// уходят в конец при сортировке по цене — у них нечего сравнивать.
func Sort(list []Product, mode string) []Product {
	switch mode {
	case SortPriceAsc:
		sort.SliceStable(list, func(i, j int) bool {
			return lessByPrice(list[i], list[j], true)
		})
	case SortPriceDesc:
		sort.SliceStable(list, func(i, j int) bool {
			return lessByPrice(list[i], list[j], false)
		})
	case SortName:
		sort.SliceStable(list, func(i, j int) bool {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		})
	}
	return list
}

func lessByPrice(a, b Product, asc bool) bool {
	switch {
	case a.Price == 0 && b.Price == 0:
		return false
	case a.Price == 0:
		return false
	case b.Price == 0:
		return true
	case asc:
		return a.Price < b.Price
	default:
		return a.Price > b.Price
	}
}

// PriceLabel форматирует цену так, как она показана на витрине: «от 35 000 ₽»,
// «17 000 ₽» или «—», если цены нет.
func (p Product) PriceLabel() string {
	if p.Price == 0 {
		return "—"
	}
	if p.From {
		return "от " + FormatRub(p.Price)
	}
	return FormatRub(p.Price)
}

// OldPriceLabel возвращает зачёркнутую цену или пустую строку, если скидки нет.
func (p Product) OldPriceLabel() string {
	if p.OldPrice == 0 {
		return ""
	}
	return FormatRub(p.OldPrice)
}

// CategoryTitle — название категории изделия.
func (p Product) CategoryTitle() string { return CategoryTitle(p.Category) }

// Tag — надпись над названием на странице изделия. Когда плашка дублирует
// категорию, показываем нейтральное «Изделие каталога».
func (p Product) Tag() string {
	if p.Kicker == p.CategoryTitle() {
		return "Изделие каталога"
	}
	return p.Kicker
}

// FormatRub печатает сумму с неразрывными тонкими пробелами между разрядами
// и знаком рубля: 17000 → «17 000 ₽».
func FormatRub(v int) string {
	s := strconv.Itoa(v)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	b.WriteString(" ₽")
	return b.String()
}
