package sveltejs2go

import (
	"strings"
)

// CSRF auto-inject (issue #493). The pre-pass walks every TemplateLiteral
// in the AST looking for HTML `<form ... method="post" ... >` open tags
// in the static (quasi) text. When it finds one, it splices a hidden
// CSRF input — `<input type="hidden" name="_csrf_token" value="${pageState.CSRFToken}">` —
// in immediately after the form's `>`. The splice rewrites the affected
// quasi into two halves and inserts a synthetic
// `MemberExpression(pageState, CSRFToken)` between them so the existing
// emitter renders the new pieces through its normal template-literal
// path.
//
// Per-form opt-out: a `nocsrf` attribute on the form skips injection
// AND is stripped from the rendered HTML so it does not leak into the
// user's DOM. Mirrors the legacy Mustache-Go behaviour deleted in #486.
//
// Per-route opt-in is the caller's responsibility — runSSRTranspile
// honours the route's `kit.PageOptions.CSRF` flag and only enables this
// pass when CSRF is true. The pass itself is unconditional once enabled:
// every POST form in the AST gets a hidden input.
//
// Limitations:
//
//   - Form open tags whose `<form` and matching `>` straddle a template
//     interpolation (e.g. `<form action={dyn} method="post">` where the
//     dynamic value lives in a $.attr() call) are skipped silently. The
//     dominant compiled-server shape collapses static attributes into a
//     single quasi prefix, so the common authoring pattern is covered.
//   - `method` detection requires the literal token `method="post"` (or
//     equivalent quoted variant; case-insensitive on POST) in the open
//     tag's static prefix. Dynamic method (`<form method={...}>`) is not
//     recognised — Svelte rarely emits that shape and treating it as a
//     non-POST form keeps the pass narrow.

// injectCSRF rewrites every <form method="post"> open tag found in the
// program's TemplateLiteral nodes to immediately emit a hidden CSRF
// input. The rewrite is destructive — Quasis and Expressions slices on
// affected TemplateLiteral nodes are replaced with newly allocated
// versions. Idempotent: a form already preceded by an injected input is
// detected via the unique token name and skipped.
//
// pageStateRef is the Go expression the emitter should splice in for
// the per-request CSRF token. The pre-pass synthesises a MemberExpression
// (pageState.CSRFToken) by default; callers that need a different
// expression (tests, future request-bound shapes) can override.
func injectCSRF(prog *Node) {
	if prog == nil {
		return
	}
	walkPushArguments(prog, rewriteFormsInPushArgument)
}

