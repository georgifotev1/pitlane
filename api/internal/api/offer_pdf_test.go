package api

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/gfotev/pitlane/internal/api/dto"
)

// getOfferPDF fetches the PDF endpoint. query is appended verbatim (e.g.
// "?disposition=inline"); the body is buffered so headers and bytes can both
// be asserted.
func (tc *tenantClient) getOfferPDF(t *testing.T, id, query string) (*http.Response, []byte) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/offers/" + id + "/pdf" + query)
	if err != nil {
		t.Fatalf("get offer pdf: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, body
}

func TestOfferPDF(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Гараж", "Собственик", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Иван Петров", Email: "ivan@example.com"})
	_, car := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "CB1234AB", Make: "Volkswagen", Model: "Golf", Year: 2018})
	_, offer := tc.createOffer(t, car.ID, twoItemOffer())

	t.Run("golden flow: non-empty PDF with pdf headers, default attachment", func(t *testing.T) {
		res, body := tc.getOfferPDF(t, offer.ID, "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d, body %s", res.StatusCode, body)
		}
		if ct := res.Header.Get("Content-Type"); ct != "application/pdf" {
			t.Fatalf("content-type %q, want application/pdf", ct)
		}
		if !bytes.HasPrefix(body, []byte("%PDF-")) {
			t.Fatal("body is not a PDF")
		}
		if !bytes.Contains(body, []byte("%%EOF")) {
			t.Fatal("PDF not terminated (no trailer)")
		}
		// Content-Length must match the streamed bytes exactly.
		if cl := res.Header.Get("Content-Length"); cl != strconv.Itoa(len(body)) {
			t.Fatalf("content-length %q != body len %d", cl, len(body))
		}
		// Default disposition is a named attachment download.
		cd := res.Header.Get("Content-Disposition")
		if !strings.HasPrefix(cd, "attachment;") || !strings.Contains(cd, "oferta-"+offer.ID[:8]+".pdf") {
			t.Fatalf("content-disposition %q", cd)
		}
	})

	t.Run("disposition=inline serves inline and permits same-origin framing", func(t *testing.T) {
		res, body := tc.getOfferPDF(t, offer.ID, "?disposition=inline")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d", res.StatusCode)
		}
		if !bytes.HasPrefix(body, []byte("%PDF-")) {
			t.Fatal("inline body is not a PDF")
		}
		if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "inline;") {
			t.Fatalf("content-disposition %q, want inline", cd)
		}
		// The endpoint must relax the middleware's X-Frame-Options: DENY so the
		// SPA can preview it in a same-origin iframe.
		if xfo := res.Header.Get("X-Frame-Options"); xfo != "SAMEORIGIN" {
			t.Fatalf("x-frame-options %q, want SAMEORIGIN", xfo)
		}
	})

	t.Run("draft offer renders too (preview before send)", func(t *testing.T) {
		_, draft := tc.createOffer(t, car.ID, twoItemOffer())
		res, body := tc.getOfferPDF(t, draft.ID, "")
		if res.StatusCode != http.StatusOK || !bytes.HasPrefix(body, []byte("%PDF-")) {
			t.Fatalf("draft pdf: status %d", res.StatusCode)
		}
	})

	t.Run("unknown offer is 404", func(t *testing.T) {
		res, _ := tc.getOfferPDF(t, "00000000-0000-0000-0000-000000000000", "")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status %d, want 404", res.StatusCode)
		}
	})

	t.Run("requires auth", func(t *testing.T) {
		anon := api.newClient()
		res, err := anon.client.Get(api.server.URL + "/api/v1/offers/" + offer.ID + "/pdf")
		if err != nil {
			t.Fatalf("anon: %v", err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", res.StatusCode)
		}
	})
}

// TestOfferPDFTenantIsolation extends the isolation guarantee to the PDF route:
// a tenant must not render another tenant's offer.
func TestOfferPDFTenantIsolation(t *testing.T) {
	api := newTestAPI(t)
	tcA, _ := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, _ := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	_, custA := tcA.createCustomer(t, dto.CreateCustomerRequest{Name: "A customer"})
	_, carA := tcA.createCar(t, custA.ID, dto.CreateCarRequest{Plate: "AAA001"})
	_, offerA := tcA.createOffer(t, carA.ID, twoItemOffer())

	t.Run("B cannot render A's offer PDF", func(t *testing.T) {
		res, _ := tcB.getOfferPDF(t, offerA.ID, "")
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("cross-tenant pdf: status %d, want 404", res.StatusCode)
		}
	})
}
