package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeTB captures pass/fail signals so we can test Equal without aborting the
// real *testing.T. Implements only the testing.TB methods Equal touches.
type fakeTB struct {
	testing.TB
	mu       sync.Mutex
	helpers  int
	errMsg   string
	fatalMsg string
	failed   bool
}

func (f *fakeTB) Helper() {
	f.mu.Lock()
	f.helpers++
	f.mu.Unlock()
}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errMsg = fmt.Sprintf(format, args...)
	f.failed = true
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fatalMsg = fmt.Sprintf(format, args...)
	f.failed = true
}

func (f *fakeTB) Name() string { return "fake" }

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Fatalf("restore chdir: %v", err)
		}
	})
}

func writeFixture(t *testing.T, name string, content []byte) {
	t.Helper()
	p := goldenPath(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestEqual_Match(t *testing.T) {
	chdir(t, t.TempDir())
	writeFixture(t, "match", []byte("hello\n"))

	f := &fakeTB{}
	Equal(f, "match", []byte("hello\n"))

	if f.failed {
		t.Fatalf("expected pass, got failure: err=%q fatal=%q", f.errMsg, f.fatalMsg)
	}
	if f.helpers == 0 {
		t.Fatalf("expected Helper() to be called")
	}
}

func TestEqual_Mismatch(t *testing.T) {
	chdir(t, t.TempDir())
	writeFixture(t, "mismatch", []byte("want\n"))

	f := &fakeTB{}
	Equal(f, "mismatch", []byte("got\n"))

	if !f.failed {
		t.Fatalf("expected failure")
	}
	if f.fatalMsg != "" {
		t.Fatalf("expected Errorf not Fatalf, got fatal=%q", f.fatalMsg)
	}
	if !strings.Contains(f.errMsg, "mismatch") {
		t.Fatalf("expected error to mention name, got %q", f.errMsg)
	}
	if !strings.Contains(f.errMsg, "-args -update") {
		t.Fatalf("expected reproduction hint, got %q", f.errMsg)
	}
}

func TestEqual_Update(t *testing.T) {
	chdir(t, t.TempDir())
	writeFixture(t, "upd", []byte("old\n"))

	t.Setenv(envUpdate, "1")

	f := &fakeTB{}
	Equal(f, "upd", []byte("new\n"))

	if f.failed {
		t.Fatalf("update mode should not fail: err=%q fatal=%q", f.errMsg, f.fatalMsg)
	}
	got, err := os.ReadFile(goldenPath("upd"))
	if err != nil {
		t.Fatalf("read after update: %v", err)
	}
	if string(got) != "new\n" {
		t.Fatalf("file not rewritten: got %q", got)
	}
}

func TestEqual_MissingFile(t *testing.T) {
	chdir(t, t.TempDir())

	f := &fakeTB{}
	Equal(f, "missing", []byte("x"))

	if !f.failed {
		t.Fatalf("expected failure on missing fixture")
	}
	if f.fatalMsg == "" {
		t.Fatalf("expected Fatalf for missing fixture, err=%q", f.errMsg)
	}
	if !strings.Contains(f.fatalMsg, "missing") {
		t.Fatalf("expected fatal to mention path, got %q", f.fatalMsg)
	}
}

func TestEqual_MissingFile_Update(t *testing.T) {
	chdir(t, t.TempDir())
	t.Setenv(envUpdate, "1")

	f := &fakeTB{}
	Equal(f, "codegen/nested", []byte("created\n"))

	if f.failed {
		t.Fatalf("update should create missing fixture: err=%q fatal=%q", f.errMsg, f.fatalMsg)
	}
	got, err := os.ReadFile(goldenPath("codegen/nested"))
	if err != nil {
		t.Fatalf("read created fixture: %v", err)
	}
	if string(got) != "created\n" {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestEqual_CRLFNormalization(t *testing.T) {
	chdir(t, t.TempDir())
	writeFixture(t, "crlf", []byte("a\r\nb\r\nc\r\n"))

	f := &fakeTB{}
	Equal(f, "crlf", []byte("a\nb\nc\n"))

	if f.failed {
		t.Fatalf("CRLF expected file should match LF got: err=%q fatal=%q", f.errMsg, f.fatalMsg)
	}
}

func TestEqualString(t *testing.T) {
	chdir(t, t.TempDir())
	writeFixture(t, "str", []byte("svelte\n"))

	f := &fakeTB{}
	EqualString(f, "str", "svelte\n")

	if f.failed {
		t.Fatalf("string wrapper failed: err=%q fatal=%q", f.errMsg, f.fatalMsg)
	}
}

func TestGoldenPath_Subdirs(t *testing.T) {
	got := goldenPath("codegen/each-simple")
	want := filepath.Join("testdata", "golden", "codegen", "each-simple.golden")
	if got != want {
		t.Fatalf("goldenPath: got %q want %q", got, want)
	}
}
