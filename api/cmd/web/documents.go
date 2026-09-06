package main

import (
	"fmt"
	"time"

	"github.com/gfotev/pitlane/internal/forms"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
)

// Offers and repairs are the same thing to a reader scanning a list: a numbered
// document for a car, with a status and a total. documentRow is that shape, so
// one board component and one compact-list component serve both entities and
// the dashboard panels instead of four near-identical tables.
type documentRow struct {
	URL          string
	Number       string
	CreatedAt    time.Time
	CustomerName string
	CarPlate     string
	Status       string
	// TypeLabel is set only where offers and repairs share one list, so the
	// reader can tell a quote from a job at a glance.
	TypeLabel   string
	TotalCents  int64
	ProfitCents int64
	// Margin is a whole percentage of net revenue. Fractions of a percent are
	// noise on a list, and the exact figures live on the document itself.
	Margin string
	// HasCost is false until someone records what the parts cost. Without it
	// profit would read as the whole invoice and margin as 100%, so the views
	// show a dash instead of a flattering fiction.
	HasCost bool
	// Note carries a row-level remark the panel needs, such as how long a quote
	// has been waiting for an answer.
	Note string
}

type emptyState struct {
	Title       string
	Text        string
	ActionLabel string
	ActionURL   string
}

type documentBoard struct {
	// NumberHeading names the first column ("Оферта" / "Ремонт").
	NumberHeading string
	Rows          []documentRow
	Currency      string
	// ShowProfit gates the margin column. It is on for every internal view and
	// has no counterpart on anything the customer sees.
	ShowProfit bool
	Empty      emptyState
}

func offerBoard(offers []store.OfferSummary, currency string) *documentBoard {
	rows := make([]documentRow, 0, len(offers))
	for _, summary := range offers {
		rows = append(rows, offerDocumentRow(summary))
	}
	return &documentBoard{
		NumberHeading: "Оферта",
		Rows:          rows,
		Currency:      currency,
		ShowProfit:    true,
		Empty: emptyState{
			Title: "Няма намерени оферти",
			Text:  "Създайте оферта от страницата на автомобил.",
		},
	}
}

func repairBoard(repairs []store.RepairSummary, currency string) *documentBoard {
	rows := make([]documentRow, 0, len(repairs))
	for _, summary := range repairs {
		rows = append(rows, repairDocumentRow(summary))
	}
	return &documentBoard{
		NumberHeading: "Ремонт",
		Rows:          rows,
		Currency:      currency,
		ShowProfit:    true,
		Empty: emptyState{
			Title: "Няма намерени ремонти",
			Text:  "Приемете оферта, за да създадете първия ремонт.",
		},
	}
}

func offerDocumentRow(summary store.OfferSummary) documentRow {
	o := summary.Offer
	return documentRow{
		URL:          "/offers/" + o.ID,
		Number:       o.DocumentNumber,
		CreatedAt:    o.CreatedAt,
		CustomerName: summary.CustomerName,
		CarPlate:     summary.CarPlate,
		Status:       string(o.Status),
		TotalCents:   o.TotalCents,
		ProfitCents:  o.ProfitCents(),
		Margin:       formatMargin(o.ProfitCents(), o.SubtotalCents),
		HasCost:      o.CostTotalCents > 0,
	}
}

func repairDocumentRow(summary store.RepairSummary) documentRow {
	r := summary.Repair
	return documentRow{
		URL:          "/repairs/" + r.ID,
		Number:       r.DocumentNumber,
		CreatedAt:    r.CreatedAt,
		CustomerName: summary.CustomerName,
		CarPlate:     summary.CarPlate,
		Status:       string(r.Status),
		TotalCents:   r.TotalCents,
		ProfitCents:  r.ProfitCents(),
		Margin:       formatMargin(r.ProfitCents(), r.SubtotalCents),
		HasCost:      r.CostTotalCents > 0,
	}
}

