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

	// TypedDataParam, when non-empty, switches the generated render
	// function signature to `func Render(payload *<server>.Payload, data
	// <TypedDataParam>)` and skips the `props["data"]` map cast that
	// Phase 3 emits for the destructured root. Phase 6 (#428) sets this
	// to the route's PageData / LayoutData type so Phase 5's lowered
	// `data.User.Name` access lands on a typed Go struct field rather
	// than `map[string]any`.
	//
	// Behavior when set:
	//   - The emitted function signature uses `data <TypedDataParam>`
	//     in place of `props map[string]any`.
	//   - The walker still records the destructured prop name (so the
	//     scope's data root is "data") but does not emit the
	//     `props["data"]` map cast.
	//
	// Empty string preserves the legacy `props map[string]any` shape so
	// the existing 30-shape priority goldens remain byte-identical.
	TypedDataParam string

	// EmitChildrenParam, when true, appends a `children
	// func(*<server>.Payload)` parameter to the generated render
	// function. Issue #440 (children-callback ABI) sets this for layout
	// SSR emit so the manifest can compose the layout chain through
	// payload-shaped callbacks. The emitter additionally registers
	// `children` as a [LocalCallback] in the render-function scope so
	// Phase 5's [Lowerer] does not rewrite `children(...)` invocations
	// through the JSON-tag map.
	//
	// Defaults to false — page Render() emits stay byte-identical for
	// the 30 priority + 50+ extended goldens that pre-date this issue.
	EmitChildrenParam bool

	// EmitPageStateParam, when true, appends a `pageState
	// <server>.PageState` parameter to the generated render function.
	// Issue #466 ($app/state lowering) sets this for SSR emit so the
	// manifest's per-route bridge can pass URL, params, route, status,
	// form, etc. into Render. The emitter additionally pre-declares
	// `page`, `navigating`, and `updated` in the render-function scope
	// as [LocalAppState] so [Lowerer] rewrites `page.<field>` chains
	// against the static framework field map instead of the typegen
	// Shape (which only knows user-data).
	//
	// Defaults to false — non-SSR call sites (legacy goldens, the
	// tests that drive the priority + extended corpora directly) keep
	// their existing parameter list.
	EmitPageStateParam bool
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
	// LocalCallback marks a callback prop the layout receives from the
	// outer render call (e.g. `children` for {@render children()} or a
	// named-snippet prop). Phase 5 leaves these alone because they
	// dispatch to a Go func value, not a user-data subtree.
	LocalCallback
	// LocalAppState marks a `$app/state` rune root (`page`,
	// `navigating`, `updated`). Issue #466. Phase 5 lowers chains rooted
	// at these names against a static framework field map instead of
	// the user-data typegen Shape — they're framework-defined surfaces,
	// not destructured props.
	LocalAppState
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
	out.Line("%s", e.renderSignature())
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

