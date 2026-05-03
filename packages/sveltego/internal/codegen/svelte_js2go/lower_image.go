package sveltejs2go

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/images"
)

// Image-element lowering (issue #492). The pre-pass walks the AST
// looking for invocations of an `Image` component imported from the
// reserved framework module `@sveltego/enhanced-img`. Each call site
// `Image($$renderer, { src: "...", alt: "...", ... })` is replaced
// with a `$$renderer.push("<img …>")` whose HTML is resolved at build
// time against the [images.Result] map — the same multi-DPR `<img
// srcset=…>` shape the previous Mustache-Go lowering emitted.
//
// Resolution is build-time only by design (ADR 0009): the request path
// must stay JS-engine-free. WebP/AVIF, dynamic `src={expr}`, class /
// style directives on `<Image>`, and runtime transform back-ends are
// out of scope for this PR — see the PR body's deferred-items list.
//
// Errors land on the [imageInjector] accumulator and are returned via
// [Options.ImageErrors] after [TranspileNode] runs so the build driver
// can surface them with a precise route+source pointer. Returning a
// hard error without short-circuiting the transpile keeps the dispatch
// uniform with how `recordImport` already handles other framework
// virtual modules.

// imageImportSource is the reserved framework virtual module path the
// user imports the `<Image>` component from. The local binding name is
// not constrained — the pre-pass tracks whatever the user picked
// (`import Image from "@sveltego/enhanced-img"`,
// `import Img from "@sveltego/enhanced-img"`, etc.) so call-site
// substitution matches the chosen alias.
const imageImportSource = "@sveltego/enhanced-img"

// rendererParamNames is the closed set of identifier names svelte/server
// uses for the renderer parameter on a child-component invocation. The
// pre-pass requires the call's first arg to be one of these — anything
// else (e.g. a user-authored helper that happens to share the local
// name) leaves the call unchanged.
var rendererParamNames = map[string]struct{}{
	"$$renderer": {},
	"$$payload":  {},
}

// imageInjector mutates the AST in place, lowering Image(...) call
// expressions to renderer.push("...") nodes whose payload is the build-
// time-resolved <img srcset> markup. The injector accumulates errors
// for missing variants and dynamic-src forms so callers can surface
// every offending site in one build pass instead of failing on the
// first one.
type imageInjector struct {
	variants map[string]images.Result
	locals   map[string]struct{}
	errs     []error
}

// injectImage runs the Image-element lowering pre-pass over root.
// variants keys are forward-slash relative paths under static/assets/
// (no leading slash) — the same convention the [images.Build] pipeline
// uses. A nil or empty variants map is a no-op: routes that import
// nothing from the reserved module surface no Image local in scope and
// the walker exits early.
//
// Returned errors are joined with errors.Join so callers can format
// each on its own line. A non-nil return does not invalidate the
// partial AST mutation — the build driver should fail the build, not
// re-emit.
func injectImage(root *Node, variants map[string]images.Result) error {
	if root == nil {
		return nil
	}
	inj := &imageInjector{variants: variants, locals: map[string]struct{}{}}
	inj.collectImports(root)
	if len(inj.locals) == 0 {
		return nil
	}
	inj.walk(root)
	if len(inj.errs) == 0 {
		return nil
	}
	return joinErrors(inj.errs)
}

// joinErrors collapses multiple lowering errors into a single error
// whose message lists each offender on its own line. The format mirrors
// the [Lowerer.Err] output so build drivers can surface either kind
// uniformly.
func joinErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	return fmt.Errorf("%s", strings.Join(parts, "\n"))
}

// collectImports walks Program.body looking for ImportDeclaration nodes
// whose source matches [imageImportSource], registering each local
// binding as a candidate Image callee. Both default and named imports
// are accepted — svelte/compiler treats any capitalised tag as a
// component reference regardless of import shape.
func (i *imageInjector) collectImports(root *Node) {
	if root == nil || root.Type != "Program" {
		return
	}
	for idx, item := range root.Body {
		if item == nil || item.Type != "ImportDeclaration" {
			continue
		}
		if item.Source == nil || item.Source.LitStr != imageImportSource {
			continue
		}
		for _, sp := range item.Specifiers {
			if sp == nil || sp.Local == nil || sp.Local.Type != "Identifier" {
				continue
			}
			if sp.Local.Name == "" {
				continue
			}
			i.locals[sp.Local.Name] = struct{}{}
		}
		// Drop the import declaration so recordImport does not hit
		// unknownShape for the framework virtual module. Replace with
		// a no-op statement (an EmptyStatement is rendered as nothing
		// by emitProgram's switch). Use ExpressionStatement carrying a
		// nil Expression — emitProgram's outer dispatch reaches only
		// ImportDeclaration / ExportDefaultDeclaration / Function-
		// Declaration / VariableDeclaration / ExportNamedDeclaration,
		// so rewriting to ExportNamedDeclaration with an empty body
		// keeps it in the silent-skip branch.
		root.Body[idx] = &Node{Type: "ExportNamedDeclaration"}
	}
}

