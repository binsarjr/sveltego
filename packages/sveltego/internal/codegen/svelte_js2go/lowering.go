package sveltejs2go

import (
	"errors"
	"fmt"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
)

// Lowerer is the Phase 5 (#427) [ExprRewriter] that walks compiled
// Svelte server output and rewrites JS-style member chains
// (`data.user.name`) into Go-style chains (`data.User.Name`) using
// the per-route JSON tag map produced by typegen.
//
// The rewriter is scope-aware: identifiers introduced by {#each},
// {@const}, {#snippet}, function parameters, and the for-init locals
// emitted by Svelte 5's each-array lowering are NOT rewritten. The
// data root (whatever name the user destructured from $$props in
// their Svelte component) IS rewritten — its chain is walked through
// the JSON tag map; missing segments produce a hard build error.
//
// Lowerer accumulates errors instead of returning them through the
// [ExprRewriter] interface — the rewriter must hand back a string,
// and corrupting it would break gofmt downstream. After Transpile
// returns, callers MUST call Err() and surface the error verbatim.
//
// Strict mode (the default) hard-errors on:
//   - Computed access (`data["x"]`).
//   - Member-access roots that are neither in scope nor the data root.
//   - Object-spread placeholders that the emitter could not resolve to
//     a typed map shape.
//   - JSON tags absent from the route's PageData / LayoutData.
type Lowerer struct {
	route  string
	shape  *typegen.Shape
	errs   []error
	strict bool
}

// LowererOptions configures [NewLowerer].
type LowererOptions struct {
	// Route is the route the AST belongs to; baked into hard-error
	// messages alongside the AST byte offset so build operators can
	// trace failures back to the offending .svelte file.
	Route string

	// Strict, when true, surfaces missing JSON tags / unknown roots /
	// computed access as hard errors. When false the rewriter silently
	// passes through the default rendering — useful for the existing
	// Phase 3 priority-shape goldens that don't carry a typegen Shape.
	Strict bool
}

// NewLowerer constructs a Lowerer bound to a typegen Shape. shape may
// be nil when the caller does not have route-specific type
// information yet (Phase 6 wiring); in that mode the rewriter is a
// no-op for member chains and Strict is forced to false.
func NewLowerer(shape *typegen.Shape, opts LowererOptions) *Lowerer {
	strict := opts.Strict
	if shape == nil {
		strict = false
	}
	return &Lowerer{
		route:  opts.Route,
		shape:  shape,
		strict: strict,
	}
}

// Err returns the accumulated lowering error, or nil when the
// rewriter visited every node successfully. Multiple errors are
// joined with errors.Join so callers can format each on its own line.
func (l *Lowerer) Err() error {
	if len(l.errs) == 0 {
		return nil
	}
	return errors.Join(l.errs...)
}

// Rewrite implements [ExprRewriter]. It dispatches on node type:
//
//   - Identifier: leave the default rendering alone — identifier
//     lowering happens implicitly via MemberExpression, which sees the
//     full chain root.
//   - MemberExpression: walk the full chain, decide whether the root
//     is a local binding (leave alone), the data root (lower via
//     JSON tag map), or unknown (hard error in strict mode).
//
// Returning the empty string tells the emitter to use the default
// (mangleIdent + dotted concatenation) rendering — that's the no-op
// path for in-scope locals.
func (l *Lowerer) Rewrite(scope *Scope, n *Node, def string) string {
	if l == nil || n == nil {
		return ""
	}
	switch n.Type {
	case "Identifier":
		// Identifier rewriting is unnecessary: when the identifier is
		// the data root and lives at the leaf of a chain, the parent
		// MemberExpression call handles it. When the identifier is a
		// local in scope or a literal, the default rendering is right.
		return ""
	case "MemberExpression":
		return l.rewriteMember(scope, n, def)
	}
	return ""
}

