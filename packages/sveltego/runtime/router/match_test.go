package router_test

import (
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/runtime/router"
)

func mustBuild(t *testing.T, routes []router.Route) *router.Tree {
	t.Helper()
	tree, err := router.NewTree(routes)
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	return tree
}

func parsePattern(t *testing.T, pattern string) []router.Segment {
	t.Helper()
	if pattern == "/" {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	out := make([]router.Segment, 0, len(parts))
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "[[") && strings.HasSuffix(p, "]]"):
			body := p[2 : len(p)-2]
			name, matcher := splitMatcher(body)
			out = append(out, router.Segment{Kind: router.SegmentOptional, Name: name, Matcher: matcher})
		case strings.HasPrefix(p, "[...") && strings.HasSuffix(p, "]"):
			body := p[4 : len(p)-1]
			name, matcher := splitMatcher(body)
			out = append(out, router.Segment{Kind: router.SegmentRest, Name: name, Matcher: matcher})
		case strings.HasPrefix(p, "[") && strings.HasSuffix(p, "]"):
			body := p[1 : len(p)-1]
			name, matcher := splitMatcher(body)
			out = append(out, router.Segment{Kind: router.SegmentParam, Name: name, Matcher: matcher})
		default:
			out = append(out, router.Segment{Kind: router.SegmentStatic, Value: p})
		}
	}
	return out
}

func splitMatcher(body string) (name, matcher string) {
	if before, after, ok := strings.Cut(body, "="); ok {
		return before, after
	}
	return body, ""
}

func mkRoute(t *testing.T, pattern string) router.Route {
	t.Helper()
	return router.Route{Pattern: pattern, Segments: parsePattern(t, pattern)}
}

func TestMatch_RootStatic(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/")})
	r, params, ok := tree.Match("/")
	if !ok || r == nil {
		t.Fatalf("no match for /")
	}
	if r.Pattern != "/" {
		t.Errorf("Pattern = %q, want /", r.Pattern)
	}
	if params != nil {
		t.Errorf("params = %v, want nil", params)
	}
}

func TestMatch_StaticDeep(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/about")})
	r, params, ok := tree.Match("/about")
	if !ok || r == nil {
		t.Fatalf("no match for /about")
	}
	if r.Pattern != "/about" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if params != nil {
		t.Errorf("params = %v, want nil", params)
	}
}

func TestMatch_Param(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/post/[id]")})
	r, params, ok := tree.Match("/post/42")
	if !ok {
		t.Fatalf("no match for /post/42")
	}
	if r.Pattern != "/post/[id]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := params["id"]; got != "42" {
		t.Errorf("id = %q, want 42", got)
	}
}

func TestMatch_StaticBeatsParam(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/post/[id]"),
		mkRoute(t, "/post/new"),
	})
	r, _, ok := tree.Match("/post/new")
	if !ok {
		t.Fatalf("no match")
	}
	if r.Pattern != "/post/new" {
		t.Errorf("Pattern = %q, want /post/new", r.Pattern)
	}
}

func TestMatch_OptionalAbsent(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/[[lang]]/about")})
	r, params, ok := tree.Match("/about")
	if !ok {
		t.Fatalf("no match for /about")
	}
	if r.Pattern != "/[[lang]]/about" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got, want := params["lang"], ""; got != want {
		t.Errorf("lang = %q, want %q", got, want)
	}
}

func TestMatch_OptionalPresent(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/[[lang]]/about")})
	r, params, ok := tree.Match("/en/about")
	if !ok {
		t.Fatalf("no match for /en/about")
	}
	if r.Pattern != "/[[lang]]/about" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := params["lang"]; got != "en" {
		t.Errorf("lang = %q, want en", got)
	}
}

func TestMatch_RestEmpty(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/docs/[...path]")})
	r, params, ok := tree.Match("/docs")
	if !ok {
		t.Fatalf("no match for /docs")
	}
	if r.Pattern != "/docs/[...path]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := params["path"]; got != "" {
		t.Errorf("path = %q, want empty", got)
	}
}

func TestMatch_RestSingle(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/docs/[...path]")})
	r, params, ok := tree.Match("/docs/a")
	if !ok {
		t.Fatalf("no match for /docs/a")
	}
	if r.Pattern != "/docs/[...path]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := params["path"]; got != "a" {
		t.Errorf("path = %q, want a", got)
	}
}

func TestMatch_RestDeep(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/docs/[...path]")})
	r, params, ok := tree.Match("/docs/a/b/c")
	if !ok {
		t.Fatalf("no match for /docs/a/b/c")
	}
	if got := params["path"]; got != "a/b/c" {
		t.Errorf("path = %q, want a/b/c", got)
	}
	_ = r
}

type intMatcher struct{}

func (intMatcher) Match(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}

func TestMatch_ParamMatcherAccepts(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/post/[id=int]")})
	tree, err := tree.WithMatchers(router.Matchers{"int": intMatcher{}})
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}
	r, params, ok := tree.Match("/post/42")
	if !ok {
		t.Fatalf("no match for /post/42")
	}
	if r.Pattern != "/post/[id=int]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := params["id"]; got != "42" {
		t.Errorf("id = %q", got)
	}
}