// walk descends into every node looking for ExpressionStatement entries
// whose Expression is a CallExpression matching one of the tracked
// Image locals. Match candidates are rewritten in place; non-matches
// are left untouched. The walker covers every ESTree branch the
// emitter visits so an Image invocation deep inside an {#each} body or
// {#if} branch is still rewritten.
func (i *imageInjector) walk(n *Node) {
	if n == nil {
		return
	}
	if n.Type == "ExpressionStatement" && n.Expression != nil {
		if rewritten, ok := i.rewriteCall(n.Expression); ok {
			n.Expression = rewritten
			return
		}
	}
	for _, c := range n.Body {
		i.walk(c)
	}
	i.walk(n.Expression)
	i.walk(n.Callee)
	i.walk(n.Object)
	i.walk(n.Property)
	i.walk(n.Argument)
	i.walk(n.Left)
	i.walk(n.Right)
	i.walk(n.Test)
	i.walk(n.Consequent)
	i.walk(n.Alternate)
	i.walk(n.Init)
	i.walk(n.Update)
	i.walk(n.FuncBody)
	i.walk(n.ID)
	i.walk(n.Source)
	i.walk(n.Declaration)
	for _, c := range n.Arguments {
		i.walk(c)
	}
	for _, c := range n.Params {
		i.walk(c)
	}
	for _, c := range n.Declarations {
		i.walk(c)
	}
	for _, c := range n.Properties {
		i.walk(c)
	}
	for _, c := range n.Specifiers {
		i.walk(c)
	}
	for _, c := range n.Quasis {
		i.walk(c)
	}
	for _, c := range n.Expressions {
		i.walk(c)
	}
}

// rewriteCall returns the substituted ExpressionStatement.Expression
// when call is `Image($$renderer, { src: "...", ... })`. The boolean
// signals whether a substitution occurred — false leaves the original
// node in place so unrelated user calls survive untouched.
//
// Rewrites land as `$$renderer.push("<img …>")` so the existing
// emitRendererPush path emits the resolved markup verbatim. The
// renderer identifier is read from the call's first arg so aliasing in
// the user's component (rare) does not break the substitution.
func (i *imageInjector) rewriteCall(call *Node) (*Node, bool) {
	if call.Type != "CallExpression" || call.Callee == nil {
		return nil, false
	}
	if call.Callee.Type != "Identifier" {
		return nil, false
	}
	if _, ok := i.locals[call.Callee.Name]; !ok {
		return nil, false
	}
	if len(call.Arguments) < 2 {
		i.errs = append(i.errs, fmt.Errorf("image lowering: %s() at byte=%d expects (renderer, props), got %d argument(s)", call.Callee.Name, call.Start, len(call.Arguments)))
		return nil, false
	}
	rendererArg := call.Arguments[0]
	if rendererArg == nil || rendererArg.Type != "Identifier" {
		i.errs = append(i.errs, fmt.Errorf("image lowering: %s() at byte=%d expects an Identifier renderer arg, got %s", call.Callee.Name, call.Start, typeOf(rendererArg)))
		return nil, false
	}
	if _, ok := rendererParamNames[rendererArg.Name]; !ok {
		i.errs = append(i.errs, fmt.Errorf("image lowering: %s() at byte=%d expects $$renderer/$$payload, got %q", call.Callee.Name, call.Start, rendererArg.Name))
		return nil, false
	}
	propsArg := call.Arguments[1]
	if propsArg == nil || propsArg.Type != "ObjectExpression" {
		i.errs = append(i.errs, fmt.Errorf("image lowering: %s() at byte=%d expects an inline ObjectExpression for props, got %s", call.Callee.Name, call.Start, typeOf(propsArg)))
		return nil, false
	}
	attrs, err := i.collectStaticAttrs(propsArg)
	if err != nil {
		i.errs = append(i.errs, fmt.Errorf("image lowering: %s() at byte=%d: %w", call.Callee.Name, call.Start, err))
		return nil, false
	}
	html, err := i.renderImageHTML(call, attrs)
	if err != nil {
		i.errs = append(i.errs, err)
		return nil, false
	}
	literal := &Node{
		Type:    "Literal",
		LitKind: litString,
		LitStr:  html,
		Raw:     strconv.Quote(html),
	}
	return &Node{
		Type: "CallExpression",
		Callee: &Node{
			Type:     "MemberExpression",
			Object:   &Node{Type: "Identifier", Name: rendererArg.Name},
			Property: &Node{Type: "Identifier", Name: "push"},
		},
		Arguments: []*Node{literal},
	}, true
}

