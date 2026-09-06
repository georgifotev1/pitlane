package main

import (
	"fmt"
	"html/template"
	"time"

	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
)

// Charts are drawn on the server as plain SVG: no chart library, no client-side
// rendering, nothing for the browser to do but paint. Every coordinate below is
// computed here so the templates stay declarative.
const (
	chartWidth      = 760
	chartHeight     = 250
	chartPadLeft    = 62
	chartPadRight   = 8
	chartPadTop     = 14
	chartPadBottom  = 32
	chartBarMaxWide = 24
	// A 2px gap in the surface colour separates the two stacked segments; a
	// stroke around them would add ink that is not data.
	chartSegmentGap = 2
	chartCornerR    = 4
	chartTicks      = 4
)

var monthNamesBG = [...]string{"яну", "фев", "мар", "апр", "май", "юни", "юли", "авг", "сеп", "окт", "ное", "дек"}

type chartBar struct {
	Label    string
	SubLabel string
	LabelX   int
	// Title is the native SVG tooltip - the hover layer, without JavaScript.
	Title      string
	ProfitPath string
	CostPath   string
	// Loss marks a month whose parts cost more than the work billed. The bar is
	// then a single mark in the status colour instead of a two-part stack,
	// because there is no profit segment to draw.
	Loss    bool
	Current bool
	// Rect covers the whole band so the tooltip is reachable anywhere above the
	// bar, not only on the few pixels the mark occupies.
	BandX     int
	BandWidth int
}

type chartTick struct {
	Y     int
	Label string
}

type chartMonthRow struct {
	Month   string
	Revenue string
	Cost    string
	Profit  string
	Margin  string
	Repairs int
}

type revenueChart struct {
	Width      int
	Height     int
	Baseline   int
	PlotLeft   int
	PlotRight  int
	PlotTop    int
	PlotHeight int
	// Text anchors, precomputed so the template does no arithmetic.
	AxisLabelX  int
	MonthLabelY int
	YearLabelY  int
	Bars        []chartBar
	Ticks       []chartTick
	// Rows is the table view of the same numbers: the accessible fallback and
	// the answer to "what exactly was March".
	Rows    []chartMonthRow
	HasData bool
	HasLoss bool
	// PartialMonth marks the last column as a month still in progress, so a
	// short final bar is read as "not finished yet" rather than as a collapse.
	PartialMonth bool
	// HasCost stays false while no supplier prices have been entered at all,
	// which turns the "profit" segment into the whole bar. The panel says so
	// rather than letting the chart imply a 100% margin.
	HasCost bool
}

