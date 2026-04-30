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

// snapshotExportRegexp matches a `snapshot` symbol exported from a
// <script module> block, as either a binding declaration
// (`export const snapshot`, `let`, `var`) or a re-export (`export {
// snapshot ... }`). Comments are stripped by stripJSComments before
// matching so a commented-out export does not register.
var snapshotExportRegexp = regexp.MustCompile(
	`(?m)export\s+(?:const|let|var)\s+snapshot\b|export\s*\{[^}]*\bsnapshot\b[^}]*\}`,
)

// detectSnapshotExport reports whether body declares a `snapshot` export.
// It is intentionally conservative — false positives only happen when a
// comment-stripper miss leaves an `export ... snapshot` token visible.
func detectSnapshotExport(body string) bool {
	return snapshotExportRegexp.MatchString(stripJSComments(body))
}

// detectFragmentSnapshot walks frag for a `<script module>` block and
// reports whether it declares a `snapshot` export. Used by emitPage to
// flag the route for the SPA router's snapshot wiring.
func detectFragmentSnapshot(frag *ast.Fragment) bool {
	if frag == nil {
		return false
	}
	for _, n := range frag.Children {
		s, ok := n.(*ast.Script)
		if !ok || !s.Module {
			continue
		}
		if detectSnapshotExport(s.Body) {
			return true
		}
	}
	return false
}

// stripJSComments removes // line comments and /* */ block comments from
// a JS source. It is naive (no template-literal or regex-literal
// awareness) which is enough for the snapshot-detection use case where
// the only goal is to avoid matching commented-out exports.
func stripJSComments(src string) string {
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				return b.String()
			}
			i += j
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			j := strings.Index(src[i:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 2
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// scriptOutput is the result of hoisting <script lang="go"> blocks to
// package scope. Imports merge with the framework set; Decls land between
// the import block and `type Page struct{}`. Runes records identifiers
// flagged for downstream rune handling (#43-#47); the slice is retained
// for diagnostics. Props, RestField, and RuneStmts capture the structured
// rune analysis: Props feeds emit of the Props struct + defaultProps
// helper, and RuneStmts is emitted into the Render body before template
// lowering so $state / $derived locals are in scope.
//
// ModuleScript captures the body of an optional `<script module>` block
// (Svelte 5 module-context script). Sveltego does not hoist it to Go —
// it stays in the source `.svelte` file for the Vite Svelte plugin to
// compile, and downstream codegen reads HasSnapshot to wire the SPA
// router's snapshot hooks (#84). Body is retained for diagnostics.
type scriptOutput struct {
	Imports      []string
	Decls        []string
	Runes        []string
	Props        []runeProp
	RestField    string
	RuneStmts    []runeStmt
	HasProps     bool
	ModuleScript string
	HasSnapshot  bool
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
			Msg: "at most two <script> blocks allowed (one regular plus one `<script module>`)",
		}
	}

	var out scriptOutput
	importSet := map[string]struct{}{}
	runeSet := map[string]struct{}{}
	moduleSeen := false

	for _, s := range scripts {
		if s.Module {
			if moduleSeen {
				return scriptOutput{}, &CodegenError{
					Pos: s.P,
					Msg: "duplicate `<script module>` block",
				}
			}
			moduleSeen = true
			out.ModuleScript = s.Body
			out.HasSnapshot = detectSnapshotExport(s.Body)
			continue
		}
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
