// Package server is the runtime entry point a sveltego app composes in
// its main package: feed it the codegen-emitted route slice, the user's
// app.html shell, and an optional matchers and hooks bundle, and it
// returns an http.Handler that runs the SvelteKit-shaped Reroute → Handle
// → Match → Load → Render → Response pipeline. Form actions remain out
// of scope for Phase 0; layouts and hooks (Handle, HandleError,
// HandleFetch, Reroute, Init) are wired.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/runtime/router"
)

// initStateVal values for Server.initState.
const (
	initPending int32 = 0
	initReady   int32 = 1
	initFailed  int32 = 2
)

// defaultInitTimeout is the per-request maximum wait for Init to complete
// before the server returns 503 with the pending fallback body.
const defaultInitTimeout = 5 * time.Second

// defaultInitPendingHTML is the 503 body served while Init is in-flight
// and the per-request InitTimeout elapses.
const defaultInitPendingHTML = `<!doctype html><html><body><p>Server is starting, please retry.</p></body></html>`

// defaultInitErrorHTML is the 500 body served for every request after
// Init returns a non-nil error.
const defaultInitErrorHTML = `<!doctype html><html><body><p>Server failed to start. Please check the logs.</p></body></html>`

// defaultReadHeaderTimeout caps how long ListenAndServe waits for
// request headers. Bounds the slowloris attack surface; user code that
// needs custom timeouts can construct an http.Server around Handler().
const defaultReadHeaderTimeout = 10 * time.Second

// Config configures a Server. Routes and Shell are required; Matchers
// defaults to params.DefaultMatchers and Logger to slog.Default.
type Config struct {
	// Routes is the ordered route table, typically gen.Routes().
	Routes []router.Route
	// Matchers is the optional ParamMatcher registry.
	Matchers router.Matchers
	// Shell is the app.html template; must contain %sveltego.head% and
	// %sveltego.body% placeholders in that order.
	Shell string
	// Logger receives lifecycle and error events. Defaults to slog.Default.
	Logger *slog.Logger
	// Hooks is the optional hook bundle. nil fields fall back to the kit
	// identity defaults; pass kit.Hooks{} or gen.Hooks() to wire user
	// hooks discovered by codegen.
	Hooks kit.Hooks
	// CSP enables per-request nonce generation and a matching
	// Content-Security-Policy header. The zero value (Mode CSPOff)
	// disables the middleware so existing behavior is preserved.
	CSP kit.CSPConfig
	// StreamTimeout caps how long the streaming render path waits for
	// each kit.Streamed value before emitting a timeout error patch.
	// Zero falls back to kit.DefaultStreamTimeout.
	StreamTimeout time.Duration
	// InitTimeout is the per-request maximum time to wait for the Init
	// hook to complete before returning 503 with InitPendingHTML. Zero
	// defaults to 5 seconds.
	InitTimeout time.Duration
	// InitPendingHTML is the HTML body returned with 503 when a request
	// arrives while Init is still running and InitTimeout elapses. An
	// empty value falls back to a built-in placeholder page.
	InitPendingHTML string
	// InitErrorHTML is the HTML body returned with 500 for every request
	// after Init returns a non-nil error. An empty value falls back to a
	// built-in error page.
	InitErrorHTML string
	// CronTasks is the optional list of scheduled background tasks. Each
	// task starts a goroutine on server init that ticks at the parsed
	// interval until the server shuts down. Tasks are skipped when their
	// Spec fails to parse; the error is logged and startup continues.
	CronTasks []kit.CronTask
}

// Server is the http.Handler implementation that drives a sveltego app.
type Server struct {
	tree          *router.Tree
	Logger        *slog.Logger
	hooks         kit.Hooks
	csp           kit.CSPConfig
	streamTimeout time.Duration
	cronTasks     []kit.CronTask

	shellHead string
	shellMid  string
	shellTail string

	// initState is initPending/initReady/initFailed; read with atomic.
	initState atomic.Int32
	// initDone is closed once Init completes (success or failure).
	// Protected by initMu when being replaced.
	initDone chan struct{}
	initMu   sync.Mutex
	// initTimeout is the per-request maximum wait time for Init.
	initTimeout time.Duration
	// initPendingHTML is served with 503 when Init hasn't finished in time.
	initPendingHTML string
	// initErrorHTML is served with 500 when Init returned an error.
	initErrorHTML string

	mu         sync.Mutex
	httpSrv    *http.Server
	cronCancel context.CancelFunc
}

