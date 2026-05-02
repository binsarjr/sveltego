package sveltejs2go

// AST builders used by the programmatic test cases. These mirror the
// shapes Acorn would produce for the Svelte 5 server-compiled output;
// they are the most expedient way to cover all 30 priority emit
// shapes without forking a JS toolchain into the test path.

func buildProgram(renderBody *Node) *Node {
	return buildProgramWithImports(renderBody)
}

// buildProgramWithImports synthesises a Program that mirrors Svelte 5's
// compiled server output and additionally carries the supplied import
// declarations (e.g. `import { page } from "$app/state"`). The
// canonical helper namespace (`import * as $ from
// "svelte/internal/server"`) is always emitted first so call-site
// dispatch in patterns.go keeps recognising `$.foo(...)` invocations.
func buildProgramWithImports(renderBody *Node, extras ...*Node) *Node {
	helperImport := &Node{
		Type: "ImportDeclaration",
		Source: &Node{
			Type:    "Literal",
			LitKind: litString,
			LitStr:  "svelte/internal/server",
		},
		Specifiers: []*Node{
			{
				Type:  "ImportNamespaceSpecifier",
				Local: ident("$"),
			},
		},
	}
	exportDefault := &Node{
		Type: "ExportDefaultDeclaration",
		Declaration: &Node{
			Type:     "FunctionDeclaration",
			ID:       ident("_page"),
			Params:   []*Node{ident("$$renderer"), ident("$$props")},
			FuncBody: renderBody,
		},
	}
	body := make([]*Node, 0, len(extras)+2)
	body = append(body, helperImport)
	body = append(body, extras...)
	body = append(body, exportDefault)
	return &Node{
		Type:       "Program",
		SourceType: "module",
		Body:       body,
	}
}

// importFrom builds an `import { name1, name2 } from "<source>"` AST
// node. Each name maps to an ImportSpecifier with imported and local
// identifiers set to the same name (no aliasing).
func importFrom(source string, names ...string) *Node {
	specs := make([]*Node, 0, len(names))
	for _, n := range names {
		specs = append(specs, &Node{
			Type:     "ImportSpecifier",
			Imported: ident(n),
			Local:    ident(n),
		})
	}
	return &Node{
		Type: "ImportDeclaration",
		Source: &Node{
			Type:    "Literal",
			LitKind: litString,
			LitStr:  source,
		},
		Specifiers: specs,
	}
}

func buildBlock(stmts ...*Node) *Node {
	return &Node{Type: "BlockStatement", Body: stmts}
}

func ident(name string) *Node {
	return &Node{Type: "Identifier", Name: name}
}

func strLit(s string) *Node {
	return &Node{Type: "Literal", LitKind: litString, LitStr: s, Raw: `"` + s + `"`}
}

func numLit(n float64) *Node {
	return &Node{Type: "Literal", LitKind: litNumber, LitNum: n}
}

func boolLit(v bool) *Node {
	return &Node{Type: "Literal", LitKind: litBool, LitBool: v}
}

func memExpr(obj, prop *Node) *Node {
	return &Node{Type: "MemberExpression", Object: obj, Property: prop}
}

func computedMember(obj, idx *Node) *Node {
	return &Node{Type: "MemberExpression", Object: obj, Property: idx, Computed: true}
}

func callExpr(callee *Node, args ...*Node) *Node {
	return &Node{Type: "CallExpression", Callee: callee, Arguments: args}
}

func callStmt(callee *Node, args ...*Node) *Node {
	return &Node{Type: "ExpressionStatement", Expression: callExpr(callee, args...)}
}

func tplLit(quasis []string, exprs []*Node) *Node {
	qs := make([]*Node, 0, len(quasis))
	for i, q := range quasis {
		qs = append(qs, &Node{
			Type:   "TemplateElement",
			Cooked: q,
			Tail:   i == len(quasis)-1,
		})
	}
	return &Node{Type: "TemplateLiteral", Quasis: qs, Expressions: exprs}
}

// pushTemplate emits `$$renderer.push(`...`)` with the given quasis
// and expressions, the dominant compiled-output shape.
func pushTemplate(quasis []string, exprs []*Node) *Node {
	return callStmt(
		memExpr(ident("$$renderer"), ident("push")),
		tplLit(quasis, exprs),
	)
}

