package store

import (
	"context"
	"strings"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

func repairFixture(t *testing.T) (context.Context, *DB, *RepairStore, *OfferStore, *domain.Tenant, *domain.Customer, *domain.Car) {
	t.Helper()
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := NewDB(tdb.Pool)
	ts := NewTenantStore(db)
	custs := NewCustomerStore(db)
	cars := NewCarStore(db)
	ctx := context.Background()

	tenant := newTenant("Garage")
	if err := ts.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	customer := newCustomer(tenant.ID, "Ivan Petrov")
	if err := custs.Create(ctx, customer); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	car := newCar(tenant.ID, customer.ID, "CB1234AB")
	if err := cars.Create(ctx, car); err != nil {
		t.Fatalf("create car: %v", err)
	}
	return ctx, db, NewRepairStore(db), NewOfferStore(db), tenant, customer, car
}

func sentOffer(t *testing.T, ctx context.Context, offers *OfferStore, tenantID, carID string) *domain.Offer {
	t.Helper()
	o := newOffer(tenantID, carID)
	if err := offers.Create(ctx, o); err != nil {
		t.Fatalf("create offer: %v", err)
	}
	if _, err := offers.MarkSending(ctx, tenantID, o.ID, "customer@example.com", &fakeEnqueuer{}); err != nil {
		t.Fatalf("send offer: %v", err)
	}
	return o
}

func TestRepairStore(t *testing.T) {
	ctx, _, repairs, offers, tenant, _, car := repairFixture(t)

	t.Run("CreateFromOffer copies items, accepts the offer, is open", func(t *testing.T) {
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)

		r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		if r.Status != domain.RepairStatusOpen {
			t.Fatalf("new repair should be open, got %s", r.Status)
		}
		if !strings.HasPrefix(r.DocumentNumber, "RP-") || !strings.HasSuffix(r.DocumentNumber, "-000001") {
			t.Fatalf("repair document number = %q", r.DocumentNumber)
		}
		if !strings.HasPrefix(o.DocumentNumber, "OF-") {
			t.Fatalf("offer document number = %q", o.DocumentNumber)
		}
		if r.OfferID == nil || *r.OfferID != o.ID {
			t.Fatalf("provenance not linked: %+v", r.OfferID)
		}
		if r.CarID != car.ID {
			t.Fatalf("car not carried over: %s", r.CarID)
		}
		if r.SubtotalCents != 12605 || r.TaxCents != 2395 || r.TotalCents != 15000 {
			t.Fatalf("totals wrong: sub=%d tax=%d total=%d", r.SubtotalCents, r.TaxCents, r.TotalCents)
		}
		if len(r.Items) != 2 {
			t.Fatalf("expected 2 copied items, got %d", len(r.Items))
		}
		byOffer, err := repairs.GetByOffer(ctx, tenant.ID, o.ID)
		if err != nil || byOffer.ID != r.ID {
			t.Fatalf("get by offer: repair=%+v err=%v", byOffer, err)
		}
		for _, it := range r.Items {
			if it.RepairID != r.ID {
				t.Fatalf("item not linked to repair: %+v", it)
			}
		}

		acc, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("reload offer: %v", err)
		}
		if acc.Status != domain.OfferStatusAccepted {
			t.Fatalf("offer should be accepted after convert, got %s", acc.Status)
		}
	})

	t.Run("copy-not-share: editing repair items leaves the offer untouched", func(t *testing.T) {
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}

		r.Mileage = 90000
		r.Items = []domain.RepairItem{
			{Kind: domain.OfferItemKindLabor, Description: "Extra diagnosis", Quantity: 3, UnitPriceCents: 5000},
		}
		if err := repairs.Update(ctx, r); err != nil {
			t.Fatalf("update repair: %v", err)
		}

		off, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("reload offer: %v", err)
		}
		if len(off.Items) != 2 || off.TotalCents != 15000 {
			t.Fatalf("offer mutated by repair edit: items=%d total=%d", len(off.Items), off.TotalCents)
		}
		reloaded, err := repairs.Get(ctx, tenant.ID, r.ID)
		if err != nil {
			t.Fatalf("reload repair: %v", err)
		}
		if len(reloaded.Items) != 1 || reloaded.SubtotalCents != 12605 || reloaded.TotalCents != 15000 || reloaded.Mileage != 90000 {
			t.Fatalf("repair not updated: items=%d sub=%d total=%d mileage=%d", len(reloaded.Items), reloaded.SubtotalCents, reloaded.TotalCents, reloaded.Mileage)
		}
		updatedCar, err := NewCarStore(NewDB(repairs.db.pool)).Get(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("reload car: %v", err)
		}
		if updatedCar.Mileage != 90000 {
			t.Fatalf("car mileage not advanced by repair edit: %d", updatedCar.Mileage)
		}
	})

	t.Run("CreateFromOffer accepts drafts and gates terminal states", func(t *testing.T) {
		draft := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, draft); err != nil {
			t.Fatalf("create draft: %v", err)
		}
		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, draft.ID); err != nil {
			t.Fatalf("convert draft: %v", err)
		}

		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, draft.ID); err != ErrOfferNotAcceptable {
			t.Fatalf("expected ErrOfferNotAcceptable on re-convert, got %v", err)
		}

		sent := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, sent.ID); err != nil {
			t.Fatalf("convert sent offer: %v", err)
		}

		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Get unknown id returns ErrNotFound", func(t *testing.T) {
		if _, err := repairs.Get(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
		if _, err := repairs.GetByOffer(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("GetByOffer: expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update is open-only", func(t *testing.T) {
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		if _, err := repairs.SetStatus(ctx, tenant.ID, r.ID, domain.RepairStatusInProgress); err != nil {
			t.Fatalf("start: %v", err)
		}
		r.Notes = "should be rejected"
		if err := repairs.Update(ctx, r); err != ErrRepairNotOpen {
			t.Fatalf("expected ErrRepairNotOpen, got %v", err)
		}

		ghost := &domain.Repair{ID: uuid.NewString(), TenantID: tenant.ID, TaxRateBps: 1900}
		if err := repairs.Update(ctx, ghost); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("SetStatus enforces the machine", func(t *testing.T) {
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}

		if _, err := repairs.SetStatus(ctx, tenant.ID, r.ID, domain.RepairStatusCompleted); err != ErrInvalidRepairStatusTransition {
			t.Fatalf("expected ErrInvalidRepairStatusTransition, got %v", err)
		}
		started, err := repairs.SetStatus(ctx, tenant.ID, r.ID, domain.RepairStatusInProgress)
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		if started.Status != domain.RepairStatusInProgress {
			t.Fatalf("status not in_progress: %s", started.Status)
		}
		if _, err := repairs.SetStatus(ctx, tenant.ID, r.ID, domain.RepairStatusOpen); err != nil {
			t.Fatalf("revert: %v", err)
		}

		if _, err := repairs.SetStatus(ctx, tenant.ID, uuid.NewString(), domain.RepairStatusInProgress); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Complete freezes and advances the car odometer", func(t *testing.T) {
		cars := NewCarStore(NewDB(repairs.db.pool))
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}

		done, err := repairs.Complete(ctx, tenant.ID, r.ID, 120000)
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
		if done.Status != domain.RepairStatusCompleted || done.CompletedAt == nil {
			t.Fatalf("not completed: status=%s completedAt=%v", done.Status, done.CompletedAt)
		}
		if done.Mileage != 120000 {
			t.Fatalf("mileage not recorded on repair: %d", done.Mileage)
		}
		c, err := cars.Get(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("reload car: %v", err)
		}
		if c.Mileage != 120000 {
			t.Fatalf("car mileage not advanced: %d", c.Mileage)
		}

		corrected, err := repairs.UpdateMileage(ctx, tenant.ID, r.ID, 125000)
		if err != nil {
			t.Fatalf("correct mileage: %v", err)
		}
		if corrected.Mileage != 125000 {
			t.Fatalf("repair mileage not corrected: %d", corrected.Mileage)
		}
		c, err = cars.Get(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("reload car after correction: %v", err)
		}
		if c.Mileage != 125000 {
			t.Fatalf("car mileage not advanced by correction: %d", c.Mileage)
		}

		corrected, err = repairs.UpdateMileage(ctx, tenant.ID, r.ID, 115000)
		if err != nil {
			t.Fatalf("correct mileage downward: %v", err)
		}
		c, err = cars.Get(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("reload car after downward correction: %v", err)
		}
		if corrected.Mileage != 115000 || c.Mileage != 115000 {
			t.Fatalf("downward correction not synced: repair=%d car=%d", corrected.Mileage, c.Mileage)
		}

		stats, err := repairs.Stats(ctx, tenant.ID)
		if err != nil {
			t.Fatalf("stats: %v", err)
		}
		if stats.Completed < 1 || stats.RevenueCents < done.TotalCents {
			t.Fatalf("completed repair missing from revenue stats: %+v", stats)
		}

		if _, err := repairs.Complete(ctx, tenant.ID, r.ID, 130000); err != ErrRepairNotOpen {
			t.Fatalf("expected ErrRepairNotOpen on re-complete, got %v", err)
		}
		o2 := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		r2, err := repairs.CreateFromOffer(ctx, tenant.ID, o2.ID)
		if err != nil {
			t.Fatalf("convert 2: %v", err)
		}
		if _, err := repairs.Complete(ctx, tenant.ID, r2.ID, 100000); err != nil {
			t.Fatalf("complete 2: %v", err)
		}
		c2, err := cars.Get(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("reload car 2: %v", err)
		}
		if c2.Mileage != 115000 {
			t.Fatalf("car mileage rolled backwards: %d", c2.Mileage)
		}
		older, err := repairs.UpdateMileage(ctx, tenant.ID, r2.ID, 50000)
		if err != nil {
			t.Fatalf("correct older repair: %v", err)
		}
		c2, err = cars.Get(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("reload car after older correction: %v", err)
		}
		if older.Mileage != 50000 || c2.Mileage != 115000 {
			t.Fatalf("older correction changed current car mileage: repair=%d car=%d", older.Mileage, c2.Mileage)
		}

		if _, err := repairs.Complete(ctx, tenant.ID, uuid.NewString(), 1); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
		if _, err := repairs.UpdateMileage(ctx, tenant.ID, uuid.NewString(), 1); err != ErrNotFound {
			t.Fatalf("UpdateMileage: expected ErrNotFound, got %v", err)
		}
	})

	t.Run("List is tenant-wide, status-filtered, and enriched", func(t *testing.T) {
		ctx2, _, repairs2, offers2, tenant2, customer2, car2 := repairFixture(t)

		oA := sentOffer(t, ctx2, offers2, tenant2.ID, car2.ID)
		rA, err := repairs2.CreateFromOffer(ctx2, tenant2.ID, oA.ID)
		if err != nil {
			t.Fatalf("convert A: %v", err)
		}
		oB := sentOffer(t, ctx2, offers2, tenant2.ID, car2.ID)
		rB, err := repairs2.CreateFromOffer(ctx2, tenant2.ID, oB.ID)
		if err != nil {
			t.Fatalf("convert B: %v", err)
		}
		if _, err := repairs2.Complete(ctx2, tenant2.ID, rB.ID, 50000); err != nil {
			t.Fatalf("complete B: %v", err)
		}

		all, total, err := repairs2.List(ctx2, tenant2.ID, RepairListParams{Limit: 10})
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if total != 2 || len(all) != 2 {
			t.Fatalf("expected 2 repairs, got total=%d len=%d", total, len(all))
		}
		if all[0].CarPlate != car2.Plate || all[0].CustomerName != customer2.Name {
			t.Fatalf("enrichment missing: plate=%q name=%q", all[0].CarPlate, all[0].CustomerName)
		}

		open, openTotal, err := repairs2.List(ctx2, tenant2.ID, RepairListParams{Status: "open", Limit: 10})
		if err != nil {
			t.Fatalf("list open: %v", err)
		}
		if openTotal != 1 || len(open) != 1 || open[0].Repair.ID != rA.ID {
			t.Fatalf("status filter wrong: total=%d len=%d", openTotal, len(open))
		}
	})
}

func TestRepairTenantIsolation(t *testing.T) {
	ctx, db, repairs, offers, tenantA, _, carA := repairFixture(t)

	oA := sentOffer(t, ctx, offers, tenantA.ID, carA.ID)
	rA, err := repairs.CreateFromOffer(ctx, tenantA.ID, oA.ID)
	if err != nil {
		t.Fatalf("convert A: %v", err)
	}

	ts := NewTenantStore(db)
	tenantB := newTenant("Garage B")
	if err := ts.Create(ctx, tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	if _, err := repairs.Get(ctx, tenantB.ID, rA.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant Get: expected ErrNotFound, got %v", err)
	}
	if _, err := repairs.GetByOffer(ctx, tenantB.ID, oA.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant GetByOffer: expected ErrNotFound, got %v", err)
	}
	stats, err := repairs.Stats(ctx, tenantB.ID)
	if err != nil || stats.Total != 0 || stats.RevenueCents != 0 {
		t.Fatalf("cross-tenant Stats leaked data: stats=%+v err=%v", stats, err)
	}
	poison := *rA
	poison.TenantID = tenantB.ID
	poison.Notes = "hacked"
	if err := repairs.Update(ctx, &poison); err != ErrNotFound {
		t.Fatalf("cross-tenant Update: expected ErrNotFound, got %v", err)
	}
	if _, err := repairs.SetStatus(ctx, tenantB.ID, rA.ID, domain.RepairStatusInProgress); err != ErrNotFound {
		t.Fatalf("cross-tenant SetStatus: expected ErrNotFound, got %v", err)
	}
	if _, err := repairs.Complete(ctx, tenantB.ID, rA.ID, 1); err != ErrNotFound {
		t.Fatalf("cross-tenant Complete: expected ErrNotFound, got %v", err)
	}
	if _, err := repairs.UpdateMileage(ctx, tenantB.ID, rA.ID, 1); err != ErrNotFound {
		t.Fatalf("cross-tenant UpdateMileage: expected ErrNotFound, got %v", err)
	}
	if _, err := repairs.CreateFromOffer(ctx, tenantB.ID, oA.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant convert: expected ErrNotFound, got %v", err)
	}
}
