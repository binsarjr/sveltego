package codegen

import (
	"fmt"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

// emitIfBlock lowers {#if cond} ... {:else if c2} ... {:else} ... {/if} to
// a Go if/else if/else chain.
func emitIfBlock(b *Builder, n *ast.IfBlock) {
	if n == nil {
		return
	}
	if err := validateExpr(n.Cond, n.P); err != nil {
		b.Fail(err)
		return
	}
	b.nestDepth++
	defer func() { b.nestDepth-- }()
	b.Linef("if %s {", n.Cond)
	b.Indent()
	emitChildren(b, n.Then)
	b.Dedent()

	for i := range n.Elifs {
		e := &n.Elifs[i]
		if err := validateExpr(e.Cond, e.P); err != nil {
			b.Fail(err)
			return
		}
		b.Linef("} else if %s {", e.Cond)
		b.Indent()
		emitChildren(b, e.Body)
		b.Dedent()
	}

	if len(n.Else) > 0 {
		b.Line("} else {")
		b.Indent()
		emitChildren(b, n.Else)
		b.Dedent()
	}
	b.Line("}")
}

// emitEachBlock lowers {#each iter as item, idx (key)} ... {:else} ...
// {/each}. The key clause is server-side ignored (matters only for client
// diffing) but its expression is still validated to surface user typos.
// The :else branch wraps the for-range in a len() guard, which limits the
// iter type to slices, arrays, maps, strings, and channels — Go's range
// targets that also support len(). Range-over-func is intentionally not
// covered (deferred per #13).
func emitEachBlock(b *Builder, n *ast.EachBlock) {
	if n == nil {
		return
	}
	if err := validateExpr(n.Iter, n.P); err != nil {
		b.Fail(err)
		return
	}
	if n.Key != "" {
		if err := validateExpr(n.Key, n.P); err != nil {
			b.Fail(err)
			return
		}
		b.Linef("// key: %s", n.Key)
	}

	idx := n.Index
	if idx == "" {
		idx = "_"
	}
	item := n.Item
	if item == "" {
		item = "_"
	}

	b.nestDepth++
	defer func() { b.nestDepth-- }()
	if len(n.Else) > 0 {
		b.Linef("if len(%s) == 0 {", n.Iter)
		b.Indent()
		emitChildren(b, n.Else)
		b.Dedent()
		b.Line("} else {")
		b.Indent()
		b.Linef("for %s, %s := range %s {", idx, item, n.Iter)
		b.Indent()
		emitChildren(b, n.Body)
		b.Dedent()
		b.Line("}")
		b.Dedent()
		b.Line("}")
		return
	}

	b.Linef("for %s, %s := range %s {", idx, item, n.Iter)
	b.Indent()
	emitChildren(b, n.Body)
	b.Dedent()
	b.Line("}")
}

// emitKeyBlock lowers {#key expr} ... {/key} to a pair of HTML comment
// anchors wrapping the body. SSR has no remount semantics; the anchors
// carry a per-template index plus the source expression so client-side
// hydration metadata can locate the boundary and decide whether to remount
// when the value changes (see #35).
func emitKeyBlock(b *Builder, n *ast.KeyBlock) {
	if n == nil {
		return
	}
	if err := validateExpr(n.Key, n.P); err != nil {
		b.Fail(err)
		return
	}
	idx := b.keyCounter
	b.keyCounter++
	open := fmt.Sprintf("<!--sgkey:%d:%s-->", idx, n.Key)
	closeMarker := fmt.Sprintf("<!--/sgkey:%d-->", idx)
	b.Linef("w.WriteString(%s)", quoteGo(open))
	b.nestDepth++
	emitChildren(b, n.Body)
	b.nestDepth--
	b.Linef("w.WriteString(%s)", quoteGo(closeMarker))
}

// emitAwaitBlock lowers {#await fn() then x} ... {:catch err} ... {/await}
// to a synchronous call against a `func() (T, error)`. The then-branch
// renders on success; the catch-branch on error. Without a catch branch,
// the error returns from Render so the error boundary (#28) handles it.
//
// SSR has no streaming pending state — when the source carries a pending
// branch (`{#await fn()} ... {:then x} ...`), codegen surfaces a
// diagnostic. Streaming is #64.
func emitAwaitBlock(b *Builder, n *ast.AwaitBlock) {
	if n == nil {
		return
	}
	if err := validateExpr(n.Expr, n.P); err != nil {
		b.Fail(err)
		return
	}
	if hasNonEmptyBody(n.Pending) {
		b.Fail(&CodegenError{
			Pos: n.P,
			Msg: "{#await} pending branch unsupported under non-streaming SSR; use {:then} only or wait for streaming (#64)",
		})
		return
	}
	if len(n.Then) == 0 && len(n.Catch) == 0 {
		b.Fail(&CodegenError{
			Pos: n.P,
			Msg: "{#await} requires at least a {:then} or {:catch} branch under non-streaming SSR",
		})
		return
	}

	thenVar := n.ThenVar
	if thenVar == "" || len(n.Then) == 0 {
		thenVar = "_"
	}
	b.nestDepth++
	defer func() { b.nestDepth-- }()
	b.Line("{")
	b.Indent()
	b.Linef("%s, _err := %s", thenVar, n.Expr)
	b.Line("if _err != nil {")
	b.Indent()
	if len(n.Catch) > 0 {
		if n.CatchVar != "" {
			b.Linef("%s := _err", n.CatchVar)
		}
		emitChildren(b, n.Catch)
	} else {
		b.Line("return _err")
	}
	b.Dedent()
	if len(n.Then) > 0 {
		b.Line("} else {")
		b.Indent()
		emitChildren(b, n.Then)
		b.Dedent()
	}
	b.Line("}")
	b.Dedent()
	b.Line("}")
}

// hasNonEmptyBody reports whether children carry any node beyond pure
// whitespace text. Pending branches consisting of stray newlines between
// {#await} and {:then} are common in source formatting and should not
// trigger the no-streaming diagnostic.
func hasNonEmptyBody(children []ast.Node) bool {
	for _, c := range children {
		if t, ok := c.(*ast.Text); ok {
			if isWhitespace(t.Value) {
				continue
			}
		}
		return true
	}
	return false
}

func isWhitespace(s string) bool {
	for i := range len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return false
		}
	}
	return true
}
