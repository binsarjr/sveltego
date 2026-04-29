package codegen

import "github.com/binsarjr/sveltego/internal/ast"

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
