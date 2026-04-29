package ast

// Visitor walks an AST. Visit is called for every node; returning a
// non-nil Visitor descends into the node's children, returning nil prunes.
// Shape mirrors go/ast.Visitor so existing patterns transfer.
type Visitor interface {
	Visit(n Node) Visitor
}

// Walk traverses an AST rooted at n in depth-first order. For each node
// w := v.Visit(n) is called; if w is non-nil Walk is invoked recursively
// for each child of n with w. After all children have been visited Walk
// calls w.Visit(nil).
func Walk(v Visitor, n Node) {
	if v = v.Visit(n); v == nil {
		return
	}

	switch n := n.(type) {
	case *Fragment:
		walkList(v, n.Children)
	case *Text:
		// leaf
	case *Element:
		for i := range n.Attributes {
			walkAttribute(v, &n.Attributes[i])
		}
		walkList(v, n.Children)
	case *StaticValue:
		// leaf
	case *DynamicValue:
		// leaf
	case *InterpolatedValue:
		walkList(v, n.Parts)
	case *Mustache:
		// leaf
	case *IfBlock:
		walkList(v, n.Then)
		for i := range n.Elifs {
			walkList(v, n.Elifs[i].Body)
		}
		walkList(v, n.Else)
	case *EachBlock:
		walkList(v, n.Body)
		walkList(v, n.Else)
	case *AwaitBlock:
		walkList(v, n.Pending)
		walkList(v, n.Then)
		walkList(v, n.Catch)
	case *KeyBlock:
		walkList(v, n.Body)
	case *SnippetBlock:
		walkList(v, n.Body)
	case *RawHTML:
		// leaf
	case *Const:
		// leaf
	case *Render:
		// leaf
	case *Script:
		// leaf
	case *Style:
		// leaf
	case *Comment:
		// leaf
	}

	v.Visit(nil)
}

func walkList(v Visitor, list []Node) {
	for _, c := range list {
		Walk(v, c)
	}
}

func walkAttribute(v Visitor, a *Attribute) {
	if a.Value != nil {
		Walk(v, a.Value)
	}
}
