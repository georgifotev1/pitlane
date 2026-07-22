package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gfotev/pitlane/internal/api/dto"
)

// createCar POSTs a car under a customer and returns the decoded response plus
// status code (nil car on non-201, with the body buffered for error inspection).
func (tc *tenantClient) createCar(t *testing.T, customerID string, req dto.CreateCarRequest) (*http.Response, *dto.CarResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/customers/"+customerID+"/cars",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create car: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusCreated {
		return res, nil
	}
	var env struct {
		Car dto.CarResponse `json:"car"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("create car decode: %v", err)
	}
	return res, &env.Car
}

func (tc *tenantClient) listCars(t *testing.T, customerID, query string) (*http.Response, *dto.CarListResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/customers/" + customerID + "/cars" + query)
	if err != nil {
		t.Fatalf("list cars: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var out dto.CarListResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	return res, &out
}

func TestCarCRUD(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Garage", "Owner", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Ivan Petrov"})

	t.Run("create then get round-trips, plate upper-cased", func(t *testing.T) {
		res, c := tc.createCar(t, customer.ID, dto.CreateCarRequest{
			Plate:   "  cb1234ab ",
			Make:    "Volkswagen",
			Model:   "Golf",
			Year:    2018,
			Mileage: 120000,
		})
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create: status %d", res.StatusCode)
		}
		if c.Plate != "CB1234AB" {
			t.Fatalf("plate not normalized: %q", c.Plate)
		}
		if c.CustomerID != customer.ID {
			t.Fatalf("car not linked to customer: %+v", c)
		}
		if c.ArchivedAt != nil {
			t.Fatalf("new car archived")
		}

		getRes, err := tc.client.Get(api.server.URL + "/api/v1/cars/" + c.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer getRes.Body.Close()
		if getRes.StatusCode != http.StatusOK {
			t.Fatalf("get: status %d", getRes.StatusCode)
		}
	})

	t.Run("missing plate is 422 with field code", func(t *testing.T) {
		res, _ := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: ""})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		var p problemDetail
		b, _ := io.ReadAll(res.Body)
		_ = json.Unmarshal(b, &p)
		if p.Errors["plate"] != "required" {
			t.Fatalf("expected plate=required, got %+v", p.Errors)
		}
	})

	t.Run("duplicate plate is 422 with duplicate code", func(t *testing.T) {
		if _, c := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "DUP111"}); c == nil {
			t.Fatalf("first create failed")
		}
		res, _ := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "dup111"}) // case-insensitive
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		var p problemDetail
		b, _ := io.ReadAll(res.Body)
		_ = json.Unmarshal(b, &p)
		if p.Errors["plate"] != "duplicate" {
			t.Fatalf("expected plate=duplicate, got %+v", p.Errors)
		}
	})

	t.Run("invalid year is 422", func(t *testing.T) {
		res, _ := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "YR0001", Year: 1800})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
	})

	t.Run("create under unknown customer is 404", func(t *testing.T) {
		res, _ := tc.createCar(t, "00000000-0000-0000-0000-000000000000", dto.CreateCarRequest{Plate: "NOPE01"})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", res.StatusCode)
		}
	})

	t.Run("list carries pagination metadata", func(t *testing.T) {
		res, list := tc.listCars(t, customer.ID, "?page=1&pageSize=10")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("list: status %d", res.StatusCode)
		}
		if list.Metadata.Page != 1 || list.Metadata.PageSize != 10 {
			t.Fatalf("metadata wrong: %+v", list.Metadata)
		}
		if list.Metadata.Total < 1 {
			t.Fatalf("expected at least one car, total=%d", list.Metadata.Total)
		}
	})

	t.Run("list under unknown customer is 404", func(t *testing.T) {
		res, _ := tc.listCars(t, "00000000-0000-0000-0000-000000000000", "")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", res.StatusCode)
		}
	})

	t.Run("update replaces fields", func(t *testing.T) {
		_, c := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "UPD001", Make: "Before"})
		body, _ := json.Marshal(dto.UpdateCarRequest{Plate: "UPD001", Make: "After", Mileage: 42})
		req, _ := http.NewRequest(http.MethodPut, api.server.URL+"/api/v1/cars/"+c.ID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := tc.client.Do(req)
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("update: status %d", res.StatusCode)
		}
		var env struct {
			Car dto.CarResponse `json:"car"`
		}
		_ = json.NewDecoder(res.Body).Decode(&env)
		if env.Car.Make != "After" || env.Car.Mileage != 42 {
			t.Fatalf("update not applied: %+v", env.Car)
		}
	})

	t.Run("archive hides from default list, 204 then 404", func(t *testing.T) {
		_, c := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "DOOM01"})
		res, err := tc.client.Post(api.server.URL+"/api/v1/cars/"+c.ID+"/archive", "application/json", nil)
		if err != nil {
			t.Fatalf("archive: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("archive: status %d, want 204", res.StatusCode)
		}
		res2, _ := tc.client.Post(api.server.URL+"/api/v1/cars/"+c.ID+"/archive", "application/json", nil)
		res2.Body.Close()
		if res2.StatusCode != http.StatusNotFound {
			t.Fatalf("re-archive: status %d, want 404", res2.StatusCode)
		}
	})

	t.Run("get unknown id is 404", func(t *testing.T) {
		res, err := tc.client.Get(api.server.URL + "/api/v1/cars/00000000-0000-0000-0000-000000000000")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", res.StatusCode)
		}
	})

	t.Run("list requires auth", func(t *testing.T) {
		anon := api.newClient()
		res, err := anon.client.Get(api.server.URL + "/api/v1/customers/" + customer.ID + "/cars")
		if err != nil {
			t.Fatalf("anon list: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want 401", res.StatusCode)
		}
	})
}

// TestCarTenantIsolation is the car arm of the mandatory two-tenant isolation
// test. Every new entity joins this test (house law).
func TestCarTenantIsolation(t *testing.T) {
	api := newTestAPI(t)
	tcA, _ := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, _ := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	_, custA := tcA.createCustomer(t, dto.CreateCustomerRequest{Name: "A's customer"})
	_, custB := tcB.createCustomer(t, dto.CreateCustomerRequest{Name: "B's customer"})
	_, carA := tcA.createCar(t, custA.ID, dto.CreateCarRequest{Plate: "AAA001"})
	_, _ = tcB.createCar(t, custB.ID, dto.CreateCarRequest{Plate: "BBB001"})

	t.Run("each tenant lists only its own customer's cars", func(t *testing.T) {
		_, listA := tcA.listCars(t, custA.ID, "")
		if listA.Metadata.Total != 1 || len(listA.Cars) != 1 || listA.Cars[0].ID != carA.ID {
			t.Fatalf("A should see only its own car, got %+v", listA)
		}
	})

	t.Run("B cannot list cars under A's customer", func(t *testing.T) {
		// A's customer is invisible to B, so the nested list 404s.
		res, _ := tcB.listCars(t, custA.ID, "")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant list: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot create a car under A's customer", func(t *testing.T) {
		res, _ := tcB.createCar(t, custA.ID, dto.CreateCarRequest{Plate: "HACK01"})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant create: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot GET A's car", func(t *testing.T) {
		res, err := tcB.client.Get(api.server.URL + "/api/v1/cars/" + carA.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant get: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot update A's car", func(t *testing.T) {
		body, _ := json.Marshal(dto.UpdateCarRequest{Plate: "HACK02"})
		req, _ := http.NewRequest(http.MethodPut, api.server.URL+"/api/v1/cars/"+carA.ID, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res, err := tcB.client.Do(req)
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant update: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot archive A's car", func(t *testing.T) {
		res, err := tcB.client.Post(api.server.URL+"/api/v1/cars/"+carA.ID+"/archive", "application/json", nil)
		if err != nil {
			t.Fatalf("archive: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant archive: status %d, want 404", res.StatusCode)
		}
	})
}
