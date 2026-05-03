// Package csrffallback rewrites the Svelte source of an ssr-fallback
// route's `_page.svelte` so the hidden `_csrf_token` input lives in the
// component's own vDOM. Closes #540: the runtime sidecar's post-hoc
// `csrfinject.Rewrite` produces correct SSR HTML, but Svelte 5
// `hydrate()` walks DOM-vs-vDOM and strips the unmatched DOM input
// because the user's source has no input element. Lowering at the
// source level puts the input in vDOM so hydration matches.
//
// The lowered file is paired with the unmodified user source: the
// sidecar continues reading the user's `_page.svelte` (post-hoc rewrite
// adds the real token to the SSR HTML), but the per-route client
// `entry.ts` imports the lowered file. The client's vDOM therefore
// contains the input element with a value mustache that resolves
// against `globalThis.__sveltego__?.csrfToken` — the same token the
// hydration payload carries — so Svelte 5 reconciles the live DOM
// input value with the SSR-rendered token without stripping the node.
//
// The scanner mirrors the rules already documented on
// `runtime/svelte/csrfinject` and
// `internal/codegen/svelte_js2go/lower_csrf.go`:
//
//   - method="post" / 'post' / POST etc. — case-insensitive.
//   - The `nocsrf` attribute opts a single form out and is stripped.
//   - A form that already carries a hidden `_csrf_token` input
//     immediately after its open tag is left alone (idempotent).
//   - Self-closing forms and forms with a dynamic method are skipped.
//   - `<script>` and `<style>` blocks are skipped wholesale so a `<form`
//     string literal in JS or CSS is not mistaken for HTML.
package csrffallback

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CSRFValueExpr is the Svelte mustache expression spliced into the
// hidden input's `value=` attribute. It resolves at hydration time
// against the `window.__sveltego__` object that `cliententry.ts`
// seeds from the JSON payload before calling `hydrate()`. SSR
// rendering through the sidecar evaluates the expression to the
// empty string (the Node sandbox has no `__sveltego__` global), but
// the post-hoc `csrfinject.Rewrite` on the sidecar HTML still emits
// a complete input with the real token in the response bytes — the
// vDOM input only has to exist for hydration to match the DOM.
const CSRFValueExpr = `globalThis.__sveltego__?.csrfToken ?? ''`

// LowerOptions controls a single Lower invocation.
//
// Source is the absolute path of the user's `_page.svelte` on disk.
// SourceContent holds its bytes. Returning the path lets the rewriter
// emit absolute import specifiers when it has to relocate a relative
// import out of the user's directory.
type LowerOptions struct {
	Source        string
	SourceContent []byte
}

// LowerResult bundles the rewritten Svelte source and a mutated flag.
//
// Mutated is false when the input contained no POST forms (and no
// nocsrf opt-outs); callers can fall back to the original source in
// that case to avoid emitting an unnecessary lowered file.
type LowerResult struct {
	Content []byte
	Mutated bool
}

// Lower rewrites src so every `<form method="post">` open tag is
// followed by a hidden `_csrf_token` input whose value mustache
// resolves to `CSRFValueExpr`. Forms carrying a `nocsrf` attribute are
// left alone (and the attribute is stripped). Forms already followed
// by a hidden `_csrf_token` input are left alone. Relative imports in
// the source's `<script>` block are rewritten to absolute file paths
// anchored at the source's directory so the lowered file can live
// outside the user's route directory without breaking module
// resolution. Returns the original bytes (Mutated=false) when no form
// or import required a rewrite.
func Lower(opts LowerOptions) (LowerResult, error) {
	if len(opts.SourceContent) == 0 {
		return LowerResult{Content: opts.SourceContent}, nil
	}
	src := string(opts.SourceContent)

	formsRewrote, body, err := spliceForms(src)
	if err != nil {
		return LowerResult{}, err
	}

	importsRewrote := false
	if opts.Source != "" {
		body, importsRewrote = rewriteRelativeImports(body, filepath.Dir(opts.Source))
	}

	if !formsRewrote && !importsRewrote {
		return LowerResult{Content: opts.SourceContent}, nil
	}
	return LowerResult{Content: []byte(body), Mutated: formsRewrote}, nil
}

// spliceForms walks the Svelte source skipping `<script>` and `<style>`
// blocks and rewrites POST form open tags. Returns whether any form was
// rewritten and the resulting source.
func spliceForms(src string) (bool, string, error) {
	var out strings.Builder
	out.Grow(len(src) + 128)

	mutated := false
	pos := 0
	for pos < len(src) {
		nextSkip, skipEnd, err := nextSkipBlock(src, pos)
		if err != nil {
			return false, "", err
		}
		end := nextSkip
		if end < 0 {
			end = len(src)
		}
		// Process the [pos, end) HTML range looking for form opens.
		mutatedHere, processed := processFormRange(src[pos:end])
		if mutatedHere {
			mutated = true
		}
		out.WriteString(processed)
		if nextSkip < 0 {
			break
		}
		out.WriteString(src[nextSkip:skipEnd])
		pos = skipEnd
	}
	return mutated, out.String(), nil
}

