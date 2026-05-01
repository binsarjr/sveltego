package typegen

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

// RouteShape is the walker's output: the inferred set of fields on the
// data type declared by the route's Load function. NamedTypes carries
// any auxiliary struct declarations referenced from the root shape so
// the emitter can hoist them as TypeScript interfaces.
type RouteShape struct {
	Fields     []Field
	NamedTypes []NamedType
}

// Field is one entry on the data interface. Name is the JSON-style
// property name (post JSON-tag resolution); TSType is the rendered
// TypeScript type for the field. GoName preserves the original Go
// struct field identifier so SSR property-access lowering (Phase 5,
// #427) can rewrite `data.<json>` to `data.<GoName>` without a second
// AST walk. GoType is the original Go type expression as written in
// the source (e.g. "string", "*User", "[]Post"); the lowerer reads its
// pointer-ness, slice-ness, and named-type root for nested chains.
type Field struct {
	Name    string
	TSType  string
	GoName  string
	GoType  string
	Pointer bool
	Slice   bool
	// NamedType is the root struct identifier when the field's Go type
	// resolves to (or wraps) a top-level struct in the same file. Empty
	// for primitives, maps, generics, or anonymous nested structs.
	NamedType string
}

// NamedType is a top-level Go struct declaration referenced from the
// route's data shape. The emitter renders it as a sibling TypeScript
// interface so the rendered `.d.ts` is self-contained.
type NamedType struct {
	Name   string
	Fields []Field
}

// walkServerFile parses srcPath, locates Load, and pulls its data
// shape out as a [RouteShape]. typeName is "PageData" for pages or
// "LayoutData" for layouts; it drives the named-type-decl detection
// path. The shape may also originate from an inline anonymous struct
// in the Load return signature, mirroring the Mustache-Go pagedata
// inference.
func walkServerFile(srcPath, typeName string) (RouteShape, []Diagnostic, error) {
	src, err := os.ReadFile(srcPath) //nolint:gosec // path is caller-controlled
	if err != nil {
		return RouteShape{}, nil, fmt.Errorf("typegen: read %s: %w", srcPath, err)
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, srcPath, src, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return RouteShape{}, nil, fmt.Errorf("typegen: parse %s: %w", srcPath, err)
	}

	loadFn := findLoadFunc(f)
	if loadFn == nil {
		return RouteShape{}, []Diagnostic{{Path: srcPath, Message: "no Load function found; emitting empty " + typeName}}, nil
	}

	returnType := firstReturnType(loadFn)
	if returnType == nil {
		return RouteShape{}, []Diagnostic{{Path: srcPath, Message: "Load has no return type; emitting empty " + typeName}}, nil
	}

	rootStruct, namedRoot := resolveStruct(f, returnType, typeName)
	if rootStruct == nil {
		// Named return like `(PageData, error)` — locate the type decl.
		if namedRoot != "" {
			rootStruct = findStructDecl(f, namedRoot)
		}
		if rootStruct == nil {
			return RouteShape{}, []Diagnostic{{Path: srcPath, Message: "Load return type is not a struct; emitting empty " + typeName}}, nil
		}
	}

	resolver := &structResolver{
		file:      f,
		filePath:  srcPath,
		named:     map[string]NamedType{},
		namedKeys: nil,
	}
	fields, diags := resolver.fieldsFromStruct(rootStruct)
	shape := RouteShape{
		Fields:     fields,
		NamedTypes: resolver.collected(),
	}
	return shape, diags, nil
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

// firstReturnType returns the first declared result type of fn, or
// nil when fn has no results. The error result (always second) is
// ignored.
func firstReturnType(fn *goast.FuncDecl) goast.Expr {
	if fn == nil || fn.Type == nil || fn.Type.Results == nil {
		return nil
	}
	if len(fn.Type.Results.List) == 0 {
		return nil
	}
	return fn.Type.Results.List[0].Type
}

// resolveStruct unwraps the return type to its underlying struct.
// Returns either the struct literal directly or a non-empty namedRoot
// to look up via findStructDecl. Pointers and parenthesized types are
// stripped; only the canonical inline struct + named-type cases are
// supported here.
func resolveStruct(f *goast.File, expr goast.Expr, expectedName string) (*goast.StructType, string) {
	switch t := expr.(type) {
	case *goast.StructType:
		return t, ""
	case *goast.StarExpr:
		return resolveStruct(f, t.X, expectedName)
	case *goast.ParenExpr:
		return resolveStruct(f, t.X, expectedName)
	case *goast.Ident:
		// `func Load(...) (PageData, error)` — caller resolves via type decl.
		_ = expectedName
		return nil, t.Name
	}
	return nil, ""
}

// findStructDecl walks the file's top-level type declarations for a
// `type <name> struct{...}` and returns the struct body. Returns nil
// when no such declaration is found or when the named type aliases a
// non-struct type.
func findStructDecl(f *goast.File, name string) *goast.StructType {
	for _, decl := range f.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*goast.TypeSpec)
			if !ok || ts.Name == nil || ts.Name.Name != name {
				continue
			}
			st, ok := ts.Type.(*goast.StructType)
			if !ok {
				return nil
			}
			return st
		}
	}
	return nil
}

