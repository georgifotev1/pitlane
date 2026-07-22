package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/gfotev/pitlane/internal/api/dto"
)

func (tc *tenantClient) uploadCarAttachment(t *testing.T, carID, filename string, data []byte) (*http.Response, *dto.AttachmentResponse) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req, err := http.NewRequest("POST", tc.api.server.URL+"/api/v1/cars/"+carID+"/attachments", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := tc.client.Do(req)
	if err != nil {
		t.Fatalf("upload attachment: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusCreated {
		return res, nil
	}
	var env struct {
		Attachment dto.AttachmentResponse `json:"attachment"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode attachment: %v", err)
	}
	return res, &env.Attachment
}

func (tc *tenantClient) uploadRepairAttachment(t *testing.T, repairID, filename string, data []byte) (*http.Response, *dto.AttachmentResponse) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req, err := http.NewRequest("POST", tc.api.server.URL+"/api/v1/repairs/"+repairID+"/attachments", &body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	res, err := tc.client.Do(req)
	if err != nil {
		t.Fatalf("upload attachment: %v", err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	res.Body = io.NopCloser(bytes.NewReader(raw))
	if res.StatusCode != http.StatusCreated {
		return res, nil
	}
	var env struct {
		Attachment dto.AttachmentResponse `json:"attachment"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode attachment: %v", err)
	}
	return res, &env.Attachment
}

func (tc *tenantClient) listRepairAttachments(t *testing.T, repairID string) (*http.Response, *dto.AttachmentListResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/repairs/" + repairID + "/attachments")
	if err != nil {
		t.Fatalf("list repair attachments: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var out dto.AttachmentListResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode attachments: %v", err)
	}
	return res, &out
}

func (tc *tenantClient) listCarAttachments(t *testing.T, carID string) (*http.Response, *dto.AttachmentListResponse) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/cars/" + carID + "/attachments")
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return res, nil
	}
	var out dto.AttachmentListResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode attachments: %v", err)
	}
	return res, &out
}

func TestAttachmentHandlers(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Garage", "Owner", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Ivan Petrov"})
	_, car := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "CB1234AB", Make: "VW", Model: "Golf"})

	t.Run("upload and list car attachment", func(t *testing.T) {
		res, att := tc.uploadCarAttachment(t, car.ID, "damage.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0}) // JPEG magic
		if res.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("upload: status %d, body %s", res.StatusCode, body)
		}
		if att.Name != "damage.jpg" {
			t.Fatalf("attachment name mismatch: %+v", att)
		}
		if att.ContentType != "image/jpeg" {
			t.Fatalf("expected jpeg, got %q", att.ContentType)
		}

		res2, list := tc.listCarAttachments(t, car.ID)
		if res2.StatusCode != http.StatusOK {
			t.Fatalf("list: status %d", res2.StatusCode)
		}
		if len(list.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(list.Attachments))
		}

		// Download returns the file with Content-Disposition: attachment.
		res3, err := tc.client.Get(api.server.URL + "/api/v1/attachments/" + att.ID)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		defer res3.Body.Close()
		if res3.StatusCode != http.StatusOK {
			t.Fatalf("download: status %d", res3.StatusCode)
		}
		cd := res3.Header.Get("Content-Disposition")
		if cd == "" || cd[:10] != "attachment" {
			t.Fatalf("expected attachment disposition, got %q", cd)
		}
	})

	t.Run("oversized upload is rejected", func(t *testing.T) {
		big := make([]byte, 10*1024*1024+1)
		res, _ := tc.uploadCarAttachment(t, car.ID, "big.bin", big)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for oversized, got %d", res.StatusCode)
		}
	})

	t.Run("empty upload is rejected", func(t *testing.T) {
		res, _ := tc.uploadCarAttachment(t, car.ID, "empty.jpg", []byte{})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for empty file, got %d", res.StatusCode)
		}
	})

	t.Run("spoofed content type is neutralized by sniffing", func(t *testing.T) {
		// A PDF masquerading as a .jpg: the server must ignore the .jpg extension
		// and record the sniffed application/pdf type, so a spoof achieves nothing.
		res, att := tc.uploadCarAttachment(t, car.ID, "not-really.jpg", []byte("%PDF-1.7\n%âãÏÓ\n"))
		if res.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("upload: status %d, body %s", res.StatusCode, body)
		}
		if att.ContentType != "application/pdf" {
			t.Fatalf("expected sniffed application/pdf, got %q", att.ContentType)
		}
	})

	t.Run("missing car is 404", func(t *testing.T) {
		res, _ := tc.uploadCarAttachment(t, "00000000-0000-0000-0000-000000000000", "x.jpg", []byte{0xFF, 0xD8})
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", res.StatusCode)
		}
	})
}

