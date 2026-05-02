package codegen

import (
	"errors"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

func TestExtractSvelteOptions_capturesAll(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Element{
				Name: "svelte:options",
				P:    ast.Pos{Line: 1, Col: 1},
				Attributes: []ast.Attribute{
					{Name: "runes", Kind: ast.AttrStatic, Value: &ast.StaticValue{Value: "true"}},
					{Name: "customElement", Kind: ast.AttrStatic, Value: &ast.StaticValue{Value: "my-thing"}},
					{Name: "namespace", Kind: ast.AttrStatic, Value: &ast.StaticValue{Value: "svg"}},
					{Name: "accessors", Kind: ast.AttrStatic, Value: nil},
					{Name: "immutable", Kind: ast.AttrDynamic, Value: &ast.DynamicValue{Expr: "false"}},
				},
			},
			&ast.Element{Name: "div"},
		},
	}
	got, err := extractSvelteOptions(frag)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !got.Present {
		t.Fatal("expected Present")
	}
	if got.Runes == nil || *got.Runes != true {
		t.Errorf("Runes = %v", got.Runes)
	}
	if got.CustomElement != "my-thing" {
		t.Errorf("CustomElement = %q", got.CustomElement)
	}
	if got.Namespace != "svg" {
		t.Errorf("Namespace = %q", got.Namespace)
	}
	if got.Accessors == nil || *got.Accessors != true {
		t.Errorf("Accessors = %v", got.Accessors)
	}
	if got.Immutable == nil || *got.Immutable != false {
		t.Errorf("Immutable = %v", got.Immutable)
	}
	if len(frag.Children) != 1 {
		t.Errorf("svelte:options not removed, children = %d", len(frag.Children))
	}
}

func TestExtractSvelteOptions_absent(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{Children: []ast.Node{&ast.Element{Name: "div"}}}
	got, err := extractSvelteOptions(frag)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if got.Present {
		t.Errorf("expected absent")
	}
}

func TestExtractSvelteOptions_nestedRejected(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Element{
				Name: "div",
				Children: []ast.Node{
					&ast.Element{Name: "svelte:options", P: ast.Pos{Line: 4, Col: 2}},
				},
			},
		},
	}
	_, err := extractSvelteOptions(frag)
	if err == nil {
		t.Fatal("expected error on nested <svelte:options>")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if !strings.Contains(ce.Msg, "must appear at the template root") {
		t.Errorf("msg = %q", ce.Msg)
	}
	if ce.Pos.Line != 4 || ce.Pos.Col != 2 {
		t.Errorf("pos = %v", ce.Pos)
	}
}

func TestExtractSvelteOptions_duplicate(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Element{Name: "svelte:options", P: ast.Pos{Line: 1, Col: 1}},
			&ast.Element{Name: "svelte:options", P: ast.Pos{Line: 5, Col: 1}},
		},
	}
	_, err := extractSvelteOptions(frag)
	if err == nil {
		t.Fatal("expected duplicate error")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T", err)
	}
	if ce.Pos.Line != 5 {
		t.Errorf("pos.line = %d, want 5", ce.Pos.Line)
	}
}

func TestExtractSvelteOptions_unknownAttr(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Element{
				Name: "svelte:options",
				P:    ast.Pos{Line: 1, Col: 1},
				Attributes: []ast.Attribute{
					{Name: "bogus", Kind: ast.AttrStatic, Value: &ast.StaticValue{Value: "x"}},
				},
			},
		},
	}
	_, err := extractSvelteOptions(frag)
	if err == nil {
		t.Fatal("expected unknown-attr error")
	}
	if !strings.Contains(err.Error(), "unknown attribute") {
		t.Errorf("err = %v", err)
	}
}

func TestExtractSvelteOptions_invalidNamespace(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Element{
				Name:       "svelte:options",
				Attributes: []ast.Attribute{{Name: "namespace", Value: &ast.StaticValue{Value: "weird"}}},
			},
		},
	}
	_, err := extractSvelteOptions(frag)
	if err == nil || !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("expected namespace error, got %v", err)
	}
}

func TestExtractSvelteOptions_childrenForbidden(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Element{
				Name:     "svelte:options",
				Children: []ast.Node{&ast.Text{Value: "x"}},
			},
		},
	}
	_, err := extractSvelteOptions(frag)
	if err == nil || !strings.Contains(err.Error(), "must not have children") {
		t.Fatalf("expected children error, got %v", err)
	}
}
