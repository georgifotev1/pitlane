package api

import (
	"encoding/json"
	"errors"
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

// listUsers returns every member of the tenant (team screen), oldest first.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())

	users, err := s.users.List(r.Context(), tenantID)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list users", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	out := make([]dto.UserResponse, 0, len(users))
	for _, u := range users {
		out = append(out, userResponse(u))
	}
	renderJSON(w, http.StatusOK, envelope{"users": out})
}

// updateUserRole changes a team member's role. Two business guards live here
// (object-level checks, ADR §10): you cannot change your own role (an owner
// demoting themselves could lock the tenant out of its admin surface), and the
// owner role is immutable through this path (ownership transfer is out of
// scope; owner is born at signup). Both are 409s — state conflicts, not
// permission failures: the caller HAD users:write to reach this code.
func (s *Server) updateUserRole(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	targetID := r.PathValue("id")
	log := loggerFromContext(r.Context(), s.logger)

	var req dto.UpdateUserRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}

	v := validator.New()
	v.OneOf("role", req.Role, string(domain.RoleAdmin), string(domain.RoleMechanic))
	if !v.Valid() {
		renderValidation(w, r, v.Errors())
		return
	}

	if targetID == userID {
		renderProblem(w, r, http.StatusConflict, CodeCannotChangeOwnRole, "cannot change own role")
		return
	}

	target, err := s.users.GetByID(r.Context(), tenantID, targetID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "user not found")
			return
		}
		log.Error("update role: load target", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	if target.Role == domain.RoleOwner {
		renderProblem(w, r, http.StatusConflict, CodeCannotChangeOwnerRole, "owner role is immutable")
		return
	}

	if err := s.users.UpdateRole(r.Context(), tenantID, targetID, domain.Role(req.Role)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "user not found")
			return
		}
		log.Error("update role", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "user.role", "user", targetID, map[string]any{
		"role": req.Role,
	})

	updated, err := s.users.GetByID(r.Context(), tenantID, targetID)
	if err != nil {
		log.Error("update role: reload", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	renderJSON(w, http.StatusOK, envelope{"user": userResponse(updated)})
}

// inviteUser creates a staff invitation and dispatches the invite email in the
// same transaction. The invited email must not already be a registered user
// (globally unique emails — one email = one tenant, ADR §Domain Model) nor
// have a pending invite in this tenant; both surface as 422 field errors.
func (s *Server) inviteUser(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	log := loggerFromContext(r.Context(), s.logger)

	var req dto.InviteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))

	v := validator.New()
	v.Email("email", req.Email)
	v.MaxLength("email", req.Email, 254)
	v.OneOf("role", req.Role, string(domain.RoleAdmin), string(domain.RoleMechanic))
	if !v.Valid() {
		renderValidation(w, r, v.Errors())
		return
	}

	if _, err := s.users.GetByEmail(r.Context(), req.Email); err == nil {
		renderValidation(w, r, map[string]string{"email": validator.CodeEmailTaken})
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		log.Error("invite: check email", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	token, tokenHash, err := newAuthToken()
	if err != nil {
		log.Error("invite: mint token", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	inv := &domain.Invitation{
		ID:        uuid.NewString(),
		TenantID:  tenantID,
		Email:     req.Email,
		Role:      domain.Role(req.Role),
		TokenHash: tokenHash,
		InvitedBy: userID,
		ExpiresAt: time.Now().Add(domain.InvitationTokenTTL),
	}
	if err := s.invitations.Create(r.Context(), inv, token, s.inviteEnqueuer); err != nil {
		if errors.Is(err, store.ErrDuplicateInvitation) {
			renderValidation(w, r, map[string]string{"email": validator.CodeAlreadyInvited})
			return
		}
		log.Error("invite: create", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "user.invite", "user", inv.ID, map[string]any{
		"email": inv.Email,
		"role":  string(inv.Role),
	})

	renderJSON(w, http.StatusCreated, envelope{"invitation": invitationResponse(inv)})
}

// listInvitations returns the tenant's pending invitations, newest first.
func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())

	invitations, err := s.invitations.ListPending(r.Context(), tenantID)
	if err != nil {
		loggerFromContext(r.Context(), s.logger).Error("list invitations", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	out := make([]dto.InvitationResponse, 0, len(invitations))
	for _, inv := range invitations {
		out = append(out, invitationResponse(inv))
	}
	renderJSON(w, http.StatusOK, envelope{"invitations": out})
}

// revokeInvitation hard-deletes a pending invitation. The invite link dies
// with the row, and the email becomes re-invitable.
func (s *Server) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	tenantID := tenantIDFromContext(r.Context())
	userID := userIDFromContext(r.Context())
	id := r.PathValue("id")

	if err := s.invitations.Delete(r.Context(), tenantID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusNotFound, CodeNotFound, "invitation not found")
			return
		}
		loggerFromContext(r.Context(), s.logger).Error("revoke invitation", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), tenantID, userID, "user.invite_revoke", "user", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// acceptInvite completes an invited account from the emailed token: it creates
// the user with the invited role in the inviting tenant (same transaction as
// marking the invite consumed) and starts a session, mirroring signup. The
// invitee supplies only name + password — the email and role come from the
// invitation. Any token problem is the same 400 invalid_token as the reset
// flow. If the email registered elsewhere after the invite was sent, the
// accept fails cleanly with a root form error (422 email_taken).
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	var req dto.AcceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidJSON, "invalid JSON body")
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	req.Name = strings.TrimSpace(req.Name)

	v := validator.New()
	v.NotEmpty("token", req.Token)
	v.NotEmpty("name", req.Name)
	v.MaxLength("name", req.Name, 100)
	v.NotEmpty("password", req.Password)
	v.MinLength("password", req.Password, 8)
	v.MaxLength("password", req.Password, 72) // bcrypt cap (ADR decision 11)
	if !v.Valid() {
		renderValidation(w, r, v.Errors())
		return
	}

	inv, err := s.invitations.GetByToken(r.Context(), hashAuthToken(req.Token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			renderProblem(w, r, http.StatusBadRequest, CodeInvalidToken, "invalid or expired token")
			return
		}
		s.logger.Error("accept invite: lookup", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}
	if inv.AcceptedAt != nil || time.Now().After(inv.ExpiresAt) {
		renderProblem(w, r, http.StatusBadRequest, CodeInvalidToken, "invalid or expired token")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		s.logger.Error("accept invite: bcrypt", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	user := &domain.User{
		ID:           uuid.NewString(),
		TenantID:     inv.TenantID,
		Email:        inv.Email,
		PasswordHash: string(hash),
		Role:         inv.Role,
		Name:         req.Name,
	}
	if err := s.invitations.Accept(r.Context(), inv, user); err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidToken):
			renderProblem(w, r, http.StatusBadRequest, CodeInvalidToken, "invalid or expired token")
		case errors.Is(err, store.ErrDuplicateEmail):
			renderValidation(w, r, map[string]string{"_form": validator.CodeEmailTaken})
		default:
			s.logger.Error("accept invite", "err", err)
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		}
		return
	}

	if err := s.beginSession(r, user); err != nil {
		s.logger.Error("accept invite: begin session", "err", err)
		renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
		return
	}

	_ = s.audit.Insert(r.Context(), inv.TenantID, user.ID, "auth.invite_accept", "user", user.ID, map[string]any{
		"email": user.Email,
		"role":  string(user.Role),
	})

	renderJSON(w, http.StatusCreated, envelope{
		"user": userResponse(user),
	})
}

func invitationResponse(inv *domain.Invitation) dto.InvitationResponse {
	return dto.InvitationResponse{
		ID:        inv.ID,
		Email:     inv.Email,
		Role:      string(inv.Role),
		ExpiresAt: inv.ExpiresAt,
		CreatedAt: inv.CreatedAt,
	}
}
