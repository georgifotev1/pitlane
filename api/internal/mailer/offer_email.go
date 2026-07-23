package mailer

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

// The offer email body is Bulgarian-only (Phase 2.5): the SPA Lingui catalog
// covers the browser UI, but this is a server-rendered document with a single
// fixed locale, so the strings live here beside the templates.
//
//go:embed templates/offer_email.html.tmpl templates/offer_email.txt.tmpl templates/password_reset_email.html.tmpl templates/password_reset_email.txt.tmpl templates/invite_email.html.tmpl templates/invite_email.txt.tmpl
var templateFS embed.FS

// Parsed once at init and reused (templates are read-only, so this is
// concurrency-safe across workers). html/template auto-escapes the HTML body;
// text/template keeps the plain-text alternative verbatim.
var (
	offerHTMLTemplate = htmltemplate.Must(htmltemplate.ParseFS(templateFS, "templates/offer_email.html.tmpl"))
	offerTextTemplate = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/offer_email.txt.tmpl"))
)

// OfferEmailData is what the offer email templates render. Total is preformatted
// (bg-BG) by the caller via pdf.FormatMoney, so this package carries no currency
// logic and the email figure matches the PDF exactly.
type OfferEmailData struct {
	GarageName   string
	CustomerName string
	OfferNumber  string
	Total        string
}

// OfferEmail renders the Bulgarian offer email as subject + HTML + plain text.
// The PDF is attached by the caller (the worker), not here.
func OfferEmail(data OfferEmailData) (subject, html, text string, err error) {
	var htmlBuf, textBuf bytes.Buffer
	if err := offerHTMLTemplate.ExecuteTemplate(&htmlBuf, "offer_email.html.tmpl", data); err != nil {
		return "", "", "", fmt.Errorf("render offer email html: %w", err)
	}
	if err := offerTextTemplate.ExecuteTemplate(&textBuf, "offer_email.txt.tmpl", data); err != nil {
		return "", "", "", fmt.Errorf("render offer email text: %w", err)
	}
	subject = fmt.Sprintf("Оферта № %s от %s", data.OfferNumber, data.GarageName)
	return subject, htmlBuf.String(), textBuf.String(), nil
}
