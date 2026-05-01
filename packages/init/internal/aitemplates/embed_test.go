package aitemplates

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFiles_ListMatchesEmbed(t *testing.T) {
	for _, name := range Files {
		if _, err := fs.ReadFile(FS, name); err != nil {
			t.Errorf("FS missing template %q: %v", name, err)
		}
	}
}

func TestTemplates_NonEmpty(t *testing.T) {
	for _, name := range Files {
		body, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		if len(body) == 0 {
			t.Errorf("template %q is empty", name)
		}
	}
}

// TestTemplates_MirrorTemplatesAI guards against drift between this
// embedded copy and the canonical templates/ai/ tree at the repo root.
// The check only runs when the source tree is reachable on disk
// (workspace mode); a fresh `go install @latest` from the proxy ships
// only this package and skips the check.
func TestTemplates_MirrorTemplatesAI(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Skip("runtime.Caller failed; cannot locate templates/ai")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	templatesAI := filepath.Join(repoRoot, "templates", "ai")
	if _, err := os.Stat(templatesAI); err != nil {
		t.Skipf("templates/ai not on disk (%v); skipping mirror check", err)
	}
	for _, name := range Files {
		want, err := os.ReadFile(filepath.Join(templatesAI, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read templates/ai/%s: %v", name, err)
		}
		got, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("read embed %s: %v", name, err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("packages/init/internal/aitemplates/files/%s drifted from templates/ai/%s — re-copy", name, name)
		}
	}
}
