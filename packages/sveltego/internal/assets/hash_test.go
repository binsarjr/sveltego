package assets_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/binsarjr/sveltego/internal/assets"
)

func TestBuild_EmptyProjectIsNotAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plan, err := assets.Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(plan.URLs) != 0 {
		t.Fatalf("want empty URLs, got %v", plan.URLs)
	}
}

func TestBuild_HashesAndStages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "assets", "logo.png"), []byte("PNG-bytes"))
	mustWrite(t, filepath.Join(dir, "assets", "img", "banner.webp"), []byte("WEBP-bytes"))

	plan, err := assets.Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got := len(plan.URLs); got != 2 {
		t.Fatalf("want 2 URLs, got %d (%v)", got, plan.URLs)
	}

	logo, ok := plan.URLs["logo.png"]
	if !ok {
		t.Fatalf("logo.png missing from URLs: %v", plan.URLs)
	}
	if want := "/_app/immutable/assets/logo."; len(logo) <= len(want) || logo[:len(want)] != want {
		t.Fatalf("logo URL = %q, want prefix %q", logo, want)
	}
	if logo[len(logo)-4:] != ".png" {
		t.Fatalf("logo URL = %q, want .png suffix", logo)
	}

	banner, ok := plan.URLs["img/banner.webp"]
	if !ok {
		t.Fatalf("img/banner.webp missing from URLs: %v", plan.URLs)
	}
	if banner[len(banner)-5:] != ".webp" {
		t.Fatalf("banner URL = %q, want .webp suffix", banner)
	}

	for _, url := range plan.URLs {
		staged := filepath.Join(dir, "_app", "immutable", "assets", filepath.Base(url))
		if _, err := os.Stat(staged); err != nil {
			t.Fatalf("staged file missing for %s: %v", url, err)
		}
	}

	// Sources must be sorted for deterministic codegen.
	want := append([]string{}, plan.Sources...)
	sort.Strings(want)
	if !reflect.DeepEqual(plan.Sources, want) {
		t.Fatalf("Sources not sorted: got %v", plan.Sources)
	}
}

func TestBuild_Deterministic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "assets", "logo.png"), []byte("PNG-bytes"))

	first, err := assets.Build(dir)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}
	second, err := assets.Build(dir)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}
	if !reflect.DeepEqual(first.URLs, second.URLs) {
		t.Fatalf("non-deterministic URLs:\n first:  %v\n second: %v", first.URLs, second.URLs)
	}
}

func TestBuild_DistinctContentDistinctHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "assets", "a.txt"), []byte("alpha"))
	mustWrite(t, filepath.Join(dir, "assets", "b.txt"), []byte("bravo"))

	plan, err := assets.Build(dir)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plan.URLs["a.txt"] == plan.URLs["b.txt"] {
		t.Fatalf("distinct content collided: %s", plan.URLs["a.txt"])
	}
}

func TestBuild_RejectsRelativeStaticDir(t *testing.T) {
	t.Parallel()
	if _, err := assets.Build("relative/path"); err == nil {
		t.Fatal("expected error for relative staticDir")
	}
}

func mustWrite(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
