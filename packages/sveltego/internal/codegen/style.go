package codegen

import (
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/codegen/css"
)

// styleInfo captures the at-most-one <style> block extracted from a
// fragment. ScopeClass holds the resolved `svelte-<hash>` class name.
// Body is the raw CSS text — emitted verbatim into the SSR output until
// transform-pipe support lands.
type styleInfo struct {
	Body       string
	ScopeClass string
	Pos        ast.Pos
	Present    bool
}

// extractStyle finds the (at most one) <style> block in frag's top-level
// children, computes its scope class, and removes the node from the
// fragment so emitChildren no longer dispatches to *ast.Style. Filename
// is the source path used for the class hash; an empty filename falls
// back to the CSS body, matching upstream Svelte's default cssHash rule.
func extractStyle(frag *ast.Fragment, filename string) (styleInfo, error) {
	if frag == nil {
		return styleInfo{}, nil
	}
	var info styleInfo
	kept := frag.Children[:0]
	for _, child := range frag.Children {
		s, ok := child.(*ast.Style)
		if !ok {
			kept = append(kept, child)
			continue
		}
		if info.Present {
			return styleInfo{}, &CodegenError{
				Pos: s.P,
				Msg: "duplicate <style> block: only one <style> per component",
			}
		}
		info.Present = true
		info.Body = s.Body
		info.Pos = s.P
		info.ScopeClass = css.ScopeClass(filename, s.Body)
	}
	for i := len(kept); i < len(frag.Children); i++ {
		frag.Children[i] = nil
	}
	frag.Children = kept
	return info, nil
}

// applyScopeClass walks every regular HTML element in nodes and ensures
// the class attribute carries scope. Components, slot outlets, and the
// svelte:* special elements are skipped — scoped classes are an HTML
// concern. The walk is recursive over Element children, IfBlock branches,
// EachBlock body/else, AwaitBlock branches, KeyBlock body, and
// SnippetBlock body so every element reachable from the fragment receives
// the class.
//
// MVP simplification (#54): every regular element gets the scope class
// when a <style> block exists, regardless of selector match. The full
// selector-target matching algorithm is filed as a follow-up.
func applyScopeClass(nodes []ast.Node, scope string) {
	if scope == "" {
		return
	}
	for _, n := range nodes {
		applyScopeClassNode(n, scope)
	}
}

func applyScopeClassNode(n ast.Node, scope string) {
	switch v := n.(type) {
	case *ast.Element:
		if shouldScope(v) {
			addScopeClass(v, scope)
		}
		applyScopeClass(v.Children, scope)
	case *ast.IfBlock:
		applyScopeClass(v.Then, scope)
		for i := range v.Elifs {
			applyScopeClass(v.Elifs[i].Body, scope)
		}
		applyScopeClass(v.Else, scope)
	case *ast.EachBlock:
		applyScopeClass(v.Body, scope)
		applyScopeClass(v.Else, scope)
	case *ast.AwaitBlock:
		applyScopeClass(v.Pending, scope)
		applyScopeClass(v.Then, scope)
		applyScopeClass(v.Catch, scope)
	case *ast.KeyBlock:
		applyScopeClass(v.Body, scope)
	case *ast.SnippetBlock:
		applyScopeClass(v.Body, scope)
	}
}

// shouldScope reports whether e is a regular HTML element that should
// receive the scope class. Components, slot outlets, svelte:* specials,
// and svelte:component/element dispatchers are excluded.
func shouldScope(e *ast.Element) bool {
	if e.Component {
		return false
	}
	if e.Name == "" {
		return false
	}
	if e.Name == "slot" {
		return false
	}
	if strings.HasPrefix(e.Name, "svelte:") {
		return false
	}
	return true
}

// addScopeClass merges scope into e's class attribute. If a static class
// attribute exists, the scope is appended space-separated. If a dynamic
// class= or interpolated class= exists, scope is added as a sibling
// static class attribute (HTML allows multiple class attributes only
// loosely — for SSR we extend the existing one). When no class attribute
// exists, a static class="<scope>" is appended.
func addScopeClass(e *ast.Element, scope string) {
	for i := range e.Attributes {
		a := &e.Attributes[i]
		if a.Name != "class" {
			continue
		}
		switch v := a.Value.(type) {
		case *ast.StaticValue:
			if v.Value == "" {
				v.Value = scope
				return
			}
			if hasClassToken(v.Value, scope) {
				return
			}
			v.Value = v.Value + " " + scope
			return
		case *ast.InterpolatedValue:
			v.Parts = append(v.Parts, &ast.Text{Value: " " + scope})
			return
		case *ast.DynamicValue:
			a.Value = &ast.InterpolatedValue{
				P: a.P,
				Parts: []ast.Node{
					&ast.Mustache{P: a.P, Expr: v.Expr},
					&ast.Text{Value: " " + scope},
				},
			}
			a.Kind = ast.AttrDynamic
			return
		}
	}
	e.Attributes = append(e.Attributes, ast.Attribute{
		P:     e.P,
		Name:  "class",
		Kind:  ast.AttrStatic,
		Value: &ast.StaticValue{P: e.P, Value: scope},
	})
}

func hasClassToken(value, token string) bool {
	if value == token {
		return true
	}
	for _, t := range strings.Fields(value) {
		if t == token {
			return true
		}
	}
	return false
}

// emitStyleBlock writes the <style> block to the SSR output verbatim.
// The block is emitted at the end of the fragment's render so the scope
// class on already-emitted elements is in scope by the time the browser
// applies the rules. Server-side scope rewriting of selectors is deferred
// to the same follow-up that owns selector-target matching.
func emitStyleBlock(b *Builder, info styleInfo) {
	if !info.Present {
		return
	}
	b.Linef("w.WriteString(%s)", quoteGo("<style>"+info.Body+"</style>"))
}
