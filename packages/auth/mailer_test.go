package auth_test

import (
	"context"
	"testing"

	"github.com/binsarjr/sveltego/auth"
)

func TestNoopMailer_RecordsCall(t *testing.T) {
	m := auth.NewNoopMailer()

	msg := auth.Email{
		From:     "sender@example.com",
		To:       []string{"recipient@example.com"},
		Subject:  "Hello",
		TextBody: "Hello, world!",
	}

	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}

	sent := m.Sent()
	if len(sent) != 1 {
		t.Fatalf("expected 1 recorded email, got %d", len(sent))
	}
	if sent[0].Subject != msg.Subject {
		t.Errorf("recorded subject = %q, want %q", sent[0].Subject, msg.Subject)
	}
	if sent[0].To[0] != msg.To[0] {
		t.Errorf("recorded To = %q, want %q", sent[0].To[0], msg.To[0])
	}
}

func TestNoopMailer_MultipleCallsAccumulate(t *testing.T) {
	m := auth.NewNoopMailer()
	ctx := context.Background()

	for i := range 3 {
		_ = i
		_ = m.Send(ctx, auth.Email{To: []string{"a@b.com"}, Subject: "x"})
	}
	if got := len(m.Sent()); got != 3 {
		t.Errorf("expected 3 recorded emails, got %d", got)
	}
}

func TestNoopMailer_NeverErrors(t *testing.T) {
	m := auth.NewNoopMailer()
	if err := m.Send(context.Background(), auth.Email{}); err != nil {
		t.Errorf("Send on empty Email returned error: %v", err)
	}
}
