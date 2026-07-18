package api

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// dist holds the production frontend build. The Dockerfile copies
// frontend/dist here before compiling (ADR decision 33: one binary serves
// API + SPA, same-origin). In development only the committed .gitkeep
// placeholder is present — Vite owns the frontend there, and the handler
// says so instead of failing opaquely.
//
//go:embed all:dist
var distFS embed.FS

// spaHandler serves the embedded SPA, falling back to index.html for
// non-API paths so client-side routes (TanStack Router) survive refresh.
type spaHandler struct {
	dist fs.FS
}

func newSPAHandler() (*spaHandler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, fmt.Errorf("spa dist: %w", err)
	}
	return &spaHandler{dist: sub}, nil
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")

	if path != "" {
		if f, err := h.dist.Open(path); err == nil {
			defer f.Close()
			if stat, err := f.Stat(); err == nil && !stat.IsDir() {
				h.serveFile(w, r, path, f)
				return
			}
		}
	}
	// Not a real file: a client-side route — serve the shell.
	h.serveIndex(w, r)
}

func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	f, err := h.dist.Open("index.html")
	if errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "frontend not built — in development run `make dev` and use the Vite server on http://localhost:5173\n", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	// The shell references hashed assets by name, so it must always be
	// revalidated — never cached beyond the current deploy.
	w.Header().Set("Cache-Control", "no-cache")
	serveContent(w, r, "index.html", f)
}

func (h *spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	// Vite content-hashes everything under assets/ — safe to cache forever
	// (ADR decision 16). The chain-wide `no-store` from secureHeaders is
	// overridden here deliberately.
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	serveContent(w, r, name, f)
}

func serveContent(w http.ResponseWriter, r *http.Request, name string, f fs.File) {
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(f)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		rs = bytes.NewReader(data)
	}
	// Zero modtime: no Last-Modified — embedded files have no meaningful mtime.
	http.ServeContent(w, r, name, time.Time{}, rs)
}
