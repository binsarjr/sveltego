package kit

import "time"

// DefaultStaticImmutablePrefix is the URL path prefix the static handler
// treats as fingerprinted output. Files served from under this prefix
// receive the long-lived `immutable` cache directive; everything else
// falls back to no-cache. The default matches SvelteKit's client bundle
// layout (`_app/immutable/...`).
const DefaultStaticImmutablePrefix = "/_app/immutable/"

// DefaultStaticMaxAge is the cache lifetime applied to fingerprinted
// assets when StaticConfig.MaxAge is zero. One year aligns with the
// HTTP semantics for `immutable` and matches Caddy/Nginx defaults.
const DefaultStaticMaxAge = 365 * 24 * time.Hour

// StaticConfig configures the static asset handler returned by
// server.StaticHandler. Dir is the only required field; the rest enable
// optional behavior with safe defaults.
type StaticConfig struct {
	// Dir is the directory the handler serves from. Must be non-empty
	// and must exist; absolute paths are recommended so relative-path
	// resolution is not coupled to the binary's CWD.
	Dir string

	// MaxAge is the Cache-Control max-age applied to fingerprinted
	// assets matched by ImmutablePrefix. Zero falls back to
	// DefaultStaticMaxAge.
	MaxAge time.Duration

	// ImmutablePrefix overrides DefaultStaticImmutablePrefix. Requests
	// whose URL path begins with this string get the long-TTL,
	// `immutable` cache header; all other requests get
	// `no-cache, must-revalidate`.
	ImmutablePrefix string

	// ETag toggles emission of strong ETag headers derived from file
	// mtime and size. The zero value (false) keeps the handler quiet
	// so callers that front the handler with their own validators do
	// not get duplicated headers.
	ETag bool

	// Brotli enables `.br` sibling lookup. When true the handler serves
	// `<path>.br` with `Content-Encoding: br` if the request advertises
	// `br` in Accept-Encoding and the sibling exists on disk.
	Brotli bool

	// Gzip enables `.gz` sibling lookup with the same semantics as
	// Brotli, falling back to gzip when brotli is not negotiated.
	Gzip bool
}
