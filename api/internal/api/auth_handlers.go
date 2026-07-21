package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/validator"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// signup creates a new tenant and an owner user in a single transaction.
func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	var req dto.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	req.TenantName = strings.TrimSpace(req.TenantName)
	req.UserName = strings.TrimSpace(req.UserName)
	req.Email = strings.TrimSpace(req.Email)

	if err := validateSignup(&req); err != nil {
		renderValidation(w, r, err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		s.logger.Error("bcrypt hash", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	tenantID := uuid.NewString()
	userID := uuid.NewString()
	tenant := &domain.Tenant{
		ID:             tenantID,
		Name:           req.TenantName,
		Currency:       "EUR",
		Locale:         "bg",
		DefaultTaxRate: 1900,
		Settings:       map[string]any{},
	}
	user := &domain.User{
		ID:           userID,
		TenantID:     tenantID,
		Email:        strings.ToLower(req.Email),
		PasswordHash: string(hash),
		Role:         domain.RoleOwner,
		Name:         req.UserName,
	}

	if err := s.tenants.Create(r.Context(), tenant); err != nil {
		s.logger.Error("create tenant", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeAccountCreationFail, "could not create account")
		return
	}
	if err := s.users.Create(r.Context(), user); err != nil {
		s.logger.Error("create user", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeAccountCreationFail, "could not create account")
		return
	}

	// Establish the session immediately so signup is a one-step flow.
	if err := s.beginSession(r, user); err != nil {
		s.logger.Error("begin session", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "auth.signup", "tenant", tenantID, map[string]any{
		"email": user.Email,
	})

	renderJSON(w, http.StatusCreated, envelope{
		"user": userResponse(user),
	})
}

// login authenticates by email + password. Unknown email performs a dummy
// bcrypt comparison to equalize timing (ADR §Security enumeration).
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		renderProblem(w, r, http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password")
		return
	}

	user, err := s.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Dummy bcrypt to equalize timing.
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$12$dummyhashforenumerationequalization"), []byte(req.Password))
			renderProblem(w, r, http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password")
			return
		}
		s.logger.Error("get user by email", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		renderProblem(w, r, http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password")
		return
	}

	if err := s.beginSession(r, user); err != nil {
		s.logger.Error("begin session", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), user.TenantID, user.ID, "auth.login", "user", user.ID, nil)

	w.WriteHeader(http.StatusNoContent)
}

// logout destroys the session.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())

	if err := s.session.Destroy(r.Context()); err != nil {
		s.logger.Error("session destroy", "err", err)
	}
	_ = s.audit.Insert(r.Context(), tenantID, userID, "auth.logout", "user", userID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// me returns the authenticated user plus computed permissions.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	user, err := s.users.GetByID(r.Context(), tenantID, userID)
	if err != nil {
		s.logger.Error("me: get user", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	renderJSON(w, http.StatusOK, envelope{
		"user": userResponse(user),
	})
}

func (s *Server) beginSession(r *http.Request, user *domain.User) error {
	sm := s.session
	// RenewToken prevents session fixation (ADR §Security).
	if err := sm.RenewToken(r.Context()); err != nil {
		return err
	}
	sm.Put(r.Context(), "userID", user.ID)
	sm.Put(r.Context(), "tenantID", user.TenantID)
	sm.Put(r.Context(), "role", string(user.Role))
	sm.Put(r.Context(), "createdAt", time.Now().Unix())
	return nil
}

func userResponse(u *domain.User) dto.UserResponse {
	perms := domain.PermissionsFor(u.Role)
	names := make([]string, 0, len(perms))
	for _, p := range perms {
		names = append(names, string(p))
	}
	return dto.UserResponse{
		ID:          u.ID,
		TenantID:    u.TenantID,
		Email:       u.Email,
		Name:        u.Name,
		Role:        string(u.Role),
		Permissions: names,
	}
}

func validateSignup(r *dto.SignupRequest) map[string]string {
	v := validator.New()
	v.NotEmpty("tenantName", r.TenantName)
	v.MaxLength("tenantName", r.TenantName, 100)
	v.NotEmpty("userName", r.UserName)
	v.MaxLength("userName", r.UserName, 100)
	v.Email("email", r.Email)
	v.MaxLength("email", r.Email, 254)
	v.NotEmpty("password", r.Password)
	v.MinLength("password", r.Password, 8)
	v.MaxLength("password", r.Password, 72) // bcrypt cap (ADR decision 11)
	return v.Errors()
}

// renderValidation writes a 422 problem+json with the field→code map.
func renderValidation(w http.ResponseWriter, r *http.Request, errs map[string]string) {
	p := problemDetail{
		Type:      "about:blank",
		Title:     http.StatusText(http.StatusUnprocessableEntity),
		Status:    http.StatusUnprocessableEntity,
		Code:      CodeValidationFailed,
		RequestID: requestIDFromContext(r.Context()),
		Errors:    errs,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(p)
}

var _ = slog.Default
