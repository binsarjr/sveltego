package fsutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/adapter-fastly/internal/fsutil"
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

func TestCopyFileDstIsDirectory(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.bin")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed src: %v", err)
	}
	dstDir := filepath.Join(tmp, "dst-as-dir")
	if err := os.Mkdir(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	err := fsutil.CopyFile(src, dstDir, 0o644)
	if err == nil {
		t.Fatalf("expected error when dst is a directory")
	}
	if !strings.Contains(err.Error(), dstDir) {
		t.Fatalf("err = %v, expected to mention destination path", err)
	}
}
