package codegen

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

// validateExpr parses src as a Go expression and runs the env codegen
// guard against the resulting AST. The returned error is a *CodegenError
// carrying pos when src is empty, fails to parse, or references a private
// env accessor (env.StaticPrivate / env.DynamicPrivate) inside a template
// expression.
func validateExpr(src string, pos ast.Pos) error {
	if strings.TrimSpace(src) == "" {
		return &CodegenError{Pos: pos, Msg: "invalid Go expression: empty"}
	}
	expr, err := parser.ParseExpr(src)
	if err != nil {
		return newExprError(pos, err)
	}
	if err := checkPrivateEnv(expr, pos); err != nil {
		return err
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

// emitRender lowers {@render expr} to a call against the local snippet
// closure produced by emitSnippetBlock. The writer is appended as the
// final argument so invocation reads `name(args, w)`. Closures returning
// error propagate via `if err := name(...); err != nil { return err }`.
//
// When the expression is not an identifier-prefixed call (e.g. snippet
// passed as prop and rendered through a method selector), the writer is
// still appended as the tail arg.
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
		b.Fail(&CodegenError{
			Pos: r.P,
			Msg: fmt.Sprintf("{@render} expects a callable expression, got %q", r.Expr),
		})
		return
	}
	call := name + "(" + appendWriter(args) + ")"
	b.Linef("if err := %s; err != nil {", call)
	b.Indent()
	b.Line("return err")
	b.Dedent()
	b.Line("}")
}

// splitRenderCall pulls the snippet name and argument list out of a
// {@render} expression like `card(post)`. The name segment is everything
// before the matching opening paren; it must be a Go expression itself
// (identifier, selector, or chain). Validation is delegated to
// parser.ParseExpr via validateExpr — this helper only structurally
// separates the call.
func splitRenderCall(src string) (string, string, bool) {
	src = strings.TrimSpace(src)
	if !strings.HasSuffix(src, ")") {
		return "", "", false
	}
	open := strings.IndexByte(src, '(')
	if open <= 0 {
		return "", "", false
	}
	name := strings.TrimSpace(src[:open])
	if name == "" {
		return "", "", false
	}
	close := strings.LastIndexByte(src, ')')
	args := strings.TrimSpace(src[open+1 : close])
	return name, args, true
}

// appendWriter formats the lowered argument list. The implicit writer
// (`w`) is appended as the trailing argument so call sites read like a
// normal Go function call with the writer threaded through.
func appendWriter(args string) string {
	if args == "" {
		return "w"
	}
	return args + ", w"
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
