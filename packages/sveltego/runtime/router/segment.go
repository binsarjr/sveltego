// Package router holds the runtime route table consumed by the
// codegen-emitted manifest and the request dispatcher. A Tree is built
// once from a slice of Route and then queried concurrently by Match.
package router

// SegmentKind classifies a single path segment in a Route pattern.
type SegmentKind uint8

const (
	// SegmentStatic is a literal path segment matched by exact compare.
	SegmentStatic SegmentKind = iota
	// SegmentParam is a required `[name]` segment capturing one path piece.
	SegmentParam
	// SegmentOptional is an optional `[[name]]` segment matching zero or one piece.
	SegmentOptional
	// SegmentRest is a `[...name]` catch-all matching zero or more pieces.
	SegmentRest
)

// Segment describes one segment of a Route pattern. Value carries the
// literal text for SegmentStatic; Name carries the parameter identifier
// for the other kinds. Matcher names a registered ParamMatcher when the
// pattern uses `[name=matcher]` syntax; it is empty otherwise.
type Segment struct {
	Kind    SegmentKind
	Name    string
	Value   string
	Matcher string
}

// String returns the SvelteKit-style canonical form of s without leading
// or trailing slashes; the caller joins segments with "/".
func (s Segment) String() string {
	switch s.Kind {
	case SegmentStatic:
		return s.Value
	case SegmentParam:
		if s.Matcher != "" {
			return "[" + s.Name + "=" + s.Matcher + "]"
		}
		return "[" + s.Name + "]"
	case SegmentOptional:
		if s.Matcher != "" {
			return "[[" + s.Name + "=" + s.Matcher + "]]"
		}
		return "[[" + s.Name + "]]"
	case SegmentRest:
		if s.Matcher != "" {
			return "[..." + s.Name + "=" + s.Matcher + "]"
		}
		return "[..." + s.Name + "]"
	default:
		return ""
	}
}
