// Package pdf renders offers to PDF. The concrete Renderer wraps maroto v2
// (ADR §13: server-side generation, never stored, regenerated on demand). The
// API layer owns the consuming interface (ADR §106: interfaces where consumed);
// this package only exports the concrete renderer and its input bundle.
//
// The whole app is Bulgarian-only (Phase 2.5), so the document is rendered in
// Bulgarian with bg-BG number/date formatting. Labels are constants here rather
// than flowing through Lingui — the SPA catalog covers the browser UI, and this
// is a server-side document with a single, fixed locale.
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

// OfferData is the complete render input: everything the document shows,
// pre-loaded by the handler. The renderer performs no I/O — it is a pure
// function from this bundle to bytes, which keeps it trivially testable and
// safe for concurrent use.
type OfferData struct {
	Tenant   *domain.Tenant
	Customer *domain.Customer
	Car      *domain.Car
	Offer    *domain.Offer
}

// fontFamily is the registered family name for the embedded Go font. The Go
// font family (Bigelow & Holmes, shipped as TTF bytes by golang.org/x/image —
// already in the module graph via maroto) is used because the default Arial
// covers only Latin-1 and would drop every Cyrillic glyph. It is open-licensed,
// compact (~150KB/style), and needs no runtime file or external download.
const fontFamily = "go"

// Bulgarian document strings. Kept together so the whole vocabulary of the
// document is visible in one place.
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
	lblUnit     = "Ед. цена"
	lblLine     = "Сума"
	lblSubtotal = "Междинна сума"
	lblTax      = "ДДС"
	lblTotal    = "Общо"
	lblNotes    = "Забележки"
)

// Renderer generates offer PDFs. The embedded fonts are parsed once at
// construction and reused across renders (they are read-only), so a single
// Renderer is shared for the process lifetime.
type Renderer struct {
	fonts []*entity.CustomFont
}

// NewRenderer builds a Renderer with the embedded Cyrillic-capable fonts
// registered in Normal and Bold styles (the only two the layout uses).
func NewRenderer() *Renderer {
	return &Renderer{
		fonts: []*entity.CustomFont{
			{Family: fontFamily, Style: fontstyle.Normal, Bytes: goregular.TTF},
			{Family: fontFamily, Style: fontstyle.Bold, Bytes: gobold.TTF},
		},
	}
}

// grey is the table-header / rule tint.
var grey = &props.Color{Red: 90, Green: 90, Blue: 90}

// RenderOffer produces the PDF bytes for one offer. It never touches the
// database or filesystem; the handler supplies every field.
func (r *Renderer) RenderOffer(data OfferData) ([]byte, error) {
	o := data.Offer
	currency := data.Tenant.Currency

	cfg := config.NewBuilder().
		WithPageSize(pagesize.A4).
		WithCustomFonts(r.fonts).
		WithDefaultFont(&props.Font{Family: fontFamily, Size: 10}).
		WithTitle(lblTitle+" "+lblNumber+shortID(o.ID), true).
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

// addHeader draws the garage identity on the left and the offer's number/date
// on the right.
func (r *Renderer) addHeader(m core.Maroto, data OfferData) {
	t := data.Tenant
	o := data.Offer

	m.AddRow(12,
		text.NewCol(8, t.Name, props.Text{Style: fontstyle.Bold, Size: 18}),
		text.NewCol(4, lblTitle, props.Text{Style: fontstyle.Bold, Size: 18, Align: align.Right}),
	)
	m.AddRow(5,
		text.NewCol(8, t.Address, props.Text{Size: 9, Color: grey}),
		text.NewCol(4, lblNumber+" "+shortID(o.ID), props.Text{Size: 9, Color: grey, Align: align.Right}),
	)
	m.AddRow(5,
		text.NewCol(8, vatLine(t.VATNumber), props.Text{Size: 9, Color: grey}),
		text.NewCol(4, lblDate+": "+o.CreatedAt.Format("02.01.2006"), props.Text{Size: 9, Color: grey, Align: align.Right}),
	)
}

// addParties draws the customer block beside the car block, one field per row
// so the two columns stay aligned. Empty fields render as blank cells.
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

// addItems draws the tinted header row and one row per line item.
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

// addTotals draws the subtotal / tax / total block, right-aligned under the
// item table's money columns.
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

// addNotes appends the free-text notes block if the offer carries any.
func (r *Renderer) addNotes(m core.Maroto, o *domain.Offer) {
	if o.Notes == "" {
		return
	}
	rule(m)
	m.AddRow(6, text.NewCol(12, lblNotes, props.Text{Style: fontstyle.Bold, Size: 10}))
	m.AddAutoRow(text.NewCol(12, o.Notes, props.Text{Size: 9, Color: grey}))
}

// rule draws a thin horizontal separator row.
func rule(m core.Maroto) {
	m.AddRow(4, line.NewCol(12, props.Line{Color: &props.Color{Red: 210, Green: 210, Blue: 210}, Thickness: 0.2}))
}

// pairRow renders two side-by-side text cells (customer field | car field).
func pairRow(m core.Maroto, left, right string) {
	m.AddRow(5,
		text.NewCol(6, left, props.Text{Size: 9}),
		text.NewCol(6, right, props.Text{Size: 9}),
	)
}

// --- formatting helpers (bg-BG conventions) -------------------------------

// formatCents renders integer cents as "1 234,56": space-grouped thousands and
// a comma decimal separator, matching the SPA's bg-BG money formatting.
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

// FormatMoney renders integer cents with the tenant currency in bg-BG style
// ("1 234,56 €"). Exported so other server-side documents that must match the
// PDF exactly — notably the offer email (Phase 7) — format money identically,
// with no second implementation to drift.
func FormatMoney(cents int64, currency string) string {
	return money(cents, currency)
}

// money appends the tenant currency symbol to a formatted amount. Bulgaria
// adopted the euro on 2026-01-01, so an unset or euro currency renders with the
// "€" sign; any other ISO code is shown verbatim.
func money(cents int64, currency string) string {
	suffix := currency
	if currency == "" || currency == "EUR" {
		suffix = "€"
	}
	return formatCents(cents) + " " + suffix
}

// percent renders a basis-points rate as a human percentage: 1900 → "19%",
// 1950 → "19,50%".
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

// ShortID is the compact human-facing offer number (first UUID segment) printed
// on the PDF. Exported so the offer email cites the same number as the document.
func ShortID(id string) string {
	return shortID(id)
}

// shortID returns the first segment of a UUID for a compact human-facing
// offer number (the full ID stays the canonical reference).
func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
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
