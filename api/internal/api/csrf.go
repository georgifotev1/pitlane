package api

import "net/http"

// csrfProtection wraps http.CrossOriginProtection (Go 1.25+). It rejects
// non-safe cross-origin browser requests. The SPA and API share the same
// origin in production; in development the Vite proxy preserves the Origin
// header so cross-origin protection still applies correctly.
func csrfProtection() func(http.Handler) http.Handler {
	cp := http.NewCrossOriginProtection()
	return func(next http.Handler) http.Handler {
		return cp.Handler(next)
	}
}
