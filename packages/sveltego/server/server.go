// Package server is the runtime entry point a sveltego app composes in
// its main package: feed it the codegen-emitted route slice, the user's
// app.html shell, and an optional matchers and hooks bundle, and it
// returns an http.Handler that runs the SvelteKit-shaped request pipeline:
//
//	CSP nonce → Reroute → Handle → Match → Load → Render → Response
//
// The CSP middleware (applyCSP) runs before Handle so the
// Content-Security-Policy header and nonce are present on every response
// path — success, error boundary, redirect, and short-circuit alike.
//
// Hooks wired: Handle, HandleError, HandleFetch, Reroute, Init.
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

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/fallback"
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
	// ViteManifest is the JSON content of the Vite manifest produced by
	// `vite build`. When non-empty the server parses it at startup and
	// injects <script type="module"> and <link rel="modulepreload"> tags
	// for the matching route chunk during SSR. Typically embedded in the
	// binary so no runtime FS read is required.
	ViteManifest string
	// ViteBase is the URL base path for Vite assets, e.g. "/static/_app".
	// Defaults to "/static/_app" when ViteManifest is non-empty.
	ViteBase string
	// Prerender, when non-nil, enables the static-first short-circuit:
	// requests whose URL.Path matches a manifest entry receive the
	// pre-baked HTML instead of running the SSR pipeline. Load it via
	// LoadPrerenderManifest at startup.
	Prerender *prerenderTable
	// PrerenderAuth gates protected prerendered routes (#187). When nil
	// the runtime falls back to kit.DenyAllPrerenderAuth — protected
	// pages never serve until the embedding app supplies a real gate.
	PrerenderAuth kit.PrerenderAuthGate
	// ServiceWorker enables the auto-registration <script> for
	// /service-worker.js on every SSR-rendered page. Wire it from the
	// generated `gen.HasServiceWorker` constant; users opt out with
	// `Config{ServiceWorker: false}` even when the file exists. The
	// emitted script is feature-gated on `'serviceWorker' in navigator`
	// so non-supporting browsers no-op silently. Scope is rooted at "/"
	// so SPA navigation under any sub-path is covered (#89).
	ServiceWorker bool
	// SSRFallback configures the long-running Node sidecar used for
	// routes annotated with `<!-- sveltego:ssr-fallback -->` (Phase 8 /
	// #430). When SSRFallback.SidecarDir is empty and at least one
	// route opted in, server boot returns an error pointing at the
	// configuration gap. When no route opted in, the entire field is
	// ignored and Node is never spawned.
	SSRFallback SSRFallbackConfig
	// VersionPoll configures the SvelteKit-style `updated` rune. When
	// ViteManifest is non-empty the server hashes it at boot and
	// serves the digest at /_app/version.json. The generated client
	// poller compares the digest against the hydrated build version
	// on Resolve().PollInterval and flips `updated.current` on drift.
	// Disabled true keeps the endpoint alive (so updated.check() still
	// works) but suppresses the background poll.
	VersionPoll kit.VersionPollConfig
}

