package sendgrid_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	sveltauth "github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/mailer/sendgrid"
)

func TestSendgridMailer_Send_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer sg-key" {
			t.Errorf("unexpected Authorization: %s", r.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) == 0 {
			t.Error("expected non-empty body")
		}
		// SendGrid returns 202 Accepted on success
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	m := sendgrid.New("sg-key",
		sendgrid.WithBaseURL(srv.URL),
		sendgrid.WithHTTPClient(srv.Client()),
	)

	err := m.Send(context.Background(), sveltauth.Email{
		From:     "sender@example.com",
		To:       []string{"recv@example.com"},
		Subject:  "Hello",
		HTMLBody: "<p>Hi</p>",
	})
	if err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}
}

func TestSendgridMailer_Send_NonOK_WrapsErrMailerSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	m := sendgrid.New("bad",
		sendgrid.WithBaseURL(srv.URL),
		sendgrid.WithHTTPClient(srv.Client()),
	)

	err := m.Send(context.Background(), sveltauth.Email{
		From:    "a@b.com",
		To:      []string{"c@d.com"},
		Subject: "x",
	})
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	if !errors.Is(err, sveltauth.ErrMailerSend) {
		t.Errorf("expected errors.Is(err, ErrMailerSend); err = %v", err)
	}
}

func TestSendgridMailer_DefaultFrom(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	m := sendgrid.New("key",
		sendgrid.WithBaseURL(srv.URL),
		sendgrid.WithHTTPClient(srv.Client()),
		sendgrid.WithFrom("default@example.com"),
	)

	_ = m.Send(context.Background(), sveltauth.Email{
		To:      []string{"b@example.com"},
		Subject: "hi",
	})

	if !called {
		t.Error("expected stub server to be called")
	}
}
