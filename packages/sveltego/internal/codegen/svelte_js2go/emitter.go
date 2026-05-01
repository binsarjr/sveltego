package sveltejs2go

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"strconv"
	"strings"
)

// Options configures Transpile.
type Options struct {
	// PackageName is the Go package the generated file belongs to.
	// Defaults to "gen" when empty.
	PackageName string

	// FuncName is the exported render function name. Defaults to
	// "Render".
	FuncName string

	// HelperImport is the import path of the runtime helpers package.
	// Defaults to the canonical runtime path.
	HelperImport string

	// HelperAlias is the local alias used for helper calls in the
	// generated code. Defaults to "server".
	HelperAlias string

	// Rewriter, if non-nil, is invoked on every Identifier and
	// MemberExpression as the emitter formats expressions. Phase 5
	// (#427) plugs property-access lowering here. The default
	// implementation is identity.
	Rewriter ExprRewriter
}

// ExprRewriter is the extension point Phase 5 uses to lower JS
// property accesses (data.name) into Go field accesses (data.Name).
// The walker invokes Rewrite for each Identifier or MemberExpression
// it formats; the returned string replaces the default rendering.
// Returning the empty string means "use default rendering".
type ExprRewriter interface {
	Rewrite(scope *Scope, n *Node, def string) string
}

// SpreadRewriter is an optional interface a rewriter can implement to
// also lower {`...expr`} object-spread elements that the Phase 3
// emitter renders as `/* spread */ <expr>` placeholders. The emitter
// invokes RewriteObjectSpread on each SpreadElement found in an
// ObjectExpression; the returned string replaces the placeholder when
// expanded is true. expanded=false leaves the placeholder in place
// (the legacy Phase 3 rendering) so callers without spread support
// keep working unchanged.
type SpreadRewriter interface {
	RewriteObjectSpread(scope *Scope, spread *Node, inner string) (rewritten string, expanded bool)
}

// Scope tracks locals introduced inside the render function. Phase 5
// reads it to know which identifiers are user-data references (subject
// to lowering) and which are emitter-introduced bookkeeping (left
// alone).
type Scope struct {
	parent  *Scope
	locals  map[string]LocalKind
	dataVar string
}

// LocalKind classifies how a name was bound in scope. Phase 5 uses
// this to decide whether to apply property-access lowering — only
// LocalProp identifiers root user-data trees that get JSON-tag
// translation.
type LocalKind uint8

const (
	LocalUnknown LocalKind = iota
	// LocalProp marks a destructured prop (data, params, etc.) —
	// the root of a user-data subtree subject to Phase 5 lowering.
	LocalProp
	// LocalEach marks an {#each} item alias inside a loop body.
	LocalEach
	// LocalScratch marks emitter-introduced bookkeeping names
	// (ssvar_index, each_array). Phase 5 leaves these alone.
	LocalScratch
	// LocalSnippet marks a {#snippet}-bound closure.
	LocalSnippet
)

func newScope(parent *Scope) *Scope {
	return &Scope{parent: parent, locals: map[string]LocalKind{}}
}

// Lookup returns the kind of a local, walking parent scopes. Returns
// LocalUnknown when the name isn't bound.
func (s *Scope) Lookup(name string) LocalKind {
	for cur := s; cur != nil; cur = cur.parent {
		if k, ok := cur.locals[name]; ok {
			return k
		}
	}
	return LocalUnknown
}

// IsDataRoot reports whether name refers to the destructured props
// root the user's Svelte component declared (typically "data").
// Phase 5 uses this to gate JSON-tag lowering.
func (s *Scope) IsDataRoot(name string) bool {
	for cur := s; cur != nil; cur = cur.parent {
		if cur.dataVar == name {
			return true
		}
	}
	return false
}

func (s *Scope) declare(name string, kind LocalKind) {
	if name == "" {
		return
	}
	s.locals[name] = kind
}

// Buf is the typed wrapper around the Go-source builder. Keeps line
// indentation explicit so callers don't accidentally double-indent.
type Buf struct {
	bytes.Buffer
	indent int
}

func (b *Buf) writeIndent() {
	for i := 0; i < b.indent; i++ {
		b.WriteByte('\t')
	}
}

