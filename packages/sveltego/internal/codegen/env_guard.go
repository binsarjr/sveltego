package codegen

import (
	goast "go/ast"

	"github.com/binsarjr/sveltego/internal/ast"
)

// checkPrivateEnv inspects expr for calls to env.StaticPrivate or
// env.DynamicPrivate. Template expressions flow into the client bundle
// during hydration; calls to private-env accessors leak server-only
// secrets. Public counterparts (env.StaticPublic, env.DynamicPublic) are
// permitted by design — the PUBLIC_ prefix is the carrier convention.
//
// The returned *CodegenError carries the originating .svelte position so
// the CLI can render caret context.
func checkPrivateEnv(expr goast.Expr, pos ast.Pos) error {
	if expr == nil {
		return nil
	}
	var hit string
	goast.Inspect(expr, func(n goast.Node) bool {
		call, ok := n.(*goast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*goast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*goast.Ident)
		if !ok {
			return true
		}
		if ident.Name != "env" {
			return true
		}
		switch sel.Sel.Name {
		case "StaticPrivate", "DynamicPrivate":
			hit = "env." + sel.Sel.Name
			return false
		}
		return true
	})
	if hit == "" {
		return nil
	}
	return &CodegenError{
		Pos: pos,
		Msg: "private env access not allowed in template (will leak to client bundle): " + hit,
	}
}
