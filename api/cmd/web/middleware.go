package main

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gfotev/pitlane/internal/store"
)

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				app.serverError(w, r, fmt.Errorf("%s\n%s", err, debug.Stack()))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (app *application) logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		app.logger.Info("request", "method", r.Method, "url", r.URL.RequestURI(), "remote", r.RemoteAddr, "duration", time.Since(start))
	})
}

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'self'; form-action 'self'; base-uri 'self'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		next.ServeHTTP(w, r)
	})
}

func (app *application) requireAuthentication(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.sessions.Exists(r.Context(), "userID") {
			app.redirectToLogin(w, r)
			return
		}

		user, err := app.users.GetByID(r.Context(), app.tenantID(r), app.userID(r))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				_ = app.sessions.Destroy(r.Context())
				app.redirectToLogin(w, r)
				return
			}
			app.serverError(w, r, err)
			return
		}
		if user.PasswordChangedAt != nil {
			authenticatedAt := time.Unix(0, app.sessions.GetInt64(r.Context(), "authenticatedAt"))
			if authenticatedAt.Before(*user.PasswordChangedAt) {
				_ = app.sessions.Destroy(r.Context())
				app.redirectToLogin(w, r)
				return
			}
		}
		next(w, r)
	}
}

func (app *application) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	app.sessions.Put(r.Context(), "redirectPath", r.URL.RequestURI())
	http.Redirect(w, r, "/account/login", http.StatusSeeOther)
}
