package server

import "strings"

// EscapeHTML escapes v for HTML content position. Mirrors svelte/src/escaping.js
// escape_html(value, false): replaces & with &amp; and < with &lt;. Nil/missing
// values render as the empty string. Matches Svelte's CONTENT_REGEX exactly —
// > and ' are NOT escaped, by design.
func EscapeHTML(v any) string {
	return escapeAttrOrContent(Stringify(v), false)
}

// EscapeHTMLAttr escapes v for HTML attribute position. Mirrors
// escape_html(value, true): replaces &, ", and <. Used by Attr internally and
// by codegen wherever Svelte emits is_attr=true.
func EscapeHTMLAttr(v any) string {
	return escapeAttrOrContent(Stringify(v), true)
}

// EscapeHTMLString is the typed fast path for the common case where the
// caller already has a string. Skips the any-boxing in EscapeHTML's
// Stringify pre-pass.
func EscapeHTMLString(s string) string {
	return escapeAttrOrContent(s, false)
}

// EscapeHTMLAttrString is the typed fast path of EscapeHTMLAttr.
func EscapeHTMLAttrString(s string) string {
	return escapeAttrOrContent(s, true)
}

func escapeAttrOrContent(s string, isAttr bool) string {
	n := len(s)
	if n == 0 {
		return ""
	}

	first := -1
	for i := 0; i < n; i++ {
		c := s[i]
		if c == '&' || c == '<' || (isAttr && c == '"') {
			first = i
			break
		}
	}
	if first == -1 {
		return s
	}

	var b strings.Builder
	b.Grow(n + 16)
	b.WriteString(s[:first])
	for i := first; i < n; i++ {
		c := s[i]
		switch {
		case c == '&':
			b.WriteString("&amp;")
		case c == '<':
			b.WriteString("&lt;")
		case isAttr && c == '"':
			b.WriteString("&quot;")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
