package codegen

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempServerActions(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "_page.server.go")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestScanActions_LiteralKeys(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package login

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

var Actions = kit.ActionMap{
	"default": func(ev *kit.RequestEvent) kit.ActionResult { return nil },
	"submit":  func(ev *kit.RequestEvent) kit.ActionResult { return nil },
}
`
	path := writeTempServerActions(t, body)
	got, err := scanActions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got.HasActions {
		t.Fatal("HasActions = false")
	}
	if len(got.Names) != 2 || got.Names[0] != "default" || got.Names[1] != "submit" {
		t.Errorf("Names = %v, want [default submit]", got.Names)
	}
}

func TestScanActions_NoVar(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package x

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (any, error) { return nil, nil }
`
	path := writeTempServerActions(t, body)
	got, err := scanActions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got.HasActions {
		t.Errorf("HasActions = true, want false")
	}
}

func TestScanActions_DynamicKeysOmitted(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package x

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

var Actions kit.ActionMap
`
	path := writeTempServerActions(t, body)
	got, err := scanActions(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !got.HasActions {
		t.Fatal("HasActions = false")
	}
	if len(got.Names) != 0 {
		t.Errorf("Names = %v, want empty", got.Names)
	}
}

func TestScanActions_MissingFile(t *testing.T) {
	t.Parallel()
	got, err := scanActions(filepath.Join(t.TempDir(), "missing.go"))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.HasActions {
		t.Errorf("HasActions on missing = true")
	}
}
