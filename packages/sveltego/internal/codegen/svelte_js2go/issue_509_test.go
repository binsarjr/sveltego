package sveltejs2go

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
)

// Issue #509 — four lowerer gaps surfaced on kitchen-sink. One file
// per bug to keep the regression coverage searchable; each test also
// drops a golden under testdata/golden/issue_509/ for byte-stable
// diffs against future emitter changes.

// --- Bug 2.1 — _error.svelte destructure references undefined props -----

// TestIssue509_ErrorRouteAliasesTypedData verifies that
// `let { error } = $props()` in an `_error.svelte` template binds
// `error` as a Go alias of the typed `data ErrorData` parameter
// instead of casting through a non-existent `props` map.
func TestIssue509_ErrorRouteAliasesTypedData(t *testing.T) {
	t.Parallel()
	// _error.svelte:  let { error } = $props(); <p>{error.message}</p>
	root := buildProgram(buildBlock(
		propsDestructure("error"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(ident("error"), ident("message")),
			),
		),
	))
	errorShape := shape("ErrorData",
		typegen.ShapeType{Name: "ErrorData", Fields: []typegen.Field{
			{Name: "message", GoName: "Message", GoType: "string"},
			{Name: "code", GoName: "Code", GoType: "int"},
			{Name: "id", GoName: "ID", GoType: "string"},
		}},
	)
	lo := NewLowerer(errorShape, LowererOptions{Route: "/__error__/test", Strict: true})
	got, err := TranspileNode(root, "/__error__/test", Options{
		PackageName:    "errorsrc",
		Rewriter:       lo,
		TypedDataParam: "ErrorData",
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	src := string(got)
	if strings.Contains(src, "props[") {
		t.Fatalf("error route must not reference undefined props map:\n%s", src)
	}
	if !strings.Contains(src, "error := data") {
		t.Fatalf("expected error alias to typed data parameter:\n%s", src)
	}
	if !strings.Contains(src, "error.Message") {
		t.Fatalf("expected lowered error.Message:\n%s", src)
	}
	assertGolden(t, "issue_509/error-route-props", got)
}

// --- Bug 2.2 — {#each} over typed slice loses element type --------------

// TestIssue509_EachOverTypedSlicePreservesElement verifies that an
// each-array lowering against `data.Links` (typed `[]Link`) preserves
// the element type so `link.href` lowers to `link.Href` instead of
// failing to compile against `any`.
func TestIssue509_EachOverTypedSlicePreservesElement(t *testing.T) {
	t.Parallel()
	// {#each data.links as link}<a href={link.href}>{link.label}</a>{/each}
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("each_array",
			helperCall("ensure_array_like", memExpr(ident("data"), ident("links"))),
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
				letDecl("link", computedMember(ident("each_array"), ident("$$index"))),
				pushTemplate(
					[]string{"<a href=\"", "\">", "</a>"},
					[]*Node{
						escapeOf(memExpr(ident("link"), ident("href"))),
						escapeOf(memExpr(ident("link"), ident("label"))),
					},
				),
			),
		),
	))
	sh := shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "links", GoName: "Links", GoType: "[]Link", NamedType: "Link", Slice: true},
		}},
		typegen.ShapeType{Name: "Link", Fields: []typegen.Field{
			{Name: "href", GoName: "Href", GoType: "string"},
			{Name: "label", GoName: "Label", GoType: "string"},
		}},
	)
	lo := NewLowerer(sh, LowererOptions{Route: "/eachtest", Strict: true})
	got, err := TranspileNode(root, "/eachtest", Options{
		PackageName:    "gen",
		Rewriter:       lo,
		TypedDataParam: "PageData",
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	src := string(got)
	if strings.Contains(src, "EnsureArrayLike") {
		t.Fatalf("typed slice should bypass EnsureArrayLike:\n%s", src)
	}
	if !strings.Contains(src, "each_array := data.Links") {
		t.Fatalf("expected typed slice direct assignment:\n%s", src)
	}
	if !strings.Contains(src, "link.Href") || !strings.Contains(src, "link.Label") {
		t.Fatalf("expected typed-element field access:\n%s", src)
	}
	assertGolden(t, "issue_509/each-typed-slice", got)
}

// --- Bug 2.3 — JS `undefined` keyword emits Go `undefined` identifier ---

// TestIssue509_UndefinedKeywordLowersToNil verifies the JS `undefined`
// literal in an attribute expression lowers to Go `nil` rather than
// emitting an undefined identifier reference.
func TestIssue509_UndefinedKeywordLowersToNil(t *testing.T) {
	t.Parallel()
	// <a aria-current={active ? 'page' : undefined}>Home</a>
	// $.attr("aria-current", active ? 'page' : undefined, false)
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			tplLit(
				[]string{"<a", ">Home</a>"},
				[]*Node{
					helperCall("attr",
						strLit("aria-current"),
						conditional(
							memExpr(ident("data"), ident("active")),
							strLit("page"),
							ident("undefined"),
						),
						boolLit(false),
					),
				},
			),
		),
	))
	sh := shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "active", GoName: "Active", GoType: "bool"},
		}},
	)
	lo := NewLowerer(sh, LowererOptions{Route: "/undef", Strict: true})
	got, err := TranspileNode(root, "/undef", Options{
		PackageName:    "gen",
		Rewriter:       lo,
		TypedDataParam: "PageData",
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	src := string(got)
	// `undefined` must not appear as a bare Go identifier.
	if strings.Contains(src, "return undefined") {
		t.Fatalf("undefined keyword leaked into Go output:\n%s", src)
	}
	// Conditional alternate should be `nil`.
	if !strings.Contains(src, "return nil") {
		t.Fatalf("expected nil as the conditional alternate:\n%s", src)
	}
	assertGolden(t, "issue_509/undefined-to-nil", got)
}

// --- Bug 2.4 — Optional chain on typed struct emits invalid nil compare -

// TestIssue509_OptionalChainOnStructDropsGuard verifies that
// `data?.active` against a struct-valued data root drops the nil guard
// (Go's `data == nil` against a struct value would not compile).
func TestIssue509_OptionalChainOnStructDropsGuard(t *testing.T) {
	t.Parallel()
	// let active = data?.active ?? false;
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("active", logical("??",
			chain(optionalMember(ident("data"), ident("active"))),
			boolLit(false),
		)),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")), ident("active")),
		),
	))
	sh := shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "active", GoName: "Active", GoType: "bool"},
		}},
	)
	lo := NewLowerer(sh, LowererOptions{Route: "/optchain", Strict: true})
	got, err := TranspileNode(root, "/optchain", Options{
		PackageName:    "gen",
		Rewriter:       lo,
		TypedDataParam: "PageData",
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("unexpected lowering errors: %v", errs)
	}
	src := string(got)
	if strings.Contains(src, "data == nil") {
		t.Fatalf("optional chain on struct value must skip nil guard:\n%s", src)
	}
	if !strings.Contains(src, "data.Active") {
		t.Fatalf("expected direct struct field access:\n%s", src)
	}
	assertGolden(t, "issue_509/optional-chain-on-struct", got)
}
