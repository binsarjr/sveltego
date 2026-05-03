// Package csrfinject splices a hidden `_csrf_token` input into rendered
// HTML for every `<form method="post">` open tag found in the input. It
// powers the runtime side of the CSRF auto-inject contract that the
// build-time pre-pass under internal/codegen/svelte_js2go/lower_csrf.go
// implements for transpiled Render() output: the sidecar fallback path
// (runtime/svelte/fallback) calls Rewrite on the HTML the Node sidecar
// returns so opted-in routes still get the hidden input even though
// they bypass the build-time AST splice.
//
// The scanner mirrors the rules documented on the build-time pass so
// the two paths produce equivalent markup:
//
//   - method="post" / 'post' / POST etc. — case-insensitive on POST.
//   - The `nocsrf` attribute opts a single form out and is stripped
//     from the rendered HTML so it does not leak into the user's DOM.
//   - A form already preceded by the hidden _csrf_token input is left
//     alone (idempotent re-runs).
//   - Self-closing `<form ... />`, malformed open tags, and forms
//     whose method attribute is dynamic (no static `method="post"`
//     token) are not modified — Svelte never emits those for
//     interactive forms; treating them as non-POST keeps the pass
//     narrow.
//
// The rewriter is HTML-aware enough to skip `>` characters inside
// quoted attribute values so an `action="?/x>y"` does not look like a
// premature tag close.
package csrfinject

import (
	"strings"
)

// Rewrite returns html with a hidden CSRF input spliced just inside
// every `<form method="post">` open tag, plus any `nocsrf` attributes
// stripped from the source bytes. token is the per-request CSRF value
// the splice writes into the input's value attribute; HTML-encoded
// before insertion so a token containing `&`, `<`, or `"` cannot break
// out of the attribute. When html contains no POST forms (and no
// nocsrf opt-outs) the original string is returned unchanged so
// callers may avoid an allocation in the hot path.
func Rewrite(html, token string) string {
	if html == "" {
		return html
	}
	if !containsFormStart(html) {
		return html
	}

	encodedToken := htmlAttrEscape(token)

	var (
		out     strings.Builder
		mutated bool
	)
	out.Grow(len(html) + 80)

	pos := 0
	for {
		idx := indexFormStart(html, pos)
		if idx < 0 {
			break
		}
		end, rest, attrs, ok := scanFormOpenTag(html, idx)
		if !ok {
			out.WriteString(html[pos : idx+1])
			pos = idx + 1
			continue
		}
		out.WriteString(html[pos:idx])

		if !attrs.isPost {
			out.WriteString(html[idx:end])
			pos = end
			continue
		}
		if attrs.nocsrf {
			// Strip the marker attribute and skip the splice.
			out.WriteString(rest)
			pos = end
			mutated = true
			continue
		}
		if attrs.alreadyInjected {
			out.WriteString(html[idx:end])
			pos = end
			continue
		}

		out.WriteString(rest)
		out.WriteString(`<input type="hidden" name="_csrf_token" value="`)
		out.WriteString(encodedToken)
		out.WriteString(`">`)
		pos = end
		mutated = true
	}

	if !mutated {
		return html
	}
	out.WriteString(html[pos:])
	return out.String()
}

// formAttrs captures the subset of form-open-tag information the
// rewriter needs to decide whether to inject. isPost is true when a
// `method` attribute resolves (case-insensitive) to "post". nocsrf is
// true when the open tag carries a `nocsrf` attribute (stripped from
// the rewritten output). alreadyInjected is true when the byte stream
// immediately following the close `>` already begins with a hidden
// input named `_csrf_token`, signalling a previous pass already ran.
type formAttrs struct {
	isPost          bool
	nocsrf          bool
	alreadyInjected bool
}

// containsFormStart cheaply rejects strings with no `<form` token.
// Most real-world HTML chunks carry no form tag at all; the bail-out
// keeps Rewrite a no-op in the common case.
func containsFormStart(s string) bool {
	for i := 0; i+5 <= len(s); i++ {
		if s[i] != '<' {
			continue
		}
		if asciiToLowerByte(s[i+1]) == 'f' &&
			asciiToLowerByte(s[i+2]) == 'o' &&
			asciiToLowerByte(s[i+3]) == 'r' &&
			asciiToLowerByte(s[i+4]) == 'm' {
			return true
		}
	}
	return false
}

// indexFormStart returns the byte index of the next `<form` token in s
// at or after start, or -1 when none. The match is case-insensitive on
// the tag name and requires the byte after `<form` to be either `>`,
// `/`, or whitespace (so `<form>` and `<formal>` don't collide).
func indexFormStart(s string, start int) int {
	for i := start; i+5 <= len(s); i++ {
		if s[i] != '<' {
			continue
		}
		if asciiToLowerByte(s[i+1]) != 'f' ||
			asciiToLowerByte(s[i+2]) != 'o' ||
			asciiToLowerByte(s[i+3]) != 'r' ||
			asciiToLowerByte(s[i+4]) != 'm' {
			continue
		}
		if i+5 == len(s) {
			return i
		}
		next := s[i+5]
		if next == '>' || next == '/' || next == ' ' || next == '\t' || next == '\n' || next == '\r' {
			return i
		}
	}
	return -1
}

