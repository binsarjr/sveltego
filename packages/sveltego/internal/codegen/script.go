package codegen

import (
	"bytes"
	"fmt"
	goast "go/ast"
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
// flagged for downstream rune handling (#43-#47); the slice is retained
// for diagnostics. Props, RestField, and RuneStmts capture the structured
// rune analysis: Props feeds emit of the Props struct + defaultProps
// helper, and RuneStmts is emitted into the Render body before template
// lowering so $state / $derived locals are in scope.
type scriptOutput struct {
	Imports   []string
	Decls     []string
	Runes     []string
	Props     []runeProp
	RestField string
	RuneStmts []runeStmt
	HasProps  bool
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

		ana, err := analyzeRunes(body, s.P)
		if err != nil {
			return scriptOutput{}, err
		}
		for _, imp := range ana.Imports {
			importSet[imp] = struct{}{}
		}
		out.Decls = append(out.Decls, ana.Decls...)
		if ana.HasProps {
			if out.HasProps {
				return scriptOutput{}, &CodegenError{Pos: s.P, Msg: "<script>: only one $props() destructure allowed"}
			}
			out.HasProps = true
			out.Props = ana.Props
			out.RestField = ana.RestField
		}
		out.RuneStmts = append(out.RuneStmts, ana.Stmts...)
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
