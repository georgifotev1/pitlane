// Package testdb provides the testcontainers harness shared by the store and
// API integration tests. It boots a Postgres 18 container, runs migrations,
// and returns a pool connected as the app role (pitlane_app) so RLS is
// enforced.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gfotev/pitlane/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// DB is a per-test database handle.
type DB struct {
	Container *postgres.PostgresContainer
	Pool      *pgxpool.Pool
}

// New starts a Postgres container, runs migrations, and returns a pool
// connected as the app role.
func New(t *testing.T) *DB {
	t.Helper()
	ctx := context.Background()

	ctr, err := postgres.Run(ctx,
		"postgres:18",
		postgres.WithDatabase("pitlane"),
		postgres.WithUsername("pitlane"),
		postgres.WithPassword("pitlane"),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := ctr.Terminate(ctx); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	ownerDSN, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("owner connection string: %v", err)
	}

	if err := runMigrations(ctx, ownerDSN); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	appDSN := strings.Replace(ownerDSN, "user=pitlane", "user=pitlane_app password=pitlane_app", 1)
	pool, err := pgxpool.New(ctx, appDSN)
	if err != nil {
		t.Fatalf("app pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return &DB{
		Container: ctr,
		Pool:      pool,
	}
}

// Cleanup truncates all tenant-owned tables using the owner connection.
func (d *DB) Cleanup(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	ownerDSN, err := d.Container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("owner dsn: %v", err)
	}
	db, err := sql.Open("pgx", ownerDSN)
	if err != nil {
		t.Fatalf("open owner db: %v", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		TRUNCATE invitations, attachments, history_notes, repair_items, repairs, audit_log, password_reset_tokens, sessions, offer_items, offers, cars, customers, users, tenants RESTART IDENTITY CASCADE;
	`)
	if err != nil {
		t.Fatalf("truncate tables: %v", err)
	}
}

func runMigrations(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("open pool: %w", err)
	}
	defer pool.Close()

	if err := waitForDB(ctx, db); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	sqlFS, err := migrations.Sub()
	if err != nil {
		return fmt.Errorf("migrations fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, sqlFS)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	defer provider.Close()
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("river up: %w", err)
	}
	return nil
}

func waitForDB(ctx context.Context, db *sql.DB) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}
	for time.Now().Before(deadline) {
		if err := db.PingContext(ctx); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return db.PingContext(ctx)
}
