package codegen

import (
	"github.com/binsarjr/sveltego/internal/ast"
)

// isSvelteSpecialGlobal reports whether name is one of the global-target
// special elements <svelte:body>, <svelte:window>, <svelte:document>.
// These elements attach handlers to global DOM targets and have no SSR
// representation; codegen emits nothing and the parser preserves them
// for the client bundle.
func isSvelteSpecialGlobal(name string) bool {
	switch name {
	case "svelte:body", "svelte:window", "svelte:document":
		return true
	}
	return false
}

// emitSvelteSpecialGlobal validates and lowers a <svelte:body|window|
// document> element. SSR has no window or document; the element emits
// nothing. Children are not allowed.
func emitSvelteSpecialGlobal(b *Builder, e *ast.Element) {
	if e == nil {
		return
	}
	if len(e.Children) > 0 {
		b.Fail(&CodegenError{
			Pos: e.P,
			Msg: "<" + e.Name + "> may not have children",
		})
		return
	}
	// SSR no-op. Attributes are listener bindings consumed by the client
	// bundle; nothing reaches the rendered HTML.
}
