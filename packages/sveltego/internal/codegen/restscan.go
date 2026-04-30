package codegen

import (
	"errors"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
)

// restVerbs is the closed set of HTTP verb function names recognized in
// a server.go file. Anything else exported is treated as a build error
// per issue #29 ("Build fails clearly if a +server.go exports an
// unknown verb").
var restVerbs = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"}

// restVerbSet is the lookup form of restVerbs.
var restVerbSet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(restVerbs))
	for _, v := range restVerbs {
		out[v] = struct{}{}
	}
	return out
}()

// errBadRestVerbSig is the canonical error returned when an HTTP verb
// function does not match `func(*kit.RequestEvent) *kit.Response`.
var errBadRestVerbSig = errors.New("HTTP verb handlers must have signature func(ev *kit.RequestEvent) *kit.Response")

// RESTHandlers records which HTTP verbs a route's server.go declares.
// SourcePath is the absolute file path; Verbs is sorted in restVerbs
// order so emitted dispatch tables stay deterministic.
type RESTHandlers struct {
	SourcePath string
	Verbs      []string
}

// Has reports whether verb is in the set.
func (r RESTHandlers) Has(verb string) bool {
	for _, v := range r.Verbs {
		if v == verb {
			return true
		}
	}
	return false
}

// scanRESTHandlers parses one server.go file and returns the recognized
// HTTP verb funcs. Unknown exported funcs are rejected so user typos
// do not silently disappear (e.g. `func Get` lowercased). A missing
// file returns the zero value with no error.
func scanRESTHandlers(path string) (RESTHandlers, error) {
	if path == "" {
		return RESTHandlers{}, nil
	}
	src, err := os.ReadFile(path) //nolint:gosec // path is scanner-controlled
	if err != nil {
		if os.IsNotExist(err) {
			return RESTHandlers{}, nil
		}
		return RESTHandlers{}, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	stripped := stripBuildConstraint(src)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, stripped, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return RESTHandlers{}, fmt.Errorf("codegen: parse %s: %w", path, err)
	}

	out := RESTHandlers{SourcePath: path}
	seen := make(map[string]struct{})
	for _, decl := range f.Decls {
		fn, ok := decl.(*goast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name == nil {
			continue
		}
		name := fn.Name.Name
		if !goast.IsExported(name) {
			continue
		}
		if _, recognized := restVerbSet[name]; !recognized {
			return RESTHandlers{}, fmt.Errorf("codegen: %s: unknown exported function %q (allowed verbs: %v)", path, name, restVerbs)
		}
		if err := validateRESTVerbSig(fn); err != nil {
			return RESTHandlers{}, fmt.Errorf("codegen: %s: %s: %w", path, name, err)
		}
		if _, dup := seen[name]; dup {
			return RESTHandlers{}, fmt.Errorf("codegen: %s: duplicate %s declaration", path, name)
		}
		seen[name] = struct{}{}
		out.Verbs = append(out.Verbs, name)
	}
	sort.Slice(out.Verbs, func(i, j int) bool {
		return verbIndex(out.Verbs[i]) < verbIndex(out.Verbs[j])
	})
	return out, nil
}

// validateRESTVerbSig matches `func(*kit.RequestEvent) *kit.Response` by
// arity. Full type identity is enforced at `go build` time on the
// emitted dispatcher.
func validateRESTVerbSig(fn *goast.FuncDecl) error {
	if paramCount(fn) != 1 || resultCount(fn) != 1 {
		return errBadRestVerbSig
	}
	return nil
}

func verbIndex(verb string) int {
	for i, v := range restVerbs {
		if v == verb {
			return i
		}
	}
	return len(restVerbs)
}
