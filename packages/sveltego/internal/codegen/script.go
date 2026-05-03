package codegen

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// scriptModuleBlockRegexp matches a `<script ... context="module">` or
// `<script module ...>` opening tag and captures the body up to its
// closing tag. The lazy match keeps adjacent <script> blocks in the
// same file independent.
var scriptModuleBlockRegexp = regexp.MustCompile(
	`(?s)<script\b[^>]*\b(?:context\s*=\s*["']module["']|module)\b[^>]*>(.*?)</script>`,
)

// moduleExportBindingRegexp captures the bound identifier from an
// `export const|let|var <name>` declaration in a `<script module>`
// block. The trailing `\b` keeps `snapshotMaker` from matching as
// `snapshot`. Function and class exports follow the same pattern via
// moduleExportFuncClassRegexp.
var moduleExportBindingRegexp = regexp.MustCompile(
	`(?m)export\s+(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`,
)

// moduleExportFuncClassRegexp captures `export function name(...)` and
// `export class Name {...}` declarations. `async` is allowed before
// `function` to keep async exports in scope.
var moduleExportFuncClassRegexp = regexp.MustCompile(
	`(?m)export\s+(?:async\s+)?(?:function\*?|class)\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`,
)

// moduleExportNamedRegexp captures the name list inside `export { a, b
// as c, d }` re-exports. Each entry is parsed by parseExportSpecifiers
// which handles the optional `as <alias>` rename.
var moduleExportNamedRegexp = regexp.MustCompile(
	`(?m)export\s*\{([^}]*)\}`,
)

// extractModuleExports returns the sorted, deduplicated list of names
// exported from `body` (the body of a single `<script module>` block).
// It recognises:
//
//   - `export const|let|var <name>`
//   - `export function|async function|function*|class <name>`
//   - `export { a, b as c }` (the alias `c` is the public name)
//
// `export default …` is intentionally excluded — the wrapper consumes
// the page's default export under a different identifier (`Page`), so
// re-exporting the default would clash. `export * from …` and dynamic
// patterns (`Object.assign(module.exports …)`) are out of scope; they
// are illegal in a Svelte 5 `<script module>` anyway.
//
// Comments are stripped via stripJSComments before matching so a
// commented-out export does not register. The function is intentionally
// regex-based, mirroring detectSnapshotExport's prior shape — Svelte's
// own compiler runs at build time downstream and will catch any
// pathological case the regex misclassifies.
func extractModuleExports(body string) []string {
	src := stripJSComments(body)
	seen := make(map[string]struct{})
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || name == "default" {
			return
		}
		seen[name] = struct{}{}
	}
	for _, m := range moduleExportBindingRegexp.FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	for _, m := range moduleExportFuncClassRegexp.FindAllStringSubmatch(src, -1) {
		add(m[1])
	}
	for _, m := range moduleExportNamedRegexp.FindAllStringSubmatch(src, -1) {
		for _, spec := range strings.Split(m[1], ",") {
			parts := strings.Fields(spec)
			switch len(parts) {
			case 1:
				add(parts[0])
			case 3:
				// `<orig> as <alias>` — the public name is the alias.
				if strings.EqualFold(parts[1], "as") {
					add(parts[2])
				}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// extractModuleExportsFromSvelte returns the sorted union of module-level
// export names declared across every `<script module>` block in the
// .svelte file at path. Used by the per-route wrapper emitter to
// re-export the same set so any consumer importing through the wrapper
// (e.g. entry.ts pulling `snapshot`) keeps resolving (#84, #508). Runs
// before the parser pass to keep cost proportional to the routes that
// actually carry module exports.
func extractModuleExportsFromSvelte(path string) ([]string, error) {
	src, err := os.ReadFile(path) //nolint:gosec // path comes from the route scanner walk under projectRoot
	if err != nil {
		return nil, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	seen := make(map[string]struct{})
	for _, m := range scriptModuleBlockRegexp.FindAllSubmatch(src, -1) {
		for _, name := range extractModuleExports(string(m[1])) {
			seen[name] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
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
