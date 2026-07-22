package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/config"
	"github.com/gfotev/pitlane/internal/jobs"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

// testAPI boots the full API stack against a per-test Postgres container.
type testAPI struct {
	server *httptest.Server
	tdb    *testdb.DB
}

// tenantClient is a cookie-jar-backed HTTP client for one tenant.
type tenantClient struct {
	client *http.Client
	api    *testAPI
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	cfg := config.Config{
		Port: 0,
		Env:  "test",
	}
	cfg.Session.Lifetime = 12 * time.Hour

	sessionManager := scs.New()
	sessionManager.Store = pgxstore.New(tdb.Pool)
	sessionManager.Lifetime = 12 * time.Hour
	sessionManager.IdleTimeout = 30 * time.Minute
	sessionManager.Cookie.Name = "pitlane_session"
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	sessionManager.Cookie.Path = "/"

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db := store.NewDB(tdb.Pool)
	// An insert-only River client (never Started) backs the enqueuer, so
	// POST /offers/{id}/send exercises the real transactional insert into
	// river_job. Jobs are not worked in handler tests — they assert the row
	// lands; the worker itself is tested in internal/jobs.
	riverClient, err := jobs.NewInsertOnlyClient(tdb.Pool)
	if err != nil {
		t.Fatalf("river insert-only client: %v", err)
	}
	server, err := NewServer(ServerDeps{
		Logger:       logger,
		Cfg:          cfg,
		Session:      sessionManager,
		Tenants:      store.NewTenantStore(db),
		Users:        store.NewUserStore(db),
		Customers:    store.NewCustomerStore(db),
		Cars:         store.NewCarStore(db),
		Offers:       store.NewOfferStore(db),
		Audit:        store.NewAuditLogStore(db),
		PDF:          pdf.NewRenderer(),
		SendEnqueuer: jobs.NewOfferEmailEnqueuer(riverClient),
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	return &testAPI{server: ts, tdb: tdb}
}

func (a *testAPI) newClient() *tenantClient {
	jar, _ := cookiejar.New(nil)
	return &tenantClient{
		client: &http.Client{Jar: jar},
		api:    a,
	}
}

func (a *testAPI) signup(t *testing.T, tenantName, userName, email, password string) (*tenantClient, *dto.UserResponse) {
	t.Helper()
	tc := a.newClient()
	body, _ := json.Marshal(dto.SignupRequest{
		TenantName: tenantName,
		UserName:   userName,
		Email:      email,
		Password:   password,
	})
	res, err := tc.client.Post(a.server.URL+"/api/v1/auth/signup", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("signup: status %d, body %s", res.StatusCode, b)
	}
	var env struct {
		User dto.UserResponse `json:"user"`
	}
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("signup decode: %v", err)
	}
	return tc, &env.User
}

func (tc *tenantClient) me(t *testing.T) (*http.Response, []byte) {
	t.Helper()
	res, err := tc.client.Get(tc.api.server.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, body
}

func (tc *tenantClient) logout(t *testing.T) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", tc.api.server.URL+"/api/v1/auth/logout", nil)
	res, err := tc.client.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	return res
}

func TestTwoTenantIsolation(t *testing.T) {
	api := newTestAPI(t)
	ctx := context.Background()

	tcA, userA := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	tcB, userB := api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	if userA.ID == userB.ID || userA.TenantID == userB.TenantID {
		t.Fatalf("tenants must be distinct")
	}
	if userA.Role != "owner" || userB.Role != "owner" {
		t.Fatalf("both should be owners, got %s / %s", userA.Role, userB.Role)
	}

	t.Run("A sees only A in /me", func(t *testing.T) {
		res, body := tcA.me(t)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("me A: status %d, body %s", res.StatusCode, body)
		}
		var env struct {
			User dto.UserResponse `json:"user"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("me A decode: %v", err)
		}
		if env.User.TenantID != userA.TenantID {
			t.Fatalf("A sees wrong tenant: got %s, want %s", env.User.TenantID, userA.TenantID)
		}
		if env.User.Email != "a@example.com" {
			t.Fatalf("A sees wrong email: got %s", env.User.Email)
		}
	})

	t.Run("B sees only B in /me", func(t *testing.T) {
		res, body := tcB.me(t)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("me B: status %d, body %s", res.StatusCode, body)
		}
		var env struct {
			User dto.UserResponse `json:"user"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("me B decode: %v", err)
		}
		if env.User.TenantID != userB.TenantID {
			t.Fatalf("B sees wrong tenant: got %s, want %s", env.User.TenantID, userB.TenantID)
		}
		if env.User.Email != "b@example.com" {
			t.Fatalf("B sees wrong email: got %s", env.User.Email)
		}
	})

	t.Run("B cannot see A via the store", func(t *testing.T) {
		us := store.NewUserStore(store.NewDB(api.tdb.Pool))
		_, err := us.GetByID(ctx, userB.TenantID, userA.ID)
		if err == nil {
			t.Fatalf("expected not found, got nil")
		}
		if err.Error() != "not found" {
			t.Fatalf("expected 'not found', got %v", err)
		}
	})

	t.Run("logout invalidates session", func(t *testing.T) {
		res := tcA.logout(t)
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("logout: status %d", res.StatusCode)
		}
		res2, _ := tcA.me(t)
		if res2.StatusCode != http.StatusUnauthorized {
			t.Fatalf("me after logout: status %d, want 401", res2.StatusCode)
		}
	})
}

