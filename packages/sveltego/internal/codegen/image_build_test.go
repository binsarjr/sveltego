package codegen

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuild_ImageVariantsEndToEnd exercises the build pipeline with one
// page that references a real JPEG under static/assets/. The pipeline
// must produce sized variants on disk and emit the matching <img>
// markup with srcset into the generated page.
func TestBuild_ImageVariantsEndToEnd(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.23\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"),
		`<Image src="hero.jpg" alt="Hero" width="800" height="600" />`+"\n")

	writeJPEGFixture(t, filepath.Join(root, "static", "assets", "hero.jpg"), 1600, 1200)

	res, err := Build(BuildOptions{
		ProjectRoot: root,
		NoClient:    true,
		ImageWidths: []int{320, 640, 1280},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Routes != 1 {
		t.Errorf("Routes = %d, want 1", res.Routes)
	}

	stageDir := filepath.Join(root, "static", "_app", "immutable", "assets")
	entries, err := os.ReadDir(stageDir)
	if err != nil {
		t.Fatalf("read stage dir: %v", err)
	}
	// 4 variants: 320, 640, 1280, plus intrinsic 1600. Existing assets
	// pipeline may also stage the original hero.jpg via static/assets
	// hashing — count only image-pipeline outputs.
	wantVariants := []string{".320.jpg", ".640.jpg", ".1280.jpg", ".1600.jpg"}
	for _, want := range wantVariants {
		found := false
		for _, e := range entries {
			if strings.Contains(e.Name(), want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("variant ending in %q not found under %s", want, stageDir)
		}
	}

	pageBytes, err := os.ReadFile(filepath.Join(root, ".gen", "routes", "page.gen.go"))
	if err != nil {
		t.Fatalf("read page.gen.go: %v", err)
	}
	body := string(pageBytes)
	for _, want := range []string{
		`width="800"`,
		`height="600"`,
		`alt="Hero"`,
		`loading="lazy"`,
		`decoding="async"`,
		`srcset=`,
		`320w`,
		`640w`,
		`1280w`,
		`1600w`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page.gen.go missing %q:\n%s", want, body)
		}
	}

	// Verify the actual variant file is a valid JPEG of the expected width.
	var anyVariant string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".640.jpg") {
			anyVariant = filepath.Join(stageDir, e.Name())
			break
		}
	}
	if anyVariant == "" {
		t.Fatal("no .640.jpg variant found")
	}
	f, err := os.Open(anyVariant)
	if err != nil {
		t.Fatalf("open variant: %v", err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatalf("decode variant: %v", err)
	}
	if got := img.Bounds().Dx(); got != 640 {
		t.Errorf("variant width = %d, want 640", got)
	}
}

// TestBuild_NoImagesShortCircuits confirms a project without <Image>
// elements does not invoke the image pipeline (no errors when
// static/assets is missing entirely).
func TestBuild_NoImagesShortCircuits(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.23\n")
	writeFile(t, filepath.Join(root, "src", "routes", "_page.svelte"),
		"<h1>plain</h1>\n")
	if _, err := Build(BuildOptions{ProjectRoot: root, NoClient: true}); err != nil {
		t.Fatalf("Build: %v", err)
	}
}

func writeJPEGFixture(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 96, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
