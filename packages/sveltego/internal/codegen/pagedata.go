package codegen

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

type pageDataField struct {
	Name string
	Type string
}

type pageDataResult struct {
	Fields  []pageDataField
	Imports []string
	// HasNamedType is true when the user's server file declares
	// `type PageData struct{...}` at package scope (instead of returning
	// an inline anonymous struct). The codegen emits a `type PageData =
	// <mirror>.PageData` alias in that case so the runtime type assertion
	// in the manifest adapter sees the same Go type the user authored.
	// See #109, the standalone-scaffold variant of #143.
	HasNamedType bool
}

type importBinding struct {
	Path  string
	Alias string
}

// inferPageData parses serverFilePath, locates Load(), and reads the
// page's data shape. See inferDataShape for the two supported forms.
func inferPageData(serverFilePath string) (pageDataResult, error) {
	return inferDataShape(serverFilePath, "PageData")
}

// inferLayoutData parses serverFilePath, locates Load(), and reads the
// layout's data shape. Mirrors inferPageData but keys on a `LayoutData`
// named-type declaration instead.
func inferLayoutData(serverFilePath string) (pageDataResult, error) {
	return inferDataShape(serverFilePath, "LayoutData")
}

// inferDataShape parses serverFilePath, locates Load(), and reads its
// data shape. Two source forms are supported:
//
//  1. Inline anonymous struct return — `return struct{...}{...}, nil`.
//     The struct fields are extracted and the caller emits a `type
//     <Data> = struct{...}` alias whose fields mirror them.
//  2. Named-type declaration — a top-level `type <Data> struct{...}`
//     accompanies a `func Load(...) (<Data>, error)`. The caller emits
//     a `type <Data> = <mirror>.<Data>` alias instead, preserving type
//     identity with the user-authored type so the manifest's adapter
//     assertion succeeds.
//
// Missing files and unrecognized return shapes produce a zero result;
// the caller falls back to the empty `type <Data> = struct{}` form.
// dataTypeName selects between page (PageData) and layout (LayoutData).
func inferDataShape(serverFilePath, dataTypeName string) (pageDataResult, error) {
	if serverFilePath == "" {
		return pageDataResult{}, nil
	}
	src, err := os.ReadFile(serverFilePath) //nolint:gosec // path is caller-controlled
	if err != nil {
		if os.IsNotExist(err) {
			return pageDataResult{}, nil
		}
		return pageDataResult{}, fmt.Errorf("read %s: %w", serverFilePath, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, serverFilePath, src, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return pageDataResult{}, fmt.Errorf("parse %s: %w", serverFilePath, err)
	}

	loadFn := findLoadFunc(f)
	if loadFn == nil {
		return pageDataResult{}, nil
	}

	// Case 2: user declared `type <Data> ...` at package scope. The gen
	// file aliases to the mirrored type rather than synthesizing a fresh
	// inline struct. Detected first because a named-type Load return
	// collides with the inline-struct branch (lit.Type is an Ident, not
	// StructType).
	if hasTypeDecl(f, dataTypeName) {
		return pageDataResult{HasNamedType: true}, nil
	}

	lit := findFirstReturnComposite(loadFn)
	if lit == nil {
		return pageDataResult{}, nil
	}
	st, ok := lit.Type.(*goast.StructType)
	if !ok {
		return pageDataResult{}, nil
	}

	imports := serverFileImports(f)
	used := map[string]struct{}{}
	var fields []pageDataField
	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue
		}
		typeStr, err := formatNode(fset, field.Type)
		if err != nil {
			return pageDataResult{}, err
		}
		collectSelectorPackages(field.Type, used)
		for _, name := range field.Names {
			fields = append(fields, pageDataField{Name: name.Name, Type: typeStr})
		}
	}

	var imps []string
	for pkg := range used {
		bind, ok := imports[pkg]
		if !ok {
			continue
		}
		if bind.Alias != "" {
			imps = append(imps, fmt.Sprintf("%s %q", bind.Alias, bind.Path))
		} else {
			imps = append(imps, fmt.Sprintf("%q", bind.Path))
		}
	}
	sort.Strings(imps)

	return pageDataResult{Fields: fields, Imports: imps}, nil
}

// hasTypeDecl reports whether the file declares a top-level type with
// the given name (struct, alias, or any other type spec). Codegen uses
// this to switch between the inline-struct alias form and the
// mirror-import alias form. The check is purely structural — the type
// body is not validated; the user-authored declaration is the source of
// truth and the mirror compiles independently.
func hasTypeDecl(f *goast.File, name string) bool {
	for _, decl := range f.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*goast.TypeSpec)
			if !ok || ts.Name == nil {
				continue
			}
			if ts.Name.Name == name {
				return true
			}
		}
	}
	return false
}

func findLoadFunc(f *goast.File) *goast.FuncDecl {
	for _, decl := range f.Decls {
		fn, ok := decl.(*goast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name != nil && fn.Name.Name == "Load" {
			return fn
		}
	}
	return nil
}

func findFirstReturnComposite(fn *goast.FuncDecl) *goast.CompositeLit {
	if fn == nil || fn.Body == nil {
		return nil
	}
	var found *goast.CompositeLit
	goast.Inspect(fn.Body, func(n goast.Node) bool {
		if found != nil {
			return false
		}
		ret, ok := n.(*goast.ReturnStmt)
		if !ok {
			return true
		}
		if len(ret.Results) == 0 {
			return true
		}
		lit, ok := ret.Results[0].(*goast.CompositeLit)
		if !ok {
			return true
		}
		found = lit
		return false
	})
	return found
}

// serverFileImports indexes the server file's import declarations by the
// identifier each import binds (alias or last path segment).
func serverFileImports(f *goast.File) map[string]importBinding {
	out := map[string]importBinding{}
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, "\"")
		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		key := alias
		if key == "" {
			key = path
			if i := strings.LastIndex(key, "/"); i >= 0 {
				key = key[i+1:]
			}
		}
		out[key] = importBinding{Path: path, Alias: alias}
	}
	return out
}