// rewriteMember performs the chain walk described in issue #427's
// detailed design. The walk runs in two passes:
//
//  1. flattenChain — collect the chain segments from leaf back to
//     root, recording whether any link is computed or optional.
//  2. resolve — start at the root, classify it (local, data root,
//     unknown), and either return the default (locals) or rewrite the
//     chain by walking the typegen Shape (data root).
//
// Hard-error paths return the default string (so emitter still
// produces a Go file — gofmt is free to fail downstream); the build
// driver should consult Err() before trusting the output.
func (l *Lowerer) rewriteMember(scope *Scope, n *Node, def string) string {
	chain, root, ok := flattenChain(n)
	if !ok {
		// Computed access (`data["x"]`) or non-Identifier root.
		// Strict mode rejects; relaxed mode passes through.
		if l.strict {
			l.recordComputedAccess(n, def)
		}
		return ""
	}

	// $app/state runes (`page`, `navigating`, `updated`) lower against
	// the static framework field map — they're not user-data. Issue
	// #466. Checked before the local-skip branch so the LocalAppState
	// scope kind doesn't collapse into the generic local-skip path.
	if scope != nil && scope.Lookup(root.Name) == LocalAppState {
		return l.lowerAppStateChain(root, chain, n, def)
	}

	// Locals (each-loop alias, snippet param, @const, function param,
	// hoisted for-init scratch) skip lowering. Their default rendering
	// (mangled identifier dotted into raw JS member names) is what we
	// want — the body code references them as JS-side names. The
	// LocalProp kind (the destructured props root, typically `data`)
	// is NOT a skip case: that's the chain entry point Phase 5 lowers
	// against the JSON tag map.
	if scope != nil && scope.Lookup(root.Name) != LocalProp && hasLocal(scope, root.Name) {
		return ""
	}

	// Data root: lower the chain via the typegen Shape.
	if scope != nil && scope.IsDataRoot(root.Name) {
		return l.lowerDataChain(scope, root, chain, n, def)
	}

	// Some emitter-internal names (e.g. ssvar_payload) appear without
	// being declared via the scope tracker — they originate from
	// helper closures that take their parameter as an Identifier.
	// Treat those as locals too.
	if isEmitterScratch(root.Name) {
		return ""
	}

	// Unknown root — strict mode hard-errors with a suggestion to
	// either declare the data field or annotate the route for the
	// Phase 8 sidecar fallback.
	if l.strict {
		l.recordUnknownRoot(root, n, def)
	}
	return ""
}

// chainSegment captures one member access in the JS chain. computed
// segments correspond to bracket access (`x[expr]`) and force a hard
// error in strict mode; optional segments come from `?.` and signal
// that the lowered Go expression must guard against nil at this link.
type chainSegment struct {
	name     string
	computed bool
	optional bool
	node     *Node
}

// flattenChain walks back from n through MemberExpression links until
// it hits a non-member root. Returns the segments leaf-first reversed
// to root-first, the root identifier node, and ok = true when every
// segment is a non-computed Identifier property. ChainExpression
// wrappers are unwrapped transparently — Acorn emits those for `?.`
// chains and the inner MemberExpression carries the actual links.
func flattenChain(n *Node) ([]chainSegment, *Node, bool) {
	var segs []chainSegment
	cur := n
	for cur != nil && cur.Type == "MemberExpression" {
		seg := chainSegment{
			computed: cur.Computed,
			optional: cur.Optional,
			node:     cur,
		}
		if cur.Computed {
			// Bracket access: name stays empty; downstream rejects.
			segs = append(segs, seg)
			cur = cur.Object
			continue
		}
		if cur.Property == nil || cur.Property.Type != "Identifier" {
			return nil, nil, false
		}
		seg.name = cur.Property.Name
		segs = append(segs, seg)
		cur = cur.Object
	}
	if cur == nil || cur.Type != "Identifier" {
		return nil, nil, false
	}
	// Reverse leaf-first to root-first.
	for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
		segs[i], segs[j] = segs[j], segs[i]
	}
	for _, s := range segs {
		if s.computed {
			return nil, nil, false
		}
	}
	return segs, cur, true
}

