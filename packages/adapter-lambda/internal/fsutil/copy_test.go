package fsutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/adapter-lambda/internal/fsutil"
)

func TestWriteFileHappyPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	dst := filepath.Join(tmp, "out.txt")
	if err := fsutil.WriteFile(dst, "hello lambda", 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "hello lambda" {
		t.Fatalf("dst = %q, want %q", got, "hello lambda")
	}
}

// TestWriteFileSurfacesOpenError proves the helper returns a non-nil
// error when the destination cannot be opened, exercising the
// deferred-close path that errors.Join is meant to feed.
func TestWriteFileSurfacesOpenError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
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
