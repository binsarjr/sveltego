// Package sendgrid provides a SendGrid HTTP API Mailer adapter for sveltego/auth.
package sendgrid

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	sveltauth "github.com/binsarjr/sveltego/auth"
)

const defaultBaseURL = "https://api.sendgrid.com"

// Option configures a Mailer.
type Option func(*Mailer)

// WithHTTPClient replaces the default http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(m *Mailer) { m.client = c }
}

// WithBaseURL overrides the SendGrid API base URL. Useful in tests.
func WithBaseURL(u string) Option {
	return func(m *Mailer) { m.baseURL = u }
}

// WithFrom sets the default From address used when Email.From is empty.
func WithFrom(from string) Option {
	return func(m *Mailer) { m.defaultFrom = from }
}

// Mailer delivers email via the SendGrid v3 HTTP API. It implements auth.Mailer.
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

type sgAddress struct {
	Email string `json:"email"`
}

type sgContent struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type sgPersonalization struct {
	To      []sgAddress `json:"to"`
	Subject string      `json:"subject"`
}

type sgPayload struct {
	From             sgAddress           `json:"from"`
	Personalizations []sgPersonalization `json:"personalizations"`
	Content          []sgContent         `json:"content"`
	Headers          map[string]string   `json:"headers,omitempty"`
}

// Send delivers msg via the SendGrid v3 API. It returns a wrapped
// auth.ErrMailerSend on any HTTP or API error.
func (m *Mailer) Send(ctx context.Context, msg sveltauth.Email) error {
	from := msg.From
	if from == "" {
		from = m.defaultFrom
	}

	to := make([]sgAddress, len(msg.To))
	for i, addr := range msg.To {
		to[i] = sgAddress{Email: addr}
	}

	var content []sgContent
	if msg.TextBody != "" {
		content = append(content, sgContent{Type: "text/plain", Value: msg.TextBody})
	}
	if msg.HTMLBody != "" {
		content = append(content, sgContent{Type: "text/html", Value: msg.HTMLBody})
	}

	payload := sgPayload{
		From: sgAddress{Email: from},
		Personalizations: []sgPersonalization{
			{To: to, Subject: msg.Subject},
		},
		Content: content,
		Headers: msg.Headers,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("auth/mailer: sendgrid: %w: %w", sveltauth.ErrMailerSend, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v3/mail/send", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("auth/mailer: sendgrid: %w: %w", sveltauth.ErrMailerSend, err)
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("auth/mailer: sendgrid: %w: %w", sveltauth.ErrMailerSend, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("auth/mailer: sendgrid: %w: unexpected status %d", sveltauth.ErrMailerSend, resp.StatusCode)
	}
	return nil
}
