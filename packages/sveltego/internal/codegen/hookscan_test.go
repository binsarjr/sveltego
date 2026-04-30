package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempHooks(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "hooks.server.go"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return root
}

func TestScanHooksServer_missingFile(t *testing.T) {
	t.Parallel()
	set, err := scanHooksServer(t.TempDir())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if set.Present() {
		t.Errorf("Present() = true on missing file, want false")
	}
	if set.Any() {
		t.Errorf("Any() = true on missing file, want false")
	}
}

func TestScanHooksServer_allHooks(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package src

import (
	"context"
	"net/http"
	"net/url"

	"github.com/binsarjr/sveltego/exports/kit"
)

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
	return resolve(ev)
}

func HandleError(ev *kit.RequestEvent, err error) kit.SafeError {
	return kit.SafeError{Code: 500}
}

func HandleFetch(ev *kit.RequestEvent, req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func Reroute(u *url.URL) string { return "" }

func Init(ctx context.Context) error { return nil }
`
	root := writeTempHooks(t, body)
	set, err := scanHooksServer(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !set.Present() || !set.Handle || !set.HandleError || !set.HandleFetch || !set.Reroute || !set.Init {
		t.Errorf("set = %+v, want all true", set)
	}
}

func TestScanHooksServer_partial(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package src

import "github.com/binsarjr/sveltego/exports/kit"

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
	return resolve(ev)
}
`
	root := writeTempHooks(t, body)
	set, err := scanHooksServer(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !set.Handle {
		t.Errorf("Handle = false, want true")
	}
	if set.HandleError || set.HandleFetch || set.Reroute || set.Init {
		t.Errorf("only Handle expected, got %+v", set)
	}
	if !set.Any() {
		t.Errorf("Any() = false, want true")
	}
}

func TestScanHooksServer_signatureMismatch(t *testing.T) {
	t.Parallel()
	body := `//go:build sveltego

package src

func Handle() {}
`
	root := writeTempHooks(t, body)
	if _, err := scanHooksServer(root); err == nil {
		t.Fatal("expected error on bad Handle signature")
	} else if !strings.Contains(err.Error(), "Handle must have signature") {
		t.Errorf("err = %v, want Handle signature message", err)
	}
}

func TestScanHooksServer_ignoresMethods(t *testing.T) {
	t.Parallel()
	// A receiver-bearing method named Handle must not register as a hook.
	body := `//go:build sveltego

package src

type holder struct{}

func (h holder) Handle() {}
`
	root := writeTempHooks(t, body)
	set, err := scanHooksServer(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if set.Handle {
		t.Error("method Handle must not register as hook")
	}
}