func TestMatch_ParamMatcherRejects(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/post/[id=int]")})
	tree, err := tree.WithMatchers(router.Matchers{"int": intMatcher{}})
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}
	if r, _, ok := tree.Match("/post/abc"); ok {
		t.Errorf("expected no match, got %q", r.Pattern)
	}
}

func TestMatch_AmbiguousParamVsStaticTrailing(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/[a]/[b]"),
		mkRoute(t, "/[c]/d"),
	})
	r, params, ok := tree.Match("/x/d")
	if !ok {
		t.Fatalf("no match for /x/d")
	}
	if r.Pattern != "/[c]/d" {
		t.Errorf("Pattern = %q, want /[c]/d", r.Pattern)
	}
	if got := params["c"]; got != "x" {
		t.Errorf("c = %q", got)
	}
	if _, has := params["b"]; has {
		t.Errorf("expected no b, got %v", params)
	}
}

func TestMatch_AmbiguousFallbackToParam(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/[a]/[b]"),
		mkRoute(t, "/[c]/d"),
	})
	r, params, ok := tree.Match("/x/y")
	if !ok {
		t.Fatalf("no match for /x/y")
	}
	if r.Pattern != "/[a]/[b]" {
		t.Errorf("Pattern = %q, want /[a]/[b]", r.Pattern)
	}
	if got := params["a"]; got != "x" {
		t.Errorf("a = %q", got)
	}
	if got := params["b"]; got != "y" {
		t.Errorf("b = %q", got)
	}
}

func TestMatch_PercentEncodedSlashPreserved(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/files/[...path]")})
	r, params, ok := tree.Match("/files/a%2Fb")
	if !ok {
		t.Fatalf("no match")
	}
	if r.Pattern != "/files/[...path]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := params["path"]; got != "a/b" {
		t.Errorf("path = %q, want a/b", got)
	}
}

func TestMatch_PercentEncodedParam(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/post/[slug]")})
	r, params, ok := tree.Match("/post/hello%20world")
	if !ok {
		t.Fatalf("no match")
	}
	_ = r
	if got := params["slug"]; got != "hello world" {
		t.Errorf("slug = %q, want %q", got, "hello world")
	}
}

func TestMatch_NoMatch(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/about")})
	if r, _, ok := tree.Match("/missing"); ok {
		t.Errorf("expected no match, got %q", r.Pattern)
	}
}

func TestMatch_StaticBeatsOptional(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/about"),
		mkRoute(t, "/[[lang]]/about"),
	})
	r, params, ok := tree.Match("/about")
	if !ok {
		t.Fatalf("no match")
	}
	if r.Pattern != "/about" {
		t.Errorf("Pattern = %q, want /about", r.Pattern)
	}
	if params != nil {
		t.Errorf("expected nil params, got %v", params)
	}
}

func TestMatch_OptionalBeatsRest(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/[[lang]]/about"),
		mkRoute(t, "/[...path]"),
	})
	r, _, ok := tree.Match("/en/about")
	if !ok {
		t.Fatalf("no match")
	}
	if r.Pattern != "/[[lang]]/about" {
		t.Errorf("Pattern = %q, want /[[lang]]/about", r.Pattern)
	}
}

func TestNewTree_DuplicatePattern(t *testing.T) {
	_, err := router.NewTree([]router.Route{
		mkRoute(t, "/about"),
		mkRoute(t, "/about"),
	})
	if err == nil {
		t.Fatalf("expected error on duplicate pattern")
	}
}

func TestWithMatchers_MissingMatcherErrors(t *testing.T) {
	tree, err := router.NewTree([]router.Route{mkRoute(t, "/post/[id=missing]")})
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	if _, err := tree.WithMatchers(router.Matchers{}); err == nil {
		t.Fatalf("expected build error for missing matcher")
	}
}

func TestRouteID_Stable(t *testing.T) {
	tree, err := router.NewTree([]router.Route{
		mkRoute(t, "/about"),
		mkRoute(t, "/post/[id]"),
	})
	if err != nil {
		t.Fatalf("NewTree: %v", err)
	}
	rs := tree.Routes()
	if len(rs[0].ID) != 8 {
		t.Errorf("ID len = %d, want 8", len(rs[0].ID))
	}
	if rs[0].ID == rs[1].ID {
		t.Errorf("IDs collide: %q", rs[0].ID)
	}
}

