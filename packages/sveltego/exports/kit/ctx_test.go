package kit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// TestLoadCtx_Parent_NilWhenEmpty pins the no-parent semantics. A bare
// LoadCtx must report nil so user code asserting against typed parents
// can short-circuit on a missing layer.
func TestLoadCtx_Parent_NilWhenEmpty(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	if got := ctx.Parent(); got != nil {
		t.Fatalf("Parent on fresh LoadCtx = %v, want nil", got)
	}
}

// TestLoadCtx_Parent_ReturnsImmediateParent pins the single-level
// semantics: Parent returns the most recently pushed value, not the
// outermost. Children read only their direct parent; cross-layer access
// is intentionally out of scope for the MVP.
func TestLoadCtx_Parent_ReturnsImmediateParent(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	type root struct{ User string }
	type section struct{ Org string }

	ctx.PushParent(root{User: "alice"})
	if got, ok := ctx.Parent().(root); !ok || got.User != "alice" {
		t.Fatalf("after first push, Parent = %v ok=%v, want root{alice}", ctx.Parent(), ok)
	}

	ctx.PushParent(section{Org: "acme"})
	if got, ok := ctx.Parent().(section); !ok || got.Org != "acme" {
		t.Fatalf("after second push, Parent = %v ok=%v, want section{acme}", ctx.Parent(), ok)
	}
}

// TestLoadCtx_PushParent_PreservesOrder confirms successive pushes
// stack rather than overwrite. The pipeline depends on outer→inner push
// order producing inner-most data on top.
func TestLoadCtx_PushParent_PreservesOrder(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	ctx.PushParent("outer")
	ctx.PushParent("middle")
	ctx.PushParent("inner")

	if got := ctx.Parent(); got != "inner" {
		t.Fatalf("Parent after three pushes = %v, want \"inner\"", got)
	}
}

// TestLoadCtx_Param_DecodesPercent pins that Param returns the
// URL-decoded value for a percent-encoded route segment.
func TestLoadCtx_Param_DecodesPercent(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{
		"slug": "hello world",
	})
	got, ok := ctx.Param("slug")
	if !ok {
		t.Fatal("Param returned ok=false, want true")
	}
	if got != "hello world" {
		t.Fatalf("Param = %q, want %q", got, "hello world")
	}
}

// TestLoadCtx_RawParam_ReturnsEncodedSpace pins that RawParam returns
// the percent-encoded value when the decoded Param contains a space.
// This mirrors sveltejs/kit#12492: callers that need to forward the raw
// segment (e.g. building a cache key) must not re-encode a decoded value.
func TestLoadCtx_RawParam_ReturnsEncodedSpace(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{
		"slug": "hello world",
	})
	ctx.RawParams = map[string]string{"slug": "hello%20world"}

	got, ok := ctx.RawParam("slug")
	if !ok {
		t.Fatal("RawParam returned ok=false, want true")
	}
	if got != "hello%20world" {
		t.Fatalf("RawParam = %q, want %q", got, "hello%20world")
	}
}

// TestLoadCtx_RawParam_ReturnsEncodedSlash pins that RawParam preserves
// %2F in a segment; the decoded Param sees "/" but callers that need to
// distinguish a literal slash from a path separator require the raw form.
func TestLoadCtx_RawParam_ReturnsEncodedSlash(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), map[string]string{
		"path": "a/b",
	})
	ctx.RawParams = map[string]string{"path": "a%2Fb"}

	got, ok := ctx.RawParam("path")
	if !ok {
		t.Fatal("RawParam returned ok=false, want true")
	}
	if got != "a%2Fb" {
		t.Fatalf("RawParam = %q, want %q", got, "a%2Fb")
	}
}

// TestLoadCtx_RawParam_MissingParam pins that RawParam returns ("", false)
// when the name is not a capture in the matched route.
func TestLoadCtx_RawParam_MissingParam(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	ctx.RawParams = map[string]string{"id": "42"}

	got, ok := ctx.RawParam("missing")
	if ok {
		t.Fatalf("RawParam for missing key returned ok=true, value=%q", got)
	}
	if got != "" {
		t.Fatalf("RawParam for missing key = %q, want %q", got, "")
	}
}

