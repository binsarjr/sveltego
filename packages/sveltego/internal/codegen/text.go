package codegen

import "github.com/binsarjr/sveltego/internal/ast"

// emitText emits a single Text node as one WriteString call. Adjacent text
// merging is the caller's responsibility; see mergeAdjacentText.
func emitText(b *Builder, t *ast.Text) {
	if t == nil || t.Value == "" {
		return
	}
	b.Linef("w.WriteString(%s)", quoteGo(t.Value))
}

// mergeAdjacentText concatenates runs of consecutive *ast.Text siblings
// into a single Text node so emit produces one WriteString per run. Text
// runs do not cross any non-text boundary; Mustache, Element, and block
// nodes split the sequence.
func mergeAdjacentText(children []ast.Node) []ast.Node {
	if len(children) == 0 {
		return children
	}
	out := make([]ast.Node, 0, len(children))
	var pending *ast.Text
	for _, c := range children {
		if t, ok := c.(*ast.Text); ok {
			if pending == nil {
				clone := *t
				pending = &clone
			} else {
				pending.Value += t.Value
			}
			continue
		}
		if pending != nil {
			out = append(out, pending)
			pending = nil
		}
		out = append(out, c)
	}
	if pending != nil {
		out = append(out, pending)
	}
	return out
}
