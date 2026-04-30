package smtp_test

import (
	"crypto/tls"
	"testing"
	"time"

	smtpadapter "github.com/binsarjr/sveltego/auth/mailer/smtp"
)

func TestNew_DefaultOptions(t *testing.T) {
	m := smtpadapter.New("mail.example.com", 587, "user", "pass")
	if m == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_WithOptions(t *testing.T) {
	cfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // test only
	m := smtpadapter.New("mail.example.com", 587, "user", "pass",
		smtpadapter.WithTLSConfig(cfg),
		smtpadapter.WithTimeout(30*time.Second),
		smtpadapter.WithFrom("noreply@example.com"),
	)
	if m == nil {
		t.Fatal("New with options returned nil")
	}
}

// Full SMTP protocol round-trip is tested against a live mailhog instance
// (testcontainer) in the integration test suite. Skip here to avoid
// requiring a network listener in the unit test run.
