package main

import (
	"html/template"
	"log/slog"

	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
)

type application struct {
	logger     *slog.Logger
	sessions   *scs.SessionManager
	templates  map[string]*template.Template
	tenants    *store.TenantStore
	users      *store.UserStore
	customers  *store.CustomerStore
	cars       *store.CarStore
	history    *store.HistoryStore
	offers     *store.OfferStore
	repairs    *store.RepairStore
	dashboards *store.DashboardStore
	resets     *store.PasswordResetTokenStore
	pdf        *pdf.Renderer
	mailer     mailer.Mailer
	baseURL    string
}

type templateData struct {
	CurrentPath    string
	CurrentUser    string
	GarageName     string
	SiteURL        string
	CanonicalURL   string
	Flash          string
	Form           *forms.Form
	ResetRequested bool
	InvalidToken   bool

	Tenant       *domain.Tenant
	Customer     *domain.Customer
	Customers    []*domain.Customer
	Car          *domain.Car
	Cars         []store.CarSummary
	CustomerCars []*domain.Car
	History      []store.HistoryEntry
	Note         *domain.HistoryNote
	Offer        *domain.Offer
	Offers       []store.OfferSummary
	CarOffers    []*domain.Offer
	Repair       *domain.Repair
	Repairs      []store.RepairSummary
	RepairStats  store.RepairStats
	Total        int

	// Board, Document and Margin feed the components offers and repairs share:
	// one list table, one line-item table, one internal margin panel.
	Board    *documentBoard
	Document *documentView
	Margin   *marginView
	Editor   *itemEditor

	// Dashboard-only view models.
	Dashboard     store.Dashboard
	Stats         []statTile
	Revenue       *revenueChart
	Split         *splitChart
	RecentRepairs *documentBoard
	RecentOffers  *documentBoard
	Attention     *documentBoard
}

type offerRow struct {
	Kind        string
	Description string
	Quantity    string
	UnitPrice   string
	// Cost is the supplier price for this line, kept as raw form text so an
	// invalid entry is redisplayed exactly as it was typed.
	Cost string
}

type offerBundle struct {
	Offer    *domain.Offer
	Car      *domain.Car
	Customer *domain.Customer
	Tenant   *domain.Tenant
}

type repairBundle struct {
	Repair   *domain.Repair
	Car      *domain.Car
	Customer *domain.Customer
	Tenant   *domain.Tenant
}
