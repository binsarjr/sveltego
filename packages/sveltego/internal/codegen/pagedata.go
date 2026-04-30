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
}

type importBinding struct {
	Path  string
	Alias string
}

// inferPageData parses serverFilePath, locates Load(), and reads its first
// return composite literal. Only inline struct literals (struct{...}{...})
// produce field inference. Named-type returns and missing files yield a
// zero result, leaving the caller to emit `type PageData struct{}`.
func inferPageData(serverFilePath string) (pageDataResult, error) {
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

// emitPageDataStruct writes `type PageData = struct{...}` as a type alias
// (note the `=`). The alias preserves type identity between the user's
// inline anonymous struct literal returned by Load() and PageData, so the
// manifest adapter's `data.(PageData)` assertion succeeds. A new named
// type would require an explicit value conversion at the wire boundary.
// See #109. Empty fields produce the zero-field alias form.
func emitPageDataStruct(b *Builder, fields []pageDataField) {
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
