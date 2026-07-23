package api

import (
	"log/slog"
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/api/dto"
	"github.com/gfotev/pitlane/internal/config"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/filestore"
	"github.com/gfotev/pitlane/internal/store"
)

type Server struct {
	logger       *slog.Logger
	env          string
	cfg          config.Config
	spa          *spaHandler
	session      *scs.SessionManager
	tenants      *store.TenantStore
	users        *store.UserStore
	customers    *store.CustomerStore
	cars         *store.CarStore
	offers       *store.OfferStore
	repairs      *store.RepairStore
	history      *store.HistoryStore
	attachments  *store.AttachmentStore
	audit        *store.AuditLogStore
	resets       *store.PasswordResetTokenStore
	invitations  *store.InvitationStore
	pdf          offerRenderer
	sendEnqueuer offerEmailEnqueuer
	// The Phase 10 enqueuers are the store's consumer interfaces directly
	// (nothing per-request to bind, unlike the offer's Reply-To).
	resetEnqueuer  store.PasswordResetEmailEnqueuer
	inviteEnqueuer store.InviteEmailEnqueuer
	files          filestore.Store
}

// ServerDeps bundles the runtime dependencies the HTTP layer needs.
type ServerDeps struct {
	Logger       *slog.Logger
	Cfg          config.Config
	Session      *scs.SessionManager
	Tenants      *store.TenantStore
	Users        *store.UserStore
	Customers    *store.CustomerStore
	Cars         *store.CarStore
	Offers       *store.OfferStore
	Repairs      *store.RepairStore
	History      *store.HistoryStore
	Attachments  *store.AttachmentStore
	Audit        *store.AuditLogStore
	Resets       *store.PasswordResetTokenStore
	Invitations  *store.InvitationStore
	PDF          offerRenderer
	SendEnqueuer offerEmailEnqueuer
	ResetEnqueuer  store.PasswordResetEmailEnqueuer
	InviteEnqueuer store.InviteEmailEnqueuer
	Files          filestore.Store
}

