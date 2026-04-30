// Package resend provides a Resend HTTP API Mailer adapter for sveltego/auth.
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sveltauth "github.com/binsarjr/sveltego/auth"
)

const defaultBaseURL = "https://api.resend.com"

// Option configures a Mailer.
type Option func(*Mailer)

// WithHTTPClient replaces the default http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(m *Mailer) { m.client = c }
}

// WithBaseURL overrides the Resend API base URL. Useful in tests.
func WithBaseURL(u string) Option {
	return func(m *Mailer) { m.baseURL = u }
}

// WithFrom sets the default From address used when Email.From is empty.
func WithFrom(from string) Option {
	return func(m *Mailer) { m.defaultFrom = from }
}

// Mailer delivers email via the Resend HTTP API. It implements auth.Mailer.
type Mailer struct {
	apiKey      string
	defaultFrom string
	baseURL     string
	client      *http.Client
}

// New returns a new Mailer authenticated with apiKey. Apply functional
// opts to override defaults (HTTP client, base URL, default From).
func New(apiKey string, opts ...Option) *Mailer {
	m := &Mailer{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

type resendPayload struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	Subject string            `json:"subject"`
	Text    string            `json:"text,omitempty"`
	HTML    string            `json:"html,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// Send delivers msg via the Resend API. It returns a wrapped
// auth.ErrMailerSend on any HTTP or API error.
func (m *Mailer) Send(ctx context.Context, msg sveltauth.Email) error {
	from := msg.From
	if from == "" {
		from = m.defaultFrom
	}

	payload := resendPayload{
		From:    from,
		To:      msg.To,
		Subject: msg.Subject,
		Text:    msg.TextBody,
		HTML:    msg.HTMLBody,
		Headers: msg.Headers,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("auth/mailer: resend: %w: %w", sveltauth.ErrMailerSend, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("auth/mailer: resend: %w: %w", sveltauth.ErrMailerSend, err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("auth/mailer: resend: %w: %w", sveltauth.ErrMailerSend, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("auth/mailer: resend: %w: unexpected status %d", sveltauth.ErrMailerSend, resp.StatusCode)
	}
	return nil
}
