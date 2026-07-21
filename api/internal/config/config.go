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

	// TODO(phase 7): consumed by the SMTP mailer.
	SMTP struct {
		Host     string
		Port     int
		Username string
		Password string
		From     string
	}

	// TODO(phase 9): consumed by the R2 file store.
	R2 struct {
		Endpoint        string
		Region          string
		Bucket          string
		AccessKeyID     string
		SecretAccessKey string
	}

	// TODO(phase 2): trusted proxies for client-IP extraction in rate limiting.
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
