package demo_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"

	"github.com/gfotev/pitlane/internal/demo"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/testdb"
)

func newDB(t *testing.T) *store.DB {
	t.Helper()
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })
	return store.NewDB(tdb.Pool)
}

func TestSeedProducesAShowableGarage(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	now := time.Now()

	result, err := demo.Seed(ctx, db, demo.Options{Now: now})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if result.Customers == 0 || result.Cars == 0 || result.Repairs == 0 || result.Offers == 0 {
		t.Fatalf("seed produced an empty garage: %+v", result)
	}

	dash, err := store.NewDashboardStore(db).Load(ctx, result.TenantID, now)
	if err != nil {
		t.Fatalf("load dashboard: %v", err)
	}

	// Every month of the chart must have finished work in it: a demonstration
	// with gaps in the trend is the thing this fixture exists to avoid.
	for _, month := range dash.Months {
		if month.Repairs == 0 {
			t.Errorf("%s has no completed repairs", month.Month.Format("2006-01"))
		}
		if month.NetProfitCents() <= 0 || month.NetProfitCents() >= month.NetRevenueCents {
			t.Errorf("%s profit %d is not a believable share of revenue %d",
				month.Month.Format("2006-01"), month.NetProfitCents(), month.NetRevenueCents)
		}
	}

	// Parts must be resold at a markup and labour must be pure margin, or the
	// "where the money comes from" panel has nothing to say.
	byKind := map[string]store.KindSplit{}
	for _, s := range dash.Split {
		byKind[s.Kind] = s
	}
	if byKind["part"].NetCostCents == 0 {
		t.Error("parts carry no supplier cost")
	}
	if byKind["labor"].NetCostCents != 0 {
		t.Error("labour carries a supplier cost")
	}

	if dash.ActiveRepairs == 0 {
		t.Error("nothing is in the workshop")
	}
	if dash.OpenOffers == 0 {
		t.Error("no quotes are waiting for an answer")
	}
	if rate := dash.AcceptanceRate(); rate == 0 || rate >= 100 {
		t.Errorf("acceptance rate %d%% is not believable - some quotes must be refused", rate)
	}
	if len(dash.StaleOffers) == 0 {
		t.Error("no stale quote for the attention panel")
	}
	if len(dash.StalledRepairs) == 0 {
		t.Error("no stalled repair for the attention panel")
	}
}

// A document written last November must not be numbered as if it were issued
// this year, and the counters must continue from what was issued.
func TestSeedNumbersDocumentsByTheirOwnYear(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()

	result, err := demo.Seed(ctx, db, demo.Options{Now: time.Now()})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	repairs, _, err := store.NewRepairStore(db).List(ctx, result.TenantID, store.RepairListParams{Limit: 500})
	if err != nil {
		t.Fatalf("list repairs: %v", err)
	}
	seen := map[string]bool{}
	for _, summary := range repairs {
		number := summary.Repair.DocumentNumber
		if seen[number] {
			t.Fatalf("duplicate document number %s", number)
		}
		seen[number] = true
		if want := summary.Repair.CreatedAt.Format("2006"); number[3:7] != want {
			t.Errorf("repair %s was created in %s", number, want)
		}
	}
}

func TestPurgeEmptiesTheTenantAndRefusesRealOnes(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()

	result, err := demo.Seed(ctx, db, demo.Options{Now: time.Now()})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A garage that was not created as a demonstration is untouchable, which is
	// the whole point of the flag.
	real := newTenant(t, ctx, db)
	if err := demo.Purge(ctx, db, real); err == nil {
		t.Fatal("purge deleted a real tenant")
	} else if !isNotDemo(err) {
		t.Fatalf("purge failed for the wrong reason: %v", err)
	}

	if err := demo.Purge(ctx, db, result.TenantID); err != nil {
		t.Fatalf("purge: %v", err)
	}
	if _, err := demo.Find(ctx, db, result.Email); err == nil {
		t.Fatal("the demonstration login still exists")
	}

	// Seeding again after a purge must work, because that is what happens
	// before every demonstration.
	if _, err := demo.Seed(ctx, db, demo.Options{Now: time.Now()}); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
}

func TestSeedRefusesToRunTwice(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	if _, err := demo.Seed(ctx, db, demo.Options{Now: time.Now()}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := demo.Seed(ctx, db, demo.Options{Now: time.Now()}); err == nil {
		t.Fatal("seeding twice silently duplicated the demonstration")
	}
}

func newTenant(t *testing.T, ctx context.Context, db *store.DB) string {
	t.Helper()
	tenant := &domain.Tenant{ID: uuid.NewString(), Name: "Истински сервиз", Currency: "EUR", Locale: "bg", DefaultTaxRate: domain.StandardVATRateBPS, Settings: map[string]any{}}
	if err := store.NewTenantStore(db).Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	return tenant.ID
}

func isNotDemo(err error) bool { return errors.Is(err, demo.ErrNotDemoTenant) }
