package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestPublicAndProtectedRoutes(t *testing.T) {
	app := &application{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), sessions: scs.New()}
	handler := app.routes()

	t.Run("health", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if response.Code != http.StatusOK || response.Body.String() != "ok" {
			t.Fatalf("got status %d body %q", response.Code, response.Body.String())
		}
	})

	t.Run("brand and crawler assets", func(t *testing.T) {
		for _, path := range []string{"/favicon.ico", "/site.webmanifest", "/robots.txt", "/static/favicon.svg"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Errorf("GET %s: got status %d; want 200", path, response.Code)
			}
		}
	})

	t.Run("protected page redirects", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/customers", nil))
		if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/account/login" {
			t.Fatalf("got status %d location %q", response.Code, response.Header().Get("Location"))
		}
	})

	t.Run("unknown page is not found", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/not-a-page", nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("got status %d; want 404", response.Code)
		}
	})

	t.Run("cross-origin post is rejected", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/account/logout", nil)
		request.Header.Set("Origin", "https://attacker.example")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("got status %d; want 403", response.Code)
		}
	})
}
