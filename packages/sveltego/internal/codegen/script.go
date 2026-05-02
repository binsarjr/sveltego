package codegen

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

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

// scriptModuleBlockRegexp matches a `<script ... context="module">` or
// `<script module ...>` opening tag and captures the body up to its
// closing tag. The lazy match keeps adjacent <script> blocks in the
// same file independent.
var scriptModuleBlockRegexp = regexp.MustCompile(
	`(?s)<script\b[^>]*\b(?:context\s*=\s*["']module["']|module)\b[^>]*>(.*?)</script>`,
)

// detectSnapshotInSvelte reports whether the .svelte file at path
// declares a `snapshot` export from a `<script module>` block. It runs
// before the codegen parse to drive SPA-router snapshot wiring (#84)
// without spending a full parser pass on every page route.
func detectSnapshotInSvelte(path string) (bool, error) {
	src, err := os.ReadFile(path) //nolint:gosec // path comes from the route scanner walk under projectRoot
	if err != nil {
		return false, fmt.Errorf("codegen: read %s: %w", path, err)
	}
	for _, m := range scriptModuleBlockRegexp.FindAllSubmatch(src, -1) {
		if detectSnapshotExport(string(m[1])) {
			return true, nil
		}
	}
	return false, nil
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
