package sveltejs2go

import (
	"bytes"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/typegen"
)

// TestTypedDataParam_SignatureSwap verifies Phase 6's typed-data-param
// switch produces a typed Render(payload, data PageData) signature and
// drops the props["data"] map cast.
func TestTypedDataParam_SignatureSwap(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			callExpr(memExpr(ident("$"), ident("escape")),
				memExpr(ident("data"), ident("name")),
			),
		),
	))
	lo := NewLowerer(shape("PageData",
		typegen.ShapeType{Name: "PageData", Fields: []typegen.Field{
			{Name: "name", GoName: "Name", GoType: "string"},
		}},
	), LowererOptions{Route: "/p", Strict: true})

	got, err := TranspileNode(root, "/p", Options{
		PackageName:    "gen",
		Rewriter:       lo,
		TypedDataParam: "PageData",
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if errs := lo.Err(); errs != nil {
		t.Fatalf("lowering errors: %v", errs)
	}

	want := []byte("func Render(payload *server.Payload, data PageData)")
	if !bytes.Contains(got, want) {
		t.Fatalf("missing typed signature %q in output:\n%s", want, got)
	}
	if bytes.Contains(got, []byte(`props["data"].(map[string]any)`)) {
		t.Fatalf("typed-data mode should skip map cast for data:\n%s", got)
	}
	if !bytes.Contains(got, []byte("data.Name")) {
		t.Fatalf("expected lowered data.Name in output:\n%s", got)
	}
}

// TestTypedDataParam_DefaultsUnchanged ensures empty TypedDataParam
// keeps the legacy props map[string]any signature so existing 30-shape
// goldens remain byte-identical.
func TestTypedDataParam_DefaultsUnchanged(t *testing.T) {
	t.Parallel()
	root := buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			strLit("hello"),
		),
	))
	got, err := TranspileNode(root, "/p", Options{PackageName: "gen"})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if !bytes.Contains(got, []byte("props map[string]any")) {
		t.Fatalf("default signature should keep props map[string]any:\n%s", got)
	}
	if !bytes.Contains(got, []byte(`data, _ := props["data"].(map[string]any)`)) {
		t.Fatalf("default mode should keep props map cast:\n%s", got)
	}
}
