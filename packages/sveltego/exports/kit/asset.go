package kit

import (
	"strings"
	"sync"
)

// DefaultAssetsImmutablePrefix is the URL path prefix under which hashed
// static assets are served. Anything under this prefix receives the
// immutable Cache-Control treatment from server.StaticHandler.
const DefaultAssetsImmutablePrefix = "/_app/immutable/assets/"

var (
	assetsMu  sync.RWMutex
	assetsMap map[string]string
)

// RegisterAssets installs the source-path -> hashed-URL lookup table that
// [Asset] consults at request time. Codegen calls this from generated
// init() during program startup; user code rarely needs to invoke it
// directly, except in tests that exercise [Asset] in isolation.
//
// A nil m clears the registration so subsequent [Asset] calls fall back
// to the input string. The map is shallow-copied so callers may mutate
// their copy after registration without racing the runtime lookup.
func RegisterAssets(m map[string]string) {
	assetsMu.Lock()
	defer assetsMu.Unlock()
	if m == nil {
		assetsMap = nil
		return
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	assetsMap = cp
}

// Asset returns the hashed, cache-bustable URL for the static asset
// named by path (e.g. "logo.png" -> "/_app/immutable/assets/logo.abc12345.png").
//
// Lookups normalize a leading slash so callers may write either
// kit.Asset("logo.png") or kit.Asset("/logo.png").
//
// When the assets manifest has not been registered (dev mode, tests, or
// a build that skipped client emission), Asset returns path unchanged
// with a leading slash so the result is still a valid URL. Codegen is
// responsible for promoting unknown-asset references to a build error;
// the runtime is intentionally permissive so tests do not need to wire
// the full manifest.
func Asset(path string) string {
	clean := strings.TrimPrefix(path, "/")
	if clean == "" {
		return "/"
	}
	assetsMu.RLock()
	hashed, ok := assetsMap[clean]
	assetsMu.RUnlock()
	if ok {
		return hashed
	}
	return "/" + clean
}
