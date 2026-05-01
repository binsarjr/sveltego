package devserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen"
)

// Options configures Run.
type Options struct {
	// ProjectRoot is the absolute path to the project (must contain go.mod).
	ProjectRoot string
	// MainPkg is the Go package the user's HTTP server lives in,
	// passed verbatim to `go build`. Defaults to "./cmd/app".
	MainPkg string
	// Port is the address the dev proxy listens on. Defaults to 5173,
	// the SvelteKit-canonical dev port.
	Port int
	// GoPort is the address the user's Go server listens on. Defaults
	// to 5174. Must differ from Port.
	GoPort int
	// VitePort is the address Vite listens on. Defaults to 5175.
	VitePort int
	// NoClient skips Vite altogether and routes every request to Go.
	// The build-side --no-client flag has the same shape.
	NoClient bool
	// Logger overrides the default slog logger. Optional.
	Logger *slog.Logger
	// Stdout / Stderr receive the child processes' output. Defaults to
	// os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// defaults populates the zero-valued fields with sensible values.
func (o *Options) defaults() {
	if o.MainPkg == "" {
		o.MainPkg = "./cmd/app"
	}
	if o.Port == 0 {
		o.Port = 5173
	}
	if o.GoPort == 0 {
		o.GoPort = 5174
	}
	if o.VitePort == 0 {
		o.VitePort = 5175
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// Run starts the dev server: it codegens once, brings up the Go and
// Vite child processes, opens the dev proxy on Options.Port, and then
// loops forever forwarding watcher events back into codegen / restart
// calls. It blocks until ctx is cancelled.
func Run(ctx context.Context, opts Options) error {
	opts.defaults()
	if !filepath.IsAbs(opts.ProjectRoot) {
		return fmt.Errorf("devserver: ProjectRoot must be absolute (got %q)", opts.ProjectRoot)
	}
	if opts.Port == opts.GoPort || opts.Port == opts.VitePort || opts.GoPort == opts.VitePort {
		return errors.New("devserver: Port, GoPort, and VitePort must all differ")
	}

	logger := opts.Logger.With(logKeyComponent, "devserver")
	logger.Info("devserver: starting",
		logKeyRoot, opts.ProjectRoot,
		logKeyPort, opts.Port,
	)

	// Initial codegen so the Go server can build against `.gen/`.
	result, err := runCodegen(ctx, opts, logger)
	if err != nil {
		return err
	}

	viteConfig := result.ViteConfigPath
	if opts.NoClient {
		viteConfig = ""
	}

	gosup, err := newGoSupervisor(opts.ProjectRoot, opts.MainPkg, opts.GoPort, logger, opts.Stdout, opts.Stderr)
	if err != nil {
		return err
	}
	defer gosup.Stop()

	vitesup := newViteSupervisor(opts.ProjectRoot, viteConfig, opts.VitePort, logger, opts.Stdout, opts.Stderr)
	defer vitesup.Stop()

	watcher, err := NewWatcher(opts.ProjectRoot, logger)
	if err != nil {
		return err
	}

	// Start Vite first; the Go server may want to know whether the asset
	// pipeline is up via the env var SVELTEGO_DEV_VITE.
	if err := vitesup.Start(ctx); err != nil {
		logger.Warn("devserver: vite did not start; continuing without client HMR", logKeyError, err)
	}

	if err := gosup.Start(ctx); err != nil {
		return err
	}

	listenAddr := fmt.Sprintf("127.0.0.1:%d", opts.Port)
	prx, err := newProxy(listenAddr, opts.GoPort, vitesup.Port(), logger)
	if err != nil {
		return err
	}
	if err := prx.Start(ctx); err != nil {
		return err
	}
	defer prx.Stop() //nolint:contextcheck // shutdown uses Background by design; ctx is already cancelled

	logger.Info("devserver: ready", logKeyURL, "http://"+listenAddr)

	// Run the watcher loop in a goroutine so we can also listen for ctx.
	watchErr := make(chan error, 1)
	go func() { watchErr <- watcher.Run(ctx) }()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-watchErr:
			if err != nil {
				return fmt.Errorf("devserver: watcher: %w", err)
			}
			return nil
		case ev := <-watcher.Events():
			handleChange(ctx, ev, opts, logger, gosup, prx)
		}
	}
}

// handleChange runs codegen and conditionally restarts the Go server in
// response to one debounced batch.
func handleChange(ctx context.Context, ev Change, opts Options, logger *slog.Logger, gosup *goSupervisor, prx *proxy) {
	start := time.Now()
	rel := relativizePaths(opts.ProjectRoot, ev.Paths)
	logger.Info("devserver: change",
		logKeyKind, changeKindLabel(ev.Kind),
		logKeyFiles, rel,
	)

	if _, err := runCodegen(ctx, opts, logger); err != nil {
		logger.Error("devserver: codegen failed",
			logKeyError, err,
			logKeyElapsed, time.Since(start),
		)
		return
	}

	// .svelte → Go source via codegen, so SSR output only updates after
	// the Go binary is rebuilt. Vite handles client HMR independently;
	// the server-side response rebuild covers both kinds of changes.
	prx.SetRebuilding(true)
	defer prx.SetRebuilding(false)
	if err := gosup.Reload(ctx); err != nil {
		logger.Error("devserver: rebuild failed",
			logKeyError, err,
			logKeyElapsed, time.Since(start),
		)
		return
	}
	logger.Info("devserver: ready", logKeyElapsed, time.Since(start))
}

func changeKindLabel(k ChangeKind) string {
	switch k {
	case ChangeSvelte:
		return "svelte"
	case ChangeGo:
		return "go"
	default:
		return "unknown"
	}
}

// runCodegen invokes codegen.Build with the dev defaults.
func runCodegen(ctx context.Context, opts Options, logger *slog.Logger) (*codegen.BuildResult, error) {
	return codegen.Build(ctx, codegen.BuildOptions{
		ProjectRoot: opts.ProjectRoot,
		Verbose:     false,
		Release:     false,
		Logger:      logger,
		Provenance:  true,
		NoClient:    opts.NoClient,
	})
}

// relativizePaths converts a list of absolute paths into project-root-
// relative slash-separated paths for friendlier log output.
func relativizePaths(root string, paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			out = append(out, p)
			continue
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

// newListener binds a TCP listener on addr. Extracted so tests can swap
// in a different binding strategy if needed.
func newListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}
