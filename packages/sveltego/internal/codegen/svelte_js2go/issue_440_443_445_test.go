package sveltejs2go

import (
	"bytes"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
)

// --- Issue #440: children-callback ABI ---------------------------------

// TestEmitChildrenParam_DefaultSignature verifies the new option
// appends a `children func(*server.Payload)` parameter to the legacy
// (props-map) signature.
func TestEmitChildrenParam_DefaultSignature(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
	))
	got, err := TranspileNode(root, "/layout", Options{
		PackageName:       "gen",
		EmitChildrenParam: true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	want := []byte("func Render(payload *server.Payload, props map[string]any, children func(*server.Payload))")
	if !bytes.Contains(got, want) {
		t.Fatalf("missing children-callback signature; got:\n%s", got)
	}
}

// TestEmitChildrenParam_TypedSignature verifies the option composes
// with TypedDataParam: the typed PageData / LayoutData parameter
// stays first, the children callback lands last.
func TestEmitChildrenParam_TypedSignature(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
	))
	got, err := TranspileNode(root, "/layout", Options{
		PackageName:       "gen",
		TypedDataParam:    "LayoutData",
		EmitChildrenParam: true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	want := []byte("func Render(payload *server.Payload, data LayoutData, children func(*server.Payload))")
	if !bytes.Contains(got, want) {
		t.Fatalf("missing typed children-callback signature; got:\n%s", got)
	}
}

// TestEmitChildrenParam_DispatchPropsMember verifies the emitter
// recognises `$$props.children($$renderer)` and lowers it to a
// nil-guarded callback invocation against the Go-side payload.
func TestEmitChildrenParam_DispatchPropsMember(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			strLit("<header>top</header>"),
		),
		// $$props.children($$renderer) — Svelte's compiled output for
		// {@render children()} when the layout did not destructure the
		// callback prop.
		callStmt(
			memExpr(ident("$$props"), ident("children")),
			ident("$$renderer"),
		),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			strLit("<footer>bot</footer>"),
		),
	))
	got, err := TranspileNode(root, "/layout", Options{
		PackageName:       "gen",
		EmitChildrenParam: true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	src := string(got)
	if !strings.Contains(src, "if children != nil {") {
		t.Fatalf("expected nil guard around callback; got:\n%s", src)
	}
	if !strings.Contains(src, "children(payload)") {
		t.Fatalf("expected callback invocation against payload; got:\n%s", src)
	}
	if !strings.Contains(src, "<header>top</header>") || !strings.Contains(src, "<footer>bot</footer>") {
		t.Fatalf("layout chrome surrounding the callback dropped; got:\n%s", src)
	}
}

// TestEmitChildrenParam_DispatchDestructured verifies the emitter
// handles `let { children } = $$props` followed by a bare
// `children($$renderer)` call. The destructured prop is reclassified
// as LocalCallback so the Lowerer leaves it alone, and the bare call
// lowers to the same nil-guarded shape.
func TestEmitChildrenParam_DispatchDestructured(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		// let { data, children } = $$props
		&Node{
			Type: "VariableDeclaration",
			Kind: "let",
			Declarations: []*Node{
				{
					Type: "VariableDeclarator",
					ID: &Node{
						Type: "ObjectPattern",
						Properties: []*Node{
							{Type: "Property", Key: ident("data"), Value: ident("data"), Shorthand: true, Kind: "init"},
							{Type: "Property", Key: ident("children"), Value: ident("children"), Shorthand: true, Kind: "init"},
						},
					},
					Init: ident("$$props"),
				},
			},
		},
		callStmt(ident("children"), ident("$$renderer")),
	))
	got, err := TranspileNode(root, "/layout", Options{
		PackageName:       "gen",
		TypedDataParam:    "LayoutData",
		EmitChildrenParam: true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	src := string(got)
	if !strings.Contains(src, "if children != nil {") || !strings.Contains(src, "children(payload)") {
		t.Fatalf("expected nil-guarded callback dispatch; got:\n%s", src)
	}
	// The destructure must NOT emit a map cast for `children` (it would
	// shadow the typed parameter); only `data` would, and we're in
	// typed-data mode so neither does.
	if strings.Contains(src, `props["children"]`) {
		t.Fatalf("children must not fall through the props map; got:\n%s", src)
	}
}

// TestLocalCallback_NotLowered verifies the Phase 5 Lowerer leaves
// callback identifiers alone (no JSON-tag rewrite) — issue #440
// acceptance criterion 2.
func TestLocalCallback_NotLowered(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		// let { data, children } = $$props
		&Node{
			Type: "VariableDeclaration",
			Kind: "let",
			Declarations: []*Node{
				{
					Type: "VariableDeclarator",
					ID: &Node{
						Type: "ObjectPattern",
						Properties: []*Node{
							{Type: "Property", Key: ident("data"), Value: ident("data"), Shorthand: true, Kind: "init"},
							{Type: "Property", Key: ident("children"), Value: ident("children"), Shorthand: true, Kind: "init"},
						},
					},
					Init: ident("$$props"),
				},
			},
		},
		callStmt(memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")), memExpr(ident("data"), ident("title"))),
		),
		callStmt(ident("children"), ident("$$renderer")),
	))
	lo := NewLowerer(shape("LayoutData",
		typegen.ShapeType{Name: "LayoutData", Fields: []typegen.Field{
			{Name: "title", GoName: "Title", GoType: "string"},
		}},
	), LowererOptions{Route: "/layout", Strict: true})
	got, err := TranspileNode(root, "/layout", Options{
		PackageName:       "gen",
		TypedDataParam:    "LayoutData",
		EmitChildrenParam: true,
		Rewriter:          lo,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	src := string(got)
	if !strings.Contains(src, "data.Title") {
		t.Fatalf("expected lowered data.Title; got:\n%s", src)
	}
	if !strings.Contains(src, "children(payload)") {
		t.Fatalf("expected callback dispatch; got:\n%s", src)
	}
}

// --- Issue #443: snippet hoisting + truthy ----------------------------

// TestSnippetHoist_ForwardCall verifies a snippet declared after its
// first call site is lifted above the call so the generated `:=`
// satisfies Go's declare-before-use rule.
func TestSnippetHoist_ForwardCall(t *testing.T) {
	t.Parallel()
	// Body: call card(item) BEFORE const card = (...) => {...}.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(ident("card"), memExpr(ident("data"), ident("item"))),
		constDecl("card", arrowFn(
			[]*Node{ident("item")},
			buildBlock(pushString("<article></article>")),
		)),
	))
	got, err := TranspileNode(root, "/snippet-forward", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	src := string(got)
	declIdx := strings.Index(src, "card := func")
	callIdx := strings.Index(src, "card(data.item)")
	if declIdx < 0 || callIdx < 0 {
		t.Fatalf("missing declaration or call; got:\n%s", src)
	}
	if declIdx >= callIdx {
		t.Fatalf("snippet declaration must precede call after hoist; got:\n%s", src)
	}
}

