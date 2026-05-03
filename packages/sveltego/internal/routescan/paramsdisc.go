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
// the manifest emitter without requiring a project-level
// src/params/<name>/<name>.go.
var builtinMatchers = map[string]struct{}{
	"int":  {},
	"uuid": {},
	"slug": {},
}

// discoverMatchers walks paramsDir and returns one DiscoveredMatcher per
// `src/params/<name>/<name>.go` file that exports `func Match(s string)
// bool`. The expected layout — one matcher per subdirectory — keeps the
// directory name and the package name aligned, so user code can import
// the matcher package without an alias and the codegen registry emit
// can write `<name>.Match` directly.
//
// Files that fail to parse, lack a Match function, carry the wrong
// signature, or whose package name disagrees with the directory name
// are surfaced as diagnostics; valid neighbors still appear in the
// returned slice.
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
		if !e.IsDir() {
			// Tolerate stray non-Go files (READMEs, .DS_Store, …).
			// A flat *.go drop is a legacy layout — surface a hint so
			// the user can migrate.
			if strings.HasSuffix(e.Name(), ".go") &&
				!strings.HasSuffix(e.Name(), "_test.go") {
				name := strings.TrimSuffix(e.Name(), ".go")
				diagnostics = append(diagnostics, Diagnostic{
					Path: filepath.Join(paramsDir, e.Name()),
					Message: fmt.Sprintf(
						"matcher %q must live at src/params/%s/%s.go (one matcher per subdirectory)",
						name, name, name,
					),
					Hint: "move the file into a subdirectory named after the matcher",
				})
			}
			continue
		}
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") {
			continue
		}
		dir := filepath.Join(paramsDir, e.Name())
		m, ds := discoverMatcherDir(dir, e.Name())
		diagnostics = append(diagnostics, ds...)
		if m != nil {
			matchers = append(matchers, *m)
		}
	}

	sort.Slice(matchers, func(i, j int) bool { return matchers[i].Name < matchers[j].Name })
	return matchers, diagnostics
}

// discoverMatcherDir parses src/params/<name>/<name>.go and returns a
// DiscoveredMatcher when the file exports `func Match(s string) bool`.
// Other .go files in the directory (helpers, _test.go) are ignored;
// only the file whose basename matches the directory carries the
// canonical Match symbol.
func discoverMatcherDir(dir, name string) (*DiscoveredMatcher, []Diagnostic) {
	path := filepath.Join(dir, name+".go")
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, []Diagnostic{{
			Path: dir,
			Message: fmt.Sprintf(
				"matcher subdirectory %q is missing %s.go",
				name, name,
			),
			Hint: fmt.Sprintf(
				"create src/params/%s/%s.go with `package %s` and `func Match(s string) bool`",
				name, name, name,
			),
		}}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, []Diagnostic{{
			Path:    path,
			Message: fmt.Sprintf("cannot parse matcher file: %v", err),
		}}
	}

	if file.Name == nil || file.Name.Name != name {
		return nil, []Diagnostic{{
			Path: path,
			Message: fmt.Sprintf(
				"matcher %q has package %q; want package %s",
				name, packageNameOrEmpty(file), name,
			),
			Hint: fmt.Sprintf("rename the package clause to `package %s`", name),
		}}
	}

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

	return &DiscoveredMatcher{
		Name:        name,
		Path:        path,
		PackageName: name,
	}, nil
}

func packageNameOrEmpty(file *ast.File) string {
	if file == nil || file.Name == nil {
		return ""
	}
	return file.Name.Name
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
