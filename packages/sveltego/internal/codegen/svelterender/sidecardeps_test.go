package svelterender

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureSidecarDeps_FastPathWritableSource verifies the back-compat
// fast path: when the source dir already has node_modules/acorn, it is
// returned as-is and no cache materialization happens. This protects
// the existing dev-from-checkout workflow.
func TestEnsureSidecarDeps_FastPathWritableSource(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "index.mjs"), "// stub")
	mustWrite(t, filepath.Join(src, "package.json"), `{"name":"stub","version":"0.0.0"}`)
	if err := os.MkdirAll(filepath.Join(src, "node_modules", "acorn"), 0o755); err != nil {
		t.Fatalf("mk node_modules/acorn: %v", err)
	}
	got, err := EnsureSidecarDeps(src)
	if err != nil {
		t.Fatalf("EnsureSidecarDeps: %v", err)
	}
	if got != src {
		t.Fatalf("fast path should return src unchanged; got %s want %s", got, src)
	}
}

// TestEnsureSidecarDeps_EmptySrc rejects an empty srcDir argument
// rather than silently materializing into an unrelated cache slot.
func TestEnsureSidecarDeps_EmptySrc(t *testing.T) {
	t.Parallel()
	if _, err := EnsureSidecarDeps(""); err == nil {
		t.Fatal("expected error for empty srcDir")
	}
}

// TestEnsureSidecarDeps_MissingEntry rejects a srcDir without index.mjs
// — it is not a sidecar tree and we should not silently install npm
// deps in some random directory.
func TestEnsureSidecarDeps_MissingEntry(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	if _, err := EnsureSidecarDeps(src); err == nil {
		t.Fatal("expected error for missing index.mjs")
	}
}

// TestLockHash_StableAcrossRuns verifies the cache key is deterministic
// for identical inputs across runs, otherwise we'd materialize the
// sidecar tree on every build.
func TestLockHash_StableAcrossRuns(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "package.json"), `{"name":"x","version":"0.0.0"}`)
	mustWrite(t, filepath.Join(src, "package-lock.json"), `{"name":"x","lockfileVersion":3}`)
	a, err := lockHash(src)
	if err != nil {
		t.Fatalf("hash 1: %v", err)
	}
	b, err := lockHash(src)
	if err != nil {
		t.Fatalf("hash 2: %v", err)
	}
	if a != b {
		t.Fatalf("hash drift: %s vs %s", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("hash length = %d, want 16", len(a))
	}
}

// TestLockHash_ChangesWithLockfile verifies a package-lock.json bump
// invalidates the cache. Without this, dependency upgrades wouldn't
// trigger a fresh npm install.
func TestLockHash_ChangesWithLockfile(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "package.json"), `{"name":"x","version":"0.0.0"}`)
	mustWrite(t, filepath.Join(src, "package-lock.json"), `{"name":"x","lockfileVersion":3,"v":1}`)
	a, err := lockHash(src)
	if err != nil {
		t.Fatalf("hash 1: %v", err)
	}
	mustWrite(t, filepath.Join(src, "package-lock.json"), `{"name":"x","lockfileVersion":3,"v":2}`)
	b, err := lockHash(src)
	if err != nil {
		t.Fatalf("hash 2: %v", err)
	}
	if a == b {
		t.Fatal("hash should change when lockfile changes")
	}
}

// TestMaterializeSidecar_SkipsNodeModules verifies the source's
// node_modules tree (if any) is NOT copied to the cache — npm in the
// cache will populate its own.
func TestMaterializeSidecar_SkipsNodeModules(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "index.mjs"), "// entry")
	mustWrite(t, filepath.Join(src, "package.json"), "{}")
	if err := os.MkdirAll(filepath.Join(src, "node_modules", "garbage"), 0o755); err != nil {
		t.Fatalf("mk garbage: %v", err)
	}
	mustWrite(t, filepath.Join(src, "node_modules", "garbage", "stale.txt"), "STALE")
	dst := filepath.Join(t.TempDir(), "out")
	if err := materializeSidecar(src, dst); err != nil {
		t.Fatalf("materializeSidecar: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "index.mjs")); err != nil {
		t.Fatalf("entry not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "node_modules")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("node_modules should not be copied; stat err = %v", err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