// TestSnippetHoist_NoForwardCall_ByteIdentical verifies declarations
// already preceding their calls stay in source order, so the existing
// 30+ Phase 7 corpus goldens remain byte-identical.
func TestSnippetHoist_NoForwardCall_ByteIdentical(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("card", arrowFn(
			[]*Node{ident("item")},
			buildBlock(pushString("<article></article>")),
		)),
		callStmt(ident("card"), memExpr(ident("data"), ident("item"))),
	))
	got, err := TranspileNode(root, "/snippet-ordered", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	src := string(got)
	declIdx := strings.Index(src, "card := func")
	callIdx := strings.Index(src, "card(data.item)")
	dataIdx := strings.Index(src, `data, _ := props["data"]`)
	if dataIdx < 0 || declIdx < 0 || callIdx < 0 {
		t.Fatalf("missing expected lines:\n%s", src)
	}
	if !(dataIdx < declIdx && declIdx < callIdx) {
		t.Fatalf("expected destructure → snippet → call order; got:\n%s", src)
	}
}

// TestTruthy_BareIdentifier verifies a bare identifier condition
// (e.g. `{@const v = …}` followed by `{#if v}`) is wrapped in
// server.Truthy() so non-bool Go types compile.
func TestTruthy_BareIdentifier(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(ident("flag"),
			buildBlock(pushString("<p>on</p>")),
			nil,
		),
	))
	got, err := TranspileNode(root, "/truthy-ident", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if !strings.Contains(string(got), "if server.Truthy(flag) {") {
		t.Fatalf("expected server.Truthy wrap on bare identifier; got:\n%s", got)
	}
}

