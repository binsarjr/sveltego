package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/runtime/router"
)

// enhanceRoute returns a /login route with CSRF disabled so individual
// tests can focus on the enhance JSON envelope without juggling tokens.
// CSRF + enhance interactions live in TestEnhance_CSRFForbidden.
func enhanceRoute(t *testing.T, action kit.ActionFn) []router.Route {
	t.Helper()
	opts := kit.DefaultPageOptions()
	opts.CSRF = false
	return []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions: func() any {
			return kit.ActionMap{"default": action}
		},
		Options: opts,
	}}
}

func postEnhance(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(EnhanceHeader, "1")
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	return resp
}

func TestEnhance_SuccessReturnsJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, enhanceRoute(t, func(_ *kit.RequestEvent) kit.ActionResult {
		return kit.ActionDataResult(200, map[string]string{"msg": "saved"})
	}))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp := postEnhance(t, ts.URL+"/login", url.Values{"email": {"a@b.com"}}.Encode())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body = %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var env enhanceEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Type != "success" {
		t.Errorf("Type = %q, want success", env.Type)
	}
	if env.Status != 200 {
		t.Errorf("Status = %d, want 200", env.Status)
	}
	data, ok := env.Data.(map[string]any)
	if !ok || data["msg"] != "saved" {
		t.Errorf("Data = %v, want map with msg=saved", env.Data)
	}
}

func TestEnhance_FailureReturnsFailureEnvelope(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, enhanceRoute(t, func(_ *kit.RequestEvent) kit.ActionResult {
		return kit.ActionFail(422, map[string]string{"field": "required"})
	}))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp := postEnhance(t, ts.URL+"/login", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200 (envelope status carries the action code)", resp.StatusCode)
	}
	var env enhanceEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if env.Type != "failure" {
		t.Errorf("Type = %q, want failure", env.Type)
	}
	if env.Status != 422 {
		t.Errorf("Status = %d, want 422", env.Status)
	}
}

func TestEnhance_RedirectReturnsRedirectEnvelope(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, enhanceRoute(t, func(_ *kit.RequestEvent) kit.ActionResult {
		return kit.ActionRedirect(303, "/dashboard")
	}))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp := postEnhance(t, ts.URL+"/login", "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200 (must not be 3xx — fetch would auto-follow)", resp.StatusCode)
	}
	var env enhanceEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if env.Type != "redirect" {
		t.Errorf("Type = %q, want redirect", env.Type)
	}
	if env.Location != "/dashboard" {
		t.Errorf("Location = %q, want /dashboard", env.Location)
	}
}

func TestEnhance_CSRFForbidden(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     formAwarePage(),
		Load:     formAwareLoad(),
		Actions:  func() any { return actions },
		Options:  kit.DefaultPageOptions(),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp := postEnhance(t, ts.URL+"/login", "email=a@b.com")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d, want 200", resp.StatusCode)
	}
	var env enhanceEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if env.Type != "error" {
		t.Errorf("Type = %q, want error", env.Type)
	}
	if env.Status != http.StatusForbidden {
		t.Errorf("Status = %d, want 403", env.Status)
	}
}

func TestEnhance_NoHeaderFallsThroughToHTML(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, enhanceRoute(t, func(_ *kit.RequestEvent) kit.ActionResult {
		return kit.ActionDataResult(200, "ok")
	}))
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.PostForm(ts.URL+"/login", url.Values{"email": {"a@b.com"}})
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html (no enhance header → full HTML)", ct)
	}
}
