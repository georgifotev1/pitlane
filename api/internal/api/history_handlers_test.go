package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gfotev/pitlane/internal/api/dto"
)

func (tc *tenantClient) createHistoryNote(t *testing.T, carID string, req dto.CreateHistoryNoteRequest) (*http.Response, *dto.HistoryNoteResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	res, err := tc.client.Post(
		tc.api.server.URL+"/api/v1/cars/"+carID+"/history/notes",
		"application/json", bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("create history note: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusCreated {
		return res, nil
	}
	var env struct {
		Note dto.HistoryNoteResponse `json:"note"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode history note: %v", err)
	}
	return res, &env.Note
}

func (tc *tenantClient) getCarHistory(t *testing.T, carID string) (*http.Response, *dto.HistoryResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/cars/" + carID + "/history")
	if err != nil {
		t.Fatalf("get car history: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var out dto.HistoryResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	return res, &out
}

func TestHistoryHandlers(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Garage", "Owner", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Ivan Petrov"})
	_, car := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "CB1234AB", Make: "VW", Model: "Golf"})

	t.Run("create and list history note", func(t *testing.T) {
		res, note := tc.createHistoryNote(t, car.ID, dto.CreateHistoryNoteRequest{
			Title:       "Dealer service",
			Description: "Oil change at dealer",
			RecordedAt:  time.Now().UTC().Format(time.RFC3339),
		})
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("create note: status %d", res.StatusCode)
		}
		if note.Title != "Dealer service" {
			t.Fatalf("note mismatch: %+v", note)
		}

		res2, history := tc.getCarHistory(t, car.ID)
		if res2.StatusCode != http.StatusOK {
			t.Fatalf("get history: status %d", res2.StatusCode)
		}
		if len(history.History) != 1 {
			t.Fatalf("expected 1 history entry, got %d", len(history.History))
		}
		if history.History[0].Type != "note" {
			t.Fatalf("expected note entry, got %s", history.History[0].Type)
		}
	})

	t.Run("empty title is 422", func(t *testing.T) {
		res, _ := tc.createHistoryNote(t, car.ID, dto.CreateHistoryNoteRequest{Title: "", Description: "x"})
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want 422", res.StatusCode)
		}
	})

	t.Run("missing car is 404", func(t *testing.T) {
		res, _ := tc.createHistoryNote(t, "00000000-0000-0000-0000-000000000000", dto.CreateHistoryNoteRequest{
			Title: "x", Description: "y",
		})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status: got %d, want 404", res.StatusCode)
		}
	})
}

func TestHistoryHandlerTenantIsolation(t *testing.T) {
	api := newTestAPI(t)
	tcA, _ := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, _ := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	_, custA := tcA.createCustomer(t, dto.CreateCustomerRequest{Name: "A"})
	_, carA := tcA.createCar(t, custA.ID, dto.CreateCarRequest{Plate: "AA1234AA", Make: "VW", Model: "Golf"})

	_, note := tcA.createHistoryNote(t, carA.ID, dto.CreateHistoryNoteRequest{
		Title: "A note", Description: "A desc",
	})

	res, _ := tcB.getCarHistory(t, carA.ID)
	if res.StatusCode != http.StatusNotFound {
		// B should get 404 because the car itself is not visible to B.
		t.Fatalf("B history on A car: expected 404, got %d", res.StatusCode)
	}

	body, _ := json.Marshal(dto.UpdateHistoryNoteRequest{Title: "hacked", Description: "x"})
	req, _ := http.NewRequest("PUT", api.server.URL+"/api/v1/history/notes/"+note.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res2, _ := tcB.client.Do(req)
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("B update A note: expected 404, got %d", res2.StatusCode)
	}
	res2.Body.Close()

	req2, _ := http.NewRequest("DELETE", api.server.URL+"/api/v1/history/notes/"+note.ID, nil)
	res3, _ := tcB.client.Do(req2)
	if res3.StatusCode != http.StatusNotFound {
		t.Fatalf("B delete A note: expected 404, got %d", res3.StatusCode)
	}
	res3.Body.Close()
}