// TestLoadCtx_RawParam_NilRawParams confirms that RawParam is safe when
// RawParams is nil (e.g. static routes with no captures).
func TestLoadCtx_RawParam_NilRawParams(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	// RawParams deliberately left nil.

	got, ok := ctx.RawParam("id")
	if ok {
		t.Fatalf("RawParam on nil RawParams returned ok=true, value=%q", got)
	}
	if got != "" {
		t.Fatalf("RawParam on nil RawParams = %q, want %q", got, "")
	}
}

// TestLoadCtx_Speculative_FalseForRealNav pins the common case: a plain
// GET with no preload header must return false.
func TestLoadCtx_Speculative_FalseForRealNav(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	ctx := kit.NewLoadCtx(r, nil)
	if ctx.Speculative() {
		t.Fatal("Speculative() = true for plain GET, want false")
	}
}

// TestLoadCtx_Speculative_TrueForSveltegoHeader pins the sveltego-specific
// client header: X-Sveltego-Preload: 1 must flip Speculative to true.
func TestLoadCtx_Speculative_TrueForSveltegoHeader(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/dashboard/__data.json", nil)
	r.Header.Set("X-Sveltego-Preload", "1")
	ctx := kit.NewLoadCtx(r, nil)
	if !ctx.Speculative() {
		t.Fatal("Speculative() = false with X-Sveltego-Preload: 1, want true")
	}
}

// TestLoadCtx_Speculative_TrueForSecPurposePrefetch pins the standard HTTP
// browser hint: Sec-Purpose containing "prefetch" must flip Speculative to true.
func TestLoadCtx_Speculative_TrueForSecPurposePrefetch(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/dashboard/__data.json", nil)
	r.Header.Set("Sec-Purpose", "prefetch")
	ctx := kit.NewLoadCtx(r, nil)
	if !ctx.Speculative() {
		t.Fatal("Speculative() = false with Sec-Purpose: prefetch, want true")
	}
}

// TestLoadCtx_Speculative_TrueForSecPurposeWithTokens confirms that
// Sec-Purpose carrying extra tokens (e.g. "prefetch;prerender") still
// triggers Speculative, matching the "contains prefetch" substring rule.
func TestLoadCtx_Speculative_TrueForSecPurposeWithTokens(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest(http.MethodGet, "/dashboard/__data.json", nil)
	r.Header.Set("Sec-Purpose", "prefetch;prerender")
	ctx := kit.NewLoadCtx(r, nil)
	if !ctx.Speculative() {
		t.Fatal("Speculative() = false with Sec-Purpose: prefetch;prerender, want true")
	}
}

// TestLoadCtx_Speculative_FalseWithNilRequest guards the nil-request edge
// case (e.g. direct construction in unit tests) — must return false, not panic.
func TestLoadCtx_Speculative_FalseWithNilRequest(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(nil, nil)
	if ctx.Speculative() {
		t.Fatal("Speculative() = true with nil request, want false")
	}
}

// TestLoadCtx_Depends_RecordsTags pins the basic accumulation path: each
// Depends() call appends to the tag set so the pipeline can ship the
// union to the client.
func TestLoadCtx_Depends_RecordsTags(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	if got := ctx.CollectDeps(); len(got) != 0 {
		t.Fatalf("fresh ctx CollectDeps = %v, want empty", got)
	}
	ctx.Depends("posts:list")
	ctx.Depends("user:42", "session")
	got := ctx.CollectDeps()
	want := []string{"posts:list", "user:42", "session"}
	if len(got) != len(want) {
		t.Fatalf("CollectDeps = %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("CollectDeps[%d] = %q, want %q", i, got[i], w)
		}
	}
}

// TestLoadCtx_OnLeave_NoOpOnServer pins the server-side contract from
// #172: OnLeave is registered for surface symmetry but must never invoke
// the callback during the request lifetime. Cleanup belongs to the
// client SPA router; on the server, defer is the right tool.
func TestLoadCtx_OnLeave_NoOpOnServer(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	called := false
	ctx.OnLeave(func() { called = true })
	if called {
		t.Fatal("OnLeave callback fired on server; want no-op")
	}
}

// TestLoadCtx_OnLeave_NilCallbackSafe confirms the no-op tolerates a nil
// callback — useful when user code conditionally registers a cleanup.
func TestLoadCtx_OnLeave_NilCallbackSafe(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("OnLeave(nil) panicked: %v", r)
		}
	}()
	ctx.OnLeave(nil)
}
