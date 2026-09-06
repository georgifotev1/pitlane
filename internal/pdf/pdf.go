package pdf

import (
	"fmt"
	"strconv"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/pagesize"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/core/entity"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

type OfferData struct {
	Tenant   *domain.Tenant
	Customer *domain.Customer
	Car      *domain.Car
	Offer    *domain.Offer
}

// Embedded Go fonts provide Cyrillic support without runtime files.
const fontFamily = "go"

const (
	lblTitle    = "ОФЕРТА"
	lblNumber   = "№"
	lblDate     = "Дата"
	lblVAT      = "ДДС №"
	lblCustomer = "Клиент"
	lblCar      = "Автомобил"
	lblVIN      = "Рама (VIN)"
	lblMileage  = "Пробег"
	lblDesc     = "Описание"
	lblKind     = "Вид"
	lblQty      = "Кол."
	lblUnit     = "Ед. цена с ДДС"
	lblLine     = "Сума с ДДС"
	lblSubtotal = "Данъчна основа"
	lblTax      = "ДДС"
	lblTotal    = "Общо"
	lblNotes    = "Забележки"
)

type Renderer struct {
	fonts []*entity.CustomFont
}

func NewRenderer() *Renderer {
	return &Renderer{
		fonts: []*entity.CustomFont{
			{Family: fontFamily, Style: fontstyle.Normal, Bytes: goregular.TTF},
			{Family: fontFamily, Style: fontstyle.Bold, Bytes: gobold.TTF},
		},
	}
}

var grey = &props.Color{Red: 90, Green: 90, Blue: 90}

func (r *Renderer) RenderOffer(data OfferData) ([]byte, error) {
	o := data.Offer
	currency := data.Tenant.Currency

	cfg := config.NewBuilder().
		WithPageSize(pagesize.A4).
		WithCustomFonts(r.fonts).
		WithDefaultFont(&props.Font{Family: fontFamily, Size: 10}).
		WithTitle(lblTitle+" "+lblNumber+o.DocumentNumber, true).
		WithAuthor(data.Tenant.Name, true).
		WithSubject(lblTitle, true).
		WithCreator("pitlane", true).
		Build()

	m := maroto.New(cfg)

	r.addHeader(m, data)
	rule(m)
	r.addParties(m, data)
	rule(m)
	r.addItems(m, o, currency)
	rule(m)
	r.addTotals(m, o, currency)
	r.addNotes(m, o)

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate offer pdf: %w", err)
	}
	return doc.GetBytes(), nil
}

func (r *Renderer) addHeader(m core.Maroto, data OfferData) {
	t := data.Tenant
	o := data.Offer

	m.AddRow(12,
		text.NewCol(8, t.Name, props.Text{Style: fontstyle.Bold, Size: 18}),
		text.NewCol(4, lblTitle, props.Text{Style: fontstyle.Bold, Size: 18, Align: align.Right}),
	)
	m.AddRow(5,
		text.NewCol(8, t.Address, props.Text{Size: 9, Color: grey}),
		text.NewCol(4, lblNumber+" "+o.DocumentNumber, props.Text{Size: 9, Color: grey, Align: align.Right}),
	)
	m.AddRow(5,
		text.NewCol(8, vatLine(t.VATNumber), props.Text{Size: 9, Color: grey}),
		text.NewCol(4, lblDate+": "+o.CreatedAt.Format("02.01.2006"), props.Text{Size: 9, Color: grey, Align: align.Right}),
	)
}

func (r *Renderer) addParties(m core.Maroto, data OfferData) {
	c := data.Customer
	car := data.Car

	m.AddRow(7,
		text.NewCol(6, lblCustomer, props.Text{Style: fontstyle.Bold, Size: 11}),
		text.NewCol(6, lblCar, props.Text{Style: fontstyle.Bold, Size: 11}),
	)
	pairRow(m, c.Name, car.Plate)
	pairRow(m, firstNonEmpty(c.Company, c.Email), makeModelYear(car))
	pairRow(m, firstNonEmpty(c.Phone, ""), labeled(lblVIN, car.VIN))
	pairRow(m, c.Address, mileageLine(car.Mileage))
}

