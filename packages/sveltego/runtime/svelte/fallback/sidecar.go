package fallback

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// sidecarReadyTimeout bounds how long Start waits for the Node child to
// announce its listening port on stderr. The sidecar prints a single
// `SVELTEGO_SSR_FALLBACK_LISTEN port=NNN` line on boot; if it doesn't
// come within this window we treat the process as broken.
const sidecarReadyTimeout = 30 * time.Second

// readyLinePrefix is the literal stderr line the sidecar emits when it
// has bound a port and is accepting requests. The Go side parses
// `port=NNN` after this prefix.
const readyLinePrefix = "SVELTEGO_SSR_FALLBACK_LISTEN "

// log attribute keys for sloglint compliance.
const (
	logKeyStderr   = "stderr"
	logKeyErr      = "err"
	logKeyRestarts = "restarts"
	logKeyAttempt  = "attempt"
	logKeyBackoff  = "backoff"
)

// SidecarOptions configures the long-running sidecar process.
//
//   - NodePath is the absolute path to `node`. Use exec.LookPath when
//     the caller hasn't pre-resolved it.
//   - SidecarDir is the directory holding the sidecar's index.mjs (the
//     same vendored tree the build-time SSG/SSR mode uses).
//   - ProjectRoot is the absolute project root the sidecar treats as
//     CWD when resolving _page.svelte paths.
//   - Logger receives stderr lines that don't match the ready prefix so
//     operators can debug crashes.
//   - Restart, when true, lets the Supervisor relaunch the process up
//     to MaxRestarts times after a crash. Each restart uses an
//     exponential backoff (1s, 2s, 4s, …, capped at 30s).
type SidecarOptions struct {
	NodePath     string
	SidecarDir   string
	ProjectRoot  string
	Logger       *slog.Logger
	Restart      bool
	MaxRestarts  int
	StartupExtra []string
}

// Sidecar owns a single Node process. Endpoint reflects the address the
// child reported as listening; cancel terminates the process group on
// Stop.
type Sidecar struct {
	endpoint string
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	doneCh   chan error
}

// Endpoint returns the URL the sidecar is listening on (e.g.
// "http://127.0.0.1:54231"). Empty until Start succeeds.
func (s *Sidecar) Endpoint() string {
	return s.endpoint
}

// Wait blocks until the sidecar process exits (whether through Stop,
// crash, or external signal) and returns the exit error. Multiple
// callers may Wait concurrently; the same error is delivered to each.
func (s *Sidecar) Wait() error {
	return <-s.doneCh
}

// Stop cancels the process. Safe to call more than once.
func (s *Sidecar) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Start launches the sidecar and waits for the ready line. It returns
// a Sidecar handle whose Endpoint reports the listening URL. If the
// child does not report ready within sidecarReadyTimeout, Start kills
// it and returns an error.
func Start(parent context.Context, opts SidecarOptions) (*Sidecar, error) {
	if opts.NodePath == "" {
		return nil, errors.New("fallback: NodePath is required")
	}
	if opts.SidecarDir == "" {
		return nil, errors.New("fallback: SidecarDir is required")
	}
	if opts.ProjectRoot == "" {
		return nil, errors.New("fallback: ProjectRoot is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	entry := filepath.Join(opts.SidecarDir, "index.mjs")
	if _, err := os.Stat(entry); err != nil {
		return nil, &wrappedErr{op: "fallback: sidecar entry " + entry, err: err}
	}

	ctx, cancel := context.WithCancel(parent)
	args := []string{entry, "--mode=ssr-serve", "--root=" + opts.ProjectRoot}
	args = append(args, opts.StartupExtra...)
	//nolint:gosec // NodePath/SidecarDir are operator-supplied paths to the vendored sidecar tree, not user-controlled HTTP input
	cmd := exec.CommandContext(ctx, opts.NodePath, args...)
	cmd.Dir = opts.SidecarDir
	cmd.Env = append(os.Environ(), "NODE_NO_WARNINGS=1")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, &wrappedErr{op: "fallback: stderr pipe", err: err}
	}
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, &wrappedErr{op: "fallback: start sidecar", err: err}
	}

	endpointCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go scanStderr(stderr, endpointCh, errCh, logger)

	doneCh := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		doneCh <- err
		close(doneCh)
	}()

	select {
	case ep := <-endpointCh:
		return &Sidecar{
			endpoint: ep,
			cmd:      cmd,
			cancel:   cancel,
			doneCh:   doneCh,
		}, nil
	case err := <-errCh:
		cancel()
		<-doneCh
		return nil, &wrappedErr{op: "fallback: sidecar stderr scan failed", err: err}
	case <-time.After(sidecarReadyTimeout):
		cancel()
		<-doneCh
		return nil, errors.New("fallback: sidecar did not emit ready line within " + sidecarReadyTimeout.String())
	case exitErr := <-doneCh:
		cancel()
		if exitErr != nil {
			return nil, &wrappedErr{op: "fallback: sidecar exited before ready", err: exitErr}
		}
		return nil, errors.New("fallback: sidecar exited before reporting ready")
	}
}

