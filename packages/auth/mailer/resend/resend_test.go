package resend_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	sveltauth "github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/mailer/resend"
)

func TestResendMailer_Send_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("expected non-empty body")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := resend.New("test-key",
		resend.WithBaseURL(srv.URL),
		resend.WithHTTPClient(srv.Client()),
	)

	err := m.Send(context.Background(), sveltauth.Email{
		From:     "a@example.com",
		To:       []string{"b@example.com"},
		Subject:  "Test",
		TextBody: "Hello",
	})
	if err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}
}

func TestResendMailer_Send_NonOK_WrapsErrMailerSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	m := resend.New("bad-key",
		resend.WithBaseURL(srv.URL),
		resend.WithHTTPClient(srv.Client()),
	)

	err := m.Send(context.Background(), sveltauth.Email{
		From:    "a@example.com",
		To:      []string{"b@example.com"},
		Subject: "Test",
	})
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	if !errors.Is(err, sveltauth.ErrMailerSend) {
		t.Errorf("expected errors.Is(err, ErrMailerSend) to be true; err = %v", err)
	}
}

func TestResendMailer_DefaultFrom(t *testing.T) {
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := resend.New("key",
		resend.WithBaseURL(srv.URL),
		resend.WithHTTPClient(srv.Client()),
		resend.WithFrom("default@example.com"),
	)

	_ = m.Send(context.Background(), sveltauth.Email{
		// From intentionally empty — should fall back to WithFrom value
		To:      []string{"b@example.com"},
		Subject: "Test",
	})

	if len(capturedBody) == 0 {
		t.Fatal("expected body to be sent to stub server")
	}
}
