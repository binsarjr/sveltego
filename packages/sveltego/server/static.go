package server

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// StaticHandler returns an http.Handler that serves static files from
// cfg.Dir with optional precompression and conditional-request support.
//
// Behavior summary:
//
//   - URL path is cleaned and rejected if it escapes cfg.Dir via "..".
//   - When cfg.Brotli is true and the request advertises `br`, the
//     handler serves `<file>.br` with Content-Encoding: br if that
//     sibling exists; otherwise it falls back to gzip then to the raw
//     file.
//   - When cfg.Gzip is true and the request advertises `gzip`, the
//     handler serves `<file>.gz` with Content-Encoding: gzip if that
//     sibling exists.
//   - Cache-Control: requests whose URL path begins with
//     cfg.ImmutablePrefix (default kit.DefaultStaticImmutablePrefix)
//     receive `public, max-age=<MaxAge seconds>, immutable`. All other
//     requests receive `public, max-age=0, must-revalidate`.
//   - Range requests are delegated to http.ServeContent for the chosen
//     file (precompressed or raw); compressed siblings are served as
//     opaque byte streams, matching nginx's gzip_static behavior.
//   - ETag (cfg.ETag = true) is computed from the served file's mtime
//     and size so 304 responses are cheap.
//
// The handler is a standalone http.Handler; integrate it by mounting
// under a path prefix in the user's router or via http.StripPrefix:
//
//	mux := http.NewServeMux()
//	mux.Handle("/static/", http.StripPrefix("/static",
//	    server.StaticHandler(kit.StaticConfig{Dir: "static", Gzip: true, Brotli: true}),
//	))
//
// StaticHandler does not mutate cfg.
func StaticHandler(cfg kit.StaticConfig) http.Handler {
	root := strings.TrimSpace(cfg.Dir)
	if root == "" {
		return errStaticHandler(errors.New("server: StaticConfig.Dir is empty"))
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return errStaticHandler(fmt.Errorf("server: resolve static dir: %w", err))
	}
	prefix := cfg.ImmutablePrefix
	if prefix == "" {
		prefix = kit.DefaultStaticImmutablePrefix
	}
	maxAge := cfg.MaxAge
	if maxAge <= 0 {
		maxAge = kit.DefaultStaticMaxAge
	}
	return &staticHandler{
		root:            abs,
		immutablePrefix: prefix,
		maxAgeSeconds:   int(maxAge / time.Second),
		etag:            cfg.ETag,
		brotli:          cfg.Brotli,
		gzip:            cfg.Gzip,
	}
}

