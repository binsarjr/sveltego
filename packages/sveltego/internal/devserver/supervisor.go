package devserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// startTimeout is how long we wait for the freshly-built Go server to
// open its listening port before declaring the start a failure.
const startTimeout = 8 * time.Second

// shutdownGrace is how long we let the previous Go server flush after
// SIGTERM before sending SIGKILL.
const shutdownGrace = 3 * time.Second

// goSupervisor builds and runs the user's Go server as a child process.
// On every Reload() it stops the running binary, rebuilds against the
// fresh `.gen/` output, then starts it again on the same port. The
// supervisor blocks new requests during the rebuild window — there is no
// hot-swap of in-flight requests in this MVP.
type goSupervisor struct {
	root    string
	mainPkg string
	binPath string
	port    int
	logger  *slog.Logger
	stdout  io.Writer
	stderr  io.Writer
	mu      sync.Mutex
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	done    chan struct{}
}

// newGoSupervisor wires a supervisor that builds mainPkg into a temp
// binary and runs it pointed at port.
func newGoSupervisor(root, mainPkg string, port int, logger *slog.Logger, stdout, stderr io.Writer) (*goSupervisor, error) {
	binDir, err := os.MkdirTemp("", "sveltego-dev-*")
	if err != nil {
		return nil, fmt.Errorf("devserver: tempdir: %w", err)
	}
	binName := "app"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	return &goSupervisor{
		root:    root,
		mainPkg: mainPkg,
		binPath: filepath.Join(binDir, binName),
		port:    port,
		logger:  logger,
		stdout:  stdout,
		stderr:  stderr,
	}, nil
}

// Start performs the initial build + run. The returned error is
// non-nil only when the build fails; a started process that later exits
// is reported via the logger.
func (s *goSupervisor) Start(ctx context.Context) error {
	if err := s.build(ctx); err != nil {
		return err
	}
	return s.spawn(ctx)
}

// Reload stops the current process, rebuilds, and starts a fresh one.
// The returned error covers the build phase only; spawn failures are
// reported but do not propagate so the watcher loop can keep running.
func (s *goSupervisor) Reload(ctx context.Context) error {
	s.stop()
	if err := s.build(ctx); err != nil {
		return err
	}
	if err := s.spawn(ctx); err != nil {
		s.logger.Error("devserver: server spawn failed", logKeyError, err)
		return err
	}
	return nil
}

// Stop terminates the running child and removes the temp binary.
func (s *goSupervisor) Stop() {
	s.stop()
	if dir := filepath.Dir(s.binPath); dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// Port returns the port the supervised process is asked to listen on.
func (s *goSupervisor) Port() int { return s.port }

func (s *goSupervisor) build(ctx context.Context) error {
	build := exec.CommandContext(ctx, "go", "build", "-o", s.binPath, s.mainPkg) //nolint:gosec // args composed from validated config
	build.Dir = s.root
	build.Stdout = s.stdout
	build.Stderr = s.stderr
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if err := build.Run(); err != nil {
		return fmt.Errorf("devserver: go build %s: %w", s.mainPkg, err)
	}
	return nil
}

func (s *goSupervisor) spawn(parent context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, s.binPath) //nolint:gosec // path is supervisor-owned tempfile
	cmd.Dir = s.root
	cmd.Stdout = s.stdout
	cmd.Stderr = s.stderr
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", s.port),
		fmt.Sprintf("SVELTEGO_DEV_PORT=%d", s.port),
		"SVELTEGO_DEV=1",
	)
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("devserver: start %s: %w", s.binPath, err)
	}
	done := make(chan struct{})
	s.cmd = cmd
	s.cancel = cancel
	s.done = done

	go func() {
		defer close(done)
		err := cmd.Wait()
		if err != nil && parent.Err() == nil && ctx.Err() == nil {
			// Unexpected exit (not a Stop / Reload). Surface it so the
			// developer sees the crash without having to hunt the log.
			s.logger.Error("devserver: server process exited", logKeyError, err)
		}
	}()

	if err := waitDial(parent, s.port, startTimeout); err != nil {
		return fmt.Errorf("devserver: server did not start listening on 127.0.0.1:%d within %s: %w", s.port, startTimeout, err)
	}
	return nil
}

// stop terminates the running child via SIGTERM, falls back to SIGKILL
// (via CommandContext cancellation) after shutdownGrace, and waits for
// the spawn goroutine to release the underlying *os.Process. Calling
// stop with no running cmd is a no-op.
func (s *goSupervisor) stop() {
	s.mu.Lock()
	cmd := s.cmd
	cancel := s.cancel
	done := s.done
	s.cmd = nil
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()

	if cmd == nil {
		return
	}
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case <-done:
	case <-time.After(shutdownGrace):
		if cancel != nil {
			cancel() // CommandContext sends SIGKILL on cancel.
		}
		<-done
	}
	if cancel != nil {
		cancel()
	}
}
