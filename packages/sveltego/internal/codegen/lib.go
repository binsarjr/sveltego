package codegen

import (
	"fmt"
	"regexp"
	"strings"
)

// libImportRe matches a double-quoted Go import literal whose path begins
// with the `$lib` alias. The regexp captures the trailing path (which may
// be empty for a bare `"$lib"`) so callers can inspect the segment.
var libImportRe = regexp.MustCompile(`"\$lib(/[^"]*)?"`)

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
