package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/gfotev/pitlane/internal/api/dto"
)

// createOffer POSTs an offer under a car and returns the response plus decoded
// offer (nil on non-201, body buffered for inspection).
func (tc *tenantClient) createOffer(t *testing.T, carID string, req dto.CreateOfferRequest) (*http.Response, *dto.OfferResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/cars/"+carID+"/offers",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusCreated {
		return res, nil
	}
	var env struct {
		Offer dto.OfferResponse `json:"offer"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("create offer decode: %v", err)
	}
	return res, &env.Offer
}

func (tc *tenantClient) getOffer(t *testing.T, id string) (*http.Response, *dto.OfferResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/offers/" + id)
	if err != nil {
		t.Fatalf("get offer: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var env struct {
		Offer dto.OfferResponse `json:"offer"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("get offer decode: %v", err)
	}
	return res, &env.Offer
}

func (tc *tenantClient) updateOffer(t *testing.T, id string, req dto.UpdateOfferRequest) (*http.Response, *dto.OfferResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPut, tc.api.server.URL+"/api/v1/offers/"+id, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := tc.client.Do(httpReq)
	if err != nil {
		t.Fatalf("update offer: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var env struct {
		Offer dto.OfferResponse `json:"offer"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("update offer decode: %v", err)
	}
	return res, &env.Offer
}

func (tc *tenantClient) setOfferStatus(t *testing.T, id, status string) (*http.Response, *dto.OfferResponse) {
	t.Helper()
	body, _ := json.Marshal(dto.UpdateOfferStatusRequest{Status: status})
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/offers/"+id+"/status",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("set status: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var env struct {
		Offer dto.OfferResponse `json:"offer"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("set status decode: %v", err)
	}
	return res, &env.Offer
}

func (tc *tenantClient) sendOffer(t *testing.T, id, recipient string) (*http.Response, *dto.OfferResponse) {
	t.Helper()
	body, _ := json.Marshal(dto.SendOfferRequest{Recipient: recipient})
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/offers/"+id+"/send",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("send offer: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var env struct {
		Offer dto.OfferResponse `json:"offer"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("send offer decode: %v", err)
	}
	return res, &env.Offer
}

// countSendJobs reads the River queue directly (the test pool connects as the
// app role, which has SELECT on river_job) to prove the send endpoint enqueued
// a job. The job is never worked here — that is internal/jobs' remit.
func countSendJobs(t *testing.T, api *testAPI) int {
	t.Helper()
	var n int
	if err := api.tdb.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = $1`, "send_offer_email").Scan(&n); err != nil {
		t.Fatalf("count send jobs: %v", err)
	}
	return n
}

func (tc *tenantClient) listOffers(t *testing.T, carID, query string) (*http.Response, *dto.OfferListResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/cars/" + carID + "/offers" + query)
	if err != nil {
		t.Fatalf("list offers: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var out dto.OfferListResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("list offers decode: %v", err)
	}
	return res, &out
}

// twoItemOffer is a fixture body: 90.00 + 60.00 = 150.00 subtotal.
func twoItemOffer() dto.CreateOfferRequest {
	return dto.CreateOfferRequest{
		Notes: "Estimate valid 30 days",
		Items: []dto.OfferItemRequest{
			{Kind: "part", Description: "Brake pads", Quantity: 2, UnitPriceCents: 4500},
			{Kind: "labor", Description: "Fitting", Quantity: 1, UnitPriceCents: 6000},
		},
	}
}

func problemErrors(t *testing.T, res *http.Response) map[string]string {
	t.Helper()
	var p problemDetail
	b, _ := io.ReadAll(res.Body)
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatalf("decode problem: %v (body %s)", err, b)
	}
	return p.Errors
}

func TestOfferCRUD(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Garage", "Owner", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Ivan Petrov"})
	_, car := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "CB1234AB"})

	t.Run("create computes exact totals, defaults to draft, snapshots tenant tax", func(t *testing.T) {
		res, o := tc.createOffer(t, car.ID, twoItemOffer())
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create: status %d", res.StatusCode)
		}
		if o.Status != "draft" || o.SendStatus != "pending" {
			t.Fatalf("wrong initial lifecycle: %+v", o)
		}
		// Tenant default is 1900 bps (19%). subtotal 150.00, tax 28.50, total 178.50.
		if o.TaxRateBps != 1900 {
			t.Fatalf("tax rate not snapshotted from tenant: %d", o.TaxRateBps)
		}
		if o.SubtotalCents != 15000 || o.TaxCents != 2850 || o.TotalCents != 17850 {
			t.Fatalf("totals wrong: sub=%d tax=%d total=%d", o.SubtotalCents, o.TaxCents, o.TotalCents)
		}
		if len(o.Items) != 2 || o.Items[0].LineTotalCents != 9000 || o.Items[1].LineTotalCents != 6000 {
			t.Fatalf("item line totals wrong: %+v", o.Items)
		}

		// Get round-trips.
		getRes, got := tc.getOffer(t, o.ID)
		if getRes.StatusCode != http.StatusOK {
			t.Fatalf("get: status %d", getRes.StatusCode)
		}
		if got.CarID != car.ID || got.Notes != "Estimate valid 30 days" || len(got.Items) != 2 {
			t.Fatalf("get mismatch: %+v", got)
		}
	})

	t.Run("empty items is 422 items=required", func(t *testing.T) {
		res, _ := tc.createOffer(t, car.ID, dto.CreateOfferRequest{Items: nil})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		if errs := problemErrors(t, res); errs["items"] != "required" {
			t.Fatalf("expected items=required, got %+v", errs)
		}
	})

	t.Run("bad line fields are 422 with per-line keys", func(t *testing.T) {
		res, _ := tc.createOffer(t, car.ID, dto.CreateOfferRequest{
			Items: []dto.OfferItemRequest{
				{Kind: "banana", Description: "", Quantity: 0, UnitPriceCents: -5},
			},
		})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		errs := problemErrors(t, res)
		for _, key := range []string{"items.0.kind", "items.0.description", "items.0.quantity", "items.0.unitPriceCents"} {
			if errs[key] == "" {
				t.Fatalf("expected error on %s, got %+v", key, errs)
			}
		}
	})

	t.Run("update replaces items and recomputes while draft", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		res, updated := tc.updateOffer(t, o.ID, dto.UpdateOfferRequest{
			Notes:      "revised",
			TaxRateBps: 2000,
			Items: []dto.OfferItemRequest{
				{Kind: "other", Description: "Diagnostics", Quantity: 1, UnitPriceCents: 3000},
			},
		})
		if res.StatusCode != http.StatusOK {
			t.Fatalf("update: status %d", res.StatusCode)
		}
		// subtotal 30.00, tax 20% = 6.00, total 36.00.
		if updated.SubtotalCents != 3000 || updated.TaxCents != 600 || updated.TotalCents != 3600 {
			t.Fatalf("recompute wrong: %+v", updated)
		}
		if len(updated.Items) != 1 || updated.Items[0].Description != "Diagnostics" || updated.Notes != "revised" {
			t.Fatalf("update not applied: %+v", updated)
		}
	})

	t.Run("send freezes the offer, then post-send edit is 409, offer untouched", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())

		sres, sent := tc.sendOffer(t, o.ID, "customer@example.com")
		if sres.StatusCode != http.StatusOK {
			t.Fatalf("send: status %d", sres.StatusCode)
		}
		if sent.Status != "sent" {
			t.Fatalf("status not sent: %s", sent.Status)
		}

		res, _ := tc.updateOffer(t, o.ID, dto.UpdateOfferRequest{
			Notes:      "sneaky post-send edit",
			TaxRateBps: 1900,
			Items:      twoItemOffer().Items,
		})
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("post-send edit: got %d, want 409", res.StatusCode)
		}

		_, got := tc.getOffer(t, o.ID)
		if got.Notes == "sneaky post-send edit" {
			t.Fatalf("sent offer was mutated")
		}
	})

	t.Run("invalid transition is 409", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		// draft → rejected is not allowed (must send first).
		res, _ := tc.setOfferStatus(t, o.ID, "rejected")
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("status: got %d, want 409", res.StatusCode)
		}
	})

	t.Run("accepted is not reachable via the generic status endpoint", func(t *testing.T) {
		// Accepting an offer converts it to a repair (POST /offers/{id}/accept),
		// so the generic machine must refuse 'accepted' even from sent → 409.
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		if res, _ := tc.sendOffer(t, o.ID, "customer@example.com"); res.StatusCode != http.StatusOK {
			t.Fatalf("send: %d", res.StatusCode)
		}
		res, _ := tc.setOfferStatus(t, o.ID, "accepted")
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("sent→accepted via status: got %d, want 409", res.StatusCode)
		}
	})

	t.Run("unknown status string is 422", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		res, _ := tc.setOfferStatus(t, o.ID, "banana")
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		if errs := problemErrors(t, res); errs["status"] != "invalid" {
			t.Fatalf("expected status=invalid, got %+v", errs)
		}
	})

	t.Run("full lifecycle: send→reject", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		if res, _ := tc.sendOffer(t, o.ID, "customer@example.com"); res.StatusCode != http.StatusOK {
			t.Fatalf("send: %d", res.StatusCode)
		}
		res, rejected := tc.setOfferStatus(t, o.ID, "rejected")
		if res.StatusCode != http.StatusOK || rejected.Status != "rejected" {
			t.Fatalf("reject failed: %d %+v", res.StatusCode, rejected)
		}
	})

	t.Run("create under unknown car is 404", func(t *testing.T) {
		res, _ := tc.createOffer(t, "00000000-0000-0000-0000-000000000000", twoItemOffer())
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", res.StatusCode)
		}
	})

	t.Run("get unknown id is 404", func(t *testing.T) {
		res, _ := tc.getOffer(t, "00000000-0000-0000-0000-000000000000")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", res.StatusCode)
		}
	})

	t.Run("list carries pagination metadata, scoped to the car", func(t *testing.T) {
		res, list := tc.listOffers(t, car.ID, "?page=1&pageSize=10")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("list: status %d", res.StatusCode)
		}
		if list.Metadata.Page != 1 || list.Metadata.PageSize != 10 || list.Metadata.Total < 1 {
			t.Fatalf("metadata wrong: %+v", list.Metadata)
		}
	})

	t.Run("list requires auth", func(t *testing.T) {
		anon := api.newClient()
		res, err := anon.client.Get(api.server.URL + "/api/v1/cars/" + car.ID + "/offers")
		if err != nil {
			t.Fatalf("anon list: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want 401", res.StatusCode)
		}
	})
}

