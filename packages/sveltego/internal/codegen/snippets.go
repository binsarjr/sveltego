package codegen

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

// emitSnippetBlock lowers {#snippet name(params)} ... {/snippet} to a local
// Go closure assigned to `name`. Two emissions are produced so the closure
// can reference itself recursively:
//
//	var name func(params, w *render.Writer) error
//	name = func(params, w *render.Writer) error {
//	    ...body...
//	    return nil
//	}
//	_ = name
//
// The trailing `_ = name` prevents Go's "declared and not used" error for
// snippets that are not invoked in the same template scope (e.g. when the
// snippet is exported via a future component-prop pass).
func emitSnippetBlock(b *Builder, n *ast.SnippetBlock) {
	if n == nil {
		return
	}
	if !isIdent(n.Name) {
		b.Fail(&CodegenError{
			Pos: n.P,
			Msg: fmt.Sprintf("{#snippet} requires an identifier name, got %q", n.Name),
		})
		return
	}
	if err := validateSnippetParams(n.Params, n.P); err != nil {
		b.Fail(err)
		return
	}

	sig := snippetSignature(n.Params)
	b.Linef("var %s func(%s) error", n.Name, sig)
	b.Linef("%s = func(%s) error {", n.Name, sig)
	b.Indent()
	emitChildren(b, n.Body)
	b.Line("return nil")
	b.Dedent()
	b.Line("}")
	b.Linef("_ = %s", n.Name)
}

// validateSnippetParams parses params as the parameter list of a synthetic
// Go function. Empty params are allowed. The returned *CodegenError carries
// the snippet's source position.
func validateSnippetParams(params string, pos ast.Pos) error {
	params = strings.TrimSpace(params)
	if params == "" {
		return nil
	}
	src := "package p\nfunc f(" + params + ") {}\n"
	_, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return &CodegenError{
			Pos: pos,
			Msg: fmt.Sprintf("invalid {#snippet} parameter list: %v", err),
		}
	}
	return nil
}

// snippetSignature builds the lowered Go parameter list. The user-declared
// params come first; the implicit `w *render.Writer` is appended last so
// invocation sites read like a normal function call with the writer
// threaded through as a tail argument.
func snippetSignature(params string) string {
	params = strings.TrimSpace(params)
	if params == "" {
		return "w *render.Writer"
	}
	return params + ", w *render.Writer"
}
