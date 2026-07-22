package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gfotev/pitlane/internal/api/dto"
)

// acceptOffer POSTs /offers/{id}/accept and decodes the created repair (nil on
// non-201). It is the sole path that produces a repair.
func (tc *tenantClient) acceptOffer(t *testing.T, offerID string) (*http.Response, *dto.RepairResponse) {
	t.Helper()
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/offers/"+offerID+"/accept",
		"application/json", nil,
	)
	if err != nil {
		t.Fatalf("accept offer: %v", err)
	}
	return decodeRepair(t, res, http.StatusCreated)
}

func (tc *tenantClient) getRepair(t *testing.T, id string) (*http.Response, *dto.RepairResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/repairs/" + id)
	if err != nil {
		t.Fatalf("get repair: %v", err)
	}
	return decodeRepair(t, res, http.StatusOK)
}

func (tc *tenantClient) updateRepair(t *testing.T, id string, req dto.UpdateRepairRequest) (*http.Response, *dto.RepairResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPut, tc.api.server.URL+"/api/v1/repairs/"+id, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := tc.client.Do(httpReq)
	if err != nil {
		t.Fatalf("update repair: %v", err)
	}
	return decodeRepair(t, res, http.StatusOK)
}

func (tc *tenantClient) setRepairStatus(t *testing.T, id, status string) (*http.Response, *dto.RepairResponse) {
	t.Helper()
	body, _ := json.Marshal(dto.UpdateRepairStatusRequest{Status: status})
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/repairs/"+id+"/status",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("set repair status: %v", err)
	}
	return decodeRepair(t, res, http.StatusOK)
}

func (tc *tenantClient) completeRepair(t *testing.T, id string, mileage int) (*http.Response, *dto.RepairResponse) {
	t.Helper()
	body, _ := json.Marshal(dto.CompleteRepairRequest{Mileage: mileage})
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/repairs/"+id+"/complete",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("complete repair: %v", err)
	}
	return decodeRepair(t, res, http.StatusOK)
}

