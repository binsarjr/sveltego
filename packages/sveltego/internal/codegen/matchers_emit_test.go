package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/routescan"
)

func TestEmitMatchers_noUserMatchers_writesDefaultsOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".gen"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := emitMatchers(root, ".gen", "myapp", "gen", nil); err != nil {
		t.Fatalf("emitMatchers: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gen", "matchers.gen.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	mustParseGo(t, body)
	if !bytes.Contains(body, []byte("params.DefaultMatchers()")) {
		t.Fatalf("expected DefaultMatchers seed:\n%s", body)
	}
	if bytes.Contains(body, []byte("paramssrc")) {
		t.Fatalf("did not expect paramssrc import when no user matchers:\n%s", body)
	}
	// No mirror tree should exist.
	if _, err := os.Stat(filepath.Join(root, ".gen", "paramssrc")); !os.IsNotExist(err) {
		t.Errorf("paramssrc dir created unexpectedly: %v", err)
	}
}

func TestEmitMatchers_userMatcher_writesAdapterAndMirror(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "src", "params", "postslug")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package postslug

func Match(s string) bool { return s != "" }
`
	srcPath := filepath.Join(dir, "postslug.go")
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	matchers := []routescan.DiscoveredMatcher{{
		Name:        "postslug",
		Path:        srcPath,
		PackageName: "postslug",
	}}
	if err := emitMatchers(root, ".gen", "myapp", "gen", matchers); err != nil {
		t.Fatalf("emitMatchers: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, ".gen", "matchers.gen.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	mustParseGo(t, body)
	if !bytes.Contains(body, []byte(`m["postslug"] = router.MatcherFunc(matcher_postslug.Match)`)) {
		t.Fatalf("adapter missing matcher install:\n%s", body)
	}
	if !bytes.Contains(body, []byte(`"myapp/.gen/paramssrc/postslug"`)) {
		t.Fatalf("adapter missing paramssrc import:\n%s", body)
	}

	mirror, err := os.ReadFile(filepath.Join(root, ".gen", "paramssrc", "postslug", "postslug.go"))
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	mustParseGo(t, mirror)
	if !bytes.Contains(mirror, []byte("package postslug")) {
		t.Errorf("mirror package clause wrong:\n%s", mirror)
	}
	if !bytes.Contains(mirror, []byte("func Match(s string) bool")) {
		t.Errorf("mirror missing Match func:\n%s", mirror)
	}
}

func TestEmitMatchers_buildTagStripped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "src", "params", "hex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `//go:build sveltego

package hex

func Match(s string) bool { return len(s) == 6 }
`
	srcPath := filepath.Join(dir, "hex.go")
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	matchers := []routescan.DiscoveredMatcher{{Name: "hex", Path: srcPath, PackageName: "hex"}}
	if err := emitMatchers(root, ".gen", "myapp", "gen", matchers); err != nil {
		t.Fatalf("emitMatchers: %v", err)
	}
	mirror, err := os.ReadFile(filepath.Join(root, ".gen", "paramssrc", "hex", "hex.go"))
	if err != nil {
		t.Fatalf("read mirror: %v", err)
	}
	if bytes.Contains(mirror, []byte("//go:build sveltego")) {
		t.Fatalf("build tag not stripped:\n%s", mirror)
	}
}

func TestEmitMatchers_builtinShadowEmitsWarning(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, "src", "params", "int")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := `package int

func Match(s string) bool { return s != "" }
`
	srcPath := filepath.Join(dir, "int.go")
	if err := os.WriteFile(srcPath, []byte(src), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	matchers := []routescan.DiscoveredMatcher{{Name: "int", Path: srcPath, PackageName: "int"}}
	if err := emitMatchers(root, ".gen", "myapp", "gen", matchers); err != nil {
		t.Fatalf("emitMatchers: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gen", "matchers.gen.go"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	mustParseGo(t, body)
	if !bytes.Contains(body, []byte("overrides built-in matcher")) {
		t.Fatalf("expected override warning for built-in shadow:\n%s", body)
	}
	if !bytes.Contains(body, []byte(`m["int"] = router.MatcherFunc(matcher_int.Match)`)) {
		t.Fatalf("override assignment missing:\n%s", body)
	}
}

func TestGenerateMatchersAdapter_deterministic(t *testing.T) {
	t.Parallel()
	matchers := []routescan.DiscoveredMatcher{
		{Name: "postslug", Path: "/x/src/params/postslug/postslug.go", PackageName: "postslug"},
		{Name: "hex", Path: "/x/src/params/hex/hex.go", PackageName: "hex"},
	}
	a, err := generateMatchersAdapter("gen", "myapp", ".gen", matchers)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := generateMatchersAdapter("gen", "myapp", ".gen", matchers)
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("non-deterministic:\n--- a:\n%s\n--- b:\n%s", a, b)
	}
	// Imports must be sorted by matcher name regardless of input order.
	if !strings.Contains(string(a), "matcher_hex \"myapp/.gen/paramssrc/hex\"") {
		t.Fatalf("expected sorted import ordering:\n%s", a)
	}
}
