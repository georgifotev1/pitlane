package mailer

import (
	"context"
	"errors"
	"fmt"
	"time"

	mail "github.com/wneessen/go-mail"
)

type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

type Mailer interface {
	Send(context.Context, Message) error
}

type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

type SMTP struct {
	cfg Config
}

func NewSMTP(cfg Config) *SMTP { return &SMTP{cfg: cfg} }

func (s *SMTP) Send(ctx context.Context, msg Message) error {
	if s.cfg.Host == "" || s.cfg.From == "" {
		return errors.New("SMTP host and sender are required")
	}

	m := mail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return fmt.Errorf("set from: %w", err)
	}
	if err := m.To(msg.To); err != nil {
		return fmt.Errorf("set to: %w", err)
	}
	m.Subject(msg.Subject)
	m.SetBodyString(mail.TypeTextPlain, msg.Text)
	m.AddAlternativeString(mail.TypeTextHTML, msg.HTML)

	opts := []mail.Option{mail.WithPort(s.cfg.Port), mail.WithTimeout(10 * time.Second)}
	if s.cfg.Username == "" {
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	} else {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthAutoDiscover),
			mail.WithUsername(s.cfg.Username),
			mail.WithPassword(s.cfg.Password),
			mail.WithTLSPortPolicy(mail.TLSOpportunistic),
		)
	}
	client, err := mail.NewClient(s.cfg.Host, opts...)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, m); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}
