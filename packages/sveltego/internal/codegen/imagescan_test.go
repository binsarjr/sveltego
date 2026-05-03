package codegen

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// writePNG drops a 1x1 in-memory PNG at path. Used by the imagescan
// integration tests so the variant pipeline has real bytes to decode
// without bloating the repo with binary fixture assets.
func writePNG(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 200, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestScanProjectImages_DedupAndSort(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	must := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	must("src/routes/_page.svelte", `<Image src="/hero.png" alt="x" /><Image src='/about.png' alt="y" />`)
	must("src/routes/duplicate/_page.svelte", `<Image src="/hero.png" alt="dup" />`)
	must("src/lib/Card.svelte", `<Image src="/card.png" alt="card" />`)
	must("src/routes/notimage.svelte.txt", `<Image src="/nope.png" alt="z" />`)

	got, err := scanProjectImages(root)
	if err != nil {
		t.Fatalf("scanProjectImages: %v", err)
	}
	want := []string{"about.png", "card.png", "hero.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestScanProjectImages_EmptyProject(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got, err := scanProjectImages(root)
	if err != nil {
		t.Fatalf("scanProjectImages: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected nil/empty for project without src/, got %v", got)
	}
}

func TestScanProjectImages_DynamicSrcSkipped(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "src", "page.svelte"),
		[]byte(`<Image src={data.hero} alt="dynamic" />`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	got, err := scanProjectImages(root)
	if err != nil {
		t.Fatalf("scanProjectImages: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("dynamic src must be skipped, got %v", got)
	}
}

func TestBuildImageVariants_RoundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "src.svelte"),
		[]byte(`<Image src="/pixel.png" alt="x" />`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "src", "routes", "_page.svelte"),
		[]byte(`<Image src="/pixel.png" alt="x" />`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(root, "static", "assets", "pixel.png"))

	results, err := buildImageVariants(root, []int{2})
	if err != nil {
		t.Fatalf("buildImageVariants: %v", err)
	}
	res, ok := results["pixel.png"]
	if !ok {
		t.Fatalf("expected pixel.png in results, got keys %v", keysOf(results))
	}
	if res.IntrinsicWidth != 4 || res.IntrinsicHeight != 4 {
		t.Errorf("intrinsic dims = %dx%d, want 4x4", res.IntrinsicWidth, res.IntrinsicHeight)
	}
	if len(res.Variants) < 1 {
		t.Errorf("expected at least the intrinsic variant, got %d", len(res.Variants))
	}
}

func TestBuildImageVariants_NoSourcesShortCircuits(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	results, err := buildImageVariants(root, nil)
	if err != nil {
		t.Fatalf("buildImageVariants: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results for project with no Image refs, got %v", results)
	}
}

func TestProjectImageWidths_FirstNonEmptyWins(t *testing.T) {
	t.Parallel()
	opts := map[string]kit.PageOptions{
		"/blog":  {ImageWidths: []int{640, 1280}},
		"/about": {},
	}
	got := projectImageWidths(opts)
	if !reflect.DeepEqual(got, []int{640, 1280}) {
		t.Fatalf("got %v, want [640 1280]", got)
	}
}

func TestProjectImageWidths_AllEmptyReturnsNil(t *testing.T) {
	t.Parallel()
	got := projectImageWidths(map[string]kit.PageOptions{
		"/": {},
	})
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
