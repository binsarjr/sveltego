package kit

import (
	"errors"
	"strings"
)

// ErrLinkPattern is returned by [Link] when the pattern is malformed
// (missing leading slash, empty bracketed segment).
var ErrLinkPattern = errors.New("kit: malformed link pattern")

// ErrLinkParam is returned by [Link] when a required parameter has no
// entry in the supplied params map.
var ErrLinkParam = errors.New("kit: missing link param")

// Link builds a URL by substituting [name], [[name]], and [...name]
// segments in pattern with the corresponding entries in params. It is
// the runtime fallback used when codegen-emitted typed helpers under
// `<module>/.gen/links` are unavailable — typical apps prefer those
// because they fail at compile time when the route is renamed.
//
// Optional segments ([[name]]) are dropped when their value is empty;
// rest segments ([...name]) accept a "/"-prefixed value or a plain one.
// Required segments (and any other segment kind) error when the name
// has no entry in params.
func Link(pattern string, params map[string]string) (string, error) {
	if !strings.HasPrefix(pattern, "/") {
		return "", ErrLinkPattern
	}
	if pattern == "/" {
		return "/", nil
	}
	parts := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	var b strings.Builder
	for _, part := range parts {
		switch {
		case strings.HasPrefix(part, "[[") && strings.HasSuffix(part, "]]"):
			name := stripMatcher(part[2 : len(part)-2])
			if name == "" {
				return "", ErrLinkPattern
			}
			v, ok := params[name]
			if !ok || v == "" {
				continue
			}
			b.WriteByte('/')
			b.WriteString(v)
		case strings.HasPrefix(part, "[...") && strings.HasSuffix(part, "]"):
			name := stripMatcher(part[4 : len(part)-1])
			if name == "" {
				return "", ErrLinkPattern
			}
			v, ok := params[name]
			if !ok || v == "" {
				continue
			}
			b.WriteByte('/')
			b.WriteString(strings.TrimPrefix(v, "/"))
		case strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]"):
			name := stripMatcher(part[1 : len(part)-1])
			if name == "" {
				return "", ErrLinkPattern
			}
			v, ok := params[name]
			if !ok {
				return "", ErrLinkParam
			}
			b.WriteByte('/')
			b.WriteString(v)
		default:
			b.WriteByte('/')
			b.WriteString(part)
		}
	}
	if b.Len() == 0 {
		return "/", nil
	}
	return b.String(), nil
}

func stripMatcher(s string) string {
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[:i]
	}
	return s
}
