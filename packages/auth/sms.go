package auth

import (
	"context"
	"sync"
)

// SMSSender is the contract for sending transactional SMS messages. A nil
// SMSSender in Config disables all SMS-dependent flows (OTP via SMS, SMS 2FA).
// Implementations must be safe for concurrent use.
type SMSSender interface {
	// Send delivers a text message to the E.164 phone number to with the
	// given body. Implementations must return a wrapped ErrSMSSend on
	// delivery failure so callers can use errors.Is.
	Send(ctx context.Context, to, body string) error
}

// SMSRecord holds the arguments of a single NoopSMSSender.Send call.
type SMSRecord struct {
	To   string
	Body string
}

// NoopSMSSender records every Send call in memory. It never fails and never
// transmits any SMS. Intended for tests and development environments.
type NoopSMSSender struct {
	mu   sync.Mutex
	sent []SMSRecord
}

// NewNoopSMSSender returns a new *NoopSMSSender with an empty call log.
func NewNoopSMSSender() *NoopSMSSender {
	return &NoopSMSSender{}
}

// Send records the call and returns nil.
func (s *NoopSMSSender) Send(_ context.Context, to, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, SMSRecord{To: to, Body: body})
	return nil
}

// Sent returns a copy of all SMS records since the NoopSMSSender was created.
func (s *NoopSMSSender) Sent() []SMSRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SMSRecord, len(s.sent))
	copy(out, s.sent)
	return out
}
