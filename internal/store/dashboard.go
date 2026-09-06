package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Windows the owner's dashboard reasons over. They are deliberately blunt: a
// quote nobody answered in two weeks needs chasing, a job that has sat open for
// two weeks is stuck, and a quarter is long enough to make an acceptance rate
// mean something.
const (
	OfferStaleAfter    = 14 * 24 * time.Hour
	RepairStalledAfter = 14 * 24 * time.Hour
	ConversionWindow   = 90 * 24 * time.Hour
	DashboardMonths    = 12
)

// MonthPoint is one calendar month of finished work, VAT-exclusive. Revenue is
// what the customers paid net of VAT; cost is what the parts cost net of VAT.
// The difference is the money the garage kept.
type MonthPoint struct {
	Month           time.Time
	NetRevenueCents int64
	NetCostCents    int64
	Repairs         int
}

func (m MonthPoint) NetProfitCents() int64 { return m.NetRevenueCents - m.NetCostCents }

// KindSplit answers "where does the money come from" - parts resold at a markup
// versus labour, which is almost pure margin.
type KindSplit struct {
	Kind            string
	NetRevenueCents int64
	NetCostCents    int64
}

func (k KindSplit) NetProfitCents() int64 { return k.NetRevenueCents - k.NetCostCents }

// Period is a stretch of finished work - used for the current month so far and
// for the same stretch of the month before it.
type Period struct {
	NetRevenueCents int64
	NetCostCents    int64
	Repairs         int
}

func (p Period) NetProfitCents() int64 { return p.NetRevenueCents - p.NetCostCents }

type Dashboard struct {
	Months []MonthPoint
	Split  []KindSplit

	// MonthToDate is this month up to now, and PreviousToDate is the same number
	// of days of the month before. Comparing whole months would tell an owner
	// looking at the dashboard on the 3rd that takings had collapsed.
	MonthToDate    Period
	PreviousToDate Period

	// Work in progress: jobs accepted but not yet completed, so not yet income.
	ActiveRepairs       int
	ActiveTotalCents    int64
	ActiveProfitCents   int64
	OpenOffers          int
	OpenOfferTotalCents int64

	// Quote outcomes inside ConversionWindow.
	AcceptedOffers int
	DecidedOffers  int

	StaleOffers    []OfferSummary
	StalledRepairs []RepairSummary
}

// AcceptanceRate is the share of decided quotes that turned into work, in
// percent. Undecided quotes are excluded so a busy week of fresh quotes cannot
// drag the number down.
func (d Dashboard) AcceptanceRate() int {
	if d.DecidedOffers == 0 {
		return 0
	}
	return d.AcceptedOffers * 100 / d.DecidedOffers
}

type DashboardStore struct {
	db *DB
}

func NewDashboardStore(db *DB) *DashboardStore {
	return &DashboardStore{db: db}
}

// netExpr strips VAT from a VAT-inclusive money column using the document's own
// snapshotted rate. It mirrors domain.TaxCents exactly, including its
// round-half-up, so a figure aggregated here matches one computed in Go.
func netExpr(column, rateColumn string) string {
	return fmt.Sprintf("(%[1]s - (%[1]s * %[2]s + (10000 + %[2]s) / 2) / (10000 + %[2]s))", column, rateColumn)
}

