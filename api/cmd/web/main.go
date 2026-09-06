package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type config struct {
	addr         string
	dsn          string
	migrateDSN   string
	env          string
	baseURL      string
	smtpHost     string
	smtpPort     int
	smtpUsername string
	smtpPassword string
	smtpFrom     string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config{}
	flags := flag.NewFlagSet("pitlane", flag.ContinueOnError)
	flags.StringVar(&cfg.addr, "addr", ":"+env("PORT", "4000"), "HTTP network address")
	flags.StringVar(&cfg.dsn, "dsn", env("DSN", ""), "PostgreSQL runtime DSN")
	flags.StringVar(&cfg.migrateDSN, "migrate-dsn", env("MIGRATE_DSN", env("DSN", "")), "PostgreSQL migration DSN")
	flags.StringVar(&cfg.env, "env", env("ENV", "development"), "Environment")
	cfg.baseURL = env("APP_BASE_URL", "")
	cfg.smtpHost = env("SMTP_HOST", "")
	cfg.smtpPort = envInt("SMTP_PORT", 1025)
	cfg.smtpUsername = env("SMTP_USERNAME", "")
	cfg.smtpPassword = env("SMTP_PASSWORD", "")
	cfg.smtpFrom = env("SMTP_FROM", "")

	args := os.Args[1:]
	if len(args) > 0 && args[0] == "migrate" {
		if len(args) != 2 {
			return errors.New("usage: pitlane migrate <up|down|status>")
		}
		if err := flags.Parse(nil); err != nil {
			return err
		}
		if cfg.migrateDSN == "" {
			return errors.New("MIGRATE_DSN or DSN is required")
		}
		return migrate(context.Background(), cfg.migrateDSN, args[1], os.Stdout)
	}
	if len(args) > 0 && args[0] == "demo" {
		if err := flags.Parse(nil); err != nil {
			return err
		}
		if cfg.dsn == "" {
			return errors.New("DSN is required")
		}
		return demoCommand(context.Background(), cfg.dsn, args[1:], os.Stdout)
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if cfg.dsn == "" {
		return errors.New("DSN is required")
	}
	if cfg.env != "production" {
		if cfg.baseURL == "" {
			cfg.baseURL = "http://localhost:4000"
		}
		if cfg.smtpHost == "" {
			cfg.smtpHost = "localhost"
		}
		if cfg.smtpFrom == "" {
			cfg.smtpFrom = "hello@pitlane.local"
		}
	}
	if cfg.baseURL == "" || cfg.smtpHost == "" || cfg.smtpFrom == "" {
		return errors.New("APP_BASE_URL, SMTP_HOST and SMTP_FROM are required")
	}
	if cfg.smtpPort < 1 || cfg.smtpPort > 65535 {
		return errors.New("SMTP_PORT must be between 1 and 65535")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if cfg.env == "production" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	poolCfg, err := pgxpool.ParseConfig(cfg.dsn)
	if err != nil {
		return fmt.Errorf("parse DSN: %w", err)
	}
	poolCfg.MaxConns = 8
	poolCfg.MaxConnIdleTime = 2 * time.Minute
	poolCfg.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	templates, err := newTemplateCache()
	if err != nil {
		return err
	}
	db := store.NewDB(pool)
	sessions := scs.New()
	sessions.Store = pgxstore.New(pool)
	sessions.Lifetime = 12 * time.Hour
	sessions.IdleTimeout = 8 * time.Hour
	sessions.Cookie.Name = "pitlane_session"
	sessions.Cookie.HttpOnly = true
	sessions.Cookie.Secure = cfg.env == "production"
	sessions.Cookie.SameSite = http.SameSiteLaxMode
	sessions.Cookie.Path = "/"

	app := &application{
		logger: logger, sessions: sessions, templates: templates,
		tenants: store.NewTenantStore(db), users: store.NewUserStore(db),
		customers: store.NewCustomerStore(db), cars: store.NewCarStore(db),
		history: store.NewHistoryStore(db), offers: store.NewOfferStore(db),
		repairs: store.NewRepairStore(db), resets: store.NewPasswordResetTokenStore(db),
		dashboards: store.NewDashboardStore(db),
		pdf:        pdf.NewRenderer(), baseURL: cfg.baseURL,
		mailer: mailer.NewSMTP(mailer.Config{
			Host: cfg.smtpHost, Port: cfg.smtpPort, Username: cfg.smtpUsername,
			Password: cfg.smtpPassword, From: cfg.smtpFrom,
		}),
	}

	server := &http.Server{Addr: cfg.addr, Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: time.Minute}
	errCh := make(chan error, 1)
	go func() { logger.Info("server started", "addr", cfg.addr); errCh <- server.ListenAndServe() }()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("server stopped")
	return nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}