// scanFormOpenTag walks a `<form ...>` open tag starting at idx and
// returns the position one past the closing `>`, the open tag with
// any `nocsrf` attribute stripped, the parsed flags, and ok=false
// when the open tag is malformed or self-closing.
//
// Tracks single- and double-quoted attribute values so a quoted `>`
// inside `action="?/x>y"` is not mistaken for the tag close.
func scanFormOpenTag(s string, idx int) (end int, rest string, attrs formAttrs, ok bool) {
	pos := idx + len("<form")
	if pos > len(s) {
		return 0, "", formAttrs{}, false
	}

	var (
		closeAt    = -1
		selfClose  bool
		stripStart = -1
		stripEnd   = -1
	)
	type attrSpan struct {
		nameStart, nameEnd int
		valStart, valEnd   int
		hasValue           bool
		quote              byte
	}
	var spans []attrSpan
	i := pos
	for i < len(s) {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '/' {
			if i+1 < len(s) && s[i+1] == '>' {
				selfClose = true
				closeAt = i + 1
				break
			}
			i++
			continue
		}
		if c == '>' {
			closeAt = i
			break
		}
		nameStart := i
		for i < len(s) {
			b := s[i]
			if b == '=' || b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '>' || b == '/' {
				break
			}
			i++
		}
		nameEnd := i
		span := attrSpan{nameStart: nameStart, nameEnd: nameEnd}
		j := i
		for j < len(s) {
			b := s[j]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
				j++
				continue
			}
			break
		}
		if j < len(s) && s[j] == '=' {
			span.hasValue = true
			j++
			for j < len(s) {
				b := s[j]
				if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
					j++
					continue
				}
				break
			}
			if j >= len(s) {
				break
			}
			if s[j] == '"' || s[j] == '\'' {
				span.quote = s[j]
				j++
				span.valStart = j
				for j < len(s) && s[j] != span.quote {
					j++
				}
				if j >= len(s) {
					return 0, "", formAttrs{}, false
				}
				span.valEnd = j
				j++
			} else {
				span.valStart = j
				for j < len(s) {
					b := s[j]
					if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '>' || b == '/' {
						break
					}
					j++
				}
				span.valEnd = j
			}
			i = j
		}
		spans = append(spans, span)
	}
	if closeAt < 0 {
		return 0, "", formAttrs{}, false
	}
	if selfClose {
		return closeAt + 1, s[idx : closeAt+1], formAttrs{}, true
	}
	end = closeAt + 1

	for _, sp := range spans {
		name := strings.ToLower(s[sp.nameStart:sp.nameEnd])
		switch name {
		case "method":
			if sp.hasValue {
				val := strings.TrimSpace(s[sp.valStart:sp.valEnd])
				if strings.EqualFold(val, "post") {
					attrs.isPost = true
				}
			}
		case "nocsrf":
			attrs.nocsrf = true
			delStart := sp.nameStart
			for delStart > pos && isASCIISpace(s[delStart-1]) {
				delStart--
			}
			delEnd := sp.nameEnd
			if sp.hasValue {
				if sp.quote != 0 {
					delEnd = sp.valEnd + 1
				} else {
					delEnd = sp.valEnd
				}
			}
			if stripStart == -1 {
				stripStart = delStart
				stripEnd = delEnd
			} else {
				if delStart < stripStart {
					stripStart = delStart
				}
				if delEnd > stripEnd {
					stripEnd = delEnd
				}
			}
		}
	}

	if stripStart >= 0 {
		rest = s[idx:stripStart] + s[stripEnd:end]
	} else {
		rest = s[idx:end]
	}

	// alreadyInjected: idempotency guard. We accept the hidden input
	// even when its attribute order or quoting differs from what the
	// build-time pass emits, because forms can land here that were
	// already rewritten by codegen and we don't want to re-inject.
	tail := s[end:]
	if hasInjectedHiddenCSRF(tail) {
		attrs.alreadyInjected = true
	}

	return end, rest, attrs, true
}

// hasInjectedHiddenCSRF reports whether tail begins with a hidden input
// whose name attribute is `_csrf_token` (in any quoting / attribute
// order). The check tolerates whitespace and out-of-order attributes
// because the splice may originate from either the build-time AST pass,
// the runtime rewriter, or user-authored markup that already carries
// the field.
func hasInjectedHiddenCSRF(tail string) bool {
	// Skip leading whitespace between the form open tag and a possible
	// pre-existing hidden input (Svelte server output sometimes emits
	// a blank line for readability in dev builds).
	i := 0
	for i < len(tail) && isASCIISpace(tail[i]) {
		i++
	}
	if i+6 > len(tail) {
		return false
	}
	if tail[i] != '<' {
		return false
	}
	// Match `<input` (case-insensitive).
	if asciiToLowerByte(tail[i+1]) != 'i' ||
		asciiToLowerByte(tail[i+2]) != 'n' ||
		asciiToLowerByte(tail[i+3]) != 'p' ||
		asciiToLowerByte(tail[i+4]) != 'u' ||
		asciiToLowerByte(tail[i+5]) != 't' {
		return false
	}
	// Find the closing `>`; bail if we don't see one in the next 256
	// bytes — runaway open tags shouldn't be treated as injected.
	limit := i + 6 + 256
	if limit > len(tail) {
		limit = len(tail)
	}
	end := strings.IndexByte(tail[i+6:limit], '>')
	if end < 0 {
		return false
	}
	openTag := strings.ToLower(tail[i : i+6+end])
	return strings.Contains(openTag, `name="_csrf_token"`) ||
		strings.Contains(openTag, `name='_csrf_token'`) ||
		strings.Contains(openTag, `name=_csrf_token`)
}

// htmlAttrEscape encodes the bare minimum required to safely embed s
// inside a double-quoted HTML attribute value. The token is generated
// server-side via crypto/rand + base64url so realistic inputs only
// contain url-safe bytes; the encode step is defence-in-depth so a
// hypothetical token mutation cannot break out of the attribute.
func htmlAttrEscape(s string) string {
	if !strings.ContainsAny(s, `&<>"'`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func asciiToLowerByte(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
