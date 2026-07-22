package store

import (
	"context"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

// attachmentFixture creates a tenant, customer, and car so attachments have a
// valid same-tenant car to anchor to.
func attachmentFixture(t *testing.T) (context.Context, *DB, *AttachmentStore, *domain.Tenant, *domain.Car) {
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
	return ctx, db, NewAttachmentStore(db), tenant, car
}

func newCarAttachment(tenantID, carID, name string) *domain.Attachment {
	return &domain.Attachment{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		CarID:       &carID,
		Name:        name,
		StorageKey:  AttachmentStorageKey(tenantID, uuid.NewString(), name),
		SizeBytes:   1024,
		ContentType: "image/jpeg",
	}
}

func newRepairAttachment(tenantID, repairID, name string) *domain.Attachment {
	return &domain.Attachment{
		ID:          uuid.NewString(),
		TenantID:    tenantID,
		RepairID:    &repairID,
		Name:        name,
		StorageKey:  AttachmentStorageKey(tenantID, uuid.NewString(), name),
		SizeBytes:   2048,
		ContentType: "application/pdf",
	}
}

// TestAttachmentStoreListByRepair covers the repair-scoped list, which the
// car-scoped tests never exercise. Attachments carry a NULL car_id when tied to
// a repair, so this also proves the *string scan handles the null side.
func TestAttachmentStoreListByRepair(t *testing.T) {
	ctx, db, repairs, offers, tenant, _, car := repairFixture(t)
	atts := NewAttachmentStore(db)

	o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
	r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	a := newRepairAttachment(tenant.ID, r.ID, "invoice.pdf")
	if err := atts.Create(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
	}

	list, err := atts.ListByRepair(ctx, tenant.ID, r.ID)
	if err != nil {
		t.Fatalf("list by repair: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 repair attachment, got %d", len(list))
	}
	got := list[0]
	if got.RepairID == nil || *got.RepairID != r.ID {
		t.Fatalf("repair id mismatch: %+v", got)
	}
	if got.CarID != nil {
		t.Fatalf("car id should be nil for a repair attachment: %+v", got)
	}

	// A repair attachment must not surface in the car-scoped list.
	carList, err := atts.ListByCar(ctx, tenant.ID, car.ID)
	if err != nil {
		t.Fatalf("list by car: %v", err)
	}
	if len(carList) != 0 {
		t.Fatalf("car list should not include repair attachments, got %d", len(carList))
	}
}

func TestAttachmentStore(t *testing.T) {
	ctx, _, atts, tenant, car := attachmentFixture(t)

	t.Run("Create and ListByCar", func(t *testing.T) {
		a := newCarAttachment(tenant.ID, car.ID, "damage.jpg")
		if err := atts.Create(ctx, a); err != nil {
			t.Fatalf("create: %v", err)
		}
		if a.CreatedAt.IsZero() {
			t.Fatalf("created_at not populated")
		}

		list, err := atts.ListByCar(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(list))
		}
		if list[0].Name != "damage.jpg" || list[0].SizeBytes != 1024 {
			t.Fatalf("attachment mismatch: %+v", list[0])
		}
	})

	t.Run("Get returns attachment or ErrNotFound", func(t *testing.T) {
		a := newCarAttachment(tenant.ID, car.ID, "invoice.pdf")
		if err := atts.Create(ctx, a); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := atts.Get(ctx, tenant.ID, a.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.ID != a.ID || got.Name != a.Name {
			t.Fatalf("attachment mismatch: got %+v", got)
		}

		if _, err := atts.Get(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Delete removes attachment", func(t *testing.T) {
		a := newCarAttachment(tenant.ID, car.ID, "to-delete.png")
		if err := atts.Create(ctx, a); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := atts.Delete(ctx, tenant.ID, a.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := atts.Get(ctx, tenant.ID, a.ID); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}
		if err := atts.Delete(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound for unknown id, got %v", err)
		}
	})
}

func TestAttachmentStoreTenantIsolation(t *testing.T) {
	ctx, db, atts, tenantA, carA := attachmentFixture(t)

	a := newCarAttachment(tenantA.ID, carA.ID, "a.jpg")
	if err := atts.Create(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
	}

	ts := NewTenantStore(db)
	tenantB := newTenant("Garage B")
	if err := ts.Create(ctx, tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	list, err := atts.ListByCar(ctx, tenantB.ID, carA.ID)
	if err != nil {
		t.Fatalf("list cross-tenant: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("B should see none of A's attachments, got %d", len(list))
	}
	if _, err := atts.Get(ctx, tenantB.ID, a.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant Get: expected ErrNotFound, got %v", err)
	}
	if err := atts.Delete(ctx, tenantB.ID, a.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant Delete: expected ErrNotFound, got %v", err)
	}
}
