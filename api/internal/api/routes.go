package api

import (
	"log/slog"
	"net/http"

	"github.com/gfotev/pitlane/internal/api/dto"
)

type Server struct {
	logger *slog.Logger
	env    string
}

func NewServer(logger *slog.Logger, env string) *Server {
	return &Server{logger: logger, env: env}
}

// Handler wires the middleware chain per ADR §19; rate limiting, CSRF, scs
// sessions and auth/tenant middleware slot in here in Phase 2.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/healthz", s.healthz)
	// TODO(phase 1): SPA fallback replaces this catch-all.
	mux.HandleFunc("GET /", s.notFound)
	return recoverPanic(s.logger, requestID(logRequests(s.logger, secureHeaders(mux))))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	renderJSON(w, http.StatusOK, envelope{
		"health": dto.HealthResponse{Status: "ok", Environment: s.env},
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	renderProblem(w, r, http.StatusNotFound, "resource not found")
}
