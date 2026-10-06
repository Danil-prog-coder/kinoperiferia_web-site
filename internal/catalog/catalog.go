// Package catalog содержит данные каталога «Кинопериферии» и операции над ними:
// фильтрацию по категориям, сортировку и поиск изделия по slug.
package catalog

import (
	"sort"
	"strconv"
	"strings"
)

// PriceUnknown — подпись вместо цены, пока она не согласована. Такие позиции
// остаются на витрине: покупатель видит изделие и параметры и пишет в Telegram.
const PriceUnknown = "XXX"

// PriceNegotiable — подпись у исполнений «под заказ», цена которых зависит
// от задачи и называется индивидуально.
const PriceNegotiable = "Договорная"

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
	Price    int    `json:"price"`    // цена в рублях у изделия без исполнений, 0 — цены нет
	OldPrice int    `json:"oldPrice"` // старая цена в рублях, 0 — скидки нет
	From     bool   `json:"from"`     // цена «от»
	InStock  bool   `json:"inStock"`
	Image    string `json:"image"` // пусто — фотографии ещё нет, показываем плашку
	Alt      string `json:"alt"`
	// Gallery — остальные фотографии после Image, в порядке показа в слайдере.
	Gallery []string `json:"gallery"`
	Specs   []Spec   `json:"specs"`

	// OptionNames — заголовки колонок таблицы исполнений, например
	// ["Размер", "Плотность"]. Пусто, когда исполнение одно.
	OptionNames []string `json:"optionNames"`
	// Variants — исполнения изделия. У изделия с исполнениями цена берётся
	// из них, поле Price остаётся нулевым.
	Variants []Variant `json:"variants"`

	// Featured отмечает изделия для витрины на главной — их достаточно для
	// одной сетки 4×2 без обращения к /catalog.
	Featured bool `json:"featured"`
}

// Spec — строка таблицы характеристик изделия.
type Spec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Variant — исполнение изделия: значения параметров в порядке OptionNames,
// цена и, если это исполнение выглядит иначе (другой размер, другой цвет) —
// собственная фотография. Price == 0 означает, что цена ещё не согласована.
// Image == "" означает, что у исполнения нет отдельного фото — карточка
// показывает фото изделия по умолчанию (или плашку, если и его ещё нет).
type Variant struct {
	Values []string `json:"values"`
	Price  int      `json:"price"`
	// Negotiable — цена не фиксирована и обсуждается («под заказ»); при
	// Price == 0 показываем «Договорная» вместо XXX.
	Negotiable bool   `json:"negotiable"`
	Image      string `json:"image"`
	Alt        string `json:"alt"`
	// Gallery — остальные фотографии исполнения после Image.
	Gallery []string `json:"gallery"`
}

// HasPhoto сообщает, есть ли у исполнения собственная фотография.
func (v Variant) HasPhoto() bool { return v.Image != "" }

// Photos — все фотографии исполнения в порядке показа: Image, затем Gallery.
func (v Variant) Photos() []string { return photos(v.Image, v.Gallery) }

func photos(image string, gallery []string) []string {
	if image == "" {
		return nil
	}
	return append([]string{image}, gallery...)
}

