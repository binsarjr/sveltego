package routescan

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// builtinMatchers names the matchers shipped in kit/params and resolved by
// the manifest emitter without requiring a project-level src/params/<name>.go.
var builtinMatchers = map[string]struct{}{
	"int":  {},
	"uuid": {},
	"slug": {},
}

// discoverMatchers walks paramsDir non-recursively and returns one
// DiscoveredMatcher per *.go file that exports `func Match(s string) bool`.
// Files that fail to parse, lack a Match function, or carry the wrong
// signature are surfaced as diagnostics; valid neighbors still appear in
// the returned slice.
func discoverMatchers(paramsDir string) ([]DiscoveredMatcher, []Diagnostic) {
	if paramsDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(paramsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, []Diagnostic{{
			Path:    paramsDir,
			Message: fmt.Sprintf("cannot read params directory: %v", err),
		}}
	}

	var (
		matchers    []DiscoveredMatcher
		diagnostics []Diagnostic
	)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(paramsDir, e.Name())
		m, ds := parseMatcherFile(path)
		diagnostics = append(diagnostics, ds...)
		if m != nil {
			matchers = append(matchers, *m)
		}
	}

	sort.Slice(matchers, func(i, j int) bool { return matchers[i].Name < matchers[j].Name })
	return matchers, diagnostics
}

func parseMatcherFile(path string) (*DiscoveredMatcher, []Diagnostic) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, []Diagnostic{{
			Path:    path,
			Message: fmt.Sprintf("cannot parse matcher file: %v", err),
		}}
	}

	name := strings.TrimSuffix(filepath.Base(path), ".go")
	fn := findFunc(file, "Match")
	if fn == nil {
		return nil, []Diagnostic{{
			Path:    path,
			Message: fmt.Sprintf("matcher %q is missing func Match(s string) bool", name),
			Hint:    "add `func Match(s string) bool { ... }` so the router can invoke this matcher",
		}}
	}
	if !hasMatchSignature(fn) {
		return nil, []Diagnostic{{
			Path:    path,
			Message: fmt.Sprintf("matcher %q has wrong signature for Match", name),
			Hint:    "expected `func Match(s string) bool`",
		}}
	}

	return &DiscoveredMatcher{Name: name, Path: path}, nil
}

func findFunc(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Recv != nil {
			continue
		}
		if fn.Name != nil && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func hasMatchSignature(fn *ast.FuncDecl) bool {
	t := fn.Type
	if t == nil || t.Params == nil || t.Results == nil {
		return false
	}
	if len(t.Params.List) != 1 || len(t.Results.List) != 1 {
		return false
	}
	param := t.Params.List[0]
	if len(param.Names) != 1 {
		return false
	}
	if !isIdent(param.Type, "string") {
		return false
	}
	result := t.Results.List[0]
	if len(result.Names) != 0 {
		return false
	}
	return isIdent(result.Type, "bool")
}

func isIdent(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}