// pushString emits `$$renderer.push("literal")`.
func pushString(s string) *Node {
	return callStmt(memExpr(ident("$$renderer"), ident("push")), strLit(s))
}

// helperCall emits `$.<name>(args...)` as an expression node.
func helperCall(name string, args ...*Node) *Node {
	return callExpr(memExpr(ident("$"), ident(name)), args...)
}

// escapeOf wraps an expression in `$.escape(expr)`.
func escapeOf(arg *Node) *Node {
	return helperCall("escape", arg)
}

// propsDestructure emits `let { <name> } = $$props`.
func propsDestructure(name string) *Node {
	return &Node{
		Type: "VariableDeclaration",
		Kind: "let",
		Declarations: []*Node{
			{
				Type: "VariableDeclarator",
				ID: &Node{
					Type: "ObjectPattern",
					Properties: []*Node{
						{
							Type:      "Property",
							Key:       ident(name),
							Value:     ident(name),
							Shorthand: true,
							Kind:      "init",
						},
					},
				},
				Init: ident("$$props"),
			},
		},
	}
}

// constDecl emits `const <name> = <init>`.
func constDecl(name string, init *Node) *Node {
	return &Node{
		Type: "VariableDeclaration",
		Kind: "const",
		Declarations: []*Node{
			{Type: "VariableDeclarator", ID: ident(name), Init: init},
		},
	}
}

// letDecl emits `let <name> = <init>`.
func letDecl(name string, init *Node) *Node {
	return &Node{
		Type: "VariableDeclaration",
		Kind: "let",
		Declarations: []*Node{
			{Type: "VariableDeclarator", ID: ident(name), Init: init},
		},
	}
}

// ifStmt builds an IfStatement with optional else branch.
func ifStmt(test, cons, alt *Node) *Node {
	return &Node{Type: "IfStatement", Test: test, Consequent: cons, Alternate: alt}
}

// binary builds a BinaryExpression.
func binary(op string, left, right *Node) *Node {
	return &Node{Type: "BinaryExpression", Operator: op, Left: left, Right: right}
}

// logical builds a LogicalExpression.
func logical(op string, left, right *Node) *Node {
	return &Node{Type: "LogicalExpression", Operator: op, Left: left, Right: right}
}

// update builds an UpdateExpression.
func update(op string, arg *Node) *Node {
	return &Node{Type: "UpdateExpression", Operator: op, Argument: arg}
}

// conditional builds a ConditionalExpression (`test ? cons : alt`).
func conditional(test, cons, alt *Node) *Node {
	return &Node{Type: "ConditionalExpression", Test: test, Consequent: cons, Alternate: alt}
}

// arrowFn builds an ArrowFunctionExpression with the given params and body.
func arrowFn(params []*Node, body *Node) *Node {
	return &Node{Type: "ArrowFunctionExpression", Params: params, FuncBody: body}
}

// forLoop builds a C-style ForStatement.
func forLoop(init, test, upd, body *Node) *Node {
	return &Node{Type: "ForStatement", Init: init, Test: test, Update: upd, FuncBody: body}
}

// forOf builds a ForOfStatement.
func forOf(left, right, body *Node) *Node {
	return &Node{Type: "ForOfStatement", Left: left, Right: right, FuncBody: body}
}

// optionalMember marks a MemberExpression's Optional chain.
func optionalMember(obj, prop *Node) *Node {
	n := memExpr(obj, prop)
	n.Optional = true
	return n
}

// chain wraps an expression in a ChainExpression.
func chain(inner *Node) *Node {
	return &Node{Type: "ChainExpression", Expression: inner}
}

// objExpr builds an ObjectExpression with the given key/value pairs.
func objExpr(pairs ...[2]*Node) *Node {
	props := make([]*Node, 0, len(pairs))
	for _, p := range pairs {
		props = append(props, &Node{Type: "Property", Key: p[0], Value: p[1], Kind: "init"})
	}
	return &Node{Type: "ObjectExpression", Properties: props}
}
