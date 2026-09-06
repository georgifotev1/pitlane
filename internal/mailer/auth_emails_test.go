package mailer

import (
	"strings"
	"testing"
)

func TestWelcomeEmail(t *testing.T) {
	subject, html, text, err := WelcomeEmail(WelcomeEmailData{
		UserName: "Иван", GarageName: "Автосервиз Спийд", LoginURL: "https://pitlane.example/account/login",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"subject": subject, "html": html, "text": text} {
		if !strings.Contains(body, "Pitlane") {
			t.Errorf("%s does not identify Pitlane: %q", name, body)
		}
	}
	if !strings.Contains(html, "Автосервиз Спийд") || !strings.Contains(text, "https://pitlane.example/account/login") {
		t.Fatal("welcome email is missing garage name or login URL")
	}
}

func TestPasswordResetEmail(t *testing.T) {
	resetURL := "https://pitlane.example/account/reset-password?token=secret"
	subject, html, text, err := PasswordResetEmail(PasswordResetEmailData{UserName: "Иван", ResetURL: resetURL})
	if err != nil {
		t.Fatal(err)
	}
	if subject == "" || !strings.Contains(html, resetURL) || !strings.Contains(text, resetURL) {
		t.Fatal("password reset email is missing a subject or reset URL")
	}
	if strings.Contains(html, "offer") || strings.Contains(text, "оферта") {
		t.Fatal("account email must not contain offer-email content")
	}
}
