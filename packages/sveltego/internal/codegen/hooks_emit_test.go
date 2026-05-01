package codegen

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmitHooks_noUserFile_writesDefaultStub(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".gen"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := emitHooks(root, ".gen", "myapp", "gen", HookSet{}); err != nil {
		t.Fatalf("emitHooks: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gen", "hooks.gen.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Contains(body, []byte("kit.DefaultHooks()")) {
		t.Fatalf("expected DefaultHooks call:\n%s", body)
	}
	if bytes.Contains(body, []byte("hookssrc")) {
		t.Fatalf("did not expect hookssrc import:\n%s", body)
	}
	mustParseGo(t, body)

	// No mirror should exist.
	if _, err := os.Stat(filepath.Join(root, ".gen", "hookssrc")); !os.IsNotExist(err) {
		t.Errorf("hookssrc dir created unexpectedly: %v", err)
	}
}

func TestEmitHooks_userHandleOnly_writesAdapterAndMirror(t *testing.T) {
	t.Parallel()
	root := writeTempHooks(t, `//go:build sveltego

package src

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
	return resolve(ev)
}
`)
	if err := os.MkdirAll(filepath.Join(root, ".gen"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	set, err := scanHooksServer(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if err := emitHooks(root, ".gen", "myapp", "gen", set); err != nil {
		t.Fatalf("emitHooks: %v", err)
	}

	adapter, err := os.ReadFile(filepath.Join(root, ".gen", "hooks.gen.go"))
	if err != nil {
		t.Fatalf("read adapter: %v", err)
	}
	mustParseGo(t, adapter)
	if !bytes.Contains(adapter, []byte("hookssrc.Handle")) {
		t.Errorf("adapter missing Handle wire:\n%s", adapter)
	}
	if !bytes.Contains(adapter, []byte("h.WithDefaults()")) {
		t.Errorf("adapter missing WithDefaults call:\n%s", adapter)
	}
	if bytes.Contains(adapter, []byte("hookssrc.HandleError")) {
		t.Errorf("adapter wired non-existent HandleError:\n%s", adapter)
	}

	mirror, err := os.ReadFile(filepath.Join(root, ".gen", "hookssrc", "hooks_server.go"))
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	mustParseGo(t, mirror)
	if !bytes.Contains(mirror, []byte("package hookssrc")) {
		t.Errorf("mirror package clause wrong:\n%s", mirror)
	}
	if bytes.Contains(mirror, []byte("//go:build sveltego")) {
		t.Errorf("build tag not stripped in mirror:\n%s", mirror)
	}
}

func TestEmitHooks_emptyUserFile_defaultsOnly(t *testing.T) {
	t.Parallel()
	// File present but declares no recognized hooks: emitter must fall
	// through to the default stub and skip the mirror.
	root := writeTempHooks(t, `//go:build sveltego

package src

func helper() {}
`)
	if err := os.MkdirAll(filepath.Join(root, ".gen"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	set, err := scanHooksServer(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !set.Present() || set.Any() {
		t.Fatalf("set = %+v, want present-but-empty", set)
	}
	if err := emitHooks(root, ".gen", "myapp", "gen", set); err != nil {
		t.Fatalf("emitHooks: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gen", "hooks.gen.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Contains(body, []byte("kit.DefaultHooks()")) {
		t.Fatalf("expected DefaultHooks call:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, ".gen", "hookssrc")); !os.IsNotExist(err) {
		t.Errorf("hookssrc must not exist: %v", err)
	}
}

func TestGenerateHooksAdapter_deterministic(t *testing.T) {
	t.Parallel()
	set := HookSet{
		SourcePath:  "/tmp/x/src/hooks.server.go",
		Handle:      true,
		HandleError: true,
		Init:        true,
	}
	a, err := generateHooksAdapter("gen", "myapp", ".gen", set)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := generateHooksAdapter("gen", "myapp", ".gen", set)
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("non-deterministic:\n--- a:\n%s\n--- b:\n%s", a, b)
	}
}

func TestGenerateHooksAdapter_goldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		set  HookSet
	}{
		{name: "no-user-file", set: HookSet{}},
		{name: "all-hooks", set: HookSet{
			SourcePath:  "/x/src/hooks.server.go",
			Handle:      true,
			HandleError: true,
			HandleFetch: true,
			Reroute:     true,
			Init:        true,
		}},
		{name: "handle-only", set: HookSet{
			SourcePath: "/x/src/hooks.server.go",
			Handle:     true,
		}},
		{name: "init-only", set: HookSet{
			SourcePath: "/x/src/hooks.server.go",
			Init:       true,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := generateHooksAdapter("gen", "myapp", ".gen", tc.set)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			assertHooksGolden(t, tc.name, got)
		})
	}
}

func assertHooksGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", "hooks", name+".golden")
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with GOLDEN_UPDATE=1): %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden mismatch in %s; run GOLDEN_UPDATE=1\n--- want:\n%s\n--- got:\n%s", path, want, got)
	}
}

func mustParseGo(t *testing.T, src []byte) {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.AllErrors); err != nil {
		t.Fatalf("emitted source does not parse: %v\n%s", err, indented(src))
	}
}

func indented(b []byte) string {
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		lines[i] = "\t| " + l
	}
	return strings.Join(lines, "\n")
}
