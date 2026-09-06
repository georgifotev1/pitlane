package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/gfotev/pitlane/internal/demo"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// demoCommand runs `pitlane demo <seed|reset|drop>`. It shares the runtime DSN
// and therefore the pitlane_app role, so every statement it issues is subject
// to the same row-level security as a request: a demonstration can be built or
// removed on a live deployment without any path to another tenant's data.
func demoCommand(ctx context.Context, dsn string, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: pitlane demo <seed|reset|drop> [-email ...] [-password ...] [-garage ...]")
	}
	command := args[0]
	switch command {
	case "seed", "reset", "drop":
	default:
		return fmt.Errorf("unknown demo command %q (want seed, reset or drop)", command)
	}

	var opts demo.Options
	flags := flag.NewFlagSet("pitlane demo", flag.ContinueOnError)
	flags.SetOutput(out)
	flags.StringVar(&opts.Email, "email", demo.DefaultEmail, "Login for the demonstration owner")
	flags.StringVar(&opts.Password, "password", demo.DefaultPassword, "Password for the demonstration owner")
	flags.StringVar(&opts.GarageName, "garage", demo.DefaultGarageName, "Garage name shown in the demonstration")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	opts.Now = time.Now()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()
	db := store.NewDB(pool)

	if command == "reset" || command == "drop" {
		tenantID, err := demo.Find(ctx, db, opts.Email)
		switch {
		case errors.Is(err, store.ErrNotFound):
			fmt.Fprintf(out, "No demonstration tenant found for %s.\n", opts.Email)
			if command == "drop" {
				return nil
			}
		case err != nil:
			return err
		default:
			if err := demo.Purge(ctx, db, tenantID); err != nil {
				if errors.Is(err, demo.ErrNotDemoTenant) {
					return fmt.Errorf("%s belongs to a real garage, not a demonstration; refusing to delete anything", opts.Email)
				}
				return err
			}
			fmt.Fprintf(out, "Removed the demonstration garage for %s.\n", opts.Email)
		}
		if command == "drop" {
			return nil
		}
	}

	result, err := demo.Seed(ctx, db, opts)
	if err != nil {
		if errors.Is(err, demo.ErrAlreadySeeded) {
			return fmt.Errorf("%s already exists; run `pitlane demo reset` to rebuild it", opts.Email)
		}
		return err
	}

	fmt.Fprintf(out, "Demonstration garage ready: %d customers, %d cars, %d offers, %d repairs.\n",
		result.Customers, result.Cars, result.Offers, result.Repairs)
	fmt.Fprintf(out, "Sign in with %s / %s\n", result.Email, result.Password)
	return nil
}