// Line appends a newline-terminated indented line.
func (b *Buf) Line(format string, args ...any) {
	b.writeIndent()
	if len(args) == 0 {
		b.WriteString(format)
	} else {
		fmt.Fprintf(b, format, args...)
	}
	b.WriteByte('\n')
}

// In bumps the indentation, runs fn, then unindents.
func (b *Buf) In(fn func()) {
	b.indent++
	fn()
	b.indent--
}

// errUnknown is the canonical "unknown shape" build failure. Format
// per ADR 0009 sub-decision 2: `unknown emit shape at <pos>: <snippet>`.
type errUnknown struct {
	Pos  int
	Kind string
}

func (e *errUnknown) Error() string {
	return fmt.Sprintf("unknown emit shape at %d: %s", e.Pos, e.Kind)
}

func unknownShape(n *Node, kind string) error {
	pos := 0
	if n != nil {
		pos = n.Start
	}
	return &errUnknown{Pos: pos, Kind: kind}
}

// Transpile lowers an AST envelope into a Go source file. Output is
// gofumpt/goimports clean; callers don't need to reformat.
func Transpile(envelope []byte, opts Options) ([]byte, error) {
	var env Envelope
	if err := jsonUnmarshal(envelope, &env); err != nil {
		return nil, fmt.Errorf("svelte_js2go: parse envelope: %w", err)
	}
	if env.Schema != "" && env.Schema != "ssr-json-ast/v1" {
		return nil, fmt.Errorf("svelte_js2go: unsupported schema %q (want ssr-json-ast/v1)", env.Schema)
	}
	root, err := Decode(env.AST)
	if err != nil {
		return nil, err
	}
	return TranspileNode(root, env.Route, opts)
}

// TranspileNode is the variant for callers that already have a parsed
// Node tree (used by tests that synthesize ASTs directly without
// going through the sidecar).
func TranspileNode(root *Node, route string, opts Options) ([]byte, error) {
	if opts.PackageName == "" {
		opts.PackageName = "gen"
	}
	if opts.FuncName == "" {
		opts.FuncName = "Render"
	}
	if opts.HelperImport == "" {
		opts.HelperImport = "github.com/binsarjr/sveltego/packages/sveltego/runtime/svelte/server"
	}
	if opts.HelperAlias == "" {
		opts.HelperAlias = "server"
	}

	e := &emitter{
		opts:    opts,
		root:    root,
		route:   route,
		scope:   newScope(nil),
		helpers: defaultHelpers(),
	}
	return e.run()
}

type emitter struct {
	opts    Options
	root    *Node
	route   string
	scope   *Scope
	helpers map[string]helperHandler

	// rendererName holds the local identifier the compiled code uses
	// for the renderer parameter (typically "$$renderer"). Captured
	// during walk so push/component dispatch can match it.
	rendererName string
	helperNS     string // local namespace identifier for `import * as $`
}

func (e *emitter) run() ([]byte, error) {
	if e.root == nil || e.root.Type != "Program" {
		return nil, fmt.Errorf("svelte_js2go: expected Program root, got %q", typeOf(e.root))
	}

	body := &Buf{}
	body.indent = 1
	if err := e.emitProgram(body, e.root); err != nil {
		return nil, err
	}

	var out Buf
	out.Line("// Code generated by sveltego svelte_js2go. DO NOT EDIT.")
	out.Line("// route: %s", e.route)
	out.Line("")
	out.Line("package %s", e.opts.PackageName)
	out.Line("")
	out.Line("import (")
	out.indent = 1
	out.Line("%s %q", e.opts.HelperAlias, e.opts.HelperImport)
	out.indent = 0
	out.Line(")")
	out.Line("")
	out.Line("// %s mirrors the Svelte component's compiled server output.", e.opts.FuncName)
	out.Line("// Generated from route %s.", e.route)
	out.Line("func %s(payload *%s.Payload, props map[string]any) {", e.opts.FuncName, e.opts.HelperAlias)
	out.Write(body.Bytes())
	out.Line("}")

	// go/format re-indents the whole file. This eliminates the
	// per-emitter indent bookkeeping needed for nested closures and
	// guarantees gofmt-clean output regardless of how nested helper
	// emitters laid out their lines.
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		return nil, fmt.Errorf("svelte_js2go: format generated source: %w\n--- source:\n%s", err, out.Bytes())
	}
	return formatted, nil
}

