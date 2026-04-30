package images

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuild_GeneratesVariantsBelowIntrinsic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "assets", "hero.jpg"), 1024, 768)

	plan, err := Build(BuildOptions{
		StaticDir: root,
		Sources:   []string{"hero.jpg"},
		Widths:    []int{320, 640, 1280},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	res, ok := plan.Results["hero.jpg"]
	if !ok {
		t.Fatalf("Results missing hero.jpg: %#v", plan.Results)
	}
	if res.IntrinsicWidth != 1024 || res.IntrinsicHeight != 768 {
		t.Fatalf("intrinsic = %dx%d, want 1024x768", res.IntrinsicWidth, res.IntrinsicHeight)
	}
	// 1280 is >= 1024, must be skipped. 320, 640 generated, plus intrinsic 1024.
	wantWidths := []int{320, 640, 1024}
	if len(res.Variants) != len(wantWidths) {
		t.Fatalf("variants = %d, want %d (%+v)", len(res.Variants), len(wantWidths), res.Variants)
	}
	for i, w := range wantWidths {
		if res.Variants[i].Width != w {
			t.Errorf("variants[%d].Width = %d, want %d", i, res.Variants[i].Width, w)
		}
		if _, err := os.Stat(res.Variants[i].Path); err != nil {
			t.Errorf("variants[%d].Path missing: %v", i, err)
		}
	}
	// Aspect ratio preserved: 320 wide -> 240 tall.
	if got := res.Variants[0].Height; got != 240 {
		t.Errorf("variants[0].Height = %d, want 240", got)
	}
}

func TestBuild_PNGSource(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "assets", "logo.png"), 800, 400)

	plan, err := Build(BuildOptions{
		StaticDir: root,
		Sources:   []string{"logo.png"},
		Widths:    []int{320},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res := plan.Results["logo.png"]
	if len(res.Variants) != 2 {
		t.Fatalf("variants = %d, want 2 (320 + intrinsic 800)", len(res.Variants))
	}
	for _, v := range res.Variants {
		if !strings.HasSuffix(v.Path, ".png") {
			t.Errorf("variant path %q should end with .png", v.Path)
		}
	}
}

func TestBuild_DefaultWidths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "assets", "wide.jpg"), 2000, 1000)

	plan, err := Build(BuildOptions{
		StaticDir: root,
		Sources:   []string{"wide.jpg"},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	res := plan.Results["wide.jpg"]
	// DefaultWidths = 320, 640, 1280 — all below 2000. Plus intrinsic.
	wantWidths := []int{320, 640, 1280, 2000}
	if len(res.Variants) != len(wantWidths) {
		t.Fatalf("variants = %d, want %d", len(res.Variants), len(wantWidths))
	}
	for i, w := range wantWidths {
		if res.Variants[i].Width != w {
			t.Errorf("variants[%d].Width = %d, want %d", i, res.Variants[i].Width, w)
		}
	}
}

func TestBuild_MissingFileFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Build(BuildOptions{
		StaticDir: root,
		Sources:   []string{"missing.jpg"},
	})
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
}

func TestBuild_IdempotentURLs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeJPEG(t, filepath.Join(root, "assets", "pic.jpg"), 800, 600)

	a, err := Build(BuildOptions{StaticDir: root, Sources: []string{"pic.jpg"}, Widths: []int{320}})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := Build(BuildOptions{StaticDir: root, Sources: []string{"pic.jpg"}, Widths: []int{320}})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a.Results["pic.jpg"].Variants[0].URL != b.Results["pic.jpg"].Variants[0].URL {
		t.Errorf("URLs not stable: %q vs %q", a.Results["pic.jpg"].Variants[0].URL, b.Results["pic.jpg"].Variants[0].URL)
	}
}

func TestBuild_RejectsPathTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Build(BuildOptions{
		StaticDir: root,
		Sources:   []string{"../../etc/passwd.jpg"},
	})
	if err == nil {
		t.Fatal("expected error on path traversal, got nil")
	}
}

func TestNormalizeWidths(t *testing.T) {
	t.Parallel()
	got := normalizeWidths([]int{640, 320, 640, -10, 0, 1280})
	want := []int{320, 640, 1280}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func writeJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: uint8(x % 256), B: uint8(y % 256), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
