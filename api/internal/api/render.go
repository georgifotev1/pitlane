package api

import (
	"encoding/json"
	"net/http"
)

type envelope map[string]any

// Stable, machine-readable error codes. The client maps each code to a
// localized message; the server never sends user-facing strings as `detail`
// or in the `errors` map.
const (
	CodeInvalidCredentials  = "invalid_credentials"
	CodeInvalidJSON         = "invalid_json"
	CodeInternalError       = "internal_error"
	CodeAccountCreationFail = "account_creation_failed"
	CodeAuthentication      = "authentication_required"
	CodePermissionDenied    = "permission_denied"
	CodeNotFound            = "not_found"
	CodeConflict            = "conflict"
	CodeRateLimited         = "rate_limit_exceeded"
	CodeValidationFailed    = "validation_failed"
)

// RFC 9457 problem detail. The errors map carries field-level validation codes
// on 422 responses; the `code` field carries a top-level error code that the
// client uses to localize the `detail` string (ADR §8, 2026-07-20).
type problemDetail struct {
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Status    int               `json:"status"`
	Code      string            `json:"code,omitempty"`
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

func renderProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	p := problemDetail{
		Type:      "about:blank",
		Title:     http.StatusText(status),
		Status:    status,
		Code:      code,
		Detail:    detail,
		RequestID: requestIDFromContext(r.Context()),
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}
