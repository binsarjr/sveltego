package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_NoHooksFile_emitsDefaultStub(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")

	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, ".gen", "hooks.gen.go"))
	if err != nil {
		t.Fatalf("read hooks.gen.go: %v", err)
	}
	if !bytes.Contains(body, []byte("kit.DefaultHooks()")) {
		t.Errorf("expected DefaultHooks call, got:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, ".gen", "hookssrc")); !os.IsNotExist(err) {
		t.Errorf("hookssrc dir created without user file: %v", err)
	}
	assertParsesAsGo(t, filepath.Join(root, ".gen", "hooks.gen.go"))
}

func TestBuild_UserHooks_emitsAdapterAndMirror(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")
	writeFile(t, filepath.Join(root, "src", "hooks.server.go"),
		`//go:build sveltego

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
`)

	if _, err := Build(BuildOptions{ProjectRoot: root}); err != nil {
		t.Fatalf("Build: %v", err)
	}

	adapter := filepath.Join(root, ".gen", "hooks.gen.go")
	mirror := filepath.Join(root, ".gen", "hookssrc", "hooks_server.go")
	for _, p := range []string{adapter, mirror} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
		assertParsesAsGo(t, p)
	}
	body, err := os.ReadFile(adapter)
	if err != nil {
		t.Fatalf("read adapter: %v", err)
	}
	for _, want := range []string{
		`hookssrc "example.com/app/.gen/hookssrc"`,
		"Handle:      hookssrc.Handle",
		"HandleError: hookssrc.HandleError",
		"HandleFetch: hookssrc.HandleFetch",
		"Reroute:     hookssrc.Reroute",
		"Init:        hookssrc.Init",
		"h.WithDefaults()",
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("adapter missing %q:\n%s", want, body)
		}
	}
	mirrorBytes, err := os.ReadFile(mirror)
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	if bytes.Contains(mirrorBytes, []byte("//go:build")) {
		t.Errorf("mirror retained build constraint:\n%s", mirrorBytes)
	}
	if !bytes.Contains(mirrorBytes, []byte("package hookssrc")) {
		t.Errorf("mirror package clause = wrong:\n%s", mirrorBytes)
	}
}

func TestBuild_HookSignatureMismatch_failsBuild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	scaffoldProject(t, root, "example.com/app")
	writeFile(t, filepath.Join(root, "src", "hooks.server.go"),
		`//go:build sveltego

package src

func Handle() {}
`)

	if _, err := Build(BuildOptions{ProjectRoot: root}); err == nil {
		t.Fatal("expected error on bad Handle signature")
	}
}
