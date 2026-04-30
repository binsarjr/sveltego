package codegen

import (
	"strings"

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

// emitNestedComponent lowers a dot-namespaced component invocation
// `<pkg.Comp attr={expr}/>` to a direct `pkg.Comp{}.Render(w, ctx,
// pkg.CompProps{Attr: expr}, slots)` call. The package alias is the
// first dot-segment of the tag name; the component identifier is the
// remainder. Attribute names are PascalCased into Props field
// references; static attribute values are wrapped in Go string literals
// and dynamic ones flow through verbatim.
//
// Children and slot content forward through Phase 0bb's existing
// slot-lifting helpers so component composition stays uniform.
func emitNestedComponent(b *Builder, e *ast.Element) {
	pkg, comp := splitComponentName(e.Name)
	if pkg == "" || comp == "" {
		b.Fail(&CodegenError{
			Pos: e.P,
			Msg: "component tag <" + e.Name + "> must be `<pkg.Component>`",
		})
		return
	}
	props, err := componentProps(pkg, comp, e.Attributes, e.P)
	if err != nil {
		b.Fail(err)
		return
	}
	letBindings := collectLetBindings(e.Attributes)
	defaultBody, named := liftSlotChildren(e.Children)
	hasSlots := len(named) > 0 || hasNonEmptyBody(defaultBody)

	b.Line("{")
	b.Indent()
	if hasSlots {
		b.Linef("_slots := %s.%sSlots{}", pkg, comp)
		if hasNonEmptyBody(defaultBody) {
			emitSlotClosure(b, "_slots.Default", letBindings, defaultBody)
		}
		for _, ns := range named {
			field := slotFieldName(ns.name)
			emitSlotClosure(b, "_slots."+field, ns.lets, ns.body)
		}
		b.Linef("if err := (%s.%s{}).Render(w, ctx, %s, _slots); err != nil {", pkg, comp, props)
	} else {
		b.Linef("if err := (%s.%s{}).Render(w, ctx, %s, %s.%sSlots{}); err != nil {", pkg, comp, props, pkg, comp)
	}
	b.Indent()
	b.Line("return err")
	b.Dedent()
	b.Line("}")
	b.Dedent()
	b.Line("}")
}

// splitComponentName returns the package alias and component identifier
// from a dot-namespaced tag name. `<button.Comp>` returns ("button",
// "Comp"). Names without a dot return ("", "").
func splitComponentName(name string) (string, string) {
	idx := strings.IndexByte(name, '.')
	if idx <= 0 || idx >= len(name)-1 {
		return "", ""
	}
	return name[:idx], name[idx+1:]
}

// componentProps builds the literal `pkg.CompProps{...}` expression for
// a component invocation. Static attributes lower to Go string literals;
// dynamic attributes thread the user's expression verbatim. Spread
// `{...rest}` attributes are not supported in the MVP — reject with an
// explicit follow-up so users see a clear diagnostic rather than a
// silent miss.
func componentProps(pkg, comp string, attrs []ast.Attribute, pos ast.Pos) (string, error) {
	var fields []string
	for i := range attrs {
		a := &attrs[i]
		switch a.Kind {
		case ast.AttrLet:
			continue
		case ast.AttrEventHandler, ast.AttrBind, ast.AttrUse, ast.AttrClassDirective, ast.AttrStyleDirective:
			continue
		}
		if !isComponentPropName(a.Name) {
			continue
		}
		field := pascalIdent(a.Name)
		switch v := a.Value.(type) {
		case nil:
			fields = append(fields, field+": true")
		case *ast.StaticValue:
			fields = append(fields, field+": "+quoteGo(v.Value))
		case *ast.DynamicValue:
			if err := validateExpr(v.Expr, pos); err != nil {
				return "", err
			}
			fields = append(fields, field+": "+v.Expr)
		case *ast.InterpolatedValue:
			fields = append(fields, field+": "+interpolatedValueExpr(v))
		}
	}
	body := strings.Join(fields, ", ")
	return pkg + "." + comp + "Props{" + body + "}", nil
}

// isComponentPropName drops the `slot` reserved attribute (handled by
// slot lifting) so it never reaches Props mapping.
func isComponentPropName(name string) bool {
	if name == "" {
		return false
	}
	if name == "slot" {
		return false
	}
	return true
}

// pascalIdent converts a Svelte attribute name (kebab-case, snake_case,
// or camelCase) to a PascalCase Go identifier. Hyphens and underscores
// are word separators; the first character of every segment uppercases.
func pascalIdent(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	upper := true
	for i := range len(name) {
		c := name[i]
		if c == '-' || c == '_' {
			upper = true
			continue
		}
		if upper {
			b.WriteByte(upperByte(c))
			upper = false
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// interpolatedValueExpr lowers a quoted-with-mustache attribute value
// like `class="card-{Theme}"` to a `"card-" + Theme` string concat
// expression usable as a Props field initializer. Each Mustache part is
// wrapped in fmt.Sprint when it might not be a string at compile time;
// for the MVP we trust the user to pass a string-typed expression.
func interpolatedValueExpr(v *ast.InterpolatedValue) string {
	var parts []string
	for _, part := range v.Parts {
		switch p := part.(type) {
		case *ast.Text:
			parts = append(parts, quoteGo(p.Value))
		case *ast.Mustache:
			parts = append(parts, p.Expr)
		}
	}
	if len(parts) == 0 {
		return `""`
	}
	return strings.Join(parts, " + ")
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
