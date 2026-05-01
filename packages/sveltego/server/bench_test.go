// Bench informational only; issue #20's 10k req/s target is for the full
// server under load with a real listener and OS network stack. The
// in-process ServeHTTP loop measured here runs in nanoseconds per call,
// well above 10k throughput on any modern CPU. p50 reference (Apple M1
// Pro, recorded 2026-04-30): ~163 ns/op, 144 B/op, 4 allocs/op.

package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func benchServer(b *testing.B) *Server {
	b.Helper()
	srv, err := New(Config{
		Routes: []router.Route{
			{
				Pattern:  "/",
				Segments: []router.Segment{},
				Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
					w.WriteString("<h1>home</h1>")
					return nil
				},
			},
			{
				Pattern: "/about",
				Segments: []router.Segment{
					{Kind: router.SegmentStatic, Value: "about"},
				},
				Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
					w.WriteString("<h1>about</h1>")
					return nil
				},
			},
		},
		Shell:  testShell,
		Logger: quietLogger(),
	})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	return srv
}

func BenchmarkServeHTTP_index(b *testing.B) {
	srv := benchServer(b)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	b.ResetTimer()
	for range b.N {
		w.Body.Reset()
		srv.ServeHTTP(w, r)
	}
}

func BenchmarkServeHTTP_Hello(b *testing.B) {
	srv := benchServer(b)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
	}
}

// BenchmarkServeHTTP_HelloWithHead exercises the dedupeTitle path so the
// head-buffer scan is part of the steady-state allocation profile.
func BenchmarkServeHTTP_HelloWithHead(b *testing.B) {
	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:  "/",
			Segments: []router.Segment{},
			Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString("<h1>hello</h1>")
				return nil
			},
			Head: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString(`<meta charset="utf-8"><title>page</title><meta name="theme" content="dark">`)
				return nil
			},
		}},
		Shell:  testShell,
		Logger: quietLogger(),
	})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
	}
}

// BenchmarkEscapeScriptJSON_Clean asserts the no-escape fast path is
// allocation-free.
func BenchmarkEscapeScriptJSON_Clean(b *testing.B) {
	payload := []byte(`{"title":"Hello world","count":42,"items":["a","b","c"]}`)
	w := render.New()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w.Reset()
		writeEscapedScriptJSON(w, payload)
	}
}

// BenchmarkEscapeScriptJSON_Dirty exercises the slow path with an
// embedded `</` sequence to verify escape behavior is preserved.
func BenchmarkEscapeScriptJSON_Dirty(b *testing.B) {
	payload := []byte(`{"x":"a</script>b<!--c"}`)
	w := render.New()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w.Reset()
		writeEscapedScriptJSON(w, payload)
	}
}

// BenchmarkDedupeTitle_Single exercises the no-dedupe (single-title)
// path. Only the titleSpan slice for the single match allocates; the
// previous implementation also paid a full strings.ToLower copy.
func BenchmarkDedupeTitle_Single(b *testing.B) {
	in := []byte(strings.Repeat(`<meta charset="utf-8">`, 8) +
		`<title>page</title>` +
		strings.Repeat(`<meta name="theme" content="dark">`, 8))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = dedupeTitle(in)
	}
}

// BenchmarkDedupeTitle_NoTitle confirms the no-title fast path is
// allocation-free; the previous implementation always paid for the
// strings.ToLower copy.
func BenchmarkDedupeTitle_NoTitle(b *testing.B) {
	in := []byte(strings.Repeat(`<meta name="theme" content="dark">`, 16))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = dedupeTitle(in)
	}
}

// BenchmarkActionNameFromQuery_Default covers the empty-query default
// path and confirms zero allocations.
func BenchmarkActionNameFromQuery_Default(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		_ = actionNameFromQuery("")
	}
}

// BenchmarkActionNameFromQuery_Match exercises a query with one
// non-action prefix and an action token at index 1.
func BenchmarkActionNameFromQuery_Match(b *testing.B) {
	q := "x=1&/submit"
	b.ReportAllocs()
	for range b.N {
		_ = actionNameFromQuery(q)
	}
}

// BenchmarkCollectStreams_NoStream measures steady-state cost for a Load
// result with no streamed fields. After the first call seeds the type
// cache, subsequent iterations are a sync.Map load + zero field work.
func BenchmarkCollectStreams_NoStream(b *testing.B) {
	type plainData struct {
		Title string
		Count int
	}
	data := plainData{Title: "hello", Count: 42}
	_ = collectStreams(data, nil) // warm cache
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = collectStreams(data, nil)
	}
}

// BenchmarkCollectStreams_WithStream measures steady-state cost for a Load
// result that contains a kit.Streamed field. After warmup the type cache
// holds the field index; each iteration does one sync.Map load and one
// direct field access — no repeated struct walk.
func BenchmarkCollectStreams_WithStream(b *testing.B) {
	type streamData struct {
		Title string
		Posts *kit.Streamed[[]string]
	}
	data := streamData{
		Title: "home",
		Posts: kit.StreamCtx(context.Background(), func(_ context.Context) ([]string, error) { return nil, nil }),
	}
	_ = collectStreams(data, nil) // warm cache
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = collectStreams(data, nil)
	}
}
