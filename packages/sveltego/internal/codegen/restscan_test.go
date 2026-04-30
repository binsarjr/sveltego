package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempServerRest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "server.go")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestScanRESTHandlers_allVerbs(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package api

import "github.com/binsarjr/sveltego/exports/kit"

func GET(ev *kit.RequestEvent) *kit.Response     { return nil }
func POST(ev *kit.RequestEvent) *kit.Response    { return nil }
func PUT(ev *kit.RequestEvent) *kit.Response     { return nil }
func PATCH(ev *kit.RequestEvent) *kit.Response   { return nil }
func DELETE(ev *kit.RequestEvent) *kit.Response  { return nil }
func OPTIONS(ev *kit.RequestEvent) *kit.Response { return nil }
func HEAD(ev *kit.RequestEvent) *kit.Response    { return nil }
`
	path := writeTempServerRest(t, body)
	got, err := scanRESTHandlers(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	want := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}
	if len(got.Verbs) != len(want) {
		t.Fatalf("verbs len = %d, want %d (%v)", len(got.Verbs), len(want), got.Verbs)
	}
	for i, v := range want {
		if got.Verbs[i] != v {
			t.Errorf("verbs[%d] = %q, want %q (full: %v)", i, got.Verbs[i], v, got.Verbs)
		}
	}
}

func TestScanRESTHandlers_unknownExportedFunc(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package api

import "github.com/binsarjr/sveltego/exports/kit"

func GET(ev *kit.RequestEvent) *kit.Response { return nil }
func Get(ev *kit.RequestEvent) *kit.Response { return nil }
`
	path := writeTempServerRest(t, body)
	if _, err := scanRESTHandlers(path); err == nil {
		t.Fatal("expected error on unknown exported function")
	} else if !strings.Contains(err.Error(), "unknown exported function") {
		t.Errorf("err = %v", err)
	}
}

func TestScanRESTHandlers_signatureMismatch(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package api

func GET() {}
`
	path := writeTempServerRest(t, body)
	if _, err := scanRESTHandlers(path); err == nil {
		t.Fatal("expected error on bad GET signature")
	} else if !strings.Contains(err.Error(), "func(ev *kit.RequestEvent)") {
		t.Errorf("err = %v", err)
	}
}

func TestScanRESTHandlers_unexportedIgnored(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package api

import "github.com/binsarjr/sveltego/exports/kit"

func GET(ev *kit.RequestEvent) *kit.Response { return helper(ev) }
func helper(ev *kit.RequestEvent) *kit.Response { return nil }
`
	path := writeTempServerRest(t, body)
	got, err := scanRESTHandlers(path)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(got.Verbs) != 1 || got.Verbs[0] != "GET" {
		t.Errorf("expected [GET], got %v", got.Verbs)
	}
}

func TestScanRESTHandlers_missingFile(t *testing.T) {
	t.Parallel()
	got, err := scanRESTHandlers(filepath.Join(t.TempDir(), "missing.go"))
	if err != nil {
		t.Fatalf("err on missing file: %v", err)
	}
	if len(got.Verbs) != 0 {
		t.Errorf("verbs on missing = %v", got.Verbs)
	}
}
