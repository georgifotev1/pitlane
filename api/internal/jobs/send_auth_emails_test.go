package jobs

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// authWorkerFixture builds a tenant and returns a WorkerDeps factory with the
// given mailer, mirroring setupWorker for the offer tests.
func authWorkerFixture(t *testing.T) (*domain.Tenant, func(m mailer.Mailer) WorkerDeps) {
	t.Helper()
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := store.NewDB(tdb.Pool)
	ctx := context.Background()
	tenants := store.NewTenantStore(db)

	tenant := &domain.Tenant{ID: uuid.NewString(), Name: "Автосервиз Спийд", Currency: "EUR", Locale: "bg", DefaultTaxRate: 1900, Settings: map[string]any{}}
	if err := tenants.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	return tenant, func(m mailer.Mailer) WorkerDeps {
		return WorkerDeps{
			Tenants: tenants,
			Mailer:  m,
			Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
			BaseURL: "http://localhost:5173/",
		}
	}
}

func TestSendPasswordResetEmailWorker(t *testing.T) {
	_, makeDeps := authWorkerFixture(t)

	mail := &captureMailer{}
	worker := &sendPasswordResetEmailWorker{deps: makeDeps(mail)}

	job := &river.Job[SendPasswordResetEmailArgs]{
		JobRow: &rivertype.JobRow{Kind: SendPasswordResetEmailArgs{}.Kind(), Attempt: 1, MaxAttempts: maxSendAttempts},
		Args: SendPasswordResetEmailArgs{
			TenantID: "t1", UserID: "u1", Email: "lost@example.com", UserName: "Иван", Token: "tok-abc",
		},
	}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("work: %v", err)
	}

	if len(mail.sent) != 1 {
		t.Fatalf("expected one email, got %d", len(mail.sent))
	}
	msg := mail.sent[0]
	if msg.To != "lost@example.com" {
		t.Fatalf("wrong recipient: %q", msg.To)
	}
	if msg.Subject == "" || msg.HTML == "" || msg.Text == "" {
		t.Fatalf("empty subject/body: %+v", msg)
	}
	// The link points at the SPA reset route with the token, base URL trimmed
	// of its trailing slash.
	wantURL := "http://localhost:5173/reset-password?token=tok-abc"
	if !strings.Contains(msg.HTML, wantURL) || !strings.Contains(msg.Text, wantURL) {
		t.Fatalf("reset link missing from bodies.\nHTML: %s\nText: %s", msg.HTML, msg.Text)
	}
}

func TestSendInviteEmailWorker(t *testing.T) {
	tenant, makeDeps := authWorkerFixture(t)

	mail := &captureMailer{}
	worker := &sendInviteEmailWorker{deps: makeDeps(mail)}

	job := &river.Job[SendInviteEmailArgs]{
		JobRow: &rivertype.JobRow{Kind: SendInviteEmailArgs{}.Kind(), Attempt: 1, MaxAttempts: maxSendAttempts},
		Args: SendInviteEmailArgs{
			TenantID: tenant.ID, InvitationID: uuid.NewString(), Email: "new@example.com", Role: "mechanic", Token: "tok-inv",
		},
	}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("work: %v", err)
	}

	if len(mail.sent) != 1 {
		t.Fatalf("expected one email, got %d", len(mail.sent))
	}
	msg := mail.sent[0]
	if msg.To != "new@example.com" {
		t.Fatalf("wrong recipient: %q", msg.To)
	}
	if !strings.Contains(msg.Subject, tenant.Name) {
		t.Fatalf("subject missing garage name: %q", msg.Subject)
	}
	wantURL := "http://localhost:5173/accept-invite?token=tok-inv"
	if !strings.Contains(msg.HTML, wantURL) || !strings.Contains(msg.Text, wantURL) {
		t.Fatalf("invite link missing from bodies.\nHTML: %s\nText: %s", msg.HTML, msg.Text)
	}
	// The role renders as its Bulgarian label, not the enum value.
	if !strings.Contains(msg.Text, "механик") {
		t.Fatalf("Bulgarian role label missing: %s", msg.Text)
	}
	if !strings.Contains(msg.Text, "72") {
		t.Fatalf("validity hours missing: %s", msg.Text)
	}
}