func NewServer(deps ServerDeps) (*Server, error) {
	spa, err := newSPAHandler()
	if err != nil {
		return nil, err
	}
	return &Server{
		logger:       deps.Logger,
		env:          deps.Cfg.Env,
		cfg:          deps.Cfg,
		spa:          spa,
		session:      deps.Session,
		tenants:      deps.Tenants,
		users:        deps.Users,
		customers:    deps.Customers,
		cars:         deps.Cars,
		offers:       deps.Offers,
		repairs:      deps.Repairs,
		history:      deps.History,
		attachments:  deps.Attachments,
		audit:        deps.Audit,
		resets:       deps.Resets,
		invitations:  deps.Invitations,
		pdf:          deps.PDF,
		sendEnqueuer: deps.SendEnqueuer,
		resetEnqueuer:  deps.ResetEnqueuer,
		inviteEnqueuer: deps.InviteEnqueuer,
		files:          deps.Files,
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
	// Password reset (public, ADR §Security: enumeration-safe, strict limiter
	// per ADR decision 18) and invitation acceptance (the emailed link's target).
	mux.Handle("POST /api/v1/auth/password-reset", s.strictRateLimit(http.HandlerFunc(s.requestPasswordReset)))
	mux.Handle("POST /api/v1/auth/password-reset/confirm", s.strictRateLimit(http.HandlerFunc(s.confirmPasswordReset)))
	mux.Handle("POST /api/v1/auth/accept-invite", s.strictRateLimit(http.HandlerFunc(s.acceptInvite)))

	// Team management: the user list reads, invitations and role changes write.
	// Mechanics hold neither users:read nor users:write — every route here is
	// 403 for them (the Phase 10 gate verifies exactly that).
	mux.Handle("GET /api/v1/users", s.protected(domain.PermissionUsersRead, s.listUsers))
	mux.Handle("PUT /api/v1/users/{id}/role", s.protected(domain.PermissionUsersWrite, s.updateUserRole))
	mux.Handle("GET /api/v1/users/invitations", s.protected(domain.PermissionUsersRead, s.listInvitations))
	mux.Handle("POST /api/v1/users/invitations", s.protected(domain.PermissionUsersWrite, s.inviteUser))
	mux.Handle("DELETE /api/v1/users/invitations/{id}", s.protected(domain.PermissionUsersWrite, s.revokeInvitation))

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
	// Send emails the offer PDF to the customer; it is the sole draft→sent path
	// (freeze-on-send) and also drives retries of a failed delivery.
	mux.Handle("POST /api/v1/offers/{id}/send", s.protected(domain.PermissionOffersWrite, s.sendOffer))
	// PDF is a read: gated by offers:read, streamed on demand (never stored).
	mux.Handle("GET /api/v1/offers/{id}/pdf", s.protected(domain.PermissionOffersRead, s.offerPDF))
	// Accept converts a sent offer into a repair (the sole accept path). Its
	// primary effect is creating the repair — the offer→accepted flip is a side
	// effect of conversion — so it is gated by repairs:write (a mechanic turns
	// quotes into jobs), not offers:write.
	mux.Handle("POST /api/v1/offers/{id}/accept", s.protected(domain.PermissionRepairsWrite, s.acceptOffer))

	// Repairs — the work performed on a car. Born only by converting an offer
	// (accept, above), so there is no create route here. The board list is
	// tenant-wide (?status= filter); item ops address the repair directly. Same
	// read/write split. Repairs are never deleted (ADR §Deletion policy). The
	// status endpoint drives open↔in_progress; completion (with the mileage
	// reading) is its own endpoint, the sole path to `completed`.
	mux.Handle("GET /api/v1/repairs", s.protected(domain.PermissionRepairsRead, s.listRepairs))
	mux.Handle("GET /api/v1/repairs/{id}", s.protected(domain.PermissionRepairsRead, s.getRepair))
	mux.Handle("PUT /api/v1/repairs/{id}", s.protected(domain.PermissionRepairsWrite, s.updateRepair))
	mux.Handle("POST /api/v1/repairs/{id}/status", s.protected(domain.PermissionRepairsWrite, s.updateRepairStatus))
	mux.Handle("POST /api/v1/repairs/{id}/complete", s.protected(domain.PermissionRepairsWrite, s.completeRepair))
	// Attachments nested under repairs.
	mux.Handle("GET /api/v1/repairs/{repairId}/attachments", s.protected(domain.PermissionAttachmentsRead, s.listRepairAttachments))
	mux.Handle("POST /api/v1/repairs/{repairId}/attachments", s.protected(domain.PermissionAttachmentsWrite, s.uploadRepairAttachment))

	// Service history + attachments for cars. History is read-only derived data plus
	// manual notes; attachments are photos/documents on the car.
	mux.Handle("GET /api/v1/cars/{carId}/history", s.protected(domain.PermissionHistoryRead, s.getCarHistory))
	mux.Handle("POST /api/v1/cars/{carId}/history/notes", s.protected(domain.PermissionHistoryWrite, s.createHistoryNote))
	mux.Handle("GET /api/v1/cars/{carId}/attachments", s.protected(domain.PermissionAttachmentsRead, s.listCarAttachments))
	mux.Handle("POST /api/v1/cars/{carId}/attachments", s.protected(domain.PermissionAttachmentsWrite, s.uploadCarAttachment))

	// Attachment item ops are addressed directly by id.
	mux.Handle("GET /api/v1/attachments/{id}", s.protected(domain.PermissionAttachmentsRead, s.downloadAttachment))
	mux.Handle("DELETE /api/v1/attachments/{id}", s.protected(domain.PermissionAttachmentsWrite, s.deleteAttachment))

	// History note item ops are addressed directly by id.
	mux.Handle("PUT /api/v1/history/notes/{id}", s.protected(domain.PermissionHistoryWrite, s.updateHistoryNote))
	mux.Handle("DELETE /api/v1/history/notes/{id}", s.protected(domain.PermissionHistoryWrite, s.deleteHistoryNote))

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
