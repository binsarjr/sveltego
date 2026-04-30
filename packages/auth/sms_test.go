package auth_test

import (
	"context"
	"testing"

	"github.com/binsarjr/sveltego/auth"
)

func TestNoopSMSSender_RecordsCall(t *testing.T) {
	s := auth.NewNoopSMSSender()

	if err := s.Send(context.Background(), "+15551234567", "Your code is 123456"); err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}

	sent := s.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 recorded SMS, got %d", len(sent))
	}
	if sent[0].To != "+15551234567" {
		t.Errorf("recorded To = %q, want %q", sent[0].To, "+15551234567")
	}
	if sent[0].Body != "Your code is 123456" {
		t.Errorf("recorded Body = %q, want %q", sent[0].Body, "Your code is 123456")
	}
}

func TestNoopSMSSender_MultipleCallsAccumulate(t *testing.T) {
	s := auth.NewNoopSMSSender()
	ctx := context.Background()

	for i := range 5 {
		_ = i
		_ = s.Send(ctx, "+15550000001", "msg")
	}
	if got := len(s.Sent()); got != 5 {
		t.Errorf("expected 5 recorded SMS, got %d", got)
	}
}

func TestNoopSMSSender_NeverErrors(t *testing.T) {
	s := auth.NewNoopSMSSender()
	if err := s.Send(context.Background(), "", ""); err != nil {
		t.Errorf("Send returned error: %v", err)
	}
}
