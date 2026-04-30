package devserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// viteStartTimeout is how long we wait for the Vite dev server to come
// up before giving up. Cold installs can take a while; the proxy still
// starts even if Vite isn't ready, but client HMR will be unavailable.
const viteStartTimeout = 30 * time.Second

// viteSupervisor runs `vite` as a child process pointed at the project's
// generated config. The browser-facing dev port speaks to Vite for
// `/static/_app/*` (asset + HMR) and to the user's Go server for
// everything else.
type viteSupervisor struct {
	root       string
	configPath string
	port       int
	logger     *slog.Logger
	stdout     io.Writer
	stderr     io.Writer
	mu         sync.Mutex
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	done       chan struct{}
	disabled   bool
}

// newViteSupervisor returns a supervisor that runs Vite in dev mode.
// configPath is the absolute path to the generated `vite.config.gen.js`
// or empty when the project skipped client emission. When configPath is
// empty the supervisor is a no-op: Run / Stop succeed and Port returns 0.
func newViteSupervisor(root, configPath string, port int, logger *slog.Logger, stdout, stderr io.Writer) *viteSupervisor {
	v := &viteSupervisor{
		root:       root,
		configPath: configPath,
		port:       port,
		logger:     logger,
		stdout:     stdout,
		stderr:     stderr,
	}
	if configPath == "" {
		v.disabled = true
	}
	return v
}

// Port returns the port Vite was asked to listen on, or 0 when disabled.
func (v *viteSupervisor) Port() int {
	if v.disabled {
		return 0
	}
	return v.port
}

// Disabled reports whether the supervisor is a no-op (no Vite config).
func (v *viteSupervisor) Disabled() bool { return v.disabled }

// Start launches Vite. The returned error is non-nil only when the
// command itself fails to start; a Vite that exits later is reported via
// the logger.
func (v *viteSupervisor) Start(parent context.Context) error {
	if v.disabled {
		return nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	pm := detectPackageManager(v.root)
	if err := ensureNodeModules(parent, v.root, pm, v.stdout, v.stderr); err != nil {
		return err
	}

	args := viteArgs(pm, v.configPath, v.port)
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // pm + flags composed from validated config
	cmd.Dir = v.root
	cmd.Stdout = v.stdout
	cmd.Stderr = v.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("devserver: start vite: %w", err)
	}
	done := make(chan struct{})
	v.cmd = cmd
	v.cancel = cancel
	v.done = done

	go func() {
		defer close(done)
		err := cmd.Wait()
		if err != nil && parent.Err() == nil && ctx.Err() == nil {
			v.logger.Warn("devserver: vite exited", logKeyError, err)
		}
	}()

	if err := waitDial(parent, v.port, viteStartTimeout); err != nil {
		v.logger.Warn("devserver: vite did not become ready", logKeyError, err)
	}
	return nil
}

// Stop terminates the Vite process if it's running.
func (v *viteSupervisor) Stop() {
	v.mu.Lock()
	cmd := v.cmd
	cancel := v.cancel
	done := v.done
	v.cmd = nil
	v.cancel = nil
	v.done = nil
	v.mu.Unlock()
	if cmd == nil {
		return
	}
	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
	case <-time.After(shutdownGrace):
	}
}

// viteArgs returns argv to invoke `vite --config <cfg> --port <port>`
// using the detected package manager.
func viteArgs(pm, configPath string, port int) []string {
	core := []string{"vite", "--config", configPath, "--port", strconv.Itoa(port), "--strictPort"}
	switch pm {
	case "pnpm":
		return append([]string{"pnpm", "exec"}, core...)
	case "bun":
		return append([]string{"bun", "x"}, core...)
	default:
		return append([]string{"npx"}, core...)
	}
}

// detectPackageManager mirrors the build-side detector. SVELTEGO_PM
// overrides; otherwise lockfile presence picks pnpm/bun, falling back to
// npm.
func detectPackageManager(root string) string {
	if pm := os.Getenv("SVELTEGO_PM"); pm != "" {
		return pm
	}
	for _, c := range []struct {
		lock string
		pm   string
	}{
		{"pnpm-lock.yaml", "pnpm"},
		{"bun.lockb", "bun"},
		{"bun.lock", "bun"},
	} {
		if _, err := os.Stat(filepath.Join(root, c.lock)); err == nil {
			return c.pm
		}
	}
	return "npm"
}

// ensureNodeModules runs `<pm> install` once when package.json exists
// but node_modules does not. Mirrors the build-side helper so the dev
// loop has the same first-run experience.
func ensureNodeModules(ctx context.Context, root, pm string, stdout, stderr io.Writer) error {
	pkgJSON := filepath.Join(root, "package.json")
	if _, err := os.Stat(pkgJSON); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, "node_modules")); err == nil {
		return nil
	}
	install := exec.CommandContext(ctx, pm, "install") //nolint:gosec // pm comes from validated detector
	install.Dir = root
	install.Stdout = stdout
	install.Stderr = stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("devserver: %s install: %w", pm, err)
	}
	return nil
}

// waitDial polls 127.0.0.1:port until either the dial succeeds or the
// timeout elapses. Used by both the Go and Vite supervisors.
func waitDial(ctx context.Context, port int, timeout time.Duration) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("dial %s: %w", addr, err)
		}
		time.Sleep(150 * time.Millisecond)
	}
}