// Server is the http.Handler implementation that drives a sveltego app.
//
// Logger is a public field for ergonomic access in tests and single-binary
// apps. Do not mutate Logger after calling New or once the server begins
// serving; concurrent reads in ServeHTTP vs writes from the caller are
// racy. A future stable release will replace this with an unexported field
// and a Logger() accessor.
type Server struct {
	tree          *router.Tree
	Logger        *slog.Logger
	hooks         kit.Hooks
	csp           kit.CSPConfig
	cspTemplate   *kit.CSPTemplate
	streamTimeout time.Duration
	cronTasks     []kit.CronTask

	shellHead string
	shellMid  string
	shellTail string

	// viteManifest holds parsed Vite chunk metadata for asset tag injection.
	viteManifest viteManifestMap
	viteBase     string

	// serviceWorker is the precomputed registration <script> tag emitted
	// before </body> when Config.ServiceWorker is true. Empty disables the
	// runtime injection entirely (#89).
	serviceWorker string

	// clientManifest is the SPA route table embedded in the initial SSR
	// payload so the client SPA router can match link URLs and look up
	// route modules without a separate manifest fetch (#37).
	clientManifest []clientManifestEntry

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

	// prerender, when non-nil, holds the manifest the runtime consults
	// before falling through to SSR. prerenderAuth gates protected
	// entries (#187).
	prerender     *prerenderTable
	prerenderAuth kit.PrerenderAuthGate

	// fallbackConfig captures the Phase 8 fallback sidecar settings.
	// fallbackSupervisor holds the live Supervisor when the sidecar
	// has been started via Init/ListenAndServe; nil otherwise. The
	// field is read by Shutdown to stop the supervisor on graceful
	// exit and is owned by the server (so users don't manage two
	// Stop calls for it).
	fallbackConfig     SSRFallbackConfig
	fallbackSupervisor *fallback.Supervisor

	// appVersion is the build version digest served at
	// /_app/version.json and seeded into the client hydration payload
	// so the poller knows what to compare against. Empty string when
	// no ViteManifest was supplied — the endpoint then reports 404 so
	// a client never flips on the absence of a hash.
	appVersion string
	// versionPoll is the resolved poller config (zero values filled in
	// from kit defaults).
	versionPoll kit.VersionPollConfig
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
	var vm viteManifestMap
	if cfg.ViteManifest != "" {
		vm, err = parseViteManifest(cfg.ViteManifest)
		if err != nil {
			return nil, err
		}
	}
	viteBase := cfg.ViteBase
	if viteBase == "" && vm != nil {
		viteBase = "/static/_app"
	}

	swTag := ""
	if cfg.ServiceWorker {
		swTag = serviceWorkerRegisterScript
	}
	done := make(chan struct{})
	srv := &Server{
		tree:            tree,
		Logger:          logger,
		hooks:           cfg.Hooks.WithDefaults(),
		csp:             cfg.CSP,
		cspTemplate:     kit.NewCSPTemplate(cfg.CSP),
		streamTimeout:   streamTimeout,
		cronTasks:       cfg.CronTasks,
		shellHead:       head,
		shellMid:        mid,
		shellTail:       tail,
		viteManifest:    vm,
		viteBase:        viteBase,
		serviceWorker:   swTag,
		clientManifest:  buildClientManifest(tree.Routes()),
		initDone:        done,
		initTimeout:     initTimeout,
		initPendingHTML: initPendingHTML,
		initErrorHTML:   initErrorHTML,
		prerender:       cfg.Prerender,
		prerenderAuth:   cfg.PrerenderAuth,
		fallbackConfig:  cfg.SSRFallback,
		appVersion:      computeAppVersion(cfg.ViteManifest),
		versionPoll:     cfg.VersionPoll.Resolve(),
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
//
// The prerender table, when present, short-circuits before Init: a
// matching static HTML hit is served without waiting for Init or
// running Handle. This is intentional — the whole point of prerender
// is "no Go code per request". Protected entries gate via
// PrerenderAuth, falling through to the live SSR pipeline (with the
// normal init checks) on deny.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.serveVersion(w, r) {
		return
	}
	if s.prerender != nil && s.servePrerendered(w, r) {
		return
	}
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
		close(done)
		return
	}
	if err := s.startFallbackSidecar(ctx); err != nil {
		s.Logger.Error("server: ssr fallback sidecar boot failed", logKeyError, err.Error())
		s.initState.Store(initFailed)
		close(done)
		return
	}
	s.initState.Store(initReady)
	s.startCron(ctx)
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
	if err := s.startFallbackSidecar(ctx); err != nil {
		s.initState.Store(initFailed)
		close(done)
		return fmt.Errorf("server: ssr fallback sidecar: %w", err)
	}
	s.initState.Store(initReady)
	close(done)
	s.startCron(ctx)
	return nil
}

// startFallbackSidecar boots the long-running Node sidecar exactly when
// the codegen-emitted init() registered at least one annotated route on
// fallback.Default(). The supervisor handle is stashed on the Server so
// Shutdown can stop it.
func (s *Server) startFallbackSidecar(ctx context.Context) error {
	supervisor, err := StartFallbackSidecar(ctx, s.fallbackConfig)
	if err != nil {
		return err
	}
	if supervisor != nil {
		s.mu.Lock()
		s.fallbackSupervisor = supervisor
		s.mu.Unlock()
		s.Logger.Info("server: ssr fallback sidecar started", logKeyEndpoint, supervisor.CurrentEndpoint())
	}
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
// The fallback sidecar (Phase 8 / #430) is also stopped when running.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	cancel := s.cronCancel
	supervisor := s.fallbackSupervisor
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if supervisor != nil {
		supervisor.Stop()
	}
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	return nil
}
