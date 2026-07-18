package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/gfotev/pitlane/internal/config"
	"github.com/gfotev/pitlane/migrations"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// runMigrate implements "api migrate up|down|status". Two migration lines
// share the one database: goose owns the application schema (api/migrations),
// River self-migrates its job tables (ADR decisions 7 + 17). Both run here so
// a single command takes a fresh database to ready. Migrations are always an
// explicit operator action — the server never migrates on boot.
func runMigrate(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: api migrate <up|down|status>")
	}
	cmd := args[0]
	switch cmd {
	case "up", "down", "status":
	default:
		return fmt.Errorf("unknown migrate command %q\nusage: api migrate <up|down|status>", cmd)
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// goose speaks database/sql over the pgx stdlib driver; River wants the
	// native pgx pool. Two thin handles, one database.
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return fmt.Errorf("db pool: %w", err)
	}
	defer pool.Close()

	if err := migrateGoose(ctx, db, cmd, stdout); err != nil {
		return err
	}
	return migrateRiver(ctx, pool, cmd, stdout)
}

func migrateGoose(ctx context.Context, db *sql.DB, cmd string, stdout io.Writer) error {
	sqlFS, err := fs.Sub(migrations.FS, "sql")
	if err != nil {
		return fmt.Errorf("migrations fs: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, sqlFS)
	if errors.Is(err, goose.ErrNoMigrations) {
		// Expected until Phase 2 lands migration 0001 — not an error.
		fmt.Fprintln(stdout, "goose: no application migrations present")
		return nil
	}
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	defer func() { _ = provider.Close() }()

	switch cmd {
	case "up":
		results, err := provider.Up(ctx)
		if err != nil {
			return fmt.Errorf("goose up: %w", err)
		}
		if len(results) == 0 {
			fmt.Fprintln(stdout, "goose: already up to date")
		}
		for _, r := range results {
			fmt.Fprintf(stdout, "goose: %s\n", r)
		}
	case "down":
		result, err := provider.Down(ctx)
		if errors.Is(err, goose.ErrNoMigrations) || errors.Is(err, goose.ErrNoCurrentVersion) {
			fmt.Fprintln(stdout, "goose: nothing to roll back")
			return nil
		}
		if err != nil {
			return fmt.Errorf("goose down: %w", err)
		}
		fmt.Fprintf(stdout, "goose: %s\n", result)
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return fmt.Errorf("goose status: %w", err)
		}
		for _, s := range statuses {
			fmt.Fprintf(stdout, "goose: %-7s %05d %s\n", s.State, s.Source.Version, s.Source.Path)
		}
	}
	return nil
}

func migrateRiver(ctx context.Context, pool *pgxpool.Pool, cmd string, stdout io.Writer) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("river migrator: %w", err)
	}

	switch cmd {
	case "up", "down":
		direction := rivermigrate.DirectionUp
		if cmd == "down" {
			// Down defaults to a single step, mirroring goose down.
			direction = rivermigrate.DirectionDown
		}
		res, err := migrator.Migrate(ctx, direction, nil)
		if err != nil {
			return fmt.Errorf("river migrate %s: %w", direction, err)
		}
		if len(res.Versions) == 0 {
			fmt.Fprintln(stdout, "river: no change")
		}
		for _, v := range res.Versions {
			fmt.Fprintf(stdout, "river: %s %03d %s\n", res.Direction, v.Version, v.Name)
		}
	case "status":
		applied := make(map[int]bool)
		existing, err := migrator.ExistingVersions(ctx)
		if err != nil {
			// 42P01 (undefined table): river_migration does not exist yet,
			// i.e. a pristine database — report everything as pending.
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "42P01" {
				return fmt.Errorf("river status: %w", err)
			}
		}
		for _, m := range existing {
			applied[m.Version] = true
		}
		for _, m := range migrator.AllVersions() {
			state := "pending"
			if applied[m.Version] {
				state = "applied"
			}
			fmt.Fprintf(stdout, "river: %-7s %03d %s\n", state, m.Version, m.Name)
		}
	}
	return nil
}
