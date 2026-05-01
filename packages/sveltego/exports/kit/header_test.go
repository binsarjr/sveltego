package kit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// TestHeaderWriter_Set pins the replace-all semantics: a second Set on the
// same key must overwrite the first value, leaving exactly one entry.
func TestHeaderWriter_Set(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	ctx.Header().Set("X-Foo", "first")
	ctx.Header().Set("X-Foo", "second")

	got := ctx.CollectHeaders().Get("X-Foo")
	if got != "second" {
		t.Fatalf("Set twice: got %q, want %q", got, "second")
	}
	if n := len(ctx.CollectHeaders()["X-Foo"]); n != 1 {
		t.Fatalf("Set twice: %d values, want 1", n)
	}
}

// TestHeaderWriter_Add pins the append semantics: successive Add calls on
// the same key accumulate values — matching net/http.Header.Add behavior.
// This is the correct path for Set-Cookie, Vary, and Link headers.
func TestHeaderWriter_Add(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	ctx.Header().Add("Link", `</a.css>; rel=preload`)
	ctx.Header().Add("Link", `</b.css>; rel=preload`)

	vals := ctx.CollectHeaders()["Link"]
	if len(vals) != 2 {
		t.Fatalf("Add twice: %d values, want 2; got %v", len(vals), vals)
	}
	if vals[0] != `</a.css>; rel=preload` {
		t.Errorf("vals[0] = %q, want </a.css>; rel=preload", vals[0])
	}
	if vals[1] != `</b.css>; rel=preload` {
		t.Errorf("vals[1] = %q, want </b.css>; rel=preload", vals[1])
	}
}

// TestHeaderWriter_Del confirms Del removes all values for a key without
// disturbing other keys.
func TestHeaderWriter_Del(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	ctx.Header().Set("X-Keep", "yes")
	ctx.Header().Add("X-Remove", "a")
	ctx.Header().Add("X-Remove", "b")
	ctx.Header().Del("X-Remove")

	if v := ctx.CollectHeaders().Get("X-Keep"); v != "yes" {
		t.Errorf("X-Keep after Del = %q, want %q", v, "yes")
	}
	if vals := ctx.CollectHeaders()["X-Remove"]; len(vals) != 0 {
		t.Errorf("X-Remove after Del = %v, want empty", vals)
	}
}

// TestHeaderWriter_SetThenAdd verifies that Set followed by Add yields
// exactly two values: the set one and the appended one, in that order.
func TestHeaderWriter_SetThenAdd(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	ctx.Header().Set("Vary", "Accept-Encoding")
	ctx.Header().Add("Vary", "Accept-Language")

	vals := ctx.CollectHeaders()["Vary"]
	if len(vals) != 2 {
		t.Fatalf("Set+Add: %d values, want 2; got %v", len(vals), vals)
	}
	if vals[0] != "Accept-Encoding" {
		t.Errorf("vals[0] = %q, want Accept-Encoding", vals[0])
	}
	if vals[1] != "Accept-Language" {
		t.Errorf("vals[1] = %q, want Accept-Language", vals[1])
	}
}

// TestHeaderWriter_LazyInit verifies that calling CollectHeaders on a
// fresh LoadCtx without ever calling Header() returns nil (no allocation).
func TestHeaderWriter_LazyInit(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	if h := ctx.CollectHeaders(); h != nil {
		t.Fatalf("CollectHeaders on untouched LoadCtx = %v, want nil", h)
	}
}

// TestHeaderWriter_MultipleCallsShareMap confirms that multiple calls to
// Header() on the same LoadCtx return writers backed by the same map, so
// mutations from any call site are visible to CollectHeaders.
func TestHeaderWriter_MultipleCallsShareMap(t *testing.T) {
	t.Parallel()
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)

	ctx.Header().Set("X-A", "1")
	ctx.Header().Add("X-B", "2")

	h := ctx.CollectHeaders()
	if h.Get("X-A") != "1" || h.Get("X-B") != "2" {
		t.Fatalf("headers not shared: got %v", h)
	}
}