// emitProgram walks the Program body. Only ImportDeclaration and the
// ExportDefaultDeclaration carrying the render function matter; other
// top-level statements are surfaced as unknown shapes so Phase 8 can
// route the route through the sidecar fallback.
func (e *emitter) emitProgram(b *Buf, prog *Node) error {
	var renderFn *Node
	for _, item := range prog.Body {
		switch item.Type {
		case "ImportDeclaration":
			if err := e.recordImport(item); err != nil {
				return err
			}
		case "ExportDefaultDeclaration":
			renderFn = item.Declaration
		case "ExportNamedDeclaration":
			// Side helpers Svelte sometimes emits (e.g. $$bindings).
			// v1 ignores them — they're consumed by features
			// outside the priority list.
			continue
		case "FunctionDeclaration", "VariableDeclaration":
			// Side helpers like generated `function _$foo(...) {}`.
			// v1 only walks the export-default render function.
			continue
		default:
			return unknownShape(item, "top:"+item.Type)
		}
	}
	if renderFn == nil {
		return fmt.Errorf("svelte_js2go: no export default render function in route %s", e.route)
	}
	return e.emitRenderFunction(b, renderFn)
}

func (e *emitter) recordImport(decl *Node) error {
	if decl.Source == nil || decl.Source.LitStr != "svelte/internal/server" {
		return unknownShape(decl, "import:"+litStr(decl.Source))
	}
	for _, sp := range decl.Specifiers {
		if sp.Type == "ImportNamespaceSpecifier" && sp.Local != nil {
			e.helperNS = sp.Local.Name
		}
	}
	if e.helperNS == "" {
		return unknownShape(decl, "import:no namespace specifier")
	}
	return nil
}

func (e *emitter) emitRenderFunction(b *Buf, fn *Node) error {
	if fn == nil {
		return errors.New("svelte_js2go: render function missing")
	}
	if fn.Type != "FunctionDeclaration" && fn.Type != "ArrowFunctionExpression" && fn.Type != "FunctionExpression" {
		return unknownShape(fn, "render-fn:"+fn.Type)
	}
	// Capture the renderer parameter name. The compiled output uses
	// `$$renderer` consistently, but read it dynamically for
	// robustness against minor pin moves.
	if len(fn.Params) >= 1 && fn.Params[0].Type == "Identifier" {
		e.rendererName = fn.Params[0].Name
	} else {
		e.rendererName = "$$renderer"
	}
	if fn.FuncBody == nil || fn.FuncBody.Type != "BlockStatement" {
		return unknownShape(fn, "render-fn:body")
	}
	return e.emitBlock(b, fn.FuncBody, true)
}

// emitBlock walks a BlockStatement. When isRoot the renderer is the
// outer one; nested blocks (slot fragments, control flow arms) inherit
// scope but reuse the same renderer reference.
func (e *emitter) emitBlock(b *Buf, block *Node, isRoot bool) error {
	for _, stmt := range block.Body {
		if err := e.emitStatement(b, stmt); err != nil {
			return err
		}
	}
	_ = isRoot
	return nil
}

func (e *emitter) emitStatement(b *Buf, stmt *Node) error {
	switch stmt.Type {
	case "VariableDeclaration":
		return e.emitVarDecl(b, stmt)
	case "ExpressionStatement":
		return e.emitExpressionStatement(b, stmt)
	case "IfStatement":
		return e.emitIf(b, stmt)
	case "ForStatement":
		return e.emitFor(b, stmt)
	case "ForOfStatement":
		return e.emitForOf(b, stmt)
	case "BlockStatement":
		return e.emitBlock(b, stmt, false)
	case "ReturnStatement":
		// ReturnStatement inside helper closures (slot fragments).
		// At the top level the render function has no return; treat
		// any return as a hard error.
		return unknownShape(stmt, "return at top level")
	}
	return unknownShape(stmt, "stmt:"+stmt.Type)
}

