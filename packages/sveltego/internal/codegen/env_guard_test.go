package codegen

import (
	"errors"
	"go/parser"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
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
