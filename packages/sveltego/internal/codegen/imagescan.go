package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/images"
)

// imageSrcPattern captures the literal src= value on an <Image> opening
// tag. Only the static double- or single-quoted form is recognized;
// dynamic src={...} surfaces as a codegen error at emit time so a typo
// never silently disables the variant pipeline.
var imageSrcPattern = regexp.MustCompile(`<Image\b[^>]*?\bsrc\s*=\s*(?:"([^"]+)"|'([^']+)')`)

// scanProjectImages walks the project for every .svelte file and
// returns the deduplicated set of <Image src=...> values. Paths are
// normalized to forward slashes and stripped of any leading slash so
// they match the keys produced by the asset pipeline.
//
// The walker covers src/routes/, src/lib/, and any other src/ subtree
// because <Image> may appear in a component imported from anywhere.
func scanProjectImages(projectRoot string) ([]string, error) {
	srcRoot := filepath.Join(projectRoot, "src")
	if _, err := os.Stat(srcRoot); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("codegen: stat %s: %w", srcRoot, err)
	}
	seen := make(map[string]struct{})
	walkErr := filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".svelte") {
			return nil
		}
		body, rerr := os.ReadFile(path) //nolint:gosec // path comes from the WalkDir under projectRoot
		if rerr != nil {
			return fmt.Errorf("codegen: read %s: %w", path, rerr)
		}
		matches := imageSrcPattern.FindAllSubmatch(body, -1)
		for _, m := range matches {
			var src string
			if len(m[1]) > 0 {
				src = string(m[1])
			} else if len(m[2]) > 0 {
				src = string(m[2])
			}
			if src == "" {
				continue
			}
			seen[strings.TrimPrefix(src, "/")] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// buildImageVariants runs the image pipeline against every <Image> source
// referenced in the project. An empty source list short-circuits to a
// nil map so projects with no <Image> elements pay no I/O cost.
func buildImageVariants(projectRoot string, widths []int) (map[string]images.Result, error) {
	sources, err := scanProjectImages(projectRoot)
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, nil
	}
	plan, err := images.Build(images.BuildOptions{
		StaticDir: filepath.Join(projectRoot, "static"),
		Sources:   sources,
		Widths:    widths,
	})
	if err != nil {
		return nil, fmt.Errorf("codegen: build images: %w", err)
	}
	return plan.Results, nil
}
