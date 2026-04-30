// Package images implements the build-time pipeline that generates
// responsive size variants for source images referenced by <Image>
// elements. The output filenames carry both the source content hash and
// the target width so a downstream cache-busting URL is stable across
// builds and unique per (source, width) pair.
//
// The encoder set is JPEG and PNG only; WebP and AVIF are intentionally
// out of scope for v1 to avoid pulling in cgo or third-party encoders.
// The resampler is stdlib-only (a small bilinear filter implemented in
// scale.go) so the package keeps Go 1.23 vanilla.
package images

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/binsarjr/sveltego/exports/kit"
)

// HashLength is the number of hex characters of SHA-256 used in the
// staged filename. Eight characters keeps URLs short while making
// accidental collisions vanishingly rare in a single project.
const HashLength = 8

// jpegQuality is the libjpeg-equivalent quality factor used for every
// JPEG variant. 82 is the same default disintegration/imaging chose; it
// trades ~10% file size for visually-identical output versus 90.
const jpegQuality = 82

// DefaultWidths is the variant width set used when [BuildOptions.Widths]
// is empty. The values match the SvelteKit enhanced-img defaults so
// downstream guidance carries over.
var DefaultWidths = []int{320, 640, 1280}

// Variant describes one emitted size of a source image. Width and Height
// are the post-resize pixel dimensions; URL is the hashed public path
// (e.g. "/_app/immutable/assets/hero.abc12345.640.jpg") served by the
// runtime static handler.
type Variant struct {
	Width  int
	Height int
	URL    string
	Path   string
}

// Result describes the variant set generated for one source image.
// Intrinsic{Width,Height} are the source dimensions; Variants is sorted
// ascending by Width.
type Result struct {
	Source          string
	IntrinsicWidth  int
	IntrinsicHeight int
	Variants        []Variant
}

// BuildOptions configures [Build].
type BuildOptions struct {
	// StaticDir is the absolute path to the project's static/ directory.
	// Source images live under StaticDir/assets/; outputs are staged under
	// StaticDir/_app/immutable/assets/.
	StaticDir string
	// Sources lists forward-slash relative paths under StaticDir/assets/
	// that <Image> references. Non-image files and missing files surface
	// as errors so a typo'd src never silently disappears from the page.
	Sources []string
	// Widths overrides [DefaultWidths]. Negative or duplicate entries are
	// dropped; a width >= the intrinsic width is skipped (no upscaling).
	Widths []int
	// Concurrency caps the number of goroutines decoding/encoding in
	// parallel. Zero defaults to runtime.NumCPU().
	Concurrency int
}

// Plan summarizes a [Build] invocation. Results is keyed by source path
// (the forward-slash relative path supplied in [BuildOptions.Sources]).
type Plan struct {
	Results map[string]Result
}

// Build resizes every entry in opts.Sources to the configured widths and
// stages the encoded variants under StaticDir/_app/immutable/assets/.
// Sources are processed concurrently (capped at opts.Concurrency or
// runtime.NumCPU()); the function returns once every variant has been
// written or the first error surfaces.
//
// Re-running Build is idempotent: identical inputs produce identical
// hashes and bytes, so the staging directory converges across runs.
func Build(opts BuildOptions) (Plan, error) {
	if opts.StaticDir == "" {
		return Plan{}, errors.New("images: empty StaticDir")
	}
	if !filepath.IsAbs(opts.StaticDir) {
		return Plan{}, fmt.Errorf("images: StaticDir must be absolute (got %q)", opts.StaticDir)
	}

	widths := normalizeWidths(opts.Widths)
	if len(widths) == 0 {
		widths = DefaultWidths
	}

	srcRoot := filepath.Join(opts.StaticDir, "assets")
	stageRoot := filepath.Join(opts.StaticDir, "_app", "immutable", "assets")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return Plan{}, fmt.Errorf("images: mkdir %s: %w", stageRoot, err)
	}

	conc := opts.Concurrency
	if conc <= 0 {
		conc = runtime.NumCPU()
	}
	if conc > len(opts.Sources) && len(opts.Sources) > 0 {
		conc = len(opts.Sources)
	}

	results := make(map[string]Result, len(opts.Sources))
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)
	sem := make(chan struct{}, conc)
	for _, src := range opts.Sources {
		src := src
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res, err := processSource(srcRoot, stageRoot, src, widths)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			mu.Lock()
			results[src] = res
			mu.Unlock()
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return Plan{}, firstErr
	}

	return Plan{Results: results}, nil
}