func TestOfferSend(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Гараж", "Owner", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Иван", Email: "ivan@example.com"})
	_, car := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "CB1234AB"})

	t.Run("send a draft: 200 sent+pending+sentTo, exactly one job enqueued", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		before := countSendJobs(t, api)

		res, sent := tc.sendOffer(t, o.ID, "ivan@example.com")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("send: status %d", res.StatusCode)
		}
		if sent.Status != "sent" || sent.SendStatus != "pending" || sent.SentTo != "ivan@example.com" {
			t.Fatalf("wrong state after send: %+v", sent)
		}
		// Transactional enqueue: the state change committed with a job row.
		if got := countSendJobs(t, api); got != before+1 {
			t.Fatalf("expected one new send job, count went %d → %d", before, got)
		}
	})

	t.Run("invalid recipient is 422 recipient=invalid_email, no job", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		before := countSendJobs(t, api)
		res, _ := tc.sendOffer(t, o.ID, "not-an-email")
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
		if errs := problemErrors(t, res); errs["recipient"] != "invalid_email" {
			t.Fatalf("expected recipient=invalid_email, got %+v", errs)
		}
		if got := countSendJobs(t, api); got != before {
			t.Fatalf("a rejected send must not enqueue: %d → %d", before, got)
		}
	})

	t.Run("re-sending a pending offer is 409", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		if res, _ := tc.sendOffer(t, o.ID, "ivan@example.com"); res.StatusCode != http.StatusOK {
			t.Fatalf("first send: %d", res.StatusCode)
		}
		res, _ := tc.sendOffer(t, o.ID, "ivan@example.com")
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("re-send while pending: got %d, want 409", res.StatusCode)
		}
	})

	t.Run("post-send edit is 409 (freeze-on-send)", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		if res, _ := tc.sendOffer(t, o.ID, "ivan@example.com"); res.StatusCode != http.StatusOK {
			t.Fatalf("send: %d", res.StatusCode)
		}
		res, _ := tc.updateOffer(t, o.ID, dto.UpdateOfferRequest{TaxRateBps: 1900, Items: twoItemOffer().Items})
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("post-send edit: got %d, want 409", res.StatusCode)
		}
	})

	t.Run("the status endpoint can no longer send (draft→sent is 409)", func(t *testing.T) {
		_, o := tc.createOffer(t, car.ID, twoItemOffer())
		res, _ := tc.setOfferStatus(t, o.ID, "sent")
		if res.StatusCode != http.StatusConflict {
			t.Fatalf("status draft→sent: got %d, want 409", res.StatusCode)
		}
	})

	t.Run("send unknown offer is 404", func(t *testing.T) {
		res, _ := tc.sendOffer(t, "00000000-0000-0000-0000-000000000000", "ivan@example.com")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", res.StatusCode)
		}
	})
}