// walkPushArguments invokes fn on every node that occupies the
// argument position of a `$$renderer.push(...)` call or the right-hand
// side of a `$$payload.out += ...` assignment — the two ways Svelte 5
// emits HTML literals into the buffer. Anything else (helper calls,
// component dispatches, control flow) carries no static HTML the CSRF
// pre-pass can rewrite.
//
// Both shapes accept a Literal (single-quasi static HTML) or a
// TemplateLiteral (interleaved interpolations). fn must handle either.
func walkPushArguments(n *Node, fn func(*Node)) {
	if n == nil {
		return
	}
	if n.Type == "CallExpression" && n.Callee != nil &&
		n.Callee.Type == "MemberExpression" &&
		n.Callee.Object != nil && n.Callee.Object.Type == "Identifier" &&
		n.Callee.Property != nil && n.Callee.Property.Type == "Identifier" &&
		n.Callee.Property.Name == "push" &&
		len(n.Arguments) == 1 {
		// $$renderer.push(arg) — rewrite the single argument. We do
		// NOT gate on the receiver name because Svelte sometimes
		// renames it (`$$payload`, `$$renderer`); the structural
		// `.push(<arg>)` shape uniquely identifies the buffer write
		// at this layer.
		fn(n.Arguments[0])
	}
	if n.Type == "AssignmentExpression" && n.Operator == "+=" &&
		n.Left != nil && n.Left.Type == "MemberExpression" &&
		n.Left.Property != nil && n.Left.Property.Type == "Identifier" &&
		n.Left.Property.Name == "out" {
		// $$payload.out += <rhs>.
		fn(n.Right)
	}
	walkPushArguments(n.Expression, fn)
	walkPushArguments(n.Callee, fn)
	walkPushArguments(n.Object, fn)
	walkPushArguments(n.Property, fn)
	walkPushArguments(n.Argument, fn)
	walkPushArguments(n.Left, fn)
	walkPushArguments(n.Right, fn)
	walkPushArguments(n.Test, fn)
	walkPushArguments(n.Consequent, fn)
	walkPushArguments(n.Alternate, fn)
	walkPushArguments(n.Init, fn)
	walkPushArguments(n.Update, fn)
	walkPushArguments(n.FuncBody, fn)
	walkPushArguments(n.ID, fn)
	walkPushArguments(n.Source, fn)
	walkPushArguments(n.Declaration, fn)
	walkPushArguments(n.Imported, fn)
	walkPushArguments(n.Local, fn)
	walkPushArguments(n.Key, fn)
	walkPushArguments(n.Value, fn)
	for _, c := range n.Body {
		walkPushArguments(c, fn)
	}
	for _, c := range n.Arguments {
		walkPushArguments(c, fn)
	}
	for _, c := range n.Params {
		walkPushArguments(c, fn)
	}
	for _, c := range n.Declarations {
		walkPushArguments(c, fn)
	}
	for _, c := range n.Properties {
		walkPushArguments(c, fn)
	}
	for _, c := range n.Specifiers {
		walkPushArguments(c, fn)
	}
	for _, c := range n.Expressions {
		walkPushArguments(c, fn)
	}
}

// rewriteFormsInPushArgument dispatches the splice based on the arg
// shape: Literal becomes a TemplateLiteral when injection happens
// (the buffer-write semantics are identical and the emitter already
// handles both), TemplateLiteral gets in-place quasi/expression
// rewriting.
func rewriteFormsInPushArgument(arg *Node) {
	if arg == nil {
		return
	}
	switch arg.Type {
	case "Literal":
		if arg.LitKind != litString {
			return
		}
		pieces, exprs, didRewrite := spliceFormInQuasi(arg.LitStr)
		if !didRewrite {
			return
		}
		// Promote the Literal in place into a TemplateLiteral. The
		// emitter dispatches on `arg.Type == "TemplateLiteral"` in
		// formatPushArgument and walks Quasis + Expressions, so the
		// transformation lands on the existing rendering path with
		// no further changes.
		arg.Type = "TemplateLiteral"
		arg.Quasis = make([]*Node, 0, len(pieces))
		arg.Expressions = make([]*Node, 0, len(exprs))
		for i, p := range pieces {
			arg.Quasis = append(arg.Quasis, &Node{
				Type:   "TemplateElement",
				Cooked: p,
				Tail:   i == len(pieces)-1,
			})
			if i < len(exprs) {
				arg.Expressions = append(arg.Expressions, exprs[i])
			}
		}
		// Clear literal fields so future inspections (and JSON
		// re-encoding hypothetically) see a clean TemplateLiteral.
		arg.LitKind = litUnknown
		arg.LitStr = ""
		arg.Raw = ""
	case "TemplateLiteral":
		rewriteFormsInTemplateLiteral(arg)
	}
}

