// Package codegen lowers a parsed Svelte 5 fragment to Go source that runs
// against the render package. The generated file declares a Page receiver
// whose Render method writes the SSR HTML into a *render.Writer.
package codegen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/images"
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
	// hasChildren is true when the enclosing Render method declares the
	// `children func(*render.Writer) error` parameter (layout templates).
	// emitElement consults this flag when lowering <slot />.
	hasChildren bool
	// keyCounter assigns stable per-template indices to {#key} blocks so
	// the SSR anchor comments line up with the client-side hydration
	// metadata table.
	keyCounter int
	// nestDepth counts how deep the current emit position sits inside
	// element wrappers or block constructs ({#if}, {#each}, {#await},
	// {#key}). Special elements like <svelte:body> may only appear at
	// the template root and consult this counter to validate placement.
	nestDepth int
	// componentMode is true while emitting a Svelte component's Render
	// body (GenerateComponent). Slot outlets in this mode dispatch to
	// the per-component Slots struct rather than the layout `children`
	// closure.
	componentMode bool
	// slots collects every <slot> outlet seen during a component render
	// so GenerateComponent can emit the matching Slots struct field set.
	// Names are normalized: empty/"default" both map to "Default"; named
	// slots map to PascalCase identifiers.
	slots []slotOutlet
	// provenance, when true, causes emitNode to prefix each span with a
	// // gen: source=... kind=... comment so LLMs and humans can trace the
	// generated code back to its .svelte source line.
	provenance bool
	// srcPath is the relative .svelte source path written into span
	// comments. Set once from Options.Filename before the first emit.
	srcPath string
	// imageVariants maps each <Image src=...> path to its build-time
	// generated variant set. emitImage consults this table to resolve
	// the hashed URLs and intrinsic dimensions written into the page.
	imageVariants map[string]images.Result
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
