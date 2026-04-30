package routescan

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/binsarjr/sveltego/runtime/router"
)

// ErrGroup is returned by ParseSegment when the directory name encodes a
// route group like (marketing). Group segments are URL-invisible: callers
// must drop them from the segment slice while still encoding them in the
// generated package name per ADR 0003.
var ErrGroup = errors.New("routescan: group segment")

// ParseSegment classifies a single directory name into a router.Segment.
// It returns ErrGroup for (group) directories so the caller can keep the
// group in PackageName but drop it from URL segments. Any other error
// indicates a malformed directory name and should be surfaced as a
// diagnostic.
func ParseSegment(name string) (router.Segment, error) {
	if name == "" {
		return router.Segment{}, errors.New("empty segment name")
	}

	// Group: (name)
	if name[0] == '(' {
		if !strings.HasSuffix(name, ")") {
			return router.Segment{}, fmt.Errorf("unbalanced group brackets in %q", name)
		}
		inner := name[1 : len(name)-1]
		if inner == "" {
			return router.Segment{}, fmt.Errorf("empty group name in %q", name)
		}
		if !isGoIdent(inner) {
			return router.Segment{}, fmt.Errorf("invalid group name %q: not a Go identifier", inner)
		}
		return router.Segment{}, ErrGroup
	}

	// Bracketed: [...], [[...]], [...]
	if name[0] == '[' {
		return parseBracketed(name)
	}

	// Static segment: must be a valid path token (no [, ], (, )).
	if strings.ContainsAny(name, "[]()") {
		return router.Segment{}, fmt.Errorf("invalid characters in static segment %q", name)
	}
	return router.Segment{Kind: router.SegmentStatic, Value: name}, nil
}

func parseBracketed(name string) (router.Segment, error) {
	// Optional: [[name]] or [[name=matcher]]
	if strings.HasPrefix(name, "[[") {
		if !strings.HasSuffix(name, "]]") {
			return router.Segment{}, fmt.Errorf("unbalanced optional brackets in %q", name)
		}
		inner := name[2 : len(name)-2]
		if strings.ContainsAny(inner, "[]") {
			return router.Segment{}, fmt.Errorf("nested brackets in %q", name)
		}
		ident, matcher, err := splitNameMatcher(inner)
		if err != nil {
			return router.Segment{}, fmt.Errorf("optional segment %q: %w", name, err)
		}
		return router.Segment{Kind: router.SegmentOptional, Name: ident, Matcher: matcher}, nil
	}

	// Required or rest: [name], [name=matcher], [...rest], [...rest=matcher]
	if !strings.HasSuffix(name, "]") {
		return router.Segment{}, fmt.Errorf("unbalanced brackets in %q", name)
	}
	inner := name[1 : len(name)-1]
	if strings.ContainsAny(inner, "[]") {
		return router.Segment{}, fmt.Errorf("nested brackets in %q", name)
	}
	if rest, ok := strings.CutPrefix(inner, "..."); ok {
		ident, matcher, err := splitNameMatcher(rest)
		if err != nil {
			return router.Segment{}, fmt.Errorf("rest segment %q: %w", name, err)
		}
		return router.Segment{Kind: router.SegmentRest, Name: ident, Matcher: matcher}, nil
	}
	ident, matcher, err := splitNameMatcher(inner)
	if err != nil {
		return router.Segment{}, fmt.Errorf("param segment %q: %w", name, err)
	}
	return router.Segment{Kind: router.SegmentParam, Name: ident, Matcher: matcher}, nil
}

func splitNameMatcher(s string) (name, matcher string, err error) {
	if s == "" {
		return "", "", errors.New("empty parameter name")
	}
	if before, after, ok := strings.Cut(s, "="); ok {
		name, matcher = before, after
	} else {
		name = s
	}
	if !isGoIdent(name) {
		return "", "", fmt.Errorf("invalid parameter name %q: not a Go identifier", name)
	}
	if isGoKeyword(name) {
		return "", "", fmt.Errorf("invalid parameter name %q: Go keyword", name)
	}
	if matcher != "" && !isGoIdent(matcher) {
		return "", "", fmt.Errorf("invalid matcher name %q: not a Go identifier", matcher)
	}
	return name, matcher, nil
}

func isGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
}

// goKeywords matches the Go 1.22 reserved-word set; keep in sync with the
// language spec.
var goKeywords = map[string]struct{}{
	"break": {}, "case": {}, "chan": {}, "const": {}, "continue": {},
	"default": {}, "defer": {}, "else": {}, "fallthrough": {}, "for": {},
	"func": {}, "go": {}, "goto": {}, "if": {}, "import": {},
	"interface": {}, "map": {}, "package": {}, "range": {}, "return": {},
	"select": {}, "struct": {}, "switch": {}, "type": {}, "var": {},
}

func isGoKeyword(s string) bool {
	_, ok := goKeywords[s]
	return ok
}

// BuildPattern reconstructs the canonical SvelteKit-style URL pattern from
// segments. It always emits a leading slash; an empty slice yields "/".
func BuildPattern(segments []router.Segment) string {
	if len(segments) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, s := range segments {
		b.WriteByte('/')
		b.WriteString(s.String())
	}
	return b.String()
}

// encodePackageName returns the ADR 0003 encoded directory name for one
// segment plus the original raw name. Group segments need the raw name
// because ParseSegment drops them; for non-groups the raw name is used
// only to round-trip static segments.
func encodePackageName(raw string, seg router.Segment, isGroup bool) string {
	if isGroup {
		// (marketing) -> _g_marketing
		inner := raw[1 : len(raw)-1]
		return "_g_" + inner
	}
	switch seg.Kind {
	case router.SegmentStatic:
		return seg.Value
	case router.SegmentParam:
		return "_" + seg.Name + "_"
	case router.SegmentOptional:
		return "__" + seg.Name + "__"
	case router.SegmentRest:
		return "___" + seg.Name
	default:
		return raw
	}
}