// newRevenueChart lays out one column per month, each stacked as parts cost
// (recessive grey, on top) plus profit (accent, anchored to the baseline), so
// the column's full height is the month's net turnover and the coloured part is
// what the garage actually kept.
func newRevenueChart(months []store.MonthPoint, currency string) *revenueChart {
	c := &revenueChart{
		Width:       chartWidth,
		Height:      chartHeight,
		Baseline:    chartHeight - chartPadBottom,
		PlotLeft:    chartPadLeft,
		PlotRight:   chartWidth - chartPadRight,
		PlotTop:     chartPadTop,
		PlotHeight:  chartHeight - chartPadTop - chartPadBottom,
		AxisLabelX:  chartPadLeft - 10,
		MonthLabelY: chartHeight - chartPadBottom + 17,
		YearLabelY:  chartHeight - chartPadBottom + 29,
	}
	if len(months) == 0 {
		return c
	}

	plotHeight := c.PlotHeight
	var peak int64
	for _, m := range months {
		peak = max(peak, m.NetRevenueCents)
		c.HasData = c.HasData || m.NetRevenueCents != 0 || m.NetCostCents != 0
		c.HasCost = c.HasCost || m.NetCostCents > 0
	}
	maxCents := niceMax(peak)

	step := maxCents / chartTicks
	for i := 0; i <= chartTicks; i++ {
		value := step * int64(i)
		c.Ticks = append(c.Ticks, chartTick{
			Y:     c.Baseline - int(int64(plotHeight)*int64(i)/int64(chartTicks)),
			Label: formatAxisCents(value),
		})
	}

	band := float64(chartWidth-chartPadLeft-chartPadRight) / float64(len(months))
	barWidth := min(chartBarMaxWide, int(band)-10)
	barWidth = max(barWidth, 6)
	now := time.Now()

	for i, m := range months {
		bandX := chartPadLeft + int(band*float64(i))
		x := bandX + (int(band)-barWidth)/2
		height := scaleCents(m.NetRevenueCents, maxCents, plotHeight)
		costHeight := scaleCents(m.NetCostCents, maxCents, plotHeight)
		profit := m.NetProfitCents()

		bar := chartBar{
			Label:     monthNamesBG[int(m.Month.Month())-1],
			LabelX:    x + barWidth/2,
			BandX:     bandX,
			BandWidth: int(band),
			Current:   m.Month.Year() == now.Year() && m.Month.Month() == now.Month(),
			Loss:      profit < 0,
			Title:     monthTooltip(m, currency),
		}
		if m.Month.Month() == time.January || i == 0 {
			bar.SubLabel = fmt.Sprintf("%d", m.Month.Year())
		}

		switch {
		case height <= 0:
			// Nothing billed that month: no mark, and the gap says so.
		case bar.Loss:
			bar.ProfitPath = roundedTopRect(x, c.Baseline-height, barWidth, height)
		default:
			profitHeight := height - costHeight
			if costHeight > 0 {
				bar.CostPath = roundedTopRect(x, c.Baseline-height, barWidth, costHeight)
				profitHeight -= chartSegmentGap
			}
			if profitHeight > 0 {
				if costHeight > 0 {
					bar.ProfitPath = squareRect(x, c.Baseline-profitHeight, barWidth, profitHeight)
				} else {
					bar.ProfitPath = roundedTopRect(x, c.Baseline-profitHeight, barWidth, profitHeight)
				}
			}
		}
		c.HasLoss = c.HasLoss || bar.Loss
		if bar.Current && now.Day() < daysInMonth(m.Month) {
			c.PartialMonth = true
		}
		c.Bars = append(c.Bars, bar)
		c.Rows = append(c.Rows, chartMonthRow{
			Month:   fmt.Sprintf("%s %d", monthNamesBG[int(m.Month.Month())-1], m.Month.Year()),
			Revenue: pdf.FormatMoney(m.NetRevenueCents, currency),
			Cost:    pdf.FormatMoney(m.NetCostCents, currency),
			Profit:  pdf.FormatMoney(profit, currency),
			Margin:  formatMargin(profit, m.NetRevenueCents),
			Repairs: m.Repairs,
		})
	}
	return c
}

func daysInMonth(month time.Time) int {
	return time.Date(month.Year(), month.Month()+1, 0, 0, 0, 0, 0, month.Location()).Day()
}

func monthTooltip(m store.MonthPoint, currency string) string {
	label := fmt.Sprintf("%s %d", monthNamesBG[int(m.Month.Month())-1], m.Month.Year())
	if m.NetRevenueCents == 0 && m.NetCostCents == 0 {
		return label + ": няма завършени ремонти"
	}
	return fmt.Sprintf("%s · оборот %s · части %s · печалба %s (%s) · %d ремонта",
		label,
		pdf.FormatMoney(m.NetRevenueCents, currency),
		pdf.FormatMoney(m.NetCostCents, currency),
		pdf.FormatMoney(m.NetProfitCents(), currency),
		formatMargin(m.NetProfitCents(), m.NetRevenueCents),
		m.Repairs,
	)
}

// splitBar is one horizontal part-to-whole bar in the "where the money comes
// from" panel. Widths are percentages of the largest category so the bars stay
// comparable and reflow with the panel instead of being pinned to a viewBox.
type splitBar struct {
	Label string
	// The bar widths arrive as complete style attributes rather than as values
	// inside one. A template action within style="..." is valid Go and renders
	// correctly, but every HTML tool that parses the attribute as CSS - editors
	// included - reports it as a syntax error, so the attribute is built here.
	ProfitStyle template.HTMLAttr
	CostStyle   template.HTMLAttr
	HasCost     bool
	Revenue     string
	Profit      string
	Margin      string
	Loss        bool
}