// walkTemplateLiterals invokes fn on every TemplateLiteral node found
// in n. Walks the same set of child links the emitter cares about
// (mirrors collectBareCallees in emitter.go) so newly added node types
// get covered there too.
func walkTemplateLiterals(n *Node, fn func(*Node)) {
	if n == nil {
		return
	}
	if n.Type == "TemplateLiteral" {
		fn(n)
		// Fall through so nested templates inside interpolations are
		// also visited — rare but possible.
	}
	walkTemplateLiterals(n.Expression, fn)
	walkTemplateLiterals(n.Callee, fn)
	walkTemplateLiterals(n.Object, fn)
	walkTemplateLiterals(n.Property, fn)
	walkTemplateLiterals(n.Argument, fn)
	walkTemplateLiterals(n.Left, fn)
	walkTemplateLiterals(n.Right, fn)
	walkTemplateLiterals(n.Test, fn)
	walkTemplateLiterals(n.Consequent, fn)
	walkTemplateLiterals(n.Alternate, fn)
	walkTemplateLiterals(n.Init, fn)
	walkTemplateLiterals(n.Update, fn)
	walkTemplateLiterals(n.FuncBody, fn)
	walkTemplateLiterals(n.ID, fn)
	walkTemplateLiterals(n.Source, fn)
	walkTemplateLiterals(n.Declaration, fn)
	walkTemplateLiterals(n.Imported, fn)
	walkTemplateLiterals(n.Local, fn)
	walkTemplateLiterals(n.Key, fn)
	walkTemplateLiterals(n.Value, fn)
	for _, c := range n.Body {
		walkTemplateLiterals(c, fn)
	}
	for _, c := range n.Arguments {
		walkTemplateLiterals(c, fn)
	}
	for _, c := range n.Params {
		walkTemplateLiterals(c, fn)
	}
	for _, c := range n.Declarations {
		walkTemplateLiterals(c, fn)
	}
	for _, c := range n.Properties {
		walkTemplateLiterals(c, fn)
	}
	for _, c := range n.Specifiers {
		walkTemplateLiterals(c, fn)
	}
	// Quasis carry no rewriteable nodes (TemplateElement.Cooked is a
	// string), but child Expressions do — those are walked above.
	for _, c := range n.Expressions {
		walkTemplateLiterals(c, fn)
	}
}

// rewriteFormsInTemplateLiteral scans tl's quasis for POST-form open
// tags and splices the hidden CSRF input in after each. Operates on
// quasis one at a time; multi-quasi form open tags (dynamic attributes
// on the open tag) are skipped — the function only matches whole open
// tags inside a single quasi.
//
// When at least one form was rewritten, both Quasis and Expressions are
// replaced wholesale with newly allocated slices. Untouched template
// literals leave the original slices alone (cheap no-op).
func rewriteFormsInTemplateLiteral(tl *Node) {
	if tl == nil || len(tl.Quasis) == 0 {
		return
	}

	// Build new quasi/expression streams in lockstep. The original
	// expression list interleaves between quasis: quasis[i] precedes
	// expressions[i]. After splicing we may have inserted extra
	// (quasi, expr) pairs corresponding to the hidden-input emit.
	newQuasis := make([]*Node, 0, len(tl.Quasis))
	newExprs := make([]*Node, 0, len(tl.Expressions))
	rewrote := false

	for i, q := range tl.Quasis {
		var trailing *Node
		if i < len(tl.Expressions) {
			trailing = tl.Expressions[i]
		}
		pieces, exprs, didRewrite := spliceFormInQuasi(q.Cooked)
		if !didRewrite {
			newQuasis = append(newQuasis, q)
			if trailing != nil {
				newExprs = append(newExprs, trailing)
			}
			continue
		}
		rewrote = true
		// Append the inner pieces first. spliceFormInQuasi returns
		// pieces and exprs in the same interleaved shape: pieces[i]
		// precedes exprs[i]; len(pieces) == len(exprs) + 1.
		for j, p := range pieces {
			isLast := j == len(pieces)-1
			elem := &Node{
				Type:   "TemplateElement",
				Cooked: p,
				// Tail flag carries through to the final synthesised
				// element of the literal as a whole; intermediate
				// elements (those with a following expression) are
				// non-tail. The original q.Tail flag belongs to the
				// final piece of THIS quasi; if there's still a
				// trailing expression after this quasi the synthesised
				// last piece is non-tail.
				Tail: isLast && q.Tail && trailing == nil,
			}
			newQuasis = append(newQuasis, elem)
			if !isLast {
				newExprs = append(newExprs, exprs[j])
			}
		}
		if trailing != nil {
			newExprs = append(newExprs, trailing)
		}
	}

	if !rewrote {
		return
	}
	tl.Quasis = newQuasis
	tl.Expressions = newExprs
}

