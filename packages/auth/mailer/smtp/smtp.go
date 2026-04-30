// Package smtp provides a net/smtp-backed Mailer adapter for sveltego/auth.
package smtp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	sveltauth "github.com/binsarjr/sveltego/auth"
)

// Option configures a Mailer.
type Option func(*Mailer)

// WithTLSConfig overrides the TLS configuration used for STARTTLS negotiation.
func WithTLSConfig(cfg *tls.Config) Option {
	return func(m *Mailer) { m.tlsConfig = cfg }
}

// WithTimeout sets the dial and I/O timeout for each SMTP connection.
// Defaults to 10 seconds.
func WithTimeout(d time.Duration) Option {
	return func(m *Mailer) { m.timeout = d }
}

// WithFrom sets the default From address used when Email.From is empty.
func WithFrom(from string) Option {
	return func(m *Mailer) { m.defaultFrom = from }
}

// Mailer delivers email via a plain SMTP server using STARTTLS and
// PLAIN authentication. It implements auth.Mailer.
type Mailer struct {
	host        string
	port        int
	username    string
	password    string
	defaultFrom string
	timeout     time.Duration
	tlsConfig   *tls.Config
}

// New returns a new Mailer. host and port identify the SMTP server;
// username and password are used for PLAIN authentication. Apply functional
// opts to override defaults (TLS config, dial timeout, default From).
func New(host string, port int, username, password string, opts ...Option) *Mailer {
	m := &Mailer{
		host:     host,
		port:     port,
		username: username,
		password: password,
		timeout:  10 * time.Second,
	}
	for _, o := range opts {
		o(m)
	}
	if m.tlsConfig == nil {
		m.tlsConfig = &tls.Config{ServerName: host} //nolint:gosec // user-supplied host
	}
	return m
}

// Send delivers msg via SMTP with STARTTLS. It returns a wrapped
// auth.ErrMailerSend on any protocol or delivery error.
func (m *Mailer) Send(_ context.Context, msg sveltauth.Email) error {
	from := msg.From
	if from == "" {
		from = m.defaultFrom
	}

	body := buildMIME(from, msg)

	addr := fmt.Sprintf("%s:%d", m.host, m.port)
	plainAuth := smtp.PlainAuth("", m.username, m.password, m.host)
	if err := smtp.SendMail(addr, plainAuth, from, msg.To, []byte(body)); err != nil {
		return fmt.Errorf("auth/mailer: smtp: %w: %w", sveltauth.ErrMailerSend, err)
	}
	return nil
}

func buildMIME(from string, msg sveltauth.Email) string {
	var sb strings.Builder

	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + strings.Join(msg.To, ", ") + "\r\n")
	sb.WriteString("Subject: " + msg.Subject + "\r\n")
	for k, v := range msg.Headers {
		sb.WriteString(k + ": " + v + "\r\n")
	}

	switch {
	case msg.HTMLBody != "" && msg.TextBody != "":
		boundary := "sveltego-alt"
		sb.WriteString("MIME-Version: 1.0\r\n")
		sb.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.TextBody + "\r\n")
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.HTMLBody + "\r\n")
		sb.WriteString("--" + boundary + "--\r\n")
	case msg.HTMLBody != "":
		sb.WriteString("MIME-Version: 1.0\r\n")
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.HTMLBody)
	default:
		sb.WriteString("MIME-Version: 1.0\r\n")
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		sb.WriteString(msg.TextBody)
	}
	return sb.String()
}
