package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeServerFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanLocalsAccess_DetectsCtxLocals(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	body := `//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (any, error) {
	user := ctx.Locals["user"]
	_ = user
	return nil, nil
}
`
	p := writeServerFile(t, tmp, "page.server.go", body)
	diags, err := scanLocalsAccessUnderPrerender(p)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("diags = %d, want 1: %+v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "Locals accessed") {
		t.Errorf("message = %q", diags[0].Message)
	}
}

func TestScanLocalsAccess_DirectiveSuppresses(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	body := `//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

//sveltego:allow-locals-prerender
func Load(ctx *kit.LoadCtx) (any, error) {
	_ = ctx.Locals
	return nil, nil
}
`
	p := writeServerFile(t, tmp, "page.server.go", body)
	diags, err := scanLocalsAccessUnderPrerender(p)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("expected directive to suppress; got %+v", diags)
	}
}

func TestScanLocalsAccess_NoLoadFnNoDiags(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	body := `//go:build sveltego

package routes

var Marker = 1
`
	p := writeServerFile(t, tmp, "page.server.go", body)
	diags, err := scanLocalsAccessUnderPrerender(p)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(diags) != 0 {
		t.Errorf("expected 0 diags, got %+v", diags)
	}
}

func TestScanLocalsAccess_LayoutLoad(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	body := `//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func LayoutLoad(ctx *kit.LoadCtx) (any, error) {
	if v, ok := ctx.Locals["nonce"]; ok {
		_ = v
	}
	return nil, nil
}
`
	p := writeServerFile(t, tmp, "layout.server.go", body)
	diags, err := scanLocalsAccessUnderPrerender(p)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("LayoutLoad diags = %d, want 1", len(diags))
	}
}