// emitExpressionStatement dispatches on the most common shape:
// $$renderer.push(template-or-string) and $$renderer.component(arrowFn),
// plus helper calls.
func (e *emitter) emitExpressionStatement(b *Buf, stmt *Node) error {
	expr := stmt.Expression
	if expr == nil {
		return nil
	}
	if expr.Type == "AssignmentExpression" && expr.Operator == "+=" {
		// Legacy `$$payload.out += '...'` shape — kept for cross-compat
		// even though Svelte 5.55.5 emits renderer.push.
		return e.emitPayloadAdd(b, expr)
	}
	if expr.Type == "CallExpression" {
		return e.emitCallStatement(b, expr)
	}
	return unknownShape(stmt, "expr-stmt:"+expr.Type)
}

func (e *emitter) emitPayloadAdd(b *Buf, assign *Node) error {
	// Only $$payload.out += ... is recognised.
	if !isPayloadOut(assign.Left) {
		return unknownShape(assign, "assign-target")
	}
	rhs, err := e.formatExpression(assign.Right)
	if err != nil {
		return err
	}
	b.Line("payload.Push(%s)", rhs)
	return nil
}

func (e *emitter) emitCallStatement(b *Buf, call *Node) error {
	if call.Callee != nil && call.Callee.Type == "MemberExpression" {
		obj := call.Callee.Object
		prop := call.Callee.Property
		if obj != nil && prop != nil && obj.Type == "Identifier" && prop.Type == "Identifier" {
			// $$renderer.push(...)
			if e.rendererName != "" && obj.Name == e.rendererName && prop.Name == "push" {
				return e.emitRendererPush(b, call.Arguments)
			}
			// $$renderer.component(arrowFn)
			if e.rendererName != "" && obj.Name == e.rendererName && prop.Name == "component" {
				return e.emitRendererComponent(b, call.Arguments)
			}
			// $.head(...) and other helper-as-statement entries
			if e.helperNS != "" && obj.Name == e.helperNS {
				return e.emitHelperStatement(b, prop.Name, call.Arguments, call)
			}
		}
	}
	// Bare-identifier call: locally-bound snippet or other lowered
	// closure invocation. Rendered as a Go function-call statement.
	if call.Callee != nil && call.Callee.Type == "Identifier" {
		expr, err := e.formatCall(call)
		if err != nil {
			return err
		}
		b.Line("%s", expr)
		return nil
	}
	return unknownShape(call, "call-stmt")
}

func (e *emitter) emitRendererPush(b *Buf, args []*Node) error {
	if len(args) != 1 {
		return unknownShape(nil, fmt.Sprintf("renderer.push args=%d", len(args)))
	}
	pieces, err := e.formatPushArgument(args[0])
	if err != nil {
		return err
	}
	for _, p := range pieces {
		b.Line("payload.Push(%s)", p)
	}
	return nil
}

// formatPushArgument expands a renderer.push() argument into a list of
// Go expressions to push individually. Splitting per-quasi keeps the
// emitter readable and matches Svelte's interleaved buffer semantics.
func (e *emitter) formatPushArgument(arg *Node) ([]string, error) {
	switch arg.Type {
	case "Literal":
		if arg.LitKind == litString {
			return []string{strconv.Quote(arg.LitStr)}, nil
		}
		return nil, unknownShape(arg, "push:non-string-literal")
	case "TemplateLiteral":
		return e.formatTemplateLiteralPieces(arg)
	}
	// Single expression argument (no template wrap). Stringify it.
	expr, err := e.formatExpression(arg)
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("%s.Stringify(%s)", e.opts.HelperAlias, expr)}, nil
}

func (e *emitter) formatTemplateLiteralPieces(tl *Node) ([]string, error) {
	out := make([]string, 0, len(tl.Quasis)+len(tl.Expressions))
	for i, q := range tl.Quasis {
		if q.Cooked != "" {
			out = append(out, strconv.Quote(q.Cooked))
		}
		if i < len(tl.Expressions) {
			expr, err := e.formatExpression(tl.Expressions[i])
			if err != nil {
				return nil, err
			}
			// Interpolations are already-stringified runtime values
			// (escape_html, attr, etc.) — pass through directly.
			out = append(out, expr)
		}
	}
	return out, nil
}

