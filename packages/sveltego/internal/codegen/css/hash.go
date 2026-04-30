// Package css implements the CSS scope-hash algorithm used by upstream
// Svelte so SSR-emitted class names match the client compiler byte-for-byte.
//
// The hash is a port of `hash` in
// https://github.com/sveltejs/svelte/blob/main/packages/svelte/src/utils.js
// (snapshot pinned 2026-04-30). The pin lives in this comment because
// Svelte does not expose the function as a stable export — replicate it,
// re-verify on upgrade.
package css

import (
	"strconv"
	"unicode/utf16"
)

// Hash returns the upstream-equivalent base36 hash of s. The implementation
// mirrors the JavaScript reference: strip carriage returns, walk the string
// as UTF-16 code units (matching JS `charCodeAt`), fold from end to start
// with `((h << 5) - h) ^ c`, and emit the unsigned 32-bit result in base36.
// JS evaluates the inner arithmetic on 32-bit signed integers and
// finalizes via `>>> 0`; running the fold in uint32 reproduces the same
// bit pattern because two's-complement wrap and modular arithmetic
// coincide on +/- and shifts. ASCII inputs run a byte-level fast path
// because UTF-16 and bytes coincide.
func Hash(s string) string {
	if s == "" {
		return strconv.FormatUint(5381, 36)
	}
	cleaned := stripCR(s)
	var h uint32 = 5381
	if isASCII(cleaned) {
		for i := len(cleaned) - 1; i >= 0; i-- {
			h = ((h << 5) - h) ^ uint32(cleaned[i])
		}
	} else {
		units := utf16.Encode([]rune(cleaned))
		for i := len(units) - 1; i >= 0; i-- {
			h = ((h << 5) - h) ^ uint32(units[i])
		}
	}
	return strconv.FormatUint(uint64(h), 36)
}

// ScopeClass returns the upstream default `svelte-<hash>` class. Filename
// is preferred when present — matching upstream's `cssHash` default
// `svelte-${hash(filename === '(unknown)' ? css : filename ?? css)}`.
func ScopeClass(filename, css string) string {
	src := filename
	if src == "" || src == "(unknown)" {
		src = css
	}
	return "svelte-" + Hash(src)
}

func stripCR(s string) string {
	if !containsCR(s) {
		return s
	}
	out := make([]byte, 0, len(s))
	for i := range len(s) {
		if s[i] != '\r' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func containsCR(s string) bool {
	for i := range len(s) {
		if s[i] == '\r' {
			return true
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
