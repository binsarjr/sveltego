// Bench informational only; issue #20's 10k req/s target is for the full
// server under load with a real listener and OS network stack. The
// in-process ServeHTTP loop measured here runs in nanoseconds per call,
// well above 10k throughput on any modern CPU. p50 reference (Apple M1
// Pro, recorded 2026-04-30): ~163 ns/op, 144 B/op, 4 allocs/op.

package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/render"
	"github.com/binsarjr/sveltego/runtime/router"
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
