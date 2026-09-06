package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeEnqueuer struct {
	calls   int
	failErr error
}

func (f *fakeEnqueuer) EnqueueOfferEmail(_ context.Context, _ pgx.Tx, _, _ string) error {
	f.calls++
	return f.failErr
}

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

	t.Run("Create allocates sequential tenant document numbers", func(t *testing.T) {
		ctx2, _, offers2, tenant2, car2 := offerFixture(t)
		first := newOffer(tenant2.ID, car2.ID)
		second := newOffer(tenant2.ID, car2.ID)
		if err := offers2.Create(ctx2, first); err != nil {
			t.Fatalf("create first: %v", err)
		}
		if err := offers2.Create(ctx2, second); err != nil {
			t.Fatalf("create second: %v", err)
		}
		if !strings.HasPrefix(first.DocumentNumber, "OF-") || !strings.HasSuffix(first.DocumentNumber, "-000001") {
			t.Fatalf("first document number = %q", first.DocumentNumber)
		}
		if !strings.HasPrefix(second.DocumentNumber, "OF-") || !strings.HasSuffix(second.DocumentNumber, "-000002") {
			t.Fatalf("second document number = %q", second.DocumentNumber)
		}
		if first.DocumentNumber[3:7] != second.DocumentNumber[3:7] {
			t.Fatalf("documents should use the same annual series: %q, %q", first.DocumentNumber, second.DocumentNumber)
		}
	})

	t.Run("Create computes totals, defaults status, Get round-trips items", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		o.Notes = "Estimate valid 30 days"
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create offer: %v", err)
		}
		if o.Status != domain.OfferStatusDraft || o.SendStatus != domain.SendStatusPending {
			t.Fatalf("wrong initial lifecycle: status=%s send=%s", o.Status, o.SendStatus)
		}
		if !strings.HasPrefix(o.DocumentNumber, "OF-") {
			t.Fatalf("document number not allocated: %q", o.DocumentNumber)
		}
		if o.CreatedAt.IsZero() || o.UpdatedAt.IsZero() {
			t.Fatalf("timestamps not populated: %+v", o)
		}
		if o.SubtotalCents != 12605 || o.TaxCents != 2395 || o.TotalCents != 15000 {
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
		if got.SubtotalCents != 2500 || got.TaxCents != 500 || got.TotalCents != 3000 {
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

		if _, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusRejected); err != ErrInvalidStatusTransition {
			t.Fatalf("expected ErrInvalidStatusTransition, got %v", err)
		}
		if _, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusSent); err != ErrInvalidStatusTransition {
			t.Fatalf("expected ErrInvalidStatusTransition for draft→sent, got %v", err)
		}

		sent, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", &fakeEnqueuer{})
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		if sent.Status != domain.OfferStatusSent {
			t.Fatalf("status not sent: %s", sent.Status)
		}
		if len(sent.Items) != 2 {
			t.Fatalf("MarkSending should return items, got %d", len(sent.Items))
		}

		if _, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusAccepted); err != ErrInvalidStatusTransition {
			t.Fatalf("expected ErrInvalidStatusTransition for sent→accepted, got %v", err)
		}

		rejected, err := offers.SetStatus(ctx, tenant.ID, o.ID, domain.OfferStatusRejected)
		if err != nil {
			t.Fatalf("reject: %v", err)
		}
		if rejected.Status != domain.OfferStatusRejected {
			t.Fatalf("status not rejected: %s", rejected.Status)
		}

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
		if _, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", &fakeEnqueuer{}); err != nil {
			t.Fatalf("send: %v", err)
		}

		o.Notes = "sneaky post-send edit"
		if err := offers.Update(ctx, o); err != ErrOfferNotDraft {
			t.Fatalf("expected ErrOfferNotDraft, got %v", err)
		}

		got, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Notes == "sneaky post-send edit" {
			t.Fatalf("sent offer was mutated")
		}
	})

	t.Run("List is scoped to a car and ordered newest first", func(t *testing.T) {
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
		if list[0].CreatedAt.Before(list[1].CreatedAt) {
			t.Fatalf("not newest-first: %v then %v", list[0].CreatedAt, list[1].CreatedAt)
		}
		if list[0].Items != nil {
			t.Fatalf("list should not load items")
		}
	})

	t.Run("ListAll is a tenant-wide board with status filter and enrichment", func(t *testing.T) {
		ctx2, db2, offers2, tenant2, car2 := offerFixture(t)
		other := newCar(tenant2.ID, car2.CustomerID, "OT9999HH")
		if err := NewCarStore(db2).Create(ctx2, other); err != nil {
			t.Fatalf("create second car: %v", err)
		}

		draft := newOffer(tenant2.ID, car2.ID)
		if err := offers2.Create(ctx2, draft); err != nil {
			t.Fatalf("create draft: %v", err)
		}
		sent := newOffer(tenant2.ID, other.ID)
		if err := offers2.Create(ctx2, sent); err != nil {
			t.Fatalf("create sent: %v", err)
		}
		if _, err := offers2.MarkSending(ctx2, tenant2.ID, sent.ID, "customer@example.com", &fakeEnqueuer{}); err != nil {
			t.Fatalf("send: %v", err)
		}

		board, total, err := offers2.ListAll(ctx2, tenant2.ID, OfferBoardParams{Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if total != 2 || len(board) != 2 {
			t.Fatalf("board: got total=%d len=%d, want 2/2", total, len(board))
		}
		if board[0].Offer.ID != sent.ID {
			t.Fatalf("not newest-first: first is %s", board[0].Offer.ID)
		}
		byID := map[string]OfferSummary{}
		for _, sm := range board {
			byID[sm.Offer.ID] = sm
		}
		if byID[draft.ID].CarPlate != car2.Plate || byID[draft.ID].CustomerName != "Ivan Petrov" {
			t.Fatalf("draft enrichment wrong: %+v", byID[draft.ID])
		}
		if byID[sent.ID].CarPlate != other.Plate || byID[sent.ID].CustomerName != "Ivan Petrov" {
			t.Fatalf("sent enrichment wrong: %+v", byID[sent.ID])
		}
		if board[0].Offer.Items != nil {
			t.Fatalf("board should not load items")
		}

		drafts, total, err := offers2.ListAll(ctx2, tenant2.ID, OfferBoardParams{Status: "draft", Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list drafts: %v", err)
		}
		if total != 1 || len(drafts) != 1 || drafts[0].Offer.ID != draft.ID {
			t.Fatalf("draft filter wrong: total=%d len=%d", total, len(drafts))
		}

		page1, total, err := offers2.ListAll(ctx2, tenant2.ID, OfferBoardParams{Limit: 1, Offset: 0})
		if err != nil {
			t.Fatalf("page 1: %v", err)
		}
		if total != 2 || len(page1) != 1 || page1[0].Offer.ID != sent.ID {
			t.Fatalf("page 1 wrong: total=%d len=%d", total, len(page1))
		}

		_, _, offers3, tenant3, _ := offerFixture(t)
		foreign, total, err := offers3.ListAll(ctx2, tenant3.ID, OfferBoardParams{Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("foreign board: %v", err)
		}
		if total != 0 || len(foreign) != 0 {
			t.Fatalf("tenant leak: got total=%d len=%d, want 0/0", total, len(foreign))
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
		if _, err := offers2.SetStatus(ctx2, other, mine.ID, domain.OfferStatusRejected); err != ErrNotFound {
			t.Fatalf("cross-tenant SetStatus: expected ErrNotFound, got %v", err)
		}
		if _, err := offers2.MarkSending(ctx2, other, mine.ID, "x@example.com", &fakeEnqueuer{}); err != ErrNotFound {
			t.Fatalf("cross-tenant MarkSending: expected ErrNotFound, got %v", err)
		}
	})

	t.Run("MarkSending sends a draft, enqueues once, then blocks a second send", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create: %v", err)
		}
		enq := &fakeEnqueuer{}
		sent, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", enq)
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		if sent.Status != domain.OfferStatusSent || sent.SendStatus != domain.SendStatusPending {
			t.Fatalf("wrong state after send: status=%s send=%s", sent.Status, sent.SendStatus)
		}
		if sent.SentTo != "customer@example.com" {
			t.Fatalf("sent_to not recorded: %q", sent.SentTo)
		}
		if enq.calls != 1 {
			t.Fatalf("expected exactly one enqueue, got %d", enq.calls)
		}
		if _, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", enq); err != ErrOfferNotSendable {
			t.Fatalf("expected ErrOfferNotSendable on re-send, got %v", err)
		}
	})

	t.Run("MarkSending rolls back the state change when enqueue fails", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create: %v", err)
		}
		boom := errors.New("enqueue boom")
		if _, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", &fakeEnqueuer{failErr: boom}); !errors.Is(err, boom) {
			t.Fatalf("expected enqueue error, got %v", err)
		}
		got, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Status != domain.OfferStatusDraft || got.SendStatus != domain.SendStatusPending || got.SentTo != "" {
			t.Fatalf("state leaked past a failed enqueue: %+v", got)
		}
	})

	t.Run("MarkSent then a failed retry then MarkSent again", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", &fakeEnqueuer{}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if err := offers.MarkSent(ctx, tenant.ID, o.ID); err != nil {
			t.Fatalf("mark sent: %v", err)
		}
		got, err := offers.Get(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.SendStatus != domain.SendStatusSent || got.SentAt == nil {
			t.Fatalf("not marked sent: send=%s sentAt=%v", got.SendStatus, got.SentAt)
		}

		if _, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", &fakeEnqueuer{}); err != ErrOfferNotSendable {
			t.Fatalf("expected ErrOfferNotSendable for delivered offer, got %v", err)
		}
	})

	t.Run("MarkSendFailed enables a retry that re-enqueues", func(t *testing.T) {
		o := newOffer(tenant.ID, car.ID)
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := offers.MarkSending(ctx, tenant.ID, o.ID, "customer@example.com", &fakeEnqueuer{}); err != nil {
			t.Fatalf("send: %v", err)
		}
		if err := offers.MarkSendFailed(ctx, tenant.ID, o.ID); err != nil {
			t.Fatalf("mark failed: %v", err)
		}

		enq := &fakeEnqueuer{}
		retried, err := offers.MarkSending(ctx, tenant.ID, o.ID, "other@example.com", enq)
		if err != nil {
			t.Fatalf("retry: %v", err)
		}
		if retried.SendStatus != domain.SendStatusPending || retried.SentTo != "other@example.com" {
			t.Fatalf("retry did not reset send state: %+v", retried)
		}
		if enq.calls != 1 {
			t.Fatalf("retry should enqueue once, got %d", enq.calls)
		}
	})

	t.Run("MarkSent/MarkSendFailed on unknown id return ErrNotFound", func(t *testing.T) {
		if err := offers.MarkSent(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("MarkSent: expected ErrNotFound, got %v", err)
		}
		if err := offers.MarkSendFailed(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("MarkSendFailed: expected ErrNotFound, got %v", err)
		}
	})
}
