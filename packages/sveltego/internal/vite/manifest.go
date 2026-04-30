// Package vite provides helpers to parse the Vite manifest JSON and emit the
// corresponding <script type="module"> and <link rel="modulepreload"> HTML
// tags for a given route's client-entry chunk.
package vite

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Chunk is a single entry in the Vite manifest. Only the fields sveltego
// needs are parsed; Vite may include additional keys that are ignored.
type Chunk struct {
	File    string   `json:"file"`
	Imports []string `json:"imports"`
	CSS     []string `json:"css"`
	IsEntry bool     `json:"isEntry"`
}

// Manifest maps logical chunk names (as they appear in vite.config.js
// `input` keys, e.g. "routes/+page") to their hashed output chunks.
type Manifest map[string]Chunk

// Parse decodes the JSON manifest produced by `vite build`.
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("vite: parse manifest: %w", err)
	}
	return m, nil
}

// Tags returns the HTML fragment that must be injected into <head> for the
// given routeKey (e.g. "routes/+page"). It emits:
//   - one <link rel="modulepreload"> per transitive import chunk
//   - one <link rel="stylesheet"> per CSS file
//   - one <script type="module"> for the entry chunk itself
//
// Returns an empty string when routeKey is not found in the manifest (e.g.
// when the client build was skipped with --no-client).
func (m Manifest) Tags(routeKey, base string) string {
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