type staticHandler struct {
	root            string
	immutablePrefix string
	maxAgeSeconds   int
	etag            bool
	brotli          bool
	gzip            bool
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rel, ok := safeRelPath(r.URL.Path)
	if !ok {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	rawPath := filepath.Join(h.root, filepath.FromSlash(rel))
	rawInfo, err := os.Stat(rawPath)
	if err != nil || rawInfo.IsDir() {
		http.NotFound(w, r)
		return
	}

	enc, encName, servePath, info := h.negotiate(r, rawPath, rawInfo)

	setCacheHeaders(w, r.URL.Path, h.immutablePrefix, h.maxAgeSeconds)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if encName != "" {
		w.Header().Set("Content-Encoding", encName)
		w.Header().Add("Vary", "Accept-Encoding")
	} else if h.brotli || h.gzip {
		w.Header().Add("Vary", "Accept-Encoding")
	}
	w.Header().Set("Content-Type", contentTypeFor(rawPath))
	if h.etag {
		w.Header().Set("ETag", buildETag(info, enc))
	}

	f, err := os.Open(servePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	http.ServeContent(w, r, filepath.Base(rawPath), info.ModTime(), f)
}

// negotiate picks the best precompressed sibling for the request and
// returns the chosen encoding token (used in the ETag), its HTTP name,
// the path to serve, and the FileInfo for that path.
func (h *staticHandler) negotiate(r *http.Request, rawPath string, rawInfo os.FileInfo) (string, string, string, os.FileInfo) {
	accept := r.Header.Get("Accept-Encoding")
	if h.brotli && acceptsEncoding(accept, "br") {
		if info, err := os.Stat(rawPath + ".br"); err == nil && !info.IsDir() {
			return "br", "br", rawPath + ".br", info
		}
	}
	if h.gzip && acceptsEncoding(accept, "gzip") {
		if info, err := os.Stat(rawPath + ".gz"); err == nil && !info.IsDir() {
			return "gzip", "gzip", rawPath + ".gz", info
		}
	}
	return "", "", rawPath, rawInfo
}

// safeRelPath returns urlPath as a forward-slash relative path under
// the static root. It rejects any input whose original segments contain
// "..", null bytes, or that would resolve to the root directory itself.
// Rejecting before path.Clean is deliberate: clean() silently collapses
// "/foo/../bar" to "/bar", which would let an attacker request files
// they should not be allowed to name even when the resolved target is
// safe. The handler treats traversal attempts as 400, not as silent
// rewrites.
func safeRelPath(urlPath string) (string, bool) {
	if urlPath == "" {
		return "", false
	}
	if strings.ContainsAny(urlPath, "\x00") {
		return "", false
	}
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == ".." {
			return "", false
		}
	}
	cleaned := path.Clean("/" + urlPath)
	if cleaned == "/" {
		return "", false
	}
	return strings.TrimPrefix(cleaned, "/"), true
}

// acceptsEncoding reports whether tok appears as a non-zero-quality
// token in an Accept-Encoding header value. Quality parsing is minimal:
// `;q=0` (with optional whitespace and decimals) suppresses the token.
// Any qualifier that is not a well-formed `q=<float>` is treated as
// acceptable — this is a deliberate short-circuit. RFC 9110 §12.5.4
// defines extension parameters (e.g. `;d=4`) that are not parsed here;
// the failure mode is conservative: the handler may serve an encoding the
// client did not prefer but can silently ignore. A full parser is future
// work not required for v0.1.
func acceptsEncoding(header, tok string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		seg := strings.TrimSpace(part)
		if seg == "" {
			continue
		}
		name := seg
		qual := ""
		if i := strings.IndexByte(seg, ';'); i >= 0 {
			name = strings.TrimSpace(seg[:i])
			qual = strings.TrimSpace(seg[i+1:])
		}
		if !strings.EqualFold(name, tok) && name != "*" {
			continue
		}
		if qual == "" {
			return true
		}
		if !strings.HasPrefix(qual, "q=") {
			return true
		}
		q, err := strconv.ParseFloat(strings.TrimPrefix(qual, "q="), 64)
		if err != nil {
			return true
		}
		return q > 0
	}
	return false
}

// setCacheHeaders writes Cache-Control for the given URL path. Paths
// under immutablePrefix (which must end in "/" for prefix semantics)
// are treated as fingerprinted and get the long TTL + immutable; all
// other paths get must-revalidate so updates are picked up promptly.
func setCacheHeaders(w http.ResponseWriter, urlPath, immutablePrefix string, maxAgeSeconds int) {
	if immutablePrefix != "" && strings.HasPrefix(urlPath, immutablePrefix) {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d, immutable", maxAgeSeconds))
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
}

// buildETag returns a strong ETag derived from the served file's mtime
// and size. The encoding token participates so the same logical asset
// served as raw vs gzip vs brotli has distinct validators — required so
// shared caches do not return a brotli body to a gzip-only client.
func buildETag(info os.FileInfo, enc string) string {
	tag := strconv.FormatInt(info.ModTime().UnixNano(), 16) + "-" + strconv.FormatInt(info.Size(), 16)
	if enc != "" {
		tag += "-" + enc
	}
	return `"` + tag + `"`
}

// contentTypeFor returns the MIME type the handler should set on the
// response. Net/http picks Content-Type from the file's leading bytes,
// which mis-identifies precompressed siblings; deriving from the raw
// extension keeps text/css and application/javascript stable across
// encoded responses.
//
// The explicit map overrides OS MIME databases for extensions that are
// mishandled on some platforms (e.g. .ico on Windows, .wasm and .woff2
// absent from many default mime.types). Falls back to
// mime.TypeByExtension, then application/octet-stream as last resort so
// X-Content-Type-Options: nosniff is always effective.
func contentTypeFor(rawPath string) string {
	ext := strings.ToLower(filepath.Ext(rawPath))
	switch ext {
	case ".js", ".mjs":
		return "application/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".xml":
		return "application/xml; charset=utf-8"
	case ".wasm":
		return "application/wasm"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ico":
		return "image/x-icon"
	case ".webmanifest":
		return "application/manifest+json"
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// errStaticHandler returns an http.Handler that serves a 500 with err's
// message on every request. Used to surface configuration errors at the
// first request instead of panicking at startup, so library callers can
// embed StaticHandler in larger pipelines without two-phase init.
func errStaticHandler(err error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	})
}
