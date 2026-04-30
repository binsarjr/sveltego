package codegen

import (
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
)

// emitElement lowers a single Element to its open-tag, child, and
// close-tag sequence. <slot> outlets, <svelte:body|window|document>
// no-ops, <svelte:component> dynamic dispatch, and component invocations
// dispatch through dedicated emitters; on:, bind:, use: directives are
// silently dropped here (lowered elsewhere).
func emitElement(b *Builder, e *ast.Element) {
	if e == nil {
		return
	}
	if e.Name == "slot" {
		emitSlotOutlet(b, e)
		return
	}
	if isSvelteSpecialGlobal(e.Name) {
		if b.nestDepth > 0 {
			b.Fail(&CodegenError{
				Pos: e.P,
				Msg: "<" + e.Name + "> must appear at the template root, not inside another element or block",
			})
			return
		}
		emitSvelteSpecialGlobal(b, e)
		return
	}
	if e.Name == "svelte:component" {
		emitSvelteComponent(b, e)
		return
	}
	if isComponentName(e.Name) {
		if strings.Contains(e.Name, ".") {
			emitNestedComponent(b, e)
			return
		}
		emitComponentCall(b, e)
		return
	}

	emitOpenTag(b, e)

	if isVoidElement(e.Name) {
		return
	}
	if e.SelfClosing {
		emitCloseTag(b, e.Name)
		return
	}
	b.nestDepth++
	emitChildren(b, e.Children)
	b.nestDepth--
	emitCloseTag(b, e.Name)
}

// emitSlotOutlet lowers a <slot/> outlet on the receiving (component or
// layout) side. Layout templates carry a single anonymous `children`
// closure; pages reject slots (placement makes no sense outside a
// component). Component templates handle slots through the dedicated
// component-render emitter (see GenerateComponent in slot.go) which sets
// b.componentMode and consumes the slot directly.
func emitSlotOutlet(b *Builder, e *ast.Element) {
	if b.componentMode {
		emitComponentSlotOutlet(b, e)
		return
	}
	if b.hasChildren {
		b.Line("if children != nil {")
		b.Indent()
		b.Line("if err := children(w); err != nil {")
		b.Indent()
		b.Line("return err")
		b.Dedent()
		b.Line("}")
		b.Dedent()
		b.Line("}")
		return
	}
	b.Line("// TODO: <slot /> outside layout (#49 named slots)")
}

func emitOpenTag(b *Builder, e *ast.Element) {
	var head strings.Builder
	head.WriteByte('<')
	head.WriteString(e.Name)

	parts := partitionAttrs(e.Attributes)
	wantClassWrap := len(parts.classDirs) > 0

	for _, a := range parts.emit {
		emitAttribute(b, &head, a)
	}

	if wantClassWrap {
		head.WriteString(` class="`)
		if parts.hasStaticClass {
			head.WriteString(parts.staticClass)
		}
		flushHead(b, &head)
		for _, d := range parts.classDirs {
			b.Linef("if %s {", d.Expr)
			b.Indent()
			b.Linef("w.WriteString(%s)", quoteGo(" "+d.Modifier))
			b.Dedent()
			b.Line("}")
		}
		head.WriteString(`"`)
	}

	if len(parts.styleDirs) > 0 {
		head.WriteString(` style="`)
		flushHead(b, &head)
		for _, d := range parts.styleDirs {
			b.Linef("if %s != \"\" {", d.Expr)
			b.Indent()
			b.Linef("w.WriteString(%s)", quoteGo(d.Modifier+":"))
			b.Linef("w.WriteEscapeAttr(%s)", d.Expr)
			b.Line(`w.WriteString(";")`)
			b.Dedent()
			b.Line("}")
		}
		head.WriteString(`"`)
	}

	head.WriteByte('>')
	flushHead(b, &head)
}

func emitCloseTag(b *Builder, name string) {
	b.Linef("w.WriteString(%s)", quoteGo("</"+name+">"))
}

