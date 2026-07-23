package jobs

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/riverqueue/river"
)

// SendInviteEmailArgs is the typed payload of the invitation-email job. Same
// plaintext-token handling as the reset job (see its doc comment).
type SendInviteEmailArgs struct {
	TenantID     string `json:"tenantId"`
	InvitationID string `json:"invitationId"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Token        string `json:"token"`
}

// Kind is River's stable job identifier (persisted in river_job); never rename.
func (SendInviteEmailArgs) Kind() string { return "send_invite_email" }

// InsertOpts pins the retry ceiling, same policy as the offer send.
func (SendInviteEmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: maxSendAttempts}
}

// sendInviteEmailWorker delivers the invitation link. The garage's display
// name is loaded fresh from the tenant row (renames between invite and send
// are reflected); the role label is rendered in Bulgarian here because the
// email is a server-rendered, single-locale document like the offer email.
type sendInviteEmailWorker struct {
	river.WorkerDefaults[SendInviteEmailArgs]
	deps WorkerDeps
}

// roleNameBG maps an invitable role to its Bulgarian label for the email body.
func roleNameBG(role string) string {
	switch domain.Role(role) {
	case domain.RoleAdmin:
		return "администратор"
	case domain.RoleMechanic:
		return "механик"
	}
	return role
}

// Work performs one send attempt; River retries with backoff on error.
func (w *sendInviteEmailWorker) Work(ctx context.Context, job *river.Job[SendInviteEmailArgs]) error {
	args := job.Args

	tenant, err := w.deps.Tenants.GetByID(ctx, args.TenantID)
	if err != nil {
		return fmt.Errorf("load tenant: %w", err)
	}

	inviteURL := strings.TrimRight(w.deps.BaseURL, "/") + "/accept-invite?token=" + url.QueryEscape(args.Token)
	subject, html, text, err := mailer.InviteEmail(mailer.InviteEmailData{
		GarageName: tenant.Name,
		RoleName:   roleNameBG(args.Role),
		InviteURL:  inviteURL,
		ValidHours: int(domain.InvitationTokenTTL.Hours()),
	})
	if err != nil {
		return fmt.Errorf("render invite email: %w", err)
	}

	if err := w.deps.Mailer.Send(ctx, mailer.Message{
		To:      args.Email,
		Subject: subject,
		HTML:    html,
		Text:    text,
	}); err != nil {
		return fmt.Errorf("send invite email: %w", err)
	}
	w.deps.Logger.Info("invitation email sent", "invitationId", args.InvitationID, "tenantId", args.TenantID, "attempt", job.Attempt)
	return nil
}
