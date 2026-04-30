// Package twilio provides a Twilio Programmable SMS adapter for sveltego/auth.
package twilio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	sveltauth "github.com/binsarjr/sveltego/auth"
)

const defaultBaseURL = "https://api.twilio.com"

// e164RE matches E.164 phone numbers: + followed by 7–15 digits.
var e164RE = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

// Option configures a Sender.
type Option func(*Sender)

// WithHTTPClient replaces the default http.Client.
func WithHTTPClient(c *http.Client) Option {
	return func(t *Sender) { t.client = c }
}

// WithBaseURL overrides the Twilio API base URL. Useful in tests.
func WithBaseURL(u string) Option {
	return func(t *Sender) { t.baseURL = u }
}

// Sender delivers SMS via the Twilio Programmable Messaging REST API.
// It implements auth.SMSSender.
type Sender struct {
	accountSID string
	authToken  string
	fromNumber string
	baseURL    string
	client     *http.Client
}

// New returns a new Sender. accountSID and authToken are your Twilio
// credentials; fromNumber must be an E.164 Twilio number or messaging
// service SID. Apply functional opts to override defaults.
func New(accountSID, authToken, fromNumber string, opts ...Option) *Sender {
	t := &Sender{
		accountSID: accountSID,
		authToken:  authToken,
		fromNumber: fromNumber,
		baseURL:    defaultBaseURL,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// Send delivers an SMS to the E.164 number to with the given body. It
// validates that to conforms to E.164 format before dispatching. Returns a
// wrapped auth.ErrSMSSend on validation or API error.
func (t *Sender) Send(ctx context.Context, to, body string) error {
	if !e164RE.MatchString(to) {
		return fmt.Errorf("auth/sms: twilio: %w: recipient %q is not a valid E.164 number", sveltauth.ErrSMSSend, to)
	}

	endpoint := fmt.Sprintf(
		"%s/2010-04-01/Accounts/%s/Messages.json",
		t.baseURL,
		t.accountSID,
	)

	form := url.Values{}
	form.Set("To", to)
	form.Set("From", t.fromNumber)
	form.Set("Body", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("auth/sms: twilio: %w: %w", sveltauth.ErrSMSSend, err)
	}
	req.SetBasicAuth(t.accountSID, t.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("auth/sms: twilio: %w: %w", sveltauth.ErrSMSSend, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("auth/sms: twilio: %w: unexpected status %d", sveltauth.ErrSMSSend, resp.StatusCode)
	}
	return nil
}
