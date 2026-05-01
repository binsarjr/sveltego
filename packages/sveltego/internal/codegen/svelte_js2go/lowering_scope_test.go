package sveltejs2go

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
)

// scopeShape backs each scope-shadow test. The root has a few fields
// the body might destructure into local bindings; the lowerer must
// NOT touch those bindings even though they share a name with a JSON
// tag (e.g. `name`, `item`).
func scopeShape() *typegen.Shape {
	return shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "items", GoName: "Items", GoType: "[]Item", NamedType: "Item", Slice: true},
			{Name: "name", GoName: "Name", GoType: "string"},
		}},
		typegen.ShapeType{Name: "Item", Fields: []typegen.Field{
			{Name: "title", GoName: "Title", GoType: "string"},
		}},
	)
}

func TestLowerer_EachLoopShadowsDataName(t *testing.T) {
	t.Parallel()
	// {#each data.items as item}<p>{item.title}</p>{/each} — `item` is
	// a LocalEach binding, NOT subject to JSON-tag rewriting even
	// though the typegen Shape happens to have a field literally
	// called `item` (rare but possible). The lowerer must trust
	// scope.Lookup over name collisions.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("each_array",
			helperCall("ensure_array_like", memExpr(ident("data"), ident("items"))),
		),
		forLoop(
			&Node{
				Type: "VariableDeclaration",
				Kind: "let",
				Declarations: []*Node{
					{Type: "VariableDeclarator", ID: ident("$$index"), Init: numLit(0)},
					{Type: "VariableDeclarator", ID: ident("$$length"), Init: memExpr(ident("each_array"), ident("length"))},
				},
			},
			binary("<", ident("$$index"), ident("$$length")),
			update("++", ident("$$index")),
			buildBlock(
				letDecl("item", computedMember(ident("each_array"), ident("$$index"))),
				pushTemplate(
					[]string{"<li>", "</li>"},
					[]*Node{escapeOf(memExpr(ident("item"), ident("title")))},
				),
			),
		),
	))
	lo := NewLowerer(scopeShape(), LowererOptions{Route: "/r", Strict: true})
	got, err := TranspileNode(root, "/r", Options{PackageName: "gen", Rewriter: lo})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	src := string(got)
	// `item.title` must remain JS-style — it's a local in scope.
	if !strings.Contains(src, "item.title") {
		t.Errorf("expected unrewritten item.title:\n%s", src)
	}
	// And `data.items` (root chain) must lower to data.Items.
	if !strings.Contains(src, "data.Items") {
		t.Errorf("expected lowered data.Items:\n%s", src)
	}
}

func TestLowerer_AtConstShadowsDataName(t *testing.T) {
	t.Parallel()
	// {@const total = data.a + data.b} <p>{total}</p> — `total` is a
	// LocalUnknown declared via the generic var-decl path; the
	// lowerer must leave it alone even though it appears inside an
	// expression.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("total", strLit("hello")),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")), ident("total")),
		),
	))
	lo := NewLowerer(scopeShape(), LowererOptions{Route: "/r", Strict: true})
	got, err := TranspileNode(root, "/r", Options{PackageName: "gen", Rewriter: lo})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	if !strings.Contains(string(got), "server.EscapeHTML(total)") {
		t.Errorf("expected unrewritten total identifier:\n%s", got)
	}
}

func TestLowerer_SnippetParamShadowsDataField(t *testing.T) {
	t.Parallel()
	// {#snippet card(item)}<article>{item.name}</article>{/snippet}
	// — `item` is a snippet param (LocalUnknown via the generic
	// fn-expr path); the lowerer leaves item.name alone.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("card", arrowFn(
			[]*Node{ident("item")},
			buildBlock(pushTemplate(
				[]string{"<article>", "</article>"},
				[]*Node{escapeOf(memExpr(ident("item"), ident("name")))},
			)),
		)),
	))
	lo := NewLowerer(scopeShape(), LowererOptions{Route: "/r", Strict: true})
	got, err := TranspileNode(root, "/r", Options{PackageName: "gen", Rewriter: lo})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	if !strings.Contains(string(got), "item.name") {
		t.Errorf("expected unrewritten item.name in snippet body:\n%s", got)
	}
}

func TestLowerer_HoistedForInitLocalsNotRewritten(t *testing.T) {
	t.Parallel()
	// The hoisted ssvar_index / ssvar_length names are emitter
	// scratch and must never be lowered. Verified indirectly: a
	// for-loop body that references ssvar_index should see no
	// ssvar_Index appear.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("each_array",
			helperCall("ensure_array_like", memExpr(ident("data"), ident("items"))),
		),
		forLoop(
			&Node{
				Type: "VariableDeclaration",
				Kind: "let",
				Declarations: []*Node{
					{Type: "VariableDeclarator", ID: ident("$$index"), Init: numLit(0)},
					{Type: "VariableDeclarator", ID: ident("$$length"), Init: memExpr(ident("each_array"), ident("length"))},
				},
			},
			binary("<", ident("$$index"), ident("$$length")),
			update("++", ident("$$index")),
			buildBlock(
				letDecl("item", computedMember(ident("each_array"), ident("$$index"))),
				pushTemplate(
					[]string{"<li>", " (", ")</li>"},
					[]*Node{
						escapeOf(memExpr(ident("item"), ident("title"))),
						escapeOf(ident("$$index")),
					},
				),
			),
		),
	))
	lo := NewLowerer(scopeShape(), LowererOptions{Route: "/r", Strict: true})
	got, err := TranspileNode(root, "/r", Options{PackageName: "gen", Rewriter: lo})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	src := string(got)
	if strings.Contains(src, "ssvar_Index") || strings.Contains(src, "ssvar_Length") {
		t.Errorf("emitter-scratch identifiers got rewritten:\n%s", src)
	}
	// Sanity — they must still appear in their mangled form.
	if !strings.Contains(src, "ssvar_index") || !strings.Contains(src, "ssvar_length") {
		t.Errorf("expected mangled ssvar_index/ssvar_length:\n%s", src)
	}
}

func TestLowerer_FunctionParamShadowsRoot(t *testing.T) {
	t.Parallel()
	// Component helper closures take an inner $$renderer (renamed to
	// payload by Phase 4 emit). Inside, references to data still
	// resolve via the parent scope's LocalProp binding — the lowerer
	// must NOT trip over the inner closure boundary.
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("component")),
			arrowFn(
				[]*Node{ident("$$renderer")},
				buildBlock(callStmt(
					memExpr(ident("$$renderer"), ident("push")),
					callExpr(memExpr(ident("$"), ident("escape")),
						memExpr(ident("data"), ident("name")),
					),
				)),
			),
		),
	))
	lo := NewLowerer(scopeShape(), LowererOptions{Route: "/r", Strict: true})
	got, err := TranspileNode(root, "/r", Options{PackageName: "gen", Rewriter: lo})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	if !strings.Contains(string(got), "data.Name") {
		t.Errorf("expected lowered data.Name in component closure:\n%s", got)
	}
}