// csrfTokenExpr returns the AST node the splice inserts where the
// CSRF token interpolation lives. The expression resolves to the
// per-request token via the PageState the SSR bridge passes into
// Render(). Synthesising a MemberExpression keeps the rewriter
// emitter-agnostic — formatExpression renders it the same way it
// renders any user expression.
func csrfTokenExpr() *Node {
	return &Node{
		Type:     "MemberExpression",
		Object:   &Node{Type: "Identifier", Name: "pageState"},
		Property: &Node{Type: "Identifier", Name: "CSRFToken"},
	}
}

// spliceFormInQuasi scans cooked HTML for `<form ... method="post" ... >`
// open tags and returns the cooked text split into N+1 pieces with N
// hidden-input expressions to emit between them. didRewrite reports
// whether any splice happened; when false the returned pieces / exprs
// are nil and the caller should keep the original quasi unchanged.
//
// The scan is HTML-aware enough to skip:
//
//   - Tag names that are not exactly "form" (case-insensitive).
//   - Open tags lacking method="post" / method='post' / method=POST etc.
//   - Open tags carrying a `nocsrf` attribute (also stripped from output).
//   - Self-closing tags ("<form ... />") — Svelte never emits these for
//     interactive forms; treat as non-form.
//   - Forms that already contain a hidden input named "_csrf_token"
//     immediately after the open tag (idempotent re-runs).
//
// Anything more exotic (e.g. nested `>` inside a quoted attribute value)
// is handled by the attribute-aware boundary scan in scanFormOpenTag.
func spliceFormInQuasi(cooked string) (pieces []string, exprs []*Node, didRewrite bool) {
	if cooked == "" {
		return nil, nil, false
	}
	if !containsFormStart(cooked) {
		return nil, nil, false
	}

	var (
		out         strings.Builder
		parts       []string
		ins         []*Node
		mutated     bool // text changed (nocsrf strip) even if no expressions injected
		injectedAny bool // at least one CSRF expression was inserted
	)
	pos := 0
	for {
		idx := indexFormStart(cooked, pos)
		if idx < 0 {
			break
		}
		end, rest, attrs, ok := scanFormOpenTag(cooked, idx)
		if !ok {
			// Malformed or multi-quasi straddle — copy the byte and
			// keep scanning past it so we don't infinite-loop.
			out.WriteString(cooked[pos : idx+1])
			pos = idx + 1
			continue
		}
		// Emit everything up to (but not including) the opening `<`.
		out.WriteString(cooked[pos:idx])

		if !attrs.isPost {
			// Non-POST form — copy the open tag verbatim and continue.
			out.WriteString(cooked[idx:end])
			pos = end
			continue
		}
		if attrs.nocsrf {
			// nocsrf opt-out: emit the open tag with the marker
			// attribute stripped (rest != cooked[idx:end]) and skip
			// the splice. The text mutation alone counts as a rewrite
			// so the caller propagates the cleaned quasi.
			out.WriteString(rest)
			pos = end
			mutated = true
			continue
		}
		if attrs.alreadyInjected {
			out.WriteString(cooked[idx:end])
			pos = end
			continue
		}

		// Splice. The accumulated `out` so far becomes one piece; emit
		// the open tag plus the literal hidden-input prefix; the next
		// piece starts at `">` to wrap the interpolated token.
		out.WriteString(rest)
		out.WriteString(`<input type="hidden" name="_csrf_token" value="`)
		parts = append(parts, out.String())
		ins = append(ins, csrfTokenExpr())
		out.Reset()
		out.WriteString(`">`)
		pos = end
		injectedAny = true
		mutated = true
	}

	if !mutated {
		return nil, nil, false
	}
	out.WriteString(cooked[pos:])
	if !injectedAny {
		// nocsrf-only path: a single rewritten piece, no CSRF
		// expression. The caller emits this as a one-element pieces
		// slice with zero expressions.
		return []string{out.String()}, nil, true
	}
	parts = append(parts, out.String())
	return parts, ins, true
}

