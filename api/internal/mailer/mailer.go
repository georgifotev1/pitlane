// Package mailer sends transactional email (ADR §14: go-mail SMTP behind a
// Mailer interface, Mailpit in dev, Resend in prod). Message is a
// transport-agnostic envelope so callers — the send-offer worker being the only
// one today — never touch go-mail types, and tests can substitute a fake that
// captures what would have been sent.
package mailer

import (
	"bytes"
	"context"
	"fmt"

	"github.com/wneessen/go-mail"
)

// Attachment is one file attached to a Message.
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// Message is a single email. From is fixed by the sender's configuration (the
// app domain), so it is not a per-message field — callers supply only what
// varies per send.
type Message struct {
	To          string
	ReplyTo     string // optional; empty means no Reply-To header
	Subject     string
	HTML        string
	Text        string // plain-text alternative
	Attachments []Attachment
}

// Mailer sends a Message. The send-offer worker is the sole consumer; SMTP is
// the real implementation and tests inject fakes.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// Config carries the SMTP settings lifted from the app config.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// SMTP is the go-mail-backed Mailer. One value is built at startup and shared
// across River workers; DialAndSendWithContext opens a fresh connection per
// send, which is fine at a garage's volume (a handful of offers a day) and
// keeps the value safe for concurrent use.
type SMTP struct {
	cfg Config
}

// NewSMTP builds an SMTP mailer from config.
func NewSMTP(cfg Config) *SMTP {
	return &SMTP{cfg: cfg}
}

// Send builds a go-mail message and delivers it. Plain text is the primary body
// with HTML as the richer alternative, so text-only clients still read cleanly.
// TLS/auth are chosen from whether a username is configured: Mailpit in dev
// needs neither; a real provider supplies credentials and gets STARTTLS.
func (s *SMTP) Send(ctx context.Context, msg Message) error {
	m := mail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return fmt.Errorf("set from: %w", err)
	}
	if err := m.To(msg.To); err != nil {
		return fmt.Errorf("set to: %w", err)
	}
	if msg.ReplyTo != "" {
		if err := m.ReplyTo(msg.ReplyTo); err != nil {
			return fmt.Errorf("set reply-to: %w", err)
		}
	}
	m.Subject(msg.Subject)
	m.SetBodyString(mail.TypeTextPlain, msg.Text)
	m.AddAlternativeString(mail.TypeTextHTML, msg.HTML)

	for _, att := range msg.Attachments {
		if err := m.AttachReader(att.Filename, bytes.NewReader(att.Content),
			mail.WithFileContentType(mail.ContentType(att.ContentType))); err != nil {
			return fmt.Errorf("attach %s: %w", att.Filename, err)
		}
	}

	opts := []mail.Option{mail.WithPort(s.cfg.Port)}
	if s.cfg.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(s.cfg.Username),
			mail.WithPassword(s.cfg.Password),
			mail.WithTLSPortPolicy(mail.TLSOpportunistic),
		)
	} else {
		// Mailpit in dev: no auth, plaintext on the SMTP port.
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	}

	client, err := mail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, m); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}