// scanStderr forwards stderr to logger except for the ready line; the
// ready line's port is extracted and pushed onto endpointCh exactly
// once. Subsequent stderr is logged as warnings so operators can see
// crashes mid-flight.
func scanStderr(r io.Reader, endpointCh chan<- string, errCh chan<- error, logger *slog.Logger) {
	scanner := bufio.NewScanner(r)
	announced := false
	for scanner.Scan() {
		line := scanner.Text()
		if !announced && strings.HasPrefix(line, readyLinePrefix) {
			announced = true
			port, err := parseReadyLine(line)
			if err != nil {
				errCh <- err
				return
			}
			endpointCh <- "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
			continue
		}
		logger.Info("svelte fallback sidecar", logKeyStderr, line)
	}
	if err := scanner.Err(); err != nil && !announced {
		errCh <- err
	}
}

// parseReadyLine extracts the integer port from `SVELTEGO_SSR_FALLBACK_LISTEN port=NNN`.
func parseReadyLine(line string) (int, error) {
	rest := strings.TrimPrefix(line, readyLinePrefix)
	const portKey = "port="
	idx := strings.Index(rest, portKey)
	if idx < 0 {
		return 0, errors.New("fallback: ready line missing port=: " + line)
	}
	tail := rest[idx+len(portKey):]
	if sp := strings.IndexAny(tail, " \t\r\n"); sp >= 0 {
		tail = tail[:sp]
	}
	port, err := strconv.Atoi(tail)
	if err != nil {
		return 0, &wrappedErr{op: "fallback: ready line bad port: " + line, err: err}
	}
	if port <= 0 || port > 65535 {
		return 0, errors.New("fallback: ready line port out of range: " + strconv.Itoa(port))
	}
	return port, nil
}

// Supervisor runs a Sidecar with bounded restart-on-crash behaviour.
// The single-attempt path uses Start directly; the supervisor wraps
// Start so per-request handlers can keep dispatching after a one-off
// transient failure. After MaxRestarts crashes within the supervisor's
// lifetime the process is left dead and subsequent requests will
// receive errors from Client.Render.
type Supervisor struct {
	mu       sync.RWMutex
	current  *Sidecar
	logger   *slog.Logger
	opts     SidecarOptions
	cancel   context.CancelFunc
	stopCh   chan struct{}
	maxRetry int
}

// NewSupervisor returns an unstarted Supervisor; call Run to launch.
func NewSupervisor(opts SidecarOptions) *Supervisor {
	maxRetry := opts.MaxRestarts
	if maxRetry <= 0 {
		maxRetry = 3
	}
	return &Supervisor{
		opts:     opts,
		logger:   opts.Logger,
		stopCh:   make(chan struct{}),
		maxRetry: maxRetry,
	}
}

// Run launches the sidecar and replaces it on crash up to MaxRestarts
// times. Returns the first successfully-started Sidecar so the caller
// can read its initial Endpoint. Subsequent endpoints (after a
// restart) are exposed via CurrentEndpoint.
func (s *Supervisor) Run(parent context.Context) (*Sidecar, error) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	first, err := Start(ctx, s.opts)
	if err != nil {
		cancel()
		return nil, err
	}
	s.mu.Lock()
	s.current = first
	s.mu.Unlock()
	if s.opts.Restart {
		go s.loop(ctx)
	}
	return first, nil
}

// loop watches the current sidecar, relaunches on unexpected exit.
func (s *Supervisor) loop(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	restarts := 0
	for {
		s.mu.RLock()
		cur := s.current
		s.mu.RUnlock()
		if cur == nil {
			return
		}
		err := cur.Wait()
		if ctx.Err() != nil {
			return
		}
		if restarts >= s.maxRetry {
			if s.logger != nil {
				s.logger.Error("svelte fallback sidecar crashed past restart budget; falling back routes will error",
					logKeyErr, err, logKeyRestarts, restarts)
			}
			return
		}
		restarts++
		if s.logger != nil {
			s.logger.Warn("svelte fallback sidecar exited; restarting",
				logKeyErr, err, logKeyAttempt, restarts, logKeyBackoff, backoff)
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		next, startErr := Start(ctx, s.opts)
		if startErr != nil {
			if s.logger != nil {
				s.logger.Error("svelte fallback sidecar restart failed", logKeyErr, startErr)
			}
			continue
		}
		s.mu.Lock()
		s.current = next
		s.mu.Unlock()
		backoff = time.Second
	}
}

// CurrentEndpoint returns the sidecar's most recent listening endpoint,
// or empty string when the supervisor has not started a sidecar.
func (s *Supervisor) CurrentEndpoint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current == nil {
		return ""
	}
	return s.current.Endpoint()
}

// Stop kills the sidecar and exits the supervisor loop. Safe to call
// more than once.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	cur := s.current
	s.mu.Unlock()
	if cur != nil {
		cur.Stop()
	}
	if s.cancel != nil {
		s.cancel()
	}
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}