func (e *emitter) emitRendererComponent(b *Buf, args []*Node) error {
	if len(args) != 1 {
		return unknownShape(nil, fmt.Sprintf("renderer.component args=%d", len(args)))
	}
	fn := args[0]
	if fn.Type != "ArrowFunctionExpression" && fn.Type != "FunctionExpression" {
		return unknownShape(fn, "component:non-fn")
	}
	if fn.FuncBody == nil || fn.FuncBody.Type != "BlockStatement" {
		return unknownShape(fn, "component:body")
	}
	// The component arrow takes ($$renderer); we already track the
	// outer renderer name. Inner bodies push to the same payload, so
	// we descend without scope changes.
	return e.emitBlock(b, fn.FuncBody, false)
}

// emitVarDecl handles the small set of declarations Svelte emits at
// the top of the render function and inside each-loop bodies.
func (e *emitter) emitVarDecl(b *Buf, decl *Node) error {
	for _, d := range decl.Declarations {
		if err := e.emitDeclarator(b, decl.Kind, d); err != nil {
			return err
		}
	}
	return nil
}

func (e *emitter) emitDeclarator(b *Buf, kind string, d *Node) error {
	if d.ID == nil {
		return unknownShape(d, "declarator-no-id")
	}

	// Pattern 1: `let { data } = $$props` — destructure props.
	if d.ID.Type == "ObjectPattern" {
		if d.Init != nil && d.Init.Type == "Identifier" && d.Init.Name == "$$props" {
			return e.emitPropsDestructure(b, d.ID)
		}
		return unknownShape(d, "objpat-non-props")
	}

	if d.ID.Type != "Identifier" {
		return unknownShape(d, "declarator-id:"+d.ID.Type)
	}
	name := mangleIdent(d.ID.Name)

	// Pattern 2: `const each_array = $.ensure_array_like(...)`
	if d.Init != nil && e.isHelperCall(d.Init, "ensure_array_like") {
		inner, err := e.formatExpression(d.Init.Arguments[0])
		if err != nil {
			return err
		}
		e.scope.declare(name, LocalScratch)
		b.Line("%s := %s.EnsureArrayLike(%s)", name, e.opts.HelperAlias, inner)
		return nil
	}

	// Pattern 3: each-iteration item alias
	//   let item = each_array[$$index]
	if d.Init != nil && d.Init.Type == "MemberExpression" && d.Init.Computed {
		obj, err := e.formatExpression(d.Init.Object)
		if err != nil {
			return err
		}
		idx, err := e.formatExpression(d.Init.Property)
		if err != nil {
			return err
		}
		e.scope.declare(name, LocalEach)
		b.Line("%s := %s[%s]", name, obj, idx)
		return nil
	}

	// Pattern 4: counter init `let $$index = 0`
	if d.Init != nil && d.Init.Type == "Literal" && d.Init.LitKind == litNumber {
		e.scope.declare(name, LocalScratch)
		b.Line("%s := %s", name, strconv.FormatInt(int64(d.Init.LitNum), 10))
		return nil
	}

	// Pattern 5: `let $$length = each_array.length`
	if d.Init != nil && d.Init.Type == "MemberExpression" && !d.Init.Computed {
		expr, err := e.formatExpression(d.Init)
		if err != nil {
			return err
		}
		e.scope.declare(name, LocalScratch)
		b.Line("%s := len(%s)", name, e.lengthExpr(d.Init, expr))
		return nil
	}

	// Pattern 6: generic declaration with computable init.
	if d.Init != nil {
		expr, err := e.formatExpression(d.Init)
		if err != nil {
			return err
		}
		e.scope.declare(name, LocalUnknown)
		b.Line("%s := %s", name, expr)
		_ = kind
		return nil
	}

	// var without init — rare; emit a zero-valued any.
	e.scope.declare(name, LocalUnknown)
	b.Line("var %s any", name)
	return nil
}

// lengthExpr converts `x.length` into `len(x)` when the property is
// the JS .length accessor. Falls back to the default expression when
// it isn't.
func (e *emitter) lengthExpr(member *Node, def string) string {
	if member.Property != nil && member.Property.Type == "Identifier" && member.Property.Name == "length" {
		obj, err := e.formatExpression(member.Object)
		if err == nil {
			return obj
		}
	}
	// Strip the trailing ".length" so len() can wrap the base
	// expression. If we can't, return the default — the caller wraps
	// it in len() regardless and the build will fail loudly.
	if i := strings.LastIndex(def, ".length"); i > 0 {
		return def[:i]
	}
	return def
}