// lowerDataChain walks the typegen Shape from the route's RootType
// down through each segment, replacing JSON tags with Go field names.
// Optional segments lower into a runtime helper call so deep chains
// stay readable; for shallow single-link optional chains we emit an
// inline guard.
func (l *Lowerer) lowerDataChain(_ *Scope, root *Node, segs []chainSegment, n *Node, def string) string {
	if l.shape == nil || l.shape.RootType == "" {
		// No type info — leave default rendering. Strict mode is
		// disabled when shape is nil (NewLowerer enforces this).
		return ""
	}

	// Detect optional chaining on any segment. Lowering picks one of
	// two strategies based on chain depth: a single `?.` lowers into
	// an inline guard, deeper chains route through a reflective
	// helper to keep generated code readable.
	hasOptional := false
	for _, s := range segs {
		if s.optional {
			hasOptional = true
			break
		}
	}

	// Walk the type chain. tracker.NamedType carries the current
	// position in the shape graph; primitives or unknown leaves stop
	// the walk early but still allow trailing JS-name fallbacks (used
	// by `data.someUnknown.toString()` shapes — the lowerer doesn't
	// know JS prototype methods, so the unrecognised tail surfaces as
	// an error in strict mode).
	currentType := l.shape.RootType
	parts := []string{root.Name}
	for i, s := range segs {
		st, ok := l.shape.Types[currentType]
		if !ok {
			if l.strict {
				l.recordUnknownRoot(root, n, def)
			}
			return ""
		}
		field, found := st.Lookup(s.name)
		if !found {
			if l.strict {
				l.recordMissingField(currentType, s.name, n, def)
			}
			return ""
		}
		goName := field.GoName
		if goName == "" {
			// Defensive — typegen's walker always sets GoName for
			// exported fields. If somehow blank, fall back to the JSON
			// tag's title-cased form.
			goName = titleCase(s.name)
		}
		parts = append(parts, goName)
		// Advance the cursor to the next nested type. Slices end the
		// chain (further dots would mean indexing, which Svelte's
		// compiled output does not emit at this layer).
		switch {
		case field.Slice:
			if i < len(segs)-1 {
				if l.strict {
					l.recordTraverseThroughSlice(field, n, def)
				}
				return ""
			}
		case field.NamedType != "":
			currentType = field.NamedType
		default:
			// Primitive / map / generic leaf — any further chain link
			// has nowhere to go. Strict mode rejects.
			if i < len(segs)-1 {
				if l.strict {
					l.recordTraversePrimitive(field, n, def)
				}
				return ""
			}
		}
	}

	if hasOptional {
		return l.lowerOptionalChain(parts, segs)
	}
	return strings.Join(parts, ".")
}

// appStateField describes one segment in a `page.*` /
// `navigating.*` / `updated.*` chain after lowering. emit is the Go
// expression to splice in for the *full prefix up to and including
// this segment*; computed indicates the segment uses bracket access on
// the Go side (e.g. `Params["id"]` for `page.params.id`); terminal is
// true when no further chain segment is allowed (helper-form fields
// like `URL.String()` cannot be dotted into).
type appStateField struct {
	emit     string
	computed bool
	terminal bool
}

// lowerAppStateChain rewrites a chain rooted at one of the
// `$app/state` runes (`page`, `navigating`, `updated`) against the
// static framework field map. Issue #466. The map is hand-coded
// because these structures are framework-defined; the typegen Shape
// only knows user-data.
//
// Returned string is the full Go expression (including the root
// `pageState` segment); an empty return signals the lowerer recorded
// an error and the emitter should fall back to the default rendering
// (which will fail to compile, surfacing the gap loudly at build time).
func (l *Lowerer) lowerAppStateChain(root *Node, segs []chainSegment, n *Node, def string) string {
	// All three roots feed off pageState — the framework param the
	// emitter binds when EmitPageStateParam is set.
	prefix := "pageState"
	switch root.Name {
	case "page":
		// fall through
	case "navigating":
		// `navigating.<x>` chain rebases on pageState.Navigating; the
		// only meaningful field is `current` which collapses to the
		// pageState's Navigating pointer (always nil server-side).
		return l.lowerNavigatingChain(prefix, segs, n, def)
	case "updated":
		return l.lowerUpdatedChain(prefix, segs, n, def)
	default:
		// Should not be reachable because LocalAppState is only
		// declared for the three roots.
		if l.strict {
			l.recordUnknownRoot(root, n, def)
		}
		return ""
	}

	// `page` chain: walk the static field map.
	current := prefix
	for i, seg := range segs {
		f, ok := pageFieldMap(seg.name, current)
		if !ok {
			if l.strict {
				l.recordUnknownAppStateField("page", seg.name, n, def)
			}
			return ""
		}
		current = f.emit
		if f.terminal && i < len(segs)-1 {
			if l.strict {
				l.recordTerminalAppStateField("page."+seg.name, n, def)
			}
			return ""
		}
	}
	if hasAnyOptional(segs) {
		return l.guardAppStateOptional(prefix, segs, current)
	}
	return current
}

