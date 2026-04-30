package fsutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/adapter-docker/internal/fsutil"
)

func TestWriteFileHappyPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dst := filepath.Join(tmp, "out.txt")
	if err := fsutil.WriteFile(dst, "hello docker", 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "hello docker" {
		t.Fatalf("dst = %q, want %q", got, "hello docker")
	}
}

// TestWriteFileSurfacesOpenError proves the helper returns a non-nil
// error when the destination cannot be opened. This implicitly exercises
// the deferred-close path that errors.Join is meant to feed: any close
// error is joined into the returned err, never silently dropped.
func TestWriteFileSurfacesOpenError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	// Path that already exists as a directory — OpenFile fails.
	dst := filepath.Join(tmp, "dst-dir")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := fsutil.WriteFile(dst, "data", 0o644)
	if err == nil {
		t.Fatalf("expected error when dst is a directory")
	}
	if !strings.Contains(err.Error(), dst) {
		t.Fatalf("err = %v, expected to mention destination", err)
	}
}
