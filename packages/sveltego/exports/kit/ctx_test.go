package kit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit"
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