// imageAttrs holds the parsed static prop set on an `<Image>` call.
// src is the only required field; everything else is optional and
// contributes either to the open-tag attribute list or to the HTML
// hints (loading / decoding).
type imageAttrs struct {
	src       string
	alt       string
	hasAlt    bool
	width     int
	height    int
	hasW      bool
	hasH      bool
	priority  bool
	classAttr string
	hasClass  bool
	sizes     string
	hasSizes  bool
}

// collectStaticAttrs walks the ObjectExpression that compiled-server
// emits for `<Image src=… alt=… />` props. Only static-string and
// static-number prop values are accepted; dynamic forms surface as a
// clear error so the user can either inline the value or annotate the
// route for the sidecar fallback.
func (i *imageInjector) collectStaticAttrs(obj *Node) (imageAttrs, error) {
	var attrs imageAttrs
	for _, p := range obj.Properties {
		if p == nil {
			continue
		}
		if p.Type != "Property" {
			return attrs, fmt.Errorf("Image props: unsupported property kind %q (only static keys allowed)", p.Type)
		}
		if p.Computed {
			return attrs, fmt.Errorf("Image props: computed property keys are not supported")
		}
		if p.Key == nil || p.Key.Type != "Identifier" {
			return attrs, fmt.Errorf("Image props: prop key must be a bare identifier")
		}
		name := p.Key.Name
		val := p.Value
		switch name {
		case "src":
			s, ok := literalString(val)
			if !ok {
				return attrs, fmt.Errorf(`Image src=%s is dynamic; only static "src=\"…\"" is supported`, formatNodeKind(val))
			}
			attrs.src = strings.TrimPrefix(s, "/")
		case "alt":
			s, ok := literalString(val)
			if !ok {
				return attrs, fmt.Errorf(`Image alt=%s is dynamic; only static alt is supported`, formatNodeKind(val))
			}
			attrs.alt = s
			attrs.hasAlt = true
		case "width":
			n, ok := literalIntish(val)
			if !ok {
				return attrs, fmt.Errorf("Image width must be a number literal")
			}
			attrs.width = n
			attrs.hasW = true
		case "height":
			n, ok := literalIntish(val)
			if !ok {
				return attrs, fmt.Errorf("Image height must be a number literal")
			}
			attrs.height = n
			attrs.hasH = true
		case "eager", "priority":
			b, ok := literalBool(val)
			if !ok {
				// Bare prop shorthand `<Image priority />` compiles to
				// `priority: true` so the literal-bool path covers it.
				return attrs, fmt.Errorf("Image %s must be a boolean literal", name)
			}
			if b {
				attrs.priority = true
			}
		case "class":
			s, ok := literalString(val)
			if !ok {
				return attrs, fmt.Errorf("Image class must be a static string (dynamic class= is not supported)")
			}
			attrs.classAttr = s
			attrs.hasClass = true
		case "sizes":
			s, ok := literalString(val)
			if !ok {
				return attrs, fmt.Errorf("Image sizes must be a static string")
			}
			attrs.sizes = s
			attrs.hasSizes = true
		case "loading", "decoding":
			// Honor explicit user override but require static.
			s, ok := literalString(val)
			if !ok {
				return attrs, fmt.Errorf("Image %s must be a static string", name)
			}
			if name == "loading" && s == "eager" {
				attrs.priority = true
			}
		default:
			return attrs, fmt.Errorf("Image: unsupported attribute %q (supported: src, alt, width, height, sizes, class, eager, priority, loading, decoding)", name)
		}
	}
	if attrs.src == "" {
		return attrs, fmt.Errorf("Image is missing required src attribute")
	}
	return attrs, nil
}

