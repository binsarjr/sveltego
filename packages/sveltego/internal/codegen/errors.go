package codegen

import (
	"fmt"

	"github.com/binsarjr/sveltego/internal/ast"
)

// CodegenError is a positioned codegen-time failure. Pos points back to the
// originating .svelte source so the message can be rendered with caret
// context by the CLI.
//
//nolint:revive // public API name is locked by ADR 0002.
type CodegenError struct {
	Pos ast.Pos
	Msg string
}

// Error formats the error as "line:col: message".
func (e *CodegenError) Error() string {
	return e.Pos.String() + ": " + e.Msg
}

// newExprError wraps a go/parser failure with the originating .svelte
// position. The returned error's message starts with "invalid Go
// expression:" so callers (and tests) can match a stable prefix without
// pinning the exact go/parser wording, which shifts across Go versions.
func newExprError(pos ast.Pos, err error) *CodegenError {
	return &CodegenError{
		Pos: pos,
		Msg: fmt.Sprintf("invalid Go expression: %v", err),
	}
}

// newStmtError wraps a statement-level go/parser failure.
func newStmtError(pos ast.Pos, err error) *CodegenError {
	return &CodegenError{
		Pos: pos,
		Msg: fmt.Sprintf("invalid Go statement: %v", err),
	}
}
