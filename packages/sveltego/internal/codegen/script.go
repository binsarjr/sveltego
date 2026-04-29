package codegen

import (
	"bytes"
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"regexp"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
)

// runePrefix is the placeholder substituted in for `$name` rune references
// before go/parser sees the script body. Go does not allow `$` in
// identifiers, so detection works against the rewritten name set instead.
const runePrefix = "__sveltegoRune__"

var runeRegexp = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// scriptOutput is the result of hoisting <script lang="go"> blocks to
// package scope. Imports merge with the framework set; Decls land between
// the import block and `type Page struct{}`. Runes records identifiers
// flagged for downstream rune handling (#43-#47); the slice is intentionally
// not emitted yet.
type scriptOutput struct {
	Imports []string
	Decls   []string
	Runes   []string
}

// extractScripts walks frag for *ast.Script nodes and lowers them. At most
// two scripts are accepted (a regular and a `context="module"` companion);
// both fold into the same package-scope output under the simplified rule
// that loose top-level statements are rejected. Caller is responsible for
// skipping *ast.Script nodes during render-body emission.
func extractScripts(frag *ast.Fragment) (scriptOutput, error) {
	if frag == nil {
		return scriptOutput{}, nil
	}
	scripts := collectScripts(frag.Children)
	if len(scripts) == 0 {
		return scriptOutput{}, nil
	}
	if len(scripts) > 2 {
		return scriptOutput{}, &CodegenError{
			Pos: scripts[2].P,
			Msg: "at most two <script> blocks allowed (one regular plus one context=\"module\")",
		}
	}

	var out scriptOutput
	importSet := map[string]struct{}{}
	runeSet := map[string]struct{}{}

	for _, s := range scripts {
		if s.Lang != "go" {
			return scriptOutput{}, &CodegenError{
				Pos: s.P,
				Msg: fmt.Sprintf("<script> requires lang=\"go\" (got %q)", s.Lang),
			}
		}
		body, runeNames := rewriteRunes(s.Body)
		for _, name := range runeNames {
			runeSet[name] = struct{}{}
		}
		fileSrc := "package _x\n" + body + "\n"
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "", fileSrc, parser.AllErrors|parser.SkipObjectResolution)
		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "expected declaration") {
				return scriptOutput{}, &CodegenError{
					Pos: s.P,
					Msg: "<script> body must contain only imports and top-level declarations",
				}
			}
			return scriptOutput{}, &CodegenError{
				Pos: s.P,
				Msg: fmt.Sprintf("invalid Go in <script>: %v", err),
			}
		}

		for _, decl := range f.Decls {
			gen, ok := decl.(*goast.GenDecl)
			if !ok {
				continue
			}
			if gen.Tok != token.IMPORT {
				continue
			}
			for _, spec := range gen.Specs {
				is, ok := spec.(*goast.ImportSpec)
				if !ok {
					continue
				}
				rendered, err := formatImportSpec(fset, is)
				if err != nil {
					return scriptOutput{}, &CodegenError{Pos: s.P, Msg: err.Error()}
				}
				importSet[rendered] = struct{}{}
			}
		}

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *goast.GenDecl:
				if d.Tok == token.IMPORT {
					continue
				}
				rendered, err := formatNode(fset, d)
				if err != nil {
					return scriptOutput{}, &CodegenError{Pos: s.P, Msg: err.Error()}
				}
				out.Decls = append(out.Decls, rendered)
			case *goast.FuncDecl:
				rendered, err := formatNode(fset, d)
				if err != nil {
					return scriptOutput{}, &CodegenError{Pos: s.P, Msg: err.Error()}
				}
				out.Decls = append(out.Decls, rendered)
			default:
				return scriptOutput{}, &CodegenError{
					Pos: s.P,
					Msg: fmt.Sprintf("<script> body must contain only imports and top-level declarations (got %T)", d),
				}
			}
		}
	}

	for k := range importSet {
		out.Imports = append(out.Imports, k)
	}
	sort.Strings(out.Imports)

	for k := range runeSet {
		out.Runes = append(out.Runes, k)
	}
	sort.Strings(out.Runes)

	return out, nil
}

func collectScripts(nodes []ast.Node) []*ast.Script {
	var out []*ast.Script
	for _, n := range nodes {
		if s, ok := n.(*ast.Script); ok {
			out = append(out, s)
		}
	}
	return out
}

// formatImportSpec returns the canonical `path` or `name "path"` form.
func formatImportSpec(fset *token.FileSet, is *goast.ImportSpec) (string, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, is); err != nil {
		return "", fmt.Errorf("print import: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func formatNode(fset *token.FileSet, n goast.Node) (string, error) {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, n); err != nil {
		return "", fmt.Errorf("print decl: %w", err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// rewriteRunes substitutes `$name` rune references with a placeholder that
// go/parser accepts as a regular identifier. Returns the rewritten body and
// the list of detected rune names (with the leading `$` retained).
func rewriteRunes(body string) (string, []string) {
	if !strings.Contains(body, "$") {
		return body, nil
	}
	seen := map[string]struct{}{}
	rewritten := runeRegexp.ReplaceAllStringFunc(body, func(match string) string {
		name := match[1:]
		seen["$"+name] = struct{}{}
		return runePrefix + name
	})
	if len(seen) == 0 {
		return body, nil
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	return rewritten, names
}

// restoreRunesBytes is the inverse of rewriteRunes applied to formatted
// output so recorded `$rune(...)` calls round-trip into the generated
// source verbatim. The generated file will not compile while the rune
// reference is intact; downstream rune handling in #43-#47 replaces the
// reference with framework calls before output.
func restoreRunesBytes(src []byte) []byte {
	if !bytes.Contains(src, []byte(runePrefix)) {
		return src
	}
	return bytes.ReplaceAll(src, []byte(runePrefix), []byte("$"))
}
