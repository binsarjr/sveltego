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

// assetTags returns the HTML fragment (script + modulepreload + stylesheet
// tags) for routeKey. Returns an empty string when routeKey is not in the
// manifest or when m is nil.
func (m viteManifestMap) assetTags(routeKey, base string) string {
	if m == nil {
		return ""
	}
	chunk, ok := m[routeKey]
	if !ok {
		return ""
	}
	base = strings.TrimRight(base, "/")

	var b strings.Builder

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

	fmt.Fprintf(&b, `<script type="module" src="%s/%s"></script>`, base, chunk.File)
	b.WriteByte('\n')

	return b.String()
}
