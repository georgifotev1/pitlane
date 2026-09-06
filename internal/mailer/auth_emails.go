package mailer

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var (
	welcomeHTMLTemplate = htmltemplate.Must(htmltemplate.ParseFS(templateFS, "templates/welcome_email.html.tmpl"))
	welcomeTextTemplate = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/welcome_email.txt.tmpl"))
	resetHTMLTemplate   = htmltemplate.Must(htmltemplate.ParseFS(templateFS, "templates/password_reset_email.html.tmpl"))
	resetTextTemplate   = texttemplate.Must(texttemplate.ParseFS(templateFS, "templates/password_reset_email.txt.tmpl"))
)

type WelcomeEmailData struct {
	UserName   string
	GarageName string
	LoginURL   string
}

func WelcomeEmail(data WelcomeEmailData) (subject, html, text string, err error) {
	html, text, err = renderEmail(welcomeHTMLTemplate, welcomeTextTemplate, "welcome_email.html.tmpl", "welcome_email.txt.tmpl", data)
	if err != nil {
		return "", "", "", fmt.Errorf("render welcome email: %w", err)
	}
	return "Добре дошли в Pitlane", html, text, nil
}

type PasswordResetEmailData struct {
	UserName string
	ResetURL string
}

func PasswordResetEmail(data PasswordResetEmailData) (subject, html, text string, err error) {
	html, text, err = renderEmail(resetHTMLTemplate, resetTextTemplate, "password_reset_email.html.tmpl", "password_reset_email.txt.tmpl", data)
	if err != nil {
		return "", "", "", fmt.Errorf("render password reset email: %w", err)
	}
	return "Нулиране на паролата Ви в Pitlane", html, text, nil
}

func renderEmail(htmlTemplate *htmltemplate.Template, textTemplate *texttemplate.Template, htmlName, textName string, data any) (string, string, error) {
	var htmlBuf, textBuf bytes.Buffer
	if err := htmlTemplate.ExecuteTemplate(&htmlBuf, htmlName, data); err != nil {
		return "", "", err
	}
	if err := textTemplate.ExecuteTemplate(&textBuf, textName, data); err != nil {
		return "", "", err
	}
	return htmlBuf.String(), textBuf.String(), nil
}
