package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/fallback"
)

// resolveSidecarDir tries to locate the vendored Node sidecar tree
// when the caller did not pin one explicitly. It walks the Go runtime
// caller chain to find the source path of this very file, which lives
// under packages/sveltego/server, and joins
// "../internal/codegen/svelterender/sidecar" relative to that. The
// path is verified by stat-ing index.mjs; an error means the user has
// to set SSRFallbackConfig.SidecarDir explicitly (typical for deploys
// where the source tree isn't shipped alongside the binary).
func resolveSidecarDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("server: runtime.Caller failed")
	}
	candidate := filepath.Join(filepath.Dir(thisFile), "..", "internal", "codegen", "svelterender", "sidecar")
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("server: abs sidecar dir: %w", err)
	}
	entry := filepath.Join(abs, "index.mjs")
	if _, err := os.Stat(entry); err != nil {
		return "", fmt.Errorf("server: vendored sidecar not at %s: %w", abs, err)
	}
	return abs, nil
}

// SSRFallbackConfig configures the long-running Node sidecar that
// renders routes annotated with `<!-- sveltego:ssr-fallback -->`
// (ADR 0009 sub-decision 2 / Phase 8 #430).
//
// Boot is conditional: codegen registers fallback routes via init() in
// the generated manifest package; if no route opted in, the registry
// is empty and StartFallbackSidecar is a no-op. When at least one
// route registered, the server boot supervises the sidecar process.
//
// Zero-value defaults are: max 1000 cache entries, 60 s TTL, 3
// supervised restarts. Callers may override via SetTTL / SetMaxEntries
// or (for tests) by injecting a custom http.Client into the fallback
// Client they configure manually.
type SSRFallbackConfig struct {
	// SidecarDir is the absolute path to the vendored sidecar tree
	// (the directory holding index.mjs). Required when at least one
	// route is annotated. Typically the gen-emitted constant points
	// at the same vendored tree the build-time SSG / SSR modes use.
	SidecarDir string
	// ProjectRoot is the absolute project root the sidecar uses as
	// CWD when resolving the project-relative `_page.svelte` paths
	// codegen captured at build time. Required.
	ProjectRoot string
	// NodePath optionally overrides exec.LookPath("node"). Useful for
	// tests pinning a specific Node binary. Empty means auto-detect.
	NodePath string
	// CacheTTL is the per-entry TTL for the request cache. Zero falls
	// back to 60 seconds.
	CacheTTL time.Duration
	// CacheSize is the maximum number of cached `(route, hash(data))`
	// entries. Zero falls back to 1000.
	CacheSize int
	// MaxRestarts caps how many times the supervisor relaunches the
	// sidecar after a crash. Zero falls back to 3.
	MaxRestarts int
}

// StartFallbackSidecar boots the long-running Node sidecar when at
// least one annotated route is registered with the runtime fallback
// registry. The returned *fallback.Supervisor is non-nil only when the
// sidecar started; nil means no annotated routes, no work to do.
//
// Stop the supervisor on graceful shutdown via Server.RegisterFallback
// (preferred) or by calling Stop() directly.
//
// SidecarDir defaults to the vendored sidecar tree resolved via
// runtime.Caller. ProjectRoot defaults to the current working
// directory. Operators with non-source deploys override both via the
// passed cfg.
func StartFallbackSidecar(ctx context.Context, cfg SSRFallbackConfig) (*fallback.Supervisor, error) {
	registry := fallback.Default()
	if !registry.HasRoutes() {
		return nil, nil
	}
	if cfg.SidecarDir == "" {
		dir, err := resolveSidecarDir()
		if err != nil {
			return nil, fmt.Errorf("server: SSRFallbackConfig.SidecarDir empty and auto-detect failed; set it to the directory holding the vendored sidecar's index.mjs: %w", err)
		}
		cfg.SidecarDir = dir
	}
	if cfg.ProjectRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("server: SSRFallbackConfig.ProjectRoot empty and os.Getwd failed: %w", err)
		}
		cfg.ProjectRoot = wd
	}
	nodePath := cfg.NodePath
	if nodePath == "" {
		p, err := exec.LookPath("node")
		if err != nil {
			return nil, fmt.Errorf("server: ssr fallback requires node 18+ on $PATH: %w", err)
		}
		nodePath = p
	}
	supervisor := fallback.NewSupervisor(fallback.SidecarOptions{
		NodePath:    nodePath,
		SidecarDir:  cfg.SidecarDir,
		ProjectRoot: cfg.ProjectRoot,
		Restart:     true,
		MaxRestarts: cfg.MaxRestarts,
	})
	first, err := supervisor.Run(ctx)
	if err != nil {
		return nil, err
	}
	client := fallback.NewClient(fallback.ClientOptions{
		Endpoint:  first.Endpoint(),
		CacheSize: cfg.CacheSize,
		TTL:       cfg.CacheTTL,
	})
	registry.Configure(client)
	return supervisor, nil
}