func (e *emitter) emitPropsDestructure(b *Buf, pat *Node) error {
	for _, p := range pat.Properties {
		if p.Type != "Property" {
			return unknownShape(p, "prop-pat:"+p.Type)
		}
		if p.Key == nil || p.Key.Type != "Identifier" {
			return unknownShape(p, "prop-key")
		}
		jsName := p.Key.Name
		goName := mangleIdent(jsName)
		// The destructured root variable is what Svelte wraps with
		// $props(); typically "data". Phase 5 hooks this slot via
		// scope.dataVar.
		e.scope.declare(goName, LocalProp)
		if e.scope.dataVar == "" {
			e.scope.dataVar = goName
		}
		b.Line(`%s, _ := props[%q].(map[string]any)`, goName, jsName)
	}
	return nil
}

func (e *emitter) emitIf(b *Buf, stmt *Node) error {
	cond, err := e.formatTruthy(stmt.Test)
	if err != nil {
		return err
	}
	b.Line("if %s {", cond)
	b.In(func() {
		_ = e.emitStatement(b, stmt.Consequent)
	})
	cur := stmt.Alternate
	for cur != nil && cur.Type == "IfStatement" {
		altCond, err := e.formatTruthy(cur.Test)
		if err != nil {
			return err
		}
		b.Line("} else if %s {", altCond)
		b.In(func() {
			_ = e.emitStatement(b, cur.Consequent)
		})
		cur = cur.Alternate
	}
	if cur != nil {
		b.Line("} else {")
		b.In(func() {
			_ = e.emitStatement(b, cur)
		})
	}
	b.Line("}")
	return nil
}

func (e *emitter) emitFor(b *Buf, stmt *Node) error {
	// Svelte's each-array lowering uses
	//   for (let $$index = 0, $$length = arr.length; $$index < $$length; $$index++) { ... }
	// Go's for-init only takes a single SimpleStmt, so we hoist all
	// declarators except the first one into preceding `name :=` lines.
	loopInit := ""
	if stmt.Init != nil {
		switch stmt.Init.Type {
		case "VariableDeclaration":
			decls := stmt.Init.Declarations
			if len(decls) == 0 {
				return unknownShape(stmt.Init, "for-init:empty")
			}
			// Hoist all but the first declarator into preceding lines.
			for _, d := range decls[1:] {
				if err := e.emitForInitDecl(b, d, false); err != nil {
					return err
				}
			}
			s, err := e.formatForInitDecl(decls[0])
			if err != nil {
				return err
			}
			loopInit = s
		default:
			return unknownShape(stmt.Init, "for-init:"+stmt.Init.Type)
		}
	}
	condStr := ""
	if stmt.Test != nil {
		c, err := e.formatExpression(stmt.Test)
		if err != nil {
			return err
		}
		condStr = c
	}
	updateStr := ""
	if stmt.Update != nil {
		u, err := e.formatExpression(stmt.Update)
		if err != nil {
			return err
		}
		updateStr = u
	}
	b.Line("for %s; %s; %s {", loopInit, condStr, updateStr)
	b.In(func() {
		_ = e.emitStatement(b, stmt.FuncBody)
	})
	b.Line("}")
	return nil
}

// emitForInitDecl emits a hoisted declaration line for a for-init
// declarator that doesn't fit in Go's single-statement init slot.
func (e *emitter) emitForInitDecl(b *Buf, d *Node, _ bool) error {
	s, err := e.formatForInitDecl(d)
	if err != nil {
		return err
	}
	b.Line("%s", s)
	return nil
}

// formatForInitDecl renders a for-init declarator as a `name := expr`
// snippet. Special-cases the .length idiom so each_array.length lowers
// to len(each_array).
func (e *emitter) formatForInitDecl(d *Node) (string, error) {
	if d.ID == nil || d.ID.Type != "Identifier" {
		return "", unknownShape(d, "for-init-id")
	}
	name := mangleIdent(d.ID.Name)
	e.scope.declare(name, LocalScratch)
	if d.Init == nil {
		return fmt.Sprintf("var %s int", name), nil
	}
	switch {
	case d.Init.Type == "Literal" && d.Init.LitKind == litNumber:
		return fmt.Sprintf("%s := %d", name, int(d.Init.LitNum)), nil
	case d.Init.Type == "MemberExpression" && d.Init.Property != nil &&
		d.Init.Property.Type == "Identifier" && d.Init.Property.Name == "length":
		inner, err := e.formatExpression(d.Init.Object)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s := len(%s)", name, inner), nil
	}
	expr, err := e.formatExpression(d.Init)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s := %s", name, expr), nil
}

