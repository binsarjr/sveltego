package auth

import (
	"context"
	"sync"
)

// Email holds the fields for a single outbound email message.
type Email struct {
	// From is the sender address (e.g. "noreply@example.com"). When empty,
	// the adapter's configured default From address is used.
	From string

	// To is the list of recipient addresses. Must contain at least one entry.
	To []string

	// Subject is the email subject line.
	Subject string

	// TextBody is the plain-text version of the message body.
	TextBody string

	// HTMLBody is the HTML version of the message body.
	HTMLBody string

	// Headers holds additional MIME headers to include in the message.
	Headers map[string]string
}

// Mailer is the contract for sending transactional email. A nil Mailer in
// Config disables all email-dependent flows (verification, password-reset,
// magic-link, OTP). Implementations must be safe for concurrent use.
type Mailer interface {
	// Send delivers the given Email. Implementations must return a wrapped
	// ErrMailerSend on delivery failure so callers can use errors.Is.
	Send(ctx context.Context, msg Email) error
}

// NoopMailer records every Send call in memory. It never fails and never
// transmits any email. Intended for tests and development environments.
type NoopMailer struct {
	mu   sync.Mutex
	sent []Email
}

// NewNoopMailer returns a new *NoopMailer with an empty call log.
func NewNoopMailer() *NoopMailer {
	return &NoopMailer{}
}

// Send records msg in the call log and returns nil.
func (m *NoopMailer) Send(_ context.Context, msg Email) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

// Sent returns a copy of all emails recorded since the NoopMailer was created.
func (m *NoopMailer) Sent() []Email {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Email, len(m.sent))
	copy(out, m.sent)
	return out
}