// Load reads every figure the dashboard shows in one tenant-scoped transaction, so
// the panels are a consistent snapshot rather than a set of racing reads.
func (s *DashboardStore) Load(ctx context.Context, tenantID string, now time.Time) (Dashboard, error) {
	var d Dashboard

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var err error
		if d.Months, err = monthlyRevenue(ctx, tx, tenantID, now); err != nil {
			return err
		}
		if d.Split, err = revenueByKind(ctx, tx, tenantID, now); err != nil {
			return err
		}
		if d.MonthToDate, d.PreviousToDate, err = monthToDate(ctx, tx, tenantID, now); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `
			SELECT count(*),
			       COALESCE(sum(total_cents), 0),
			       COALESCE(sum(subtotal_cents - cost_subtotal_cents), 0)
			FROM repairs
			WHERE tenant_id = $1 AND status IN ('open', 'in_progress')
		`, tenantID).Scan(&d.ActiveRepairs, &d.ActiveTotalCents, &d.ActiveProfitCents); err != nil {
			return fmt.Errorf("active repairs: %w", err)
		}

		since := now.Add(-ConversionWindow)
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE status IN ('draft', 'sent')),
			       COALESCE(sum(total_cents) FILTER (WHERE status IN ('draft', 'sent')), 0),
			       count(*) FILTER (WHERE status = 'accepted' AND created_at >= $2),
			       count(*) FILTER (WHERE status IN ('accepted', 'rejected', 'expired') AND created_at >= $2)
			FROM offers
			WHERE tenant_id = $1
		`, tenantID, since).Scan(&d.OpenOffers, &d.OpenOfferTotalCents, &d.AcceptedOffers, &d.DecidedOffers); err != nil {
			return fmt.Errorf("offer pipeline: %w", err)
		}

		rows, err := tx.Query(ctx,
			offerBoardSelect+`WHERE o.tenant_id = $1 AND o.status IN ('draft', 'sent') AND o.created_at < $2
			 ORDER BY o.created_at ASC, o.id ASC
			 LIMIT 5`,
			tenantID, now.Add(-OfferStaleAfter),
		)
		if err != nil {
			return fmt.Errorf("stale offers: %w", err)
		}
		d.StaleOffers, err = scanOfferSummaries(rows)
		rows.Close()
		if err != nil {
			return err
		}

		rows, err = tx.Query(ctx,
			repairBoardSelect+`WHERE r.tenant_id = $1 AND r.status IN ('open', 'in_progress') AND r.created_at < $2
			 ORDER BY r.created_at ASC, r.id ASC
			 LIMIT 5`,
			tenantID, now.Add(-RepairStalledAfter),
		)
		if err != nil {
			return fmt.Errorf("stalled repairs: %w", err)
		}
		d.StalledRepairs, err = scanRepairSummaries(rows)
		rows.Close()
		return err
	})
	return d, err
}

// monthToDate reads this month so far and the matching stretch of last month.
// The comparison window is the elapsed time itself, so it lands correctly
// whatever the two months' lengths are.
func monthToDate(ctx context.Context, tx pgx.Tx, tenantID string, now time.Time) (current, previous Period, err error) {
	row := tx.QueryRow(ctx, `
		WITH bounds AS (
		    SELECT date_trunc('month', $2::timestamptz) AS this_month,
		           date_trunc('month', $2::timestamptz) - interval '1 month' AS last_month,
		           $2::timestamptz - date_trunc('month', $2::timestamptz) AS elapsed
		)
		SELECT COALESCE(sum(r.subtotal_cents) FILTER (WHERE r.completed_at >= b.this_month), 0),
		       COALESCE(sum(r.cost_subtotal_cents) FILTER (WHERE r.completed_at >= b.this_month), 0),
		       count(*) FILTER (WHERE r.completed_at >= b.this_month),
		       COALESCE(sum(r.subtotal_cents) FILTER (WHERE r.completed_at >= b.last_month AND r.completed_at < b.last_month + b.elapsed), 0),
		       COALESCE(sum(r.cost_subtotal_cents) FILTER (WHERE r.completed_at >= b.last_month AND r.completed_at < b.last_month + b.elapsed), 0),
		       count(*) FILTER (WHERE r.completed_at >= b.last_month AND r.completed_at < b.last_month + b.elapsed)
		FROM bounds b
		LEFT JOIN repairs r
		       ON r.tenant_id = $1
		      AND r.status = 'completed'
		      AND r.completed_at >= b.last_month
		GROUP BY b.this_month
	`, tenantID, now)
	if err := row.Scan(
		&current.NetRevenueCents, &current.NetCostCents, &current.Repairs,
		&previous.NetRevenueCents, &previous.NetCostCents, &previous.Repairs,
	); err != nil {
		return Period{}, Period{}, fmt.Errorf("month to date: %w", err)
	}
	return current, previous, nil
}

// monthlyRevenue returns DashboardMonths consecutive months ending with the
// current one. generate_series supplies the calendar so months without a single
// completed repair still come back as zeros - a gap in the bars is data, and
// dropping it would silently redraw the trend.
func monthlyRevenue(ctx context.Context, tx pgx.Tx, tenantID string, now time.Time) ([]MonthPoint, error) {
	rows, err := tx.Query(ctx, `
		SELECT m.month,
		       COALESCE(sum(r.subtotal_cents), 0),
		       COALESCE(sum(r.cost_subtotal_cents), 0),
		       count(r.id)
		FROM generate_series(
		         date_trunc('month', $2::timestamptz) - make_interval(months => $3::int - 1),
		         date_trunc('month', $2::timestamptz),
		         interval '1 month') AS m(month)
		LEFT JOIN repairs r
		       ON r.tenant_id = $1
		      AND r.status = 'completed'
		      AND r.completed_at >= m.month
		      AND r.completed_at < m.month + interval '1 month'
		GROUP BY m.month
		ORDER BY m.month
	`, tenantID, now, DashboardMonths)
	if err != nil {
		return nil, fmt.Errorf("monthly revenue: %w", err)
	}
	defer rows.Close()

	var out []MonthPoint
	for rows.Next() {
		var p MonthPoint
		if err := rows.Scan(&p.Month, &p.NetRevenueCents, &p.NetCostCents, &p.Repairs); err != nil {
			return nil, fmt.Errorf("scan monthly revenue: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func revenueByKind(ctx context.Context, tx pgx.Tx, tenantID string, now time.Time) ([]KindSplit, error) {
	rows, err := tx.Query(ctx, `
		SELECT i.kind,
		       COALESCE(sum(`+netExpr("i.line_total_cents", "r.tax_rate_bps")+`), 0),
		       COALESCE(sum(`+netExpr("i.line_cost_cents", "r.tax_rate_bps")+`), 0)
		FROM repair_items i
		JOIN repairs r ON r.id = i.repair_id AND r.tenant_id = i.tenant_id
		WHERE i.tenant_id = $1
		  AND r.status = 'completed'
		  AND r.completed_at >= date_trunc('month', $2::timestamptz) - make_interval(months => $3::int - 1)
		GROUP BY i.kind
	`, tenantID, now, DashboardMonths)
	if err != nil {
		return nil, fmt.Errorf("revenue by kind: %w", err)
	}
	defer rows.Close()

	var out []KindSplit
	for rows.Next() {
		var k KindSplit
		if err := rows.Scan(&k.Kind, &k.NetRevenueCents, &k.NetCostCents); err != nil {
			return nil, fmt.Errorf("scan revenue by kind: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
