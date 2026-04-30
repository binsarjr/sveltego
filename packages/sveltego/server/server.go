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
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/runtime/router"
)

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
}

// Server is the http.Handler implementation that drives a sveltego app.
type Server struct {
	tree   *router.Tree
	Logger *slog.Logger
	hooks  kit.Hooks

	shellHead string
	shellMid  string
	shellTail string

	mu      sync.Mutex
	httpSrv *http.Server
}

// New validates cfg and returns a Server ready for use as an http.Handler.
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
	return &Server{
		tree:      tree,
		Logger:    logger,
		hooks:     cfg.Hooks.WithDefaults(),
		shellHead: head,
		shellMid:  mid,
		shellTail: tail,
	}, nil
}

// ServeHTTP routes a single request through the pipeline.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handle(w, r)
}

// Handler returns the http.Handler form of s; useful when wrapping
// the server in user-supplied middleware.
func (s *Server) Handler() http.Handler {
	return s
}

// ListenAndServe binds the server to addr and serves until Shutdown is
// called or the listener errors. The Init hook (when configured) runs
// once before the listener binds; an error from Init aborts startup.
func (s *Server) ListenAndServe(addr string) error {
	if err := s.runInit(context.Background()); err != nil {
		return err
	}
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

// Init runs the configured Init hook with ctx. Useful for callers that
// want to control startup (signal handling, custom listener) outside of
// ListenAndServe. ListenAndServe calls this internally with a
// context.Background() before binding.
func (s *Server) Init(ctx context.Context) error {
	return s.runInit(ctx)
}

// runInit invokes the Init hook and wraps its error with a stable prefix
// so callers can distinguish startup failures from listener errors.
func (s *Server) runInit(ctx context.Context) error {
	if s.hooks.Init == nil {
		return nil
	}
	if err := s.hooks.Init(ctx); err != nil {
		return fmt.Errorf("server: init hook: %w", err)
	}
	return nil
}

// Shutdown gracefully stops a server started via ListenAndServe.
// In-flight requests complete; new connections are refused.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server: shutdown: %w", err)
	}
	return nil
}