// lowerNavigatingChain handles `navigating.current` and any sub-chain
// rooted under it. Server-side the Navigating pointer is always nil;
// the chain lowers to `pageState.Navigating` so `{#if navigating.current}`
// branches resolve the truthy-check against the pointer. Sub-fields
// of a non-nil Navigation (`type`, `from`, `to`) lower against the
// typed struct.
func (l *Lowerer) lowerNavigatingChain(prefix string, segs []chainSegment, n *Node, def string) string {
	if len(segs) == 0 {
		return prefix + ".Navigating"
	}
	if segs[0].name != "current" {
		if l.strict {
			l.recordUnknownAppStateField("navigating", segs[0].name, n, def)
		}
		return ""
	}
	current := prefix + ".Navigating"
	for i := 1; i < len(segs); i++ {
		seg := segs[i]
		f, ok := navigationFieldMap(seg.name, current)
		if !ok {
			if l.strict {
				l.recordUnknownAppStateField("navigating.current", seg.name, n, def)
			}
			return ""
		}
		current = f.emit
		if f.terminal && i < len(segs)-1 {
			if l.strict {
				l.recordTerminalAppStateField("navigating.current."+seg.name, n, def)
			}
			return ""
		}
	}
	if hasAnyOptional(segs) {
		return l.guardAppStateOptional(prefix+".Navigating", segs[1:], current)
	}
	return current
}

// lowerUpdatedChain handles `updated.current`. Always renders the
// pageState.Updated boolean; server-side it's always false.
func (l *Lowerer) lowerUpdatedChain(prefix string, segs []chainSegment, n *Node, def string) string {
	if len(segs) == 0 {
		// Bare `updated` — odd but lower to the struct itself so the
		// caller's truthy wrap behaves.
		return prefix + ".Updated"
	}
	if segs[0].name != "current" || len(segs) > 1 {
		if l.strict {
			l.recordUnknownAppStateField("updated", segs[0].name, n, def)
		}
		return ""
	}
	return prefix + ".Updated"
}

// pageFieldMap lowers one segment of a chain rooted at the `page` rune.
// current is the Go expression already rendered for the chain prefix
// (e.g. `pageState` on the first call, `pageState.URL` on the second).
//
// The map is intentionally narrow: only fields the framework exposes
// land here. Anything else surfaces as a hard build error pointing at
// the SSR fallback annotation.
func pageFieldMap(name, current string) (appStateField, bool) {
	switch name {
	case "url":
		return appStateField{emit: current + ".URL"}, true
	case "params":
		return appStateField{emit: current + ".Params"}, true
	case "route":
		return appStateField{emit: current + ".Route"}, true
	case "status":
		return appStateField{emit: current + ".Status", terminal: true}, true
	case "error":
		return appStateField{emit: current + ".Error"}, true
	case "data":
		return appStateField{emit: current + ".Data", terminal: true}, true
	case "form":
		return appStateField{emit: current + ".Form", terminal: true}, true
	case "state":
		return appStateField{emit: current + ".State"}, true
	}
	// After the first segment, dispatch to per-subtype maps.
	switch {
	case strings.HasSuffix(current, ".URL"):
		return urlFieldMap(name, current)
	case strings.HasSuffix(current, ".Route"):
		return routeFieldMap(name, current)
	case strings.HasSuffix(current, ".Error"):
		return errorFieldMap(name, current)
	case strings.HasSuffix(current, ".Params"), strings.HasSuffix(current, ".State"):
		return appStateField{emit: fmt.Sprintf("%s[%q]", current, name), computed: true, terminal: true}, true
	}
	return appStateField{}, false
}

