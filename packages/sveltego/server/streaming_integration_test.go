package server

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

// countingHandler is a slog.Handler that tallies records by level without
// producing any output. Used to assert exactly one debug log and zero
// error logs on client disconnect.
type countingHandler struct {
	debug atomic.Int64
	warn  atomic.Int64
	errs  atomic.Int64
}

func (h *countingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	switch {
	case r.Level >= slog.LevelError:
		h.errs.Add(1)
	case r.Level >= slog.LevelWarn:
		h.warn.Add(1)
	case r.Level == slog.LevelDebug:
		h.debug.Add(1)
	}
	return nil
}
func (h *countingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(_ string) slog.Handler      { return h }

type streamingPageData struct {
	Title string
	Posts *kit.Streamed[[]string]
}

type doubleStreamData struct {
	First  *kit.Streamed[string]
	Second *kit.Streamed[int]
}

func TestStreaming_PlaceholderShellFlushesBeforePayload(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Title: "Home",
				Posts: kit.StreamCtx(lctx.Request.Context(), func(_ context.Context) ([]string, error) {
					<-release
					return []string{"a", "b"}, nil
				}),
			}, nil
		},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString(`<h1>shell</h1>`)
			return nil
		},
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	br := bufio.NewReader(resp.Body)
	shell := readUntil(t, br, "<h1>shell</h1>", 500*time.Millisecond)
	if !strings.Contains(shell, "<h1>shell</h1>") {
		t.Fatalf("shell not flushed first; got %q", shell)
	}
	release <- struct{}{}

	rest, err := io.ReadAll(br)
	if err != nil {
		t.Fatalf("read rest: %v", err)
	}
	full := shell + string(rest)
	if !strings.Contains(full, `__sveltego__resolve(`) {
		t.Fatalf("resolve script missing; body=%q", full)
	}
	if !strings.Contains(full, `"data":["a","b"]`) {
		t.Fatalf("payload missing; body=%q", full)
	}
	if !strings.HasSuffix(strings.TrimSpace(full), "</html>") {
		t.Fatalf("missing closing html; body=%q", full)
	}
	if got := resp.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length should be unset for streaming; got %q", got)
	}
}

func TestStreaming_TimeoutEmitsErrorPayload(t *testing.T) {
	t.Parallel()

	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Title: "Slow",
				Posts: kit.StreamCtx(lctx.Request.Context(), func(_ context.Context) ([]string, error) {
					<-stuck
					return nil, nil
				}),
			}, nil
		},
		Page: staticPage("<main>shell</main>"),
	}}

	srv, err := New(Config{
		Routes:        routes,
		Shell:         testShell,
		Logger:        quietLogger(),
		StreamTimeout: 25 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, `"error":"kit: stream timeout"`) {
		t.Fatalf("expected timeout error in body; got %q", s)
	}
	if !strings.Contains(s, "<main>shell</main>") {
		t.Fatalf("shell missing; got %q", s)
	}
}

func TestStreaming_MultipleStreamsResolveInOrder(t *testing.T) {
	t.Parallel()

	gateA := make(chan struct{})
	gateB := make(chan struct{})

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			rctx := lctx.Request.Context()
			return doubleStreamData{
				First: kit.StreamCtx(rctx, func(_ context.Context) (string, error) {
					<-gateA
					return "alpha", nil
				}),
				Second: kit.StreamCtx(rctx, func(_ context.Context) (int, error) {
					<-gateB
					return 7, nil
				}),
			}, nil
		},
		Page: staticPage("<p>shell</p>"),
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	var (
		body string
		wg   sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, err := http.Get(ts.URL + "/")
		if err != nil {
			t.Errorf("GET: %v", err)
			return
		}
		defer resp.Body.Close()
		buf, _ := io.ReadAll(resp.Body)
		body = string(buf)
	}()

	close(gateB)
	time.Sleep(20 * time.Millisecond)
	close(gateA)
	wg.Wait()

	idxA := strings.Index(body, `"data":"alpha"`)
	idxB := strings.Index(body, `"data":7`)
	if idxA < 0 || idxB < 0 {
		t.Fatalf("missing payloads; body=%q", body)
	}
	if idxA >= idxB {
		t.Fatalf("First stream should resolve before Second in payload order; body=%q", body)
	}
}

func TestStreaming_ClientDisconnectStopsWaiting(t *testing.T) {
	t.Parallel()

	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Posts: kit.StreamCtx(lctx.Request.Context(), func(_ context.Context) ([]string, error) {
					<-stuck
					return nil, nil
				}),
			}, nil
		},
		Page: staticPage("<p>shell</p>"),
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			done <- err
			return
		}
		_, err = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		done <- err
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("request did not unblock after client cancel")
	}
}

func TestStreaming_NonStreamedPageStillBuffered(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load:     func(_ *kit.LoadCtx) (any, error) { return "ok", nil },
		Page:     loadingPage(),
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if cl := resp.Header.Get("Content-Length"); cl == "" {
		t.Fatalf("non-streaming response should have Content-Length")
	}
	if !strings.Contains(string(body), "<h1>ok</h1>") {
		t.Fatalf("body missing page; got %q", body)
	}
	if strings.Contains(string(body), "__sveltego__resolve") {
		t.Fatalf("non-streaming response should not contain resolve script; got %q", body)
	}
}

