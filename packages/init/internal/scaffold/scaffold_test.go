package scaffold

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	aitemplates "github.com/binsarjr/sveltego/templates/ai"
)

func TestRun_BaseScaffold(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(Options{Dir: dir, Module: "example.com/hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Fatalf("unexpected skipped on fresh dir: %v", res.Skipped)
	}
	wantPaths := []string{
		"go.mod",
		"README.md",
		".gitignore",
		"sveltego.config.go",
		"hooks.server.go",
		"src/routes/+page.svelte",
		"src/routes/page.server.go",
		"src/routes/+layout.svelte",
		"src/lib/.gitkeep",
	}
	for _, p := range wantPaths {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}

	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	if !bytes.Contains(gomod, []byte("module example.com/hello")) {
		t.Errorf("go.mod missing module line, got: %s", gomod)
	}
}

func TestRun_AICopiesEmbedFSByteEqual(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{Dir: dir, AI: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, name := range aitemplates.Files {
		want, err := fs.ReadFile(aitemplates.FS, name)
		if err != nil {
			t.Fatalf("embed read %q: %v", name, err)
		}
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Errorf("read scaffolded %q: %v", name, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("template %q drifted from embed.FS bytes", name)
		}
	}
}

func TestRun_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.server.go")
	if err := os.WriteFile(hooksPath, []byte("// pre-existing\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := Run(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !contains(res.Skipped, "hooks.server.go") {
		t.Errorf("expected hooks.server.go in skipped, got %v", res.Skipped)
	}
	body, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	if string(body) != "// pre-existing\n" {
		t.Errorf("hooks.server.go was overwritten without --force; got %q", body)
	}
}

func TestRun_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.server.go")
	if err := os.WriteFile(hooksPath, []byte("// pre-existing\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := Run(Options{Dir: dir, Force: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if contains(res.Skipped, "hooks.server.go") {
		t.Errorf("hooks.server.go skipped despite --force: %v", res.Skipped)
	}
	body, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("read hooks: %v", err)
	}
	if !bytes.Contains(body, []byte("kit.HandleFn")) {
		t.Errorf("hooks.server.go not overwritten; got %q", body)
	}
}

func TestWriteAITemplates_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(target, []byte("local notes\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := WriteAITemplates(dir, false)
	if err != nil {
		t.Fatalf("WriteAITemplates: %v", err)
	}
	if !contains(res.Skipped, "CLAUDE.md") {
		t.Errorf("expected CLAUDE.md in skipped, got %v", res.Skipped)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if string(body) != "local notes\n" {
		t.Errorf("CLAUDE.md overwritten without --force; got %q", body)
	}
}

func TestRun_EmptyDirRejected(t *testing.T) {
	if _, err := Run(Options{}); err == nil {
		t.Errorf("expected error on empty Dir")
	}
}

func contains(xs []string, s string) bool {
	i := sort.SearchStrings(xs, s)
	return i < len(xs) && xs[i] == s
}
