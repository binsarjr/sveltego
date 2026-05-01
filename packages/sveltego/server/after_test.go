package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// afterRoute builds a route that queues n After callbacks via the Handle
// hook and records each execution into calls (in order) using mu for
// synchronisation with the test goroutine.
func afterRoute(_ *[]int, _ *sync.Mutex, _ ...func(context.Context)) router.Route {
	return router.Route{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("ok"),
		// Hooks are wired per-request via kit.Hooks.Handle, not on the
		// route itself, so we use a server-level Handle hook below.
	}
}

func newAfterServer(t *testing.T, handle kit.HandleFn) *Server {
	t.Helper()
	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:  "/",
			Segments: segmentsFor("/"),
			Page:     staticPage("ok"),
		}},
		Shell:  testShell,
		Logger: quietLogger(),
		Hooks:  kit.Hooks{Handle: handle},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return srv
}

// TestAfter_firesInOrder verifies that multiple After callbacks execute in
// registration order after the response is written.
func TestAfter_firesInOrder(t *testing.T) {
	t.Parallel()

	var (
		mu   sync.Mutex
		got  []int
		done = make(chan struct{})
	)

	handle := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		ev.After(func(_ context.Context) {
			mu.Lock()
			got = append(got, 1)
			mu.Unlock()
		})
		ev.After(func(_ context.Context) {
			mu.Lock()
			got = append(got, 2)
			mu.Unlock()
		})
		ev.After(func(_ context.Context) {
			mu.Lock()
			got = append(got, 3)
			close(done)
			mu.Unlock()
		})
		return resolve(ev)
	}
	_ = afterRoute(nil, nil)

	srv := newAfterServer(t, handle)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("After callbacks never completed")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("After order: got %v want [1 2 3]", got)
	}
}

// TestAfter_allRun verifies that every queued callback runs (not just the first).
func TestAfter_allRun(t *testing.T) {
	t.Parallel()

	var count atomic.Int32
	n := 10
	done := make(chan struct{})

	handle := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		for i := 0; i < n; i++ {
			ev.After(func(_ context.Context) {
				if count.Add(1) == int32(n) {
					close(done)
				}
			})
		}
		return resolve(ev)
	}

	srv := newAfterServer(t, handle)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("After: only %d/%d callbacks ran", count.Load(), n)
	}
}

// TestAfter_ctxCancelStopsDrain verifies that a cancelled drain context
// stops execution of remaining After callbacks.
func TestAfter_ctxCancelStopsDrain(t *testing.T) {
	t.Parallel()

	// DrainAfter is the exported entry point tested directly here,
	// bypassing the full HTTP stack so we can inject a pre-cancelled ctx.
	ev := kit.NewRequestEvent(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	var ran atomic.Int32
	for i := 0; i < 5; i++ {
		ev.After(func(_ context.Context) {
			ran.Add(1)
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	kit.DrainAfter(ctx, ev)

	// With a pre-cancelled context, no callbacks should execute.
	if got := ran.Load(); got != 0 {
		t.Fatalf("expected 0 callbacks with cancelled ctx, got %d", got)
	}
}

// TestAfter_nilFnIgnored verifies that passing nil to After does not panic.
func TestAfter_nilFnIgnored(t *testing.T) {
	t.Parallel()

	ev := kit.NewRequestEvent(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	ev.After(nil) // must not panic

	var ran atomic.Int32
	ev.After(func(_ context.Context) { ran.Add(1) })

	ctx := context.Background()
	kit.DrainAfter(ctx, ev)

	if ran.Load() != 1 {
		t.Fatalf("expected 1 callback run, got %d", ran.Load())
	}
}

// TestAfter_responseAlreadySent verifies that the response status is
// unaffected by After callbacks (they run after the write).
func TestAfter_responseAlreadySent(t *testing.T) {
	t.Parallel()

	var done atomic.Bool

	handle := func(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
		ev.After(func(_ context.Context) {
			done.Store(true)
		})
		return resolve(ev)
	}

	srv := newAfterServer(t, handle)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d want 200", resp.StatusCode)
	}

	// Give the drain goroutine a moment — it runs inline in ServeHTTP,
	// so by the time the client receives the response it should be done.
	deadline := time.Now().Add(3 * time.Second)
	for !done.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !done.Load() {
		t.Fatal("After callback did not run after response")
	}
}
