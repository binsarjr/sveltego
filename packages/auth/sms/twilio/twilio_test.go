package twilio_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sveltauth "github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/sms/twilio"
)

func TestTwilioSMS_Send_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "To=%2B15551234567") {
			t.Errorf("body did not contain encoded To: %s", body)
		}
		user, pass, ok := r.BasicAuth()
		if !ok {
			t.Error("expected basic auth")
		}
		if user != "ACTEST" || pass != "token" {
			t.Errorf("unexpected creds: %s / %s", user, pass)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	s := twilio.New("ACTEST", "token", "+18005550000",
		twilio.WithBaseURL(srv.URL),
		twilio.WithHTTPClient(srv.Client()),
	)

	err := s.Send(context.Background(), "+15551234567", "Your code is 999888")
	if err != nil {
		t.Fatalf("Send returned unexpected error: %v", err)
	}
}

func TestTwilioSMS_Send_NonOK_WrapsErrSMSSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	s := twilio.New("AC", "bad", "+18005550000",
		twilio.WithBaseURL(srv.URL),
		twilio.WithHTTPClient(srv.Client()),
	)

	err := s.Send(context.Background(), "+15559990000", "hi")
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	if !errors.Is(err, sveltauth.ErrSMSSend) {
		t.Errorf("expected errors.Is(err, ErrSMSSend); err = %v", err)
	}
}

func TestTwilioSMS_Send_InvalidE164_WrapsErrSMSSend(t *testing.T) {
	s := twilio.New("AC", "tok", "+18005550000")

	cases := []string{
		"5551234567",   // missing +
		"+155512345",   // too short (< 7 digits after country code)
		"+1 555 1234",  // spaces
		"not-a-number", // letters
	}

	for _, to := range cases {
		err := s.Send(context.Background(), to, "hi")
		if err == nil {
			t.Errorf("expected error for invalid E.164 %q", to)
			continue
		}
		if !errors.Is(err, sveltauth.ErrSMSSend) {
			t.Errorf("expected ErrSMSSend for %q; got %v", to, err)
		}
	}
}

func TestTwilioSMS_Options(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	s := twilio.New("AC", "tok", "+18001111111",
		twilio.WithBaseURL(srv.URL),
		twilio.WithHTTPClient(srv.Client()),
	)

	_ = s.Send(context.Background(), "+15557654321", "test")
	if !called {
		t.Error("stub server was not called")
	}
}
