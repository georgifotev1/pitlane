// Package jobs wires the River durable job queue (ADR decision 17) and hosts
// its workers. Phase 7 gives River its real debut: the SendOfferEmail job,
// enqueued transactionally alongside an offer's send-state change and worked
// off asynchronously (render PDF → send email → mark sent), with durable
// retries so a mid-send crash never drops a delivery.
package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// offerRenderer is the slice of pdf.Renderer the worker needs. Declared here
// (the consumer) so the worker can be tested with a fake that skips real PDF
// generation (ADR §106).
type offerRenderer interface {
	RenderOffer(pdf.OfferData) ([]byte, error)
}

// WorkerDeps are everything the workers need, injected from main. Stores load
// the offer graph; the renderer builds the attachment; the mailer delivers it.
// BaseURL is the public SPA origin emailed links point at (Phase 10 auth
// emails).
type WorkerDeps struct {
	Offers    *store.OfferStore
	Cars      *store.CarStore
	Customers *store.CustomerStore
	Tenants   *store.TenantStore
	PDF       offerRenderer
	Mailer    mailer.Mailer
	Logger    *slog.Logger
	BaseURL   string
}

// NewClient builds a working River client on the default queue, backed by the
// app's Postgres pool, with all workers registered. River requires its schema
// to be migrated before Start (see "api migrate up").
func NewClient(pool *pgxpool.Pool, deps WorkerDeps) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &sendOfferEmailWorker{deps: deps})
	river.AddWorker(workers, &sendPasswordResetEmailWorker{deps: deps})
	river.AddWorker(workers, &sendInviteEmailWorker{deps: deps})

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 10}},
		Workers: workers,
	})
	if err != nil {
		return nil, fmt.Errorf("river client: %w", err)
	}
	return client, nil
}

// NewInsertOnlyClient builds a client that can only enqueue jobs — no queues,
// no workers, never Started. It exists so a process (or a test) that needs the
// transactional-enqueue path without running jobs can still get an enqueuer.
func NewInsertOnlyClient(pool *pgxpool.Pool) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, fmt.Errorf("river insert-only client: %w", err)
	}
	return client, nil
}

// OfferEmailEnqueuer implements store.OfferEmailEnqueuer by inserting a
// SendOfferEmail job on the caller's transaction (River's InsertTx). Inserting
// on the same tx that flips the offer to sent is the transactional-enqueue
// guarantee: job and state change commit or roll back together.
type OfferEmailEnqueuer struct {
	client  *river.Client[pgx.Tx]
	replyTo string
}

// NewOfferEmailEnqueuer wraps a River client as an enqueuer.
func NewOfferEmailEnqueuer(client *river.Client[pgx.Tx]) *OfferEmailEnqueuer {
	return &OfferEmailEnqueuer{client: client}
}

// WithReplyTo returns an enqueuer that stamps replyTo onto the jobs it inserts.
// Reply-To is per-send (the emailing staff member), so the handler binds it
// here rather than baking a single address into the shared client. The return
// type is the store's consumer interface so callers stay decoupled from jobs.
func (e *OfferEmailEnqueuer) WithReplyTo(replyTo string) store.OfferEmailEnqueuer {
	clone := *e
	clone.replyTo = replyTo
	return &clone
}

// EnqueueOfferEmail inserts the send job on tx. The recipient is read from the
// offer row by the worker (sent_to), so only the reply-to travels in the args.
func (e *OfferEmailEnqueuer) EnqueueOfferEmail(ctx context.Context, tx pgx.Tx, tenantID, offerID string) error {
	_, err := e.client.InsertTx(ctx, tx, SendOfferEmailArgs{
		TenantID: tenantID,
		OfferID:  offerID,
		ReplyTo:  e.replyTo,
	}, nil)
	return err
}

// PasswordResetEnqueuer implements store.PasswordResetEmailEnqueuer by
// inserting a SendPasswordResetEmail job on the caller's transaction — the
// same transactional-enqueue guarantee as the offer send: token row and email
// dispatch commit or roll back together.
type PasswordResetEnqueuer struct {
	client *river.Client[pgx.Tx]
}

// NewPasswordResetEnqueuer wraps a River client as an enqueuer.
func NewPasswordResetEnqueuer(client *river.Client[pgx.Tx]) *PasswordResetEnqueuer {
	return &PasswordResetEnqueuer{client: client}
}

// EnqueuePasswordResetEmail inserts the reset-email job on tx.
func (e *PasswordResetEnqueuer) EnqueuePasswordResetEmail(ctx context.Context, tx pgx.Tx, tenantID, userID, email, userName, token string) error {
	_, err := e.client.InsertTx(ctx, tx, SendPasswordResetEmailArgs{
		TenantID: tenantID,
		UserID:   userID,
		Email:    email,
		UserName: userName,
		Token:    token,
	}, nil)
	return err
}

// InviteEnqueuer implements store.InviteEmailEnqueuer by inserting a
// SendInviteEmail job on the caller's transaction.
type InviteEnqueuer struct {
	client *river.Client[pgx.Tx]
}

// NewInviteEnqueuer wraps a River client as an enqueuer.
func NewInviteEnqueuer(client *river.Client[pgx.Tx]) *InviteEnqueuer {
	return &InviteEnqueuer{client: client}
}

// EnqueueInviteEmail inserts the invite-email job on tx.
func (e *InviteEnqueuer) EnqueueInviteEmail(ctx context.Context, tx pgx.Tx, tenantID, invitationID, email, role, token string) error {
	_, err := e.client.InsertTx(ctx, tx, SendInviteEmailArgs{
		TenantID:     tenantID,
		InvitationID: invitationID,
		Email:        email,
		Role:         role,
		Token:        token,
	}, nil)
	return err
}
