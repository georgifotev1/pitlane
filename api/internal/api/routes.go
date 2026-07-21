package api

import (
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/config"
	"github.com/gfotev/pitlane/internal/store"
)

type Server struct {
	logger  *slog.Logger
	env     string
	cfg     config.Config
	spa     *spaHandler
	session *scs.SessionManager
	tenants *store.TenantStore
	users   *store.UserStore
	audit   *store.AuditLogStore
}

// ServerDeps bundles the runtime dependencies the HTTP layer needs.
type ServerDeps struct {
	Logger  *slog.Logger
	Cfg     config.Config
	Session *scs.SessionManager
	Tenants *store.TenantStore
	Users   *store.UserStore
	Audit   *store.AuditLogStore
}

func NewServer(deps ServerDeps) (*Server, error) {
	spa, err := newSPAHandler()
	if err != nil {
		return nil, err
	}
	return &Server{
		logger:  deps.Logger,
		env:     deps.Cfg.Env,
		cfg:     deps.Cfg,
		spa:     spa,
		session: deps.Session,
		tenants: deps.Tenants,
		users:   deps.Users,
		audit:   deps.Audit,
	}, nil
}

// Handler wires the middleware chain per ADR §19:
//
//	recover → requestID → logRequests → secureHeaders
//	→ rateLimit → CSRF → scs LoadAndSave
//	→ (mux; protected routes wrap requireAuth + requirePermission)
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /api/v1/healthz", s.healthz)
	mux.Handle("POST /api/v1/auth/signup", http.HandlerFunc(s.signup))
	mux.Handle("POST /api/v1/auth/login", s.strictRateLimit(http.HandlerFunc(s.login)))
	mux.Handle("POST /api/v1/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/auth/me", s.requireAuth(http.HandlerFunc(s.me)))

	// Unmatched API paths get problem+json — never the SPA shell.
	mux.HandleFunc("GET /api/", s.notFound)
	mux.Handle("GET /", s.spa)

	chain := recoverPanic(s.logger,
		requestID(
			logRequests(s.logger,
				secureHeaders(
					s.rateLimit(
						csrfProtection()(
							sessionMiddleware(s.session)(mux),
						),
					),
				),
			),
		),
	)
	return chain
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	renderJSON(w, http.StatusOK, envelope{
		"health": dto.HealthResponse{Status: "ok", Environment: s.env},
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	renderProblem(w, r, http.StatusNotFound, CodeNotFound, "resource not found")
}
