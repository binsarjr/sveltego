package render

const (
	escAmp  = "&amp;"
	escLt   = "&lt;"
	escGt   = "&gt;"
	escQuot = "&#34;"
	escApos = "&#39;"
)

// appendEscapeText appends s to dst with HTML text-context escaping. Fast
// path returns dst with s appended verbatim when s contains no escapable
// byte; this avoids the per-byte allocation cost on the hot path where
// most template values are ASCII without HTML metacharacters.
func appendEscapeText(dst []byte, s string) []byte {
	i := indexTextSpecial(s)
	if i < 0 {
		return append(dst, s...)
	}
	dst = append(dst, s[:i]...)
	for ; i < len(s); i++ {
		switch s[i] {
		case '&':
			dst = append(dst, escAmp...)
		case '<':
			dst = append(dst, escLt...)
		case '>':
			dst = append(dst, escGt...)
		case '"':
			dst = append(dst, escQuot...)
		case '\'':
			dst = append(dst, escApos...)
		default:
			dst = append(dst, s[i])
		}
	}
	return dst
}

// appendEscapeAttr appends s to dst with HTML attribute-context escaping.
// Apostrophes are not escaped: codegen always wraps attribute values in
// double quotes, so a literal apostrophe is safe.
func appendEscapeAttr(dst []byte, s string) []byte {
	i := indexAttrSpecial(s)
	if i < 0 {
		return append(dst, s...)
	}
	dst = append(dst, s[:i]...)
	for ; i < len(s); i++ {
		switch s[i] {
		case '&':
			dst = append(dst, escAmp...)
		case '<':
			dst = append(dst, escLt...)
		case '>':
			dst = append(dst, escGt...)
		case '"':
			dst = append(dst, escQuot...)
		default:
			dst = append(dst, s[i])
		}
	}
	return dst
}

func indexTextSpecial(s string) int {
	for i := range len(s) {
		switch s[i] {
		case '&', '<', '>', '"', '\'':
			return i
		}
	}
	return -1
}

func indexAttrSpecial(s string) int {
	for i := range len(s) {
		switch s[i] {
		case '&', '<', '>', '"':
			return i
		}
	}
	return -1
}
