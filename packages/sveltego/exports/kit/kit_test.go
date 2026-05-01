package kit_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func TestNewRenderCtx_InitializesFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/posts/42?x=1", nil)
	rw := httptest.NewRecorder()
	params := map[string]string{"slug": "42"}

	ctx := kit.NewRenderCtx(req, rw, params)

	if ctx == nil {
		t.Fatal("NewRenderCtx returned nil")
	}
	if ctx.Locals == nil {
		t.Error("Locals = nil, want non-nil")
	}
	if len(ctx.Locals) != 0 {
		t.Errorf("Locals len = %d, want 0", len(ctx.Locals))
	}
	if ctx.Cookies == nil {
		t.Error("Cookies = nil, want non-nil")
	}
	if ctx.Params["slug"] != "42" {
		t.Errorf("Params[slug] = %q, want 42", ctx.Params["slug"])
	}
	if ctx.Request != req {
		t.Error("Request not threaded through")
	}
	if ctx.Writer != rw {
		t.Error("Writer not threaded through")
	}
	if ctx.URL == nil || ctx.URL.Path != "/posts/42" {
		t.Errorf("URL = %v, want /posts/42", ctx.URL)
	}
}

func TestNewRenderCtx_NilRequest(t *testing.T) {
	ctx := kit.NewRenderCtx(nil, nil, nil)
	if ctx.URL != nil {
		t.Errorf("URL = %v, want nil for nil request", ctx.URL)
	}
	if ctx.Locals == nil {
		t.Error("Locals = nil, want non-nil")
	}
	if ctx.Cookies == nil {
		t.Error("Cookies = nil, want non-nil")
	}
}

func TestNewLoadCtx_InitializesFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com/login", nil)
	params := map[string]string{}

	ctx := kit.NewLoadCtx(req, params)

	if ctx == nil {
		t.Fatal("NewLoadCtx returned nil")
	}
	if ctx.Locals == nil {
		t.Error("Locals = nil, want non-nil")
	}
	if len(ctx.Locals) != 0 {
		t.Errorf("Locals len = %d, want 0", len(ctx.Locals))
	}
	if ctx.Cookies == nil {
		t.Error("Cookies = nil, want non-nil")
	}
	if ctx.Request != req {
		t.Error("Request not threaded through")
	}
	if ctx.URL == nil || ctx.URL.Path != "/login" {
		t.Errorf("URL = %v, want /login", ctx.URL)
	}
}

func TestNewLoadCtx_NilRequest(t *testing.T) {
	ctx := kit.NewLoadCtx(nil, nil)
	if ctx.URL != nil {
		t.Errorf("URL = %v, want nil for nil request", ctx.URL)
	}
	if ctx.Locals == nil {
		t.Error("Locals = nil, want non-nil")
	}
	if ctx.Cookies == nil {
		t.Error("Cookies = nil, want non-nil")
	}
}

func TestRenderCtx_LocalsWritable(t *testing.T) {
	ctx := kit.NewRenderCtx(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder(), nil)
	ctx.Locals["user"] = "alice"
	if got := ctx.Locals["user"]; got != "alice" {
		t.Errorf("Locals[user] = %v, want alice", got)
	}
}

func TestLoadCtx_LocalsWritable(t *testing.T) {
	ctx := kit.NewLoadCtx(httptest.NewRequest(http.MethodGet, "/", nil), nil)
	ctx.Locals["trace"] = 7
	if got := ctx.Locals["trace"]; got != 7 {
		t.Errorf("Locals[trace] = %v, want 7", got)
	}
}
