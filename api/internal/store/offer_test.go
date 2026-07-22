package store

import (
	"context"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

func newOffer(tenantID, carID string) *domain.Offer {
	return &domain.Offer{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		CarID:      carID,
		TaxRateBps: 1900,
		Items: []domain.OfferItem{
			{Kind: domain.OfferItemKindPart, Description: "Brake pads", Quantity: 2, UnitPriceCents: 4500},
			{Kind: domain.OfferItemKindLabor, Description: "Fitting", Quantity: 1, UnitPriceCents: 6000},
		},
	}
}

// offerFixture spins up a tenant + customer + car so offers have a valid,
// same-tenant car to anchor to (composite FK). Returns the DB (for building
// sibling rows) and the store bundle.
func offerFixture(t *testing.T) (context.Context, *DB, *OfferStore, *domain.Tenant, *domain.Car) {
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
	owner := newCustomer(tenant.ID, "Ivan Petrov")
	if err := custs.Create(ctx, owner); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	car := newCar(tenant.ID, owner.ID, "CB1234AB")
	if err := cars.Create(ctx, car); err != nil {
		t.Fatalf("create car: %v", err)
	}
	return ctx, db, NewOfferStore(db), tenant, car
}

func TestOfferStore(t *testing.T) {
	ctx, _, offers, tenant, car := offerFixture(t)

	t.Run("Create computes totals, defaults status, Get round-trips items", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		o.Notes = "Estimate valid 30 days"
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create offer: %v", err)
		}
		if o.Status != domain.OfferStatusDraft || o.SendStatus != domain.SendStatusPending {
			t.Fatalf("wrong initial lifecycle: status=%s send=%s", o.Status, o.SendStatus)
		}
		if o.CreatedAt.IsZero() || o.UpdatedAt.IsZero() {
			t.Fatalf("timestamps not populated: %+v", o)
		}
		// subtotal = 90.00 + 60.00 = 150.00; tax 19% = 28.50; total = 178.50.
		if o.SubtotalCents != 15000 || o.TaxCents != 2850 || o.TotalCents != 17850 {
			t.Fatalf("totals wrong: sub=%d tax=%d total=%d", o.SubtotalCents, o.TaxCents, o.TotalCents)
		}

		got, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("get offer: %v", err)
		}
		if got.Notes != o.Notes || got.TaxRateBps != 1900 || got.CarID != car.ID {
			t.Fatalf("offer mismatch: %+v", got)
		}
		if len(got.Items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(got.Items))
		}
		// Editor order preserved, line totals persisted server-side.
		if got.Items[0].Description != "Brake pads" || got.Items[0].LineTotalCents != 9000 || got.Items[0].SortOrder != 0 {
			t.Fatalf("item[0] wrong: %+v", got.Items[0])
		}
		if got.Items[1].Description != "Fitting" || got.Items[1].LineTotalCents != 6000 || got.Items[1].SortOrder != 1 {
			t.Fatalf("item[1] wrong: %+v", got.Items[1])
		}
	})

	t.Run("Get unknown id returns ErrNotFound", func(t *testing.T) {
		if _, err := offers.Get(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update replaces items and recomputes while draft", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create: %v", err)
		}
		firstUpdated := o.UpdatedAt

		o.TaxRateBps = 2000
		o.Notes = "revised"
		o.Items = []domain.OfferItem{
			{Kind: domain.OfferItemKindOther, Description: "Diagnostics", Quantity: 1, UnitPriceCents: 3000},
		}
		if err := offers.Update(ctx, o); err != nil {
			t.Fatalf("update: %v", err)
		}

		got, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("get after update: %v", err)
		}
		if len(got.Items) != 1 || got.Items[0].Description != "Diagnostics" {
			t.Fatalf("items not replaced: %+v", got.Items)
		}
		// subtotal 30.00, tax 20% = 6.00, total 36.00.
		if got.SubtotalCents != 3000 || got.TaxCents != 600 || got.TotalCents != 3600 {
			t.Fatalf("recompute wrong: sub=%d tax=%d total=%d", got.SubtotalCents, got.TaxCents, got.TotalCents)
		}
		if got.Notes != "revised" {
			t.Fatalf("notes not updated: %q", got.Notes)
		}
		if !got.UpdatedAt.After(firstUpdated) {
			t.Fatalf("updated_at not bumped")
		}
	})

	t.Run("Update unknown id returns ErrNotFound", func(t *testing.T) {
		ghost := newOffer(tenant.ID, car.ID)
		if err := offers.Update(ctx, ghost); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("SetStatus enforces the lifecycle machine", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create: %v", err)
		}

		// draft → accepted is not allowed (must send first).
		if _, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusAccepted); err != ErrInvalidStatusTransition {
			t.Fatalf("expected ErrInvalidStatusTransition, got %v", err)
		}

		sent, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusSent)
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		if sent.Status != domain.OfferStatusSent {
			t.Fatalf("status not sent: %s", sent.Status)
		}
		if len(sent.Items) != 2 {
			t.Fatalf("SetStatus should return items, got %d", len(sent.Items))
		}

		accepted, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusAccepted)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if accepted.Status != domain.OfferStatusAccepted {
			t.Fatalf("status not accepted: %s", accepted.Status)
		}

		// accepted is terminal.
		if _, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusExpired); err != ErrInvalidStatusTransition {
			t.Fatalf("expected terminal, got %v", err)
		}
	})

	t.Run("SetStatus unknown id returns ErrNotFound", func(t *testing.T) {
		if _, err := offers.SetStatus(ctx, tenant.ID, uuid.NewString(), domain.OfferStatusSent); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update on a sent offer is rejected with ErrOfferNotDraft", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusSent); err != nil {
			t.Fatalf("send: %v", err)
		}

		o.Notes = "sneaky post-send edit"
		if err := offers.Update(ctx, o); err != ErrOfferNotDraft {
			t.Fatalf("expected ErrOfferNotDraft, got %v", err)
		}

		// The offer must be untouched by the rejected write.
		got, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Notes == "sneaky post-send edit" {
			t.Fatalf("sent offer was mutated")
		}
	})

	t.Run("List is scoped to a car and ordered newest first", func(t *testing.T) {
		// Fresh tenant so counts are deterministic. car2 gets 3 offers; a second
		// car in the same tenant gets one that must NOT leak into car2's list.
		ctx2, db2, offers2, tenant2, car2 := offerFixture(t)

		sibling := newCar(tenant2.ID, car2.CustomerID, "SIB0001")
		if err := NewCarStore(db2).Create(ctx2, sibling); err != nil {
			t.Fatalf("create sibling car: %v", err)
		}
		if err := offers2.Create(ctx2, newOffer(tenant2.ID, sibling.ID)); err != nil {
			t.Fatalf("create sibling offer: %v", err)
		}

		for i := 0; i < 3; i++ {
			o := newOffer(tenant2.ID, car2.ID)
			if err := offers2.Create(ctx2, o); err != nil {
				t.Fatalf("create %d: %v", i, err)
			}
		}

		list, total, err := offers2.List(ctx2, tenant2.ID, car2.ID, OfferListParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if total != 3 {
			t.Fatalf("total: got %d, want 3 (sibling car's offer must not count)", total)
		}
		if len(list) != 2 {
			t.Fatalf("page size: got %d, want 2", len(list))
		}
		for _, o := range list {
			if o.CarID != car2.ID {
				t.Fatalf("leaked another car's offer: %s", o.CarID)
			}
		}
		// Newest first: created_at DESC.
		if list[0].CreatedAt.Before(list[1].CreatedAt) {
			t.Fatalf("not newest-first: %v then %v", list[0].CreatedAt, list[1].CreatedAt)
		}
		// List view omits items.
		if list[0].Items != nil {
			t.Fatalf("list should not load items")
		}
	})

	t.Run("cross-tenant access is invisible", func(t *testing.T) {
		ctx2, _, offers2, tenantA, carA := offerFixture(t)
		mine := newOffer(tenantA.ID, carA.ID)
		if err := offers2.Create(ctx2, mine); err != nil {
			t.Fatalf("create mine: %v", err)
		}

		other := uuid.NewString()
		if _, err := offers2.Get(ctx2, other, mine.ID); err != ErrNotFound {
			t.Fatalf("cross-tenant Get: expected ErrNotFound, got %v", err)
		}
		poison := *mine
		poison.TenantID = other
		poison.Notes = "hacked"
		if err := offers2.Update(ctx2, &poison); err != ErrNotFound {
			t.Fatalf("cross-tenant Update: expected ErrNotFound, got %v", err)
		}
		if _, err := offers2.SetStatus(ctx2, other, mine.ID, domain.OfferStatusSent); err != ErrNotFound {
			t.Fatalf("cross-tenant SetStatus: expected ErrNotFound, got %v", err)
		}
	})
}