// TestOfferTenantIsolation is the offer arm of the mandatory two-tenant
// isolation test (house law: every new entity joins this test).
func TestOfferTenantIsolation(t *testing.T) {
	api := newTestAPI(t)
	tcA, _ := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, _ := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	_, custA := tcA.createCustomer(t, dto.CreateCustomerRequest{Name: "A's customer"})
	_, carA := tcA.createCar(t, custA.ID, dto.CreateCarRequest{Plate: "AAA001"})
	_, offerA := tcA.createOffer(t, carA.ID, twoItemOffer())

	_, custB := tcB.createCustomer(t, dto.CreateCustomerRequest{Name: "B's customer"})
	_, _ = tcB.createCar(t, custB.ID, dto.CreateCarRequest{Plate: "BBB001"})

	t.Run("each tenant lists only its own car's offers", func(t *testing.T) {
		_, listA := tcA.listOffers(t, carA.ID, "")
		if listA.Metadata.Total != 1 || len(listA.Offers) != 1 || listA.Offers[0].ID != offerA.ID {
			t.Fatalf("A should see only its own offer, got %+v", listA)
		}
	})

	t.Run("B cannot list offers under A's car", func(t *testing.T) {
		res, _ := tcB.listOffers(t, carA.ID, "")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant list: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot create an offer under A's car", func(t *testing.T) {
		res, _ := tcB.createOffer(t, carA.ID, twoItemOffer())
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant create: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot GET A's offer", func(t *testing.T) {
		res, _ := tcB.getOffer(t, offerA.ID)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant get: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot update A's offer", func(t *testing.T) {
		res, _ := tcB.updateOffer(t, offerA.ID, dto.UpdateOfferRequest{TaxRateBps: 1900, Items: twoItemOffer().Items})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant update: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot change A's offer status", func(t *testing.T) {
		res, _ := tcB.setOfferStatus(t, offerA.ID, "rejected")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant status: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot send A's offer", func(t *testing.T) {
		res, _ := tcB.sendOffer(t, offerA.ID, "b@example.com")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant send: status %d, want 404", res.StatusCode)
		}
	})

	t.Run("B cannot accept (convert) A's offer", func(t *testing.T) {
		res, _ := tcB.acceptOffer(t, offerA.ID)
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant accept: status %d, want 404", res.StatusCode)
		}
	})

	// A's offer must be untouched by all of B's attempts.
	t.Run("A's offer survives intact", func(t *testing.T) {
		_, got := tcA.getOffer(t, offerA.ID)
		if got.Status != "draft" || got.TotalCents != 17850 {
			t.Fatalf("A's offer was altered: %+v", got)
		}
	})
}
