package codegen

import (
	"errors"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

func TestExtractStyle_present(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Element{Name: "div"},
			&ast.Style{P: ast.Pos{Line: 2, Col: 1}, Body: "div { color: red; }"},
			&ast.Element{Name: "p"},
		},
	}
	info, err := extractStyle(frag, "Component.svelte")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !info.Present {
		t.Fatalf("expected Present")
	}
	if info.Body != "div { color: red; }" {
		t.Errorf("body = %q", info.Body)
	}
	if !strings.HasPrefix(info.ScopeClass, "svelte-") {
		t.Errorf("scope = %q", info.ScopeClass)
	}
	if len(frag.Children) != 2 {
		t.Errorf("style not removed, children = %d", len(frag.Children))
	}
}

func TestExtractStyle_absent(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{Children: []ast.Node{&ast.Element{Name: "div"}}}
	info, err := extractStyle(frag, "Component.svelte")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if info.Present {
		t.Errorf("expected absent")
	}
	if len(frag.Children) != 1 {
		t.Errorf("children mutated")
	}
}

func TestExtractStyle_duplicate(t *testing.T) {
	t.Parallel()
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Style{P: ast.Pos{Line: 1, Col: 1}, Body: "a {}"},
			&ast.Style{P: ast.Pos{Line: 5, Col: 1}, Body: "b {}"},
		},
	}
	_, err := extractStyle(frag, "")
	if err == nil {
		t.Fatal("expected error on duplicate <style>")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if ce.Pos.Line != 5 {
		t.Errorf("pos line = %d, want 5", ce.Pos.Line)
	}
}

func TestApplyScopeClass_addsToBareElement(t *testing.T) {
	t.Parallel()
	el := &ast.Element{Name: "div"}
	applyScopeClass([]ast.Node{el}, "svelte-abc")
	if len(el.Attributes) != 1 {
		t.Fatalf("attrs = %d", len(el.Attributes))
	}
	a := el.Attributes[0]
	if a.Name != "class" {
		t.Errorf("name = %q", a.Name)
	}
	sv, ok := a.Value.(*ast.StaticValue)
	if !ok || sv.Value != "svelte-abc" {
		t.Errorf("value = %#v", a.Value)
	}
}

func TestApplyScopeClass_appendsToStaticClass(t *testing.T) {
	t.Parallel()
	el := &ast.Element{
		Name: "div",
		Attributes: []ast.Attribute{
			{Name: "class", Kind: ast.AttrStatic, Value: &ast.StaticValue{Value: "card"}},
		},
	}
	applyScopeClass([]ast.Node{el}, "svelte-abc")
	sv := el.Attributes[0].Value.(*ast.StaticValue)
	if sv.Value != "card svelte-abc" {
		t.Errorf("value = %q", sv.Value)
	}
}

func TestApplyScopeClass_idempotent(t *testing.T) {
	t.Parallel()
	el := &ast.Element{
		Name: "div",
		Attributes: []ast.Attribute{
			{Name: "class", Kind: ast.AttrStatic, Value: &ast.StaticValue{Value: "card svelte-abc"}},
		},
	}
	applyScopeClass([]ast.Node{el}, "svelte-abc")
	sv := el.Attributes[0].Value.(*ast.StaticValue)
	if sv.Value != "card svelte-abc" {
		t.Errorf("value = %q (duplicated)", sv.Value)
	}
}

func TestApplyScopeClass_dynamicWrapped(t *testing.T) {
	t.Parallel()
	el := &ast.Element{
		Name: "div",
		Attributes: []ast.Attribute{
			{Name: "class", Kind: ast.AttrDynamic, Value: &ast.DynamicValue{Expr: "Theme"}},
		},
	}
	applyScopeClass([]ast.Node{el}, "svelte-abc")
	iv, ok := el.Attributes[0].Value.(*ast.InterpolatedValue)
	if !ok {
		t.Fatalf("expected InterpolatedValue, got %T", el.Attributes[0].Value)
	}
	if len(iv.Parts) != 2 {
		t.Errorf("parts = %d", len(iv.Parts))
	}
}

func TestApplyScopeClass_skipsComponentsAndSpecials(t *testing.T) {
	t.Parallel()
	c := &ast.Element{Name: "Card", Component: true}
	s := &ast.Element{Name: "slot"}
	sp := &ast.Element{Name: "svelte:body"}
	applyScopeClass([]ast.Node{c, s, sp}, "svelte-abc")
	if len(c.Attributes) != 0 || len(s.Attributes) != 0 || len(sp.Attributes) != 0 {
		t.Errorf("scope leaked onto component/slot/special")
	}
}

func TestApplyScopeClass_recursesIntoBlocks(t *testing.T) {
	t.Parallel()
	inner := &ast.Element{Name: "span"}
	frag := []ast.Node{
		&ast.IfBlock{
			Cond: "true",
			Then: []ast.Node{inner},
		},
	}
	applyScopeClass(frag, "svelte-abc")
	if len(inner.Attributes) != 1 {
		t.Fatalf("nested element not scoped, attrs = %d", len(inner.Attributes))
	}
}

func TestEmitStyleBlock_skipsAbsent(t *testing.T) {
	t.Parallel()
	var b Builder
	emitStyleBlock(&b, styleInfo{})
	if got := string(b.Bytes()); got != "" {
		t.Errorf("emitted %q, want empty", got)
	}
}

func TestEmitStyleBlock_writesBody(t *testing.T) {
	t.Parallel()
	var b Builder
	emitStyleBlock(&b, styleInfo{Present: true, Body: "div{color:red}"})
	got := string(b.Bytes())
	if !strings.Contains(got, "<style>div{color:red}</style>") {
		t.Errorf("emitted = %q", got)
	}
}