func TestStreaming_ScriptInjectionEscaped(t *testing.T) {
	t.Parallel()

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Posts: kit.StreamCtx(lctx.Request.Context(), func(_ context.Context) ([]string, error) {
					return []string{"</script><script>alert(1)</script>"}, nil
				}),
			}, nil
		},
		Page: staticPage("<p>shell</p>"),
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if strings.Contains(s, `"</script><script>alert(1)</script>"`) {
		t.Fatalf("inline </script> was not neutralized; body=%q", s)
	}
	if !strings.Contains(s, `</script`) {
		t.Fatalf("expected escaped \\u003c/script in payload; body=%q", s)
	}
}

func TestStreaming_ClientDisconnect_NoErrorSpam(t *testing.T) {
	t.Parallel()

	// stuck keeps the stream producer blocked so the client disconnects
	// while the stream is still pending.
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Posts: kit.StreamCtx(lctx.Request.Context(), func(_ context.Context) ([]string, error) {
					<-stuck
					return nil, nil
				}),
			}, nil
		},
		Page: staticPage("<p>shell</p>"),
	}}

	logs := &countingHandler{}
	srv, err := New(Config{
		Routes: routes,
		Shell:  testShell,
		Logger: slog.New(logs),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	// Start the request; read the shell so the server has flushed the
	// first chunk before we disconnect.
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			done <- doErr
			return
		}
		// Read until the shell arrives so the server is past the first
		// FlushTo and is now waiting on the stream.
		br := bufio.NewReader(resp.Body)
		readUntil(t, br, "<p>shell</p>", 500*time.Millisecond)
		cancel() // disconnect
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		done <- nil
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not complete after client disconnect")
	}

	// Give the server goroutine a moment to record its log entries.
	time.Sleep(50 * time.Millisecond)

	if n := logs.errs.Load(); n != 0 {
		t.Errorf("want 0 error log entries on client disconnect; got %d", n)
	}
	if n := logs.warn.Load(); n != 0 {
		t.Errorf("want 0 warn log entries on client disconnect; got %d", n)
	}
	if n := logs.debug.Load(); n < 1 {
		t.Errorf("want at least 1 debug log entry on client disconnect; got %d", n)
	}
}

// TestStreaming_CancelPropagatesWithinDeadline verifies that when a request
// context is cancelled, the producer goroutine behind a StreamCtx stream
// receives the cancellation signal and exits within the deadline. The test
// uses a second gate to confirm the goroutine saw ctx.Done() rather than
// blocking forever on its own channel.
func TestStreaming_CancelPropagatesWithinDeadline(t *testing.T) {
	t.Parallel()

	producerExited := make(chan struct{})

	routes := []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Load: func(lctx *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Posts: kit.StreamCtx(lctx.Request.Context(), func(ctx context.Context) ([]string, error) {
					defer close(producerExited)
					<-ctx.Done()
					return nil, ctx.Err()
				}),
			}, nil
		},
		Page: staticPage("<p>shell</p>"),
	}}
	srv := newTestServer(t, routes)
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Give the shell time to flush before cancelling.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-producerExited:
	case <-time.After(time.Second):
		t.Fatalf("producer goroutine did not exit after context cancel within 1s")
	}
}

// TestCSP_TemplateNoPerRequestAlloc verifies that applyCSP uses the
// precompiled CSPTemplate and does not perform per-request directive-map
// allocations for the header value. generateNonce emits one string alloc
// and template.Build emits one concat alloc; the directive-merge path that
// BuildCSPHeader (the old path) exercised on every call is absent.
func TestCSP_TemplateNoPerRequestAlloc(t *testing.T) {
	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:  "/",
			Segments: segmentsFor("/"),
			Page:     staticPage("ok"),
		}},
		Shell:  testShell,
		Logger: quietLogger(),
		CSP:    kit.CSPConfig{Mode: kit.CSPStrict},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Pre-create stable objects so the alloc counter reflects only the
	// work applyCSP itself does, not recorder/event construction.
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ev := kit.NewRequestEvent(r, nil)

	// Warm once to populate any lazy state.
	srv.applyCSP(rec, ev)

	// Reset recorder header map so Set doesn't grow slices on first call.
	rec.Header().Del("Content-Security-Policy")

	const runs = 10
	allocs := testing.AllocsPerRun(runs, func() {
		// Reset the nonce between iterations; ev.Locals already exists.
		kit.SetNonce(ev, "")
		srv.applyCSP(rec, ev)
	})
	// generateNonce: 1 alloc (base64 string). SetNonce interface box: 1 alloc.
	// Build 3-way concat: 1-2 allocs (compiler-dependent). Header.Set
	// []string: 1 alloc. Allow ≤ 7 to tolerate minor runtime variation while
	// proving no directive-map rebuild path (which adds ~8+ allocs from map
	// construction + sort + slice appends). The template path is always faster
	// than the old BuildCSPHeader hot path.
	if allocs > 7 {
		t.Errorf("applyCSP allocs = %.0f per call, want ≤ 7 (no directive map rebuild)", allocs)
	}
}

// readUntil reads from br until the marker is found or timeout elapses.
// Used by streaming tests to assert the shell flushes before the
// blocked stream resolves; reading the whole body would deadlock.
func readUntil(t *testing.T, br *bufio.Reader, marker string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var buf strings.Builder
	for time.Now().Before(deadline) {
		if c, ok := br.Buffered(), true; c > 0 && ok {
			chunk := make([]byte, c)
			n, _ := br.Read(chunk)
			buf.Write(chunk[:n])
			if strings.Contains(buf.String(), marker) {
				return buf.String()
			}
			continue
		}
		b, err := br.ReadByte()
		if err != nil {
			if buf.Len() > 0 {
				return buf.String()
			}
			t.Fatalf("readUntil: %v", err)
		}
		buf.WriteByte(b)
		if strings.Contains(buf.String(), marker) {
			return buf.String()
		}
	}
	return buf.String()
}
