package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func newCSPServer(t *testing.T, csp kit.CSPConfig, routes []router.Route, hooks kit.Hooks) *Server {
	t.Helper()
	srv, err := New(Config{
		Routes: routes,
		Shell:  testShell,
		Logger: quietLogger(),
		Hooks:  hooks,
		CSP:    csp,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

func TestCSP_OffEmitsNoHeader(t *testing.T) {
	t.Parallel()
	srv := newCSPServer(t, kit.CSPConfig{}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("ok"),
	}}, kit.Hooks{})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if got := resp.Header.Get("Content-Security-Policy"); got != "" {
		t.Errorf("CSP header should be absent under CSPOff, got %q", got)
	}
	if got := resp.Header.Get("Content-Security-Policy-Report-Only"); got != "" {
		t.Errorf("report-only header should be absent under CSPOff, got %q", got)
	}
}

func TestCSP_StrictSetsHeaderAndNonce(t *testing.T) {
	t.Parallel()
	var observed string
	hooks := kit.Hooks{
		Handle: func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
			observed = kit.Nonce(ev)
			return resolve(ev)
		},
	}
	srv := newCSPServer(t, kit.CSPConfig{Mode: kit.CSPStrict}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("ok"),
	}}, hooks)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	header := resp.Header.Get("Content-Security-Policy")
	if header == "" {
		t.Fatal("Content-Security-Policy header missing under CSPStrict")
	}
	if observed == "" {
		t.Fatal("nonce not stored on ev.Locals")
	}
	if !strings.Contains(header, "'nonce-"+observed+"'") {
		t.Errorf("header %q missing nonce token %q", header, observed)
	}
	if !strings.Contains(header, "'strict-dynamic'") {
		t.Errorf("header missing strict-dynamic: %q", header)
	}
}

func TestCSP_NonceUniquePerRequest(t *testing.T) {
	t.Parallel()
	srv := newCSPServer(t, kit.CSPConfig{Mode: kit.CSPStrict}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("ok"),
	}}, kit.Hooks{})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	first, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET 1: %v", err)
	}
	defer first.Body.Close()
	second, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET 2: %v", err)
	}
	defer second.Body.Close()

	a := first.Header.Get("Content-Security-Policy")
	b := second.Header.Get("Content-Security-Policy")
	if a == "" || b == "" {
		t.Fatalf("missing header(s): %q %q", a, b)
	}
	if a == b {
		t.Errorf("nonces collided across requests:\n%s\n%s", a, b)
	}
}

func TestCSP_ReportOnlyUsesReportOnlyHeader(t *testing.T) {
	t.Parallel()
	srv := newCSPServer(t, kit.CSPConfig{Mode: kit.CSPReportOnly}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("ok"),
	}}, kit.Hooks{})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if got := resp.Header.Get("Content-Security-Policy"); got != "" {
		t.Errorf("enforcement header should be absent under report-only, got %q", got)
	}
	if got := resp.Header.Get("Content-Security-Policy-Report-Only"); got == "" {
		t.Error("report-only header should be set under CSPReportOnly")
	}
}

func TestCSP_HeaderPresentOnErrorPath(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		HandleError: func(_ *kit.RequestEvent, _ error) (kit.SafeError, error) {
			return kit.SafeError{Code: http.StatusBadGateway, Message: "upstream"}, nil
		},
	}
	srv := newCSPServer(t, kit.CSPConfig{Mode: kit.CSPStrict}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"),
		Page: staticPage("x"),
		Load: func(_ *kit.LoadCtx) (any, error) {
			return nil, errors.New("boom")
		},
	}}, hooks)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got == "" {
		t.Error("CSP header should be present on error responses")
	}
}

func TestCSP_NotFoundPathStillCarriesHeader(t *testing.T) {
	t.Parallel()
	srv := newCSPServer(t, kit.CSPConfig{Mode: kit.CSPStrict}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("ok"),
	}}, kit.Hooks{})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/missing")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got == "" {
		t.Error("CSP header should be present on 404 responses")
	}
}

func TestCSP_NonceAttrInUserScript(t *testing.T) {
	t.Parallel()
	hooks := kit.Hooks{
		Handle: func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
			res, err := resolve(ev)
			if err != nil || res == nil {
				return res, err
			}
			res.Headers.Set("X-Nonce-Attr", kit.NonceAttr(ev))
			return res, nil
		},
	}
	srv := newCSPServer(t, kit.CSPConfig{Mode: kit.CSPStrict}, []router.Route{{
		Pattern: "/", Segments: segmentsFor("/"), Page: staticPage("ok"),
	}}, hooks)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	// HTTP transports trim leading whitespace from header values, so the
	// canonical " nonce=..." prefix appears as `nonce="..."`. Verify the
	// payload shape rather than the wire-trimmed prefix; users embed the
	// helper directly into HTML where the leading space stays intact.
	got := resp.Header.Get("X-Nonce-Attr")
	if !strings.HasPrefix(got, `nonce="`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("NonceAttr shape = %q", got)
	}
}
