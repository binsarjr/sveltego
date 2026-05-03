package server

import (
	"encoding/json"
	"fmt"
	"strings"
)

// viteChunk mirrors one entry in the Vite manifest JSON. Only the fields
// the server needs for tag injection are decoded; additional fields are
// ignored.
type viteChunk struct {
	File    string   `json:"file"`
	Imports []string `json:"imports"`
	CSS     []string `json:"css"`
	IsEntry bool     `json:"isEntry"`
}

// viteManifestMap is the parsed Vite manifest: logical input key → chunk.
type viteManifestMap map[string]viteChunk

// parseViteManifest decodes the JSON manifest produced by `vite build`.
func parseViteManifest(data string) (viteManifestMap, error) {
	var m viteManifestMap
	if err := json.Unmarshal([]byte(data), &m); err != nil {
		return nil, fmt.Errorf("server: parse vite manifest: %w", err)
	}
	return m, nil
}

// globalCSSFile finds a manifest entry whose source is the conventional
// `src/app.css` and whose output is a CSS asset, returning its hashed
// file path or "". Tailwind (and any addon that pipes through src/app.css)
// produces such an entry; pages need its tag in <head> regardless of
// route, since the imports are global.
func (m viteManifestMap) globalCSSFile() string {
	if m == nil {
		return ""
	}
	c, ok := m["src/app.css"]
	if !ok {
		return ""
	}
	if !strings.HasSuffix(c.File, ".css") {
		return ""
	}
	return c.File
}

// headAssetTags returns the head-belonging HTML fragment for routeKey:
// stylesheet links plus <link rel="modulepreload"> hints for every
// transitive import. Returns an empty string when routeKey is not in the
// manifest or when m is nil. When the manifest has a `src/app.css` entry
// (Tailwind / PostCSS / global stylesheet path), its hashed file is
// prepended as a <link rel="stylesheet"> so the global stylesheet loads
// alongside route-scoped CSS.
//
// The entry <script type="module"> is intentionally NOT in this fragment
// — see bodyEntryTag. End-of-body script placement matches SvelteKit's
// %sveltekit.body% convention so the browser paints SSR HTML before any
// JS chunk executes (better LCP, fewer hydration-timing races). The
// modulepreload hints stay in <head> so the browser still discovers and
// parallel-fetches the chunks during HTML parse.
func (m viteManifestMap) headAssetTags(routeKey, base string) string {
	if m == nil {
		return ""
	}
	if _, ok := m[routeKey]; !ok {
		return ""
	}
	chunk := m[routeKey]
	base = strings.TrimRight(base, "/")

	var b strings.Builder

	if globalCSS := m.globalCSSFile(); globalCSS != "" {
		fmt.Fprintf(&b, `<link rel="stylesheet" href="%s/%s">`, base, globalCSS)
		b.WriteByte('\n')
	}

	seen := make(map[string]struct{})
	var collectImports func(key string)
	collectImports = func(key string) {
		c, ok := m[key]
		if !ok {
			return
		}
		for _, imp := range c.Imports {
			if _, done := seen[imp]; done {
				continue
			}
			seen[imp] = struct{}{}
			ic, ok := m[imp]
			if !ok {
				continue
			}
			fmt.Fprintf(&b, `<link rel="modulepreload" href="%s/%s">`, base, ic.File)
			b.WriteByte('\n')
			collectImports(imp)
		}
	}
	collectImports(routeKey)

	for _, css := range chunk.CSS {
		fmt.Fprintf(&b, `<link rel="stylesheet" href="%s/%s">`, base, css)
		b.WriteByte('\n')
	}

	return b.String()
}

// bodyEntryTag returns the per-route entry <script type="module"> tag for
// routeKey, suitable for emission at the end of <body> just before the
// shell tail. Returns an empty string when routeKey is not in the
// manifest or when m is nil.
//
// Pairs with headAssetTags: the head fragment carries stylesheet + module
// preload hints (parsed during the HEAD walk so the browser starts the
// chunk fetches in parallel with HTML parsing); the body fragment carries
// the entry script that consumes those preloaded chunks once the SSR
// body has fully parsed.
func (m viteManifestMap) bodyEntryTag(routeKey, base string) string {
	if m == nil {
		return ""
	}
	chunk, ok := m[routeKey]
	if !ok {
		return ""
	}
	base = strings.TrimRight(base, "/")

	var b strings.Builder
	fmt.Fprintf(&b, `<script type="module" src="%s/%s"></script>`, base, chunk.File)
	b.WriteByte('\n')
	return b.String()
}
