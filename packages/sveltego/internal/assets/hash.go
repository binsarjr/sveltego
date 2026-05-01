// Package assets implements the build-time pipeline that fingerprints
// files in static/assets/ and stages copies under
// static/_app/immutable/assets/<base>.<hash>.<ext> for cache-busted
// delivery. Codegen consumes the resulting source -> hashed-URL map to
// emit the runtime registration consumed by kit.Asset.
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// HashLength is the number of hex characters of SHA-256 used in the
// filename fingerprint. Eight characters (32 bits) is short enough to
// keep filenames readable and long enough that the chance of an
// accidental collision in a single project is vanishingly small.
const HashLength = 8

// Plan summarizes the result of [Build]. URLs maps each source path
// (forward-slash relative to staticDir/assets, e.g. "logo.png" or
// "img/banner.webp") to the staged URL ("/_app/immutable/assets/
// logo.abc12345.png"). Sources iterates URLs in deterministic
// (lexicographic) order so codegen output is stable across runs.
type Plan struct {
	URLs    map[string]string
	Sources []string
}

// Build walks staticDir/assets/ recursively, computes a SHA-256-based
// fingerprint for every regular file, and stages a copy at
// staticDir/_app/immutable/assets/<base>.<hash>.<ext>. The returned
// [Plan] holds the source -> URL mapping codegen needs to emit the
// runtime manifest.
//
// staticDir must be an absolute path. A missing staticDir/assets is
// not an error and yields an empty plan; the rest of the build
// pipeline tolerates projects with no static assets.
//
// Re-running Build is idempotent: identical inputs produce identical
// hashes and identical staged copies. Existing files in the staging
// dir are overwritten so a removed source eventually disappears from
// the staging dir if the caller wipes the staging dir first; Build
// itself does not delete unrelated files.
func Build(staticDir string) (Plan, error) {
	if staticDir == "" {
		return Plan{}, errors.New("assets: empty staticDir")
	}
	if !filepath.IsAbs(staticDir) {
		return Plan{}, fmt.Errorf("assets: staticDir must be absolute (got %q)", staticDir)
	}

	srcRoot := filepath.Join(staticDir, "assets")
	info, err := os.Stat(srcRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Plan{URLs: map[string]string{}}, nil
		}
		return Plan{}, fmt.Errorf("assets: stat %s: %w", srcRoot, err)
	}
	if !info.IsDir() {
		return Plan{}, fmt.Errorf("assets: %s is not a directory", srcRoot)
	}

	stageRoot := filepath.Join(staticDir, "_app", "immutable", "assets")
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return Plan{}, fmt.Errorf("assets: mkdir %s: %w", stageRoot, err)
	}

	urls := make(map[string]string)
	walkErr := filepath.WalkDir(srcRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, rerr := filepath.Rel(srcRoot, path)
		if rerr != nil {
			return fmt.Errorf("assets: rel %s: %w", path, rerr)
		}
		key := filepath.ToSlash(rel)

		hash, herr := hashFile(path)
		if herr != nil {
			return herr
		}

		base := filepath.Base(rel)
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		stagedName := stem + "." + hash + ext
		stagedPath := filepath.Join(stageRoot, stagedName)

		if cerr := copyFile(path, stagedPath); cerr != nil {
			return cerr
		}

		urls[key] = kit.DefaultAssetsImmutablePrefix + stagedName
		return nil
	})
	if walkErr != nil {
		return Plan{}, walkErr
	}

	sources := make([]string, 0, len(urls))
	for k := range urls {
		sources = append(sources, k)
	}
	sort.Strings(sources)
	return Plan{URLs: urls, Sources: sources}, nil
}

// hashFile returns the first [HashLength] hex characters of the SHA-256
// of the file at path. Reading is streaming so very large assets do not
// pull the whole file into memory.
func hashFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is supplied by the WalkDir under staticDir/assets
	if err != nil {
		return "", fmt.Errorf("assets: open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("assets: hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil))[:HashLength], nil
}

// copyFile writes src to dst byte-for-byte, creating any missing parent
// directories. The destination is created with 0o644 because static
// assets are typically served by a web server that runs as a different
// user; world-readable bits are deliberate.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("assets: mkdir %s: %w", filepath.Dir(dst), err)
	}
	in, err := os.Open(src) //nolint:gosec // src is constrained to the assets root by the caller
	if err != nil {
		return fmt.Errorf("assets: open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // public assets are world-readable on purpose
	if err != nil {
		return fmt.Errorf("assets: create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("assets: copy %s -> %s: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("assets: close %s: %w", dst, err)
	}
	return nil
}
