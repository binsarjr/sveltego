// Package bench groups end-to-end SSR benchmarks driven through Go's
// testing.B. Each scenario is built once per benchmark and reused; the
// hot loop performs a single ServeHTTP call against a recorder.
//
// Run: go test -bench=. -benchmem -count=6 ./...
// Compare: benchstat baseline/baseline.txt /tmp/new.txt
package bench

import (
	"net/http/httptest"
	"testing"

	"github.com/binsarjr/sveltego/bench/scenarios"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

func BenchmarkServeHTTP_Hello(b *testing.B)  { runScenario(b, mustHello) }
func BenchmarkServeHTTP_List(b *testing.B)   { runScenario(b, mustList) }
func BenchmarkServeHTTP_Detail(b *testing.B) { runScenario(b, mustDetail) }
func BenchmarkServeHTTP_Action(b *testing.B) { runScenario(b, mustAction) }

func runScenario(b *testing.B, build func(*testing.B) scenarios.Scenario) {
	b.Helper()
	sc := build(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rec := httptest.NewRecorder()
		_ = sc.Run(rec)
	}
}

// BenchmarkRouteResolution measures only the path-match step, isolated
// from the render pipeline. Useful for tracking router-level regressions
// independently of template work.
func BenchmarkRouteResolution(b *testing.B) {
	tree := buildRouteTree(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _, _ = tree.Match("/posts/42")
	}
}

// BenchmarkRenderWriter measures the render.Writer hot loop with mixed
// trusted and escaped output, mirroring what page handlers emit.
func BenchmarkRenderWriter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		w := render.Acquire()
		w.WriteString("<article><h1>post ")
		w.WriteEscape("42")
		w.WriteString("</h1><p>body</p></article>")
		render.Release(w)
	}
}

// BenchmarkManifestColdStart times scenario construction (route tree
// build + matchers + shell parse). It approximates the per-process
// startup cost the runtime pays once before the first request.
func BenchmarkManifestColdStart(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := scenarios.Hello(); err != nil {
			b.Fatalf("hello: %v", err)
		}
	}
}

func mustHello(b *testing.B) scenarios.Scenario  { return must(b, scenarios.Hello) }
func mustList(b *testing.B) scenarios.Scenario   { return must(b, scenarios.List) }
func mustDetail(b *testing.B) scenarios.Scenario { return must(b, scenarios.Detail) }
func mustAction(b *testing.B) scenarios.Scenario { return must(b, scenarios.Action) }

func must(b *testing.B, build func() (scenarios.Scenario, error)) scenarios.Scenario {
	b.Helper()
	sc, err := build()
	if err != nil {
		b.Fatalf("build scenario: %v", err)
	}
	return sc
}

func buildRouteTree(b *testing.B) *router.Tree {
	b.Helper()
	tree, err := router.NewTree([]router.Route{
		{
			Pattern: "/posts/[id]",
			Segments: []router.Segment{
				{Kind: router.SegmentStatic, Value: "posts"},
				{Kind: router.SegmentParam, Name: "id"},
			},
		},
	})
	if err != nil {
		b.Fatalf("NewTree: %v", err)
	}
	return tree
}
