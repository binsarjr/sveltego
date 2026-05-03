package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/csrfinject"
)

// fallbackInjectedPage mirrors the codegen-emitted renderFallback__*
// adapter shape: render the route's HTML, then pipe it through
// csrfinject.Rewrite so POST forms gain the hidden _csrf_token input.
// The simulated body matches what the Node sidecar would have returned
// for an authored `<form method="post" action="?/login">`.
func fallbackInjectedPage() router.PageHandler {
	return func(w *render.Writer, ctx *kit.RenderCtx, _ any) error {
		body := `<form method="post" action="?/login"><input name="username"/><button>Sign in</button></form>`
		w.WriteString(csrfinject.Rewrite(body, ctx.CSRFToken()))
		return nil
	}
}

// TestCSRFInject_FallbackPathInjectsHiddenInput exercises the full
// pipeline issue #510 fixes: a POST form rendered through the sidecar
// fallback path receives a hidden _csrf_token input populated from the
// per-request token, and a subsequent POST that submits only the form
// fields (without manually appending _csrf_token) clears the CSRF
// middleware's double-submit check.
func TestCSRFInject_FallbackPathInjectsHiddenInput(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"login": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	opts := kit.DefaultPageOptions()
	opts.Templates = ""
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     fallbackInjectedPage(),
		Actions:  func() any { return actions },
		Options:  opts,
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. GET issues the cookie + renders the form with a hidden input
	//    carrying the same token.
	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", resp.StatusCode)
	}

	var cookieToken string
	for _, c := range resp.Cookies() {
		if c.Name == "_csrf" {
			cookieToken = c.Value
			break
		}
	}
	if cookieToken == "" {
		t.Fatalf("expected _csrf cookie on GET; got cookies=%v", resp.Cookies())
	}

	// The hidden input must be present, with value == cookie token.
	hiddenRE := regexp.MustCompile(`<input type="hidden" name="_csrf_token" value="([^"]+)">`)
	m := hiddenRE.FindStringSubmatch(string(body))
	if m == nil {
		t.Fatalf("expected hidden _csrf_token input in body; body=\n%s", body)
	}
	if m[1] != cookieToken {
		t.Fatalf("hidden input value %q does not match cookie %q", m[1], cookieToken)
	}

	// 2. POST with only form fields (no manual _csrf_token query/body)
	//    must succeed because the middleware reads the field from the
	//    submitted form body — and we proved above that the field is
	//    in the rendered HTML for the user agent to submit.
	form := url.Values{
		"username":    {"admin"},
		"password":    {"secret"},
		"_csrf_token": {m[1]}, // simulate the browser submitting the hidden field
	}
	req, err := http.NewRequest(http.MethodPost,
		ts.URL+"/login?/login",
		strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "_csrf", Value: cookieToken})

	postResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, want 200 (CSRF should accept the hidden field value)", postResp.StatusCode)
	}
}

// TestCSRFInject_HydrationPayloadIncludesCSRFToken covers issue #523: the
// JSON hydration payload shipped to the client must carry the per-request
// `csrfToken` so the post-mount splicer in entry.ts can re-add the hidden
// input whenever Svelte 5 hydration strips it (typical on ssr-fallback
// routes whose source `.svelte` lacks the input in its vDOM). Without
// this field, `__sveltego_csrf__()` early-returns and the form submits
// without `_csrf_token`, triggering 403 forbidden.
func TestCSRFInject_HydrationPayloadIncludesCSRFToken(t *testing.T) {
	t.Parallel()
	actions := kit.ActionMap{
		"login": func(_ *kit.RequestEvent) kit.ActionResult {
			return kit.ActionDataResult(200, "ok")
		},
	}
	opts := kit.DefaultPageOptions()
	opts.Templates = ""
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/login",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "login"}},
		Page:     fallbackInjectedPage(),
		Actions:  func() any { return actions },
		Options:  opts,
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/login")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var cookieToken string
	for _, c := range resp.Cookies() {
		if c.Name == "_csrf" {
			cookieToken = c.Value
			break
		}
	}
	if cookieToken == "" {
		t.Fatalf("expected _csrf cookie on GET; got cookies=%v", resp.Cookies())
	}

	// The hydration payload script tag must carry "csrfToken":"<value>"
	// so client entry.ts has something to splice back in after Svelte
	// hydration strips the SSR-injected hidden input. The value must
	// match the `_csrf` cookie so a POST sent with that hidden field
	// passes the double-submit check.
	want := `"csrfToken":"` + cookieToken + `"`
	if !strings.Contains(string(body), want) {
		t.Fatalf("hydration payload missing %q; body=\n%s", want, body)
	}
}

// TestCSRFInject_FallbackPathSkipsGetForm asserts the same renderFallback
// shape leaves GET forms alone — the runtime rewriter gates on method
// just like the build-time pass.
func TestCSRFInject_FallbackPathSkipsGetForm(t *testing.T) {
	t.Parallel()
	page := router.PageHandler(func(w *render.Writer, ctx *kit.RenderCtx, _ any) error {
		body := `<form method="get"><input name="q"/></form>`
		w.WriteString(csrfinject.Rewrite(body, ctx.CSRFToken()))
		return nil
	})
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/search",
		Segments: []router.Segment{{Kind: router.SegmentStatic, Value: "search"}},
		Page:     page,
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/search")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "_csrf_token") {
		t.Fatalf("GET form should not get hidden CSRF input:\n%s", body)
	}
}