// emitAttribute appends one attribute to the in-progress head buffer,
// breaking out into its own emission when the value is dynamic.
func emitAttribute(b *Builder, head *strings.Builder, a *ast.Attribute) {
	switch a.Kind {
	case ast.AttrEventHandler, ast.AttrBind, ast.AttrUse:
		// TODO: directive handlers (#34, #36, #38).
		return
	case ast.AttrClassDirective, ast.AttrStyleDirective:
		return
	}

	if a.Value == nil {
		head.WriteByte(' ')
		head.WriteString(a.Name)
		return
	}

	switch v := a.Value.(type) {
	case *ast.StaticValue:
		head.WriteByte(' ')
		head.WriteString(a.Name)
		head.WriteString(`="`)
		head.WriteString(v.Value)
		head.WriteString(`"`)
	case *ast.DynamicValue:
		if isBooleanAttr(a.Name) {
			flushHead(b, head)
			b.Linef("if %s {", v.Expr)
			b.Indent()
			b.Linef("w.WriteString(%s)", quoteGo(" "+a.Name))
			b.Dedent()
			b.Line("}")
			return
		}
		head.WriteByte(' ')
		head.WriteString(a.Name)
		head.WriteString(`="`)
		flushHead(b, head)
		b.Linef("w.WriteEscapeAttr(%s)", v.Expr)
		head.WriteString(`"`)
	case *ast.InterpolatedValue:
		head.WriteByte(' ')
		head.WriteString(a.Name)
		head.WriteString(`="`)
		flushHead(b, head)
		for _, part := range v.Parts {
			switch p := part.(type) {
			case *ast.Text:
				if p.Value != "" {
					b.Linef("w.WriteString(%s)", quoteGo(escapeAttrLiteral(p.Value)))
				}
			case *ast.Mustache:
				b.Linef("w.WriteEscapeAttr(%s)", p.Expr)
			}
		}
		head.WriteString(`"`)
	}
}

type directive struct {
	Modifier string
	Expr     string
}

// attrPartition bins an element's attribute list into the slices and
// flags emitOpenTag needs in a single pass. emit holds attributes
// destined for the head buffer in source order, with class:/style:
// directives stripped.
type attrPartition struct {
	emit           []*ast.Attribute
	classDirs      []directive
	styleDirs      []directive
	staticClass    string
	hasStaticClass bool
}

func partitionAttrs(attrs []ast.Attribute) attrPartition {
	p := attrPartition{emit: make([]*ast.Attribute, 0, len(attrs))}
	staticClassIdx := -1
	for i := range attrs {
		a := &attrs[i]
		switch a.Kind {
		case ast.AttrClassDirective:
			if expr := dynamicExpr(a.Value); expr != "" {
				p.classDirs = append(p.classDirs, directive{Modifier: a.Modifier, Expr: expr})
			}
			continue
		case ast.AttrStyleDirective:
			if expr := dynamicExpr(a.Value); expr != "" {
				p.styleDirs = append(p.styleDirs, directive{Modifier: a.Modifier, Expr: expr})
			}
			continue
		}
		if a.Name == "class" && a.Kind == ast.AttrStatic && !p.hasStaticClass {
			if s, ok := a.Value.(*ast.StaticValue); ok {
				p.staticClass = s.Value
				p.hasStaticClass = true
				staticClassIdx = len(p.emit)
			}
		}
		p.emit = append(p.emit, a)
	}
	// When class directives wrap the class attribute, the literal class=
	// emit is folded into the wrapper; drop it from the head pass.
	if len(p.classDirs) > 0 && staticClassIdx >= 0 {
		p.emit = append(p.emit[:staticClassIdx], p.emit[staticClassIdx+1:]...)
	}
	return p
}

func dynamicExpr(v ast.AttributeValue) string {
	d, ok := v.(*ast.DynamicValue)
	if !ok {
		return ""
	}
	return d.Expr
}

func flushHead(b *Builder, head *strings.Builder) {
	if head.Len() == 0 {
		return
	}
	b.Linef("w.WriteString(%s)", quoteGo(head.String()))
	head.Reset()
}

// escapeAttrLiteral escapes the bytes that matter inside a double-quoted
// attribute value emitted as a literal: `"` and `&`.
func escapeAttrLiteral(s string) string {
	if !strings.ContainsAny(s, `"&`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := range len(s) {
		c := s[i]
		switch c {
		case '"':
			b.WriteString("&#34;")
		case '&':
			b.WriteString("&amp;")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// isComponentName mirrors the parser's rule (uppercase first char or
// contains a dot, but not a `svelte:*` namespace). Duplicated here so the
// codegen package does not depend on parser internals.
func isComponentName(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, "svelte:") {
		return false
	}
	if strings.Contains(name, ".") {
		return true
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

// isVoidElement matches the HTML void element list. Self-closing slashes
// in source are ignored; output never carries a slash.
func isVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input",
		"link", "meta", "source", "track", "wbr":
		return true
	}
	return false
}

// isBooleanAttr lists HTML attributes whose presence (not value) toggles
// behavior. Codegen emits a guarded WriteString rather than a quoted value
// for these when the source binds a dynamic expression.
func isBooleanAttr(name string) bool {
	switch name {
	case "allowfullscreen", "async", "autofocus", "autoplay", "checked",
		"controls", "default", "defer", "disabled", "formnovalidate",
		"hidden", "ismap", "itemscope", "loop", "multiple", "muted",
		"nomodule", "novalidate", "open", "playsinline", "readonly",
		"required", "reversed", "selected":
		return true
	}
	return false
}
