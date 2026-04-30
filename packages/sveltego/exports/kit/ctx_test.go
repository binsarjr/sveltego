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
