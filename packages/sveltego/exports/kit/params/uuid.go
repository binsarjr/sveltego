package params

import "github.com/binsarjr/sveltego/packages/sveltego/runtime/router"

// UUID matches the canonical 8-4-4-4-12 UUID textual form. Hex digits are
// case-insensitive; hyphen positions are fixed at 8, 13, 18, 23. Total
// length must be exactly 36 bytes.
var UUID router.ParamMatcher = router.MatcherFunc(func(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !isHexDigit(c) {
				return false
			}
		}
	}
	return true
})

func isHexDigit(c byte) bool {
	switch {
	case c >= '0' && c <= '9':
		return true
	case c >= 'a' && c <= 'f':
		return true
	case c >= 'A' && c <= 'F':
		return true
	}
	return false
}
