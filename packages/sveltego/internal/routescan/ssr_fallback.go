package routescan

import (
	"bytes"
	"fmt"
	"os"
)

// ssrFallbackMarker is the HTML comment a route's _page.svelte uses to
// opt out of the build-time JS→Go SSR transpiler and route through the
// long-running Node sidecar at request time. Detection is byte-level,
// not Svelte-AST-level, so the comment lands in the same scan pass that
// reads the file's existence; the transpile pipeline then skips
// annotated routes entirely (ADR 0009 sub-decision 2 / Phase 8 #430).
//
// Whitespace inside the comment is permissive: any combination of
// spaces or tabs around the marker text is accepted, but the comment
// must use the literal `<!-- ... -->` form so it survives Vite's
// downstream tooling.
const ssrFallbackMarker = "sveltego:ssr-fallback"

// detectSSRFallbackAnnotation reads source and reports whether it
// contains an HTML comment matching `<!-- sveltego:ssr-fallback -->`.
// A read error is surfaced verbatim — callers turn it into a scan
// diagnostic so the user sees the path that failed.
//
// The scanner reads each `_page.svelte` once during route discovery;
// scaling cost is one OS open + slurp per route, identical in shape to
// the existing route walk's filesystem touches.
func detectSSRFallbackAnnotation(path string) (bool, error) {
	src, err := os.ReadFile(path) //nolint:gosec // path is from filepath.WalkDir over user routes dir
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return hasSSRFallbackMarker(src), nil
}

// hasSSRFallbackMarker scans src for any HTML comment whose body, after
// trimming spaces and tabs, equals ssrFallbackMarker. Returns false
// when no such comment exists, including the case where the marker
// text appears outside an HTML comment (e.g. inside a string literal
// in <script>).
func hasSSRFallbackMarker(src []byte) bool {
	openTag := []byte("<!--")
	closeTag := []byte("-->")
	for {
		i := bytes.Index(src, openTag)
		if i < 0 {
			return false
		}
		rest := src[i+len(openTag):]
		j := bytes.Index(rest, closeTag)
		if j < 0 {
			return false
		}
		body := bytes.TrimFunc(rest[:j], func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r'
		})
		if bytes.Equal(body, []byte(ssrFallbackMarker)) {
			return true
		}
		src = rest[j+len(closeTag):]
	}
}