// structResolver tracks the in-progress walk so referenced named
// struct types are inlined into [RouteShape.NamedTypes] in
// declaration order without duplicates. The walker is single-pass and
// not concurrency-safe; one resolver per file.
type structResolver struct {
	file      *goast.File
	filePath  string
	named     map[string]NamedType
	namedKeys []string
}

func (r *structResolver) collected() []NamedType {
	out := make([]NamedType, 0, len(r.namedKeys))
	for _, k := range r.namedKeys {
		out = append(out, r.named[k])
	}
	return out
}

// fieldsFromStruct walks struct fields, resolves names from JSON tags
// (or the lowercase-first fallback with a warning), and maps each
// field type to its TypeScript form.
func (r *structResolver) fieldsFromStruct(st *goast.StructType) ([]Field, []Diagnostic) {
	if st == nil || st.Fields == nil {
		return nil, nil
	}
	var fields []Field
	var diags []Diagnostic
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			// Embedded field — Phase 2 ignores; Phase 3 may revisit.
			continue
		}
		jsonName, hasTag := jsonTagName(f.Tag)
		ts, fdiags := r.mapType(f.Type)
		diags = append(diags, fdiags...)
		isPtr, isSlice, named, goSrc := describeGoType(f.Type)
		for _, n := range f.Names {
			if !n.IsExported() {
				continue
			}
			name := jsonName
			if name == "" {
				name = lowerFirst(n.Name)
				if !hasTag {
					diags = append(diags, Diagnostic{
						Path: r.filePath,
						Message: fmt.Sprintf("field %s has no json tag; defaulted to %q",
							n.Name, name),
					})
				}
			}
			if name == "-" {
				continue
			}
			fields = append(fields, Field{
				Name:      name,
				TSType:    ts,
				GoName:    n.Name,
				GoType:    goSrc,
				Pointer:   isPtr,
				Slice:     isSlice,
				NamedType: named,
			})
		}
	}
	return fields, diags
}

// describeGoType extracts shape-affecting facts from a Go type
// expression: whether it's a pointer at the outer layer, whether it's
// a slice/array, the named-struct root identifier (when the type is —
// or wraps — a same-file struct), and a stable string rendering of the
// expression suitable for diagnostics. The walk strips one layer of
// `*`/`[]` to get to the named identifier; deeper compositions
// (`[]*T`, `*[]T`) still report the leaf named type for the lowerer's
// chain walk.
func describeGoType(expr goast.Expr) (pointer, slice bool, named, src string) {
	src = renderGoType(expr)
	cur := expr
	for {
		switch t := cur.(type) {
		case *goast.StarExpr:
			pointer = true
			cur = t.X
			continue
		case *goast.ArrayType:
			slice = true
			cur = t.Elt
			continue
		case *goast.ParenExpr:
			cur = t.X
			continue
		case *goast.Ident:
			named = t.Name
			return
		}
		return
	}
}

// renderGoType produces a stable, gofmt-style string for a Go type
// expression. The shape covers the cases the SSR lowerer needs to
// surface in error messages: primitives, pointers, slices, named
// types, selectors, and a generic `<expr>` placeholder for anything
// else so the diagnostic stays useful even on exotic types.
func renderGoType(expr goast.Expr) string {
	switch t := expr.(type) {
	case *goast.Ident:
		return t.Name
	case *goast.StarExpr:
		return "*" + renderGoType(t.X)
	case *goast.ArrayType:
		return "[]" + renderGoType(t.Elt)
	case *goast.SelectorExpr:
		if id, ok := t.X.(*goast.Ident); ok && t.Sel != nil {
			return id.Name + "." + t.Sel.Name
		}
		return "<selector>"
	case *goast.MapType:
		return "map[" + renderGoType(t.Key) + "]" + renderGoType(t.Value)
	case *goast.ParenExpr:
		return renderGoType(t.X)
	}
	return "<expr>"
}

// recordNamedType registers a referenced struct under name and walks
// its fields. Subsequent references to the same name reuse the
// captured form. Used by mapper.go when an `*ast.Ident` resolves to a
// top-level struct declaration in the same file.
func (r *structResolver) recordNamedType(name string) {
	if _, ok := r.named[name]; ok {
		return
	}
	st := findStructDecl(r.file, name)
	if st == nil {
		return
	}
	// Reserve the slot before recursing so a self-referential type does
	// not loop forever. The slot is filled with an empty record first.
	r.named[name] = NamedType{Name: name}
	r.namedKeys = append(r.namedKeys, name)
	fields, _ := r.fieldsFromStruct(st)
	r.named[name] = NamedType{Name: name, Fields: fields}
}

// jsonTagName extracts the JSON property name from a `json:"name,..."`
// struct tag. Returns ("", false) when no json tag is present, ("-",
// true) for skipped fields, or the bare name otherwise.
func jsonTagName(tag *goast.BasicLit) (string, bool) {
	if tag == nil {
		return "", false
	}
	raw := strings.Trim(tag.Value, "`")
	for _, pair := range strings.Fields(raw) {
		if !strings.HasPrefix(pair, "json:") {
			continue
		}
		val := strings.TrimPrefix(pair, "json:")
		val = strings.Trim(val, "\"")
		if val == "" {
			return "", true
		}
		// Strip ",omitempty" etc.
		if i := strings.IndexByte(val, ','); i >= 0 {
			val = val[:i]
		}
		return val, true
	}
	return "", false
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = lowerRune(r[0])
	return string(r)
}

func lowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}