// TestRLSEnforcedAtDatabase probes the RLS layer directly, bypassing the
// WithTenant helper and the store's `WHERE tenant_id = $1` filters. Those app
// filters are the FIRST tenancy layer; this test guards the SECOND (Postgres
// RLS). The two-tenant API tests above pass whether or not RLS is enabled,
// because every store query filters on tenant_id itself — so only a raw probe
// like this can catch RLS silently reverting to off (as it was before 0005,
// when FORCE was set but ENABLE was never issued).
func TestRLSEnforcedAtDatabase(t *testing.T) {
	api := newTestAPI(t)
	ctx := context.Background()

	// Two tenants, each with exactly one user row (the signup owner).
	_, userA := api.signup(t, "Garage A", "Owner A", "a@example.com", "password-aaa")
	_, _ = api.signup(t, "Garage B", "Owner B", "b@example.com", "password-bbb")

	// A single raw connection. We switch into the non-owner pitlane_app role the
	// same way WithTenant does (SET ROLE) — the test harness pool connects as the
	// database superuser, which bypasses RLS, so relying on its role would make
	// this probe vacuous. What we deliberately DON'T do is call WithTenant or add
	// a `WHERE tenant_id` filter: the database policy is then the only thing
	// standing between this connection and every tenant's rows.
	conn, err := api.tdb.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire conn: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET ROLE pitlane_app"); err != nil {
		t.Fatalf("set role pitlane_app: %v", err)
	}

	t.Run("no tenant context errors", func(t *testing.T) {
		// With RLS on, the USING clause evaluates current_setting('app.tenant_id'),
		// which is unset on this fresh session and raises. With RLS off, the query
		// would happily count every tenant's users.
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err == nil {
			t.Fatalf("expected an error with no app.tenant_id set (RLS disabled?), got count=%d", n)
		}
	})

	t.Run("fake tenant sees zero rows", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "SET app.tenant_id = '00000000-0000-0000-0000-000000000000'"); err != nil {
			t.Fatalf("set fake tenant: %v", err)
		}
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
			t.Fatalf("count users: %v", err)
		}
		if n != 0 {
			t.Fatalf("a fake tenant must see 0 users (RLS disabled?), got %d", n)
		}
	})

	t.Run("real tenant sees only its own row", func(t *testing.T) {
		if _, err := conn.Exec(ctx, "SELECT set_config('app.tenant_id', $1, false)", userA.TenantID); err != nil {
			t.Fatalf("set tenant A: %v", err)
		}
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
			t.Fatalf("count users: %v", err)
		}
		if n != 1 {
			t.Fatalf("tenant A must see exactly its own 1 user, got %d", n)
		}
	})
}

func TestAuthEdgeCases(t *testing.T) {
	api := newTestAPI(t)

	t.Run("me without session is 401", func(t *testing.T) {
		tc := api.newClient()
		res, err := tc.client.Get(api.server.URL + "/api/v1/auth/me")
		if err != nil {
			t.Fatalf("get me: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want 401", res.StatusCode)
		}
	})

	t.Run("signup with bad payload is 422", func(t *testing.T) {
		tc := api.newClient()
		body, _ := json.Marshal(dto.SignupRequest{
			TenantName: "",
			UserName:   "",
			Email:      "not-an-email",
			Password:   "short",
		})
		res, err := tc.client.Post(api.server.URL+"/api/v1/auth/signup", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("signup: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusUnprocessableEntity {
			b, _ := io.ReadAll(res.Body)
			t.Fatalf("status: got %d, want 422, body %s", res.StatusCode, b)
		}
		var p map[string]any
		if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		errs, _ := p["errors"].(map[string]any)
		if errs == nil || len(errs) == 0 {
			t.Fatalf("expected errors map, got %+v", p)
		}
	})

	t.Run("login with wrong password is 401", func(t *testing.T) {
		email := uuid.NewString() + "@example.com"
		_, _ = api.signup(t, "Garage", "Owner", email, "rightpass")
		tc := api.newClient()
		body, _ := json.Marshal(dto.LoginRequest{Email: email, Password: "wrong"})
		res, err := tc.client.Post(api.server.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("login: %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want 401", res.StatusCode)
		}
	})
}