// collectSelectorPackages walks expr for `pkg.Ident` selectors and records
// the package idents so caller can map them to import paths.
func collectSelectorPackages(expr goast.Expr, set map[string]struct{}) {
	goast.Inspect(expr, func(n goast.Node) bool {
		sel, ok := n.(*goast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*goast.Ident)
		if !ok {
			return true
		}
		set[id.Name] = struct{}{}
		return true
	})
}

// dropField returns a new slice with any field whose Name matches name removed.
func dropField(fields []pageDataField, name string) []pageDataField {
	var out []pageDataField
	for _, f := range fields {
		if f.Name != name {
			out = append(out, f)
		}
	}
	return out
}

// emitPageDataStruct writes the page's PageData type alias. Three shapes:
//
//  1. mirrorAlias != "" — the user's server file declares a named
//     `type PageData ...`, mirrored into the usersrc tree. Emits
//     `type PageData = <mirrorAlias>.PageData`. Type identity flows
//     through the alias so the manifest adapter's runtime assertion
//     against the user's authored type succeeds.
//  2. mirrorAlias == "" and len(fields) > 0 — the user's Load() returns
//     an inline anonymous struct literal whose fields were inferred.
//     Emits `type PageData = struct{...}` mirroring those fields.
//  3. Both empty — Load() returns nothing inferable. Emits the
//     zero-field alias `type PageData = struct{}`.
//
// The alias `=` is load-bearing in cases 2 and 3 too: it preserves type
// identity between the user's anonymous struct value and PageData. A
// named type (no `=`) would force a value conversion at the wire
// boundary. See #109.
func emitPageDataStruct(b *Builder, fields []pageDataField, mirrorAlias string) {
	if mirrorAlias != "" {
		b.Linef("type PageData = %s.PageData", mirrorAlias)
		return
	}
	if len(fields) == 0 {
		b.Line("type PageData = struct{}")
		return
	}
	b.Line("type PageData = struct {")
	b.Indent()
	for _, fd := range fields {
		b.Linef("%s %s", fd.Name, fd.Type)
	}
	b.Dedent()
	b.Line("}")
}

// emitPropsStruct writes the component's Props struct declaration plus a
// defaultProps helper that fills in any field whose corresponding $props
// destructure default was a non-zero literal. Empty props produce
// nothing — callers consult HasProps to decide whether to emit at all.
// Bindable fields are tagged on the field via `kit:"bindable"` so the
// client codegen pass (covered by #34) can pick them up without a
// separate metadata table.
func emitPropsStruct(b *Builder, props []runeProp) {
	if len(props) == 0 {
		return
	}
	b.Line("type Props struct {")
	b.Indent()
	for _, p := range props {
		tag := propFieldTag(p)
		if tag != "" {
			b.Linef("%s %s %s", p.Name, p.Type, tag)
		} else {
			b.Linef("%s %s", p.Name, p.Type)
		}
	}
	b.Dedent()
	b.Line("}")
	b.Line("")
	emitDefaultProps(b, props)
}

func propFieldTag(p runeProp) string {
	parts := []string{}
	if p.Bindable {
		parts = append(parts, "bindable")
	}
	if p.Rest {
		parts = append(parts, "rest")
	}
	if len(parts) == 0 {
		return ""
	}
	return "`kit:\"" + strings.Join(parts, ",") + "\"`"
}

// emitDefaultProps writes a defaultProps(p *Props) helper that applies
// every rune-supplied default value when the corresponding field is at
// its Go zero value. When no field carries a default the helper is a
// no-op stub; we always emit it so Render's signature stays uniform.
func emitDefaultProps(b *Builder, props []runeProp) {
	b.Line("func defaultProps(p *Props) {")
	b.Indent()
	emitted := false
	for _, p := range props {
		if p.Default == "" || p.Rest {
			continue
		}
		zero := zeroLiteralForType(p.Type)
		if zero == "" {
			b.Linef("p.%s = %s", p.Name, p.Default)
			emitted = true
			continue
		}
		b.Linef("if p.%s == %s {", p.Name, zero)
		b.Indent()
		b.Linef("p.%s = %s", p.Name, p.Default)
		b.Dedent()
		b.Line("}")
		emitted = true
	}
	if !emitted {
		b.Line("_ = p")
	}
	b.Dedent()
	b.Line("}")
}

// zeroLiteralForType returns the textual zero literal for a small set of
// builtin Go types so emitDefaultProps can guard `if p.X == zero {}`.
// Types not in the set are treated as opaque — the caller falls back to
// unconditional assignment so the default still wins on a fresh struct.
func zeroLiteralForType(typ string) string {
	switch typ {
	case "string":
		return `""`
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"byte", "rune", "uintptr":
		return "0"
	case "float32", "float64":
		return "0"
	case "bool":
		return "false"
	}
	return ""
}
