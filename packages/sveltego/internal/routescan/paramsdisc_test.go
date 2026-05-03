package routescan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverMatchersValid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkdirMatcher(t, dir, "intish", "package intish\nfunc Match(s string) bool { return s != \"\" }\n")
	mkdirMatcher(t, dir, "hex", "package hex\nfunc Match(s string) bool { return len(s) == 6 }\n")

	matchers, diags := discoverMatchers(dir)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(matchers) != 2 {
		t.Fatalf("want 2 matchers, got %d", len(matchers))
	}
	if matchers[0].Name != "hex" || matchers[1].Name != "intish" {
		t.Fatalf("matchers not sorted: %+v", matchers)
	}
	if matchers[0].PackageName != "hex" || matchers[1].PackageName != "intish" {
		t.Fatalf("expected package names to match dir names: %+v", matchers)
	}
}

func TestDiscoverMatchersWrongSignature(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkdirMatcher(t, dir, "bad", "package bad\nfunc Match(s string, n int) bool { return false }\n")

	matchers, diags := discoverMatchers(dir)
	if len(matchers) != 0 {
		t.Fatalf("expected zero matchers, got %+v", matchers)
	}
	if len(diags) != 1 {
		t.Fatalf("want 1 diagnostic, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "wrong signature") {
		t.Fatalf("unexpected diagnostic: %s", diags[0].String())
	}
}

func TestDiscoverMatchersMissingFunc(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkdirMatcher(t, dir, "noop", "package noop\nfunc somethingElse() {}\n")

	matchers, diags := discoverMatchers(dir)
	if len(matchers) != 0 {
		t.Fatalf("expected zero matchers, got %+v", matchers)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "missing func Match") {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
}

func TestDiscoverMatchersEmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	matchers, diags := discoverMatchers(dir)
	if len(matchers) != 0 || len(diags) != 0 {
		t.Fatalf("expected empty result, got matchers=%+v diags=%+v", matchers, diags)
	}
}

func TestDiscoverMatchersUnsetParamsDir(t *testing.T) {
	t.Parallel()
	matchers, diags := discoverMatchers("")
	if matchers != nil || diags != nil {
		t.Fatalf("expected nil result, got matchers=%+v diags=%+v", matchers, diags)
	}
}

func TestDiscoverMatchersIgnoresTestFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkdirMatcher(t, dir, "intish", "package intish\nfunc Match(s string) bool { return true }\n")
	writeFile(t, filepath.Join(dir, "intish", "intish_test.go"), `package intish
import "testing"
func TestMatch(t *testing.T) {}
`)

	matchers, diags := discoverMatchers(dir)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(matchers) != 1 || matchers[0].Name != "intish" {
		t.Fatalf("want exactly the intish matcher, got %+v", matchers)
	}
}

func TestDiscoverMatchersFlatLayoutDiagnoses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "stale.go"), `package params
func Match(s string) bool { return true }
`)

	matchers, diags := discoverMatchers(dir)
	if len(matchers) != 0 {
		t.Fatalf("flat-layout file must not surface a matcher: %+v", matchers)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "must live at src/params/stale/stale.go") {
		t.Fatalf("expected migration diagnostic, got %v", diags)
	}
}

func TestDiscoverMatchersPackageNameMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkdirMatcher(t, dir, "slug", "package wrongname\nfunc Match(s string) bool { return true }\n")

	matchers, diags := discoverMatchers(dir)
	if len(matchers) != 0 {
		t.Fatalf("mismatched package name must not surface a matcher: %+v", matchers)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "want package slug") {
		t.Fatalf("expected package-mismatch diagnostic, got %v", diags)
	}
}

func TestDiscoverMatchersMissingCanonicalFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hex"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "hex", "helpers.go"), `package hex
func helper() {}
`)

	matchers, diags := discoverMatchers(dir)
	if len(matchers) != 0 {
		t.Fatalf("expected zero matchers, got %+v", matchers)
	}
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "missing hex.go") {
		t.Fatalf("expected missing-canonical-file diagnostic, got %v", diags)
	}
}

func mkdirMatcher(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	writeFile(t, filepath.Join(dir, name+".go"), body)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
