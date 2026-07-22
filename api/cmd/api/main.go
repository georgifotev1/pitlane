package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/api"
	"github.com/gfotev/pitlane/internal/config"
	"github.com/gfotev/pitlane/internal/jobs"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run dispatches subcommands Edwards-style: a plain os.Args switch, no CLI
// framework. Bare "api" is the server; "api migrate …" is the operator tool.
func run() error {
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"serve"}
	}
	switch args[0] {
	case "serve":
		return serve()
	case "migrate":
		return runMigrate(context.Background(), args[1:], os.Stdout)
	default:
		return fmt.Errorf("unknown command %q\nusage: api [serve] | api migrate <up|down|status>", args[0])
	}
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	var logger *slog.Logger
	if cfg.Env == "production" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return fmt.Errorf("db pool: %w", err)
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	logger.Info("database connected")

	// River runs in the same binary as the API (single-artifact deployment).
	// Its schema must already exist — River refuses to start unmigrated.
	riverClient, err := jobs.NewClient(pool)
	if err != nil {
		return err
	}
	if err := riverClient.Start(ctx); err != nil {
		return fmt.Errorf("river start (migrated? run: api migrate up): %w", err)
	}
	logger.Info("river client started")

	// scs session manager backed by Postgres (ADR decision 9).
	sessionManager := scs.New()
	sessionManager.Store = pgxstore.New(pool)
	sessionManager.Lifetime = cfg.Session.Lifetime
	sessionManager.IdleTimeout = 30 * time.Minute
	sessionManager.Cookie.Name = "pitlane_session"
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.Secure = cfg.Env == "production"
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	sessionManager.Cookie.Path = "/"

	// Wire stores.
	db := store.NewDB(pool)
	server, err := api.NewServer(api.ServerDeps{
		Logger:    logger,
		Cfg:       cfg,
		Session:   sessionManager,
		Tenants:   store.NewTenantStore(db),
		Users:     store.NewUserStore(db),
		Customers: store.NewCustomerStore(db),
		Cars:      store.NewCarStore(db),
		Offers:    store.NewOfferStore(db),
		Audit:     store.NewAuditLogStore(db),
	})
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	// Shutdown order: stop taking HTTP traffic, then drain job workers, then
	// the deferred pool.Close() releases connections last.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := riverClient.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("river drain: %w", err)
	}
	logger.Info("server stopped")
	return nil
}
