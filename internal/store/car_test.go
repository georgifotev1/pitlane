package store

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

func newCar(tenantID, customerID, plate string) *domain.Car {
	return &domain.Car{
		ID:         uuid.NewString(),
		TenantID:   tenantID,
		CustomerID: customerID,
		Plate:      plate,
	}
}

func TestCarStore(t *testing.T) {
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := NewDB(tdb.Pool)
	cars := NewCarStore(db)
	custs := NewCustomerStore(db)
	ts := NewTenantStore(db)
	ctx := context.Background()

	tenant := newTenant("Garage")
	if err := ts.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	owner := newCustomer(tenant.ID, "Ivan Petrov")
	if err := custs.Create(ctx, owner); err != nil {
		t.Fatalf("create customer: %v", err)
	}

	t.Run("Create populates timestamps and Get round-trips", func(t *testing.T) {
		c := newCar(tenant.ID, owner.ID, "CB1234AB")
		c.VIN = "WVWZZZ1JZXW000001"
		c.Make = "Volkswagen"
		c.Model = "Golf"
		c.Year = 2018
		c.Mileage = 120000

		if err := cars.Create(ctx, c); err != nil {
			t.Fatalf("create car: %v", err)
		}
		if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
			t.Fatalf("expected timestamps populated, got %+v", c)
		}

		got, err := cars.Get(ctx, tenant.ID, c.ID)
		if err != nil {
			t.Fatalf("get car: %v", err)
		}
		if got.Plate != c.Plate || got.Make != c.Make || got.Model != c.Model ||
			got.Year != c.Year || got.Mileage != c.Mileage || got.CustomerID != owner.ID {
			t.Fatalf("car mismatch: got %+v, want %+v", got, c)
		}
		if got.ArchivedAt != nil {
			t.Fatalf("new car should not be archived")
		}
	})

	t.Run("Get unknown id returns ErrNotFound", func(t *testing.T) {
		_, err := cars.Get(ctx, tenant.ID, uuid.NewString())
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Create rejects duplicate active plate", func(t *testing.T) {
		first := newCar(tenant.ID, owner.ID, "DUP9999")
		if err := cars.Create(ctx, first); err != nil {
			t.Fatalf("create first: %v", err)
		}
		second := newCar(tenant.ID, owner.ID, "DUP9999")
		if err := cars.Create(ctx, second); err != ErrDuplicatePlate {
			t.Fatalf("expected ErrDuplicatePlate, got %v", err)
		}

		if err := cars.Archive(ctx, tenant.ID, first.ID); err != nil {
			t.Fatalf("archive first: %v", err)
		}
		third := newCar(tenant.ID, owner.ID, "DUP9999")
		if err := cars.Create(ctx, third); err != nil {
			t.Fatalf("re-create after archive: %v", err)
		}
	})

	t.Run("Update writes fields and bumps updated_at", func(t *testing.T) {
		c := newCar(tenant.ID, owner.ID, "UPD0001")
		if err := cars.Create(ctx, c); err != nil {
			t.Fatalf("create: %v", err)
		}
		firstUpdated := c.UpdatedAt

		c.Make = "Toyota"
		c.Mileage = 5000
		if err := cars.Update(ctx, c); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, err := cars.Get(ctx, tenant.ID, c.ID)
		if err != nil {
			t.Fatalf("get after update: %v", err)
		}
		if got.Make != "Toyota" || got.Mileage != 5000 {
			t.Fatalf("update not persisted: %+v", got)
		}
		if !got.UpdatedAt.After(firstUpdated) {
			t.Fatalf("updated_at not bumped: first %v, now %v", firstUpdated, got.UpdatedAt)
		}
	})

	t.Run("Update unknown id returns ErrNotFound", func(t *testing.T) {
		ghost := newCar(tenant.ID, owner.ID, "GHOST01")
		if err := cars.Update(ctx, ghost); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update rejects duplicate active plate", func(t *testing.T) {
		a := newCar(tenant.ID, owner.ID, "AAA1111")
		b := newCar(tenant.ID, owner.ID, "BBB2222")
		if err := cars.Create(ctx, a); err != nil {
			t.Fatalf("create a: %v", err)
		}
		if err := cars.Create(ctx, b); err != nil {
			t.Fatalf("create b: %v", err)
		}
		b.Plate = "AAA1111"
		if err := cars.Update(ctx, b); err != ErrDuplicatePlate {
			t.Fatalf("expected ErrDuplicatePlate, got %v", err)
		}
	})

	t.Run("Archive hides from default list, second archive is ErrNotFound", func(t *testing.T) {
		c := newCar(tenant.ID, owner.ID, "ARCH001")
		if err := cars.Create(ctx, c); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := cars.Archive(ctx, tenant.ID, c.ID); err != nil {
			t.Fatalf("archive: %v", err)
		}
		got, err := cars.Get(ctx, tenant.ID, c.ID)
		if err != nil {
			t.Fatalf("get archived: %v", err)
		}
		if got.ArchivedAt == nil {
			t.Fatalf("archived_at should be set")
		}
		if err := cars.Archive(ctx, tenant.ID, c.ID); err != ErrNotFound {
			t.Fatalf("second archive: expected ErrNotFound, got %v", err)
		}
	})

	t.Run("List paginates, searches, and filters archived, scoped to customer", func(t *testing.T) {
		lt := newTenant("ListGarage")
		if err := ts.Create(ctx, lt); err != nil {
			t.Fatalf("create list tenant: %v", err)
		}
		cust := newCustomer(lt.ID, "List Customer")
		if err := custs.Create(ctx, cust); err != nil {
			t.Fatalf("create list customer: %v", err)
		}
		other := newCustomer(lt.ID, "Other Customer")
		if err := custs.Create(ctx, other); err != nil {
			t.Fatalf("create other customer: %v", err)
		}
		if err := cars.Create(ctx, newCar(lt.ID, other.ID, "OTHER01")); err != nil {
			t.Fatalf("create other car: %v", err)
		}

		plates := []string{"AAA0001", "BBB0002", "CCC0003", "DDD0004"}
		for _, p := range plates {
			car := newCar(lt.ID, cust.ID, p)
			car.Make = "Make " + p
			if err := cars.Create(ctx, car); err != nil {
				t.Fatalf("create %s: %v", p, err)
			}
		}

		page1, total, err := cars.List(ctx, lt.ID, cust.ID, CarListParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("list page 1: %v", err)
		}
		if total != 4 {
			t.Fatalf("total: got %d, want 4 (other customer's car must not count)", total)
		}
		if len(page1) != 2 || page1[0].Plate != "AAA0001" || page1[1].Plate != "BBB0002" {
			t.Fatalf("page 1 order wrong: %+v", plates2(page1))
		}
		page2, _, err := cars.List(ctx, lt.ID, cust.ID, CarListParams{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatalf("list page 2: %v", err)
		}
		if len(page2) != 2 || page2[0].Plate != "CCC0003" {
			t.Fatalf("page 2 wrong: %+v", plates2(page2))
		}

		found, total, err := cars.List(ctx, lt.ID, cust.ID, CarListParams{Search: "bbb0002", Limit: 10})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if total != 1 || len(found) != 1 || found[0].Plate != "BBB0002" {
			t.Fatalf("search wrong: total %d, %+v", total, plates2(found))
		}

		if err := cars.Archive(ctx, lt.ID, page1[0].ID); err != nil {
			t.Fatalf("archive: %v", err)
		}
		_, activeTotal, err := cars.List(ctx, lt.ID, cust.ID, CarListParams{Limit: 10})
		if err != nil {
			t.Fatalf("list active: %v", err)
		}
		if activeTotal != 3 {
			t.Fatalf("active total: got %d, want 3", activeTotal)
		}
		_, withArchivedTotal, err := cars.List(ctx, lt.ID, cust.ID, CarListParams{IncludeArchived: true, Limit: 10})
		if err != nil {
			t.Fatalf("list incl archived: %v", err)
		}
		if withArchivedTotal != 4 {
			t.Fatalf("incl-archived total: got %d, want 4", withArchivedTotal)
		}
	})

	t.Run("ListAll spans customers, enriches names, and filters", func(t *testing.T) {
		lt := newTenant("BoardGarage")
		if err := ts.Create(ctx, lt); err != nil {
			t.Fatalf("create board tenant: %v", err)
		}
		custA := newCustomer(lt.ID, "Ana Ivanova")
		if err := custs.Create(ctx, custA); err != nil {
			t.Fatalf("create customer A: %v", err)
		}
		custB := newCustomer(lt.ID, "Boris Dimitrov")
		if err := custs.Create(ctx, custB); err != nil {
			t.Fatalf("create customer B: %v", err)
		}
		carA := newCar(lt.ID, custA.ID, "AAA1111")
		carA.Make = "Volkswagen"
		if err := cars.Create(ctx, carA); err != nil {
			t.Fatalf("create car A: %v", err)
		}
		carB := newCar(lt.ID, custB.ID, "BBB2222")
		carB.Make = "Toyota"
		if err := cars.Create(ctx, carB); err != nil {
			t.Fatalf("create car B: %v", err)
		}

		board, total, err := cars.ListAll(ctx, lt.ID, CarListParams{Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list all: %v", err)
		}
		if total != 2 || len(board) != 2 {
			t.Fatalf("board: got total=%d len=%d, want 2/2", total, len(board))
		}
		if board[0].Car.Plate != "AAA1111" || board[0].CustomerName != "Ana Ivanova" {
			t.Fatalf("row 0 wrong: %+v", board[0])
		}
		if board[1].Car.Plate != "BBB2222" || board[1].CustomerName != "Boris Dimitrov" {
			t.Fatalf("row 1 wrong: %+v", board[1])
		}

		found, total, err := cars.ListAll(ctx, lt.ID, CarListParams{Search: "toyota", Limit: 10})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if total != 1 || len(found) != 1 || found[0].Car.ID != carB.ID {
			t.Fatalf("search wrong: total=%d len=%d", total, len(found))
		}

		if err := cars.Archive(ctx, lt.ID, carA.ID); err != nil {
			t.Fatalf("archive A: %v", err)
		}
		_, activeTotal, err := cars.ListAll(ctx, lt.ID, CarListParams{Limit: 10})
		if err != nil {
			t.Fatalf("list active: %v", err)
		}
		if activeTotal != 1 {
			t.Fatalf("active total: got %d, want 1", activeTotal)
		}
		withArchived, inclTotal, err := cars.ListAll(ctx, lt.ID, CarListParams{IncludeArchived: true, Limit: 10})
		if err != nil {
			t.Fatalf("list incl archived: %v", err)
		}
		if inclTotal != 2 || withArchived[0].Car.ArchivedAt == nil {
			t.Fatalf("incl-archived wrong: total=%d row0=%+v", inclTotal, withArchived[0].Car)
		}

		otherTenant := newTenant("OtherBoard")
		if err := ts.Create(ctx, otherTenant); err != nil {
			t.Fatalf("create other tenant: %v", err)
		}
		foreign, total, err := cars.ListAll(ctx, otherTenant.ID, CarListParams{Limit: 10})
		if err != nil {
			t.Fatalf("foreign board: %v", err)
		}
		if total != 0 || len(foreign) != 0 {
			t.Fatalf("tenant leak: got total=%d len=%d, want 0/0", total, len(foreign))
		}
	})

	t.Run("Suggestions returns distinct tenant values including archived cars", func(t *testing.T) {
		suggestionTenant := newTenant("SuggestionGarage")
		if err := ts.Create(ctx, suggestionTenant); err != nil {
			t.Fatalf("create suggestion tenant: %v", err)
		}
		suggestionCustomer := newCustomer(suggestionTenant.ID, "Suggestion Customer")
		if err := custs.Create(ctx, suggestionCustomer); err != nil {
			t.Fatalf("create suggestion customer: %v", err)
		}

		for i, details := range []struct {
			makeName string
			model    string
		}{
			{makeName: "Ford", model: "Focus"},
			{makeName: "Ford", model: "Puma"},
			{makeName: "Volkswagen", model: "Golf"},
			{makeName: "", model: ""},
		} {
			car := newCar(suggestionTenant.ID, suggestionCustomer.ID, fmt.Sprintf("SUG%04d", i))
			car.Make, car.Model = details.makeName, details.model
			if err := cars.Create(ctx, car); err != nil {
				t.Fatalf("create suggestion car: %v", err)
			}
			if details.model == "Golf" {
				if err := cars.Archive(ctx, suggestionTenant.ID, car.ID); err != nil {
					t.Fatalf("archive suggestion car: %v", err)
				}
			}
		}

		otherTenant := newTenant("ForeignSuggestionGarage")
		if err := ts.Create(ctx, otherTenant); err != nil {
			t.Fatalf("create foreign suggestion tenant: %v", err)
		}
		otherCustomer := newCustomer(otherTenant.ID, "Foreign Customer")
		if err := custs.Create(ctx, otherCustomer); err != nil {
			t.Fatalf("create foreign suggestion customer: %v", err)
		}
		foreignCar := newCar(otherTenant.ID, otherCustomer.ID, "FOREIGN")
		foreignCar.Make, foreignCar.Model = "Tesla", "Model 3"
		if err := cars.Create(ctx, foreignCar); err != nil {
			t.Fatalf("create foreign suggestion car: %v", err)
		}

		suggestions, err := cars.Suggestions(ctx, suggestionTenant.ID)
		if err != nil {
			t.Fatalf("suggestions: %v", err)
		}
		if want := []string{"Ford", "Volkswagen"}; !slices.Equal(suggestions.Makes, want) {
			t.Fatalf("makes: got %v, want %v", suggestions.Makes, want)
		}
		wantModels := []CarModelSuggestion{
			{Make: "Ford", Model: "Focus"},
			{Make: "Ford", Model: "Puma"},
			{Make: "Volkswagen", Model: "Golf"},
		}
		if !slices.Equal(suggestions.Models, wantModels) {
			t.Fatalf("models: got %v, want %v", suggestions.Models, wantModels)
		}
	})

	t.Run("cross-tenant access is invisible", func(t *testing.T) {
		otherTenant := newTenant("Other")
		if err := ts.Create(ctx, otherTenant); err != nil {
			t.Fatalf("create other tenant: %v", err)
		}
		mine := newCar(tenant.ID, owner.ID, "MINE001")
		if err := cars.Create(ctx, mine); err != nil {
			t.Fatalf("create mine: %v", err)
		}

		if _, err := cars.Get(ctx, otherTenant.ID, mine.ID); err != ErrNotFound {
			t.Fatalf("cross-tenant Get: expected ErrNotFound, got %v", err)
		}
		poison := *mine
		poison.TenantID = otherTenant.ID
		poison.Make = "Hacked"
		if err := cars.Update(ctx, &poison); err != ErrNotFound {
			t.Fatalf("cross-tenant Update: expected ErrNotFound, got %v", err)
		}
		if err := cars.Archive(ctx, otherTenant.ID, mine.ID); err != ErrNotFound {
			t.Fatalf("cross-tenant Archive: expected ErrNotFound, got %v", err)
		}
		got, err := cars.Get(ctx, tenant.ID, mine.ID)
		if err != nil {
			t.Fatalf("get mine: %v", err)
		}
		if got.Make != "" || got.ArchivedAt != nil {
			t.Fatalf("car was mutated cross-tenant: %+v", got)
		}
	})
}

func plates2(cs []*domain.Car) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Plate
	}
	return out
}
