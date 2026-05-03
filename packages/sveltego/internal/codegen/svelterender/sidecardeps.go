package svelterender

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// errSidecarDepsInstall is returned when EnsureSidecarDeps cannot
// produce a usable sidecar tree — npm is missing, the install crashed,
// or the cache directory is unwritable. Wrapped via %w so callers can
// surface a uniform "install Node + npm; check $XDG_CACHE_HOME" hint.
var errSidecarDepsInstall = errors.New("svelterender: sidecar deps install failed")

// errSidecarUnwritableCache is returned when neither the source sidecar
// dir nor the user cache dir can host a node_modules tree.
var errSidecarUnwritableCache = errors.New("svelterender: no writable location for sidecar node_modules")

// EnsureSidecarDeps returns a directory containing the sidecar tree
// (index.mjs and friends) plus a populated node_modules. When the
// source sidecar dir is writable AND already has node_modules, it is
// returned as-is — the existing dev workflow (repo checkout + manual
// `npm install`) is preserved.
//
// Otherwise the function materializes a copy of the sidecar tree under
// $XDG_CACHE_HOME/sveltego/sidecar/<hash>/ keyed by the SHA-256 of
// (package.json + package-lock.json), runs `npm ci` (or `npm install`
// when no lockfile exists) in the cache copy, and returns that path.
// Subsequent calls with an unchanged lockfile reuse the populated
// cache.
//
// This is the fix for issue #525: when sveltego is installed via
// `go install`, the sidecar lives under the read-only Go module cache
// and cannot host a node_modules tree itself.
func EnsureSidecarDeps(srcDir string) (string, error) {
	if srcDir == "" {
		return "", errors.New("svelterender: EnsureSidecarDeps: srcDir is empty")
	}
	if _, err := os.Stat(filepath.Join(srcDir, "index.mjs")); err != nil {
		return "", fmt.Errorf("svelterender: sidecar entry missing at %s: %w", srcDir, err)
	}

	// Fast path: source dir already has node_modules (dev workflow on a
	// writable repo checkout). Stat is sufficient — we don't validate
	// the contents because the original behaviour also trusted a bare
	// stat of node_modules/acorn.
	if _, err := os.Stat(filepath.Join(srcDir, "node_modules", "acorn")); err == nil {
		return srcDir, nil
	}

	cacheDir, err := sidecarCacheDir(srcDir)
	if err != nil {
		return "", err
	}
	// If a previous invocation already materialized this version, reuse
	// it. The hash is over package.json + package-lock.json so any pin
	// bump invalidates the cache automatically.
	if _, err := os.Stat(filepath.Join(cacheDir, "node_modules", "acorn")); err == nil {
		return cacheDir, nil
	}

	if _, err := exec.LookPath("npm"); err != nil {
		return "", fmt.Errorf("%w: npm binary not on $PATH; install Node 20.6+ (which ships npm) so sveltego can bootstrap the sidecar at %s",
			errSidecarDepsInstall, cacheDir)
	}

	if err := materializeSidecar(srcDir, cacheDir); err != nil {
		return "", fmt.Errorf("%w: copy sidecar to %s: %v", errSidecarDepsInstall, cacheDir, err)
	}
	if err := runNPMInstall(cacheDir); err != nil {
		return "", fmt.Errorf("%w: %v", errSidecarDepsInstall, err)
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "node_modules", "acorn")); err != nil {
		return "", fmt.Errorf("%w: install completed but node_modules/acorn missing at %s: %v",
			errSidecarDepsInstall, cacheDir, err)
	}
	return cacheDir, nil
}

// sidecarCacheDir returns the directory the auto-installed sidecar tree
// should live in. Path is $XDG_CACHE_HOME/sveltego/sidecar/<hash>/ where
// hash covers package.json + package-lock.json so a dependency pin bump
// invalidates the cache automatically.
func sidecarCacheDir(srcDir string) (string, error) {
	hash, err := lockHash(srcDir)
	if err != nil {
		return "", err
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("%w: os.UserCacheDir: %v", errSidecarUnwritableCache, err)
	}
	dir := filepath.Join(base, "sveltego", "sidecar", hash)
	return dir, nil
}

// lockHash returns the SHA-256 hex digest of (package.json bytes +
// package-lock.json bytes when present). Truncated to 16 hex chars to
// keep the cache path short on filesystems with sun_path limits.
func lockHash(srcDir string) (string, error) {
	h := sha256.New()
	pkg, err := os.ReadFile(filepath.Join(srcDir, "package.json"))
	if err != nil {
		return "", fmt.Errorf("svelterender: read package.json: %w", err)
	}
	h.Write(pkg)
	if lock, err := os.ReadFile(filepath.Join(srcDir, "package-lock.json")); err == nil {
		h.Write(lock)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// materializeSidecar copies the sidecar source tree (everything except
// node_modules and any stray .gen/ files) from srcDir to dstDir. The
// destination is created with 0o755; files keep their source mode bits.
// An existing dstDir is left in place; conflicting files are overwritten
// so a half-finished previous run doesn't poison the cache.
func materializeSidecar(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Skip node_modules and dotfile-prefixed entries. A developer
		// box may have node_modules from an in-place `npm install`, and
		// the test suite drops `.sveltego-fallback-*` scratch dirs in
		// the same tree; neither should pollute the cache copy. The
		// cache's own npm install populates a fresh node_modules.
		first, _, _ := strings.Cut(rel, string(filepath.Separator))
		if first == "node_modules" || strings.HasPrefix(first, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		return copyFile(path, dst)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// runNPMInstall runs `npm ci` in dir when a package-lock.json is
// present; otherwise falls back to `npm install`. Both invocations use
// `--omit=dev --no-audit --no-fund --no-progress` to keep the install
// quiet and small. stderr is captured and surfaced when the command
// fails so users see the real npm error, not a generic exit-code message.
func runNPMInstall(dir string) error {
	args := []string{"install", "--omit=dev", "--no-audit", "--no-fund", "--no-progress"}
	if _, err := os.Stat(filepath.Join(dir, "package-lock.json")); err == nil {
		args = []string{"ci", "--omit=dev", "--no-audit", "--no-fund", "--no-progress"}
	}
	//nolint:gosec // npm path resolved by exec.LookPath; args are static
	cmd := exec.Command("npm", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"NODE_NO_WARNINGS=1",
		"npm_config_fund=false",
		"npm_config_audit=false",
		"npm_config_update_notifier=false",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("`npm %s` in %s failed: %v; output: %s",
			strings.Join(args, " "), dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}
