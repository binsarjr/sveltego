package codegen

import (
	"errors"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/internal/routescan"
)

// optionConstNames is the set of exported constant names the scanner
// recognizes inside *.server.go files. Anything else is ignored so the
// user can declare their own package-private constants.
var optionConstNames = map[string]struct{}{
	"Prerender":     {},
	"SSR":           {},
	"CSR":           {},
	"TrailingSlash": {},
}

// scanPageOptions reads path (when present) and returns the page
// options override declared as exported constants. Missing file yields
// the zero value with no error so callers can compose it into the
// cascade unconditionally.
func scanPageOptions(path string) (kit.PageOptionsOverride, error) {
	if path == "" {
		return kit.PageOptionsOverride{}, nil
	}
	src, err := os.ReadFile(path) //nolint:gosec // path is scanner-controlled
	if err != nil {
		if os.IsNotExist(err) {
			return kit.PageOptionsOverride{}, nil
		}
		return kit.PageOptionsOverride{}, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	stripped := stripBuildConstraint(src)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, stripped, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return kit.PageOptionsOverride{}, fmt.Errorf("codegen: parse %s: %w", path, err)
	}

	var out kit.PageOptionsOverride
	for _, decl := range f.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*goast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if _, recognized := optionConstNames[name.Name]; !recognized {
					continue
				}
				if i >= len(vs.Values) {
					return kit.PageOptionsOverride{}, fmt.Errorf("codegen: %s: %s is declared without an initializer", path, name.Name)
				}
				if err := assignOption(&out, name.Name, vs.Values[i], path); err != nil {
					return kit.PageOptionsOverride{}, err
				}
			}
		}
	}
	return out, nil
}

// assignOption maps one (name, expression) pair into the override.
// Booleans accept literal `true`/`false`. TrailingSlash accepts the four
// `kit.TrailingSlash*` selector expressions plus the bare identifiers
// `TrailingSlash*` (in case the user uses a dot import).
func assignOption(out *kit.PageOptionsOverride, name string, expr goast.Expr, path string) error {
	switch name {
	case "Prerender":
		v, err := evalBool(expr)
		if err != nil {
			return fmt.Errorf("codegen: %s: Prerender: %w", path, err)
		}
		out.Prerender = v
		out.HasPrerender = true
	case "SSR":
		v, err := evalBool(expr)
		if err != nil {
			return fmt.Errorf("codegen: %s: SSR: %w", path, err)
		}
		out.SSR = v
		out.HasSSR = true
	case "CSR":
		v, err := evalBool(expr)
		if err != nil {
			return fmt.Errorf("codegen: %s: CSR: %w", path, err)
		}
		out.CSR = v
		out.HasCSR = true
	case "TrailingSlash":
		v, err := evalTrailingSlash(expr)
		if err != nil {
			return fmt.Errorf("codegen: %s: TrailingSlash: %w", path, err)
		}
		out.TrailingSlash = v
		out.HasTrailingSlash = true
	}
	return nil
}

func evalBool(expr goast.Expr) (bool, error) {
	id, ok := expr.(*goast.Ident)
	if !ok {
		return false, errors.New("must be true or false literal")
	}
	switch id.Name {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("must be true or false literal (got %s)", id.Name)
}

// evalTrailingSlash recognizes both `kit.TrailingSlashAlways` (selector)
// and bare `TrailingSlashAlways` (dot import). Anything else is rejected
// at codegen time so a typo does not silently cascade as the zero value.
func evalTrailingSlash(expr goast.Expr) (kit.TrailingSlash, error) {
	switch e := expr.(type) {
	case *goast.SelectorExpr:
		return trailingSlashFromIdent(e.Sel.Name)
	case *goast.Ident:
		return trailingSlashFromIdent(e.Name)
	}
	return 0, errors.New("must be one of kit.TrailingSlashNever, kit.TrailingSlashAlways, kit.TrailingSlashIgnore, kit.TrailingSlashDefault")
}

// resolvePageOptions walks scan.Routes and returns one effective
// PageOptions per route Pattern. The cascade starts at
// kit.DefaultPageOptions(), folds each layout's layout.server.go
// override outer -> inner, then applies the route's own
// page.server.go (or server.go) override last. Layout overrides are
// memoized so chains shared across routes parse once.
func resolvePageOptions(scan *routescan.ScanResult) (map[string]kit.PageOptions, error) {
	if scan == nil {
		return nil, nil
	}
	out := make(map[string]kit.PageOptions, len(scan.Routes))
	layoutCache := make(map[string]kit.PageOptionsOverride)
	for _, r := range scan.Routes {
		base := kit.DefaultPageOptions()
		for i := range r.LayoutChain {
			if i >= len(r.LayoutServerFiles) {
				break
			}
			path := r.LayoutServerFiles[i]
			if path == "" {
				continue
			}
			over, err := loadCached(path, layoutCache)
			if err != nil {
				return nil, err
			}
			base = base.Merge(over)
		}
		var routeFile string
		switch {
		case r.HasPageServer:
			routeFile = filepath.Join(r.Dir, "page.server.go")
		case r.HasServer:
			routeFile = filepath.Join(r.Dir, "server.go")
		}
		if routeFile != "" {
			over, err := scanPageOptions(routeFile)
			if err != nil {
				return nil, err
			}
			base = base.Merge(over)
		}
		out[r.Pattern] = base
	}
	return out, nil
}

func loadCached(path string, cache map[string]kit.PageOptionsOverride) (kit.PageOptionsOverride, error) {
	if v, ok := cache[path]; ok {
		return v, nil
	}
	v, err := scanPageOptions(path)
	if err != nil {
		return kit.PageOptionsOverride{}, err
	}
	cache[path] = v
	return v, nil
}

func trailingSlashFromIdent(name string) (kit.TrailingSlash, error) {
	switch name {
	case "TrailingSlashDefault":
		return kit.TrailingSlashDefault, nil
	case "TrailingSlashNever":
		return kit.TrailingSlashNever, nil
	case "TrailingSlashAlways":
		return kit.TrailingSlashAlways, nil
	case "TrailingSlashIgnore":
		return kit.TrailingSlashIgnore, nil
	}
	return 0, fmt.Errorf("unknown TrailingSlash value %q", name)
}
