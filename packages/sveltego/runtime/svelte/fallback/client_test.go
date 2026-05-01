package fallback

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSidecar is a minimal HTTP server that mimics the Node sidecar's
// /render endpoint. It records the number of requests so tests can
// assert the cache hit path skips the round trip.
type fakeSidecar struct {
	calls atomic.Int32
	body  string
	head  string
	srv   *httptest.Server
}

func newFakeSidecar(t *testing.T, body, head string) *fakeSidecar {
	t.Helper()
	fs := &fakeSidecar{body: body, head: head}
	fs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/render" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		fs.calls.Add(1)
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"body": fs.body,
			"head": fs.head,
		})
	}))
	t.Cleanup(fs.srv.Close)
	return fs
}

func TestClientRenderCacheHit(t *testing.T) {
	t.Parallel()
	sc := newFakeSidecar(t, "<p>hi</p>", "<title>Hi</title>")
	c := NewClient(ClientOptions{Endpoint: sc.srv.URL, CacheSize: 8, TTL: time.Minute})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp1, err := c.Render(ctx, RenderRequest{Route: "/x", Source: "src/routes/x/_page.svelte", Data: map[string]int{"id": 1}})
	if err != nil {
		t.Fatalf("first render: %v", err)
	}
	if !strings.Contains(resp1.Body, "<p>hi</p>") {
		t.Fatalf("unexpected body: %q", resp1.Body)
	}
	resp2, err := c.Render(ctx, RenderRequest{Route: "/x", Source: "src/routes/x/_page.svelte", Data: map[string]int{"id": 1}})
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if resp2 != resp1 {
		t.Fatalf("cached response should be identical")
	}
	if got := sc.calls.Load(); got != 1 {
		t.Fatalf("sidecar calls = %d, want 1 (second call should hit cache)", got)
	}
}

func TestClientRenderCacheKeyVaryByData(t *testing.T) {
	t.Parallel()
	sc := newFakeSidecar(t, "<p>hi</p>", "")
	c := NewClient(ClientOptions{Endpoint: sc.srv.URL, CacheSize: 8, TTL: time.Minute})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := c.Render(ctx, RenderRequest{Route: "/x", Data: map[string]int{"id": 1}}); err != nil {
		t.Fatalf("render 1: %v", err)
	}
	if _, err := c.Render(ctx, RenderRequest{Route: "/x", Data: map[string]int{"id": 2}}); err != nil {
		t.Fatalf("render 2: %v", err)
	}
	if got := sc.calls.Load(); got != 2 {
		t.Fatalf("sidecar calls = %d, want 2 (different data should miss)", got)
	}
}

func TestClientRenderSidecarErrorPropagates(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"bang"}`))
	}))
	defer srv.Close()
	c := NewClient(ClientOptions{Endpoint: srv.URL, CacheSize: 8, TTL: time.Minute})
	_, err := c.Render(context.Background(), RenderRequest{Route: "/x"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("error missing status: %v", err)
	}
}

func TestRegistryUnregisteredRouteErrors(t *testing.T) {
	t.Parallel()
	r := &Registry{routes: map[string]string{}}
	_, err := r.Render(context.Background(), "/missing", nil)
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("expected unregistered-route error, got %v", err)
	}
}

func TestRegistryUnconfiguredErrors(t *testing.T) {
	t.Parallel()
	r := &Registry{routes: map[string]string{"/x": "src/routes/x/_page.svelte"}}
	_, err := r.Render(context.Background(), "/x", nil)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected unconfigured-registry error, got %v", err)
	}
}

func TestRegistryDispatch(t *testing.T) {
	t.Parallel()
	sc := newFakeSidecar(t, "<p>ok</p>", "")
	c := NewClient(ClientOptions{Endpoint: sc.srv.URL, CacheSize: 8, TTL: time.Minute})
	r := &Registry{routes: map[string]string{"/x": "src/routes/x/_page.svelte"}, client: c}

	resp, err := r.Render(context.Background(), "/x", map[string]int{"id": 7})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(resp.Body, "<p>ok</p>") {
		t.Fatalf("body = %q", resp.Body)
	}
}

// Sanity: ensure errors.Is plumbing in the helpers stays predictable
// when the HTTP transport itself fails. http.Client returning a wrapped
// error should propagate through Render unchanged (not panic on a nil
// response).
func TestClientRenderTransportError(t *testing.T) {
	t.Parallel()
	httpc := &http.Client{Transport: roundTripFn(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("connect refused")
	})}
	c := NewClient(ClientOptions{Endpoint: "http://example.invalid", HTTP: httpc})
	_, err := c.Render(context.Background(), RenderRequest{Route: "/x"})
	if err == nil || !strings.Contains(err.Error(), "connect refused") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

type roundTripFn func(*http.Request) (*http.Response, error)

func (f roundTripFn) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