// nextSkipBlock locates the next `<script>` or `<style>` block start at
// or after pos. Returns its start index, the index one past the
// closing `</script>` / `</style>` tag, and an error when the open tag
// has no matching close. Returns (-1, -1, nil) when no skip block
// remains in src[pos:].
func nextSkipBlock(src string, pos int) (int, int, error) {
	for i := pos; i < len(src); {
		if src[i] != '<' {
			i++
			continue
		}
		rest := src[i:]
		switch {
		case caseInsensitivePrefix(rest, "<script"):
			closeOpen := strings.IndexByte(src[i:], '>')
			if closeOpen < 0 {
				return -1, -1, fmt.Errorf("csrffallback: unterminated <script open at offset %d", i)
			}
			endOpen := i + closeOpen + 1
			closeIdx := indexCloseTag(src, endOpen, "script")
			if closeIdx < 0 {
				return -1, -1, fmt.Errorf("csrffallback: missing </script> after offset %d", i)
			}
			return i, closeIdx, nil
		case caseInsensitivePrefix(rest, "<style"):
			closeOpen := strings.IndexByte(src[i:], '>')
			if closeOpen < 0 {
				return -1, -1, fmt.Errorf("csrffallback: unterminated <style open at offset %d", i)
			}
			endOpen := i + closeOpen + 1
			closeIdx := indexCloseTag(src, endOpen, "style")
			if closeIdx < 0 {
				return -1, -1, fmt.Errorf("csrffallback: missing </style> after offset %d", i)
			}
			return i, closeIdx, nil
		default:
			i++
		}
	}
	return -1, -1, nil
}

// caseInsensitivePrefix reports whether s starts with prefix
// (case-insensitive) AND the byte at len(prefix) is either absent or a
// non-name continuation (whitespace, `>`, or `/`). The continuation
// guard prevents `<scripted>` from matching `<script`.
func caseInsensitivePrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if asciiToLower(s[i]) != asciiToLower(prefix[i]) {
			return false
		}
	}
	if len(s) == len(prefix) {
		return true
	}
	c := s[len(prefix)]
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/'
}

// indexCloseTag returns the byte index one past `</name>` (case
// insensitive) at or after start, or -1 when no close tag exists.
func indexCloseTag(s string, start int, name string) int {
	needle := "</"
	for i := start; i < len(s); {
		j := strings.Index(s[i:], needle)
		if j < 0 {
			return -1
		}
		k := i + j + len(needle)
		if k+len(name) > len(s) {
			return -1
		}
		match := true
		for n := 0; n < len(name); n++ {
			if asciiToLower(s[k+n]) != name[n] {
				match = false
				break
			}
		}
		if !match {
			i = k
			continue
		}
		// Allow whitespace then `>`; reject if next char continues the name
		// (so `</scripted>` does not match `</script`).
		m := k + len(name)
		for m < len(s) && (s[m] == ' ' || s[m] == '\t' || s[m] == '\n' || s[m] == '\r') {
			m++
		}
		if m >= len(s) || s[m] != '>' {
			i = k
			continue
		}
		return m + 1
	}
	return -1
}

// processFormRange runs the POST-form splicer over a contiguous HTML
// range that contains no `<script>` or `<style>` blocks. Returns
// whether any form was rewritten.
func processFormRange(html string) (bool, string) {
	if !containsFormStart(html) {
		return false, html
	}
	var out strings.Builder
	out.Grow(len(html) + 64)

	mutated := false
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
		switch {
		case !attrs.isPost:
			out.WriteString(html[idx:end])
		case attrs.nocsrf:
			out.WriteString(rest)
			mutated = true
		case attrs.alreadyInjected:
			out.WriteString(html[idx:end])
		default:
			out.WriteString(rest)
			out.WriteString(`<input type="hidden" name="_csrf_token" value={`)
			out.WriteString(CSRFValueExpr)
			out.WriteString(`}>`)
			mutated = true
		}
		pos = end
	}
	out.WriteString(html[pos:])
	return mutated, out.String()
}

type formAttrs struct {
	isPost          bool
	nocsrf          bool
	alreadyInjected bool
}

// containsFormStart cheaply rejects strings without a `<form` token.
func containsFormStart(s string) bool {
	for i := 0; i+5 <= len(s); i++ {
		if s[i] != '<' {
			continue
		}
		if asciiToLower(s[i+1]) == 'f' &&
			asciiToLower(s[i+2]) == 'o' &&
			asciiToLower(s[i+3]) == 'r' &&
			asciiToLower(s[i+4]) == 'm' {
			return true
		}
	}
	return false
}

