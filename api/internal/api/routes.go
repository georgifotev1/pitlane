package api

import (
	"log/slog"
	"net/http"

	"github.com/gfotev/pitlane/internal/api/dto"
)

type Server struct {
	logger *slog.Logger
	env    string
	spa    *spaHandler
}

func NewServer(logger *slog.Logger, env string) (*Server, error) {
	spa, err := newSPAHandler()
	if err != nil {
		return nil, err
	}
	return &Server{logger: logger, env: env, spa: spa}, nil
}

// Handler wires the middleware chain per ADR §19; rate limiting, CSRF, scs
// sessions and auth/tenant middleware slot in here in Phase 2.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/healthz", s.healthz)
	// Unmatched API paths get problem+json — never the SPA shell.
	mux.HandleFunc("GET /api/", s.notFound)
	mux.Handle("GET /", s.spa)
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
