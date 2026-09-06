package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/gfotev/pitlane/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

type DB struct {
	Container *postgres.PostgresContainer
	Pool      *pgxpool.Pool
}

func New(t *testing.T) *DB {
	t.Helper()
	testcontainers.SkipIfProviderIsNotHealthy(t)
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

	pool, err := pgxpool.New(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("app pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return &DB{
		Container: ctr,
		Pool:      pool,
	}
}

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
		TRUNCATE invitations, attachments, history_notes, repair_items, repairs, audit_log, password_reset_tokens, sessions, offer_items, offers, document_counters, cars, customers, users, tenants RESTART IDENTITY CASCADE;
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
