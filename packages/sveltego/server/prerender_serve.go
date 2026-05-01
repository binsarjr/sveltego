package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

// prerenderProbeHeader marks an in-memory prerender request so user
// hooks (and the runtime fallback gate) can distinguish it from a real
// HTTP request. Always set when Server.Prerender drives the pipeline.
const prerenderProbeHeader = "X-Sveltego-Prerender"

// prerenderEntry stores one row of the runtime prerender manifest. The
// runtime keeps a path-keyed map so first-byte serving is O(1) per
// request.
type prerenderEntry struct {
	Path      string
	File      string
	Protected bool
}

// prerenderTable holds the resolved prerender manifest for the runtime.
// nil means no prerender was performed (or the manifest was absent at
// startup) and the pipeline always falls through to SSR.
type prerenderTable struct {
	root    string
	entries map[string]prerenderEntry
}

// LoadPrerenderManifest reads <projectRoot>/<dir>/manifest.json (default
// DefaultPrerenderOutDir) and returns the parsed table. A missing file
// returns (nil, nil) so callers can wire it unconditionally.
func LoadPrerenderManifest(projectRoot, dir string) (*prerenderTable, error) {
	if dir == "" {
		dir = DefaultPrerenderOutDir
	}
	root := filepath.Join(projectRoot, dir)
	manifestPath := filepath.Join(root, PrerenderManifestFilename)
	body, err := os.ReadFile(manifestPath) //nolint:gosec // path composed from caller-controlled root
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("server: read prerender manifest %s: %w", manifestPath, err)
	}
	var raw struct {
		Entries []PrerenderedEntry `json:"entries"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("server: parse prerender manifest %s: %w", manifestPath, err)
	}
	table := &prerenderTable{
		root:    root,
		entries: make(map[string]prerenderEntry, len(raw.Entries)),
	}
	for _, e := range raw.Entries {
		table.entries[e.Path] = prerenderEntry{Path: e.Path, File: e.File, Protected: e.Protected}
	}
	return table, nil
}

// servePrerendered tries to serve a static prerendered HTML file for r.
// Returns true when the response was written; false when the request
// should fall through to the SSR pipeline. Protected entries call the
// configured PrerenderAuthGate; a deny falls through.
func (s *Server) servePrerendered(w http.ResponseWriter, r *http.Request) bool {
	if s.prerender == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	entry, ok := s.prerender.entries[r.URL.Path]
	if !ok {
		return false
	}
	if entry.Protected {
		gate := s.prerenderAuth
		if gate == nil {
			gate = kit.DenyAllPrerenderAuth
		}
		if !gate.Allow(r) {
			return false
		}
	}
	full := filepath.Join(s.prerender.root, filepath.FromSlash(entry.File))
	body, err := os.ReadFile(full) //nolint:gosec // file path resolved against the trusted root
	if err != nil {
		s.Logger.Warn("server: prerender hit but file missing",
			logKeyError, err.Error(),
			logKeyPath, entry.File,
		)
		return false
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("X-Sveltego-Prerendered", "1")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return true
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	return true
}
