package fallback

import (
	"context"
	"errors"
	"sync"
)

// Registry holds the wiring between annotated routes (the codegen
// output names them) and the sidecar Client that renders them. Codegen
// emits one Register call per annotated route from a generated init();
// the server runtime calls Configure once at boot to inject the live
// Client. Render then dispatches per-request.
//
// The registry is process-global. Tests that need isolation can call
// Reset, which clears the route table and detaches the Client without
// killing the sidecar process — leaks of the latter are the test's
// responsibility.
type Registry struct {
	mu     sync.RWMutex
	routes map[string]string
	client *Client
}

// defaultRegistry is the package-level registry codegen targets.
var defaultRegistry = &Registry{routes: make(map[string]string)}

// Default returns the package-global Registry.
func Default() *Registry {
	return defaultRegistry
}

// Register associates pattern with the absolute or project-relative
// `_page.svelte` source path the sidecar should render. Codegen emits
// one Register call per annotated route from a generated init().
func (r *Registry) Register(pattern, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[pattern] = source
}

// Configure wires the Client used to render every registered route.
// Calling Configure twice replaces the previous Client; the old Client
// is not closed because there is nothing to close — its sidecar
// process is owned by the caller via Supervisor.Stop.
func (r *Registry) Configure(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.client = client
}

// Routes returns the registered patterns in registration order — used
// by the server boot to decide whether to launch the sidecar at all.
func (r *Registry) Routes() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.routes))
	for k, v := range r.routes {
		out[k] = v
	}
	return out
}

// HasRoutes reports whether at least one route was registered.
func (r *Registry) HasRoutes() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.routes) > 0
}

// Render dispatches a request through the registered Client. Returns
// an error when no Client has been configured yet (i.e. the runtime
// has not started the sidecar) or when the route was never registered
// — both indicate a build/runtime mismatch worth surfacing rather
// than papering over.
func (r *Registry) Render(ctx context.Context, route string, data any) (RenderResponse, error) {
	r.mu.RLock()
	source, hasRoute := r.routes[route]
	client := r.client
	r.mu.RUnlock()
	if !hasRoute {
		return RenderResponse{}, errors.New("fallback: route not registered: " + route)
	}
	if client == nil {
		return RenderResponse{}, errors.New("fallback: registry not configured (sidecar not started)")
	}
	return client.Render(ctx, RenderRequest{
		Route:  route,
		Source: source,
		Data:   data,
	})
}

// Reset clears the registry — used by tests so global state does not
// bleed across cases. The caller is responsible for stopping any
// sidecar process the previous Client was talking to.
func (r *Registry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = make(map[string]string)
	r.client = nil
}