// urlFieldMap lowers `page.url.<field>` segments. v1 covers the
// high-value subset that maps to net/url getters without helper
// indirection: pathname, host, hostname, port, href. Less-common
// fields (search, hash, origin, protocol, searchParams) are deferred —
// they require small string-shaping helpers we haven't added yet.
// Routes that read them surface a hard build error pointing at the
// SSR fallback annotation.
func urlFieldMap(name, current string) (appStateField, bool) {
	switch name {
	case "pathname":
		return appStateField{emit: current + ".Path", terminal: true}, true
	case "host":
		return appStateField{emit: current + ".Host", terminal: true}, true
	case "hostname":
		return appStateField{emit: current + ".Hostname()", terminal: true}, true
	case "port":
		return appStateField{emit: current + ".Port()", terminal: true}, true
	case "href":
		return appStateField{emit: current + ".String()", terminal: true}, true
	}
	return appStateField{}, false
}

// routeFieldMap lowers `page.route.<field>` segments. Only `id` is
// recognised today — Svelte's route surface adds nothing else.
func routeFieldMap(name, current string) (appStateField, bool) {
	if name == "id" {
		return appStateField{emit: current + ".ID", terminal: true}, true
	}
	return appStateField{}, false
}

// errorFieldMap lowers `page.error.<field>` segments. The Error pointer
// may be nil; chain-walk lifts that into an optional guard.
func errorFieldMap(name, current string) (appStateField, bool) {
	switch name {
	case "message":
		return appStateField{emit: current + ".Message", terminal: true}, true
	case "status":
		return appStateField{emit: current + ".Status", terminal: true}, true
	}
	return appStateField{}, false
}

// navigationFieldMap lowers `navigating.current.<field>` segments. All
// sub-fields are terminal because the Navigation struct's nested types
// are pointers/slices/maps the lowerer doesn't traverse further at v1.
func navigationFieldMap(name, current string) (appStateField, bool) {
	switch name {
	case "type":
		return appStateField{emit: current + ".Type", terminal: true}, true
	case "from":
		return appStateField{emit: current + ".From", terminal: true}, true
	case "to":
		return appStateField{emit: current + ".To", terminal: true}, true
	case "complete":
		return appStateField{emit: current + ".Complete", terminal: true}, true
	}
	return appStateField{}, false
}

// hasAnyOptional reports whether any segment in the chain uses `?.`.
// Used to decide whether to wrap the lowered expression in an inline
// nil guard.
func hasAnyOptional(segs []chainSegment) bool {
	for _, s := range segs {
		if s.optional {
			return true
		}
	}
	return false
}

// guardAppStateOptional wraps an app-state chain in an inline IIFE
// nil-guard mirroring the strategy lowerOptionalChain uses for
// data-root chains. The guard checks each prefix that precedes an
// optional segment; the final return is the full expression.
//
// Optional chains on app-state are dominated by `page.error?.message`
// — Error is the only nilable framework field. We keep the same IIFE
// shape for consistency.
func (l *Lowerer) guardAppStateOptional(prefix string, segs []chainSegment, finalExpr string) string {
	var b strings.Builder
	b.WriteString("func() any { ")
	current := prefix
	for _, seg := range segs {
		// Re-walk to find the cumulative Go expression for each prefix
		// boundary. We can't reuse the field map here because urlFieldMap
		// etc. produce *terminal* expressions that don't compose; for
		// the guard we only need the prefix chain, which always uses the
		// raw `current.<GoName>` shape.
		nextField, ok := pageFieldMap(seg.name, current)
		if !ok {
			break
		}
		current = nextField.emit
		if seg.optional {
			fmt.Fprintf(&b, "if %s == nil { return nil }; ", current)
		}
	}
	fmt.Fprintf(&b, "return %s }()", finalExpr)
	return b.String()
}

// recordUnknownAppStateField surfaces a $app/state surface read the
// lowerer does not recognise. The error message points at the SSR
// fallback annotation as the explicit opt-out.
func (l *Lowerer) recordUnknownAppStateField(parent, field string, n *Node, def string) {
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: cannot lower $app/state field at %s\n"+
			"  expression: %s\n"+
			"  reason: %q is not a recognised field of `%s`\n"+
			"  suggestion: drop the read or annotate route // sveltego:ssr-fallback",
		l.formatPos(n), def, field, parent,
	))
}

