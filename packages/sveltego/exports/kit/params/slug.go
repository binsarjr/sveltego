package params

import "github.com/binsarjr/sveltego/packages/sveltego/runtime/router"

// Slug matches lowercase a-z, digits, and hyphens, with no leading or
// trailing hyphen and no consecutive hyphens. Equivalent to the regular
// expression `^[a-z0-9]+(?:-[a-z0-9]+)*$` but implemented as a byte
// loop so the matcher stays allocation-free on the hot path.
var Slug router.ParamMatcher = router.MatcherFunc(func(s string) bool {
	if s == "" {
		return false
	}
	prevHyphen := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			prevHyphen = false
		case c == '-':
			if i == 0 || prevHyphen {
				return false
			}
			prevHyphen = true
		default:
			return false
		}
	}
	return !prevHyphen
})
