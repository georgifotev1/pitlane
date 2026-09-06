package store

import (
	"testing"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// costedOffer sells a part at 60,00 that was bought for 40,00, plus labour at
// 50,00 with no supplier cost.
func costedOffer(tenantID, carID string) *domain.Offer {
	return &domain.Offer{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		CarID:      carID,
		TaxRateBps: domain.StandardVATRateBPS,
		Items: []domain.OfferItem{
			{Kind: domain.OfferItemKindPart, Description: "Brake pads", Quantity: 1, UnitPriceCents: 6000, CostCents: 4000},
			{Kind: domain.OfferItemKindLabor, Description: "Fitting", Quantity: 1, UnitPriceCents: 5000},
		},
	}
}

func TestSupplierCostSurvivesTheOfferToRepairConversion(t *testing.T) {
	ctx, _, repairs, offers, tenant, _, car := repairFixture(t)

	offer := costedOffer(tenant.ID, car.ID)
	if err := offers.Create(ctx, offer); err != nil {
		t.Fatalf("create offer: %v", err)
	}

	stored, err := offers.Get(ctx, tenant.ID, offer.ID)
	if err != nil {
		t.Fatalf("get offer: %v", err)
	}
	if stored.CostTotalCents != 4000 {
		t.Fatalf("offer cost total = %d; want 4000", stored.CostTotalCents)
	}
	if stored.Items[0].LineCostCents != 4000 || stored.Items[1].LineCostCents != 0 {
		t.Fatalf("line costs = %d/%d; want 4000/0", stored.Items[0].LineCostCents, stored.Items[1].LineCostCents)
	}

	repair, err := repairs.CreateFromOffer(ctx, tenant.ID, offer.ID)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if repair.CostTotalCents != 4000 {
		t.Fatalf("repair cost total = %d; want 4000 copied from the offer", repair.CostTotalCents)
	}
	if repair.Items[0].CostCents != 4000 {
		t.Fatalf("repair item cost = %d; want 4000", repair.Items[0].CostCents)
	}
	if repair.ProfitCents() != repair.SubtotalCents-repair.CostSubtotalCents {
		t.Fatal("repair profit does not match its own snapshots")
	}
}

func TestDashboardReportsFinishedWorkAndWhatIsStuck(t *testing.T) {
	ctx, db, repairs, offers, tenant, _, car := repairFixture(t)
	dashboards := NewDashboardStore(db)
	now := time.Now()

	// One completed repair this month.
	offer := costedOffer(tenant.ID, car.ID)
	if err := offers.Create(ctx, offer); err != nil {
		t.Fatalf("create offer: %v", err)
	}
	repair, err := repairs.CreateFromOffer(ctx, tenant.ID, offer.ID)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	completed, err := repairs.Complete(ctx, tenant.ID, repair.ID, 120000)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// One repair left open, and one quote left unanswered.
	openOffer := costedOffer(tenant.ID, car.ID)
	if err := offers.Create(ctx, openOffer); err != nil {
		t.Fatalf("create open offer: %v", err)
	}
	if _, err := repairs.CreateFromOffer(ctx, tenant.ID, openOffer.ID); err != nil {
		t.Fatalf("convert open offer: %v", err)
	}
	waiting := costedOffer(tenant.ID, car.ID)
	if err := offers.Create(ctx, waiting); err != nil {
		t.Fatalf("create waiting offer: %v", err)
	}

	dash, err := dashboards.Load(ctx, tenant.ID, now)
	if err != nil {
		t.Fatalf("load dashboard: %v", err)
	}

	if len(dash.Months) != DashboardMonths {
		t.Fatalf("got %d months; want %d - empty months must still be plotted", len(dash.Months), DashboardMonths)
	}
	current := dash.Months[len(dash.Months)-1]
	if current.Repairs != 1 {
		t.Fatalf("completed repairs this month = %d; want 1", current.Repairs)
	}
	if current.NetRevenueCents != completed.SubtotalCents {
		t.Fatalf("month revenue = %d; want %d", current.NetRevenueCents, completed.SubtotalCents)
	}
	if current.NetCostCents != completed.CostSubtotalCents {
		t.Fatalf("month cost = %d; want %d", current.NetCostCents, completed.CostSubtotalCents)
	}
	if current.NetProfitCents() != completed.ProfitCents() {
		t.Fatalf("month profit = %d; want %d", current.NetProfitCents(), completed.ProfitCents())
	}

	// Only the completed repair counts as income; the open one is work in progress.
	if dash.ActiveRepairs != 1 {
		t.Fatalf("active repairs = %d; want 1", dash.ActiveRepairs)
	}
	if dash.OpenOffers != 1 {
		t.Fatalf("open offers = %d; want 1", dash.OpenOffers)
	}
	if dash.AcceptedOffers != 2 || dash.DecidedOffers != 2 {
		t.Fatalf("acceptance = %d/%d; want 2/2", dash.AcceptedOffers, dash.DecidedOffers)
	}
	if dash.AcceptanceRate() != 100 {
		t.Fatalf("acceptance rate = %d; want 100", dash.AcceptanceRate())
	}

	// Nothing is old enough to be stale yet.
	if len(dash.StaleOffers) != 0 || len(dash.StalledRepairs) != 0 {
		t.Fatalf("fresh documents reported as stale: %d offers, %d repairs", len(dash.StaleOffers), len(dash.StalledRepairs))
	}

	// Reading "now" three weeks ahead is the same as those documents having sat
	// untouched for three weeks.
	later, err := dashboards.Load(ctx, tenant.ID, now.Add(21*24*time.Hour))
	if err != nil {
		t.Fatalf("load later dashboard: %v", err)
	}
	if len(later.StaleOffers) != 1 {
		t.Fatalf("stale offers = %d; want the one nobody answered", len(later.StaleOffers))
	}
	if len(later.StalledRepairs) != 1 {
		t.Fatalf("stalled repairs = %d; want the one still open", len(later.StalledRepairs))
	}
}

func TestDashboardSplitsRevenueByKind(t *testing.T) {
	ctx, db, repairs, offers, tenant, _, car := repairFixture(t)
	dashboards := NewDashboardStore(db)

	offer := costedOffer(tenant.ID, car.ID)
	if err := offers.Create(ctx, offer); err != nil {
		t.Fatalf("create offer: %v", err)
	}
	repair, err := repairs.CreateFromOffer(ctx, tenant.ID, offer.ID)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if _, err := repairs.Complete(ctx, tenant.ID, repair.ID, 100000); err != nil {
		t.Fatalf("complete: %v", err)
	}

	dash, err := dashboards.Load(ctx, tenant.ID, time.Now())
	if err != nil {
		t.Fatalf("load dashboard: %v", err)
	}

	byKind := map[string]KindSplit{}
	for _, s := range dash.Split {
		byKind[s.Kind] = s
	}
	part, labor := byKind["part"], byKind["labor"]
	if part.NetCostCents == 0 {
		t.Fatal("parts report no supplier cost")
	}
	if labor.NetCostCents != 0 {
		t.Fatalf("labour reports a supplier cost of %d; want 0", labor.NetCostCents)
	}
	if labor.NetProfitCents() != labor.NetRevenueCents {
		t.Fatal("labour should be pure margin")
	}
	if part.NetProfitCents() >= part.NetRevenueCents {
		t.Fatal("resold parts should keep less than they bill")
	}
}

// A dashboard opened on the 3rd of the month must compare three days with three
// days. Comparing whole months would report every early month as a collapse.
func TestDashboardComparesLikePeriods(t *testing.T) {
	ctx, db, repairs, offers, tenant, _, car := repairFixture(t)
	dashboards := NewDashboardStore(db)

	complete := func(at time.Time) {
		t.Helper()
		offer := costedOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, offer); err != nil {
			t.Fatalf("create offer: %v", err)
		}
		repair, err := repairs.CreateFromOffer(ctx, tenant.ID, offer.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		if _, err := repairs.Complete(ctx, tenant.ID, repair.ID, 100000); err != nil {
			t.Fatalf("complete: %v", err)
		}
		if err := db.WithTenant(ctx, tenant.ID, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE repairs SET completed_at = $2 WHERE id = $1 AND tenant_id = $3`, repair.ID, at, tenant.ID)
			return err
		}); err != nil {
			t.Fatalf("backdate: %v", err)
		}
	}

	// "Now" is the 10th: one job done so far this month, one in the first ten
	// days of last month, and three more later in that month.
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	complete(time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC))
	complete(time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC))
	for _, day := range []int{18, 23, 29} {
		complete(time.Date(2026, 5, day, 10, 0, 0, 0, time.UTC))
	}

	dash, err := dashboards.Load(ctx, tenant.ID, now)
	if err != nil {
		t.Fatalf("load dashboard: %v", err)
	}
	if dash.MonthToDate.Repairs != 1 {
		t.Fatalf("month to date = %d repairs; want 1", dash.MonthToDate.Repairs)
	}
	if dash.PreviousToDate.Repairs != 1 {
		t.Fatalf("same period last month = %d repairs; want 1, not the whole month's 4", dash.PreviousToDate.Repairs)
	}
	if dash.MonthToDate.NetRevenueCents != dash.PreviousToDate.NetRevenueCents {
		t.Fatalf("like periods should compare equal: %d vs %d",
			dash.MonthToDate.NetRevenueCents, dash.PreviousToDate.NetRevenueCents)
	}
	// The chart still shows whole months, so last month keeps all four.
	if last := dash.Months[len(dash.Months)-2]; last.Repairs != 4 {
		t.Fatalf("previous month on the chart = %d repairs; want the full 4", last.Repairs)
	}
}