// indexFormStart returns the byte index of the next `<form` token at or
// after start; -1 when none. Match is case-insensitive on the tag name
// and requires the byte after `<form` to be `>`, `/`, or whitespace so
// `<form>` does not collide with `<formal>`.
func indexFormStart(s string, start int) int {
	for i := start; i+5 <= len(s); i++ {
		if s[i] != '<' {
			continue
		}
		if asciiToLower(s[i+1]) != 'f' ||
			asciiToLower(s[i+2]) != 'o' ||
			asciiToLower(s[i+3]) != 'r' ||
			asciiToLower(s[i+4]) != 'm' {
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
// returns the position one past the closing `>`, the open-tag bytes
// with any `nocsrf` attribute stripped, the parsed flags, and ok=false
// when the tag is malformed or self-closing. Tracks single- and
// double-quoted attribute values so a quoted `>` is not mistaken for
// the tag close, and tolerates Svelte mustache attribute values such
// as `action={`?/login`}` by tracking matching `{` / `}` pairs.
func scanFormOpenTag(s string, idx int) (int, string, formAttrs, bool) {
	pos := idx + len("<form")
	if pos > len(s) {
		return 0, "", formAttrs{}, false
	}
	type attrSpan struct {
		nameStart, nameEnd int
		valStart, valEnd   int
		hasValue           bool
		quote              byte
	}
	var (
		spans      []attrSpan
		closeAt    = -1
		selfClose  bool
		stripStart = -1
		stripEnd   = -1
	)
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
		for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
			j++
		}
		if j < len(s) && s[j] == '=' {
			span.hasValue = true
			j++
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j >= len(s) {
				break
			}
			switch s[j] {
			case '"', '\'':
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
			case '{':
				depth := 1
				j++
				span.valStart = j
				for j < len(s) && depth > 0 {
					switch s[j] {
					case '{':
						depth++
					case '}':
						depth--
					}
					if depth == 0 {
						break
					}
					j++
				}
				if depth != 0 {
					return 0, "", formAttrs{}, false
				}
				span.valEnd = j
				j++
			default:
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
	end := closeAt + 1

	var attrs formAttrs
	for _, sp := range spans {
		name := strings.ToLower(s[sp.nameStart:sp.nameEnd])
		switch name {
		case "method":
			if sp.hasValue && sp.quote != 0 {
				val := strings.TrimSpace(s[sp.valStart:sp.valEnd])
				if strings.EqualFold(val, "post") {
					attrs.isPost = true
				}
			}
		case "nocsrf":
			attrs.nocsrf = true
			delStart := sp.nameStart
			for delStart > pos && (s[delStart-1] == ' ' || s[delStart-1] == '\t' || s[delStart-1] == '\n' || s[delStart-1] == '\r') {
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
				stripStart, stripEnd = delStart, delEnd
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

	rest := s[idx:end]
	if stripStart >= 0 {
		rest = s[idx:stripStart] + s[stripEnd:end]
	}
	if hasInjectedHiddenCSRF(s[end:]) {
		attrs.alreadyInjected = true
	}
	return end, rest, attrs, true
}

// hasInjectedHiddenCSRF reports whether tail begins (after any
// whitespace) with a hidden input named `_csrf_token`. Tolerates
// attribute order and quoting so the splice is idempotent.
func hasInjectedHiddenCSRF(tail string) bool {
	i := 0
	for i < len(tail) && (tail[i] == ' ' || tail[i] == '\t' || tail[i] == '\n' || tail[i] == '\r') {
		i++
	}
	if i+6 > len(tail) || tail[i] != '<' {
		return false
	}
	if asciiToLower(tail[i+1]) != 'i' ||
		asciiToLower(tail[i+2]) != 'n' ||
		asciiToLower(tail[i+3]) != 'p' ||
		asciiToLower(tail[i+4]) != 'u' ||
		asciiToLower(tail[i+5]) != 't' {
		return false
	}
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

func asciiToLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// relativeImportRe matches `from "./..."` or `from "../..."` (and the
// dynamic `import("./...")` form). Captures the quote and specifier.
var relativeImportRe = regexp.MustCompile(`(\bfrom\s*|\bimport\s*\(\s*)(['"])(\.{1,2}\/[^'"]*)(['"])`)

// rewriteRelativeImports walks src once and substitutes every relative
// import specifier with an absolute path anchored at sourceDir. Vite
// resolves absolute fs paths natively so the lowered file can live
// outside the user's route directory without the relative imports
// breaking. Bare specifiers (`svelte`, `$lib/foo`) are left alone.
// Returns the rewritten source and whether any specifier was changed.
func rewriteRelativeImports(src, sourceDir string) (string, bool) {
	if sourceDir == "" {
		return src, false
	}
	mutated := false
	out := relativeImportRe.ReplaceAllStringFunc(src, func(match string) string {
		groups := relativeImportRe.FindStringSubmatch(match)
		if len(groups) != 5 {
			return match
		}
		absolute := filepath.ToSlash(filepath.Join(sourceDir, groups[3]))
		mutated = true
		return groups[1] + groups[2] + absolute + groups[4]
	})
	return out, mutated
}
