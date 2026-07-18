package api

import (
	"encoding/json"
	"net/http"
)

type envelope map[string]any

// RFC 9457 problem detail. The errors map carries field-level validation
// messages on 422 responses (from Phase 2 onward).
type problemDetail struct {
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Status    int               `json:"status"`
	Detail    string            `json:"detail,omitempty"`
	Instance  string            `json:"instance,omitempty"`
	RequestID string            `json:"requestId"`
	Errors    map[string]string `json:"errors,omitempty"`
}

func renderJSON(w http.ResponseWriter, status int, data envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func renderProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	p := problemDetail{
		Type:      "about:blank",
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		RequestID: requestIDFromContext(r.Context()),
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
