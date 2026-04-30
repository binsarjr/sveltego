package codegen

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
)

// ActionScan summarizes form-action discovery in one page.server.go.
// HasActions is true when the file declares `var Actions = ...`. Names
// collects best-effort action keys read off a literal ActionMap
// initializer; dynamically-keyed maps yield an empty Names slice and the
// runtime dispatcher resolves keys at request time.
type ActionScan struct {
	HasActions bool
	Names      []string
}

// scanActions reads path and reports whether it declares Actions and
// which keys the literal initializer (when present) contains. A missing
// or empty path returns the zero value with no error.
func scanActions(path string) (ActionScan, error) {
	if path == "" {
		return ActionScan{}, nil
	}
	src, err := os.ReadFile(path) //nolint:gosec // path is scanner-controlled
	if err != nil {
		if os.IsNotExist(err) {
			return ActionScan{}, nil
		}
		return ActionScan{}, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	stripped := stripBuildConstraint(src)

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, stripped, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return ActionScan{}, fmt.Errorf("codegen: parse %s: %w", path, err)
	}

	var out ActionScan
	for _, decl := range f.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*goast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if name == nil || name.Name != "Actions" {
					continue
				}
				out.HasActions = true
				if i < len(vs.Values) {
					if lit, ok := vs.Values[i].(*goast.CompositeLit); ok {
						out.Names = append(out.Names, literalActionKeys(lit)...)
					}
				}
			}
		}
	}
	if len(out.Names) > 0 {
		sort.Strings(out.Names)
		out.Names = dedupSorted(out.Names)
	}
	return out, nil
}

func literalActionKeys(lit *goast.CompositeLit) []string {
	var out []string
	for _, el := range lit.Elts {
		kv, ok := el.(*goast.KeyValueExpr)
		if !ok {
			continue
		}
		bl, ok := kv.Key.(*goast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			continue
		}
		// strip surrounding quotes; rely on go/parser having validated them
		val := bl.Value
		if len(val) >= 2 {
			val = val[1 : len(val)-1]
		}
		out = append(out, val)
	}
	return out
}

func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
