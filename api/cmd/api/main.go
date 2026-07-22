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
	"github.com/gfotev/pitlane/internal/filestore"
	"github.com/gfotev/pitlane/internal/jobs"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/pdf"
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

	// Stores share the one pool. Built before River because the send-offer
	// worker depends on them (it loads the offer graph to render + mail the PDF).
	db := store.NewDB(pool)
	tenants := store.NewTenantStore(db)
	users := store.NewUserStore(db)
	customers := store.NewCustomerStore(db)
	cars := store.NewCarStore(db)
	offers := store.NewOfferStore(db)
	repairs := store.NewRepairStore(db)
	history := store.NewHistoryStore(db)
	attachments := store.NewAttachmentStore(db)
	audit := store.NewAuditLogStore(db)
	pdfRenderer := pdf.NewRenderer()

	fileStore, err := filestore.NewS3(filestore.Config{
		Endpoint:        cfg.R2.Endpoint,
		Region:          cfg.R2.Region,
		Bucket:          cfg.R2.Bucket,
		AccessKeyID:     cfg.R2.AccessKeyID,
		SecretAccessKey: cfg.R2.SecretAccessKey,
	})
	if err != nil {
		return fmt.Errorf("file store: %w", err)
	}
	mailSender := mailer.NewSMTP(mailer.Config{
		Host:     cfg.SMTP.Host,
		Port:     cfg.SMTP.Port,
		Username: cfg.SMTP.Username,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
	})

	// River runs in the same binary as the API (single-artifact deployment).
	// Its schema must already exist — River refuses to start unmigrated. The
	// worker set now carries the real SendOfferEmail job (Phase 7).
	riverClient, err := jobs.NewClient(pool, jobs.WorkerDeps{
		Offers:    offers,
		Cars:      cars,
		Customers: customers,
		Tenants:   tenants,
		PDF:       pdfRenderer,
		Mailer:    mailSender,
		Logger:    logger,
	})
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

	server, err := api.NewServer(api.ServerDeps{
		Logger:       logger,
		Cfg:          cfg,
		Session:      sessionManager,
		Tenants:      tenants,
		Users:        users,
		Customers:    customers,
		Cars:         cars,
		Offers:       offers,
		Repairs:      repairs,
		History:      history,
		Attachments:  attachments,
		Audit:        audit,
		PDF:          pdfRenderer,
		SendEnqueuer: jobs.NewOfferEmailEnqueuer(riverClient),
		Files:        fileStore,
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
