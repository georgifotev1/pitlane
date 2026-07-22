package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gfotev/pitlane/internal/api/dto"
)

// createCustomer POSTs a customer for this tenant and returns the decoded
// response plus status code.
func (tc *tenantClient) createCustomer(t *testing.T, req dto.CreateCustomerRequest) (*http.Response, *dto.CustomerResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	res, err := tc.client.Post(tc.api.server.URL+"/api/v1/customers", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	// Buffer the body so callers can still read it after this helper returns
	// (the raw problem+json on error paths, in particular).
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusCreated {
		return res, nil
	}
	var env struct {
		Customer dto.CustomerResponse `json:"customer"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("create customer decode: %v", err)
	}
	return res, &env.Customer
}

func (tc *tenantClient) listCustomers(t *testing.T, query string) (*http.Response, *dto.CustomerListResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/customers" + query)
	if err != nil {
		t.Fatalf("list customers: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var out dto.CustomerListResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("list decode: %v", err)
	}
	return res, &out
}

func TestCustomerCRUD(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Garage", "Owner", "owner@example.com", "password-123")

	t.Run("create then get round-trips", func(t *testing.T) {
		res, c := tc.createCustomer(t, dto.CreateCustomerRequest{
			Name:  "  Ivan Petrov  ",
			Email: "ivan@example.com",
			Phone: "+359888",
		})
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create: status %d", res.StatusCode)
		}
		if c.Name != "Ivan Petrov" {
			t.Fatalf("name not trimmed: %q", c.Name)
		}
		if c.ArchivedAt != nil {
			t.Fatalf("new customer archived")
		}

		getRes, err := tc.client.Get(api.server.URL + "/api/v1/customers/" + c.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer getRes.Body.Close()
		if getRes.StatusCode != http.StatusOK {
			t.Fatalf("get: status %d", getRes.StatusCode)
		}
	})

	t.Run("missing name is 422 with field code", func(t *testing.T) {
		res, _ := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "", Email: "x@y.com"})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		var p problemDetail
		b, _ := io.ReadAll(res.Body)
		_ = json.Unmarshal(b, &p)
		if p.Errors["name"] != "required" {
			t.Fatalf("expected name=required, got %+v", p.Errors)
		}
	})

	t.Run("invalid email is 422", func(t *testing.T) {
		res, _ := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "A", Email: "not-an-email"})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
	})

	t.Run("list carries pagination metadata", func(t *testing.T) {
		res, list := tc.listCustomers(t, "?page=1&pageSize=10")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("list: status %d", res.StatusCode)
		}
		if list.Metadata.Page != 1 || list.Metadata.PageSize != 10 {
			t.Fatalf("metadata wrong: %+v", list.Metadata)
		}
		if list.Metadata.Total < 1 {
			t.Fatalf("expected at least one customer, total=%d", list.Metadata.Total)
		}
	})

	t.Run("update replaces fields", func(t *testing.T) {
		_, c := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Before"})
		body, _ := json.Marshal(dto.UpdateCustomerRequest{Name: "After", Phone: "+359111"})
		req, _ := http.NewRequest(http.MethodPut, api.server.URL+"/api/v1/customers/"+c.ID, bytes.NewReader(body))
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
			Customer dto.CustomerResponse `json:"customer"`
		}
		_ = json.NewDecoder(res.Body).Decode(&env)
		if env.Customer.Name != "After" || env.Customer.Phone != "+359111" {
			t.Fatalf("update not applied: %+v", env.Customer)
		}
	})

	t.Run("archive hides from default list, 204 then 404", func(t *testing.T) {
		_, c := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Doomed"})
		res, err := tc.client.Post(api.server.URL+"/api/v1/customers/"+c.ID+"/archive", "application/json", nil)
		if err != nil {
			t.Fatalf("archive: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("archive: status %d, want 204", res.StatusCode)
		}
		// Second archive → 404.
		res2, _ := tc.client.Post(api.server.URL+"/api/v1/customers/"+c.ID+"/archive", "application/json", nil)
		res2.Body.Close()
		if res2.StatusCode != http.StatusNotFound {
			t.Fatalf("re-archive: status %d, want 404", res2.StatusCode)
		}
	})

	t.Run("get unknown id is 404", func(t *testing.T) {
		res, err := tc.client.Get(api.server.URL + "/api/v1/customers/00000000-0000-0000-0000-000000000000")
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
		res, err := anon.client.Get(api.server.URL + "/api/v1/customers")
		if err != nil {
			t.Fatalf("anon list: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want 401", res.StatusCode)
		}
	})
}

// TestCustomerTenantIsolation is the customer arm of the mandatory two-tenant
// isolation test. Every new entity joins this test (house law).
func TestCustomerTenantIsolation(t *testing.T) {
	api := newTestAPI(t)
	tcA, _ := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, _ := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	_, custA := tcA.createCustomer(t, dto.CreateCustomerRequest{Name: "A's customer"})
	_, custB := tcB.createCustomer(t, dto.CreateCustomerRequest{Name: "B's customer"})

	t.Run("each tenant lists only its own", func(t *testing.T) {
		_, listA := tcA.listCustomers(t, "")
		if listA.Metadata.Total != 1 || len(listA.Customers) != 1 || listA.Customers[0].ID != custA.ID {
			t.Fatalf("A should see only its own customer, got %+v", listA)
		}
		_, listB := tcB.listCustomers(t, "")
		if listB.Metadata.Total != 1 || listB.Customers[0].ID != custB.ID {
			t.Fatalf("B should see only its own customer, got %+v", listB)
		}
	})

	t.Run("B cannot GET A's customer", func(t *testing.T) {
		res, err := tcB.client.Get(api.server.URL + "/api/v1/customers/" + custA.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant get: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot update A's customer", func(t *testing.T) {
		body, _ := json.Marshal(dto.UpdateCustomerRequest{Name: "Hacked"})
		req, _ := http.NewRequest(http.MethodPut, api.server.URL+"/api/v1/customers/"+custA.ID, bytes.NewReader(body))
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

	t.Run("B cannot archive A's customer", func(t *testing.T) {
		res, err := tcB.client.Post(api.server.URL+"/api/v1/customers/"+custA.ID+"/archive", "application/json", nil)
		if err != nil {
			t.Fatalf("archive: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant archive: status %d, want 404", res.StatusCode)
		}
	})
}