func (r *Renderer) addItems(m core.Maroto, o *domain.Offer, currency string) {
	head := m.AddRow(7,
		text.NewCol(5, lblDesc, props.Text{Style: fontstyle.Bold, Size: 9, Top: 1.5}),
		text.NewCol(2, lblKind, props.Text{Style: fontstyle.Bold, Size: 9, Top: 1.5}),
		text.NewCol(1, lblQty, props.Text{Style: fontstyle.Bold, Size: 9, Top: 1.5, Align: align.Right}),
		text.NewCol(2, lblUnit, props.Text{Style: fontstyle.Bold, Size: 9, Top: 1.5, Align: align.Right}),
		text.NewCol(2, lblLine, props.Text{Style: fontstyle.Bold, Size: 9, Top: 1.5, Align: align.Right}),
	)
	head.WithStyle(&props.Cell{BackgroundColor: &props.Color{Red: 232, Green: 232, Blue: 232}})

	for _, it := range o.Items {
		m.AddRow(6,
			text.NewCol(5, it.Description, props.Text{Size: 9, Top: 1}),
			text.NewCol(2, kindLabel(it.Kind), props.Text{Size: 9, Top: 1}),
			text.NewCol(1, strconv.Itoa(it.Quantity), props.Text{Size: 9, Top: 1, Align: align.Right}),
			text.NewCol(2, formatCents(it.UnitPriceCents), props.Text{Size: 9, Top: 1, Align: align.Right}),
			text.NewCol(2, formatCents(it.LineTotalCents), props.Text{Size: 9, Top: 1, Align: align.Right}),
		)
	}
}

func (r *Renderer) addTotals(m core.Maroto, o *domain.Offer, currency string) {
	totalRow := func(height float64, label, value string, bold bool) {
		style := fontstyle.Normal
		if bold {
			style = fontstyle.Bold
		}
		m.AddRow(height,
			col.New(6),
			text.NewCol(4, label, props.Text{Style: style, Size: 10, Align: align.Right, Top: 1}),
			text.NewCol(2, value, props.Text{Style: style, Size: 10, Align: align.Right, Top: 1}),
		)
	}
	totalRow(6, lblSubtotal, money(o.SubtotalCents, currency), false)
	totalRow(6, lblTax+" "+percent(o.TaxRateBps), money(o.TaxCents, currency), false)
	totalRow(8, lblTotal, money(o.TotalCents, currency), true)
}

func (r *Renderer) addNotes(m core.Maroto, o *domain.Offer) {
	if o.Notes == "" {
		return
	}
	rule(m)
	m.AddRow(6, text.NewCol(12, lblNotes, props.Text{Style: fontstyle.Bold, Size: 10}))
	m.AddAutoRow(text.NewCol(12, o.Notes, props.Text{Size: 9, Color: grey}))
}

func rule(m core.Maroto) {
	m.AddRow(4, line.NewCol(12, props.Line{Color: &props.Color{Red: 210, Green: 210, Blue: 210}, Thickness: 0.2}))
}

func pairRow(m core.Maroto, left, right string) {
	m.AddRow(5,
		text.NewCol(6, left, props.Text{Size: 9}),
		text.NewCol(6, right, props.Text{Size: 9}),
	)
}

func formatCents(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := strconv.FormatInt(cents/100, 10)

	var b []byte
	for i := 0; i < len(whole); i++ {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b = append(b, ' ')
		}
		b = append(b, whole[i])
	}
	s := fmt.Sprintf("%s,%02d", b, cents%100)
	if neg {
		s = "-" + s
	}
	return s
}

func FormatMoney(cents int64, currency string) string {
	return money(cents, currency)
}

func money(cents int64, currency string) string {
	suffix := currency
	if currency == "" || currency == "EUR" {
		suffix = "€"
	}
	return formatCents(cents) + " " + suffix
}

func percent(bps int) string {
	if bps%100 == 0 {
		return strconv.Itoa(bps/100) + "%"
	}
	return fmt.Sprintf("%d,%02d%%", bps/100, bps%100)
}

func kindLabel(k domain.OfferItemKind) string {
	switch k {
	case domain.OfferItemKindPart:
		return "Част"
	case domain.OfferItemKindLabor:
		return "Труд"
	default:
		return "Друго"
	}
}

func vatLine(vat string) string {
	if vat == "" {
		return ""
	}
	return lblVAT + ": " + vat
}

func labeled(label, value string) string {
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func mileageLine(mileage int) string {
	if mileage == 0 {
		return ""
	}
	return fmt.Sprintf("%s: %s км", lblMileage, formatThousands(int64(mileage)))
}

func makeModelYear(car *domain.Car) string {
	s := firstNonEmpty(joinNonEmpty(car.Make, car.Model), "")
	if car.Year != 0 {
		if s != "" {
			s += ", "
		}
		s += strconv.Itoa(car.Year)
	}
	return s
}

func formatThousands(n int64) string {
	whole := strconv.FormatInt(n, 10)
	var b []byte
	for i := 0; i < len(whole); i++ {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b = append(b, ' ')
		}
		b = append(b, whole[i])
	}
	return string(b)
}

func joinNonEmpty(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + " " + b
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