func TestRepairAttachmentHandlers(t *testing.T) {
	api := newTestAPI(t)
	tc, _ := api.signup(t, "Garage", "Owner", "owner@example.com", "password-123")
	_, customer := tc.createCustomer(t, dto.CreateCustomerRequest{Name: "Ivan Petrov"})
	_, car := tc.createCar(t, customer.ID, dto.CreateCarRequest{Plate: "CB1234AB", Make: "VW", Model: "Golf"})

	// A repair is born by converting a sent offer.
	_, offer := tc.createOffer(t, car.ID, twoItemOffer())
	if res, _ := tc.sendOffer(t, offer.ID, "customer@example.com"); res.StatusCode != http.StatusOK {
		t.Fatalf("send offer: status %d", res.StatusCode)
	}
	res, repair := tc.acceptOffer(t, offer.ID)
	if res.StatusCode != http.StatusCreated || repair == nil {
		t.Fatalf("accept offer: status %d", res.StatusCode)
	}

	t.Run("upload and list repair attachment", func(t *testing.T) {
		res, att := tc.uploadRepairAttachment(t, repair.ID, "invoice.pdf", []byte("%PDF-1.7\n"))
		if res.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("upload: status %d, body %s", res.StatusCode, body)
		}
		if att.RepairID == nil || *att.RepairID != repair.ID {
			t.Fatalf("attachment not tied to repair: %+v", att)
		}
		if att.CarID != nil {
			t.Fatalf("repair attachment should not carry a car id: %+v", att)
		}
		if att.ContentType != "application/pdf" {
			t.Fatalf("expected sniffed pdf, got %q", att.ContentType)
		}

		res2, list := tc.listRepairAttachments(t, repair.ID)
		if res2.StatusCode != http.StatusOK {
			t.Fatalf("list: status %d", res2.StatusCode)
		}
		if len(list.Attachments) != 1 {
			t.Fatalf("expected 1 attachment, got %d", len(list.Attachments))
		}
	})

	t.Run("missing repair is 404", func(t *testing.T) {
		res, _ := tc.uploadRepairAttachment(t, "00000000-0000-0000-0000-000000000000", "x.pdf", []byte("%PDF-1.7\n"))
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", res.StatusCode)
		}
	})
}

func TestAttachmentHandlerTenantIsolation(t *testing.T) {
	api := newTestAPI(t)
	tcA, _ := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, _ := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	_, custA := tcA.createCustomer(t, dto.CreateCustomerRequest{Name: "A"})
	_, carA := tcA.createCar(t, custA.ID, dto.CreateCarRequest{Plate: "AA1234AA", Make: "VW", Model: "Golf"})

	_, att := tcA.uploadCarAttachment(t, carA.ID, "a.jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0})

	res, _ := tcB.listCarAttachments(t, carA.ID)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("B list A car attachments: expected 404, got %d", res.StatusCode)
	}

	res2, _ := tcB.client.Get(api.server.URL + "/api/v1/attachments/" + att.ID)
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("B download A attachment: expected 404, got %d", res2.StatusCode)
	}
	res2.Body.Close()

	req, _ := http.NewRequest("DELETE", api.server.URL+"/api/v1/attachments/"+att.ID, nil)
	res3, _ := tcB.client.Do(req)
	if res3.StatusCode != http.StatusNotFound {
		t.Fatalf("B delete A attachment: expected 404, got %d", res3.StatusCode)
	}
	res3.Body.Close()
}
