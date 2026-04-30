package codegen

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/binsarjr/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/internal/routescan"
)

// collectLocalsPrerenderWarnings walks every route in scan and returns
// one diagnostic per ctx.Locals access inside a Load that is reachable
// from a route resolved as Prerender = true. Layout loaders are scanned
// once per file; the same diagnostic does not repeat across descendants.
func collectLocalsPrerenderWarnings(scan *routescan.ScanResult, routeOptions map[string]kit.PageOptions) ([]routescan.Diagnostic, error) {
	if scan == nil {
		return nil, nil
	}
	var diags []routescan.Diagnostic
	seen := map[string]struct{}{}
	for _, r := range scan.Routes {
		opts := routeOptions[r.Pattern]
		if !opts.Prerender {
			continue
		}
		if r.HasPageServer {
			path := filepath.Join(r.Dir, "page.server.go")
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				ds, err := scanLocalsAccessUnderPrerender(path)
				if err != nil {
					return nil, err
				}
				diags = append(diags, ds...)
			}
		}
		for _, layoutServer := range r.LayoutServerFiles {
			if layoutServer == "" {
				continue
			}
			if _, ok := seen[layoutServer]; ok {
				continue
			}
			seen[layoutServer] = struct{}{}
			ds, err := scanLocalsAccessUnderPrerender(layoutServer)
			if err != nil {
				return nil, err
			}
			diags = append(diags, ds...)
		}
	}
	return diags, nil
}

// localsAllowDirective is the line comment users can attach to a Load
// function to silence the prerender Locals warning when they know the
// access is fine (e.g. defaulting a value when the map is empty).
const localsAllowDirective = "//sveltego:allow-locals-prerender"

// scanLocalsAccessUnderPrerender walks a +page.server.go (or
// +layout.server.go) file's Load and LayoutLoad declarations looking for
// reads of ctx.Locals or ev.Locals. Each hit becomes one
// routescan.Diagnostic so the build surfaces the warning to stderr.
//
// Pages declaring //sveltego:allow-locals-prerender on the same Load
// function suppress every diagnostic from that function. The directive
// must sit on the function's doc comment or trailing comment.
func scanLocalsAccessUnderPrerender(path string) ([]routescan.Diagnostic, error) {
	if path == "" {
		return nil, nil
	}
	src, err := os.ReadFile(path) //nolint:gosec // caller-controlled scan path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	stripped := stripBuildConstraint(src)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, stripped, parser.AllErrors|parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("codegen: parse %s: %w", path, err)
	}

	hasDirectiveLine := loadDirectiveLines(f, fset, stripped)

	var diags []routescan.Diagnostic
	for _, decl := range f.Decls {
		fn, ok := decl.(*goast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if fn.Name == nil || (fn.Name.Name != "Load" && fn.Name.Name != "LayoutLoad") {
			continue
		}
		if hasDirectiveLine[fset.Position(fn.Pos()).Line] {
			continue
		}
		ctxName := loadCtxParamName(fn)
		if ctxName == "" {
			continue
		}
		goast.Inspect(fn.Body, func(n goast.Node) bool {
			sel, ok := n.(*goast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*goast.Ident)
			if !ok || id.Name != ctxName {
				return true
			}
			if sel.Sel == nil || sel.Sel.Name != "Locals" {
				return true
			}
			pos := fset.Position(sel.Pos())
			diags = append(diags, routescan.Diagnostic{
				Path:    path,
				Message: fmt.Sprintf("%s: %s.Locals accessed inside Load while route is marked Prerender — Handle hooks do not run during prerender so the map is empty (line %d)", path, ctxName, pos.Line),
				Hint:    "remove the Locals access, gate it on len(ctx.Locals) > 0, or attach " + localsAllowDirective + " to silence this warning",
			})
			return true
		})
	}
	return diags, nil
}

// loadCtxParamName returns the name the user gave the *kit.LoadCtx
// parameter for fn (typically "ctx"). The empty string is returned when
// the parameter is unnamed (`func Load(*kit.LoadCtx)` — codegen would
// reject this anyway).
func loadCtxParamName(fn *goast.FuncDecl) string {
	if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return ""
	}
	first := fn.Type.Params.List[0]
	if len(first.Names) == 0 {
		return ""
	}
	return first.Names[0].Name
}

// loadDirectiveLines returns the set of line numbers (1-indexed) that
// carry the //sveltego:allow-locals-prerender directive immediately
// before a Load or LayoutLoad declaration. The directive must sit on
// the function's doc comment, trailing comment, OR on the line directly
// above the func keyword. We accept all three so users do not have to
// remember go/format's idiosyncratic comment placement rules.
func loadDirectiveLines(f *goast.File, fset *token.FileSet, src []byte) map[int]bool {
	out := map[int]bool{}
	for _, decl := range f.Decls {
		fn, ok := decl.(*goast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name == nil || (fn.Name.Name != "Load" && fn.Name.Name != "LayoutLoad") {
			continue
		}
		funcLine := fset.Position(fn.Pos()).Line
		if fn.Doc != nil {
			for _, c := range fn.Doc.List {
				if strings.TrimSpace(c.Text) == localsAllowDirective {
					out[funcLine] = true
				}
			}
		}
		// Allow a free-floating // line directly preceding the func.
		if funcLine > 1 {
			lines := bytes.Split(src, []byte("\n"))
			if funcLine-2 < len(lines) {
				if strings.TrimSpace(string(lines[funcLine-2])) == localsAllowDirective {
					out[funcLine] = true
				}
			}
		}
	}
	return out
}
