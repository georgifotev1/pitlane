package demo

// The catalogue below is what makes a demonstration convincing: real jobs at
// Bulgarian prices, each with the supplier price the garage would actually pay.
// Margins differ on purpose - labour keeps everything, a resold part keeps the
// markup - because that contrast is the point of the dashboard.

type part struct {
	Description string
	PriceCents  int64
	CostCents   int64
}

type job struct {
	Title      string
	Parts      []part
	LaborCents int64
	// Interval is roughly how many kilometres a car covers before this job comes
	// round again; it spaces a car's history out believably.
	Interval int
}

var jobs = []job{
	{
		Title:      "Смяна на масло и филтри",
		Parts:      []part{{"Масло 5W-30 (5 л) и маслен филтър", 6500, 4100}, {"Въздушен и купеен филтър", 3400, 2000}},
		LaborCents: 3000,
		Interval:   15000,
	},
	{
		Title:      "Смяна на предни накладки",
		Parts:      []part{{"Накладки предни (комплект)", 7800, 4600}},
		LaborCents: 4500,
		Interval:   40000,
	},
	{
		Title:      "Смяна на дискове и накладки",
		Parts:      []part{{"Спирачни дискове предни (2 бр.)", 11000, 7000}, {"Накладки предни (комплект)", 7800, 4600}},
		LaborCents: 7000,
		Interval:   70000,
	},
	{
		Title:      "Ангренажен комплект и водна помпа",
		Parts:      []part{{"Ангренажен комплект", 15500, 10200}, {"Водна помпа", 8500, 5600}},
		LaborCents: 22000,
		Interval:   120000,
	},
	{
		Title:      "Смяна на съединител",
		Parts:      []part{{"Съединител комплект с лагер", 32000, 20500}},
		LaborCents: 26000,
		Interval:   150000,
	},
	{
		Title:      "Смяна на предни амортисьори",
		Parts:      []part{{"Амортисьори предни (2 бр.)", 14000, 8900}, {"Тампони и лагери", 5000, 3200}},
		LaborCents: 9000,
		Interval:   90000,
	},
	{
		Title:      "Компютърна диагностика",
		LaborCents: 4000,
		Interval:   20000,
	},
	{
		Title:      "Смяна на акумулатор",
		Parts:      []part{{"Акумулатор 70Ah", 12000, 8200}},
		LaborCents: 1500,
		Interval:   80000,
	},
	{
		Title:      "Профилактика на климатик",
		Parts:      []part{{"Фреон и купеен филтър", 3500, 1800}},
		LaborCents: 5500,
		Interval:   30000,
	},
	{
		Title:      "Смяна на ремък с ролки",
		Parts:      []part{{"Пистов ремък с обтяжна ролка", 9500, 6000}},
		LaborCents: 7000,
		Interval:   60000,
	},
	{
		Title:      "Ремонт на изпускателна система",
		Parts:      []part{{"Гърне средно", 13500, 8600}},
		LaborCents: 8000,
		Interval:   140000,
	},
	{
		Title:      "Годишно обслужване преди преглед",
		Parts:      []part{{"Чистачки и крушки", 4200, 2400}},
		LaborCents: 6000,
		Interval:   25000,
	},
}

type person struct {
	Name    string
	Company string
	Email   string
	Phone   string
	Address string
}

var customers = []person{
	{Name: "Иван Петров", Email: "ivan.petrov@example.com", Phone: "0887 412 330", Address: "гр. София, ж.к. Люлин 6, бл. 402"},
	{Name: "Мария Георгиева", Email: "m.georgieva@example.com", Phone: "0894 118 275", Address: "гр. София, ул. Козяк 12"},
	{Name: "Стоян Илиев", Company: "Стоянов Транс ЕООД", Email: "office@stoyanovtrans.bg", Phone: "0888 903 114", Address: "гр. Перник, ул. Струма 8"},
	{Name: "Елена Тодорова", Email: "elena.todorova@example.com", Phone: "0899 271 640", Address: "гр. София, бул. България 102"},
	{Name: "Петър Ангелов", Email: "p.angelov@example.com", Phone: "0876 553 218", Address: "гр. София, кв. Драгалевци"},
	{Name: "Николай Динев", Company: "Динев Строй ООД", Email: "nikolay@dinevstroy.bg", Phone: "0885 640 907", Address: "гр. София, ул. Тодор Каблешков 45"},
	{Name: "Десислава Маринова", Email: "desi.marinova@example.com", Phone: "0898 334 712", Address: "гр. Костинброд, ул. Липа 3"},
	{Name: "Красимир Ников", Email: "k.nikov@example.com", Phone: "0879 226 481", Address: "гр. София, ж.к. Младост 4"},
}

type vehicle struct {
	Customer int
	Plate    string
	VIN      string
	Make     string
	Model    string
	Year     int
	Mileage  int
}

var vehicles = []vehicle{
	{0, "CB4512BX", "WVWZZZ1KZ8W123456", "Volkswagen", "Golf V", 2008, 268000},
	{0, "CA7788MP", "WF0AXXGCDA8R12345", "Ford", "Focus", 2014, 154000},
	{1, "CB9034HT", "TMBJF25L9C6098765", "Skoda", "Octavia", 2012, 231000},
	{2, "PK1187AC", "WDB2110061A123987", "Mercedes-Benz", "E 220 CDI", 2010, 342000},
	{2, "PK4402BB", "WV1ZZZ7HZ9H054321", "Volkswagen", "Transporter T5", 2011, 411000},
	{3, "CA2260KH", "JTDKB20U803123654", "Toyota", "Corolla", 2013, 148000},
	{4, "CB6721PA", "WAUZZZ8K9BA098741", "Audi", "A4 B8", 2011, 289000},
	{5, "CA8890TX", "WBA3B11040F123654", "BMW", "320d", 2013, 224000},
	{5, "CB1503MK", "VF1RFA00X54123987", "Renault", "Megane", 2015, 132000},
	{6, "CB3345KP", "W0L0AHL3585123321", "Opel", "Astra", 2009, 276000},
	{7, "CA5519HB", "VF7DDNFPB89123456", "Citroen", "C4", 2012, 198000},
	{7, "CB7742XA", "ZFA31200000123654", "Fiat", "Punto", 2010, 213000},
}

// serviceNotes are entered by hand in the app for work that predates it or was
// done elsewhere, so a demonstration car has history from before day one.
var serviceNotes = []struct {
	Vehicle     int
	Title       string
	Description string
	MonthsAgo   int
}{
	{0, "Годишен технически преглед", "Преминат без забележки.", 14},
	{0, "Смяна на гуми - зимни", "Съхранение на летните в сервиза.", 10},
	{2, "Ремонт по застраховка", "Подмяна на предна броня след щета, извършен в друг сервиз.", 18},
	{3, "Годишен технически преглед", "Забележка за износени чистачки, подменени на място.", 13},
	{5, "Смяна на гуми - летни", "Баланс и проверка на налягането.", 7},
	{7, "Годишен технически преглед", "Преминат без забележки.", 16},
	{9, "Купен от предишен собственик", "Документирани 240 000 км при покупката.", 22},
}
