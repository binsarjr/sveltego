package svelte_js2go

import (
	"fmt"
	"strconv"
	"strings"
)

// defaultHelpers returns the v1-locked Svelte helper dispatch table.
// Each entry maps a $.<name>(args) call to the equivalent Go
// expression (when used in expression position) or statement (when
// used as a top-level call). ADR 0009 sub-decision 2 makes anything
// outside this table a hard error.
func defaultHelpers() map[string]helperHandler {
	return map[string]helperHandler{
		"escape":              helperEscapeContent,
		"escape_html":         helperEscapeContent,
		"attr":                helperAttr,
		"clsx":                helperClsx,
		"stringify":           helperStringify,
		"spread_attributes":   helperSpreadAttributes,
		"merge_styles":        helperMergeStyles,
		"head":                helperHead,
		"html":                helperHTML,
		"each":                helperEach,
		"if":                  helperIf,
		"element":             helperElement,
		"ensure_array_like":   helperEnsureArrayLike,
		"sanitize_props":      helperSanitizeProps,
		"sanitize_slots":      helperSanitizeSlots,
		"spread_props":        helperSpreadProps,
		"rest_props":          helperRestProps,
		"fallback":            helperFallback,
		"exclude_from_object": helperExcludeFromObject,
		"to_class":            helperToClass,
		"to_style":            helperToStyle,
		"slot":                helperSlot,
	}
}

func helperEscapeContent(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "escape:no-arg")
	}
	inner, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	// Optional second arg in Svelte is `is_attr`. v1 lowers attr
	// position via $.attr; bare $.escape always lands in content.
	if len(args) >= 2 {
		if args[1].Type == "Literal" && args[1].LitKind == litBool && args[1].LitBool {
			return wrap(b, asExpr, fmt.Sprintf("%s.EscapeHTMLAttr(%s)", e.opts.HelperAlias, inner))
		}
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.EscapeHTML(%s)", e.opts.HelperAlias, inner))
}

func helperAttr(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 2 {
		return "", unknownShape(call, "attr:args")
	}
	name, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	value, err := e.formatExpression(args[1])
	if err != nil {
		return "", err
	}
	isBoolean := "false"
	if len(args) >= 3 {
		bv, err := e.formatExpression(args[2])
		if err != nil {
			return "", err
		}
		isBoolean = bv
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.Attr(%s, %s, %s)", e.opts.HelperAlias, name, value, isBoolean))
}

func helperClsx(e *emitter, b *Buf, args []*Node, asExpr bool, _ *Node) (string, error) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		s, err := e.formatExpression(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.Clsx(%s)", e.opts.HelperAlias, strings.Join(parts, ", ")))
}

func helperStringify(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "stringify:args")
	}
	inner, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.Stringify(%s)", e.opts.HelperAlias, inner))
}

func helperSpreadAttributes(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "spread_attributes:args")
	}
	inner, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	cast := fmt.Sprintf("toMapStringAny(%s)", inner)
	return wrap(b, asExpr, fmt.Sprintf("%s.SpreadAttributes(%s)", e.opts.HelperAlias, cast))
}

func helperMergeStyles(e *emitter, b *Buf, args []*Node, asExpr bool, _ *Node) (string, error) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		s, err := e.formatExpression(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.MergeStyles(%s)", e.opts.HelperAlias, strings.Join(parts, ", ")))
}

func helperHead(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if asExpr {
		return "", unknownShape(call, "head:expr-position")
	}
	if len(args) < 1 {
		return "", unknownShape(call, "head:args")
	}
	hash := ""
	fnNode := args[0]
	if len(args) >= 2 {
		// Some Svelte versions pass (hash, fn). Others pass (fn).
		if args[0].Type == "Literal" && args[0].LitKind == litString {
			hash = strconv.Quote(args[0].LitStr)
			fnNode = args[1]
		}
	}
	if hash == "" {
		hash = `""`
	}
	if fnNode.Type != "ArrowFunctionExpression" && fnNode.Type != "FunctionExpression" {
		return "", unknownShape(fnNode, "head:non-fn")
	}
	closure, err := e.formatFunctionExprForHead(fnNode)
	if err != nil {
		return "", err
	}
	b.Line("%s.Head(payload, %s, %s)", e.opts.HelperAlias, hash, closure)
	return "", nil
}

