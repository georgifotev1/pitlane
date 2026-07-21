package api

import (
	"context"
	"log/slog"

	"github.com/gfotev/pitlane/internal/domain"
)

type authKey string

const (
	authUserIDKey   authKey = "userID"
	authTenantIDKey authKey = "tenantID"
	authRoleKey     authKey = "role"
	authLoggerKey   authKey = "logger"
)

// userIDFromContext returns the authenticated user ID, or empty.
func userIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(authUserIDKey).(string)
	return id
}

// tenantIDFromContext returns the authenticated tenant ID, or empty.
func tenantIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(authTenantIDKey).(string)
	return id
}

// roleFromContext returns the authenticated user's role, or empty.
func roleFromContext(ctx context.Context) domain.Role {
	r, _ := ctx.Value(authRoleKey).(domain.Role)
	return r
}

// withAuth populates the auth keys on the request context.
func withAuth(ctx context.Context, userID, tenantID string, role domain.Role) context.Context {
	ctx = context.WithValue(ctx, authUserIDKey, userID)
	ctx = context.WithValue(ctx, authTenantIDKey, tenantID)
	ctx = context.WithValue(ctx, authRoleKey, role)
	return ctx
}

// withLogger attaches a child logger that already has the request/tenant/user
// attributes bound, so handlers don't have to repeat the slog plumbing.
func withLogger(ctx context.Context, base *slog.Logger) context.Context {
	attrs := []any{}
	if id := userIDFromContext(ctx); id != "" {
		attrs = append(attrs, slog.String("userId", id))
	}
	if tid := tenantIDFromContext(ctx); tid != "" {
		attrs = append(attrs, slog.String("tenantId", tid))
	}
	return context.WithValue(ctx, authLoggerKey, base.With(attrs...))
}

// loggerFromContext returns the request-scoped child logger, or the default.
func loggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(authLoggerKey).(*slog.Logger); ok {
		return l
	}
	return fallback
}
