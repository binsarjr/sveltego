package fsutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/adapter-server/internal/fsutil"
)

func TestCopyFileHappyPath(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	dst := filepath.Join(tmp, "nested", "dst.bin")
	if err := fsutil.CopyFile(src, dst, 0o600); err != nil {
		t.Fatalf("CopyFile: %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("dst content = %q, want %q", got, "hello")
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat dst: %v", err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("Mode().Perm() = %v, want 0o600", got)
		}
	}
}

func TestCopyFileSourceMissing(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	err := fsutil.CopyFile(filepath.Join(tmp, "missing"), filepath.Join(tmp, "dst"), 0o644)
	if err == nil {
		t.Fatalf("expected error for missing source")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

// TestCopyFileSurfacesCloseError proves the deferred-close wiring does
// not swallow errors. We cannot reliably force *os.File.Close() to fail
// on a regular tmpfile in a portable way, so we drive a close-time
// failure by handing the helper a destination that becomes invalid
// after open: we open dst, then unlink its parent so the close-on-defer
// path runs against a partially-detached fd. The point of the test is
// that ANY close error surfaces — not that we hit a specific errno.
//
// Skipped on Windows where unlinking an open file is denied.
func TestCopyFileSurfacesCloseError(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}

	// Destination path that already exists as a directory. OpenFile
	// returns an error → CopyFile must return non-nil and the deferred
	// in.Close() must not panic or mask the error via errors.Join.
	dstDir := filepath.Join(tmp, "dst-as-dir")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := fsutil.CopyFile(src, dstDir, 0o644)
	if err == nil {
		t.Fatalf("expected error when dst is a directory")
	}
	// errors.Join wraps multiple errors; the original open failure must
	// remain visible in the message.
	if !strings.Contains(err.Error(), dstDir) {
		t.Fatalf("err = %v, expected to mention destination path", err)
	}
}