// recordTerminalAppStateField surfaces an attempt to dot past a
// terminal app-state field (e.g. `page.url.href.toLowerCase()`).
// Terminal fields are scalar Go expressions with no further chain.
func (l *Lowerer) recordTerminalAppStateField(field string, n *Node, def string) {
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: cannot dot-access terminal $app/state field %q at %s\n"+
			"  expression: %s\n"+
			"  reason: the lowered Go expression is scalar; further dot-access has no equivalent\n"+
			"  suggestion: stop the chain at %s, or annotate route // sveltego:ssr-fallback",
		field, l.formatPos(n), def, field,
	))
}

// lowerOptionalChain renders an optional chain as an inline IIFE
// that guards every link preceding an optional segment. Strategy
// chosen at PR per #427's open question:
//
//	JS: data.user?.profile?.name
//	Go: func() any {
//	      if data.User == nil { return nil }
//	      if data.User.Profile == nil { return nil }
//	      return data.User.Profile.Name
//	    }()
//
// Rejected the alternative reflective helper (`server.Chain(x, "f1",
// "f2")`): adding a runtime helper here pulls Phase 4 into Phase 5's
// scope, costs reflection-per-render (bench risk), and produces less
// readable generated code than direct guards. Inline guards keep the
// expression a pure Go expression suitable for template-literal
// interpolation, type-check without a new helper, and let the Go
// compiler inline the closure when SSA elects to.
//
// A single closure (rather than nested closures per link) keeps the
// generated source compact — the reader scans a flat list of guards
// rather than tracking nested IIFE binding scopes.
func (l *Lowerer) lowerOptionalChain(parts []string, segs []chainSegment) string {
	var b strings.Builder
	b.WriteString("func() any { ")
	for i, s := range segs {
		if !s.optional {
			continue
		}
		guard := strings.Join(parts[:i+1], ".")
		fmt.Fprintf(&b, "if %s == nil { return nil }; ", guard)
	}
	fmt.Fprintf(&b, "return %s }()", strings.Join(parts, "."))
	return b.String()
}

// recordUnknownRoot pushes a hard-error onto the lowerer's accumulator
// describing an unknown identifier root. Format follows the issue's
// detailed-design block, with `byte=<offset>` substituting for the
// `<line>:<col>` precision the sidecar disabled (locations: false).
func (l *Lowerer) recordUnknownRoot(root *Node, n *Node, def string) {
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: cannot lower property access at %s\n"+
			"  expression: %s\n"+
			"  reason: %q is neither in scope nor a recognised data root\n"+
			"  suggestion: declare it as a {@const}, an {#each} alias, a {#snippet} parameter, or destructure it from the route's PageData",
		l.formatPos(n), def, root.Name,
	))
}

// recordMissingField surfaces a JSON tag the typegen Shape did not
// declare on the parent struct. Mirrors the format from issue #427's
// detailed design.
func (l *Lowerer) recordMissingField(parentType, jsonTag string, n *Node, def string) {
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: cannot lower property access at %s\n"+
			"  expression: %s\n"+
			"  reason: %q not present in %s JSON tag map\n"+
			"  suggestion: add the field to %s or annotate route // sveltego:ssr-fallback",
		l.formatPos(n), def, jsonTag, parentType, parentType,
	))
}

// recordComputedAccess rejects bracket access (`data["x"]`). The user
// must rewrite to typed field access or opt the route into the Phase
// 8 sidecar fallback.
func (l *Lowerer) recordComputedAccess(n *Node, def string) {
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: computed access not supported in SSR transpile at %s\n"+
			"  expression: %s\n"+
			"  reason: bracket-style member access (`x[expr]`) lacks a JSON tag the lowerer can rewrite\n"+
			"  suggestion: rewrite to typed field access, or annotate route // sveltego:ssr-fallback",
		l.formatPos(n), def,
	))
}

// recordTraverseThroughSlice flags an attempt to traverse a slice
// without indexing — Svelte's compiled output rarely produces this,
// but a custom Svelte expression like `data.posts.length` would land
// here. The lowerer does not know about JS prototype properties.
func (l *Lowerer) recordTraverseThroughSlice(field typegen.Field, n *Node, def string) {
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: cannot traverse slice field %q at %s\n"+
			"  expression: %s\n"+
			"  reason: %q is %s; further dot-access requires indexing first\n"+
			"  suggestion: use {#each %s as item} and reference item.<field>, or annotate route // sveltego:ssr-fallback",
		field.GoName, l.formatPos(n), def, field.GoName, field.GoType, field.Name,
	))
}

