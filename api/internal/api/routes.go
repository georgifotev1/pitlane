package api

import (
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/config"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
)

type Server struct {
	logger    *slog.Logger
	env       string
	cfg       config.Config
	spa       *spaHandler
	session   *scs.SessionManager
	tenants   *store.TenantStore
	users     *store.UserStore
	customers *store.CustomerStore
	cars      *store.CarStore
	offers    *store.OfferStore
	audit     *store.AuditLogStore
}

// ServerDeps bundles the runtime dependencies the HTTP layer needs.
type ServerDeps struct {
	Logger    *slog.Logger
	Cfg       config.Config
	Session   *scs.SessionManager
	Tenants   *store.TenantStore
	Users     *store.UserStore
	Customers *store.CustomerStore
	Cars      *store.CarStore
	Offers    *store.OfferStore
	Audit     *store.AuditLogStore
}

func NewServer(deps ServerDeps) (*Server, error) {
	spa, err := newSPAHandler()
	if err != nil {
		return nil, err
	}
	return &Server{
		logger:    deps.Logger,
		env:       deps.Cfg.Env,
		cfg:       deps.Cfg,
		spa:       spa,
		session:   deps.Session,
		tenants:   deps.Tenants,
		users:     deps.Users,
		customers: deps.Customers,
		cars:      deps.Cars,
		offers:    deps.Offers,
		audit:     deps.Audit,
	}, nil
}

// Handler wires the middleware chain per ADR §19:
//
//	recover → requestID → logRequests → secureHeaders
//	→ rateLimit → CSRF → scs LoadAndSave
//	→ (mux; protected routes wrap requireAuth + requirePermission)
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Public routes.
	mux.HandleFunc("GET /api/v1/healthz", s.healthz)
	mux.Handle("POST /api/v1/auth/signup", http.HandlerFunc(s.signup))
	mux.Handle("POST /api/v1/auth/login", s.strictRateLimit(http.HandlerFunc(s.login)))
	mux.Handle("POST /api/v1/auth/logout", s.requireAuth(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/auth/me", s.requireAuth(http.HandlerFunc(s.me)))

	// Customers — the pattern-setting slice. read gates GET, write gates
	// mutations. Archive is a POST (soft-delete), leaving DELETE free for a
	// future hard-delete of dependent-free records (ADR §Deletion policy).
	mux.Handle("GET /api/v1/customers", s.protected(domain.PermissionCustomersRead, s.listCustomers))
	mux.Handle("POST /api/v1/customers", s.protected(domain.PermissionCustomersWrite, s.createCustomer))
	mux.Handle("GET /api/v1/customers/{id}", s.protected(domain.PermissionCustomersRead, s.getCustomer))
	mux.Handle("PUT /api/v1/customers/{id}", s.protected(domain.PermissionCustomersWrite, s.updateCustomer))
	mux.Handle("POST /api/v1/customers/{id}/archive", s.protected(domain.PermissionCustomersWrite, s.archiveCustomer))

	// Cars — a car belongs to a customer, so list/create are nested under the
	// customer; item ops (get/update/archive) address the car directly. Same
	// read/write permission split as customers; archive is a POST soft-delete.
	mux.Handle("GET /api/v1/customers/{customerId}/cars", s.protected(domain.PermissionCarsRead, s.listCars))
	mux.Handle("POST /api/v1/customers/{customerId}/cars", s.protected(domain.PermissionCarsWrite, s.createCar))
	mux.Handle("GET /api/v1/cars/{id}", s.protected(domain.PermissionCarsRead, s.getCar))
	mux.Handle("PUT /api/v1/cars/{id}", s.protected(domain.PermissionCarsWrite, s.updateCar))
	mux.Handle("POST /api/v1/cars/{id}/archive", s.protected(domain.PermissionCarsWrite, s.archiveCar))

	// Offers — an offer belongs to a car, so list/create are nested under the
	// car; item ops (get/update/status) address the offer directly. Same
	// read/write split. Offers are never deleted (ADR §Deletion policy); the
	// status endpoint drives the draft→sent→accepted|rejected|expired machine.
	mux.Handle("GET /api/v1/cars/{carId}/offers", s.protected(domain.PermissionOffersRead, s.listOffers))
	mux.Handle("POST /api/v1/cars/{carId}/offers", s.protected(domain.PermissionOffersWrite, s.createOffer))
	mux.Handle("GET /api/v1/offers/{id}", s.protected(domain.PermissionOffersRead, s.getOffer))
	mux.Handle("PUT /api/v1/offers/{id}", s.protected(domain.PermissionOffersWrite, s.updateOffer))
	mux.Handle("POST /api/v1/offers/{id}/status", s.protected(domain.PermissionOffersWrite, s.updateOfferStatus))

	// Unmatched API paths get problem+json — never the SPA shell.
	mux.HandleFunc("GET /api/", s.notFound)
	mux.Handle("GET /", s.spa)

	chain := recoverPanic(s.logger,
		requestID(
			logRequests(s.logger,
				secureHeaders(
					s.rateLimit(
						csrfProtection()(
							sessionMiddleware(s.session)(mux),
						),
					),
				),
			),
		),
	)
	return chain
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	renderJSON(w, http.StatusOK, envelope{
		"health": dto.HealthResponse{Status: "ok", Environment: s.env},
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	renderProblem(w, r, http.StatusNotFound, CodeNotFound, "resource not found")
}