func TestNewTree_DeterministicAcrossInsertOrder(t *testing.T) {
	patterns := []string{
		"/",
		"/about",
		"/post/[id]",
		"/post/new",
		"/[[lang]]/about",
		"/docs/[...path]",
		"/files/[id=int]",
		"/[a]/[b]",
		"/[c]/d",
	}

	build := func(seed int64) []string {
		rng := rand.New(rand.NewSource(seed))
		ordered := append([]string(nil), patterns...)
		rng.Shuffle(len(ordered), func(i, j int) { ordered[i], ordered[j] = ordered[j], ordered[i] })
		routes := make([]router.Route, len(ordered))
		for i, p := range ordered {
			routes[i] = mkRoute(t, p)
		}
		tr, err := router.NewTree(routes)
		if err != nil {
			t.Fatalf("NewTree seed=%d: %v", seed, err)
		}
		_, err = tr.WithMatchers(router.Matchers{"int": intMatcher{}})
		if err != nil {
			t.Fatalf("WithMatchers seed=%d: %v", seed, err)
		}
		paths := []string{
			"/",
			"/about",
			"/post/42",
			"/post/new",
			"/en/about",
			"/docs/a/b",
			"/files/123",
			"/x/d",
			"/x/y",
			"/missing/path",
		}
		out := make([]string, 0, len(paths))
		for _, p := range paths {
			r, params, ok := tr.Match(p)
			if !ok {
				out = append(out, p+" -> nomatch")
				continue
			}
			keys := make([]string, 0, len(params))
			for k := range params {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := []string{p, "->", r.Pattern}
			for _, k := range keys {
				parts = append(parts, k+"="+params[k])
			}
			out = append(out, strings.Join(parts, " "))
		}
		return out
	}

	a := build(1)
	b := build(2)
	c := build(42)
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(a, c) {
		t.Fatalf("non-deterministic match results:\n a=%v\n b=%v\n c=%v", a, b, c)
	}
}

func TestSegment_String(t *testing.T) {
	cases := []struct {
		seg  router.Segment
		want string
	}{
		{router.Segment{Kind: router.SegmentStatic, Value: "about"}, "about"},
		{router.Segment{Kind: router.SegmentParam, Name: "id"}, "[id]"},
		{router.Segment{Kind: router.SegmentParam, Name: "id", Matcher: "int"}, "[id=int]"},
		{router.Segment{Kind: router.SegmentOptional, Name: "lang"}, "[[lang]]"},
		{router.Segment{Kind: router.SegmentOptional, Name: "lang", Matcher: "iso"}, "[[lang=iso]]"},
		{router.Segment{Kind: router.SegmentRest, Name: "path"}, "[...path]"},
	}
	for _, tc := range cases {
		if got := tc.seg.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", tc.seg, got, tc.want)
		}
	}
}

func TestMatcherFunc(t *testing.T) {
	mf := router.MatcherFunc(func(v string) bool { return v == "yes" })
	if !mf.Match("yes") {
		t.Errorf("MatcherFunc didn't accept yes")
	}
	if mf.Match("no") {
		t.Errorf("MatcherFunc accepted no")
	}
}

func TestMatch_ConcurrentSafe(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/about"),
		mkRoute(t, "/post/[id]"),
		mkRoute(t, "/docs/[...path]"),
	})
	done := make(chan struct{})
	for range 8 {
		go func() {
			for range 1000 {
				tree.Match("/about")
				tree.Match("/post/42")
				tree.Match("/docs/a/b/c")
			}
			done <- struct{}{}
		}()
	}
	for range 8 {
		<-done
	}
}

// TestMatch_HeapTruncateBoundary exercises matchState.truncate at the
// exact maxStackParams boundary after the overflow heap has been
// populated. With ten param segments where only the last differs,
// matching the second pattern forces backtracking that unwinds through
// truncate(to=9), truncate(to=8) and finally truncate(to<8). Regresses
// the previous tangled overC counter that could panic if drop > overC.
func TestMatch_HeapTruncateBoundary(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/[a]/[b]/[c]/[d]/[e]/[f]/[g]/[h]/[i]/x"),
		mkRoute(t, "/[a]/[b]/[c]/[d]/[e]/[f]/[g]/[h]/[i]/y"),
	})
	r, params, ok := tree.Match("/1/2/3/4/5/6/7/8/9/y")
	if !ok {
		t.Fatalf("no match for 10-segment path")
	}
	if r.Pattern != "/[a]/[b]/[c]/[d]/[e]/[f]/[g]/[h]/[i]/y" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	want := map[string]string{
		"a": "1", "b": "2", "c": "3", "d": "4", "e": "5",
		"f": "6", "g": "7", "h": "8", "i": "9",
	}
	for k, v := range want {
		if got := params[k]; got != v {
			t.Errorf("params[%q] = %q, want %q", k, got, v)
		}
	}
}

// TestMatch_HeapTruncateRebuild forces the heap branch to populate,
// fail, and then a fresh match on the same Tree to succeed. The
// underlying matchState is per-call but the test guards against state
// leaks between matches if the implementation ever pools it.
func TestMatch_HeapTruncateRebuild(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/[a]/[b]/[c]/[d]/[e]/[f]/[g]/[h]/[i]/x"),
		mkRoute(t, "/short/[id]"),
	})
	if _, _, ok := tree.Match("/1/2/3/4/5/6/7/8/9/nope"); ok {
		t.Fatalf("expected no match for 10-segment miss")
	}
	r, params, ok := tree.Match("/short/42")
	if !ok {
		t.Fatalf("no match for /short/42")
	}
	if r.Pattern != "/short/[id]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if params["id"] != "42" {
		t.Errorf("id = %q", params["id"])
	}
}
