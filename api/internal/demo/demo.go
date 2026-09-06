// Package demo builds and removes a self-contained demonstration garage: a
// tenant with a year of finished work behind it, quotes in every state and a
// couple of jobs deliberately left standing still. It exists so the product can
// be shown to a prospect on a real deployment, with real numbers moving through
// the dashboard, without going anywhere near a paying customer's data.
package demo

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultEmail      = "demo@pitlane.bg"
	DefaultPassword   = "pitlane-demo"
	DefaultGarageName = "Автосервиз Мотор"

	// months of history the demonstration carries - the span the dashboard plots.
	months = 12
	// bcryptCost matches the sign-up path so the demo login behaves identically.
	bcryptCost = 12
)

// ErrNotDemoTenant guards the only destructive path in the binary.
var ErrNotDemoTenant = errors.New("tenant is not a demonstration tenant")

var ErrAlreadySeeded = errors.New("a demonstration tenant already exists for this email")

type Options struct {
	Email      string
	Password   string
	GarageName string
	// Now anchors the generated history. Everything is relative to it, so a
	// demonstration seeded today and one seeded next year look the same age.
	Now time.Time
}

func (o *Options) setDefaults() {
	if o.Email == "" {
		o.Email = DefaultEmail
	}
	if o.Password == "" {
		o.Password = DefaultPassword
	}
	if o.GarageName == "" {
		o.GarageName = DefaultGarageName
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
}

type Result struct {
	TenantID  string
	Email     string
	Password  string
	Customers int
	Cars      int
	Offers    int
	Repairs   int
}

type stores struct {
	db        *store.DB
	tenants   *store.TenantStore
	customers *store.CustomerStore
	cars      *store.CarStore
	history   *store.HistoryStore
	offers    *store.OfferStore
	repairs   *store.RepairStore
}

// Seed creates the demonstration tenant and fills it. Everything is written
// through the ordinary stores - quotes are accepted into repairs, repairs are
// completed and record mileage - so the result is data the application itself
// could have produced. Only the timestamps are rewritten afterwards, because no
// legitimate code path can place work in the past.
func Seed(ctx context.Context, db *store.DB, opts Options) (*Result, error) {
	opts.setDefaults()
	if len(opts.Password) < 8 {
		return nil, errors.New("demo password must be at least 8 characters")
	}

	s := stores{
		db:        db,
		tenants:   store.NewTenantStore(db),
		customers: store.NewCustomerStore(db),
		cars:      store.NewCarStore(db),
		history:   store.NewHistoryStore(db),
		offers:    store.NewOfferStore(db),
		repairs:   store.NewRepairStore(db),
	}

	if _, err := Find(ctx, db, opts.Email); err == nil {
		return nil, ErrAlreadySeeded
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(opts.Password), bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hash demo password: %w", err)
	}

	tenant := &domain.Tenant{
		ID:             uuid.NewString(),
		Name:           opts.GarageName,
		Address:        "гр. София, бул. Околовръстен път 128",
		VATNumber:      "BG204418822",
		Currency:       "EUR",
		Locale:         "bg",
		DefaultTaxRate: domain.StandardVATRateBPS,
		Settings:       map[string]any{},
	}
	owner := &domain.User{
		ID:           uuid.NewString(),
		TenantID:     tenant.ID,
		Email:        opts.Email,
		PasswordHash: string(hash),
		Role:         domain.RoleOwner,
		Name:         "Демо собственик",
	}
	if err := s.tenants.CreateWithOwner(ctx, tenant, owner); err != nil {
		return nil, fmt.Errorf("create demo tenant: %w", err)
	}
	if err := markDemo(ctx, db, tenant.ID); err != nil {
		return nil, err
	}

	plan := newPlan(opts.Now)
	result := &Result{TenantID: tenant.ID, Email: opts.Email, Password: opts.Password}
	if err := plan.execute(ctx, s, tenant.ID, result); err != nil {
		return nil, err
	}
	return result, nil
}

// Find returns the tenant behind a demonstration login.
func Find(ctx context.Context, db *store.DB, email string) (string, error) {
	user, err := store.NewUserStore(db).GetByEmail(ctx, email)
	if err != nil {
		return "", err
	}
	return user.TenantID, nil
}

// Purge empties a demonstration tenant and removes it. Two things keep it
// harmless: the row-level security policies confine every statement to the one
// tenant, and the is_demo flag means a tenant that was never created as a
// demonstration cannot be deleted by this command at all.
func Purge(ctx context.Context, db *store.DB, tenantID string) error {
	return db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var isDemo bool
		if err := tx.QueryRow(ctx, `SELECT is_demo FROM tenants WHERE id = $1`, tenantID).Scan(&isDemo); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return store.ErrNotFound
			}
			return err
		}
		if !isDemo {
			return ErrNotDemoTenant
		}

		// Children first: every foreign key in the schema is RESTRICT, so the
		// order below is the only one that works.
		for _, table := range []string{
			"repair_items", "repairs", "offer_items", "offers",
			"history_notes", "cars", "customers", "document_counters", "users",
		} {
			if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE tenant_id = $1`, tenantID); err != nil {
				return fmt.Errorf("purge %s: %w", table, err)
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tenantID); err != nil {
			return fmt.Errorf("purge tenant: %w", err)
		}
		return nil
	})
}

func markDemo(ctx context.Context, db *store.DB, tenantID string) error {
	return db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE tenants SET is_demo = true WHERE id = $1`, tenantID)
		return err
	})
}

// randomiser keeps the demonstration identical on every run: the same garage,
// the same jobs, the same numbers, so a pitch is never a surprise.
func randomiser() *rand.Rand { return rand.New(rand.NewPCG(0x9E3779B9, 0x85EBCA6B)) }
