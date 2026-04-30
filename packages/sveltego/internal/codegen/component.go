package codegen

import (
	"github.com/binsarjr/sveltego/internal/ast"
)

// emitSvelteComponent lowers <svelte:component this={Expr} ... /> to a
// runtime dispatch on Expr. The caller passes a value whose type
// satisfies a single-method Render interface; codegen emits a direct
// method call so the dispatch is a virtual call against whatever the
// expression resolves to.
//
// Static-set switching ("this={A}/this={B} based on a switch") is not a
// codegen-time optimization — the user writes the switch in their Go
// scope (a {@const} or a script-hoisted function) and passes the result
// here. That keeps the lowering uniform: one Render call, no per-site
// interface synthesis.
func emitSvelteComponent(b *Builder, e *ast.Element) {
	expr := svelteComponentThis(e)
	if expr == "" {
		b.Fail(&CodegenError{
			Pos: e.P,
			Msg: "<svelte:component> requires a `this={expr}` attribute",
		})
		return
	}
	if err := validateExpr(expr, e.P); err != nil {
		b.Fail(err)
		return
	}
	b.Linef("if err := %s.Render(w); err != nil {", expr)
	b.Indent()
	b.Line("return err")
	b.Dedent()
	b.Line("}")
}

// svelteComponentThis returns the dynamic `this={...}` expression on a
// <svelte:component> element. The static `this="Name"` form is rejected
// upstream by the validator because no SSR-time mapping from string to
// Go value exists in sveltego.
func svelteComponentThis(e *ast.Element) string {
	for i := range e.Attributes {
		a := &e.Attributes[i]
		if a.Name != "this" {
			continue
		}
		if dv, ok := a.Value.(*ast.DynamicValue); ok {
			return dv.Expr
		}
	}
	return ""
}
