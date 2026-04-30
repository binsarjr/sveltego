package codegen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/assets"
)

func TestGenerateAssetsManifest_Empty(t *testing.T) {
	t.Parallel()
	got, err := GenerateAssetsManifest(assets.Plan{URLs: map[string]string{}}, AssetsEmitOptions{})
	if err != nil {
		t.Fatalf("GenerateAssetsManifest: %v", err)
	}
	assertAssetsGolden(t, "empty", got)
}

func TestGenerateAssetsManifest_Populated(t *testing.T) {
	t.Parallel()
	plan := assets.Plan{
		URLs: map[string]string{
			"logo.png":        "/_app/immutable/assets/logo.abc12345.png",
			"img/banner.webp": "/_app/immutable/assets/banner.deadbeef.webp",
			"docs/intro.md":   "/_app/immutable/assets/intro.cafef00d.md",
		},
		Sources: []string{"docs/intro.md", "img/banner.webp", "logo.png"},
	}
	got, err := GenerateAssetsManifest(plan, AssetsEmitOptions{})
	if err != nil {
		t.Fatalf("GenerateAssetsManifest: %v", err)
	}
	assertAssetsGolden(t, "populated", got)
}

func TestGenerateAssetsManifest_Deterministic(t *testing.T) {
	t.Parallel()
	plan := assets.Plan{
		URLs: map[string]string{
			"logo.png":        "/_app/immutable/assets/logo.abc12345.png",
			"img/banner.webp": "/_app/immutable/assets/banner.deadbeef.webp",
		},
	}
	a, err := GenerateAssetsManifest(plan, AssetsEmitOptions{})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := GenerateAssetsManifest(plan, AssetsEmitOptions{})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("non-deterministic output:\n--- a:\n%s\n--- b:\n%s", a, b)
	}
}

func TestGenerateAssetsManifest_SortedWithoutPlanSources(t *testing.T) {
	t.Parallel()
	plan := assets.Plan{
		URLs: map[string]string{
			"z.txt": "/_app/immutable/assets/z.aaaaaaaa.txt",
			"a.txt": "/_app/immutable/assets/a.bbbbbbbb.txt",
			"m.txt": "/_app/immutable/assets/m.cccccccc.txt",
		},
	}
	got, err := GenerateAssetsManifest(plan, AssetsEmitOptions{})
	if err != nil {
		t.Fatalf("GenerateAssetsManifest: %v", err)
	}
	idxA := bytes.Index(got, []byte("a.txt"))
	idxM := bytes.Index(got, []byte("m.txt"))
	idxZ := bytes.Index(got, []byte("z.txt"))
	if !(idxA < idxM && idxM < idxZ) {
		t.Fatalf("keys not emitted in sorted order: a=%d m=%d z=%d\n%s", idxA, idxM, idxZ, got)
	}
}

func TestEmitAssetsManifest_WritesGenFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "static", "assets"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "static", "assets", "logo.png"), []byte("PNG"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := emitAssetsManifest(root, ".gen", "gen"); err != nil {
		t.Fatalf("emitAssetsManifest: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(root, ".gen", "assets.gen.go"))
	if err != nil {
		t.Fatalf("read assets.gen.go: %v", err)
	}
	if !strings.Contains(string(body), "logo.png") {
		t.Fatalf("assets.gen.go missing logo.png entry:\n%s", body)
	}
	if !strings.Contains(string(body), "kit.RegisterAssets(Assets)") {
		t.Fatalf("assets.gen.go missing RegisterAssets call:\n%s", body)
	}
}

func TestEmitAssetsManifest_NoStaticDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := emitAssetsManifest(root, ".gen", "gen"); err != nil {
		t.Fatalf("emitAssetsManifest with no static/: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, ".gen", "assets.gen.go"))
	if err != nil {
		t.Fatalf("read assets.gen.go: %v", err)
	}
	if !strings.Contains(string(body), "Assets is empty") {
		t.Fatalf("assets.gen.go for empty project should comment empty:\n%s", body)
	}
}

func assertAssetsGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", "assets", name+".golden")
	if os.Getenv("GOLDEN_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with GOLDEN_UPDATE=1): %v", path, err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("golden mismatch in %s; run GOLDEN_UPDATE=1\n--- want:\n%s\n--- got:\n%s", path, want, got)
	}
}
