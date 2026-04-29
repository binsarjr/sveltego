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

// emitPageDataStruct writes `type PageData struct{...}`. Empty fields produce
// the zero-field form.
func emitPageDataStruct(b *Builder, fields []pageDataField) {
	if len(fields) == 0 {
		b.Line("type PageData struct{}")
		return
	}
	b.Line("type PageData struct {")
	b.Indent()
	for _, fd := range fields {
		b.Linef("%s %s", fd.Name, fd.Type)
	}
	b.Dedent()
	b.Line("}")
}
