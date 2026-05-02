package codegen

import (
	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
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