// renderSignature builds the generated Render function header line.
// The parameter list is composed positionally from the active option
// flags so callers can mix data-typing, children-callback, and
// page-state plumbing without an N-way switch.
//
// Order is fixed for ABI stability across emits:
//
//	payload *<server>.Payload
//	data <PageData> | props map[string]any
//	children func(*<server>.Payload)        // EmitChildrenParam
//	pageState <server>.PageState             // EmitPageStateParam
func (e *emitter) renderSignature() string {
	params := []string{fmt.Sprintf("payload *%s.Payload", e.opts.HelperAlias)}
	if e.opts.TypedDataParam != "" {
		params = append(params, "data "+e.opts.TypedDataParam)
	} else {
		params = append(params, "props map[string]any")
	}
	if e.opts.EmitChildrenParam {
		params = append(params, "children func(*"+e.opts.HelperAlias+".Payload)")
	}
	if e.opts.EmitPageStateParam {
		params = append(params, "pageState "+e.opts.HelperAlias+".PageState")
	}
	return "func " + e.opts.FuncName + "(" + strings.Join(params, ", ") + ") {"
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

// appStateRoots is the closed set of identifiers `$app/state` exports.
// Specifiers outside this set are an unknown shape — Svelte's compiled
// server output never re-exports anything else, so a stray name almost
// certainly means the pinned Svelte minor moved or the user's import
// list has a typo the build should reject.
var appStateRoots = map[string]struct{}{
	"page":       {},
	"navigating": {},
	"updated":    {},
}

// appNavigationRoots is the closed set of identifiers `$app/navigation`
// exports. v1 ($app/state lowering, #466) accepts the import but does
// not lower call sites — these are client-only APIs (goto, invalidate,
// pushState, …). Server-side function declarations referring to them
// stay declarable; render-body call sites surface as unknownShape via
// the bare-call dispatch.
var appNavigationRoots = map[string]struct{}{
	"goto":                    {},
	"invalidate":              {},
	"invalidateAll":           {},
	"preloadCode":             {},
	"preloadData":             {},
	"pushState":               {},
	"replaceState":            {},
	"disableScrollHandling":   {},
	"afterNavigate":           {},
	"beforeNavigate":          {},
	"onNavigate":              {},
	"setStaticAssetMimeTypes": {},
}

func (e *emitter) recordImport(decl *Node) error {
	if decl.Source == nil {
		return unknownShape(decl, "import:"+litStr(decl.Source))
	}
	switch decl.Source.LitStr {
	case "svelte/internal/server":
		return e.recordHelperImport(decl)
	case "$app/state":
		return e.recordAppStateImport(decl)
	case "$app/navigation":
		return e.recordAppNavigationImport(decl)
	default:
		return unknownShape(decl, "import:"+litStr(decl.Source))
	}
}

// recordHelperImport handles the `import * as $ from
// "svelte/internal/server"` shape Svelte's compiled server output emits
// once per render module. The local namespace identifier is captured so
// the helper-call dispatch in patterns.go can recognise `$.foo(...)`
// invocations against the right alias.
func (e *emitter) recordHelperImport(decl *Node) error {
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

// recordAppStateImport recognises `import { page, navigating, updated }
// from "$app/state"` (any subset). Each accepted specifier is
// pre-registered in the render-function scope as [LocalAppState] when
// EmitPageStateParam is set so [Lowerer] rewrites chains rooted at
// these names against the static framework field map (see
// lowering.go's lowerAppStateChain).
//
// When EmitPageStateParam is not set the import is still accepted but
// the names are not registered; the lowerer's strict-mode "unknown
// root" error then surfaces if the route actually reads the runes,
// pointing the user at the SSR fallback annotation. This keeps
// existing test sites that drive Transpile directly from breaking.
func (e *emitter) recordAppStateImport(decl *Node) error {
	for _, sp := range decl.Specifiers {
		if sp.Type != "ImportSpecifier" {
			return unknownShape(sp, "import-spec:"+sp.Type)
		}
		if sp.Local == nil || sp.Local.Type != "Identifier" {
			return unknownShape(sp, "import-spec-local")
		}
		imported := importedName(sp)
		if _, ok := appStateRoots[imported]; !ok {
			return unknownShape(sp, "import:$app/state:"+imported)
		}
		if e.opts.EmitPageStateParam {
			e.scope.declare(sp.Local.Name, LocalAppState)
		}
	}
	return nil
}

// recordAppNavigationImport recognises imports from `$app/navigation`.
// The names are accepted (not unknownShape) but tracked as
// LocalCallback so any render-body call site lowers through the bare-
// identifier dispatch as `name(...)`. v1 of the appstate lowering
// (#466) does not provide Go-side implementations for these; routes
// that actually invoke them at render time surface a build error
// downstream when the call expression evaluates the un-emitted
// identifier. Function-declaration bodies (event handlers) that
// reference them are fine because they are not invoked from Render.
func (e *emitter) recordAppNavigationImport(decl *Node) error {
	for _, sp := range decl.Specifiers {
		if sp.Type != "ImportSpecifier" {
			return unknownShape(sp, "import-spec:"+sp.Type)
		}
		if sp.Local == nil || sp.Local.Type != "Identifier" {
			return unknownShape(sp, "import-spec-local")
		}
		imported := importedName(sp)
		if _, ok := appNavigationRoots[imported]; !ok {
			return unknownShape(sp, "import:$app/navigation:"+imported)
		}
		// LocalScratch keeps the Lowerer from rewriting the bare name
		// through the data-root chain walker. It does NOT promise the
		// identifier resolves to anything in the emitted Go file —
		// references at render time will fail to compile, surfacing the
		// gap loudly at build time.
		e.scope.declare(sp.Local.Name, LocalScratch)
	}
	return nil
}

// importedName returns the source-side name for an ImportSpecifier
// (the part before `as` in `import { x as y }`). Falls back to the
// local name when imported is missing — non-aliased shorthand sets
// only the local name in some Acorn shapes.
func importedName(sp *Node) string {
	if sp.Imported != nil && sp.Imported.Type == "Identifier" {
		return sp.Imported.Name
	}
	if sp.Local != nil && sp.Local.Type == "Identifier" {
		return sp.Local.Name
	}
	return ""
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
	if e.opts.EmitChildrenParam {
		// Issue #440: register the `children` parameter as a callback
		// local so the lowerer leaves invocations alone. The destructured
		// prop binding (handled in emitPropsDestructure) reclassifies it
		// when `children` shows up in the prop pattern; declaring up
		// front covers the bare-callable shape too.
		e.scope.declare("children", LocalCallback)
	}
	return e.emitBlock(b, fn.FuncBody, true)
}

// emitBlock walks a BlockStatement. When isRoot the renderer is the
// outer one; nested blocks (slot fragments, control flow arms) inherit
// scope but reuse the same renderer reference.
//
// Issue #443 (snippet hoisting): when a snippet declaration —
// `const <name> = ($$renderer, …) => { … }` — appears *after* a call
// to that name in the same block, we lift the declaration above its
// first call site so Go's declare-before-use rule for `:=` is
// satisfied. When source order is already declare-before-use the
// hoist is a no-op so the 80+ Phase 7 corpus goldens stay
// byte-identical.
func (e *emitter) emitBlock(b *Buf, block *Node, isRoot bool) error {
	body := hoistForwardSnippets(block.Body)
	for _, stmt := range body {
		if err := e.emitStatement(b, stmt); err != nil {
			return err
		}
	}
	_ = isRoot
	return nil
}

// hoistForwardSnippets reorders block statements to satisfy Go's
// declare-before-use rule for snippet bindings. Walks the body left
// to right tracking `seenCalls`, the set of bare-identifier callees
// that have appeared so far. When a snippet declaration's name is
// already in seenCalls, the declaration is moved before the first
// statement that called it; otherwise statements stay in source
// order. Issue #443.
func hoistForwardSnippets(stmts []*Node) []*Node {
	if len(stmts) == 0 {
		return stmts
	}
	// Index of the first statement that calls each candidate snippet.
	firstCall := map[string]int{}
	for i, s := range stmts {
		for name := range collectBareCallees(s) {
			if _, ok := firstCall[name]; !ok {
				firstCall[name] = i
			}
		}
	}
	if len(firstCall) == 0 {
		return stmts
	}
	out := make([]*Node, len(stmts))
	copy(out, stmts)
	for i := 0; i < len(out); i++ {
		s := out[i]
		name, ok := snippetDeclName(s)
		if !ok {
			continue
		}
		ci, ok := firstCall[name]
		if !ok || ci >= i {
			continue
		}
		// Move out[i] to position ci, shifting [ci..i-1] one slot right.
		decl := out[i]
		copy(out[ci+1:i+1], out[ci:i])
		out[ci] = decl
		// Re-scan firstCall — relative shift may have changed indices.
		// Cheaper to recompute than to track per-name deltas because
		// blocks rarely carry more than a handful of statements.
		firstCall = map[string]int{}
		for j, ss := range out {
			for n := range collectBareCallees(ss) {
				if _, ok := firstCall[n]; !ok {
					firstCall[n] = j
				}
			}
		}
	}
	return out
}

// snippetDeclName returns the declared name when s is a single-
// declarator variable declaration with an arrow / function init —
// the structural shape Svelte 5 lowers `{#snippet}` into.
func snippetDeclName(s *Node) (string, bool) {
	if s == nil || s.Type != "VariableDeclaration" || len(s.Declarations) != 1 {
		return "", false
	}
	d := s.Declarations[0]
	if d == nil || d.ID == nil || d.ID.Type != "Identifier" || d.Init == nil {
		return "", false
	}
	if d.Init.Type != "ArrowFunctionExpression" && d.Init.Type != "FunctionExpression" {
		return "", false
	}
	return d.ID.Name, true
}

// collectBareCallees walks a statement collecting the names of every
// `<ident>(...)` call site (statement or expression position) where
// the callee is a bare identifier — the shape `{@render name(...)}`
// lowers into. Member calls (`obj.fn()`), helper-namespace calls
// (`$.x(...)`), and renderer dispatch (`$$renderer.push(...)`) are
// skipped because none of those refer to a snippet binding.
func collectBareCallees(n *Node) map[string]struct{} {
	out := map[string]struct{}{}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type == "CallExpression" && n.Callee != nil && n.Callee.Type == "Identifier" {
			out[n.Callee.Name] = struct{}{}
		}
		walk(n.Expression)
		walk(n.Callee)
		walk(n.Object)
		walk(n.Property)
		walk(n.Argument)
		walk(n.Left)
		walk(n.Right)
		walk(n.Test)
		walk(n.Consequent)
		walk(n.Alternate)
		walk(n.Init)
		walk(n.Update)
		walk(n.FuncBody)
		walk(n.ID)
		walk(n.Source)
		walk(n.Declaration)
		for _, c := range n.Body {
			walk(c)
		}
		for _, c := range n.Arguments {
			walk(c)
		}
		for _, c := range n.Params {
			walk(c)
		}
		for _, c := range n.Declarations {
			walk(c)
		}
		for _, c := range n.Properties {
			walk(c)
		}
		for _, c := range n.Specifiers {
			walk(c)
		}
		for _, c := range n.Quasis {
			walk(c)
		}
		for _, c := range n.Expressions {
			walk(c)
		}
	}
	walk(n)
	return out
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
	case "FunctionDeclaration":
		// User-authored `<script>` helpers (DOM event handlers,
		// computed-value functions, etc.) appear as nested
		// FunctionDeclarations in Svelte's server output. The
		// server-mode compiler never *calls* them — DOM event
		// handlers wire up client-side at hydration; the SSR HTML
		// just emits the static markup. Treat the declaration as a
		// no-op so layouts and pages with `<script>` helpers
		// transpile cleanly. If the declaration is later referenced
		// from a render-body expression the unknown-shape dispatch
		// will surface that on the call site, not here.
		return nil
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
			// $$props.children($$renderer) — issue #440 children-callback
			// ABI. Svelte's compiled output for {@render children()}
			// dispatches through the props bag when the layout did not
			// destructure `children`. Emit the same nil-guarded callback
			// invocation the destructured shape lowers to.
			if e.opts.EmitChildrenParam && obj.Name == "$$props" {
				return e.emitChildrenCallback(b, prop.Name, call.Arguments, call)
			}
			// $.head(...) and other helper-as-statement entries
			if e.helperNS != "" && obj.Name == e.helperNS {
				return e.emitHelperStatement(b, prop.Name, call.Arguments, call)
			}
		}
	}
	// Bare-identifier call: locally-bound snippet, callback prop, or
	// other lowered closure invocation. Rendered as a Go function-call
	// statement; LocalCallback identifiers (e.g. `children` after
	// destructure) get a nil guard since callback props are optional.
	if call.Callee != nil && call.Callee.Type == "Identifier" {
		if e.opts.EmitChildrenParam && e.scope.Lookup(call.Callee.Name) == LocalCallback {
			return e.emitChildrenCallback(b, call.Callee.Name, call.Arguments, call)
		}
		expr, err := e.formatCall(call)
		if err != nil {
			return err
		}
		b.Line("%s", expr)
		return nil
	}
	return unknownShape(call, "call-stmt")
}

