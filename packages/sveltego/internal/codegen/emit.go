// Package codegen lowers a parsed Svelte 5 fragment to Go source that runs
// against the render package. The generated file declares a Page receiver
// whose Render method writes the SSR HTML into a *render.Writer.
package codegen

import (
	"fmt"
	"strconv"
	"strings"
)

// Builder accumulates generated Go source with line and indentation
// tracking. Callers append one logical line at a time; the final pass runs
// the buffer through go/format so indentation is normalized regardless.
//
// Once Err is non-nil the Builder becomes a sink: further Line / Linef
// calls are no-ops so emit code paths can skip their own error-propagation
// plumbing. Callers check Err once at the end of generation.
type Builder struct {
	buf    strings.Builder
	indent int
	err    error
}

// Line appends s as one indented source line.
func (b *Builder) Line(s string) {
	if b.err != nil {
		return
	}
	b.writeIndent()
	b.buf.WriteString(s)
	b.buf.WriteByte('\n')
}

// Linef formats and appends one indented source line.
func (b *Builder) Linef(format string, args ...any) {
	if b.err != nil {
		return
	}
	b.writeIndent()
	fmt.Fprintf(&b.buf, format, args...)
	b.buf.WriteByte('\n')
}

// Fail latches err if no error is already set. Subsequent Line / Linef
// calls become no-ops.
func (b *Builder) Fail(err error) {
	if b.err == nil {
		b.err = err
	}
}

// Err reports the latched error, if any.
func (b *Builder) Err() error { return b.err }

// Indent increases the indent depth used by subsequent Line / Linef calls.
func (b *Builder) Indent() { b.indent++ }

// Dedent decreases the indent depth used by subsequent Line / Linef calls.
func (b *Builder) Dedent() {
	if b.indent > 0 {
		b.indent--
	}
}

// Bytes returns the accumulated source as a byte slice.
func (b *Builder) Bytes() []byte { return []byte(b.buf.String()) }

func (b *Builder) writeIndent() {
	for range b.indent {
		b.buf.WriteByte('\t')
	}
}

// quoteGo renders s as a Go string literal. Default form is a raw backtick
// string, which keeps generated source readable for HTML payloads. The
// interpreted form is used when s contains a backtick (raw cannot escape
// it) or a CR (gofmt strips CRs from raw strings, which would silently
// drop bytes from emitted output).
func quoteGo(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, "`\r") {
		return strconv.Quote(s)
	}
	return "`" + s + "`"
}