// TestTruthy_MemberAccessNonBool verifies a member-access condition
// against a struct field is wrapped in server.Truthy.
func TestTruthy_MemberAccessNonBool(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(memExpr(ident("data"), ident("title")),
			buildBlock(pushString("<h1>has title</h1>")),
			nil,
		),
	))
	got, err := TranspileNode(root, "/truthy-member", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if !strings.Contains(string(got), "if server.Truthy(data.title) {") {
		t.Fatalf("expected server.Truthy wrap on member access; got:\n%s", got)
	}
}

// TestTruthy_BoolExprPassThrough verifies comparisons and !-prefixed
// expressions remain bare (no Truthy() wrap) so the dominant Phase 7
// corpus shapes stay byte-identical.
func TestTruthy_BoolExprPassThrough(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			binary("==", memExpr(ident("data"), ident("status")), strLit("ok")),
			buildBlock(pushString("<p>ok</p>")),
			nil,
		),
		ifStmt(
			unary("!", memExpr(ident("data"), ident("hidden"))),
			buildBlock(pushString("<p>shown</p>")),
			nil,
		),
	))
	got, err := TranspileNode(root, "/truthy-bool", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	src := string(got)
	if strings.Contains(src, "server.Truthy") {
		t.Fatalf("bool-shaped conditions should NOT route through Truthy; got:\n%s", src)
	}
}

// --- Issue #445: {@html raw} ------------------------------------------

// TestHTMLRaw_StringField verifies $$renderer.push($.html(data.body))
// lowers to a Stringify-without-escape call.
func TestHTMLRaw_StringField(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("html")), memExpr(ident("data"), ident("body"))),
		),
	))
	got, err := TranspileNode(root, "/html-string", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	src := string(got)
	if !strings.Contains(src, "server.Stringify(data.body)") {
		t.Fatalf("expected Stringify wrap (no escape); got:\n%s", src)
	}
	if strings.Contains(src, "EscapeHTML") {
		t.Fatalf("raw {@html} must not escape:\n%s", src)
	}
}

// TestHTMLRaw_NestedInTemplate verifies the helper handles the
// dominant template-literal shape `${$.html(x)}` where it appears as
// one piece of a `$$renderer.push(`...`)` template literal.
func TestHTMLRaw_NestedInTemplate(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<div class="prose">`, `</div>`},
			[]*Node{
				callExpr(memExpr(ident("$"), ident("html")), memExpr(ident("data"), ident("body"))),
			},
		),
	))
	got, err := TranspileNode(root, "/html-tpl", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	src := string(got)
	if !strings.Contains(src, `payload.Push("<div class=\"prose\">")`) {
		t.Fatalf("expected template literal split:\n%s", src)
	}
	if !strings.Contains(src, "payload.Push(server.Stringify(data.body))") {
		t.Fatalf("expected raw Stringify push for {@html} interpolation:\n%s", src)
	}
}

// TestHTMLRaw_StatementPosition covers the rarer shape where Svelte
// emits `$.html(...)` as a top-level statement (not wrapped in a
// renderer.push). The emitter dispatches through helperHTML in
// statement form which writes via WriteRaw.
func TestHTMLRaw_StatementPosition(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(memExpr(ident("$"), ident("html")), memExpr(ident("data"), ident("body"))),
	))
	got, err := TranspileNode(root, "/html-stmt", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if !strings.Contains(string(got), "server.WriteRaw(payload, data.body)") {
		t.Fatalf("expected WriteRaw call in statement position; got:\n%s", got)
	}
}
