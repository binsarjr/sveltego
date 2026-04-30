package cookiesession

import (
	"net/http"
	"sync"
	"time"
)

// Options configures a Session.
type Options struct {
	// Name is the cookie name (required). For chunked sessions this is also
	// the base name; chunks are stored as <Name>.0, <Name>.1, etc.
	Name string

	// MaxAge is the session lifetime. Zero means session cookie (expires on
	// browser close).
	MaxAge time.Duration

	// Path is the cookie Path attribute. Defaults to "/".
	Path string

	// Domain is the cookie Domain attribute. Empty means host-only.
	Domain string

	// Secure overrides automatic HTTPS detection for the Secure attribute.
	// When nil, Secure is set when r.TLS != nil.
	Secure *bool

	// SameSite is the cookie SameSite attribute. Defaults to Lax.
	SameSite http.SameSite
}

func (o *Options) path() string {
	if o.Path == "" {
		return "/"
	}
	return o.Path
}

func (o *Options) sameSite() http.SameSite {
	if o.SameSite == 0 {
		return http.SameSiteLaxMode
	}
	return o.SameSite
}

func (o *Options) secure(r *http.Request) bool {
	if o.Secure != nil {
		return *o.Secure
	}
	return r != nil && r.TLS != nil
}

// Session holds a decoded, type-safe session payload. It is request-scoped;
// do not share a Session across goroutines. Concurrent mutation is detected
// and panics with a clear message.
type Session[T any] struct {
	mu        sync.RWMutex
	codec     Codec
	opts      Options
	r         *http.Request
	w         http.ResponseWriter
	p         payload[T] // decoded payload
	dirty     bool       // true when changes need to be flushed
	destroyed bool       // true after Destroy
}

// NewSession creates a Session[T] and immediately loads the session data from
// the request cookies. If no cookie is found, the session starts empty with
// the zero value of T. Any decode failure (tampered, wrong key, malformed
// JSON) is returned as an error; the session is still usable with a zero
// value T.
func NewSession[T any](r *http.Request, w http.ResponseWriter, codec Codec, opts Options) (*Session[T], error) {
	s := &Session[T]{
		codec: codec,
		opts:  opts,
		r:     r,
		w:     w,
	}

	wire, err := s.readWire()
	if err != nil {
		// No cookie or chunk-meta missing — start fresh.
		return s, nil
	}

	p, err := decodePayload[T](codec, wire)
	if err != nil {
		return s, err
	}

	// Check expiry.
	if !p.Expires.IsZero() && time.Now().After(p.Expires) {
		// Expired — treat as empty, mark dirty to clear on next flush.
		s.dirty = true
		s.destroyed = true
		return s, nil
	}

	s.p = p
	return s, nil
}

// Data returns the current session value. Safe for concurrent reads.
func (s *Session[T]) Data() T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.p.Data
}

// Expires returns the session expiry time stamped in the payload.
// Zero value means the payload has no expiry set.
func (s *Session[T]) Expires() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.p.Expires
}

// NeedsSync reports whether the session has pending changes not yet flushed.
func (s *Session[T]) NeedsSync() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dirty
}

// IsDirty is an alias for NeedsSync.
func (s *Session[T]) IsDirty() bool {
	return s.NeedsSync()
}

// Set replaces the session data and marks the session dirty. It flushes the
// updated cookie(s) to the response immediately.
func (s *Session[T]) Set(v T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.p.Data = v
	if s.opts.MaxAge > 0 {
		s.p.Expires = time.Now().Add(s.opts.MaxAge)
	}
	s.dirty = true
	s.destroyed = false
	return s.flush()
}

// Update applies fn to a copy of the current data and calls Set with the
// result. It is safe to call from a single goroutine; concurrent calls on
// the same Session panic.
func (s *Session[T]) Update(fn func(T) T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.p.Data = fn(s.p.Data)
	if s.opts.MaxAge > 0 {
		s.p.Expires = time.Now().Add(s.opts.MaxAge)
	}
	s.dirty = true
	s.destroyed = false
	return s.flush()
}

// Refresh resets the expiry to now+d (or opts.MaxAge if d is empty) and
// flushes the cookie. Useful for sliding-window sessions.
func (s *Session[T]) Refresh(d ...time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dur := s.opts.MaxAge
	if len(d) > 0 && d[0] > 0 {
		dur = d[0]
	}
	if dur > 0 {
		s.p.Expires = time.Now().Add(dur)
	}
	s.dirty = true
	return s.flush()
}

