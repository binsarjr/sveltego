// Package params holds the matchers shipped with sveltego that are
// resolvable in route segments without requiring a project-level
// src/params/<name>.go file. Use [DefaultMatchers] to compose them into
// [router.Tree.WithMatchers].
package params

import (
	"strconv"

	"github.com/binsarjr/sveltego/runtime/router"
)

// Int matches segments parseable as a base-10 signed integer via
// [strconv.Atoi]. The empty string fails.
var Int router.ParamMatcher = router.MatcherFunc(func(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.Atoi(s)
	return err == nil
})
