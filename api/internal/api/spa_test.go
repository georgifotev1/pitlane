package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testSPAHandler() *spaHandler {
	return &spaHandler{dist: fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte("<html>pitlane</html>")},
		"assets/app-a1b2c3.js": &fstest.MapFile{Data: []byte("console.log('app')")},
		"favicon.svg":          &fstest.MapFile{Data: []byte("<svg></svg>")},
	}}
}

func TestSPAHandler(t *testing.T) {
	tests := []struct {
		name             string
		path             string
		wantStatus       int
		wantBody         string
		wantCacheControl string
	}{
		{
			name:             "root serves the shell",
			path:             "/",
			wantStatus:       http.StatusOK,
			wantBody:         "<html>pitlane</html>",
			wantCacheControl: "no-cache",
		},
		{
			name:             "client-side route falls back to the shell",
			path:             "/customers/42/cars",
			wantStatus:       http.StatusOK,
			wantBody:         "<html>pitlane</html>",
			wantCacheControl: "no-cache",
		},
		{
			name:             "hashed asset is immutable",
			path:             "/assets/app-a1b2c3.js",
			wantStatus:       http.StatusOK,
			wantBody:         "console.log('app')",
			wantCacheControl: "public, max-age=31536000, immutable",
		},
		{
			name:             "unhashed static file gets no special caching",
			path:             "/favicon.svg",
			wantStatus:       http.StatusOK,
			wantBody:         "<svg></svg>",
			wantCacheControl: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			testSPAHandler().ServeHTTP(rec, req)
			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status: got %d, want %d", res.StatusCode, tt.wantStatus)
			}
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != tt.wantBody {
				t.Errorf("body: got %q, want %q", body, tt.wantBody)
			}
			if got := res.Header.Get("Cache-Control"); got != tt.wantCacheControl {
				t.Errorf("Cache-Control: got %q, want %q", got, tt.wantCacheControl)
			}
		})
	}
}

func TestSPAHandlerContentTypes(t *testing.T) {
	tests := []struct {
		path        string
		wantTypeSub string
	}{
		{"/", "text/html"},
		{"/assets/app-a1b2c3.js", "javascript"},
		{"/favicon.svg", "image/svg+xml"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			testSPAHandler().ServeHTTP(rec, req)
			res := rec.Result()
			defer res.Body.Close()

			if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, tt.wantTypeSub) {
				t.Errorf("Content-Type: got %q, want substring %q", ct, tt.wantTypeSub)
			}
		})
	}
}

// In development the embedded dist holds only the .gitkeep placeholder: the
// handler must say the frontend isn't built, not 404 silently.
func TestSPAHandlerPlaceholderDist(t *testing.T) {
	h, err := newSPAHandler()
	if err != nil {
		t.Fatalf("newSPAHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// This test runs against the real embed: pass iff dist is the placeholder
	// (dev), skip when a real frontend build has been copied in.
	if strings.Contains(string(body), "<html") {
		t.Skip("real frontend build embedded — placeholder behaviour not applicable")
	}
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", res.StatusCode, http.StatusNotFound)
	}
	if !strings.Contains(string(body), "make dev") {
		t.Errorf("body should point at the dev workflow, got %q", body)
	}
}
