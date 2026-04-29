package parser

import (
	"fmt"
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
)

// Dump renders frag as a deterministic, indented S-expression-like string.
// Used by golden tests; format kept line-stable so diffs read cleanly.
func Dump(frag *ast.Fragment) string {
	var b strings.Builder
	dumpNode(&b, frag, 0)
	return b.String()
}

// DumpErrors renders es one error per line, in the order produced. Empty
// input yields an empty string.
func DumpErrors(es ast.Errors) string {
	if len(es) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range es {
		b.WriteString(e.Error())
		b.WriteByte('\n')
	}
	return b.String()
}

func dumpNode(b *strings.Builder, n ast.Node, depth int) {
	if n == nil {
		return
	}
	indent(b, depth)
	switch v := n.(type) {
	case *ast.Fragment:
		fmt.Fprintf(b, "Fragment %s\n", posStr(v.P))
		for _, c := range v.Children {
			dumpNode(b, c, depth+1)
		}
	case *ast.Text:
		fmt.Fprintf(b, "Text %s %q\n", posStr(v.P), v.Value)
	case *ast.Element:
		kind := "Element"
		if v.Component {
			kind = "Component"
		}
		self := ""
		if v.SelfClosing {
			self = " self-closing"
		}
		fmt.Fprintf(b, "%s %q%s %s\n", kind, v.Name, self, posStr(v.P))
		for i := range v.Attributes {
			dumpAttribute(b, &v.Attributes[i], depth+1)
		}
		for _, c := range v.Children {
			dumpNode(b, c, depth+1)
		}
	case *ast.Mustache:
		fmt.Fprintf(b, "Mustache %s expr=%q\n", posStr(v.P), v.Expr)
	case *ast.IfBlock:
		fmt.Fprintf(b, "IfBlock %s cond=%q\n", posStr(v.P), v.Cond)
		dumpBranch(b, "then", v.Then, depth+1)
		for i := range v.Elifs {
			indent(b, depth+1)
			fmt.Fprintf(b, "ElifBranch %s cond=%q\n", posStr(v.Elifs[i].P), v.Elifs[i].Cond)
			for _, c := range v.Elifs[i].Body {
				dumpNode(b, c, depth+2)
			}
		}
		if v.Else != nil {
			dumpBranch(b, "else", v.Else, depth+1)
		}
	case *ast.EachBlock:
		fmt.Fprintf(b, "EachBlock %s iter=%q item=%q index=%q key=%q\n",
			posStr(v.P), v.Iter, v.Item, v.Index, v.Key)
		dumpBranch(b, "body", v.Body, depth+1)
		if v.Else != nil {
			dumpBranch(b, "else", v.Else, depth+1)
		}
	case *ast.AwaitBlock:
		fmt.Fprintf(b, "AwaitBlock %s expr=%q\n", posStr(v.P), v.Expr)
		dumpBranch(b, "pending", v.Pending, depth+1)
		if v.Then != nil || v.ThenVar != "" {
			indent(b, depth+1)
			fmt.Fprintf(b, "then var=%q\n", v.ThenVar)
			for _, c := range v.Then {
				dumpNode(b, c, depth+2)
			}
		}
		if v.Catch != nil || v.CatchVar != "" {
			indent(b, depth+1)
			fmt.Fprintf(b, "catch var=%q\n", v.CatchVar)
			for _, c := range v.Catch {
				dumpNode(b, c, depth+2)
			}
		}
	case *ast.KeyBlock:
		fmt.Fprintf(b, "KeyBlock %s key=%q\n", posStr(v.P), v.Key)
		for _, c := range v.Body {
			dumpNode(b, c, depth+1)
		}
	case *ast.SnippetBlock:
		fmt.Fprintf(b, "SnippetBlock %s name=%q params=%q\n", posStr(v.P), v.Name, v.Params)
		for _, c := range v.Body {
			dumpNode(b, c, depth+1)
		}
	case *ast.RawHTML:
		fmt.Fprintf(b, "RawHTML %s expr=%q\n", posStr(v.P), v.Expr)
	case *ast.Const:
		fmt.Fprintf(b, "Const %s stmt=%q\n", posStr(v.P), v.Stmt)
	case *ast.Render:
		fmt.Fprintf(b, "Render %s expr=%q\n", posStr(v.P), v.Expr)
	case *ast.Script:
		fmt.Fprintf(b, "Script %s lang=%q body=%q\n", posStr(v.P), v.Lang, v.Body)
	case *ast.Style:
		fmt.Fprintf(b, "Style %s body=%q\n", posStr(v.P), v.Body)
	case *ast.Comment:
		fmt.Fprintf(b, "Comment %s %q\n", posStr(v.P), v.Value)
	default:
		fmt.Fprintf(b, "Unknown(%T)\n", n)
	}
}

func dumpAttribute(b *strings.Builder, a *ast.Attribute, depth int) {
	indent(b, depth)
	fmt.Fprintf(b, "Attribute %s name=%q kind=%s", posStr(a.P), a.Name, a.Kind.String())
	if a.Modifier != "" {
		fmt.Fprintf(b, " modifier=%q", a.Modifier)
	}
	switch v := a.Value.(type) {
	case nil:
		b.WriteString(" boolean")
	case *ast.StaticValue:
		fmt.Fprintf(b, " static=%q", v.Value)
	case *ast.DynamicValue:
		fmt.Fprintf(b, " dynamic=%q", v.Expr)
	case *ast.InterpolatedValue:
		fmt.Fprintf(b, " interpolated parts=%d", len(v.Parts))
	}
	b.WriteByte('\n')
}

func dumpBranch(b *strings.Builder, label string, nodes []ast.Node, depth int) {
	indent(b, depth)
	fmt.Fprintf(b, "%s\n", label)
	for _, c := range nodes {
		dumpNode(b, c, depth+1)
	}
}

func indent(b *strings.Builder, depth int) {
	for range depth {
		b.WriteString("  ")
	}
}

func posStr(p ast.Pos) string {
	return fmt.Sprintf("@%d:%d", p.Line, p.Col)
}