func (tc *tenantClient) listRepairs(t *testing.T, query string) (*http.Response, *dto.RepairListResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/repairs" + query)
	if err != nil {
		t.Fatalf("list repairs: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var out dto.RepairListResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("list repairs decode: %v", err)
	}
	return res, &out
}

// decodeRepair buffers the body (so callers can still read it on error) and
// decodes the {"repair": …} envelope only on the expected status.
func decodeRepair(t *testing.T, res *http.Response, want int) (*http.Response, *dto.RepairResponse) {
	t.Helper()
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != want {
		return res, nil
	}
	var env struct {
		Repair dto.RepairResponse `json:"repair"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("repair decode: %v (body %s)", err, raw)
	}
	return res, &env.Repair
}

// carMileage reads a car's current odometer via GET /cars/{id}.
func (tc *tenantClient) carMileage(t *testing.T, id string) int {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/cars/" + id)
	if err != nil {
		t.Fatalf("get car: %v", err)
	}
	defer res.Body.Close()
	var env struct {
		Car dto.CarResponse `json:"car"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("get car decode: %v", err)
	}
	return env.Car.Mileage
}

// sendAndAccept drives a fresh offer through create → send → accept and returns
// the resulting repair. The shop's happy path in one helper.
func (tc *tenantClient) sendAndAccept(t *testing.T, carID string) *dto.RepairResponse {
	t.Helper()
	_, o := tc.createOffer(t, carID, twoItemOffer())
	if res, _ := tc.sendOffer(t, o.ID, "customer@example.com"); res.StatusCode != http.StatusOK {
		t.Fatalf("send: %d", res.StatusCode)
	}
	res, repair := tc.acceptOffer(t, o.ID)
	if res.StatusCode != http.StatusCreated || repair == nil {
		t.Fatalf("accept: status %d", res.StatusCode)
	}
	return repair
}

func TestRepairConversionAndLifecycle(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Гараж", "Owner", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Иван", Email: "ivan@example.com"})
	_, car := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "CB1234AB"})

	t.Run("accept a draft offer is 409 (must be sent first)", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		res, _ := tc.acceptOffer(t, o.ID)
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("accept draft: got %d, want 409", res.StatusCode)
		}
	})

	t.Run("accept a sent offer converts it: 201 open repair, items copied, offer accepted", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		if res, _ := tc.sendOffer(t, o.ID, "ivan@example.com"); res.StatusCode != http.StatusOK {
			t.Fatalf("send: %d", res.StatusCode)
		}

		res, repair := tc.acceptOffer(t, o.ID)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("accept: status %d", res.StatusCode)
		}
		if repair.Status != "open" || repair.OfferID == nil || *repair.OfferID != o.ID {
			t.Fatalf("wrong repair after convert: %+v", repair)
		}
		if repair.CarID != car.ID || len(repair.Items) != 2 || repair.TotalCents != 17850 {
			t.Fatalf("repair not a faithful copy: %+v", repair)
		}
		// The offer is now accepted.
		_, off := tc.getOffer(t, o.ID)
		if off.Status != "accepted" {
			t.Fatalf("offer not accepted after convert: %s", off.Status)
		}
		// Re-accepting the now-accepted offer is a 409.
		if res2, _ := tc.acceptOffer(t, o.ID); res2.StatusCode != http.StatusConflict {
			t.Fatalf("re-accept: got %d, want 409", res2.StatusCode)
		}
	})

	t.Run("accept unknown offer is 404", func(t *testing.T) {
		res, _ := tc.acceptOffer(t, "00000000-0000-0000-0000-000000000000")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("accept unknown: got %d, want 404", res.StatusCode)
		}
	})

	t.Run("edit an open repair, then freeze on status change (copy-not-share)", func(t *testing.T) {
		repair := tc.sendAndAccept(t, car.ID)

		// Edit the repair's items while open.
		res, updated := tc.updateRepair(t, repair.ID, dto.UpdateRepairRequest{
			Notes:      "Added an extra hour",
			TaxRateBps: 1900,
			Items: []dto.OfferItemRequest{
				{Kind: "labor", Description: "Diagnosis", Quantity: 2, UnitPriceCents: 5000},
			},
		})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("update open repair: %d", res.StatusCode)
		}
		if len(updated.Items) != 1 || updated.SubtotalCents != 10000 || updated.TotalCents != 11900 {
			t.Fatalf("repair not updated: %+v", updated)
		}
		// The source offer is untouched by the repair edit.
		_, off := tc.getOffer(t, *repair.OfferID)
		if len(off.Items) != 2 || off.TotalCents != 17850 {
			t.Fatalf("offer mutated by repair edit: %+v", off)
		}

		// Start the work: now frozen. A further edit is 409.
		if res2, started := tc.setRepairStatus(t, repair.ID, "in_progress"); res2.StatusCode != http.StatusOK || started.Status != "in_progress" {
			t.Fatalf("start: %d", res2.StatusCode)
		}
		res3, _ := tc.updateRepair(t, repair.ID, dto.UpdateRepairRequest{TaxRateBps: 1900, Items: twoItemOffer().Items})
		if res3.StatusCode != http.StatusConflict {
			t.Fatalf("edit in_progress repair: got %d, want 409", res3.StatusCode)
		}
	})

	t.Run("completion is not reachable via the generic status endpoint", func(t *testing.T) {
		repair := tc.sendAndAccept(t, car.ID)
		res, _ := tc.setRepairStatus(t, repair.ID, "completed")
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("status→completed: got %d, want 409", res.StatusCode)
		}
	})

	t.Run("unknown repair status string is 422", func(t *testing.T) {
		repair := tc.sendAndAccept(t, car.ID)
		res, _ := tc.setRepairStatus(t, repair.ID, "banana")
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		if errs := problemErrors(t, res); errs["status"] != "invalid" {
			t.Fatalf("expected status=invalid, got %+v", errs)
		}
	})

	t.Run("complete records mileage and advances the car odometer", func(t *testing.T) {
		repair := tc.sendAndAccept(t, car.ID)

		res, done := tc.completeRepair(t, repair.ID, 145000)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("complete: %d", res.StatusCode)
		}
		if done.Status != "completed" || done.CompletedAt == nil || done.Mileage != 145000 {
			t.Fatalf("not completed correctly: %+v", done)
		}
		// The car odometer advanced.
		if m := tc.carMileage(t, car.ID); m != 145000 {
			t.Fatalf("car mileage not advanced: %d", m)
		}
		// Completing again is 409.
		if res2, _ := tc.completeRepair(t, repair.ID, 150000); res2.StatusCode != http.StatusConflict {
			t.Fatalf("re-complete: got %d, want 409", res2.StatusCode)
		}
	})

	t.Run("complete with negative mileage is 422", func(t *testing.T) {
		repair := tc.sendAndAccept(t, car.ID)
		res, _ := tc.completeRepair(t, repair.ID, -5)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("negative mileage: got %d, want 422", res.StatusCode)
		}
	})

	t.Run("board lists tenant repairs with status filter and enrichment", func(t *testing.T) {
		res, list := tc.listRepairs(t, "?status=open&page=1&pageSize=50")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("list: %d", res.StatusCode)
		}
		if list.Metadata.Total < 1 {
			t.Fatalf("expected some open repairs, got %+v", list.Metadata)
		}
		for _, row := range list.Repairs {
			if row.Status != "open" {
				t.Fatalf("status filter leaked a %s repair", row.Status)
			}
			if row.CarPlate == "" || row.CustomerName == "" {
				t.Fatalf("board row not enriched: %+v", row)
			}
		}
	})

	t.Run("unknown status filter is 422", func(t *testing.T) {
		res, _ := tc.listRepairs(t, "?status=banana")
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("bad filter: got %d, want 422", res.StatusCode)
		}
	})

	t.Run("get unknown repair is 404", func(t *testing.T) {
		res, _ := tc.getRepair(t, "00000000-0000-0000-0000-000000000000")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("get unknown: got %d, want 404", res.StatusCode)
		}
	})

	t.Run("board requires auth", func(t *testing.T) {
		anon := api.newClient()
		res, err := anon.client.Get(api.server.URL + "/api/v1/repairs")
		if err != nil {
			t.Fatalf("anon list: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want 401", res.StatusCode)
		}
	})
}

