package ast

import "strconv"

// Pos is a source location. Line and Col are 1-based; Offset is the byte
// offset from the start of the source.
type Pos struct {
	Offset int
	Line   int
	Col    int
}

// String formats the position as "line:col" for log and error messages.
func (p Pos) String() string {
	return strconv.Itoa(p.Line) + ":" + strconv.Itoa(p.Col)
}

// IsValid reports whether p refers to a real location. Zero values, which
// every parser path can produce on stub or recovery nodes, are invalid.
func (p Pos) IsValid() bool {
	return p.Line > 0 && p.Col > 0
}