// Destroy clears the session and emits deletion cookies (Max-Age=0) to the
// response.
func (s *Session[T]) Destroy() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var zero T
	s.p = payload[T]{}
	s.p.Data = zero
	s.dirty = true
	s.destroyed = true
	s.deleteAllCookies()
	return nil
}

// flush encodes and writes the session cookie(s) to the response. Must be
// called with s.mu held.
func (s *Session[T]) flush() error {
	if s.w == nil {
		return nil
	}
	wire, err := encodePayload(s.codec, s.p)
	if err != nil {
		return err
	}

	chunks := splitChunks(wire)

	// Read how many chunks were previously written so we can delete orphans.
	prevCount := s.readPrevChunkCount()

	if len(chunks) == 1 {
		// Single-cookie path: delete any old chunk cookies if we previously
		// used chunked mode, then set the plain cookie.
		for i := 0; i < prevCount; i++ {
			s.deleteCookie(chunkName(s.opts.Name, i))
		}
		if prevCount > 0 {
			s.deleteCookie(metaName(s.opts.Name))
		}
		s.setCookie(s.opts.Name, chunks[0])
	} else {
		// Chunked path: delete the plain cookie if it exists in the request
		// (switching from single-cookie to chunked mode).
		if s.r != nil {
			if ck, err := s.r.Cookie(s.opts.Name); err == nil && ck.Value != "" {
				s.deleteCookie(s.opts.Name)
			}
		}
		// Delete orphan chunks from a previous larger payload.
		for i := len(chunks); i < prevCount; i++ {
			s.deleteCookie(chunkName(s.opts.Name, i))
		}
		s.setCookie(metaName(s.opts.Name), encodeChunkMeta(len(chunks)))
		for i, c := range chunks {
			s.setCookie(chunkName(s.opts.Name, i), c)
		}
	}
	return nil
}

// readWire assembles the full encrypted wire string from the request cookies.
// Returns an error if the cookie is absent.
func (s *Session[T]) readWire() (string, error) {
	if s.r == nil {
		return "", http.ErrNoCookie
	}

	// Try plain cookie first (skip empty-value deletion cookies).
	if ck, err := s.r.Cookie(s.opts.Name); err == nil && ck.Value != "" {
		return ck.Value, nil
	}

	// Try chunked mode.
	metaCk, err := s.r.Cookie(metaName(s.opts.Name))
	if err != nil {
		return "", http.ErrNoCookie
	}
	n, err := decodeChunkMeta(metaCk.Value)
	if err != nil {
		return "", err
	}
	var parts []string
	for i := range n {
		ck, err := s.r.Cookie(chunkName(s.opts.Name, i))
		if err != nil {
			return "", err
		}
		parts = append(parts, ck.Value)
	}
	return joinChunks(parts), nil
}

// readPrevChunkCount returns how many chunks are currently present in the
// request cookies (0 means no chunked cookies found).
func (s *Session[T]) readPrevChunkCount() int {
	if s.r == nil {
		return 0
	}
	metaCk, err := s.r.Cookie(metaName(s.opts.Name))
	if err != nil {
		return 0
	}
	n, err := decodeChunkMeta(metaCk.Value)
	if err != nil {
		return 0
	}
	return n
}

// setCookie writes a session cookie to the response.
func (s *Session[T]) setCookie(name, value string) {
	if s.w == nil {
		return
	}
	ck := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     s.opts.path(),
		Domain:   s.opts.Domain,
		HttpOnly: true,
		Secure:   s.opts.secure(s.r),
		SameSite: s.opts.sameSite(),
	}
	if s.opts.MaxAge > 0 {
		ck.MaxAge = int(s.opts.MaxAge.Seconds())
		ck.Expires = time.Now().Add(s.opts.MaxAge)
	}
	http.SetCookie(s.w, ck)
}

// deleteCookie emits a Max-Age=-1 deletion cookie.
func (s *Session[T]) deleteCookie(name string) {
	if s.w == nil {
		return
	}
	http.SetCookie(s.w, &http.Cookie{
		Name:    name,
		Value:   "",
		Path:    s.opts.path(),
		Domain:  s.opts.Domain,
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
}

// deleteAllCookies emits deletion cookies for the plain cookie, meta, and all
// possible chunk cookies currently visible in the request.
func (s *Session[T]) deleteAllCookies() {
	if s.w == nil {
		return
	}
	s.deleteCookie(s.opts.Name)

	prevCount := s.readPrevChunkCount()
	if prevCount > 0 {
		s.deleteCookie(metaName(s.opts.Name))
		for i := range prevCount {
			s.deleteCookie(chunkName(s.opts.Name, i))
		}
	}
}
