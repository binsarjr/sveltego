package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

func csrfRoute(t *testing.T) []router.Route {
	t.Helper()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	return []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
		Options:  kit.DefaultPageOptions(),
	}}
}

func TestCSRF_GETIssuesCookie(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, csrfRoute(t))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var token string
	for _, c := range resp.Cookies() {
		if c.Name == kit.CSRFCookieName {
			token = c.Value
			break
		}
	}
	if token == "" {
		t.Fatalf("expected %s cookie on GET, got cookies: %v", kit.CSRFCookieName, resp.Cookies())
	}
}

func TestCSRF_POSTRejectsMissingToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, csrfRoute(t))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/login", url.Values{"email": {"alice@example.com"}})
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 403, body = %s", resp.StatusCode, body)
	}
}

func TestCSRF_POSTRejectsBadToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, csrfRoute(t))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	form := url.Values{kit.CSRFFieldName: {"wrong-token"}}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: kit.CSRFCookieName, Value: "real-token"})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestCSRF_POSTAcceptsMatchingToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, csrfRoute(t))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	const token = "match-token-value"
	form := url.Values{
		kit.CSRFFieldName: {token},
		"email":           {"alice@example.com"},
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: kit.CSRFCookieName, Value: token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body = %s", resp.StatusCode, body)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "form=ok") {
		t.Fatalf("expected action result in body: %s", body)
	}
}

func TestCSRF_POSTAcceptsHeaderToken(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, csrfRoute(t))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	const token = "header-token-value"
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/login", strings.NewReader("email=a@b.com"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", token)
	req.AddCookie(&http.Cookie{Name: kit.CSRFCookieName, Value: token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestCSRF_DisabledByPageOptions(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	opts := kit.DefaultPageOptions()
	opts.CSRF = false
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
		Options:  opts,
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/login", url.Values{"email": {"alice@example.com"}})
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("CSRF off: status = %d, want 200, body = %s", resp.StatusCode, body)
	}
}

func TestCSRF_RouteWithoutActionsSkipsCheck(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/about",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "about"}},
		Page:     staticPage("<h1>about</h1>"),
		Options:  kit.DefaultPageOptions(),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/about")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == kit.CSRFCookieName {
			t.Errorf("did not expect %s cookie on action-less route", kit.CSRFCookieName)
		}
	}
}

func TestCSRF_RenderCtxExposesToken(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	var capturedToken string
	pageHandler := func(w *render.Writer, ctx *kit.RenderCtx, _ any) error {
		capturedToken = ctx.CSRFToken()
		w.WriteString("<h1>login</h1>")
		return nil
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     pageHandler,
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
		Options:  kit.DefaultPageOptions(),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if capturedToken == "" {
		t.Fatalf("expected RenderCtx.CSRFToken() to be populated; got empty string")
	}
	var cookieToken string
	for _, c := range resp.Cookies() {
		if c.Name == kit.CSRFCookieName {
			cookieToken = c.Value
		}
	}
	if cookieToken != capturedToken {
		t.Errorf("RenderCtx token %q != cookie token %q", capturedToken, cookieToken)
	}
}
