package mailer

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

// Password-reset and invitation emails (Phase 10). Bulgarian-only, like the
// offer email: server-rendered documents with a single fixed locale. Templates
// are embedded with the offer templates via the same templateFS.

var (
	resetHTMLTemplate = htmltemplate.Must(htmltemplate.ParseFS(templateFS, "templates/password_reset_email.html.tmpl"))
	resetTextTemplate = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/password_reset_email.txt.tmpl"))
	inviteHTMLTemplate = htmltemplate.Must(htmltemplate.ParseFS(templateFS, "templates/invite_email.html.tmpl"))
	inviteTextTemplate = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/invite_email.txt.tmpl"))
)

// PasswordResetEmailData is what the reset templates render. ResetURL is the
// full SPA link (base URL + token) built by the caller.
type PasswordResetEmailData struct {
	UserName string
	ResetURL string
}

// PasswordResetEmail renders the reset email as subject + HTML + plain text.
func PasswordResetEmail(data PasswordResetEmailData) (subject, html, text string, err error) {
	var htmlBuf, textBuf bytes.Buffer
	if err := resetHTMLTemplate.ExecuteTemplate(&htmlBuf, "password_reset_email.html.tmpl", data); err != nil {
		return "", "", "", fmt.Errorf("render reset email html: %w", err)
	}
	if err := resetTextTemplate.ExecuteTemplate(&textBuf, "password_reset_email.txt.tmpl", data); err != nil {
		return "", "", "", fmt.Errorf("render reset email text: %w", err)
	}
	return "Нулиране на паролата Ви в pitlane", htmlBuf.String(), textBuf.String(), nil
}

// InviteEmailData is what the invitation templates render. InviteURL is the
// full SPA accept link; RoleName is the Bulgarian role label; ValidHours is
// the token lifetime shown to the invitee.
type InviteEmailData struct {
	GarageName string
	RoleName   string
	InviteURL  string
	ValidHours int
}

// InviteEmail renders the invitation email as subject + HTML + plain text.
func InviteEmail(data InviteEmailData) (subject, html, text string, err error) {
	var htmlBuf, textBuf bytes.Buffer
	if err := inviteHTMLTemplate.ExecuteTemplate(&htmlBuf, "invite_email.html.tmpl", data); err != nil {
		return "", "", "", fmt.Errorf("render invite email html: %w", err)
	}
	if err := inviteTextTemplate.ExecuteTemplate(&textBuf, "invite_email.txt.tmpl", data); err != nil {
		return "", "", "", fmt.Errorf("render invite email text: %w", err)
	}
	subject = fmt.Sprintf("Покана за екипа на %s в pitlane", data.GarageName)
	return subject, htmlBuf.String(), textBuf.String(), nil
}
