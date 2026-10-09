package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/demo"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newDemoTestApp(t *testing.T) (http.Handler, *store.DB, *pgxpool.Pool) {
	t.Helper()
	tdb := testdb.New(t)
	db := store.NewDB(tdb.Pool)
	templates, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}
	app := &application{
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)), sessions: scs.New(), templates: templates,
		tenants: store.NewTenantStore(db), users: store.NewUserStore(db),
		customers: store.NewCustomerStore(db), resets: store.NewPasswordResetTokenStore(db),
		dashboards: store.NewDashboardStore(db), offers: store.NewOfferStore(db),
		repairs: store.NewRepairStore(db), cars: store.NewCarStore(db), history: store.NewHistoryStore(db),
	}
	return app.routes(), db, tdb.Pool
}

func post(t *testing.T, handler http.Handler, path string, form url.Values, cookies []*http.Cookie, referer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Everyone who presses the demo button shares one garage, so nothing any of
// them submits may reach the database.
func TestDemoVisitorsCanBrowseButNotChangeAnything(t *testing.T) {
	handler, db, pool := newDemoTestApp(t)
	ctx := context.Background()
	if _, err := demo.Seed(ctx, db, demo.Options{Now: time.Now()}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	login := post(t, handler, "/demo", nil, nil, "")
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/dashboard" {
		t.Fatalf("demo login: status %d location %q", login.Code, login.Header().Get("Location"))
	}
	cookies := login.Result().Cookies()

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, req)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "demo-banner") {
		t.Fatalf("dashboard: status %d, demo banner shown: %v", page.Code, strings.Contains(page.Body.String(), "demo-banner"))
	}

	customersBefore := countRows(t, pool, `SELECT count(*) FROM customers`)
	created := post(t, handler, "/customers/new", url.Values{"name": {"Посетител"}}, cookies, "http://example.com/customers/new")
	if created.Code != http.StatusSeeOther || created.Header().Get("Location") != "/customers/new" {
		t.Fatalf("create customer: status %d location %q", created.Code, created.Header().Get("Location"))
	}
	if after := countRows(t, pool, `SELECT count(*) FROM customers`); after != customersBefore {
		t.Fatalf("a demo visitor added a customer: %d -> %d", customersBefore, after)
	}

	// Taking over the shared login would lock every later visitor out.
	changed := post(t, handler, "/account/profile/password", url.Values{
		"current_password": {demo.DefaultPassword}, "new_password": {"taken-over-1"}, "confirm_password": {"taken-over-1"},
	}, cookies, "")
	if changed.Code != http.StatusSeeOther || changed.Header().Get("Location") != "/dashboard" {
		t.Fatalf("password change: status %d location %q", changed.Code, changed.Header().Get("Location"))
	}
	if n := countRows(t, pool, `SELECT count(*) FROM users WHERE email = $1 AND password_changed_at IS NOT NULL`, demo.DefaultEmail); n != 0 {
		t.Fatal("a demo visitor changed the shared password")
	}

	post(t, handler, "/account/forgot-password", url.Values{"email": {demo.DefaultEmail}}, nil, "")
	if n := countRows(t, pool, `SELECT count(*) FROM password_reset_tokens`); n != 0 {
		t.Fatal("a password reset was issued for the shared demo login")
	}

	logout := post(t, handler, "/account/logout", nil, cookies, "")
	if logout.Header().Get("Location") != "/account/login" {
		t.Fatalf("logout: status %d location %q", logout.Code, logout.Header().Get("Location"))
	}
}

// The button signs in by email alone, so it must never open a real garage
// that happens to use the demo address.
func TestDemoButtonRefusesARealGarage(t *testing.T) {
	handler, db, _ := newDemoTestApp(t)
	tenant := &domain.Tenant{ID: uuid.NewString(), Name: "Истински сервиз", Currency: "EUR", Locale: "bg", DefaultTaxRate: domain.StandardVATRateBPS, Settings: map[string]any{}}
	owner := &domain.User{ID: uuid.NewString(), TenantID: tenant.ID, Email: demo.DefaultEmail, PasswordHash: "x", Role: domain.RoleOwner, Name: "Собственик"}
	if err := store.NewTenantStore(db).CreateWithOwner(context.Background(), tenant, owner); err != nil {
		t.Fatal(err)
	}

	rec := post(t, handler, "/demo", nil, nil, "")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/account/login" {
		t.Fatalf("got status %d location %q", rec.Code, rec.Header().Get("Location"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "session" && c.Value != "" {
			req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
			req.AddCookie(c)
			page := httptest.NewRecorder()
			handler.ServeHTTP(page, req)
			if page.Code == http.StatusOK {
				t.Fatal("the demo button signed in to a real garage")
			}
		}
	}
}
