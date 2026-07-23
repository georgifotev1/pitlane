package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port       int
	Env        string
	DSN        string
	MigrateDSN string

	// Session config consumed by scs (Phase 2).
	Session struct {
		Secret   string
		Lifetime time.Duration
	}

	// Consumed by the SMTP mailer (Phase 7). Dev points at Mailpit
	// (no auth, plaintext); prod at a provider like Resend (auth + STARTTLS).
	SMTP struct {
		Host     string
		Port     int
		Username string
		Password string
		From     string
	}

	// Consumed by the R2 file store (Phase 9). Dev points at MinIO.
	R2 struct {
		Endpoint        string
		Region          string
		Bucket          string
		AccessKeyID     string
		SecretAccessKey string
	}

	// App carries public-facing settings for the SPA. BaseURL is where emailed
	// links (password reset, invitations — Phase 10) point: the Vite dev server
	// locally, the same-origin deploy in prod.
	App struct {
		BaseURL string
	}

	// Trusted proxies for client-IP extraction in rate limiting.
	TrustedProxies []string
}

func Load() (Config, error) {
	var cfg Config
	var errs []error

	cfg.Port = envInt(&errs, "PORT", 4000)
	cfg.Env = envString("ENV", "development")
	cfg.DSN = envString("DSN", "")
	cfg.MigrateDSN = envString("MIGRATE_DSN", cfg.DSN)

	cfg.Session.Secret = envString("SESSION_SECRET", "")
	cfg.Session.Lifetime = envDuration(&errs, "SESSION_LIFETIME", 12*time.Hour)

	cfg.SMTP.Host = envString("SMTP_HOST", "localhost")
	cfg.SMTP.Port = envInt(&errs, "SMTP_PORT", 1025)
	cfg.SMTP.Username = envString("SMTP_USERNAME", "")
	cfg.SMTP.Password = envString("SMTP_PASSWORD", "")
	cfg.SMTP.From = envString("SMTP_FROM", "")

	cfg.R2.Endpoint = envString("R2_ENDPOINT", "")
	cfg.R2.Region = envString("R2_REGION", "auto")
	cfg.R2.Bucket = envString("R2_BUCKET", "")
	cfg.R2.AccessKeyID = envString("R2_ACCESS_KEY_ID", "")
	cfg.R2.SecretAccessKey = envString("R2_SECRET_ACCESS_KEY", "")

	// No default here: production must set it explicitly (checked below); the
	// Vite origin fallback applies to dev/test only.
	cfg.App.BaseURL = envString("APP_BASE_URL", "")
	if cfg.App.BaseURL == "" && cfg.Env != "production" {
		cfg.App.BaseURL = "http://localhost:5173"
	}

	if v := envString("TRUSTED_PROXY", ""); v != "" {
		cfg.TrustedProxies = strings.Split(v, ",")
	}

	switch cfg.Env {
	case "development", "test", "production":
	default:
		errs = append(errs, fmt.Errorf("ENV must be development, test or production, got %q", cfg.Env))
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be 1-65535, got %d", cfg.Port))
	}
	if cfg.DSN == "" {
		errs = append(errs, errors.New("DSN is required"))
	}
	if cfg.MigrateDSN == "" {
		errs = append(errs, errors.New("MIGRATE_DSN is required"))
	}
	if cfg.Env == "production" && len(cfg.Session.Secret) < 32 {
		errs = append(errs, errors.New("SESSION_SECRET must be at least 32 bytes in production"))
	}
	// Offer email is MVP scope (ADR §14): a real deployment must be able to send.
	if cfg.Env == "production" {
		if cfg.SMTP.Host == "" {
			errs = append(errs, errors.New("SMTP_HOST is required in production"))
		}
		if cfg.SMTP.From == "" {
			errs = append(errs, errors.New("SMTP_FROM is required in production"))
		}
	}
	// Emailed links must point at the real SPA origin in production; the
	// localhost fallback exists for dev only.
	if cfg.Env == "production" && cfg.App.BaseURL == "" {
		errs = append(errs, errors.New("APP_BASE_URL is required in production"))
	}

	return cfg, errors.Join(errs...)
}

func envString(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envInt(errs *[]error, key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be an integer: %w", key, err))
		return fallback
	}
	return n
}

func envDuration(errs *[]error, key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a duration: %w", key, err))
		return fallback
	}
	return d
}

func envBool(errs *[]error, key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a boolean: %w", key, err))
		return fallback
	}
	return b
}
