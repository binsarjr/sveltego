package codegen

import (
	"fmt"
	"regexp"
	"strings"
)

// libImportRe matches a double-quoted Go import literal whose path begins
// with the `$lib` alias. The regexp captures the trailing path (which may
// be empty for a bare `"$lib"`) so the rewriter can rebuild the literal
// with the user's module path substituted in.
var libImportRe = regexp.MustCompile(`"\$lib(/[^"]*)?"`)

// rewriteLibImports rewrites every `"$lib"` or `"$lib/<rest>"` import
// literal in body to `"<modulePath>/lib"` or `"<modulePath>/lib/<rest>"`.
// The second return value is true when at least one occurrence was
// replaced. The rewriter is text-level; it does not parse Go and only
// touches double-quoted import literals — back-tick paths and computed
// imports are out of scope.
func rewriteLibImports(body, modulePath string) (string, bool) {
	if !strings.Contains(body, "$lib") {
		return body, false
	}
	hit := false
	out := libImportRe.ReplaceAllStringFunc(body, func(match string) string {
		hit = true
		// match always starts with `"$lib`; submatch [1] is the optional
		// "/<rest>" tail (empty for bare `"$lib"`).
		sub := libImportRe.FindStringSubmatch(match)
		tail := ""
		if len(sub) > 1 {
			tail = sub[1]
		}
		return `"` + modulePath + "/lib" + tail + `"`
	})
	return out, hit
}

// checkLibDevImports returns an error when body contains a `$lib/dev`
// import. Called only in release mode: src/lib/dev/ is a dev-only
// subtree (mirrors sveltejs/kit#13078) and must not appear in
// production builds.
func checkLibDevImports(body, srcPath string) error {
	matches := libImportRe.FindAllStringSubmatch(body, -1)
	for _, m := range matches {
		tail := ""
		if len(m) > 1 {
			tail = m[1]
		}
		// tail is either empty or starts with "/". A match of "/dev" or
		// "/dev/<anything>" is the forbidden pattern.
		if tail == "/dev" || strings.HasPrefix(tail, "/dev/") {
			return fmt.Errorf("codegen: %s: $lib/dev imports are not allowed in release builds (sveltejs/kit#13078)", srcPath)
		}
	}
	return nil
}