// TestRepairTenantIsolationAPI is the repair arm of the mandatory two-tenant
// isolation test (house law: every new entity joins this test).
func TestRepairTenantIsolationAPI(t *testing.T) {
	api := newTestAPI(t)
	tcA, _ := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, _ := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	_, custA := tcA.createCustomer(t, dto.CreateCustomerRequest{Name: "A's customer"})
	_, carA := tcA.createCar(t, custA.ID, dto.CreateCarRequest{Plate: "AAA001"})
	repairA := tcA.sendAndAccept(t, carA.ID)

	// B has its own world so its board is independent.
	_, custB := tcB.createCustomer(t, dto.CreateCustomerRequest{Name: "B's customer"})
	_, carB := tcB.createCar(t, custB.ID, dto.CreateCarRequest{Plate: "BBB001"})
	_ = tcB.sendAndAccept(t, carB.ID)

	t.Run("each tenant's board shows only its own repairs", func(t *testing.T) {
		_, listA := tcA.listRepairs(t, "")
		if listA.Metadata.Total != 1 || len(listA.Repairs) != 1 || listA.Repairs[0].ID != repairA.ID {
			t.Fatalf("A should see only its own repair, got %+v", listA)
		}
	})

	t.Run("B cannot GET A's repair", func(t *testing.T) {
		if res, _ := tcB.getRepair(t, repairA.ID); res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant get: %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot update A's repair", func(t *testing.T) {
		res, _ := tcB.updateRepair(t, repairA.ID, dto.UpdateRepairRequest{TaxRateBps: 1900, Items: twoItemOffer().Items})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant update: %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot change A's repair status", func(t *testing.T) {
		if res, _ := tcB.setRepairStatus(t, repairA.ID, "in_progress"); res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant status: %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot complete A's repair", func(t *testing.T) {
		if res, _ := tcB.completeRepair(t, repairA.ID, 1); res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant complete: %d, want 404", res.StatusCode)
		}
	})

	t.Run("A's repair survives intact", func(t *testing.T) {
		_, got := tcA.getRepair(t, repairA.ID)
		if got.Status != "open" || got.TotalCents != 17850 {
			t.Fatalf("A's repair was altered: %+v", got)
		}
	})
}
