// Package routescan walks a SvelteKit-style src/routes/ tree and produces
// the typed metadata consumed by the codegen manifest emitter. It is an
// internal package; user code never imports it directly.
package routescan

import (
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

// Diagnostic is one recoverable problem surfaced by Scan. Diagnostics
// aggregate; the scanner aborts only on filesystem IO failures.
type Diagnostic struct {
	Path    string
	Pos     ast.Pos
	Message string
	Hint    string
}

// String formats the diagnostic as "path: message" or
// "path:line:col: message" when Pos is valid, with an optional
// " (hint: ...)" suffix.
func (d Diagnostic) String() string {
	var b strings.Builder
	b.WriteString(d.Path)
	if d.Pos.IsValid() {
		b.WriteByte(':')
		b.WriteString(d.Pos.String())
	}
	b.WriteString(": ")
	b.WriteString(d.Message)
	if d.Hint != "" {
		b.WriteString(" (hint: ")
		b.WriteString(d.Hint)
		b.WriteString(")")
	}
	return b.String()
}