// renderImageHTML builds the `<img>` open tag for the resolved variant
// set. The fallback `src` always points at the largest variant so
// non-srcset browsers see a real, full-resolution URL. width/height
// fall back to the source's intrinsic dimensions when the user did not
// supply them — keeping CLS-free rendering even when the template is
// terse.
func (i *imageInjector) renderImageHTML(call *Node, attrs imageAttrs) (string, error) {
	res, ok := i.variants[attrs.src]
	if !ok {
		return "", fmt.Errorf("image lowering: <Image src=%q> at byte=%d not found in static/assets/ (build the variant set or check the path)", attrs.src, call.Start)
	}
	if len(res.Variants) == 0 {
		return "", fmt.Errorf("image lowering: <Image src=%q> at byte=%d produced no variants", attrs.src, call.Start)
	}
	sorted := make([]images.Variant, len(res.Variants))
	copy(sorted, res.Variants)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Width < sorted[b].Width })
	fallback := sorted[len(sorted)-1]

	width, height, hasW, hasH := attrs.width, attrs.height, attrs.hasW, attrs.hasH
	if !hasW && res.IntrinsicWidth > 0 {
		width = res.IntrinsicWidth
		hasW = true
	}
	if !hasH && res.IntrinsicHeight > 0 {
		height = res.IntrinsicHeight
		hasH = true
	}

	loading := "lazy"
	decoding := "async"
	if attrs.priority {
		loading = "eager"
	}

	var sb strings.Builder
	sb.WriteString(`<img src="`)
	sb.WriteString(escapeAttrValue(fallback.URL))
	sb.WriteString(`"`)
	if len(sorted) > 1 {
		sb.WriteString(` srcset="`)
		for j, v := range sorted {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(escapeAttrValue(v.URL))
			sb.WriteString(" ")
			sb.WriteString(strconv.Itoa(v.Width))
			sb.WriteString("w")
		}
		sb.WriteString(`"`)
	}
	if attrs.hasSizes {
		sb.WriteString(` sizes="`)
		sb.WriteString(escapeAttrValue(attrs.sizes))
		sb.WriteString(`"`)
	}
	if hasW {
		sb.WriteString(` width="`)
		sb.WriteString(strconv.Itoa(width))
		sb.WriteString(`"`)
	}
	if hasH {
		sb.WriteString(` height="`)
		sb.WriteString(strconv.Itoa(height))
		sb.WriteString(`"`)
	}
	if attrs.hasAlt {
		sb.WriteString(` alt="`)
		sb.WriteString(escapeAttrValue(attrs.alt))
		sb.WriteString(`"`)
	} else {
		// Always emit alt — empty alt is the correct a11y signal for a
		// purely decorative image, and svelte/server defaults to that
		// when the prop is absent. Matches the Mustache-Go behaviour.
		sb.WriteString(` alt=""`)
	}
	if attrs.hasClass {
		sb.WriteString(` class="`)
		sb.WriteString(escapeAttrValue(attrs.classAttr))
		sb.WriteString(`"`)
	}
	sb.WriteString(` loading="`)
	sb.WriteString(loading)
	sb.WriteString(`" decoding="`)
	sb.WriteString(decoding)
	sb.WriteString(`">`)
	return sb.String(), nil
}

// literalString returns the cooked string value of a Literal node when
// kind is litString. Returns ok=false for any other shape so callers
// can surface a clear "dynamic value" error.
func literalString(n *Node) (string, bool) {
	if n == nil || n.Type != "Literal" || n.LitKind != litString {
		return "", false
	}
	return n.LitStr, true
}

// literalIntish returns the integer value of a Literal node when kind
// is litNumber and the value is a whole number. Decimal numbers are
// rejected because HTML width/height attributes are integer-valued.
func literalIntish(n *Node) (int, bool) {
	if n == nil || n.Type != "Literal" || n.LitKind != litNumber {
		return 0, false
	}
	if n.LitNum != float64(int(n.LitNum)) {
		return 0, false
	}
	return int(n.LitNum), true
}

// literalBool returns the boolean value of a Literal node when kind is
// litBool. The bare-prop shorthand `<Image priority />` compiles to
// `priority: true` so litBool covers the common path.
func literalBool(n *Node) (bool, bool) {
	if n == nil || n.Type != "Literal" || n.LitKind != litBool {
		return false, false
	}
	return n.LitBool, true
}

// formatNodeKind returns a short label describing n's structural shape.
// Used when surfacing "src is dynamic" errors so the user sees what
// kind of expression the template produced (Identifier, MemberExpression,
// TemplateLiteral, …) instead of a raw "got nil" message.
func formatNodeKind(n *Node) string {
	if n == nil {
		return "<nil>"
	}
	return n.Type
}

// escapeAttrValue HTML-escapes a value being written between double
// quotes in an HTML attribute. Mirrors the Mustache-Go escaping rules
// (& → &amp;, " → &quot;, < → &lt;, > → &gt;) so srcset/sizes/alt
// produce identical bytes to the previous lowering. Single quotes are
// safe inside double-quoted attributes.
func escapeAttrValue(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
