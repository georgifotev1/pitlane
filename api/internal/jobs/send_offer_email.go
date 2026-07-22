package jobs

import (
	"context"
	"fmt"

	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/riverqueue/river"
)

// maxSendAttempts bounds how many times a send is retried before its
// send_status flips to failed and the UI offers a manual retry. River backs off
// between attempts, so a transient SMTP outage recovers on its own; a persistent
// failure (bad address, provider rejecting) settles into failed after this many.
const maxSendAttempts = 5

// SendOfferEmailArgs is the typed payload of the send job. The recipient is not
// carried here — it lives on the offer row (sent_to), frozen at send time — so
// the args stay minimal: which offer, in which tenant, and the per-send
// Reply-To (the staff member who sent it).
type SendOfferEmailArgs struct {
	TenantID string `json:"tenantId"`
	OfferID  string `json:"offerId"`
	ReplyTo  string `json:"replyTo"`
}

// Kind is River's stable job identifier (persisted in river_job); never rename.
func (SendOfferEmailArgs) Kind() string { return "send_offer_email" }

// InsertOpts pins the retry ceiling for every send job (JobArgsWithInsertOpts).
func (SendOfferEmailArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: maxSendAttempts}
}

// sendOfferEmailWorker renders an offer's PDF and emails it to the frozen
// recipient. It is the durable half of the send: MarkSending has already
// committed the offer as sent/pending in the same tx that enqueued this job, so
// the worker's job is delivery + recording the outcome.
type sendOfferEmailWorker struct {
	river.WorkerDefaults[SendOfferEmailArgs]
	deps WorkerDeps
}

// Work performs one send attempt. On success it records send_status=sent. On
// failure it returns the error so River retries with backoff; on the final
// attempt it also records send_status=failed so the UI can surface a retry.
func (w *sendOfferEmailWorker) Work(ctx context.Context, job *river.Job[SendOfferEmailArgs]) error {
	args := job.Args
	log := w.deps.Logger.With("job", job.Kind, "offerId", args.OfferID, "tenantId", args.TenantID, "attempt", job.Attempt)

	if err := w.send(ctx, args); err != nil {
		if job.Attempt >= job.MaxAttempts {
			// Last attempt: record the terminal failure. Best-effort — if this
			// write itself fails we still return the send error below.
			if merr := w.deps.Offers.MarkSendFailed(ctx, args.TenantID, args.OfferID); merr != nil {
				log.Error("record send failure", "err", merr)
			}
		}
		return fmt.Errorf("send offer email: %w", err)
	}

	// Delivery succeeded. If recording it fails we deliberately do NOT return an
	// error: retrying would re-send a duplicate email. The badge may briefly lag
	// at "pending" in the rare event of a DB hiccup right after a good send.
	if err := w.deps.Offers.MarkSent(ctx, args.TenantID, args.OfferID); err != nil {
		log.Error("record send success (email delivered, status left pending)", "err", err)
		return nil
	}
	log.Info("offer email sent")
	return nil
}

// send loads the offer graph, renders the PDF, and delivers it. It performs no
// state writes — the caller records the outcome — so a retry re-runs it cleanly.
func (w *sendOfferEmailWorker) send(ctx context.Context, args SendOfferEmailArgs) error {
	offer, err := w.deps.Offers.Get(ctx, args.TenantID, args.OfferID)
	if err != nil {
		return fmt.Errorf("load offer: %w", err)
	}
	car, err := w.deps.Cars.Get(ctx, args.TenantID, offer.CarID)
	if err != nil {
		return fmt.Errorf("load car: %w", err)
	}
	customer, err := w.deps.Customers.Get(ctx, args.TenantID, car.CustomerID)
	if err != nil {
		return fmt.Errorf("load customer: %w", err)
	}
	tenant, err := w.deps.Tenants.GetByID(ctx, args.TenantID)
	if err != nil {
		return fmt.Errorf("load tenant: %w", err)
	}

	doc, err := w.deps.PDF.RenderOffer(pdf.OfferData{
		Tenant:   tenant,
		Customer: customer,
		Car:      car,
		Offer:    offer,
	})
	if err != nil {
		return fmt.Errorf("render pdf: %w", err)
	}

	subject, html, text, err := mailer.OfferEmail(mailer.OfferEmailData{
		GarageName:   tenant.Name,
		CustomerName: customer.Name,
		OfferNumber:  pdf.ShortID(offer.ID),
		Total:        pdf.FormatMoney(offer.TotalCents, tenant.Currency),
	})
	if err != nil {
		return err
	}

	return w.deps.Mailer.Send(ctx, mailer.Message{
		To:      offer.SentTo,
		ReplyTo: args.ReplyTo,
		Subject: subject,
		HTML:    html,
		Text:    text,
		Attachments: []mailer.Attachment{{
			Filename:    "oferta-" + pdf.ShortID(offer.ID) + ".pdf",
			ContentType: "application/pdf",
			Content:     doc,
		}},
	})
}