// PriceLabel — цена исполнения: «1 400 ₽» или XXX, если цены пока нет.
func (v Variant) PriceLabel() string {
	if v.Price == 0 {
		if v.Negotiable {
			return PriceNegotiable
		}
		return PriceUnknown
	}
	return FormatRub(v.Price)
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

// products — каталог в порядке вывода на витрине. Артикулы KP—01…KP—18
// закреплены за позициями и не зависят от фильтра. Фото лежат в web/static/img/.
var products = []Product{
	{
		Slug: "cinesaddle", Art: "KP—01", Name: "Синесэдл", Category: "camera",
		Image: "/static/img/cinesaddle-1.jpg", Alt: "Синесэдл, стандартный, чёрный", Gallery: []string{"/static/img/cinesaddle-2.jpg", "/static/img/cinesaddle-3.jpg", "/static/img/cinesaddle-4.jpg", "/static/img/cinesaddle-5.jpg"},
		Kicker: "Опора", Featured: true,
		Desc: "Стандартный и малый размер, четыре расцветки.",
		Detail: "Опора для съёмки с рук, капота, штатива или любой неровной поверхности. " +
			"Наполнитель не шуршит в кадре, чехол снимается для стирки, поясная фиксация " +
			"позволяет носить седло на себе между дублями.",
		InStock:     true,
		OptionNames: []string{"Размер", "Расцветка"},
		Variants: []Variant{
			{Values: []string{"Стандартный", "Чёрный"}, Price: 15500, Image: "/static/img/cinesaddle-black-1.jpg", Alt: "Синесэдл, стандартный, чёрный", Gallery: []string{"/static/img/cinesaddle-black-2.jpg", "/static/img/cinesaddle-black-3.jpg", "/static/img/cinesaddle-black-4.jpg", "/static/img/cinesaddle-black-5.jpg"}},
			{Values: []string{"Стандартный", "Клетчатый"}, Price: 15500, Image: "/static/img/cinesaddle-checkered-1.jpg", Alt: "Синесэдл, стандартный, клетчатый", Gallery: []string{"/static/img/cinesaddle-checkered-2.jpg", "/static/img/cinesaddle-checkered-3.jpg"}},
			{Values: []string{"Стандартный", "Louis Vuitton"}, Price: 15500, Image: "/static/img/cinesaddle-louis-vuitton-1.jpg", Alt: "Синесэдл, стандартный, Louis Vuitton", Gallery: []string{"/static/img/cinesaddle-louis-vuitton-2.jpg", "/static/img/cinesaddle-louis-vuitton-3.jpg", "/static/img/cinesaddle-louis-vuitton-4.jpg"}},
			{Values: []string{"Стандартный", "Плюшевый"}, Price: 15500, Image: "/static/img/cinesaddle-plush-1.jpg", Alt: "Синесэдл, стандартный, плюшевый", Gallery: []string{"/static/img/cinesaddle-plush-2.jpg", "/static/img/cinesaddle-plush-3.jpg"}},
			{Values: []string{"Малый", "Чёрный"}, Price: 15500, Image: "/static/img/cinesaddle-small-black-1.jpg", Alt: "Синесэдл, малый, чёрный", Gallery: []string{"/static/img/cinesaddle-small-black-2.jpg", "/static/img/cinesaddle-small-black-3.jpg"}},
		},
		Specs: baseSpecs(Spec{Name: "Чехол", Value: "Съёмный, стирается"}),
	},
	{
		Slug: "cinesaddle-podlokotnik", Art: "KP—02", Name: "Синесэдл «Подлокотник»", Category: "camera",
		Image: "/static/img/cinesaddle-podlokotnik-1.jpg", Alt: "Синесэдл «Подлокотник» на операторе", Gallery: []string{"/static/img/cinesaddle-podlokotnik-2.jpg"},
		Kicker: "Подлокотник",
		Desc:   "Устойчивая статика и меньшая нагрузка на руки.",
		Detail: "Версия седла под опору предплечьями: камера стоит стабильнее на длинных " +
			"статичных планах, руки устают заметно меньше.",
		InStock: true,
		Specs:   baseSpecs(Spec{Name: "Сценарий", Value: "Статика, съёмка с рук"}),
	},
	{
		Slug: "sunhood", Art: "KP—03", Name: "Санхуд", Category: "camera",
		Image: "/static/img/sunhood-1.jpg", Alt: "Санхуд на мониторе камеры", Gallery: []string{"/static/img/sunhood-2.jpg"},
		Kicker: "5″ и 7″",
		Desc:   "Козырёк на монитор: 5 и 7 дюймов, есть пошив под заказ.",
		Detail: "Козырёк на операторский монитор: убирает засветку на солнце, садится плотно " +
			"и не болтается на ходу. Два ходовых размера, нестандартную диагональ шьём под заказ.",
		InStock:     true,
		OptionNames: []string{"Диагональ"},
		Variants: []Variant{
			{Values: []string{"5 дюймов"}, Price: 6500},
			{Values: []string{"7 дюймов"}, Price: 6500},
			{Values: []string{"Под заказ"}, Negotiable: true},
		},
		Specs: baseSpecs(Spec{Name: "Диагонали", Value: "5″, 7″, под заказ"}),
	},
	{
		Slug: "dozhdevik", Art: "KP—04", Name: "Дождевик для кинокамеры", Category: "camera",
		Image: "/static/img/dozhdevik-1.jpg", Alt: "Дождевик на кинокамере", Gallery: []string{"/static/img/dozhdevik-2.jpg", "/static/img/dozhdevik-3.jpg"},
		Kicker: "Дождь и пыль", Featured: true,
		Desc: "Два размера под разный объём сетапа.",
		Detail: "Закрывает камеру на съёмке под дождём, снегом и в пыли. Два размера: малый — " +
			"под компактную сборку, большой — когда на камере навешано больше.",
		InStock:     true,
		OptionNames: []string{"Размер"},
		Variants: []Variant{
			{Values: []string{"Большой"}, Price: 5500},
			{Values: []string{"Малый"}},
		},
		Specs: baseSpecs(Spec{Name: "Размеры", Value: "Малый, большой"}),
	},
	{
		Slug: "sumka-mehanika", Art: "KP—05", Name: "Сумка механика", Category: "bags",
		Image: "/static/img/sumka-mehanika-1.jpg", Alt: "Сумка механика L, красная", Gallery: []string{"/static/img/sumka-mehanika-2.jpg", "/static/img/sumka-mehanika-3.jpg"},
		Kicker: "Механик", Featured: true,
		Desc: "Два размера, три цвета, наполнение под ваш комплект.",
		Detail: "Шьём под конкретный набор инструмента и расходников. Два размера — L и " +
			"увеличенный XL под больший комплект. Число отделений и раскладку карманов " +
			"согласуем до пошива — по фотографиям и размерам вашего комплекта.",
		InStock:     true,
		OptionNames: []string{"Размер", "Цвет"},
		Variants: []Variant{
			{Values: []string{"L", "Красная"}, Price: 33000, Image: "/static/img/sumka-mehanika-l-red-1.jpg", Alt: "Сумка механика L, красная", Gallery: []string{"/static/img/sumka-mehanika-l-red-2.jpg", "/static/img/sumka-mehanika-l-red-3.jpg"}},
			{Values: []string{"XL", "Чёрно-синяя"}, Price: 35000, Image: "/static/img/sumka-mehanika-xl-black-blue-1.jpg", Alt: "Сумка механика XL, чёрно-синяя", Gallery: []string{"/static/img/sumka-mehanika-xl-black-blue-2.jpg", "/static/img/sumka-mehanika-xl-black-blue-3.jpg", "/static/img/sumka-mehanika-xl-black-blue-4.jpg", "/static/img/sumka-mehanika-xl-black-blue-5.jpg", "/static/img/sumka-mehanika-xl-black-blue-6.jpg"}},
			{Values: []string{"L", "Жёлто-зелёная"}, Price: 33000, Image: "/static/img/sumka-mehanika-l-green-1.jpg", Alt: "Сумка механика L, жёлто-зелёная", Gallery: []string{"/static/img/sumka-mehanika-l-green-2.jpg", "/static/img/sumka-mehanika-l-green-3.jpg"}},
			{Values: []string{"XL", "Жёлто-зелёная"}, Price: 35000, Image: "/static/img/sumka-mehanika-xl-green-1.jpg", Alt: "Сумка механика XL, жёлто-зелёная", Gallery: []string{"/static/img/sumka-mehanika-xl-green-2.jpg", "/static/img/sumka-mehanika-xl-green-3.jpg"}},
		},
		Specs: baseSpecs(Spec{Name: "Размеры", Value: "L, XL"}, Spec{Name: "Срок", Value: "Согласуем при заказе"}),
	},
	{
		Slug: "kosmetichka-monitor", Art: "KP—07", Name: "Косметичка для монитора", Category: "bags",
		Image: "/static/img/kosmetichka-monitor-1.jpg", Alt: "Косметички для монитора", Gallery: []string{"/static/img/kosmetichka-monitor-2.jpg", "/static/img/kosmetichka-monitor-3.jpg", "/static/img/kosmetichka-monitor-4.jpg", "/static/img/kosmetichka-monitor-5.jpg", "/static/img/kosmetichka-monitor-6.jpg"},
		Kicker: "Для монитора",
		Desc:   "5 и 7 дюймов, четыре цвета на каждую диагональ.",
		Detail: "Мягкий чехол под операторский монитор: бережёт экран от царапин и пыли в " +
			"сумке. Две диагонали, цвет выбирается при заказе.",
		InStock:     true,
		OptionNames: []string{"Диагональ", "Цвет"},
		Variants: []Variant{
			{Values: []string{"5 дюймов", "Синяя"}, Price: 2900},
			{Values: []string{"5 дюймов", "Розовая"}, Price: 2900, Image: "/static/img/kosmetichka-monitor-5-pink-1.jpg", Alt: "Косметичка для монитора 5 дюймов, розовая", Gallery: []string{"/static/img/kosmetichka-monitor-5-pink-2.jpg"}},
			{Values: []string{"5 дюймов", "Красная"}, Price: 2900},
			{Values: []string{"5 дюймов", "Зелёная"}, Price: 2900},
			{Values: []string{"7 дюймов", "Синяя"}, Price: 3200},
			{Values: []string{"7 дюймов", "Розовая"}, Price: 3200, Image: "/static/img/kosmetichka-monitor-7-pink-1.jpg", Alt: "Косметичка для монитора 7 дюймов, розовая"},
			{Values: []string{"7 дюймов", "Красная"}, Price: 3200},
			{Values: []string{"7 дюймов", "Фиолетовая"}, Price: 3200, Image: "/static/img/kosmetichka-monitor-7-purple-1.jpg", Alt: "Косметичка для монитора 7 дюймов, фиолетовая", Gallery: []string{"/static/img/kosmetichka-monitor-7-purple-2.jpg"}},
		},
		Specs: baseSpecs(Spec{Name: "Размеры", Value: "18 × 10 см (5″), 23 × 13 × 5 см (7″)"}),
	},
	{
		Slug: "kofr-dlya-shtativa", Art: "KP—08", Name: "Кофр для штатива", Category: "bags",
		Image: "/static/img/kofr-dlya-shtativa-1.jpg", Alt: "Кофр для штатива Sachtler", Gallery: []string{"/static/img/kofr-dlya-shtativa-2.jpg"},
		Kicker: "Sachtler", Featured: true,
		Desc: "Под Sachtler и под ваш штатив.",
		Detail: "Кофр под штатив: готовое лекало под Sachtler, остальные модели шьём по " +
			"размерам вашего штатива с головой.",
		InStock:     true,
		OptionNames: []string{"Исполнение"},
		Variants: []Variant{
			{Values: []string{"Sachtler"}, Price: 25000},
			{Values: []string{"Под заказ"}, Negotiable: true},
		},
		Specs: baseSpecs(Spec{Name: "Изготовление", Value: "Готовое лекало или под заказ"}),
	},
	{
		Slug: "organizer-pelican", Art: "KP—09", Name: "Органайзер в Pelican", Category: "bags",
		Image: "/static/img/organizer-pelican-1.jpg", Alt: "Органайзер в кейсе Pelican", Gallery: []string{"/static/img/organizer-pelican-2.jpg", "/static/img/organizer-pelican-3.jpg"},
		Kicker: "Pelican",
		Desc:   "Под 1615, 1535 и любой другой кейс.",
		Detail: "Тканевое наполнение вместо поролона: перегородки переставляются под новую " +
			"оптику или монитор, карманы снимаются. Готовые размеры под Pelican 1615 и 1535, " +
			"остальные кейсы — по внутренним размерам.",
		InStock:     true,
		OptionNames: []string{"Кейс"},
		Variants: []Variant{
			{Values: []string{"Pelican 1615"}, Price: 21000},
			{Values: []string{"Pelican 1535"}, Price: 15000},
			{Values: []string{"Под заказ"}, Negotiable: true},
		},
		Specs: baseSpecs(Spec{Name: "Изготовление", Value: "По внутренним размерам кейса"}),
	},
	{
		Slug: "sumka-easyrig", Art: "KP—10", Name: "Сумка Easyrig", Category: "bags",
		Image: "/static/img/sumka-easyrig-1.jpg", Alt: "Сумка для Easyrig", Gallery: []string{"/static/img/sumka-easyrig-2.jpg", "/static/img/sumka-easyrig-3.jpg"},
		Kicker: "Easyrig",
		Desc:   "Переноска и хранение системы между съёмками.",
		Detail: "Сумка под Easyrig: система едет в собранном виде и не цепляется за остальное " +
			"оборудование в машине и на площадке.",
		Price: 17500, InStock: true,
		Specs: baseSpecs(),
	},
	{
		Slug: "sumka-dlya-stoek", Art: "KP—11", Name: "Сумка для стоек", Category: "bags",
		Image: "/static/img/sumka-dlya-stoek-1.jpg", Alt: "Сумка для стоек", Gallery: []string{"/static/img/sumka-dlya-stoek-2.jpg"},
		Kicker: "Стойки",
		Desc:   "Переноска комплекта стоек одним местом.",
		Detail: "Длинная сумка под осветительные стойки: комплект едет одним местом, " +
			"ручки и лямка рассчитаны на вес набора.",
		Price: 14700, InStock: true,
		Specs: baseSpecs(),
	},
	{
		Slug: "soty-titan", Art: "KP—12", Name: "Соты для Titan", Category: "light",
		Image: "/static/img/soty-titan-1.jpg", Alt: "Соты для Titan",
		Kicker: "4 размера", Featured: true,
		Desc: "120, 103, 60 и 30 см — по три исполнения в каждом.",
		Detail: "Сотовые насадки для света Titan. Четыре размера — 120, 103, 60 и 30 см, " +
			"в каждом три исполнения: Full, 1/2 и 1/4. Цена зависит от размера и исполнения, " +
			"полная таблица — ниже.",
		InStock:     true,
		OptionNames: []string{"Размер", "Исполнение"},
		Variants: []Variant{
			{Values: []string{"120 см", "Full"}, Price: 1400},
			{Values: []string{"120 см", "1/2"}, Price: 1200},
			{Values: []string{"120 см", "1/4"}, Price: 1000},
			{Values: []string{"103 см", "Full"}, Price: 1300},
			{Values: []string{"103 см", "1/2"}, Price: 1100},
			{Values: []string{"103 см", "1/4"}, Price: 900},
			{Values: []string{"60 см", "Full"}, Price: 750},
			{Values: []string{"60 см", "1/2"}, Price: 650},
			{Values: []string{"60 см", "1/4"}, Price: 550},
			{Values: []string{"30 см", "Full"}, Price: 400},
			{Values: []string{"30 см", "1/2"}, Price: 350},
			{Values: []string{"30 см", "1/4"}, Price: 300},
		},
		Specs: baseSpecs(
			Spec{Name: "Размеры", Value: "120, 103, 60, 30 см"},
			Spec{Name: "Исполнения", Value: "Full, 1/2, 1/4"},
		),
	},
	{
		Slug: "soty-ulanzi", Art: "KP—13", Name: "Соты для Ulanzi", Category: "light",
		Image: "/static/img/soty-ulanzi-1.jpg", Alt: "Соты для Ulanzi", Gallery: []string{"/static/img/soty-ulanzi-2.jpg", "/static/img/soty-ulanzi-3.jpg"},
		Kicker: "Ulanzi",
		Desc:   "Под модели AL60 и AL120.",
		Detail: "Сотовые насадки под осветители Ulanzi AL60 и AL120: сужают луч и убирают " +
			"засветку в стороны.",
		InStock:     true,
		OptionNames: []string{"Модель"},
		Variants: []Variant{
			{Values: []string{"AL60"}, Price: 8500},
			{Values: []string{"AL120"}, Price: 11500},
		},
		Specs: baseSpecs(Spec{Name: "Совместимость", Value: "Ulanzi AL60, AL120"}),
	},
	{
		Slug: "soty-ramy", Art: "KP—14", Name: "Соты для рам", Category: "light",
		Image: "/static/img/soty-ramy-1.jpg", Alt: "Соты на раме",
		Kicker: "Для рам",
		Desc:   "Четыре типоразмера: от 4 до 20 футов.",
		Detail: "Соты на съёмочные рамы: 4, 8, 12 и 20 футов. Ставятся на раму и дают " +
			"направленный, контролируемый свет без разлёта в стороны.",
		InStock:     true,
		OptionNames: []string{"Размер рамы"},
		Variants: []Variant{
			{Values: []string{"4 фута"}, Price: 9000},
			{Values: []string{"8 футов"}},
			{Values: []string{"12 футов"}},
			{Values: []string{"20 футов"}},
		},
		Specs: baseSpecs(Spec{Name: "Размеры", Value: "4, 8, 12, 20 футов"}),
	},
	{
		Slug: "checkerboard", Art: "KP—15", Name: "Checkerboard", Category: "light",
		Image: "/static/img/checkerboard-1.jpg", Alt: "Checkerboard на стойке", Gallery: []string{"/static/img/checkerboard-2.jpg"},
		Kicker: "4×4, 8×8 и 12×12", Featured: true,
		Desc: "Отражатель-шахматка, серебро или золото.",
		Detail: "Отражатель «шахматка» на раму: даёт рассеянный отражённый свет мягче " +
			"зеркального серебра. Размеры 4×4, 8×8 и 12×12; у больших — две стороны, серебро и золото.",
		InStock:     true,
		OptionNames: []string{"Размер", "Сторона"},
		Variants: []Variant{
			{Values: []string{"8×8", "Silver"}, Price: 35000},
			{Values: []string{"8×8", "Gold"}, Price: 35000},
			{Values: []string{"12×12", "Silver"}, Price: 46000},
			{Values: []string{"12×12", "Gold"}, Price: 46000},
			{Values: []string{"4×4", "Silver / Gold"}, Price: 15000, Image: "/static/img/checkerboard-4x4-1.jpg", Alt: "Checkerboard 4×4 на стойке", Gallery: []string{"/static/img/checkerboard-4x4-2.jpg"}},
		},
		Specs: baseSpecs(
			Spec{Name: "4×4", Value: "120 × 120 см"},
			Spec{Name: "8×8", Value: "230 × 230 см"},
			Spec{Name: "12×12", Value: "350 × 350 см"},
		),
	},
	{
		Slug: "floppy-120", Art: "KP—16", Name: "Флоппи флаг 120 × 120", Category: "light",
		Image: "/static/img/floppy-120-1.jpg", Alt: "Флоппи флаг 120 × 120", Gallery: []string{"/static/img/floppy-120-2.jpg", "/static/img/floppy-120-3.jpg", "/static/img/floppy-120-4.jpg", "/static/img/floppy-120-5.jpg"},
		Kicker: "С юбкой",
		Desc:   "Отсечка света, съёмный светонепроницаемый текстиль.",
		Detail: "Флаг для отсечки света с юбкой: светонепроницаемый текстиль снимается с рамы, " +
			"поэтому его удобно возить отдельно и менять при износе.",
		Price: 17000, InStock: true, Featured: true,
		Specs: baseSpecs(
			Spec{Name: "Размер", Value: "120 × 120 см"},
			Spec{Name: "Комплектация", Value: "С юбкой"},
			Spec{Name: "Текстиль", Value: "Съёмный"},
		),
	},
	{
		Slug: "dedolight-effect-50", Art: "KP—17", Name: "Аналог Dedolight Effect 50 × 50", Category: "light",
		Image: "/static/img/dedolight-effect-50-1.jpg", Alt: "Аналог Dedolight Effect 50 × 50", Gallery: []string{"/static/img/dedolight-effect-50-2.jpg", "/static/img/dedolight-effect-50-3.jpg", "/static/img/dedolight-effect-50-4.jpg"},
		Kicker: "50 × 50",
		Desc:   "Насадка для световых эффектов.",
		Detail: "Аналог Dedolight Effect в размере 50 × 50 см — насадка для световых эффектов " +
			"на площадке.",
		Price: 15500, InStock: true,
		Specs: baseSpecs(Spec{Name: "Размер", Value: "50 × 50 см"}, Spec{Name: "Чехол", Value: "5 000 ₽, отдельно"}),
	},
	{
		Slug: "sandbag", Art: "KP—18", Name: "Сандбег 12 кг", Category: "set",
		Image: "/static/img/sandbag-1.jpg", Alt: "Сандбег 12 кг", Gallery: []string{"/static/img/sandbag-2.jpg", "/static/img/sandbag-3.jpg"},
		Kicker: "12 кг",
		Desc:   "Две ручки, два кольца и защищённая молния.",
		Detail: "Противовес для стоек и журавлей: две ручки для переноски, два кольца для " +
			"подвеса, молния закрыта клапаном от пыли и зацепов.",
		Price: 3900, InStock: true, Featured: true,
		Specs: baseSpecs(Spec{Name: "Вес", Value: "12 кг"}, Spec{Name: "Партия", Value: "10 шт — 37 000 ₽"}),
	},
	{
		Slug: "organizer-prozrachnyj", Art: "KP—19", Name: "Органайзер прозрачный", Category: "bags",
		Image: "/static/img/organizer-prozrachnyj-1.jpg", Alt: "Прозрачный органайзер", Gallery: []string{"/static/img/organizer-prozrachnyj-2.jpg", "/static/img/organizer-prozrachnyj-3.jpg"},
		Kicker:      "Прозрачный",
		Desc:        "Из армированной плёнки и из обычной.",
		Detail:      "Прозрачный органайзер на молнии: содержимое видно без раскрытия. Две версии — из армированной плёнки и из обычной.",
		InStock:     true,
		OptionNames: []string{"Плёнка"},
		Variants: []Variant{
			{Values: []string{"Армированная плёнка"}, Price: 1500, Image: "/static/img/organizer-prozrachnyj-armirovannaya-1.jpg", Alt: "Прозрачный органайзер из армированной плёнки", Gallery: []string{"/static/img/organizer-prozrachnyj-armirovannaya-2.jpg"}},
			{Values: []string{"Обычная плёнка"}, Price: 1500, Image: "/static/img/organizer-prozrachnyj-obychnaya-1.jpg", Alt: "Прозрачный органайзер из обычной плёнки"},
		},
		Specs: baseSpecs(),
	},
	{
		Slug: "ryukzak-kino-foto", Art: "KP—20", Name: "Рюкзак для кино-, фото- и аудиооборудования", Category: "bags",
		Image: "/static/img/ryukzak-kino-foto-1.jpg", Alt: "Рюкзак для кино-, фото- и аудиооборудования", Gallery: []string{"/static/img/ryukzak-kino-foto-2.jpg", "/static/img/ryukzak-kino-foto-3.jpg", "/static/img/ryukzak-kino-foto-4.jpg"},
		Kicker:  "20×31×50 см",
		Desc:    "Рюкзак под технику с раскладкой отделений.",
		Detail:  "Рюкзак для переноски кино-, фото- и аудиооборудования. Внутри раскладываемые отделения, снаружи карманы.",
		InStock: true,
		Price:   35000,
		Specs:   baseSpecs(Spec{Name: "Размеры", Value: "20 × 31 × 50 см"}),
	},
	{
		Slug: "razgruzka-zhilet", Art: "KP—21", Name: "Разгрузка-жилет для видеооборудования", Category: "camera",
		Image: "/static/img/razgruzka-zhilet-1.jpg", Alt: "Разгрузка-жилет для видеооборудования", Gallery: []string{"/static/img/razgruzka-zhilet-2.jpg", "/static/img/razgruzka-zhilet-3.jpg", "/static/img/razgruzka-zhilet-4.jpg"},
		Kicker:  "Для трансляций",
		Desc:    "Жилет-разгрузка под видеотехнику и трансляции.",
		Detail:  "Жилет-разгрузка для размещения видеооборудования и техники для трансляций.",
		InStock: true,
		Price:   9500,
		Specs:   baseSpecs(),
	},
	{
		Slug: "peli-1535-air", Art: "KP—22", Name: "Peli 1535 Air", Category: "bags",
		Image: "/static/img/peli-1535-air-1.jpg", Alt: "Кейс Peli 1535 Air", Gallery: []string{"/static/img/peli-1535-air-2.jpg", "/static/img/peli-1535-air-3.jpg", "/static/img/peli-1535-air-4.jpg"},
		Kicker:  "Кейс",
		Desc:    "Новый кейс Peli 1535 Air, любая модель под заказ.",
		Detail:  "Новый жёсткий кейс Peli 1535 Air. Любую другую модель Peli привезём под заказ.",
		InStock: true,
		Price:   36000,
		Specs:   []Spec{{Name: "Состояние", Value: "Новый"}, {Name: "Другие модели", Value: "Под заказ"}, {Name: "Получение", Value: "Самовывоз / доставка / ТК"}},
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

// Featured возвращает изделия для витрины на главной — те, что отмечены
// Featured: true, в порядке каталога. Если ни одно не отмечено, возвращает
// первые n изделий, чтобы витрина не осталась пустой.
func Featured(n int) []Product {
	out := make([]Product, 0, n)
	for _, p := range products {
		if p.Featured {
			out = append(out, p)
			if len(out) == n {
				return out
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	all := All()
	if len(all) > n {
		all = all[:n]
	}
	return all
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
	pa, pb := a.SortPrice(), b.SortPrice()
	switch {
	case pa == 0 && pb == 0:
		return false
	case pa == 0:
		return false
	case pb == 0:
		return true
	case asc:
		return pa < pb
	default:
		return pa > pb
	}
}

// HasVariants сообщает, есть ли у изделия таблица исполнений.
func (p Product) HasVariants() bool { return len(p.Variants) > 0 }

// HasPhoto сообщает, есть ли у изделия фотография. Пока съёмки нет, витрина
// показывает вместо неё плашку с названием.
func (p Product) HasPhoto() bool { return p.Image != "" }

// Photos — все фотографии изделия в порядке показа: Image, затем Gallery.
func (p Product) Photos() []string { return photos(p.Image, p.Gallery) }

// MinPrice — наименьшая известная цена изделия: по исполнениям, а если их нет —
// собственная цена. 0 означает, что цены нет ни у одного исполнения.
func (p Product) MinPrice() int {
	if !p.HasVariants() {
		return p.Price
	}
	min := 0
	for _, v := range p.Variants {
		if v.Price == 0 {
			continue
		}
		if min == 0 || v.Price < min {
			min = v.Price
		}
	}
	return min
}

// MaxPrice — наибольшая известная цена изделия.
func (p Product) MaxPrice() int {
	if !p.HasVariants() {
		return p.Price
	}
	max := 0
	for _, v := range p.Variants {
		if v.Price > max {
			max = v.Price
		}
	}
	return max
}

// SortPrice — цена, по которой изделие участвует в сортировке витрины.
func (p Product) SortPrice() int { return p.MinPrice() }

// PricedVariants — число исполнений с известной ценой.
func (p Product) PricedVariants() int {
	n := 0
	for _, v := range p.Variants {
		if v.Price > 0 {
			n++
		}
	}
	return n
}

// HasUnknownPrice сообщает, что у изделия или у части его исполнений цены
// пока нет — витрина показывает XXX и предлагает уточнить её в Telegram.
func (p Product) HasUnknownPrice() bool {
	if p.HasVariants() {
		for _, v := range p.Variants {
			if v.Price == 0 && !v.Negotiable {
				return true
			}
		}
		return false
	}
	return p.Price == 0
}

// PriceLabel форматирует цену так, как она показана на витрине: «15 500 ₽»,
// «от 300 ₽» — когда исполнения стоят по-разному, и XXX — пока цены нет.
func (p Product) PriceLabel() string {
	min := p.MinPrice()
	if min == 0 {
		return PriceUnknown
	}
	if p.From || min != p.MaxPrice() || p.PricedVariants() != len(p.Variants) {
		return "от " + FormatRub(min)
	}
	return FormatRub(min)
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
