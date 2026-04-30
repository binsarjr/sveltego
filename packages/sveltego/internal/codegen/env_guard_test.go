package codegen

import (
	"errors"
	"go/parser"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/ast"
)

func TestCheckPrivateEnv(t *testing.T) {
	pos := ast.Pos{Line: 5, Col: 1}

	allow := []string{
		"x",
		"env.StaticPublic(\"PUBLIC_API\")",
		"env.DynamicPublic(\"PUBLIC_FLAG\")",
		"someFunc(env.StaticPublic(\"PUBLIC_X\"))",
		"other.StaticPrivate(\"X\")",
	}
	for _, src := range allow {
		expr, err := parser.ParseExpr(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		if err := checkPrivateEnv(expr, pos); err != nil {
			t.Errorf("checkPrivateEnv(%q) = %v; want nil", src, err)
		}
	}

	deny := []struct {
		src  string
		want string
	}{
		{`env.StaticPrivate("DATABASE_URL")`, "env.StaticPrivate"},
		{`env.DynamicPrivate("API_KEY")`, "env.DynamicPrivate"},
		{`"prefix-" + env.StaticPrivate("X")`, "env.StaticPrivate"},
		{`f(env.DynamicPrivate("Y"))`, "env.DynamicPrivate"},
	}
	for _, tc := range deny {
		expr, err := parser.ParseExpr(tc.src)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.src, err)
		}
		err = checkPrivateEnv(expr, pos)
		if err == nil {
			t.Errorf("checkPrivateEnv(%q) = nil; want error", tc.src)
			continue
		}
		var ce *CodegenError
		if !errors.As(err, &ce) {
			t.Errorf("checkPrivateEnv(%q) returned %T, want *CodegenError", tc.src, err)
			continue
		}
		if ce.Pos != pos {
			t.Errorf("pos = %v, want %v", ce.Pos, pos)
		}
		if !strings.Contains(ce.Msg, tc.want) {
			t.Errorf("msg = %q, want substring %q", ce.Msg, tc.want)
		}
		if !strings.Contains(ce.Msg, "private env access not allowed") {
			t.Errorf("msg = %q, want diagnostic prefix", ce.Msg)
		}
	}
}

func TestCheckPrivateEnv_NilExpr(t *testing.T) {
	if err := checkPrivateEnv(nil, ast.Pos{}); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

func TestGenerate_RejectsPrivateEnvInMustache(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Mustache{P: ast.Pos{Line: 3, Col: 5}, Expr: `env.StaticPrivate("DATABASE_URL")`},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected diagnostic, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if ce.Pos.Line != 3 || ce.Pos.Col != 5 {
		t.Errorf("pos = %v, want 3:5", ce.Pos)
	}
	if !strings.Contains(ce.Msg, "private env access not allowed") {
		t.Errorf("msg = %q", ce.Msg)
	}
}

func TestGenerate_AcceptsPublicEnvInMustache(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Mustache{P: ast.Pos{Line: 1, Col: 1}, Expr: `env.StaticPublic("PUBLIC_API_URL")`},
		},
	}
	if _, err := Generate(frag, Options{PackageName: "page"}); err != nil {
		t.Fatalf("public env should be permitted, got %v", err)
	}
}

func TestGenerate_RejectsPrivateEnvInIfCond(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.IfBlock{
				P:    ast.Pos{Line: 2, Col: 1},
				Cond: `env.DynamicPrivate("FLAG") != ""`,
				Then: []ast.Node{&ast.Text{Value: "x"}},
			},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected diagnostic, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if !strings.Contains(ce.Msg, "env.DynamicPrivate") {
		t.Errorf("msg = %q", ce.Msg)
	}
}

func TestGenerate_RejectsPrivateEnvInRender(t *testing.T) {
	frag := &ast.Fragment{
		Children: []ast.Node{
			&ast.Render{P: ast.Pos{Line: 4, Col: 1}, Expr: `card(env.StaticPrivate("KEY"))`},
		},
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected diagnostic, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if !strings.Contains(ce.Msg, "env.StaticPrivate") {
		t.Errorf("msg = %q", ce.Msg)
	}
}
