package auth_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/binsarjr/sveltego/auth"
)

func newTestCSRF() *auth.CSRF {
	return auth.NewCSRF(
		auth.WithCSRFAllowInsecure(),
	)
}

// issueToken is a helper that calls csrf.Issue and returns the token.
func issueToken(t *testing.T, csrf *auth.CSRF) (token string, cookies []*http.Cookie) {
	t.Helper()
	w := httptest.NewRecorder()
	tok, err := csrf.Issue(w)
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	resp := w.Result()
	defer resp.Body.Close()
	return tok, resp.Cookies()
}

func TestCSRF_IssueAndVerify_HappyPath(t *testing.T) {
	c := newTestCSRF()
	token, cookies := issueToken(t, c)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.Header.Set("X-CSRF-Token", token)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}

	if err := c.Verify(req); err != nil {
		t.Errorf("Verify returned unexpected error: %v", err)
	}
}

func TestCSRF_VerifyMissingCookie(t *testing.T) {
	c := newTestCSRF()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.Header.Set("X-CSRF-Token", "sometoken")
	// No cookie set.

	if err := c.Verify(req); !isCSRFInvalid(err) {
		t.Errorf("expected ErrCSRFInvalid, got %v", err)
	}
}

func TestCSRF_VerifyTokenMismatch(t *testing.T) {
	c := newTestCSRF()
	_, cookies := issueToken(t, c)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.Header.Set("X-CSRF-Token", "wrong-token")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}

	if err := c.Verify(req); !isCSRFInvalid(err) {
		t.Errorf("expected ErrCSRFInvalid, got %v", err)
	}
}

func TestCSRF_OriginNotTrusted(t *testing.T) {
	c := auth.NewCSRF(
		auth.WithCSRFAllowInsecure(),
		auth.WithTrustedOrigins("https://example.com"),
	)
	token, cookies := issueToken(t, c)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("X-CSRF-Token", token)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}

	if err := c.Verify(req); !isCSRFInvalid(err) {
		t.Errorf("expected ErrCSRFInvalid for untrusted origin, got %v", err)
	}
}

func TestCSRF_TrustedOriginAccepted(t *testing.T) {
	c := auth.NewCSRF(
		auth.WithCSRFAllowInsecure(),
		auth.WithTrustedOrigins("https://example.com"),
	)
	token, cookies := issueToken(t, c)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("X-CSRF-Token", token)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}

	if err := c.Verify(req); err != nil {
		t.Errorf("expected nil error for trusted origin, got %v", err)
	}
}

func TestCSRF_GETSkipped(t *testing.T) {
	c := newTestCSRF()
	// GET with no cookie or header — must pass through.
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)

	if err := c.Verify(req); err != nil {
		t.Errorf("Verify must not check GET requests, got %v", err)
	}
}

func TestCSRF_FieldFallsBack(t *testing.T) {
	c := newTestCSRF()
	token, cookies := issueToken(t, c)

	// POST via form field instead of header.
	form := url.Values{"csrf_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}

	if err := c.Verify(req); err != nil {
		t.Errorf("form-field fallback failed: %v", err)
	}
}

func TestCSRF_IssueConcurrent(t *testing.T) {
	c := newTestCSRF()
	var wg sync.WaitGroup
	const n = 8
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			_, errs[idx] = c.Issue(w)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Issue error: %v", i, err)
		}
	}
}

func isCSRFInvalid(err error) bool {
	return err != nil && err.Error() == auth.ErrCSRFInvalid.Error()
}