func (e *emitter) emitForOf(b *Buf, stmt *Node) error {
	if stmt.Left == nil || stmt.Left.Type != "VariableDeclaration" || len(stmt.Left.Declarations) != 1 {
		return unknownShape(stmt, "for-of-left")
	}
	d := stmt.Left.Declarations[0]
	if d.ID == nil || d.ID.Type != "Identifier" {
		return unknownShape(d, "for-of-id")
	}
	name := d.ID.Name
	rhs, err := e.formatExpression(stmt.Right)
	if err != nil {
		return err
	}
	e.scope.declare(name, LocalEach)
	b.Line("for _, %s := range %s {", name, rhs)
	b.In(func() {
		_ = e.emitStatement(b, stmt.FuncBody)
	})
	b.Line("}")
	return nil
}

// formatTruthy wraps a JS condition in the JS-truthy semantics Svelte
// generated. For the common boolean and comparison cases we lower
// directly; otherwise we fall back to the runtime IsTruthy helper.
// Note IsTruthy isn't part of the Phase-4 helpers package (#426
// limits the surface area); for now we emit pessimistic bool-only
// guards and surface anything unfamiliar as unknown.
func (e *emitter) formatTruthy(test *Node) (string, error) {
	if test == nil {
		return "true", nil
	}
	switch test.Type {
	case "Identifier":
		return e.formatExpression(test)
	case "BinaryExpression", "LogicalExpression", "UnaryExpression":
		return e.formatExpression(test)
	case "MemberExpression":
		return e.formatExpression(test)
	case "Literal":
		return e.formatExpression(test)
	case "CallExpression":
		return e.formatExpression(test)
	}
	return "", unknownShape(test, "truthy:"+test.Type)
}

// helperHandler maps a `$.helper(args...)` call (in expression position
// or as a statement) into the corresponding Go expression / statement.
type helperHandler func(e *emitter, b *Buf, args []*Node, asExpr bool, call *Node) (string, error)

func (e *emitter) emitHelperStatement(b *Buf, name string, args []*Node, call *Node) error {
	h, ok := e.helpers[name]
	if !ok {
		return unknownShape(call, "helper:"+name)
	}
	_, err := h(e, b, args, false, call)
	return err
}

// formatHelperCall is the expression-position dispatch.
func (e *emitter) formatHelperCall(name string, args []*Node, call *Node) (string, error) {
	h, ok := e.helpers[name]
	if !ok {
		return "", unknownShape(call, "helper:"+name)
	}
	return h(e, nil, args, true, call)
}

// isHelperCall reports whether n is `$.<name>(...)`.
func (e *emitter) isHelperCall(n *Node, name string) bool {
	if n == nil || n.Type != "CallExpression" {
		return false
	}
	if n.Callee == nil || n.Callee.Type != "MemberExpression" {
		return false
	}
	obj := n.Callee.Object
	prop := n.Callee.Property
	if obj == nil || prop == nil {
		return false
	}
	if obj.Type != "Identifier" || obj.Name != e.helperNS {
		return false
	}
	if prop.Type != "Identifier" || prop.Name != name {
		return false
	}
	return true
}

func isPayloadOut(n *Node) bool {
	if n == nil || n.Type != "MemberExpression" {
		return false
	}
	if n.Object == nil || n.Property == nil {
		return false
	}
	if n.Object.Type != "Identifier" || n.Object.Name != "$$payload" {
		return false
	}
	if n.Property.Type != "Identifier" || n.Property.Name != "out" {
		return false
	}
	return true
}

func typeOf(n *Node) string {
	if n == nil {
		return "<nil>"
	}
	return n.Type
}

func litStr(n *Node) string {
	if n == nil {
		return ""
	}
	return n.LitStr
}

// jsonUnmarshal is split out so tests can stub when needed; for now
// it's a thin alias for encoding/json.Unmarshal.
func jsonUnmarshal(b []byte, v any) error {
	return jsonUnmarshalReal(b, v)
}