// formatFunctionExprForHead is a head-specific arrow-fn formatter:
// the closure receives a *server.Payload directly (vs. anonymous any).
func (e *emitter) formatFunctionExprForHead(n *Node) (string, error) {
	if n.FuncBody == nil || n.FuncBody.Type != "BlockStatement" {
		return "", unknownShape(n, "head-fn-body")
	}
	parent := e.scope
	defer func() { e.scope = parent }()
	e.scope = newScope(parent)
	body := &Buf{}
	if err := e.emitBlock(body, n.FuncBody, false); err != nil {
		return "", err
	}
	indented := indent(body.String(), 1)
	return fmt.Sprintf("func(payload *%s.Payload) {\n%s}", e.opts.HelperAlias, indented), nil
}

func helperHTML(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	// {@html raw} — Phase 4 quirks list this as v1-skipped. Surface
	// the unknown shape so a follow-up issue can land it explicitly.
	_ = e
	_ = b
	_ = args
	_ = asExpr
	return "", unknownShape(call, "helper:html (deferred per Phase 4 quirks)")
}

func helperEach(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	// Svelte 5.55.5 lowers {#each} via a hand-rolled for loop in the
	// compiled output (see each-list fixture), so $.each should not
	// appear at the top level. Surface unknown to flag corpus drift.
	_ = e
	_ = b
	_ = args
	_ = asExpr
	return "", unknownShape(call, "helper:each (expected as inline for, not $.each)")
}

func helperIf(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	// $.if(cond, branch, else?) — older shape, surface as unknown
	// because Svelte 5.55.5 emits IfStatement directly.
	_ = e
	_ = b
	_ = args
	_ = asExpr
	return "", unknownShape(call, "helper:if (expected as IfStatement, not $.if)")
}

func helperElement(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	// $.element(payload, tag, attrsFn, childrenFn) — used by
	// <svelte:element this={tag}>.
	if len(args) < 2 {
		return "", unknownShape(call, "element:args")
	}
	tag, err := e.formatExpression(args[1])
	if err != nil {
		return "", err
	}
	attrsArg := "nil"
	childrenArg := "nil"
	if len(args) >= 3 && (args[2].Type == "ArrowFunctionExpression" || args[2].Type == "FunctionExpression") {
		closure, err := e.formatFunctionExprForHead(args[2])
		if err != nil {
			return "", err
		}
		attrsArg = closure
	}
	if len(args) >= 4 && (args[3].Type == "ArrowFunctionExpression" || args[3].Type == "FunctionExpression") {
		closure, err := e.formatFunctionExprForHead(args[3])
		if err != nil {
			return "", err
		}
		childrenArg = closure
	}
	expr := fmt.Sprintf("%s.Element(payload, %s, %s, %s)", e.opts.HelperAlias, tag, attrsArg, childrenArg)
	if asExpr {
		return expr, nil
	}
	b.Line("%s", expr)
	return "", nil
}

func helperEnsureArrayLike(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "ensure_array_like:args")
	}
	inner, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.EnsureArrayLike(%s)", e.opts.HelperAlias, inner))
}

func helperSanitizeProps(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "sanitize_props:args")
	}
	inner, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	cast := fmt.Sprintf("toMapStringAny(%s)", inner)
	return wrap(b, asExpr, fmt.Sprintf("%s.SanitizeProps(%s)", e.opts.HelperAlias, cast))
}

func helperSanitizeSlots(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "sanitize_slots:args")
	}
	inner, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	cast := fmt.Sprintf("toMapStringAny(%s)", inner)
	return wrap(b, asExpr, fmt.Sprintf("%s.SanitizeSlots(%s)", e.opts.HelperAlias, cast))
}

func helperSpreadProps(e *emitter, b *Buf, args []*Node, asExpr bool, _ *Node) (string, error) {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		s, err := e.formatExpression(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, fmt.Sprintf("toMapStringAny(%s)", s))
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.SpreadProps(%s)", e.opts.HelperAlias, strings.Join(parts, ", ")))
}