type splitChart struct {
	Bars    []splitBar
	HasData bool
	HasCost bool
}

var splitOrder = []struct{ Kind, Label string }{
	{"labor", "Труд"},
	{"part", "Части"},
	{"other", "Друго"},
}

// newSplitChart answers the question a parts markup raises: labour is nearly all
// margin, resold parts are not, so two garages with the same turnover can keep
// very different amounts of money.
func newSplitChart(split []store.KindSplit, currency string) *splitChart {
	byKind := make(map[string]store.KindSplit, len(split))
	var peak int64
	for _, s := range split {
		byKind[s.Kind] = s
		peak = max(peak, s.NetRevenueCents)
	}
	c := &splitChart{HasData: peak > 0}
	if !c.HasData {
		return c
	}

	for _, entry := range splitOrder {
		s, ok := byKind[entry.Kind]
		if !ok || s.NetRevenueCents == 0 {
			continue
		}
		profit := s.NetProfitCents()
		bar := splitBar{
			Label:   entry.Label,
			Revenue: pdf.FormatMoney(s.NetRevenueCents, currency),
			Profit:  pdf.FormatMoney(profit, currency),
			Margin:  formatMargin(profit, s.NetRevenueCents),
			Loss:    profit < 0,
		}
		if bar.Loss {
			bar.ProfitStyle = widthStyle(s.NetRevenueCents, peak)
		} else {
			bar.ProfitStyle = widthStyle(profit, peak)
			bar.CostStyle = widthStyle(s.NetCostCents, peak)
			bar.HasCost = s.NetCostCents > 0
		}
		c.HasCost = c.HasCost || s.NetCostCents > 0
		c.Bars = append(c.Bars, bar)
	}
	return c
}

// widthStyle renders a bar segment as a share of the largest category. The
// percentage is derived from two integers here, never from user input.
func widthStyle(value, peak int64) template.HTMLAttr {
	share := 0.0
	if peak > 0 && value > 0 {
		share = float64(value) * 100 / float64(peak)
	}
	return template.HTMLAttr(fmt.Sprintf(` style="width:%.2f%%"`, share))
}

func scaleCents(value, maxCents int64, plotHeight int) int {
	if maxCents <= 0 || value <= 0 {
		return 0
	}
	height := int(value * int64(plotHeight) / maxCents)
	if height == 0 {
		// A month with real money in it never renders as nothing.
		height = 1
	}
	return height
}

// niceMax rounds the tallest column up to a value whose quarters are round
// numbers, so the axis reads 0 / 2 500 / 5 000 rather than 0 / 2 317 / 4 634.
func niceMax(peakCents int64) int64 {
	if peakCents <= 0 {
		return int64(chartTicks) * 100
	}
	step := int64(100)
	for {
		for _, factor := range []int64{1, 2, 5} {
			candidate := step * factor
			if candidate*int64(chartTicks) >= peakCents {
				return candidate * int64(chartTicks)
			}
		}
		step *= 10
	}
}

func roundedTopRect(x, y, w, h int) string {
	r := min(chartCornerR, h)
	r = min(r, w/2)
	return fmt.Sprintf("M%d %dV%dQ%d %d %d %dH%dQ%d %d %d %dV%dZ",
		x, y+h,
		y+r,
		x, y, x+r, y,
		x+w-r,
		x+w, y, x+w, y+r,
		y+h,
	)
}

func squareRect(x, y, w, h int) string {
	return fmt.Sprintf("M%d %dH%dV%dH%dZ", x, y, x+w, y+h, x)
}

// formatAxisCents keeps axis ticks terse: whole currency units, thousands
// spaced, no decimals and no symbol - the axis title carries the currency.
func formatAxisCents(cents int64) string {
	units := cents / 100
	if units >= 10000 {
		return fmt.Sprintf("%d хил.", units/1000)
	}
	s := fmt.Sprintf("%d", units)
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func formatMargin(profitCents, netRevenueCents int64) string {
	if netRevenueCents <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d%%", profitCents*100/netRevenueCents)
}
