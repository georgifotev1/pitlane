package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/gfotev/pitlane/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func migrate(ctx context.Context, dsn, command string, output io.Writer) error {
	if command != "up" && command != "down" && command != "status" {
		return fmt.Errorf("unknown migration command %q", command)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	migrationFS, err := fs.Sub(migrations.FS, "sql")
	if err != nil {
		return err
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrationFS)
	if err != nil {
		return err
	}
	defer func() { _ = provider.Close() }()
	switch command {
	case "up":
		results, err := provider.Up(ctx)
		if err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
		if len(results) == 0 {
			fmt.Fprintln(output, "already up to date")
		}
		for _, result := range results {
			fmt.Fprintln(output, result)
		}
	case "down":
		result, err := provider.Down(ctx)
		if errors.Is(err, goose.ErrNoCurrentVersion) || errors.Is(err, goose.ErrNoMigrations) {
			fmt.Fprintln(output, "nothing to roll back")
			return nil
		}
		if err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		fmt.Fprintln(output, result)
	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return fmt.Errorf("migration status: %w", err)
		}
		for _, status := range statuses {
			fmt.Fprintf(output, "%-7s %05d %s\n", status.State, status.Source.Version, status.Source.Path)
		}
	}
	return nil
}
