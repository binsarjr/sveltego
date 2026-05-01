package fallback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RenderRequest carries the inputs the sidecar needs to render a
// fallback route at request time.
//
//   - Route is the canonical route pattern (e.g. "/posts/[id]"). The
//     sidecar uses it to look up the precompiled component module.
//   - Source is the project-relative `_page.svelte` path the build
//     captured for this route. The sidecar reads, compiles, and
//     module-caches it on first request per route.
//   - Data is the load-result struct passed to Svelte's `$props.data`.
//     It must be JSON-serialisable; the request hash includes the
//     marshalled bytes so two equal payloads hit the cache.
type RenderRequest struct {
	Route  string
	Source string
	Data   any
}

// RenderResponse mirrors the HTML the sidecar emits.
type RenderResponse struct {
	Body string
	Head string
}

// Client speaks to the long-running Node sidecar via HTTP on
// localhost. HTTP — instead of a Unix socket — keeps the path
// debuggable from a browser, avoids socket-path length limits on
// older systems, and matches the existing build-time sidecar's stdin
// → stdout JSON shape. The slight per-request loopback overhead is
// negligible compared to the cached path that satisfies the common
// case.
//
// Cache is a per-Client LRU with TTL; misses fall through to the
// sidecar. The cache key is `route|sha256(json(data))` — JSON marshal
// before hashing so semantically equal payloads collide regardless of
// Go pointer identity.
type Client struct {
	endpoint string
	http     *http.Client
	cache    *lruCache
}

// ClientOptions configures a fallback Client. CacheSize is the maximum
// number of cached entries (default 1000). TTL is the per-entry expiry
// (default 60s — long enough to amortise burst traffic, short enough
// to surface upstream changes). Endpoint is the base URL of the
// sidecar; the Client appends "/render" for render calls.
type ClientOptions struct {
	Endpoint  string
	CacheSize int
	TTL       time.Duration
	HTTP      *http.Client
}

// NewClient wires an HTTP client + cache for the configured sidecar.
// The HTTP client carries a short default timeout so a stuck sidecar
// can't hold a request goroutine indefinitely; callers may override.
func NewClient(opts ClientOptions) *Client {
	capacity := opts.CacheSize
	if capacity == 0 {
		capacity = 1000
	}
	ttl := opts.TTL
	if ttl == 0 {
		ttl = 60 * time.Second
	}
	httpc := opts.HTTP
	if httpc == nil {
		httpc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		endpoint: opts.Endpoint,
		http:     httpc,
		cache:    newLRUCache(capacity, ttl),
	}
}

// Render returns the HTML for req, hitting the cache when possible.
// Cache key: route + SHA-256 of the JSON-marshalled data. Cache miss
// or marshal-error both go through the sidecar; only successful renders
// land in the cache.
func (c *Client) Render(ctx context.Context, req RenderRequest) (RenderResponse, error) {
	dataBytes, err := json.Marshal(req.Data)
	if err != nil {
		return c.renderUncached(ctx, req)
	}
	key := cacheKey(req.Route, dataBytes)
	if entry, ok := c.cache.Get(key); ok {
		return RenderResponse{Body: entry.body, Head: entry.head}, nil
	}
	resp, err := c.callSidecar(ctx, req, dataBytes)
	if err != nil {
		return RenderResponse{}, err
	}
	c.cache.Put(key, cacheEntry{body: resp.Body, head: resp.Head})
	return resp, nil
}

// renderUncached bypasses the cache; used when JSON marshal of the
// data fails (no stable hash available) but the sidecar can still try.
// The sidecar will produce its own marshal error if applicable.
func (c *Client) renderUncached(ctx context.Context, req RenderRequest) (RenderResponse, error) {
	dataBytes, _ := json.Marshal(req.Data) //nolint:errchkjson // best-effort: sidecar will return an explicit error
	return c.callSidecar(ctx, req, dataBytes)
}

// callSidecar issues the HTTP POST and decodes the response.
func (c *Client) callSidecar(ctx context.Context, req RenderRequest, dataBytes []byte) (RenderResponse, error) {
	body, err := json.Marshal(struct {
		Route  string          `json:"route"`
		Source string          `json:"source"`
		Data   json.RawMessage `json:"data"`
	}{
		Route:  req.Route,
		Source: req.Source,
		Data:   dataBytes,
	})
	if err != nil {
		return RenderResponse{}, &wrappedErr{op: "fallback: marshal request", err: err}
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/render", bytes.NewReader(body))
	if err != nil {
		return RenderResponse{}, &wrappedErr{op: "fallback: build request", err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return RenderResponse{}, &wrappedErr{op: "fallback: sidecar call " + req.Route, err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return RenderResponse{}, errors.New("fallback: sidecar status " + strconv.Itoa(resp.StatusCode) + " for " + req.Route + ": " + string(bytes.TrimSpace(raw)))
	}
	var out struct {
		Body string `json:"body"`
		Head string `json:"head"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RenderResponse{}, &wrappedErr{op: "fallback: decode sidecar response", err: err}
	}
	return RenderResponse{Body: out.Body, Head: out.Head}, nil
}

// cacheKey returns "route|sha256-hex(data)".
func cacheKey(route string, data []byte) string {
	sum := sha256.Sum256(data)
	return route + "|" + hex.EncodeToString(sum[:])
}

// CacheStats exposes a tiny diagnostic surface (size only) for tests
// and admin endpoints.
type CacheStats struct {
	Entries int
}

// Stats returns CacheStats; used by tests and observability hooks.
func (c *Client) Stats() CacheStats {
	return CacheStats{Entries: c.cache.Len()}
}

// clientPool synchronises Client creation when callers race to register
// the runtime singleton. Currently unused — exported so refactors can
// route through it without a public API change.
//
//nolint:unused // kept for forward compatibility with multi-client modes
var clientPool sync.Pool

// wrappedErr is a tiny errors.New + errors.Unwrap-compatible wrapper.
// Used in place of fmt.Errorf which depguard forbids in runtime/.
type wrappedErr struct {
	op  string
	err error
}

func (e *wrappedErr) Error() string {
	return e.op + ": " + e.err.Error()
}

func (e *wrappedErr) Unwrap() error { return e.err }