func helperRestProps(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "rest_props:args")
	}
	src, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	cast := fmt.Sprintf("toMapStringAny(%s)", src)
	rest := make([]string, 0, len(args)-1)
	for _, a := range args[1:] {
		s, err := e.formatExpression(a)
		if err != nil {
			return "", err
		}
		rest = append(rest, s)
	}
	if len(rest) == 0 {
		return wrap(b, asExpr, fmt.Sprintf("%s.RestProps(%s)", e.opts.HelperAlias, cast))
	}
	// Concatenate variadic string args.
	return wrap(b, asExpr, fmt.Sprintf("%s.RestProps(%s, %s)", e.opts.HelperAlias, cast, strings.Join(rest, ", ")))
}

func helperFallback(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 2 {
		return "", unknownShape(call, "fallback:args")
	}
	v, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	d, err := e.formatExpression(args[1])
	if err != nil {
		return "", err
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.Fallback(%s, %s)", e.opts.HelperAlias, v, d))
}

func helperExcludeFromObject(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "exclude_from_object:args")
	}
	src, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	cast := fmt.Sprintf("toMapStringAny(%s)", src)
	rest := make([]string, 0, len(args)-1)
	for _, a := range args[1:] {
		s, err := e.formatExpression(a)
		if err != nil {
			return "", err
		}
		rest = append(rest, s)
	}
	if len(rest) == 0 {
		return wrap(b, asExpr, fmt.Sprintf("%s.ExcludeFromObject(%s)", e.opts.HelperAlias, cast))
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.ExcludeFromObject(%s, %s)", e.opts.HelperAlias, cast, strings.Join(rest, ", ")))
}

func helperToClass(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "to_class:args")
	}
	value, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	hash := `""`
	if len(args) >= 2 {
		h, err := e.formatExpression(args[1])
		if err != nil {
			return "", err
		}
		hash = h
	}
	directives := "nil"
	if len(args) >= 3 {
		d, err := e.formatExpression(args[2])
		if err != nil {
			return "", err
		}
		directives = fmt.Sprintf("toMapStringBool(%s)", d)
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.ToClass(%s, %s, %s)", e.opts.HelperAlias, value, hash, directives))
}

func helperToStyle(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if len(args) < 1 {
		return "", unknownShape(call, "to_style:args")
	}
	value, err := e.formatExpression(args[0])
	if err != nil {
		return "", err
	}
	styles := "nil"
	if len(args) >= 2 {
		s, err := e.formatExpression(args[1])
		if err != nil {
			return "", err
		}
		styles = fmt.Sprintf("toMapStringString(%s)", s)
	}
	return wrap(b, asExpr, fmt.Sprintf("%s.ToStyle(%s, %s)", e.opts.HelperAlias, value, styles))
}

func helperSlot(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error) {
	if asExpr {
		return "", unknownShape(call, "slot:expr-position")
	}
	if len(args) < 3 {
		return "", unknownShape(call, "slot:args")
	}
	// args[0] is $$payload, args[1] is props, args[2] is name string,
	// args[3] is optional slotProps map, args[4] optional fallback fn.
	propsExpr, err := e.formatExpression(args[1])
	if err != nil {
		return "", err
	}
	nameExpr, err := e.formatExpression(args[2])
	if err != nil {
		return "", err
	}
	slotProps := "nil"
	if len(args) >= 4 {
		sp, err := e.formatExpression(args[3])
		if err != nil {
			return "", err
		}
		slotProps = fmt.Sprintf("toMapStringAny(%s)", sp)
	}
	fallback := "nil"
	if len(args) >= 5 {
		fb, err := e.formatFunctionExprForHead(args[4])
		if err != nil {
			return "", err
		}
		fallback = fb
	}
	b.Line("%s.Slot(payload, toMapStringAny(%s), %s, %s, %s)", e.opts.HelperAlias, propsExpr, nameExpr, slotProps, fallback)
	return "", nil
}

// wrap routes the expression through the right caller-context: for
// statement position it writes to the buffer (no-op for expression
// helpers that don't side-effect); for expression position it just
// returns the string.
func wrap(b *Buf, asExpr bool, expr string) (string, error) {
	if asExpr {
		return expr, nil
	}
	b.Line("_ = %s", expr)
	return "", nil
}