// recordTraversePrimitive flags traversal past a primitive leaf. JS
// allows it (everything is an object), Go does not. We surface the
// same actionable error rather than silently emitting a broken chain.
func (l *Lowerer) recordTraversePrimitive(field typegen.Field, n *Node, def string) {
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: cannot dot-access primitive field %q at %s\n"+
			"  expression: %s\n"+
			"  reason: %q is %s; traversal past it has no Go equivalent\n"+
			"  suggestion: stop the chain at %s, or annotate route // sveltego:ssr-fallback",
		field.GoName, l.formatPos(n), def, field.GoName, field.GoType, field.GoName,
	))
}

// formatPos renders the route + byte-offset pair the sidecar produced
// for n. Locations are disabled at the Acorn layer (Phase 2 sub-
// decision: locations: false), so byte offsets are the best we can
// do without reparsing the JS. Phase 7 / 8 may extend the envelope to
// carry locations; until then `byte=<N>` is the stable hook readers
// learn to recognise.
func (l *Lowerer) formatPos(n *Node) string {
	route := l.route
	if route == "" {
		route = "<unknown route>"
	}
	if n == nil {
		return route + ":byte=?"
	}
	return fmt.Sprintf("%s:byte=%d", route, n.Start)
}

// hasLocal walks the scope chain checking for any binding under name.
// Used by the rewriter to distinguish "name has a local binding the
// emitter forgot to classify" (rare, but possible for synthetic
// snippet-arg names) from "name is undeclared" (real error).
func hasLocal(s *Scope, name string) bool {
	for cur := s; cur != nil; cur = cur.parent {
		if _, ok := cur.locals[name]; ok {
			return true
		}
	}
	return false
}

// isEmitterScratch reports whether name was produced by the Phase 3
// identifier-mangling pass (e.g. `ssvar_index`, `ssvar_payload`,
// `ssvar_arg0`) or matches the few hand-introduced bookkeeping names
// the emitter creates without declaring through the scope tracker
// (`each_array`). These never refer to user data.
func isEmitterScratch(name string) bool {
	if strings.HasPrefix(name, "ssvar_") {
		return true
	}
	switch name {
	case "each_array", "payload":
		return true
	}
	return false
}

// RewriteObjectSpread implements [SpreadRewriter]. It receives the
// SpreadElement node and the already-formatted inner expression; in
// strict mode it records a hard error pointing at the unresolved
// spread (since lowering arbitrary object spreads requires whole-
// expression type information beyond what typegen exposes today). In
// non-strict mode the rewriter signals expanded=false so the emitter
// keeps the Phase 3 placeholder.
//
// Future work (#430 sidecar fallback) may relax this by routing
// spread-heavy components through the Node sidecar; until then,
// loud failure surfaces the gap.
func (l *Lowerer) RewriteObjectSpread(_ *Scope, spread *Node, inner string) (string, bool) {
	if l == nil {
		return "", false
	}
	if !l.strict {
		return "", false
	}
	l.errs = append(l.errs, fmt.Errorf(
		"ssr transpile: object spread requires typed shape inference at %s\n"+
			"  expression: ...%s\n"+
			"  reason: spread of an arbitrary expression cannot be lowered without whole-expression type information\n"+
			"  suggestion: refactor the spread to explicit fields, or annotate route // sveltego:ssr-fallback",
		l.formatPos(spread), inner,
	))
	return "", false
}

// titleCase converts a JSON-tag style name to a Go-style exported
// identifier. Handles snake_case, kebab-case, and camelCase inputs.
// Used as the GoName fallback when typegen failed to record the
// original Go field identifier (defensive — typegen always sets it
// for exported fields).
func titleCase(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	upper := true
	for _, r := range s {
		if r == '_' || r == '-' {
			upper = true
			continue
		}
		if upper {
			if r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			upper = false
		}
		b.WriteRune(r)
	}
	return b.String()
}
