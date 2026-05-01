package codegen

import (
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

// A11y rule codes. Stable identifiers prefixed onto Diagnostic.Message so
// IDE/CLI tooling can route, dedupe, or suppress findings by code.
const (
	A11yImgAlt        = "a11y/img-alt"
	A11yAnchorContent = "a11y/anchor-has-content"
	A11yButtonContent = "a11y/button-has-content"
	A11yInputLabel    = "a11y/label-has-associated-control"
	A11yHTMLLang      = "a11y/html-has-lang"
	A11yRoleValid     = "a11y/aria-role-valid"
)

// RunA11yChecks walks frag and returns advisory accessibility findings.
// Diagnostics are non-fatal: the caller decides whether to print, log, or
// (with an opt-in flag) promote them to errors. Results are sorted by
// position so output is deterministic across runs.
//
// Codes are encoded as a "code: message" prefix on Diagnostic.Message so
// the function reuses the existing codegen Diagnostic shape without
// extending its struct.
func RunA11yChecks(frag *ast.Fragment) []Diagnostic {
	if frag == nil {
		return nil
	}
	v := &a11yVisitor{}
	ast.Walk(v, frag)
	sort.SliceStable(v.diags, func(i, j int) bool {
		a, b := v.diags[i].Pos, v.diags[j].Pos
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	return v.diags
}

type a11yVisitor struct {
	diags []Diagnostic
}

func (v *a11yVisitor) Visit(n ast.Node) ast.Visitor {
	if n == nil {
		return v
	}
	el, ok := n.(*ast.Element)
	if !ok {
		return v
	}
	v.checkElement(el)
	return v
}

func (v *a11yVisitor) checkElement(el *ast.Element) {
	switch el.Name {
	case "img":
		v.checkImgAlt(el)
	case "a":
		v.checkAnchorContent(el)
	case "button":
		v.checkButtonContent(el)
	case "input":
		v.checkInputLabel(el)
	case "html":
		v.checkHTMLLang(el)
	}
	v.checkRoleValid(el)
}

func (v *a11yVisitor) emit(pos ast.Pos, sev DiagnosticSeverity, code, msg string) {
	v.diags = append(v.diags, Diagnostic{
		Pos:      pos,
		Severity: sev,
		Message:  code + ": " + msg,
	})
}

func (v *a11yVisitor) checkImgAlt(el *ast.Element) {
	if _, ok := findAttr(el, "alt"); !ok {
		v.emit(el.P, DiagWarning, A11yImgAlt, "<img> requires an `alt` attribute (use alt=\"\" for decorative images)")
	}
}

func (v *a11yVisitor) checkAnchorContent(el *ast.Element) {
	if hasAccessibleName(el) {
		return
	}
	v.emit(el.P, DiagWarning, A11yAnchorContent, "<a> needs visible text or an `aria-label`")
}

func (v *a11yVisitor) checkButtonContent(el *ast.Element) {
	if hasAccessibleName(el) {
		return
	}
	v.emit(el.P, DiagWarning, A11yButtonContent, "<button> needs visible text or an `aria-label`")
}

func (v *a11yVisitor) checkInputLabel(el *ast.Element) {
	if inputIsExempt(el) {
		return
	}
	if hasAttr(el, "aria-label") || hasAttr(el, "aria-labelledby") || hasAttr(el, "title") {
		return
	}
	if hasAttr(el, "id") {
		return
	}
	v.emit(el.P, DiagWarning, A11yInputLabel, "<input> needs an associated <label> or `aria-label`")
}

func (v *a11yVisitor) checkHTMLLang(el *ast.Element) {
	if hasAttr(el, "lang") {
		return
	}
	v.emit(el.P, DiagInfo, A11yHTMLLang, "<html> should declare a `lang` attribute")
}

func (v *a11yVisitor) checkRoleValid(el *ast.Element) {
	attr, ok := findAttr(el, "role")
	if !ok {
		return
	}
	sv, ok := attr.Value.(*ast.StaticValue)
	if !ok {
		return
	}
	role := strings.TrimSpace(sv.Value)
	if role == "" {
		v.emit(attr.P, DiagWarning, A11yRoleValid, "`role` attribute is empty")
		return
	}
	if !isValidARIARole(role) {
		v.emit(attr.P, DiagWarning, A11yRoleValid, "`role=\""+role+"\"` is not a valid ARIA role")
	}
}

func findAttr(el *ast.Element, name string) (*ast.Attribute, bool) {
	for i := range el.Attributes {
		a := &el.Attributes[i]
		if a.Name != name {
			continue
		}
		switch a.Kind {
		case ast.AttrStatic, ast.AttrDynamic:
			return a, true
		}
	}
	return nil, false
}

func hasAttr(el *ast.Element, name string) bool {
	_, ok := findAttr(el, name)
	return ok
}

// hasAccessibleName reports whether el carries a visible label — either an
// aria-label / aria-labelledby / title attribute, or non-whitespace text /
// dynamic content nested within its children.
func hasAccessibleName(el *ast.Element) bool {
	if hasAttr(el, "aria-label") || hasAttr(el, "aria-labelledby") || hasAttr(el, "title") {
		return true
	}
	return hasMeaningfulChild(el.Children)
}

func hasMeaningfulChild(nodes []ast.Node) bool {
	for _, n := range nodes {
		switch v := n.(type) {
		case *ast.Text:
			if strings.TrimSpace(v.Value) != "" {
				return true
			}
		case *ast.Mustache, *ast.RawHTML, *ast.Render:
			return true
		case *ast.Element:
			if v.Name == "img" {
				if a, ok := findAttr(v, "alt"); ok {
					if sv, ok := a.Value.(*ast.StaticValue); ok && strings.TrimSpace(sv.Value) != "" {
						return true
					}
					if _, ok := a.Value.(*ast.DynamicValue); ok {
						return true
					}
				}
			}
			if hasAttr(v, "aria-label") || hasAttr(v, "aria-labelledby") || hasAttr(v, "title") {
				return true
			}
			if hasMeaningfulChild(v.Children) {
				return true
			}
		case *ast.IfBlock:
			if hasMeaningfulChild(v.Then) || hasMeaningfulChild(v.Else) {
				return true
			}
			for _, br := range v.Elifs {
				if hasMeaningfulChild(br.Body) {
					return true
				}
			}
		case *ast.EachBlock:
			if hasMeaningfulChild(v.Body) || hasMeaningfulChild(v.Else) {
				return true
			}
		case *ast.AwaitBlock:
			if hasMeaningfulChild(v.Pending) || hasMeaningfulChild(v.Then) || hasMeaningfulChild(v.Catch) {
				return true
			}
		case *ast.KeyBlock:
			if hasMeaningfulChild(v.Body) {
				return true
			}
		}
	}
	return false
}

// inputIsExempt skips inputs that do not require a visible label by HTML
// spec convention: hidden, submit, reset, image, button.
func inputIsExempt(el *ast.Element) bool {
	attr, ok := findAttr(el, "type")
	if !ok {
		return false
	}
	sv, ok := attr.Value.(*ast.StaticValue)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(sv.Value)) {
	case "hidden", "submit", "reset", "image", "button":
		return true
	}
	return false
}

// isValidARIARole reports whether role is a recognized WAI-ARIA 1.2 role.
// Abstract roles (e.g. `widget`, `composite`) are excluded — they are not
// permitted in author markup.
func isValidARIARole(role string) bool {
	_, ok := ariaRoles[role]
	return ok
}

var ariaRoles = map[string]struct{}{
	"alert":            {},
	"alertdialog":      {},
	"application":      {},
	"article":          {},
	"banner":           {},
	"blockquote":       {},
	"button":           {},
	"caption":          {},
	"cell":             {},
	"checkbox":         {},
	"code":             {},
	"columnheader":     {},
	"combobox":         {},
	"complementary":    {},
	"contentinfo":      {},
	"definition":       {},
	"deletion":         {},
	"dialog":           {},
	"directory":        {},
	"document":         {},
	"emphasis":         {},
	"feed":             {},
	"figure":           {},
	"form":             {},
	"generic":          {},
	"grid":             {},
	"gridcell":         {},
	"group":            {},
	"heading":          {},
	"img":              {},
	"insertion":        {},
	"link":             {},
	"list":             {},
	"listbox":          {},
	"listitem":         {},
	"log":              {},
	"main":             {},
	"marquee":          {},
	"math":             {},
	"menu":             {},
	"menubar":          {},
	"menuitem":         {},
	"menuitemcheckbox": {},
	"menuitemradio":    {},
	"meter":            {},
	"navigation":       {},
	"none":             {},
	"note":             {},
	"option":           {},
	"paragraph":        {},
	"presentation":     {},
	"progressbar":      {},
	"radio":            {},
	"radiogroup":       {},
	"region":           {},
	"row":              {},
	"rowgroup":         {},
	"rowheader":        {},
	"scrollbar":        {},
	"search":           {},
	"searchbox":        {},
	"separator":        {},
	"slider":           {},
	"spinbutton":       {},
	"status":           {},
	"strong":           {},
	"subscript":        {},
	"superscript":      {},
	"switch":           {},
	"tab":              {},
	"table":            {},
	"tablist":          {},
	"tabpanel":         {},
	"term":             {},
	"textbox":          {},
	"time":             {},
	"timer":            {},
	"toolbar":          {},
	"tooltip":          {},
	"tree":             {},
	"treegrid":         {},
	"treeitem":         {},
}