// waitingNote reads as "чака от 21 дни" next to a document nobody has moved.
func waitingNote(since time.Time, now time.Time) string {
	days := int(now.Sub(since).Hours() / 24)
	if days < 1 {
		return ""
	}
	if days == 1 {
		return "чака от 1 ден"
	}
	return fmt.Sprintf("чака от %d дни", days)
}

// documentLine and documentView describe the line-item table an offer and a
// repair both print. The same partial renders it on the internal offer page,
// the internal repair page and the customer's print sheet - which is exactly
// why the view carries no cost fields: the customer must never see what the
// garage paid its supplier. Margin lives in a separate, internal-only panel.
type documentLine struct {
	Description    string
	Kind           string
	Quantity       int
	UnitPriceCents int64
	LineTotalCents int64
}

type documentView struct {
	Lines         []documentLine
	SubtotalCents int64
	TaxCents      int64
	TotalCents    int64
	TaxRateBps    int
	Currency      string
}

// margins is the internal counterpart: what the parts cost and what is left.
type marginView struct {
	CostSubtotalCents int64
	NetRevenueCents   int64
	ProfitCents       int64
	Margin            string
	Currency          string
	// HasCost is false when nobody has entered a purchase price yet, which the
	// panel says out loud rather than reporting a 100% margin.
	HasCost bool
}

func offerDocument(o *domain.Offer, currency string) *documentView {
	lines := make([]documentLine, 0, len(o.Items))
	for _, it := range o.Items {
		lines = append(lines, documentLine{
			Description:    it.Description,
			Kind:           string(it.Kind),
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			LineTotalCents: it.LineTotalCents,
		})
	}
	return &documentView{
		Lines:         lines,
		SubtotalCents: o.SubtotalCents,
		TaxCents:      o.TaxCents,
		TotalCents:    o.TotalCents,
		TaxRateBps:    o.TaxRateBps,
		Currency:      currency,
	}
}

func repairDocument(r *domain.Repair, currency string) *documentView {
	lines := make([]documentLine, 0, len(r.Items))
	for _, it := range r.Items {
		lines = append(lines, documentLine{
			Description:    it.Description,
			Kind:           string(it.Kind),
			Quantity:       it.Quantity,
			UnitPriceCents: it.UnitPriceCents,
			LineTotalCents: it.LineTotalCents,
		})
	}
	return &documentView{
		Lines:         lines,
		SubtotalCents: r.SubtotalCents,
		TaxCents:      r.TaxCents,
		TotalCents:    r.TotalCents,
		TaxRateBps:    r.TaxRateBps,
		Currency:      currency,
	}
}

func offerMargin(o *domain.Offer, currency string) *marginView {
	return &marginView{
		CostSubtotalCents: o.CostSubtotalCents,
		NetRevenueCents:   o.SubtotalCents,
		ProfitCents:       o.ProfitCents(),
		Margin:            formatMargin(o.ProfitCents(), o.SubtotalCents),
		Currency:          currency,
		HasCost:           o.CostTotalCents > 0,
	}
}

func repairMargin(r *domain.Repair, currency string) *marginView {
	return &marginView{
		CostSubtotalCents: r.CostSubtotalCents,
		NetRevenueCents:   r.SubtotalCents,
		ProfitCents:       r.ProfitCents(),
		Margin:            formatMargin(r.ProfitCents(), r.SubtotalCents),
		Currency:          currency,
		HasCost:           r.CostTotalCents > 0,
	}
}

// itemEditor is the line-item editor both the offer form and the repair form
// render. The two used to be separate tables that drifted apart - the repair
// one could not even add a row - so they are one component now.
type itemEditor struct {
	Title string
	Hint  string
	Rows  []offerRow
	Error string
}

func newItemEditor(title, hint string, rows []offerRow, form *forms.Form) *itemEditor {
	editor := &itemEditor{Title: title, Hint: hint, Rows: rows}
	if form != nil {
		editor.Error = form.Errors["items"]
	}
	return editor
}
