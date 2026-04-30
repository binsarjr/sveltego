package devserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// proxy is the single HTTP server the developer's browser hits. It
// forwards `/static/_app/*` and any Vite-internal paths (`/@vite/`,
// `/@id/`, `/@fs/`, `/node_modules/`) to the Vite dev server, and
// everything else to the user's Go server. While a Go-side rebuild is
// in progress the proxy returns 503 with a friendly retry-after rather
// than racing the swap.
type proxy struct {
	logger     *slog.Logger
	goProxy    *httputil.ReverseProxy
	viteProxy  *httputil.ReverseProxy
	viteOK     bool
	server     *http.Server
	listenAddr string
	rebuilding atomic.Bool

	mu      sync.RWMutex
	lastErr error
}

func newProxy(listenAddr string, goPort, vitePort int, logger *slog.Logger) (*proxy, error) {
	goURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", goPort))
	if err != nil {
		return nil, fmt.Errorf("devserver: parse go target: %w", err)
	}
	p := &proxy{
		logger:     logger,
		goProxy:    httputil.NewSingleHostReverseProxy(goURL),
		listenAddr: listenAddr,
	}
	p.goProxy.ErrorHandler = p.onProxyError("go")
	if vitePort > 0 {
		viteURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", vitePort))
		if err != nil {
			return nil, fmt.Errorf("devserver: parse vite target: %w", err)
		}
		p.viteProxy = httputil.NewSingleHostReverseProxy(viteURL)
		p.viteProxy.ErrorHandler = p.onProxyError("vite")
		p.viteOK = true
	}
	return p, nil
}

// SetRebuilding toggles the 503-during-rebuild flag. Callers wrap the
// Go-side rebuild window in SetRebuilding(true)/SetRebuilding(false)
// pairs so requests arriving in between get a clear status instead of
// a connection-refused.
func (p *proxy) SetRebuilding(rebuilding bool) {
	p.rebuilding.Store(rebuilding)
}

// LastError returns the last proxy-level error, useful for tests that
// need to distinguish "Vite isn't up yet" from a real bug.
func (p *proxy) LastError() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastErr
}

func (p *proxy) onProxyError(target string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		p.mu.Lock()
		p.lastErr = err
		p.mu.Unlock()
		// Cancellation during shutdown is normal; don't shout.
		if errors.Is(err, context.Canceled) {
			return
		}
		p.logger.Warn("devserver: proxy error",
			logKeyTarget, target,
			logKeyPath, r.URL.Path,
			logKeyError, err,
		)
		http.Error(w, fmt.Sprintf("sveltego dev: %s upstream unreachable", target), http.StatusBadGateway)
	}
}

// ServeHTTP routes the request to the right backend. Order:
//  1. Rebuild window short-circuit.
//  2. Vite-owned paths (assets, HMR, internals).
//  3. Everything else → Go server.
func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.rebuilding.Load() {
		w.Header().Set("Retry-After", "1")
		http.Error(w, "sveltego dev: rebuilding...", http.StatusServiceUnavailable)
		return
	}
	if p.viteOK && isViteOwned(r.URL.Path) {
		p.viteProxy.ServeHTTP(w, r)
		return
	}
	p.goProxy.ServeHTTP(w, r)
}

// isViteOwned returns true for the paths Vite must answer in dev mode:
// the static asset prefix the project emits to and Vite's own
// development helpers.
func isViteOwned(path string) bool {
	if strings.HasPrefix(path, "/static/_app/") {
		return true
	}
	if strings.HasPrefix(path, "/@vite/") || strings.HasPrefix(path, "/@id/") || strings.HasPrefix(path, "/@fs/") {
		return true
	}
	if strings.HasPrefix(path, "/node_modules/") {
		return true
	}
	if strings.HasPrefix(path, "/__vite_ping") {
		return true
	}
	return false
}

// Start spins up the HTTP server in a goroutine. Stop is responsible
// for graceful shutdown; the returned error is non-nil only when the
// listener can't be created.
func (p *proxy) Start(_ context.Context) error {
	p.server = &http.Server{
		Addr:              p.listenAddr,
		Handler:           p,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := newListener(p.listenAddr)
	if err != nil {
		return err
	}
	go func() {
		if err := p.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.logger.Error("devserver: proxy serve", logKeyError, err)
		}
	}()
	return nil
}

// Stop shuts the proxy down with a 3s grace period.
func (p *proxy) Stop() {
	if p.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	_ = p.server.Shutdown(ctx)
}
