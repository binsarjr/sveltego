package codegen

import (
	"errors"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

func TestSnippetSignature(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "w *render.Writer"},
		{"   ", "w *render.Writer"},
		{"p Post", "p Post, w *render.Writer"},
		{"name string, count int", "name string, count int, w *render.Writer"},
	}
	for _, tc := range cases {
		if got := snippetSignature(tc.in); got != tc.want {
			t.Errorf("snippetSignature(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateSnippetParams(t *testing.T) {
	pos := ast.Pos{Line: 2, Col: 4}
	good := []string{
		"",
		"p Post",
		"name string, count int",
		"items []string",
		"fn func(int) bool",
	}
	for _, src := range good {
		if err := validateSnippetParams(src, pos); err != nil {
			t.Errorf("validateSnippetParams(%q) = %v; want nil", src, err)
		}
	}

	bad := []string{
		"p Post p2",
		"int int int",
		"(broken",
	}
	for _, src := range bad {
		err := validateSnippetParams(src, pos)
		if err == nil {
			t.Errorf("validateSnippetParams(%q) = nil; want error", src)
			continue
		}
		var ce *CodegenError
		if !errors.As(err, &ce) {
			t.Errorf("validateSnippetParams(%q) returned %T, want *CodegenError", src, err)
			continue
		}
		if ce.Pos != pos {
			t.Errorf("pos = %v, want %v", ce.Pos, pos)
		}
		if !strings.Contains(ce.Msg, "invalid {#snippet} parameter list") {
			t.Errorf("msg = %q, want substring %q", ce.Msg, "invalid {#snippet} parameter list")
		}
	}
}

func TestEmitSnippetBlock_Recursive(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.SnippetBlock{
				P:      ast.Pos{Line: 1, Col: 1},
				Name:   "tree",
				Params: "n int",
				Body: []ast.Node{
					&ast.Mustache{P: ast.Pos{Line: 1, Col: 20}, Expr: "n"},
				},
			},
		},
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	src := string(out)
	for _, want := range []string{
		"var tree func(n int, w *render.Writer) error",
		"tree = func(n int, w *render.Writer) error",
		"_ = tree",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in:\n%s", want, src)
		}
	}
}

func TestEmitSnippetBlock_RejectsBadName(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.SnippetBlock{
				P:      ast.Pos{Line: 7, Col: 3},
				Name:   "1bad",
				Params: "",
				Body:   []ast.Node{&ast.Text{Value: "x"}},
			},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if ce.Pos.Line != 7 || ce.Pos.Col != 3 {
		t.Errorf("pos = %v, want 7:3", ce.Pos)
	}
	if !strings.Contains(ce.Msg, "requires an identifier name") {
		t.Errorf("msg = %q, want substring %q", ce.Msg, "requires an identifier name")
	}
}

func TestSplitRenderCall(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantArgs string
		wantOK   bool
	}{
		{"banner()", "banner", "", true},
		{"card(post)", "card", "post", true},
		{"  card(a, b) ", "card", "a, b", true},
		{"items.row(x)", "items.row", "x", true},
		{"plain", "", "", false},
		{"", "", "", false},
		{"badcall", "", "", false},
	}
	for _, tc := range cases {
		name, args, ok := splitRenderCall(tc.in)
		if name != tc.wantName || args != tc.wantArgs || ok != tc.wantOK {
			t.Errorf("splitRenderCall(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tc.in, name, args, ok, tc.wantName, tc.wantArgs, tc.wantOK)
		}
	}
}

func TestAppendWriter(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "w"},
		{"x", "x, w"},
		{"a, b", "a, b, w"},
	}
	for _, tc := range cases {
		if got := appendWriter(tc.in); got != tc.want {
			t.Errorf("appendWriter(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEmitRender_RejectsBadExpr(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Render{P: ast.Pos{Line: 4, Col: 2}, Expr: "noCall"},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if !strings.Contains(ce.Msg, "expects a callable expression") {
		t.Errorf("msg = %q", ce.Msg)
	}
}
