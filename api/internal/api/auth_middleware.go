package api

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/domain"
)

// requireAuth reads userID/tenantID/role from the session, re-loads the user
// from the store to confirm the session is still valid, and copies the
// tenant context into the request. Missing/invalid session → 401.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sm := s.session
		if sm == nil {
			s.logger.Error("scs session manager missing from server")
			renderProblem(w, r, http.StatusInternalServerError, CodeInternalError, "internal server error")
			return
		}

		userID := sm.GetString(r.Context(), "userID")
		tenantID := sm.GetString(r.Context(), "tenantID")
		if userID == "" || tenantID == "" {
			renderProblem(w, r, http.StatusUnauthorized, CodeAuthentication, "authentication required")
			return
		}

		user, err := s.users.GetByID(r.Context(), tenantID, userID)
		if err != nil {
			// Stale or invalid session — destroy it and require login.
			_ = sm.Destroy(r.Context())
			s.logger.Warn("auth: invalid session, destroyed",
				"requestId", requestIDFromContext(r.Context()),
				"userId", userID, "err", err)
			renderProblem(w, r, http.StatusUnauthorized, CodeAuthentication, "authentication required")
			return
		}

		ctx := withAuth(r.Context(), user.ID, user.TenantID, user.Role)
		ctx = withLogger(ctx, s.logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requirePermission returns a middleware that returns 403 if the authenticated
// user's role lacks the given permission.
func (s *Server) requirePermission(perm domain.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := roleFromContext(r.Context())
			if !domain.HasPermission(role, perm) {
				renderProblem(w, r, http.StatusForbidden, CodePermissionDenied, "permission denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// sessionMiddleware wraps scs LoadAndSave and exposes the session manager on
// the context for requireAuth and the auth handlers.
func sessionMiddleware(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return sm.LoadAndSave(next)
	}
}
