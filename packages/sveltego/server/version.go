package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
)

// versionEndpointPath is the URL the client poller fetches. Matches
// SvelteKit's default `${assets}/${__SVELTEKIT_APP_VERSION_FILE__}`
// shape so user code reading /_app/version.json behaves the same way
// across both frameworks.
const versionEndpointPath = "/_app/version.json"

// computeAppVersion returns a deterministic short digest of the Vite
// manifest JSON. The manifest changes any time a route chunk's hashed
// filename changes, which is exactly the signal a deploy is fresh, so
// hashing it gives a per-build identifier without requiring the user
// to thread a build hash through their config.
//
// Empty input yields an empty string; callers treat that as "no
// version known" and skip serving the endpoint. SHA-256 truncated to
// 16 hex chars (64 bits) is overkill for collision avoidance but
// keeps the wire payload small.
func computeAppVersion(manifest string) string {
	if manifest == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(manifest))
	return hex.EncodeToString(sum[:8])
}

// serveVersion writes the JSON {"version":"<hash>"} body the client
// poller compares against the build version baked into the hydration
// payload. Returns true iff the request was handled here so the
// caller can short-circuit the rest of the pipeline. Any non-GET on
// the same path receives 405; this matches the SvelteKit handler's
// read-only contract.
//
// When appVersion is empty (no ViteManifest supplied at construction)
// the endpoint reports 404. A client hydrated without a version then
// keeps `updated.current` at false even if it manages to fetch the
// path through some other route.
func (s *Server) serveVersion(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != versionEndpointPath {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	if s.appVersion == "" {
		http.NotFound(w, r)
		return true
	}
	body := `{"version":"` + s.appVersion + `"}`
	h := w.Header()
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Cache-Control", "no-store, no-cache, must-revalidate")
	h.Set("Pragma", "no-cache")
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return true
	}
	_, _ = w.Write([]byte(body))
	return true
}
