package mcp

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// lookupKitSymbol parses every .go file under kitDir (skipping _test.go
// and build-tagged files) and returns a formatted entry for the named
// symbol. The lookup matches the bare identifier against funcs, types,
// type-bound methods, vars, consts.
func lookupKitSymbol(kitDir, symbol string) (string, error) {
	if kitDir == "" {
		return "", errors.New("kit directory not configured")
	}
	if _, err := os.Stat(kitDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("kit directory not found: %s", kitDir)
		}
		return "", fmt.Errorf("stat kit dir: %w", err)
	}

	pkg, fset, err := parseKitPackage(kitDir)
	if err != nil {
		return "", err
	}

	docPkg := doc.New(pkg, "github.com/binsarjr/sveltego/exports/kit", doc.AllDecls)

	for _, f := range docPkg.Funcs {
		if f.Name == symbol {
			return formatFunc(fset, f), nil
		}
	}
	for _, t := range docPkg.Types {
		if t.Name == symbol {
			return formatType(fset, t), nil
		}
		for _, m := range t.Methods {
			if m.Name == symbol || (t.Name+"."+m.Name) == symbol {
				return formatFunc(fset, m), nil
			}
		}
		for _, fn := range t.Funcs {
			if fn.Name == symbol {
				return formatFunc(fset, fn), nil
			}
		}
	}
	for _, v := range docPkg.Vars {
		for _, name := range v.Names {
			if name == symbol {
				return formatValue(fset, v, "var"), nil
			}
		}
	}
	for _, c := range docPkg.Consts {
		for _, name := range c.Names {
			if name == symbol {
				return formatValue(fset, c, "const"), nil
			}
		}
	}
	return "", fmt.Errorf("symbol %q not found in kit package", symbol)
}

// parseKitPackage parses every non-test .go file under dir and returns
// an ast.Package suitable for go/doc. ast.Package is deprecated in
// favour of go/types but remains the only input go/doc.New accepts, so
// we use it deliberately here.
//
//nolint:staticcheck // ast.Package required by go/doc.New
func parseKitPackage(dir string) (*ast.Package, *token.FileSet, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		name := fi.Name()
		if strings.HasSuffix(name, "_test.go") {
			return false
		}
		return strings.HasSuffix(name, ".go")
	}, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse kit package: %w", err)
	}
	if pkg, ok := pkgs["kit"]; ok {
		return pkg, fset, nil
	}
	for _, p := range pkgs {
		return p, fset, nil
	}
	return nil, nil, fmt.Errorf("no Go package found under %s", dir)
}

func formatFunc(fset *token.FileSet, f *doc.Func) string {
	var b strings.Builder
	fmt.Fprintf(&b, "func %s\n\n", f.Name)
	b.WriteString("```go\n")
	b.WriteString(printNode(fset, f.Decl))
	b.WriteString("\n```\n")
	if f.Doc != "" {
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(f.Doc))
		b.WriteString("\n")
	}
	return b.String()
}

func formatType(fset *token.FileSet, t *doc.Type) string {
	var b strings.Builder
	fmt.Fprintf(&b, "type %s\n\n", t.Name)
	b.WriteString("```go\n")
	b.WriteString(printNode(fset, t.Decl))
	b.WriteString("\n```\n")
	if t.Doc != "" {
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(t.Doc))
		b.WriteString("\n")
	}
	if len(t.Methods) > 0 {
		b.WriteString("\nMethods:\n")
		for _, m := range t.Methods {
			fmt.Fprintf(&b, "- %s\n", m.Name)
		}
	}
	return b.String()
}

func formatValue(fset *token.FileSet, v *doc.Value, kind string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n\n", kind, strings.Join(v.Names, ", "))
	b.WriteString("```go\n")
	b.WriteString(printNode(fset, v.Decl))
	b.WriteString("\n```\n")
	if v.Doc != "" {
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(v.Doc))
		b.WriteString("\n")
	}
	return b.String()
}

func printNode(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
	if err := cfg.Fprint(&buf, fset, node); err != nil {
		return fmt.Sprintf("// printer error: %v", err)
	}
	return buf.String()
}

// readExample concatenates the source text of every regular file under
// playgrounds/<name>/, capped at 100KB total. Output is fenced per-file
// with a path header.
func readExample(playgroundsDir, name string) (string, error) {
	if playgroundsDir == "" {
		return "", errors.New("playgrounds directory not configured")
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.Contains(clean, "..") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid example name: %s", name)
	}
	root := filepath.Join(playgroundsDir, clean)
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("example not found: %s", name)
		}
		return "", fmt.Errorf("stat example: %w", err)
	}

	const maxBytes = 100 * 1024
	var b strings.Builder
	var truncated bool
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".gen" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if b.Len() >= maxBytes {
			truncated = true
			return fs.SkipAll
		}
		body, err := os.ReadFile(p) //nolint:gosec // playground source path
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		rel, err := filepath.Rel(playgroundsDir, p)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "### %s\n\n```\n", filepath.ToSlash(rel))
		remaining := maxBytes - b.Len()
		if remaining <= 0 {
			truncated = true
			return fs.SkipAll
		}
		if len(body) > remaining {
			b.Write(body[:remaining])
			truncated = true
		} else {
			b.Write(body)
		}
		b.WriteString("\n```\n\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	if truncated {
		b.WriteString("\n…(truncated at 100KB)\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}
