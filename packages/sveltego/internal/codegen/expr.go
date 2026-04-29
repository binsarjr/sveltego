package codegen

import (
	"go/parser"
	"go/token"
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
)

// validateExpr parses src as a Go expression. The returned error is a
// *CodegenError carrying pos when src is empty or fails to parse.
func validateExpr(src string, pos ast.Pos) error {
	if strings.TrimSpace(src) == "" {
		return &CodegenError{Pos: pos, Msg: "invalid Go expression: empty"}
	}
	if _, err := parser.ParseExpr(src); err != nil {
		return newExprError(pos, err)
	}
	return nil
}

// validateStmt parses src as a Go statement by wrapping it in a synthetic
// function body and feeding the result to parser.ParseFile. Used by the
// {@const stmt} form, which carries a short-var-decl rather than an
// expression.
func validateStmt(src string, pos ast.Pos) error {
	if strings.TrimSpace(src) == "" {
		return &CodegenError{Pos: pos, Msg: "invalid Go statement: empty"}
	}
	wrapped := "package p\nfunc f(){\n" + src + "\n}\n"
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "", wrapped, parser.SkipObjectResolution); err != nil {
		return newStmtError(pos, err)
	}
	return nil
}

// emitMustache lowers {expr} in text context. Validates the expression and
// emits w.WriteEscape(expr) verbatim once parsing succeeds.
func emitMustache(b *Builder, m *ast.Mustache) {
	if m == nil {
		return
	}
	if err := validateExpr(m.Expr, m.P); err != nil {
		b.Fail(err)
		return
	}
	b.Linef("w.WriteEscape(%s)", m.Expr)
}

// emitRawHTML lowers {@html expr} to w.WriteRaw. WriteRaw bypasses HTML
// escaping; the user is responsible for sanitizing untrusted input
// upstream (see CONTRIBUTING.md, recommended: bluemonday).
func emitRawHTML(b *Builder, r *ast.RawHTML) {
	if r == nil {
		return
	}
	if err := validateExpr(r.Expr, r.P); err != nil {
		b.Fail(err)
		return
	}
	b.Linef("w.WriteRaw(%s)", r.Expr)
}

// emitConst lowers {@const stmt} to a verbatim Go statement scoped to the
// surrounding fragment. Validation parses the source as if it were the
// body of a synthetic function so short-var-decls and expression-statement
// forms both round-trip.
func emitConst(b *Builder, c *ast.Const) {
	if c == nil {
		return
	}
	if err := validateStmt(c.Stmt, c.P); err != nil {
		b.Fail(err)
		return
	}
	b.Line(c.Stmt)
}

// emitRender lowers {@render snippet(args)} to a method call on the Page
// receiver. The snippet_<name> method is produced by the snippet block
// pass landing in #14; for now codegen emits the call site only.
func emitRender(b *Builder, r *ast.Render) {
	if r == nil {
		return
	}
	if err := validateExpr(r.Expr, r.P); err != nil {
		b.Fail(err)
		return
	}
	name, args, ok := splitRenderCall(r.Expr)
	if !ok {
		b.Linef("// TODO: @render %q (#14 snippet wiring)", r.Expr)
		return
	}
	if args == "" {
		b.Linef("p.snippet_%s(w, ctx)", name)
		return
	}
	b.Linef("p.snippet_%s(w, ctx, %s)", name, args)
}

// splitRenderCall pulls the snippet name and argument list out of a
// {@render} expression like `card(post)`. Anything that isn't an
// identifier-prefixed call falls back to a TODO emit; the AST validator
// already accepted the expression, so the only loss is the snippet
// indirection — runtime errors land in user code, not here.
func splitRenderCall(src string) (string, string, bool) {
	open := strings.Index(src, "(")
	if open <= 0 || !strings.HasSuffix(strings.TrimSpace(src), ")") {
		return "", "", false
	}
	name := strings.TrimSpace(src[:open])
	if !isIdent(name) {
		return "", "", false
	}
	close := strings.LastIndex(src, ")")
	args := strings.TrimSpace(src[open+1 : close])
	return name, args, true
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
