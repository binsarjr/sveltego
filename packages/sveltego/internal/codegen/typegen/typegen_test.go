package typegen

import (
	"os"
	"path/filepath"
	"testing"
)

// scenario maps a testdata fixture directory to the kind of source
// expected inside it. Every fixture ships a sibling expected.d.ts
// golden file the test compares emitter output against byte-for-byte.
var scenarios = []struct {
	name string
	kind Kind
}{
	{"primitives", KindPage},
	{"nested", KindPage},
	{"streamed", KindPage},
	{"pointers", KindPage},
	{"maps", KindPage},
	{"timetime", KindPage},
	{"layout", KindLayout},
	{"jsontag", KindPage},
}

func TestEmitForRoute_Goldens(t *testing.T) {
	t.Parallel()
	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			workDir := stagingCopy(t, filepath.Join("testdata", sc.name))
			out, diags, err := EmitForRoute(EmitOptions{RouteDir: workDir, Kind: sc.kind})
			if err != nil {
				t.Fatalf("EmitForRoute: %v (diags=%v)", err, diags)
			}
			if out == "" {
				t.Fatalf("expected file emitted, got empty path")
			}
			got, err := os.ReadFile(out) //nolint:gosec // test path
			if err != nil {
				t.Fatalf("read emitted: %v", err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", sc.name, "expected.d.ts"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if string(got) != string(want) {
				t.Fatalf("output mismatch\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
		})
	}
}

// TestEmitForRoute_Drift runs the emitter twice in a row over a
// staged copy of each fixture and asserts identical output. RFC #379
// phase 2 mandates deterministic emission so editor-watcher rerunning
// builds never produces a no-op .d.ts diff.
func TestEmitForRoute_Drift(t *testing.T) {
	t.Parallel()
	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			workDir := stagingCopy(t, filepath.Join("testdata", sc.name))
			first, _, err := EmitForRoute(EmitOptions{RouteDir: workDir, Kind: sc.kind})
			if err != nil {
				t.Fatalf("first emit: %v", err)
			}
			body1, err := os.ReadFile(first) //nolint:gosec // test path
			if err != nil {
				t.Fatalf("read first: %v", err)
			}
			second, _, err := EmitForRoute(EmitOptions{RouteDir: workDir, Kind: sc.kind})
			if err != nil {
				t.Fatalf("second emit: %v", err)
			}
			body2, err := os.ReadFile(second) //nolint:gosec // test path
			if err != nil {
				t.Fatalf("read second: %v", err)
			}
			if string(body1) != string(body2) {
				t.Fatalf("non-deterministic output\nfirst:\n%s\nsecond:\n%s", body1, body2)
			}
		})
	}
}

// TestEmitForRoute_NoSource verifies that a route directory without a
// `_page.server.go` is a silent no-op: routes that do not load server
// data must not emit a misleading empty .d.ts.
func TestEmitForRoute_NoSource(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, diags, err := EmitForRoute(EmitOptions{RouteDir: dir, Kind: KindPage})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("expected no output, got %q", out)
	}
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %v", diags)
	}
}

// stagingCopy replicates a fixture directory into t.TempDir so the
// emitter writes its `.d.ts` somewhere disposable. The original
// fixtures stay untouched and golden tests can rerun without dirtying
// the worktree.
func stagingCopy(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read fixture %s: %v", src, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := os.ReadFile(filepath.Join(src, e.Name())) //nolint:gosec // test path
		if err != nil {
			t.Fatalf("read fixture file %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), body, 0o600); err != nil {
			t.Fatalf("write staged %s: %v", e.Name(), err)
		}
	}
	return dst
}
