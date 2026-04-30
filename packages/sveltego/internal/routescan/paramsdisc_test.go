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
	writeFile(t, filepath.Join(dir, "int.go"), `package params
func Match(s string) bool { return s != "" }
`)
	writeFile(t, filepath.Join(dir, "uuid.go"), `package params
func Match(s string) bool { return len(s) == 36 }
`)

	matchers, diags := discoverMatchers(dir)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(matchers) != 2 {
		t.Fatalf("want 2 matchers, got %d", len(matchers))
	}
	if matchers[0].Name != "int" || matchers[1].Name != "uuid" {
		t.Fatalf("matchers not sorted: %+v", matchers)
	}
}

func TestDiscoverMatchersWrongSignature(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.go"), `package params
func Match(s string, n int) bool { return false }
`)

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
	writeFile(t, filepath.Join(dir, "noop.go"), `package params
func somethingElse() {}
`)

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
	writeFile(t, filepath.Join(dir, "int.go"), `package params
func Match(s string) bool { return true }
`)
	writeFile(t, filepath.Join(dir, "int_test.go"), `package params
import "testing"
func TestMatch(t *testing.T) {}
`)

	matchers, diags := discoverMatchers(dir)
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if len(matchers) != 1 || matchers[0].Name != "int" {
		t.Fatalf("want exactly the int matcher, got %+v", matchers)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
