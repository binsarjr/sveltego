package codegen

import (
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
)

// emitElement lowers a single Element to its open-tag, child, and
// close-tag sequence. Components and svelte:* names are deferred via TODO
// markers; on:, bind:, use: directives are silently dropped.
func emitElement(b *Builder, e *ast.Element) {
	if e == nil {
		return
	}
	if isComponentName(e.Name) {
		b.Linef("// TODO: component <%s> (#10 component nesting)", e.Name)
		b.Line("_ = w")
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
	emitChildren(b, e.Children)
	emitCloseTag(b, e.Name)
}

func emitOpenTag(b *Builder, e *ast.Element) {
	var head strings.Builder
	head.WriteByte('<')
	head.WriteString(e.Name)

	staticClass, hasStaticClass := extractStaticClass(e.Attributes)
	classDirectives := collectClassDirectives(e.Attributes)
	wantClassWrap := len(classDirectives) > 0
	styleDirectives := collectStyleDirectives(e.Attributes)

	for i := range e.Attributes {
		a := &e.Attributes[i]
		if shouldSkipAttr(a, wantClassWrap) {
			continue
		}
		emitAttribute(b, &head, a)
	}

	if wantClassWrap {
		head.WriteString(` class="`)
		if hasStaticClass {
			head.WriteString(staticClass)
		}
		flushHead(b, &head)
		for _, d := range classDirectives {
			b.Linef("if %s {", d.Expr)
			b.Indent()
			b.Linef("w.WriteString(%s)", quoteGo(" "+d.Modifier))
			b.Dedent()
			b.Line("}")
		}
		head.WriteString(`"`)
	}

	if len(styleDirectives) > 0 {
		head.WriteString(` style="`)
		flushHead(b, &head)
		for _, d := range styleDirectives {
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

func collectClassDirectives(attrs []ast.Attribute) []directive {
	var out []directive
	for i := range attrs {
		a := &attrs[i]
		if a.Kind != ast.AttrClassDirective {
			continue
		}
		expr := dynamicExpr(a.Value)
		if expr == "" {
			continue
		}
		out = append(out, directive{Modifier: a.Modifier, Expr: expr})
	}
	return out
}

func collectStyleDirectives(attrs []ast.Attribute) []directive {
	var out []directive
	for i := range attrs {
		a := &attrs[i]
		if a.Kind != ast.AttrStyleDirective {
			continue
		}
		expr := dynamicExpr(a.Value)
		if expr == "" {
			continue
		}
		out = append(out, directive{Modifier: a.Modifier, Expr: expr})
	}
	return out
}

func dynamicExpr(v ast.AttributeValue) string {
	d, ok := v.(*ast.DynamicValue)
	if !ok {
		return ""
	}
	return d.Expr
}

// extractStaticClass returns the literal value of a static class="..."
// attribute when present so a class directive can append to it inside the
// same class quote pair.
func extractStaticClass(attrs []ast.Attribute) (string, bool) {
	for i := range attrs {
		a := &attrs[i]
		if a.Name != "class" || a.Kind != ast.AttrStatic {
			continue
		}
		s, ok := a.Value.(*ast.StaticValue)
		if !ok {
			return "", false
		}
		return s.Value, true
	}
	return "", false
}

// shouldSkipAttr decides whether to emit an attribute directly. When class
// directives wrap the class attribute, the literal class= attribute is
// folded into the wrapper instead of emitted on its own.
func shouldSkipAttr(a *ast.Attribute, wantClassWrap bool) bool {
	switch a.Kind {
	case ast.AttrClassDirective, ast.AttrStyleDirective:
		return true
	}
	if wantClassWrap && a.Name == "class" && a.Kind == ast.AttrStatic {
		if _, ok := a.Value.(*ast.StaticValue); ok {
			return true
		}
	}
	return false
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