// emitChildrenCallback lowers an invocation of the layout's children
// (or named-snippet) callback prop. The compiled-server output passes
// the renderer (`$$payload` after rename) as the sole argument; we
// translate that to the Go-side `payload` parameter and guard against
// nil because callback props are optional. Issue #440.
func (e *emitter) emitChildrenCallback(b *Buf, name string, args []*Node, call *Node) error {
	if len(args) > 1 {
		return unknownShape(call, fmt.Sprintf("children-callback args=%d", len(args)))
	}
	// The compiled output passes either the outer renderer (`$$renderer`
	// → `payload` on the Go side) or no argument at all. Either way the
	// Go-side bridge takes a single `*server.Payload`.
	b.Line("if %s != nil {", name)
	b.In(func() {
		b.Line("%s(payload)", name)
	})
	b.Line("}")
	return nil
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

	// Pattern 6: generic declaration with computable init. Arrow /
	// function expressions land as snippets (issue #443) so the lowerer
	// can refuse to rewrite their bodies through the JSON-tag map and
	// callers see them as closure values.
	if d.Init != nil {
		kindLocal := LocalUnknown
		if d.Init.Type == "ArrowFunctionExpression" || d.Init.Type == "FunctionExpression" {
			kindLocal = LocalSnippet
			// Pre-register the snippet name BEFORE walking its body so
			// the body can recurse into a self-reference (rare but
			// emitted by Svelte for recursive snippet shapes).
			e.scope.declare(name, kindLocal)
		}
		expr, err := e.formatExpression(d.Init)
		if err != nil {
			return err
		}
		if kindLocal != LocalSnippet {
			e.scope.declare(name, kindLocal)
		}
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
		// Issue #440 children-callback ABI: when the layout destructures
		// `children` from $$props the function signature already binds
		// it as a typed `func(*server.Payload)` parameter. Reclassify
		// the local so Phase 5 leaves callback invocations alone, and
		// skip the map cast that would shadow the parameter.
		if e.opts.EmitChildrenParam && jsName == "children" {
			e.scope.declare(goName, LocalCallback)
			continue
		}
		// The destructured root variable is what Svelte wraps with
		// $props(); typically "data". Phase 5 hooks this slot via
		// scope.dataVar.
		e.scope.declare(goName, LocalProp)
		if e.scope.dataVar == "" {
			e.scope.dataVar = goName
		}
		// Typed-data mode (Phase 6, #428): the function signature
		// already binds `data` as a typed parameter, so emitting a
		// map-cast here would shadow it with map[string]any and break
		// the Lowerer's typed field-access output. Skip the cast for
		// the data root only — auxiliary destructured props still need
		// the map fallback because no typed parameter exists for them.
		if e.opts.TypedDataParam != "" && jsName == "data" {
			continue
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
// generated. Issue #443 wires the runtime `server.Truthy` helper in
// for non-bool-shaped expressions (bare identifier, member-access
// chain, function call) so `{@const x = data.name}` followed by
// `{#if x}` lowers to `if server.Truthy(x) {` rather than the
// non-compiling `if x {` against a typed string field.
//
// Already-bool-shaped expressions — comparisons, logical operators
// over bool-shaped operands, `!expr` — pass through directly so the
// 80+ Phase 7 corpus shapes that already lean on bool-typed conditions
// stay byte-identical.
func (e *emitter) formatTruthy(test *Node) (string, error) {
	if test == nil {
		return "true", nil
	}
	expr, err := e.formatExpression(test)
	if err != nil {
		return "", err
	}
	if isBoolShaped(test) {
		return expr, nil
	}
	switch test.Type {
	case "Identifier", "MemberExpression", "CallExpression", "ConditionalExpression":
		return fmt.Sprintf("%s.Truthy(%s)", e.opts.HelperAlias, expr), nil
	case "Literal":
		// Pure literal — Go accepts string/numeric/bool literals as
		// conditions only when bool, so route through the helper to
		// keep the emit consistent with the typed-data path.
		if test.LitKind == litBool {
			return expr, nil
		}
		return fmt.Sprintf("%s.Truthy(%s)", e.opts.HelperAlias, expr), nil
	case "BinaryExpression", "LogicalExpression", "UnaryExpression":
		// isBoolShaped returned false for these — operand mix is not
		// statically bool. Wrap the whole expression rather than each
		// operand: simpler to read, semantics match JS.
		return fmt.Sprintf("%s.Truthy(%s)", e.opts.HelperAlias, expr), nil
	}
	return "", unknownShape(test, "truthy:"+test.Type)
}

// isBoolShaped reports whether an ESTree expression is statically
// bool-typed in Go after lowering. Used by formatTruthy to skip the
// runtime Truthy() wrap for conditions Svelte already emits in
// already-bool form. The check is structural — not type-aware — but
// catches the dominant compiler-emitted shapes:
//
//   - comparison binary ops (==, !=, <, >, <=, >=)
//   - logical ops (&&, ||) when both operands are bool-shaped
//   - logical-not unary (!)
//   - bool literals
//
// Anything else (bare identifier, member access, function call,
// ternary) is conservatively treated as non-bool and routed through
// server.Truthy.
func isBoolShaped(n *Node) bool {
	if n == nil {
		return false
	}
	switch n.Type {
	case "BinaryExpression":
		switch n.Operator {
		case "==", "!=", "===", "!==", "<", ">", "<=", ">=":
			return true
		}
		return false
	case "LogicalExpression":
		if n.Operator == "&&" || n.Operator == "||" {
			return isBoolShaped(n.Left) && isBoolShaped(n.Right)
		}
		return false
	case "UnaryExpression":
		return n.Operator == "!"
	case "Literal":
		return n.LitKind == litBool
	case "ChainExpression":
		return isBoolShaped(n.Expression)
	}
	return false
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
