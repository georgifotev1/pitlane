package store

import (
	"context"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

// repairFixture spins up a tenant + customer + car and both the offer and
// repair stores, so a repair can be born the only way it can — by converting a
// sent offer.
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

// sentOffer creates a draft offer and drives it to `sent` so it can be
// converted. newOffer (offer_test.go) builds the two-line, 19% fixture.
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
		if r.OfferID == nil || *r.OfferID != o.ID {
			t.Fatalf("provenance not linked: %+v", r.OfferID)
		}
		if r.CarID != car.ID {
			t.Fatalf("car not carried over: %s", r.CarID)
		}
		// Totals reproduce the offer's frozen snapshot exactly.
		if r.SubtotalCents != 15000 || r.TaxCents != 2850 || r.TotalCents != 17850 {
			t.Fatalf("totals wrong: sub=%d tax=%d total=%d", r.SubtotalCents, r.TaxCents, r.TotalCents)
		}
		if len(r.Items) != 2 {
			t.Fatalf("expected 2 copied items, got %d", len(r.Items))
		}
		// Items are a COPY: fresh IDs, linked to the repair, not the offer.
		for _, it := range r.Items {
			if it.RepairID != r.ID {
				t.Fatalf("item not linked to repair: %+v", it)
			}
		}

		// The offer is now accepted (accept ≡ convert).
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

		// Rewrite the repair's items completely.
		r.Items = []domain.RepairItem{
			{Kind: domain.OfferItemKindLabor, Description: "Extra diagnosis", Quantity: 3, UnitPriceCents: 5000},
		}
		if err := repairs.Update(ctx, r); err != nil {
			t.Fatalf("update repair: %v", err)
		}

		// The source offer's items and totals are unchanged.
		off, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("reload offer: %v", err)
		}
		if len(off.Items) != 2 || off.TotalCents != 17850 {
			t.Fatalf("offer mutated by repair edit: items=%d total=%d", len(off.Items), off.TotalCents)
		}
		// The repair reflects the new lines (3 * 50.00 = 150.00; +19% = 178.50).
		reloaded, err := repairs.Get(ctx, tenant.ID, r.ID)
		if err != nil {
			t.Fatalf("reload repair: %v", err)
		}
		if len(reloaded.Items) != 1 || reloaded.SubtotalCents != 15000 || reloaded.TotalCents != 17850 {
			t.Fatalf("repair not updated: items=%d sub=%d total=%d", len(reloaded.Items), reloaded.SubtotalCents, reloaded.TotalCents)
		}
	})

	t.Run("CreateFromOffer gates on offer status and existence", func(t *testing.T) {
		// A draft (never sent) offer cannot be converted.
		draft := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, draft); err != nil {
			t.Fatalf("create draft: %v", err)
		}
		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, draft.ID); err != ErrOfferNotAcceptable {
			t.Fatalf("expected ErrOfferNotAcceptable for draft, got %v", err)
		}

		// Converting the same sent offer twice fails the second time (already
		// accepted, no longer sent).
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID); err != nil {
			t.Fatalf("first convert: %v", err)
		}
		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID); err != ErrOfferNotAcceptable {
			t.Fatalf("expected ErrOfferNotAcceptable on re-convert, got %v", err)
		}

		// Unknown offer.
		if _, err := repairs.CreateFromOffer(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Get unknown id returns ErrNotFound", func(t *testing.T) {
		if _, err := repairs.Get(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update is open-only", func(t *testing.T) {
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		// Start the work: now frozen.
		if _, err := repairs.SetStatus(ctx, tenant.ID, r.ID, domain.RepairStatusInProgress); err != nil {
			t.Fatalf("start: %v", err)
		}
		r.Notes = "should be rejected"
		if err := repairs.Update(ctx, r); err != ErrRepairNotOpen {
			t.Fatalf("expected ErrRepairNotOpen, got %v", err)
		}

		// Unknown id.
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

		// open → completed is NOT a generic move (Complete owns it).
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
		// Revert is allowed.
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
		// Car odometer advanced to the reading.
		c, err := cars.Get(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("reload car: %v", err)
		}
		if c.Mileage != 120000 {
			t.Fatalf("car mileage not advanced: %d", c.Mileage)
		}

		// Completing again fails (terminal).
		if _, err := repairs.Complete(ctx, tenant.ID, r.ID, 130000); err != ErrRepairNotOpen {
			t.Fatalf("expected ErrRepairNotOpen on re-complete, got %v", err)
		}
		// A lower later reading never rolls the car back.
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
		if c2.Mileage != 120000 {
			t.Fatalf("car mileage rolled backwards: %d", c2.Mileage)
		}

		if _, err := repairs.Complete(ctx, tenant.ID, uuid.NewString(), 1); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("List is tenant-wide, status-filtered, and enriched", func(t *testing.T) {
		// Fresh tenant so the count is deterministic amid the other subtests.
		ctx2, _, repairs2, offers2, tenant2, customer2, car2 := repairFixture(t)

		// Two repairs; leave one open, complete the other.
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
		// Enriched with car plate + customer name.
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

// TestRepairTenantIsolation proves a second tenant cannot read or mutate the
// first tenant's repair through the store (the app-filter layer; RLS is proven
// separately in the API package).
func TestRepairTenantIsolation(t *testing.T) {
	ctx, db, repairs, offers, tenantA, _, carA := repairFixture(t)

	oA := sentOffer(t, ctx, offers, tenantA.ID, carA.ID)
	rA, err := repairs.CreateFromOffer(ctx, tenantA.ID, oA.ID)
	if err != nil {
		t.Fatalf("convert A: %v", err)
	}

	// A second tenant in the same database.
	ts := NewTenantStore(db)
	tenantB := newTenant("Garage B")
	if err := ts.Create(ctx, tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	if _, err := repairs.Get(ctx, tenantB.ID, rA.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant Get: expected ErrNotFound, got %v", err)
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
	// B cannot convert A's offer either.
	if _, err := repairs.CreateFromOffer(ctx, tenantB.ID, oA.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant convert: expected ErrNotFound, got %v", err)
	}
}
