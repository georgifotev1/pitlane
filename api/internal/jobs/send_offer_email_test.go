package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// captureMailer records what would have been sent (or fails on demand) so the
// worker can be tested without a real SMTP server.
type captureMailer struct {
	sent []mailer.Message
	err  error
}

func (m *captureMailer) Send(_ context.Context, msg mailer.Message) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, msg)
	return nil
}

// noopEnqueuer satisfies store.OfferEmailEnqueuer for MarkSending in tests that
// only need the state change, not a real River insert.
type noopEnqueuer struct{}

func (noopEnqueuer) EnqueueOfferEmail(context.Context, pgx.Tx, string, string) error { return nil }

type workerFixture struct {
	ctx     context.Context
	tenant  *domain.Tenant
	offers  *store.OfferStore
	makeDep func(m mailer.Mailer) WorkerDeps
}

// setupWorker builds a tenant → customer → car and returns a fixture plus a
// helper to create a fresh sent-and-pending offer per subtest.
func setupWorker(t *testing.T) (*workerFixture, func(t *testing.T) *domain.Offer) {
	t.Helper()
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := store.NewDB(tdb.Pool)
	ctx := context.Background()

	tenants := store.NewTenantStore(db)
	customers := store.NewCustomerStore(db)
	cars := store.NewCarStore(db)
	offers := store.NewOfferStore(db)

	tenant := &domain.Tenant{ID: uuid.NewString(), Name: "Автосервиз Спийд", Currency: "EUR", Locale: "bg", DefaultTaxRate: 1900, Settings: map[string]any{}}
	if err := tenants.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	customer := &domain.Customer{ID: uuid.NewString(), TenantID: tenant.ID, Name: "Иван Петров", Email: "ivan@example.com"}
	if err := customers.Create(ctx, customer); err != nil {
		t.Fatalf("create customer: %v", err)
	}
	car := &domain.Car{ID: uuid.NewString(), TenantID: tenant.ID, CustomerID: customer.ID, Plate: "CB1234AB", Make: "VW", Model: "Golf", Year: 2018}
	if err := cars.Create(ctx, car); err != nil {
		t.Fatalf("create car: %v", err)
	}

	fx := &workerFixture{
		ctx:    ctx,
		tenant: tenant,
		offers: offers,
		makeDep: func(m mailer.Mailer) WorkerDeps {
			return WorkerDeps{
				Offers:    offers,
				Cars:      cars,
				Customers: customers,
				Tenants:   tenants,
				PDF:       pdf.NewRenderer(),
				Mailer:    m,
				Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
			}
		},
	}

	newSentOffer := func(t *testing.T) *domain.Offer {
		t.Helper()
		o := &domain.Offer{
			ID: uuid.NewString(), TenantID: tenant.ID, CarID: car.ID, TaxRateBps: 1900,
			Items: []domain.OfferItem{
				{Kind: domain.OfferItemKindPart, Description: "Спирачни накладки", Quantity: 2, UnitPriceCents: 4500},
				{Kind: domain.OfferItemKindLabor, Description: "Монтаж", Quantity: 1, UnitPriceCents: 6000},
			},
		}
		if err := offers.Create(ctx, o); err != nil {
			t.Fatalf("create offer: %v", err)
		}
		if _, err := offers.MarkSending(ctx, tenant.ID, o.ID, "ivan@example.com", noopEnqueuer{}); err != nil {
			t.Fatalf("mark sending: %v", err)
		}
		return o
	}

	return fx, newSentOffer
}

func testJob(fx *workerFixture, offerID string, attempt int) *river.Job[SendOfferEmailArgs] {
	return &river.Job[SendOfferEmailArgs]{
		JobRow: &rivertype.JobRow{Kind: SendOfferEmailArgs{}.Kind(), Attempt: attempt, MaxAttempts: maxSendAttempts},
		Args:   SendOfferEmailArgs{TenantID: fx.tenant.ID, OfferID: offerID, ReplyTo: "owner@garage.bg"},
	}
}

func TestSendOfferEmailWorker(t *testing.T) {
	fx, newSentOffer := setupWorker(t)

	t.Run("renders the PDF, mails it, and marks send_status sent", func(t *testing.T) {
		offer := newSentOffer(t)
		mail := &captureMailer{}
		worker := &sendOfferEmailWorker{deps: fx.makeDep(mail)}

		if err := worker.Work(fx.ctx, testJob(fx, offer.ID, 1)); err != nil {
			t.Fatalf("work: %v", err)
		}

		if len(mail.sent) != 1 {
			t.Fatalf("expected one email, got %d", len(mail.sent))
		}
		msg := mail.sent[0]
		if msg.To != "ivan@example.com" {
			t.Fatalf("wrong recipient: %q", msg.To)
		}
		if msg.ReplyTo != "owner@garage.bg" {
			t.Fatalf("wrong reply-to: %q", msg.ReplyTo)
		}
		if msg.Subject == "" || msg.HTML == "" || msg.Text == "" {
			t.Fatalf("empty subject/body: %+v", msg)
		}
		if len(msg.Attachments) != 1 {
			t.Fatalf("expected one attachment, got %d", len(msg.Attachments))
		}
		att := msg.Attachments[0]
		if att.ContentType != "application/pdf" || !bytes.HasPrefix(att.Content, []byte("%PDF-")) {
			t.Fatalf("attachment is not a PDF: type=%q prefix=%q", att.ContentType, att.Content[:min(5, len(att.Content))])
		}

		got, err := fx.offers.Get(fx.ctx, fx.tenant.ID, offer.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.SendStatus != domain.SendStatusSent || got.SentAt == nil {
			t.Fatalf("not marked sent: send=%s sentAt=%v", got.SendStatus, got.SentAt)
		}
	})

	t.Run("final-attempt failure records send_status failed", func(t *testing.T) {
		offer := newSentOffer(t)
		mail := &captureMailer{err: errors.New("smtp unreachable")}
		worker := &sendOfferEmailWorker{deps: fx.makeDep(mail)}

		// Attempt == MaxAttempts: this is the last try.
		if err := worker.Work(fx.ctx, testJob(fx, offer.ID, maxSendAttempts)); err == nil {
			t.Fatal("expected work to fail")
		}
		got, err := fx.offers.Get(fx.ctx, fx.tenant.ID, offer.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.SendStatus != domain.SendStatusFailed {
			t.Fatalf("expected failed after last attempt, got %s", got.SendStatus)
		}
	})

	t.Run("non-final failure retries without marking failed", func(t *testing.T) {
		offer := newSentOffer(t)
		mail := &captureMailer{err: errors.New("smtp blip")}
		worker := &sendOfferEmailWorker{deps: fx.makeDep(mail)}

		// Attempt 1 of maxSendAttempts: River will retry, so the status must
		// stay pending (not prematurely failed).
		if err := worker.Work(fx.ctx, testJob(fx, offer.ID, 1)); err == nil {
			t.Fatal("expected work to fail")
		}
		got, err := fx.offers.Get(fx.ctx, fx.tenant.ID, offer.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.SendStatus != domain.SendStatusPending {
			t.Fatalf("expected still pending during retries, got %s", got.SendStatus)
		}
	})
}
