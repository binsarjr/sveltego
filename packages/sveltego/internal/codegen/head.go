package codegen

import (
	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
)

// emitPageHead writes the Page receiver's Head method covering the
// fragments lifted from <svelte:head>. The signature mirrors Render so
// the manifest adapter can widen the typed PageData parameter to the
// runtime PageHeadHandler shape.
func emitPageHead(b *Builder, scripts scriptOutput, headChildren []ast.Node) {
	b.Line("func (p Page) Head(w *render.Writer, ctx *kit.RenderCtx, data PageData) error {")
	b.Indent()
	b.Line("_ = ctx")
	b.Line("_ = data")
	if scripts.HasProps {
		b.Line("var props Props")
		b.Line("defaultProps(&props)")
		b.Line("_ = props")
	}
	emitRuneStmts(b, scripts.RuneStmts)
	rejectRootConst(b, headChildren)
	emitChildren(b, headChildren)
	b.Line("return nil")
	b.Dedent()
	b.Line("}")
}

// emitLayoutHead writes the Layout receiver's Head method. Layouts
// contribute head fragments outer→inner; the page pipeline composes
// them into the document <head> before the page-level Head fires.
func emitLayoutHead(b *Builder, scripts scriptOutput, headChildren []ast.Node) {
	b.Line("func (l Layout) Head(w *render.Writer, ctx *kit.RenderCtx, data LayoutData) error {")
	b.Indent()
	b.Line("_ = ctx")
	b.Line("_ = data")
	if scripts.HasProps {
		b.Line("var props Props")
		b.Line("defaultProps(&props)")
		b.Line("_ = props")
	}
	emitRuneStmts(b, scripts.RuneStmts)
	rejectRootConst(b, headChildren)
	emitChildren(b, headChildren)
	b.Line("return nil")
	b.Dedent()
	b.Line("}")
}

// isSvelteHead reports whether name is the <svelte:head> special element.
// Content inside <svelte:head> is lifted out of Render and into a sibling
// Head function so the page pipeline can gather head fragments from the
// layout chain + page into the document <head>.
func isSvelteHead(name string) bool {
	return name == "svelte:head"
}

// extractHeadChildren walks frag's children and returns the merged head
// children (across every <svelte:head> at the template root) plus the
// remaining body nodes with the head elements stripped. Multiple
// <svelte:head> blocks concatenate in source order. <svelte:head> is
// rejected when it appears inside another element or block — head content
// must be statically resolvable from the template root.
func extractHeadChildren(nodes []ast.Node) ([]ast.Node, []ast.Node) {
	var head, body []ast.Node
	for _, n := range nodes {
		el, ok := n.(*ast.Element)
		if ok && isSvelteHead(el.Name) {
			head = append(head, el.Children...)
			continue
		}
		body = append(body, n)
	}
	return head, body
}

// rejectNestedHead latches a CodegenError when <svelte:head> appears
// anywhere below the template root. Head content must compose
// statically; nesting it inside a block (e.g. {#if}) would force the
// pipeline to evaluate templates twice or risk inconsistent head buffers
// across requests. extractHeadChildren has already stripped the legal
// root-level forms from nodes, so any <svelte:head> sighting here is a
// nested misuse.
func rejectNestedHead(b *Builder, nodes []ast.Node) {
	for _, n := range nodes {
		switch v := n.(type) {
		case *ast.Element:
			if isSvelteHead(v.Name) {
				b.Fail(&CodegenError{
					Pos: v.P,
					Msg: "<svelte:head> must appear at the template root, not inside another element or block",
				})
				return
			}
			rejectNestedHead(b, v.Children)
		case *ast.IfBlock:
			rejectNestedHead(b, v.Then)
			for _, br := range v.Elifs {
				rejectNestedHead(b, br.Body)
			}
			rejectNestedHead(b, v.Else)
		case *ast.EachBlock:
			rejectNestedHead(b, v.Body)
			rejectNestedHead(b, v.Else)
		case *ast.AwaitBlock:
			rejectNestedHead(b, v.Pending)
			rejectNestedHead(b, v.Then)
			rejectNestedHead(b, v.Catch)
		case *ast.KeyBlock:
			rejectNestedHead(b, v.Body)
		case *ast.SnippetBlock:
			rejectNestedHead(b, v.Body)
		}
		if b.Err() != nil {
			return
		}
	}
}