// processSource decodes one source image, generates every applicable
// width variant, and writes them to stageRoot. The original file's
// content hash seeds every variant's filename so cache-busting is stable
// across runs.
func processSource(srcRoot, stageRoot, src string, widths []int) (Result, error) {
	if strings.Contains(src, "..") {
		return Result{}, fmt.Errorf("images: source %q contains path traversal", src)
	}
	abs := filepath.Join(srcRoot, filepath.FromSlash(src))
	f, err := os.Open(abs) //nolint:gosec // path is validated under srcRoot
	if err != nil {
		return Result{}, fmt.Errorf("images: open %s: %w", abs, err)
	}
	defer f.Close()

	hash, err := hashReader(f)
	if err != nil {
		return Result{}, fmt.Errorf("images: hash %s: %w", abs, err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Result{}, fmt.Errorf("images: seek %s: %w", abs, err)
	}

	img, format, err := image.Decode(f)
	if err != nil {
		return Result{}, fmt.Errorf("images: decode %s: %w", abs, err)
	}
	if format != "jpeg" && format != "png" {
		return Result{}, fmt.Errorf("images: %s: unsupported format %q (only jpeg and png in v1)", abs, format)
	}

	intrinsic := img.Bounds().Size()
	stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))

	variants := make([]Variant, 0, len(widths)+1)
	// Always emit the intrinsic-size variant so the <img src=...> can fall
	// back to a stable URL even when every configured width upscales.
	intrinsicV, err := emitVariant(stageRoot, stem, hash, format, img, intrinsic.X, intrinsic.X, intrinsic.Y)
	if err != nil {
		return Result{}, err
	}
	variants = append(variants, intrinsicV)

	for _, w := range widths {
		if w >= intrinsic.X {
			continue
		}
		h := scaledHeight(intrinsic.X, intrinsic.Y, w)
		resized := scaleBilinear(img, w, h)
		v, err := emitVariant(stageRoot, stem, hash, format, resized, w, w, h)
		if err != nil {
			return Result{}, err
		}
		variants = append(variants, v)
	}

	sort.Slice(variants, func(i, j int) bool { return variants[i].Width < variants[j].Width })

	return Result{
		Source:          src,
		IntrinsicWidth:  intrinsic.X,
		IntrinsicHeight: intrinsic.Y,
		Variants:        variants,
	}, nil
}

// emitVariant encodes one resized (or intrinsic-size) image to the
// staging directory and returns the variant descriptor.
func emitVariant(stageRoot, stem, hash, format string, img image.Image, width, w, h int) (Variant, error) {
	ext := "." + format
	if format == "jpeg" {
		ext = ".jpg"
	}
	name := stem + "." + hash + "." + strconv.Itoa(width) + ext
	abs := filepath.Join(stageRoot, name)
	out, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // public assets are world-readable on purpose
	if err != nil {
		return Variant{}, fmt.Errorf("images: create %s: %w", abs, err)
	}
	if encErr := encodeImage(out, img, format); encErr != nil {
		_ = out.Close()
		return Variant{}, encErr
	}
	if err := out.Close(); err != nil {
		return Variant{}, fmt.Errorf("images: close %s: %w", abs, err)
	}
	return Variant{
		Width:  w,
		Height: h,
		URL:    kit.DefaultAssetsImmutablePrefix + name,
		Path:   abs,
	}, nil
}

// encodeImage dispatches to the right encoder for format. format must be
// either "jpeg" or "png"; the caller checks this upstream.
func encodeImage(w io.Writer, img image.Image, format string) error {
	switch format {
	case "jpeg":
		if err := jpeg.Encode(w, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return fmt.Errorf("images: encode jpeg: %w", err)
		}
	case "png":
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		if err := enc.Encode(w, img); err != nil {
			return fmt.Errorf("images: encode png: %w", err)
		}
	default:
		return fmt.Errorf("images: unsupported format %q", format)
	}
	return nil
}

// scaledHeight returns the height that preserves the source aspect ratio
// when resizing to targetWidth. Rounds to nearest int.
func scaledHeight(srcW, srcH, targetW int) int {
	if srcW == 0 {
		return targetW
	}
	return int(float64(targetW)*float64(srcH)/float64(srcW) + 0.5)
}

// hashReader returns the first [HashLength] hex characters of the
// SHA-256 of r. Reading is streaming so very large source files do not
// pull the whole file into memory.
func hashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:HashLength], nil
}

// normalizeWidths sorts, deduplicates, and drops non-positive entries.
func normalizeWidths(widths []int) []int {
	if len(widths) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(widths))
	out := make([]int, 0, len(widths))
	for _, w := range widths {
		if w <= 0 {
			continue
		}
		if _, ok := seen[w]; ok {
			continue
		}
		seen[w] = struct{}{}
		out = append(out, w)
	}
	sort.Ints(out)
	return out
}
