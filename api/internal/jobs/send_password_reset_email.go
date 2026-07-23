package jobs

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/riverqueue/river"
)

// SendPasswordResetEmailArgs is the typed payload of the reset-email job. The
// plaintext Token travels in the job args (never in password_reset_tokens —
// that table holds the SHA-256 hash). A river_job row is therefore as
// sensitive as the token itself for its 1h lifetime: acceptable because DB
// access already implies the crown jewels (password hashes), and the token is
// single-use and expiring.
type SendPasswordResetEmailArgs struct {
	TenantID string `json:"tenantId"`
	UserID   string `json:"userId"`
	Email    string `json:"email"`
	UserName string `json:"userName"`
	Token    string `json:"token"`
}

// Kind is River's stable job identifier (persisted in river_job); never rename.
func (SendPasswordResetEmailArgs) Kind() string { return "send_password_reset_email" }

// InsertOpts pins the retry ceiling, same policy as the offer send.
func (SendPasswordResetEmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: maxSendAttempts}
}

// sendPasswordResetEmailWorker delivers the reset link. Unlike the offer send
// there is no status row to maintain — the token row's single-use semantics
// are the whole state — so the worker is a pure render-and-send.
type sendPasswordResetEmailWorker struct {
	river.WorkerDefaults[SendPasswordResetEmailArgs]
	deps WorkerDeps
}

// Work performs one send attempt; River retries with backoff on error.
func (w *sendPasswordResetEmailWorker) Work(ctx context.Context, job *river.Job[SendPasswordResetEmailArgs]) error {
	args := job.Args
	resetURL := strings.TrimRight(w.deps.BaseURL, "/") + "/reset-password?token=" + url.QueryEscape(args.Token)

	subject, html, text, err := mailer.PasswordResetEmail(mailer.PasswordResetEmailData{
		UserName: args.UserName,
		ResetURL: resetURL,
	})
	if err != nil {
		return fmt.Errorf("render reset email: %w", err)
	}

	if err := w.deps.Mailer.Send(ctx, mailer.Message{
		To:      args.Email,
		Subject: subject,
		HTML:    html,
		Text:    text,
	}); err != nil {
		return fmt.Errorf("send password reset email: %w", err)
	}
	w.deps.Logger.Info("password reset email sent", "userId", args.UserID, "tenantId", args.TenantID, "attempt", job.Attempt)
	return nil
}
