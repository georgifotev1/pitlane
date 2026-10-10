package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/gfotev/pitlane/ui"
)

var assetVersions sync.Map

// assetURL returns the /static/ URL for name with a content hash appended, so
// the file can be cached forever and still update on every deploy.
func assetURL(name string) string {
	if v, ok := assetVersions.Load(name); ok {
		return v.(string)
	}
	url := "/static/" + name
	if b, err := fs.ReadFile(ui.Files, "static/"+name); err == nil {
		sum := sha256.Sum256(b)
		url += "?v=" + hex.EncodeToString(sum[:])[:12]
	}
	assetVersions.Store(name, url)
	return url
}

// cacheStatic sets Cache-Control on static files. Embedded files have no
// modification time, so without this browsers re-download them on every page
// load. Fonts and hash-versioned URLs never change, so they are immutable;
// replacing a font file requires a new file name.
func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("v") || strings.HasPrefix(r.URL.Path, "/static/fonts/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		next.ServeHTTP(w, r)
	})
}