// New validates cfg and returns a Server ready for use as an http.Handler.
// Init is not run; call ListenAndServe (which runs it async) or Init (sync)
// before the server begins serving real traffic.
func New(cfg Config) (*Server, error) {
	if len(cfg.Routes) == 0 {
		return nil, errors.New("server: Config.Routes is empty")
	}
	if cfg.Shell == "" {
		return nil, errors.New("server: Config.Shell is empty")
	}
	head, mid, tail, err := parseShell(cfg.Shell)
	if err != nil {
		return nil, err
	}
	matchers := cfg.Matchers
	if matchers == nil {
		matchers = params.DefaultMatchers()
	}
	tree, err := router.NewTree(cfg.Routes)
	if err != nil {
		return nil, fmt.Errorf("server: build route tree: %w", err)
	}
	if _, err := tree.WithMatchers(matchers); err != nil {
		return nil, fmt.Errorf("server: install matchers: %w", err)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	streamTimeout := cfg.StreamTimeout
	if streamTimeout <= 0 {
		streamTimeout = kit.DefaultStreamTimeout
	}
	initTimeout := cfg.InitTimeout
	if initTimeout <= 0 {
		initTimeout = defaultInitTimeout
	}
	initPendingHTML := cfg.InitPendingHTML
	if initPendingHTML == "" {
		initPendingHTML = defaultInitPendingHTML
	}
	initErrorHTML := cfg.InitErrorHTML
	if initErrorHTML == "" {
		initErrorHTML = defaultInitErrorHTML
	}
	done := make(chan struct{})
	srv := &Server{
		tree:            tree,
		Logger:          logger,
		hooks:           cfg.Hooks.WithDefaults(),
		csp:             cfg.CSP,
		streamTimeout:   streamTimeout,
		cronTasks:       cfg.CronTasks,
		shellHead:       head,
		shellMid:        mid,
		shellTail:       tail,
		initDone:        done,
		initTimeout:     initTimeout,
		initPendingHTML: initPendingHTML,
		initErrorHTML:   initErrorHTML,
	}
	// Start as ready so that callers using httptest.NewServer directly
	// (without ListenAndServe or Init) see normal pipeline behavior.
	srv.initState.Store(initReady)
	close(done)
	return srv, nil
}

// initDoneCh returns the current initDone channel under the lock.
// The channel reference is stable once assigned; this is only for
// ServeHTTP to load it safely when the server transitions from
// pre-closed (ready) to a fresh pending channel via startInit.
func (s *Server) initDoneCh() chan struct{} {
	s.initMu.Lock()
	ch := s.initDone
	s.initMu.Unlock()
	return ch
}

// startInit resets initState to pending, replaces initDone with a fresh
// channel, and returns it. Must be called before launching the goroutine.
func (s *Server) startInit() chan struct{} {
	done := make(chan struct{})
	s.initMu.Lock()
	s.initDone = done
	s.initMu.Unlock()
	s.initState.Store(initPending)
	return done
}

// ServeHTTP routes a single request through the pipeline. When Init is
// still running, it waits up to InitTimeout then returns 503 with the
// pending fallback body. When Init has failed it returns 500 with the
// error fallback body immediately.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch s.initState.Load() {
	case initReady:
		// fast path — most requests land here
	case initFailed:
		writeInitFallback(w, http.StatusInternalServerError, s.initErrorHTML)
		return
	default: // initPending
		done := s.initDoneCh()
		select {
		case <-done:
			if s.initState.Load() == initFailed {
				writeInitFallback(w, http.StatusInternalServerError, s.initErrorHTML)
				return
			}
		case <-time.After(s.initTimeout):
			writeInitFallback(w, http.StatusServiceUnavailable, s.initPendingHTML)
			return
		case <-r.Context().Done():
			return
		}
	}
	s.handle(w, r)
}

// Handler returns the http.Handler form of s; useful when wrapping
// the server in user-supplied middleware.
func (s *Server) Handler() http.Handler {
	return s
}

// ListenAndServe binds the server to addr and serves until Shutdown is
// called or the listener errors. The Init hook runs asynchronously after
// the listener binds; requests arriving before Init completes wait up to
// InitTimeout and receive a 503 pending response, or 500 if Init failed.
func (s *Server) ListenAndServe(addr string) error {
	s.RunInitAsync(context.Background())
	s.mu.Lock()
	srv := &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}
	s.httpSrv = srv
	s.mu.Unlock()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server: listen and serve: %w", err)
	}
	return nil
}

// RunInitAsync launches the Init hook in a background goroutine and puts
// the server into the pending state until Init completes. ServeHTTP blocks
// incoming requests until Init resolves or InitTimeout elapses. Callers
// that bind their own listener and want async Init should call this before
// accepting connections.
func (s *Server) RunInitAsync(ctx context.Context) {
	done := s.startInit()
	go s.runInitAsync(ctx, done)
}

func (s *Server) runInitAsync(ctx context.Context, done chan struct{}) {
	err := s.hooks.Init(ctx)
	if err != nil {
		s.Logger.Error("server: init hook failed", logKeyError, err.Error())
		s.initState.Store(initFailed)
	} else {
		s.initState.Store(initReady)
		s.startCron(ctx)
	}
	close(done)
}

// Init runs the configured Init hook synchronously with ctx and returns its
// error. Useful for callers that want to control startup (signal handling,
// custom listener) and prefer a blocking init-before-serve pattern. After
// Init returns without error, ServeHTTP skips the init-state check.
func (s *Server) Init(ctx context.Context) error {
	done := s.startInit()
	err := s.hooks.Init(ctx)
	if err != nil {
		s.initState.Store(initFailed)
		close(done)
		return fmt.Errorf("server: init hook: %w", err)
	}
	s.initState.Store(initReady)
	close(done)
	s.startCron(ctx)
	return nil
}

// writeInitFallback writes a plain HTML fallback response for the init
// pending or failed states. It does not go through the pipeline or
// HandleError because the app hasn't successfully initialized yet.
func writeInitFallback(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// startCron derives a child context from parent and launches background
// cron goroutines. It records the cancel func so Shutdown can stop them.
// No-op when CronTasks is empty.
func (s *Server) startCron(parent context.Context) {
	if len(s.cronTasks) == 0 {
		return
	}
	cronCtx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	s.cronCancel = cancel
	s.mu.Unlock()
	runCronTasks(cronCtx, s.cronTasks, s.Logger)
}

// Shutdown gracefully stops a server started via ListenAndServe.
// In-flight requests complete; new connections are refused. Any running
// cron goroutines are stopped via context cancellation before returning.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	cancel := s.cronCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	return nil
}
