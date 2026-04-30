package server

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
)

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
		Load: func(_ *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Title: "Home",
				Posts: kit.Stream(func() ([]string, error) {
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
		Load: func(_ *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Title: "Slow",
				Posts: kit.Stream(func() ([]string, error) {
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
		Load: func(_ *kit.LoadCtx) (any, error) {
			return doubleStreamData{
				First: kit.Stream(func() (string, error) {
					<-gateA
					return "alpha", nil
				}),
				Second: kit.Stream(func() (int, error) {
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
		Load: func(_ *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Posts: kit.Stream(func() ([]string, error) {
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
		Load: func(_ *kit.LoadCtx) (any, error) {
			return streamingPageData{
				Posts: kit.Stream(func() ([]string, error) {
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
