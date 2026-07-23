package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/validator"
	"golang.org/x/crypto/bcrypt"
)

// newAuthToken mints a 256-bit random token. The plaintext form goes into the
// emailed link (base64url — URL-safe by construction); only its SHA-256 hash
// is ever stored (ADR §Security).
func newAuthToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	return plain, hashAuthToken(plain), nil
}

func hashAuthToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// requestPasswordReset starts a reset. The response is ALWAYS 204 with an
// identical body regardless of whether the email exists, is throttled, or got
// an email — account enumeration must learn nothing (ADR §Security). Side
// effects happen only for a known, unthrottled account: token row + River
// email job in one transaction, plus the audit entry.
func (s *Server) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req dto.PasswordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	v := validator.New()
	v.Email("email", req.Email)
	if !v.Valid() {
		renderValidation(w, r, v.Errors())
		return
	}

	user, err := s.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Unknown account: do the same CPU work as the known path (token
			// generation + hashing) so the response shape and rough timing are
			// indistinguishable, then return the identical 204. The DB-write
			// asymmetry is inherent — a token cannot be inserted for a user
			// that does not exist — and is the accepted residual (same pattern
			// as Edwards' Let's Go Further).
			_, _, _ = newAuthToken()
			_, _ = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), 12)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.logger.Error("password reset: get user", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	token, tokenHash, err := newAuthToken()
	if err != nil {
		s.logger.Error("password reset: mint token", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	expiresAt := time.Now().Add(domain.PasswordResetTokenTTL)
	if err := s.resets.RequestReset(r.Context(), user, tokenHash, token, expiresAt, s.resetEnqueuer); err != nil {
		if errors.Is(err, store.ErrResetThrottled) {
			// Silently succeed: the legitimate user stops receiving emails for
			// a while; an attacker learns nothing.
			s.logger.Warn("password reset throttled", "userId", user.ID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.logger.Error("password reset: request", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), user.TenantID, user.ID, "auth.password_reset_request", "user", user.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// confirmPasswordReset consumes a token and sets the new password. Success
// also deletes every other token for the user and stamps password_changed_at,
// which destroys all existing sessions (enforced in requireAuth). Any token
// problem is the same 400 invalid_token — unknown, expired, and used are
// indistinguishable to the caller.
func (s *Server) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	var req dto.PasswordResetConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	req.Token = strings.TrimSpace(req.Token)

	v := validator.New()
	v.NotEmpty("token", req.Token)
	v.NotEmpty("password", req.Password)
	v.MinLength("password", req.Password, 8)
	v.MaxLength("password", req.Password, 72) // bcrypt cap (ADR decision 11)
	if !v.Valid() {
		renderValidation(w, r, v.Errors())
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		s.logger.Error("password reset: bcrypt", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	user, err := s.resets.Consume(r.Context(), hashAuthToken(req.Token), string(hash))
	if err != nil {
		if errors.Is(err, store.ErrInvalidToken) {
			renderProblem(w, r, http.StatusBadRequest, CodeInvalidToken, "invalid or expired token")
			return
		}
		s.logger.Error("password reset: consume", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), user.TenantID, user.ID, "auth.password_reset", "user", user.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}