// containsFormStart cheaply rejects quasis that have no `<form` token,
// avoiding the per-rune scan in indexFormStart for the dominant case
// (most quasis carry no form tag at all).
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

// indexFormStart returns the byte index of the next `<form` token in s
// at or after start, or -1 when none. The match is case-insensitive on
// the tag name and requires the byte after `<form` to be either `>` or
// whitespace (so `<form>` and `<formal>` don't collide).
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

// scanFormOpenTag walks a `<form ...>` open tag starting at idx (the
// position of the leading `<`) and returns:
//
//   - end: byte index one past the closing `>` of the open tag.
//   - rest: the open tag with the `nocsrf` attribute stripped (if any);
//     equals s[idx:end] when no stripping occurred.
//   - attrs: parsed flags driving the rewrite decision.
//   - ok: false when the open tag is malformed, self-closing, or
//     straddles the end of the quasi; true otherwise.
//
// The scan tracks single- and double-quoted attribute values so a
// quoted `>` inside `action="?/x>y"` is not mistaken for the tag close.
func scanFormOpenTag(s string, idx int) (end int, rest string, attrs formAttrs, ok bool) {
	// Skip past the literal "<form".
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

	// Parse the body of the open tag. We re-walk the attribute soup
	// twice (once to find `>`, once to extract attribute flags) but the
	// loop is single-pass: we drive a small state machine across the
	// bytes accumulating attribute name spans.
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
		// Skip whitespace between attributes.
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}
		if c == '/' {
			// Self-closing form (`<form ... />`). Treat as non-form.
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
		// Attribute name.
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
		// Optional value. Whitespace before `=` is allowed by HTML.
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
					// Unterminated quote; bail out — likely a quasi
					// straddle.
					return 0, "", formAttrs{}, false
				}
				span.valEnd = j
				j++ // step past the closing quote
			} else {
				// Unquoted value: spans up to the next whitespace or `>`.
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
		// Self-closing — caller treats the whole tag as non-form.
		return closeAt + 1, s[idx : closeAt+1], formAttrs{}, true
	}
	end = closeAt + 1

	// Apply attribute flags.
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
			// Strip the attribute (and any preceding whitespace) from
			// the output. We pick the widest span: from the byte right
			// after the previous attribute (or after `<form`) up to the
			// end of this span (including its quoted value if any).
			delStart := sp.nameStart
			// Walk back over leading whitespace so the rendered tag
			// doesn't end up with a double space.
			for delStart > pos && isAsciiSpace(s[delStart-1]) {
				delStart--
			}
			delEnd := sp.nameEnd
			if sp.hasValue {
				if sp.quote != 0 {
					delEnd = sp.valEnd + 1 // include the closing quote
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

	// Build rest with nocsrf stripped.
	if stripStart >= 0 {
		rest = s[idx:stripStart] + s[stripEnd:end]
	} else {
		rest = s[idx:end]
	}

	// alreadyInjected: the bytes immediately after `>` already start
	// with a hidden input carrying our token name. Idempotency guard
	// for repeated runs of the pass against the same AST.
	tail := s[end:]
	const marker = `<input type="hidden" name="_csrf_token" value="`
	if strings.HasPrefix(tail, marker) {
		attrs.alreadyInjected = true
	}

	return end, rest, attrs, true
}

func asciiToLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func isAsciiSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
