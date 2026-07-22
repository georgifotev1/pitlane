// Package validator is the Edwards-style homemade validator: small, explicit,
// and returns a camelCase field→code map that renders as RFC 9457 errors on 422.
// The map values are machine-readable codes; the client resolves each code to
// a localized message via its own error-code table (ADR §8, 2026-07-20).
package validator

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

// Error codes emitted to the client. Keep this list in sync with the client
// error-code table (frontend/src/lib/errorCodes.ts).
const (
	CodeRequired     = "required"
	CodeInvalidEmail = "invalid_email"
	CodeTooShort     = "too_short"
	CodeTooLong      = "too_long"
	CodeInvalid      = "invalid"
	// CodeDuplicate is not produced by a Check method — it is emitted by handlers
	// when a database unique constraint rejects a write (e.g. a duplicate car
	// plate). It lives here so the field-code registry stays in one place,
	// mirrored by the client error-code table.
	CodeDuplicate = "duplicate"
)

// Check holds validation state. A nil Check is valid and empty.
type Check struct {
	errors map[string]string
}

// NotEmpty checks that a field is non-empty after trimming.
func (v *Check) NotEmpty(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.add(field, CodeRequired)
	}
}

// Email checks that a field looks like an email address.
func (v *Check) Email(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.add(field, CodeRequired)
		return
	}
	if _, err := mail.ParseAddress(value); err != nil {
		v.add(field, CodeInvalidEmail)
		return
	}
	// mail.ParseAddress allows display names; reject them.
	if strings.Contains(value, "<") || strings.Contains(value, ">") {
		v.add(field, CodeInvalidEmail)
		return
	}
}

// MaxLength checks that a field does not exceed a byte length.
func (v *Check) MaxLength(field, value string, max int) {
	if utf8.RuneCountInString(value) > max {
		v.add(field, CodeTooLong)
	}
}

// MinLength checks that a field meets a minimum rune length.
func (v *Check) MinLength(field, value string, min int) {
	if utf8.RuneCountInString(value) < min {
		v.add(field, CodeTooShort)
	}
}

// Range checks that an integer field falls within [min, max] inclusive.
func (v *Check) Range(field string, value, min, max int) {
	if value < min || value > max {
		v.add(field, CodeInvalid)
	}
}

// OneOf checks that a field value is one of the allowed strings.
func (v *Check) OneOf(field, value string, allowed ...string) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	v.add(field, CodeInvalid)
}

// Valid reports whether no errors have been added.
func (v *Check) Valid() bool {
	return v == nil || len(v.errors) == 0
}

// Errors returns the accumulated field→code map.
func (v *Check) Errors() map[string]string {
	if v == nil {
		return nil
	}
	return v.errors
}

func (v *Check) add(field, code string) {
	if v.errors == nil {
		v.errors = make(map[string]string)
	}
	v.errors[field] = code
}

// New returns a fresh Check.
func New() *Check {
	return &Check{}
}
