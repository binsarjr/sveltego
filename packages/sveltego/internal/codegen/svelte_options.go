package codegen

import (
	"fmt"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

// svelteOptions captures the validated `<svelte:options>` attributes.
// Pointers are used for boolean attributes that distinguish "absent" from
// "explicitly false" — `runes={false}` is a meaningful opt-out, while a
// missing `runes` attribute leaves rune detection on the script.
//
// Prerender is a string union ("", "true", "false", "auto", "protected")
// matching SvelteKit's `<svelte:options prerender>` shape; the empty
// string means the page declared nothing and inherits from its server
// file or layout cascade.
type svelteOptions struct {
	Runes         *bool
	CustomElement string
	Namespace     string
	Accessors     *bool
	Immutable     *bool
	Prerender     string
	Pos           ast.Pos
	Present       bool
}

// extractSvelteOptions finds the (at most one) <svelte:options> element
// at the fragment's top level, validates its attributes, and removes the
// node from the fragment. Nested occurrences are rejected with a position
// error. Unknown attributes are rejected so a typo does not silently
// disable a feature.
func extractSvelteOptions(frag *ast.Fragment) (svelteOptions, error) {
	if frag == nil {
		return svelteOptions{}, nil
	}
	if err := rejectNestedSvelteOptions(frag.Children); err != nil {
		return svelteOptions{}, err
	}

	var opts svelteOptions
	kept := frag.Children[:0]
	for _, child := range frag.Children {
		e, ok := child.(*ast.Element)
		if !ok || e.Name != "svelte:options" {
			kept = append(kept, child)
			continue
		}
		if opts.Present {
			return svelteOptions{}, &CodegenError{
				Pos: e.P,
				Msg: "duplicate <svelte:options>: only one allowed per component",
			}
		}
		if len(e.Children) > 0 {
			return svelteOptions{}, &CodegenError{
				Pos: e.P,
				Msg: "<svelte:options> must not have children",
			}
		}
		if err := parseSvelteOptionAttrs(&opts, e); err != nil {
			return svelteOptions{}, err
		}
		opts.Present = true
		opts.Pos = e.P
	}
	for i := len(kept); i < len(frag.Children); i++ {
		frag.Children[i] = nil
	}
	frag.Children = kept
	return opts, nil
}

// rejectNestedSvelteOptions walks every node reachable from the top-level
// children except the top-level themselves, returning a CodegenError if a
// <svelte:options> appears anywhere below the root.
func rejectNestedSvelteOptions(children []ast.Node) error {
	for _, c := range children {
		e, ok := c.(*ast.Element)
		if !ok {
			continue
		}
		if e.Name == "svelte:options" {
			continue
		}
		if err := rejectNestedSvelteOptionsScan(e.Children); err != nil {
			return err
		}
	}
	return nil
}

func rejectNestedSvelteOptionsScan(nodes []ast.Node) error {
	for _, n := range nodes {
		switch v := n.(type) {
		case *ast.Element:
			if v.Name == "svelte:options" {
				return &CodegenError{
					Pos: v.P,
					Msg: "<svelte:options> must appear at the template root, not inside another element or block",
				}
			}
			if err := rejectNestedSvelteOptionsScan(v.Children); err != nil {
				return err
			}
		case *ast.IfBlock:
			if err := rejectNestedSvelteOptionsScan(v.Then); err != nil {
				return err
			}
			for i := range v.Elifs {
				if err := rejectNestedSvelteOptionsScan(v.Elifs[i].Body); err != nil {
					return err
				}
			}
			if err := rejectNestedSvelteOptionsScan(v.Else); err != nil {
				return err
			}
		case *ast.EachBlock:
			if err := rejectNestedSvelteOptionsScan(v.Body); err != nil {
				return err
			}
			if err := rejectNestedSvelteOptionsScan(v.Else); err != nil {
				return err
			}
		case *ast.AwaitBlock:
			if err := rejectNestedSvelteOptionsScan(v.Pending); err != nil {
				return err
			}
			if err := rejectNestedSvelteOptionsScan(v.Then); err != nil {
				return err
			}
			if err := rejectNestedSvelteOptionsScan(v.Catch); err != nil {
				return err
			}
		case *ast.KeyBlock:
			if err := rejectNestedSvelteOptionsScan(v.Body); err != nil {
				return err
			}
		case *ast.SnippetBlock:
			if err := rejectNestedSvelteOptionsScan(v.Body); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseSvelteOptionAttrs(out *svelteOptions, e *ast.Element) error {
	for i := range e.Attributes {
		a := &e.Attributes[i]
		switch a.Name {
		case "runes":
			v, err := svelteOptionBool(a, e.P)
			if err != nil {
				return err
			}
			out.Runes = &v
		case "customElement":
			s, err := svelteOptionString(a, e.P)
			if err != nil {
				return err
			}
			out.CustomElement = s
		case "namespace":
			s, err := svelteOptionString(a, e.P)
			if err != nil {
				return err
			}
			switch s {
			case "html", "svg", "mathml", "foreign":
			default:
				return &CodegenError{
					Pos: e.P,
					Msg: fmt.Sprintf("<svelte:options> namespace=%q: must be one of html, svg, mathml, foreign", s),
				}
			}
			out.Namespace = s
		case "accessors":
			v, err := svelteOptionBool(a, e.P)
			if err != nil {
				return err
			}
			out.Accessors = &v
		case "immutable":
			v, err := svelteOptionBool(a, e.P)
			if err != nil {
				return err
			}
			out.Immutable = &v
		case "prerender":
			s, err := svelteOptionPrerender(a, e.P)
			if err != nil {
				return err
			}
			out.Prerender = s
		default:
			return &CodegenError{
				Pos: e.P,
				Msg: fmt.Sprintf("<svelte:options>: unknown attribute %q", a.Name),
			}
		}
	}
	return nil
}

// svelteOptionBool resolves an attribute value to a boolean. A bare
// attribute (`<svelte:options accessors />`) reads as true, matching
// HTML's boolean-attribute convention. A static `"true"`/`"false"`
// literal works the same. A `{true}`/`{false}` dynamic literal is also
// accepted because Svelte templates commonly use that form.
func svelteOptionBool(a *ast.Attribute, pos ast.Pos) (bool, error) {
	if a.Value == nil {
		return true, nil
	}
	switch v := a.Value.(type) {
	case *ast.StaticValue:
		switch strings.ToLower(strings.TrimSpace(v.Value)) {
		case "", "true":
			return true, nil
		case "false":
			return false, nil
		}
		return false, &CodegenError{
			Pos: pos,
			Msg: fmt.Sprintf("<svelte:options> %s=%q: expected true or false", a.Name, v.Value),
		}
	case *ast.DynamicValue:
		switch strings.TrimSpace(v.Expr) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
		return false, &CodegenError{
			Pos: pos,
			Msg: fmt.Sprintf("<svelte:options> %s={%s}: expected true or false literal", a.Name, v.Expr),
		}
	}
	return false, &CodegenError{
		Pos: pos,
		Msg: fmt.Sprintf("<svelte:options> %s: expected true or false", a.Name),
	}
}

// svelteOptionPrerender resolves a `prerender` attribute on
// <svelte:options>. A bare attribute is "true"; a static literal must be
// "true", "false", "auto", or "protected". `prerender={true}` and
// `prerender={false}` are tolerated for symmetry with other boolean
// attributes; "auto" and "protected" must be supplied as static strings.
func svelteOptionPrerender(a *ast.Attribute, pos ast.Pos) (string, error) {
	if a.Value == nil {
		return "true", nil
	}
	switch v := a.Value.(type) {
	case *ast.StaticValue:
		s := strings.ToLower(strings.TrimSpace(v.Value))
		switch s {
		case "", "true":
			return "true", nil
		case "false":
			return "false", nil
		case "auto":
			return "auto", nil
		case "protected":
			return "protected", nil
		}
		return "", &CodegenError{
			Pos: pos,
			Msg: fmt.Sprintf("<svelte:options> prerender=%q: must be one of true, false, auto, protected", v.Value),
		}
	case *ast.DynamicValue:
		switch strings.TrimSpace(v.Expr) {
		case "true":
			return "true", nil
		case "false":
			return "false", nil
		}
		return "", &CodegenError{
			Pos: pos,
			Msg: fmt.Sprintf("<svelte:options> prerender={%s}: dynamic values must be the literal true or false; use the string \"auto\" or \"protected\" instead", v.Expr),
		}
	}
	return "", &CodegenError{
		Pos: pos,
		Msg: "<svelte:options> prerender: expected true, false, auto, or protected",
	}
}

// svelteOptionString resolves an attribute value to a static string.
// Dynamic values are rejected — `<svelte:options>` attrs must be known
// at compile time.
func svelteOptionString(a *ast.Attribute, pos ast.Pos) (string, error) {
	if a.Value == nil {
		return "", &CodegenError{
			Pos: pos,
			Msg: fmt.Sprintf("<svelte:options> %s: missing value", a.Name),
		}
	}
	if v, ok := a.Value.(*ast.StaticValue); ok {
		return v.Value, nil
	}
	return "", &CodegenError{
		Pos: pos,
		Msg: fmt.Sprintf("<svelte:options> %s: expected a static string value", a.Name),
	}
}
