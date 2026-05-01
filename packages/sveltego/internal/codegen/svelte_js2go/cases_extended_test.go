package sveltejs2go

// Extended programmatic cases — Phase 7 (#429) corpus expansion.
// Adds 50+ shape variants on top of the 30 priority cases in
// cases_test.go to push coverage toward the ~95% v1 target from
// RFC #421.
//
// Each case captures a Svelte 5 server-output shape we want emit-stable
// goldens for. The synthesized AST trees mirror what Acorn sees from
// `svelte/compiler generate:'server'` output for the corresponding
// template construct, so adding a new compiler version means re-running
// goldens — not rewriting these builders.
func extendedProgrammaticCases() []programmaticCase {
	return []programmaticCase{
		// --- Escape and stringify variants ---
		{name: "escape-attr-position", root: caseEscapeAttrPosition},
		{name: "escape-html-alias", root: caseEscapeHTMLAlias},
		{name: "escape-of-call", root: caseEscapeOfCall},
		{name: "escape-of-binary", root: caseEscapeOfBinary},
		{name: "escape-of-conditional", root: caseEscapeOfConditional},
		{name: "stringify-helper", root: caseStringifyHelper},
		{name: "concat-strings", root: caseConcatStrings},
		{name: "concat-string-and-expr", root: caseConcatStringAndExpr},

		// --- Attribute helper variants ---
		{name: "attr-bool-true", root: caseAttrBoolTrue},
		{name: "attr-string-literal-value", root: caseAttrStringLiteralValue},
		{name: "to-class-with-hash", root: caseToClassWithHash},
		{name: "to-class-with-directives", root: caseToClassWithDirectives},
		{name: "to-style-with-styles", root: caseToStyleWithStyles},
		{name: "merge-styles-multi", root: caseMergeStylesMulti},
		{name: "clsx-multi", root: caseClsxMulti},

		// --- Control flow variants ---
		{name: "if-only", root: caseIfOnly},
		{name: "if-with-empty-alt", root: caseIfWithEmptyAlt},
		{name: "nested-if-if", root: caseNestedIfIf},
		{name: "ternary-nested", root: caseTernaryNested},
		{name: "logical-or", root: caseLogicalOr},
		{name: "logical-mixed", root: caseLogicalMixed},
		{name: "logical-not", root: caseLogicalNot},

		// --- Comparison + arithmetic ---
		{name: "binary-eq", root: caseBinaryEq},
		{name: "binary-strict-eq", root: caseBinaryStrictEq},
		{name: "binary-ne", root: caseBinaryNe},
		{name: "binary-lt", root: caseBinaryLt},
		{name: "binary-add", root: caseBinaryAdd},
		{name: "binary-sub", root: caseBinarySub},
		{name: "binary-mul", root: caseBinaryMul},
		{name: "binary-div", root: caseBinaryDiv},
		{name: "binary-mod", root: caseBinaryMod},
		{name: "unary-minus", root: caseUnaryMinus},
		{name: "unary-plus", root: caseUnaryPlus},
		{name: "unary-not", root: caseUnaryNot},

		// --- Optional chaining + nullish ---
		{name: "optional-chain-deep", root: caseOptionalChainDeep},
		{name: "optional-chain-call", root: caseOptionalChainCall},
		{name: "nullish-with-bool", root: caseNullishWithBool},

		// --- Each variants ---
		{name: "each-without-index", root: caseEachWithoutIndex},
		{name: "each-empty-body", root: caseEachEmptyBody},
		{name: "each-nested", root: caseEachNested},
		{name: "each-with-static", root: caseEachWithStatic},
		{name: "for-of-bare-array", root: caseForOfBareArray},

		// --- Snippet + render variants ---
		{name: "snippet-no-args", root: caseSnippetNoArgs},
		{name: "snippet-multi-args", root: caseSnippetMultiArgs},
		{name: "render-with-string", root: caseRenderWithString},
		{name: "snippet-then-render", root: caseSnippetThenRender},

		// --- @const variants ---
		{name: "atconst-string", root: caseAtConstString},
		{name: "atconst-with-fn", root: caseAtConstWithFn},
		{name: "atconst-multi", root: caseAtConstMulti},

		// --- Slot variants ---
		{name: "slot-with-fallback", root: caseSlotWithFallback},
		{name: "slot-with-props", root: caseSlotWithProps},

		// --- Helper edge cases ---
		{name: "helper-fallback", root: caseHelperFallback},
		{name: "helper-rest-props", root: caseHelperRestProps},
		{name: "helper-exclude-from-object", root: caseHelperExcludeFromObject},
		{name: "helper-sanitize-props", root: caseHelperSanitizeProps},
		{name: "helper-spread-props", root: caseHelperSpreadProps},

		// --- Element + head + svelte:* ---
		{name: "svelte-head-multi", root: caseSvelteHeadMulti},
		{name: "svelte-element-no-attrs", root: caseSvelteElementNoAttrs},

		// --- Template literal corner cases ---
		{name: "template-no-exprs", root: caseTemplateNoExprs},
		{name: "template-leading-expr", root: caseTemplateLeadingExpr},
		{name: "template-trailing-expr", root: caseTemplateTrailingExpr},
		{name: "template-many-exprs", root: caseTemplateManyExprs},

		// --- Function expression body cases ---
		{name: "arrow-with-statement", root: caseArrowWithStatement},
		{name: "function-expression-component", root: caseFunctionExpressionComponent},
	}
}

// allCases combines the 30 priority shapes with the extended corpus.
func allCases() []programmaticCase {
	cases := allProgrammaticCases()
	return append(cases, extendedProgrammaticCases()...)
}

// --- Builders ---

func unary(op string, arg *Node) *Node {
	return &Node{Type: "UnaryExpression", Operator: op, Argument: arg}
}

func arrayExpr(elements ...*Node) *Node {
	return &Node{Type: "ArrayExpression", Properties: elements}
}

// --- Cases ---

func caseEscapeAttrPosition() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<input value="`, `" />`},
			[]*Node{helperCall("escape", memExpr(ident("data"), ident("value")), boolLit(true))},
		),
	))
}

func caseEscapeHTMLAlias() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{helperCall("escape_html", memExpr(ident("data"), ident("body")))},
		),
	))
}

func caseEscapeOfCall() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(callExpr(memExpr(ident("data"), ident("title"))))},
		),
	))
}

func caseEscapeOfBinary() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{escapeOf(binary("+",
				memExpr(ident("data"), ident("first")),
				memExpr(ident("data"), ident("last")),
			))},
		),
	))
}

func caseEscapeOfConditional() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(conditional(
				memExpr(ident("data"), ident("loggedIn")),
				strLit("welcome"),
				strLit("guest"),
			))},
		),
	))
}

func caseStringifyHelper() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{helperCall("stringify", memExpr(ident("data"), ident("count")))},
		),
	))
}

func caseConcatStrings() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<a href=\"", "\">link</a>"},
			[]*Node{binary("+", strLit("/post/"), memExpr(ident("data"), ident("id")))},
		),
	))
}

func caseConcatStringAndExpr() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(binary("+",
				binary("+", strLit("hi "), memExpr(ident("data"), ident("name"))),
				strLit("!"),
			))},
		),
	))
}

func caseAttrBoolTrue() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<input", " />"},
			[]*Node{helperCall("attr",
				strLit("disabled"),
				memExpr(ident("data"), ident("disabled")),
				boolLit(true),
			)},
		),
	))
}

func caseAttrStringLiteralValue() *Node {
	return buildProgram(buildBlock(
		pushTemplate(
			[]string{"<button", ">click</button>"},
			[]*Node{helperCall("attr", strLit("data-action"), strLit("submit"), boolLit(false))},
		),
	))
}

func caseToClassWithHash() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<span class="`, `"></span>`},
			[]*Node{helperCall("to_class",
				memExpr(ident("data"), ident("klass")),
				strLit("svelte-abc123"),
			)},
		),
	))
}

func caseToClassWithDirectives() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<span class="`, `"></span>`},
			[]*Node{helperCall("to_class",
				memExpr(ident("data"), ident("klass")),
				strLit(""),
				memExpr(ident("data"), ident("directives")),
			)},
		),
	))
}

func caseToStyleWithStyles() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<div style="`, `"></div>`},
			[]*Node{helperCall("to_style",
				memExpr(ident("data"), ident("style")),
				memExpr(ident("data"), ident("dirs")),
			)},
		),
	))
}

func caseMergeStylesMulti() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<div style="`, `"></div>`},
			[]*Node{helperCall("merge_styles",
				strLit("color: red"),
				strLit("font-size: 12px"),
				memExpr(ident("data"), ident("style")),
			)},
		),
	))
}

func caseClsxMulti() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<span class="`, `"></span>`},
			[]*Node{helperCall("clsx",
				strLit("base"),
				strLit("primary"),
				memExpr(ident("data"), ident("active")),
				memExpr(ident("data"), ident("size")),
			)},
		),
	))
}

func caseIfOnly() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(memExpr(ident("data"), ident("show")), buildBlock(pushString("<p>shown</p>")), nil),
	))
}

func caseIfWithEmptyAlt() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(memExpr(ident("data"), ident("toggle")),
			buildBlock(pushString("<p>on</p>")),
			buildBlock(),
		),
	))
}

func caseNestedIfIf() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(memExpr(ident("data"), ident("a")),
			buildBlock(ifStmt(memExpr(ident("data"), ident("b")),
				buildBlock(pushString("<p>both</p>")),
				nil,
			)),
			nil,
		),
	))
}

func caseTernaryNested() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{conditional(
				memExpr(ident("data"), ident("a")),
				strLit("x"),
				conditional(
					memExpr(ident("data"), ident("b")),
					strLit("y"),
					strLit("z"),
				),
			)},
		),
	))
}

func caseLogicalOr() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(logical("||",
				memExpr(ident("data"), ident("name")),
				strLit("anonymous"),
			))},
		),
	))
}

func caseLogicalMixed() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			logical("||",
				logical("&&",
					memExpr(ident("data"), ident("a")),
					memExpr(ident("data"), ident("b")),
				),
				memExpr(ident("data"), ident("c")),
			),
			buildBlock(pushString("<p>true</p>")),
			nil,
		),
	))
}

func caseLogicalNot() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			unary("!", memExpr(ident("data"), ident("hidden"))),
			buildBlock(pushString("<p>visible</p>")),
			nil,
		),
	))
}

func caseBinaryEq() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			binary("==", memExpr(ident("data"), ident("status")), strLit("ready")),
			buildBlock(pushString("<p>ready</p>")),
			nil,
		),
	))
}

func caseBinaryStrictEq() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			binary("===", memExpr(ident("data"), ident("count")), numLit(0)),
			buildBlock(pushString("<p>empty</p>")),
			nil,
		),
	))
}

func caseBinaryNe() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			binary("!=", memExpr(ident("data"), ident("status")), strLit("error")),
			buildBlock(pushString("<p>ok</p>")),
			nil,
		),
	))
}

func caseBinaryLt() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			binary("<", memExpr(ident("data"), ident("count")), numLit(10)),
			buildBlock(pushString("<p>few</p>")),
			nil,
		),
	))
}

func caseBinaryAdd() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{escapeOf(binary("+",
				memExpr(ident("data"), ident("a")),
				memExpr(ident("data"), ident("b")),
			))},
		),
	))
}

func caseBinarySub() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{escapeOf(binary("-",
				memExpr(ident("data"), ident("a")),
				memExpr(ident("data"), ident("b")),
			))},
		),
	))
}

func caseBinaryMul() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{escapeOf(binary("*",
				memExpr(ident("data"), ident("price")),
				memExpr(ident("data"), ident("qty")),
			))},
		),
	))
}

func caseBinaryDiv() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{escapeOf(binary("/",
				memExpr(ident("data"), ident("total")),
				memExpr(ident("data"), ident("count")),
			))},
		),
	))
}

func caseBinaryMod() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			binary("==", binary("%", memExpr(ident("data"), ident("n")), numLit(2)), numLit(0)),
			buildBlock(pushString("<p>even</p>")),
			nil,
		),
	))
}

func caseUnaryMinus() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{escapeOf(unary("-", memExpr(ident("data"), ident("value"))))},
		),
	))
}

func caseUnaryPlus() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span>", "</span>"},
			[]*Node{escapeOf(unary("+", memExpr(ident("data"), ident("v"))))},
		),
	))
}

func caseUnaryNot() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			unary("!", memExpr(ident("data"), ident("disabled"))),
			buildBlock(pushString("<p>enabled</p>")),
			nil,
		),
	))
}

func caseOptionalChainDeep() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(chain(optionalMember(
				optionalMember(
					memExpr(ident("data"), ident("user")),
					ident("profile"),
				),
				ident("name"),
			)))},
		),
	))
}

func caseOptionalChainCall() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(chain(callExpr(optionalMember(
				memExpr(ident("data"), ident("user")),
				ident("getName"),
			))))},
		),
	))
}

func caseNullishWithBool() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			logical("??",
				memExpr(ident("data"), ident("active")),
				boolLit(false),
			),
			buildBlock(pushString("<p>active</p>")),
			nil,
		),
	))
}

func caseEachWithoutIndex() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("each_array",
			helperCall("ensure_array_like", memExpr(ident("data"), ident("items"))),
		),
		forLoop(
			&Node{
				Type: "VariableDeclaration",
				Kind: "let",
				Declarations: []*Node{
					{Type: "VariableDeclarator", ID: ident("$$index"), Init: numLit(0)},
					{Type: "VariableDeclarator", ID: ident("$$length"), Init: memExpr(ident("each_array"), ident("length"))},
				},
			},
			binary("<", ident("$$index"), ident("$$length")),
			update("++", ident("$$index")),
			buildBlock(
				letDecl("item", computedMember(ident("each_array"), ident("$$index"))),
				pushTemplate(
					[]string{"<li>", "</li>"},
					[]*Node{escapeOf(memExpr(ident("item"), ident("name")))},
				),
			),
		),
	))
}

func caseEachEmptyBody() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("each_array",
			helperCall("ensure_array_like", memExpr(ident("data"), ident("items"))),
		),
		forLoop(
			&Node{
				Type: "VariableDeclaration",
				Kind: "let",
				Declarations: []*Node{
					{Type: "VariableDeclarator", ID: ident("$$index"), Init: numLit(0)},
					{Type: "VariableDeclarator", ID: ident("$$length"), Init: memExpr(ident("each_array"), ident("length"))},
				},
			},
			binary("<", ident("$$index"), ident("$$length")),
			update("++", ident("$$index")),
			buildBlock(
				letDecl("item", computedMember(ident("each_array"), ident("$$index"))),
			),
		),
	))
}

func caseEachNested() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("each_array",
			helperCall("ensure_array_like", memExpr(ident("data"), ident("groups"))),
		),
		forLoop(
			&Node{
				Type: "VariableDeclaration",
				Kind: "let",
				Declarations: []*Node{
					{Type: "VariableDeclarator", ID: ident("$$index"), Init: numLit(0)},
					{Type: "VariableDeclarator", ID: ident("$$length"), Init: memExpr(ident("each_array"), ident("length"))},
				},
			},
			binary("<", ident("$$index"), ident("$$length")),
			update("++", ident("$$index")),
			buildBlock(
				letDecl("group", computedMember(ident("each_array"), ident("$$index"))),
				forOf(
					letDecl("item", nil),
					memExpr(ident("group"), ident("items")),
					buildBlock(pushTemplate(
						[]string{"<li>", "</li>"},
						[]*Node{escapeOf(memExpr(ident("item"), ident("title")))},
					)),
				),
			),
		),
	))
}

func caseEachWithStatic() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("each_array",
			helperCall("ensure_array_like", memExpr(ident("data"), ident("items"))),
		),
		forLoop(
			&Node{
				Type: "VariableDeclaration",
				Kind: "let",
				Declarations: []*Node{
					{Type: "VariableDeclarator", ID: ident("$$index"), Init: numLit(0)},
					{Type: "VariableDeclarator", ID: ident("$$length"), Init: memExpr(ident("each_array"), ident("length"))},
				},
			},
			binary("<", ident("$$index"), ident("$$length")),
			update("++", ident("$$index")),
			buildBlock(
				letDecl("item", computedMember(ident("each_array"), ident("$$index"))),
				pushString("<hr/>"),
			),
		),
	))
}

func caseForOfBareArray() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		forOf(
			letDecl("name", nil),
			arrayExpr(strLit("a"), strLit("b"), strLit("c")),
			buildBlock(pushTemplate(
				[]string{"<li>", "</li>"},
				[]*Node{escapeOf(ident("name"))},
			)),
		),
	))
}

func caseSnippetNoArgs() *Node {
	return buildProgram(buildBlock(
		constDecl("header", arrowFn(
			[]*Node{},
			buildBlock(pushString("<h2>header</h2>")),
		)),
		callStmt(ident("header")),
	))
}

func caseSnippetMultiArgs() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("row", arrowFn(
			[]*Node{ident("name"), ident("count")},
			buildBlock(pushTemplate(
				[]string{"<tr><td>", "</td><td>", "</td></tr>"},
				[]*Node{escapeOf(ident("name")), escapeOf(ident("count"))},
			)),
		)),
		callStmt(ident("row"),
			memExpr(ident("data"), ident("name")),
			memExpr(ident("data"), ident("count")),
		),
	))
}

func caseRenderWithString() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("greet", arrowFn(
			[]*Node{ident("name")},
			buildBlock(pushTemplate(
				[]string{"<p>hello ", "</p>"},
				[]*Node{escapeOf(ident("name"))},
			)),
		)),
		callStmt(ident("greet"), strLit("world")),
	))
}

func caseSnippetThenRender() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("link", arrowFn(
			[]*Node{ident("href"), ident("text")},
			buildBlock(pushTemplate(
				[]string{`<a href="`, `">`, `</a>`},
				[]*Node{escapeOf(ident("href")), escapeOf(ident("text"))},
			)),
		)),
		callStmt(ident("link"),
			binary("+", strLit("/post/"), memExpr(ident("data"), ident("id"))),
			memExpr(ident("data"), ident("title")),
		),
	))
}

func caseAtConstString() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("greeting",
			binary("+", strLit("hi, "), memExpr(ident("data"), ident("name"))),
		),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(ident("greeting"))},
		),
	))
}

func caseAtConstWithFn() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("upper",
			callExpr(memExpr(memExpr(ident("data"), ident("name")), ident("toUpperCase"))),
		),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(ident("upper"))},
		),
	))
}

func caseAtConstMulti() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("a", memExpr(ident("data"), ident("a"))),
		constDecl("b", memExpr(ident("data"), ident("b"))),
		pushTemplate(
			[]string{"<p>", " / ", "</p>"},
			[]*Node{escapeOf(ident("a")), escapeOf(ident("b"))},
		),
	))
}

func caseSlotWithFallback() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		callStmt(
			memExpr(ident("$"), ident("slot")),
			ident("$$payload"),
			ident("$$props"),
			strLit("default"),
			ident("$$props"),
			arrowFn(
				[]*Node{ident("$$payload")},
				buildBlock(pushString("<p>fallback</p>")),
			),
		),
	))
}

func caseSlotWithProps() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		callStmt(
			memExpr(ident("$"), ident("slot")),
			ident("$$payload"),
			ident("$$props"),
			strLit("item"),
			objExpr([2]*Node{ident("name"), strLit("foo")}, [2]*Node{ident("count"), numLit(3)}),
		),
	))
}

func caseHelperFallback() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("name",
			helperCall("fallback",
				memExpr(ident("data"), ident("name")),
				strLit("guest"),
			),
		),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(ident("name"))},
		),
	))
}

func caseHelperRestProps() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		constDecl("rest",
			helperCall("rest_props",
				ident("$$props"),
				strLit("class"),
				strLit("style"),
			),
		),
		pushTemplate(
			[]string{"<div", "></div>"},
			[]*Node{helperCall("spread_attributes", ident("rest"))},
		),
	))
}

func caseHelperExcludeFromObject() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		constDecl("trimmed",
			helperCall("exclude_from_object",
				ident("$$props"),
				strLit("internal"),
			),
		),
		pushTemplate(
			[]string{"<div", "></div>"},
			[]*Node{helperCall("spread_attributes", ident("trimmed"))},
		),
	))
}

func caseHelperSanitizeProps() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		constDecl("safe",
			helperCall("sanitize_props", ident("$$props")),
		),
		pushTemplate(
			[]string{"<div", "></div>"},
			[]*Node{helperCall("spread_attributes", ident("safe"))},
		),
	))
}

func caseHelperSpreadProps() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		pushTemplate(
			[]string{"<div", "></div>"},
			[]*Node{helperCall("spread_props",
				ident("$$props"),
				objExpr([2]*Node{ident("class"), strLit("extra")}),
			)},
		),
	))
}

func caseSvelteHeadMulti() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$"), ident("head")),
			strLit("h1"),
			arrowFn(
				[]*Node{ident("$$payload")},
				buildBlock(
					pushTemplate(
						[]string{"<title>", "</title>"},
						[]*Node{escapeOf(memExpr(ident("data"), ident("title")))},
					),
					pushString(`<meta name="description" content="x">`),
				),
			),
		),
	))
}

func caseSvelteElementNoAttrs() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$"), ident("element")),
			ident("$$payload"),
			memExpr(ident("data"), ident("tag")),
			arrowFn([]*Node{ident("$$payload")}, buildBlock()),
			arrowFn([]*Node{ident("$$payload")}, buildBlock(pushString("body"))),
		),
	))
}

func caseTemplateNoExprs() *Node {
	return buildProgram(buildBlock(
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			tplLit([]string{"<section>plain</section>"}, nil),
		),
	))
}

func caseTemplateLeadingExpr() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"", " trail"},
			[]*Node{escapeOf(memExpr(ident("data"), ident("a")))},
		),
	))
}

func caseTemplateTrailingExpr() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"lead ", ""},
			[]*Node{escapeOf(memExpr(ident("data"), ident("z")))},
		),
	))
}

func caseTemplateManyExprs() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", " ", " ", " ", "</p>"},
			[]*Node{
				escapeOf(memExpr(ident("data"), ident("a"))),
				escapeOf(memExpr(ident("data"), ident("b"))),
				escapeOf(memExpr(ident("data"), ident("c"))),
				escapeOf(memExpr(ident("data"), ident("d"))),
			},
		),
	))
}

func caseArrowWithStatement() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("inner", arrowFn(
			[]*Node{ident("v")},
			buildBlock(
				ifStmt(
					ident("v"),
					buildBlock(pushString("<p>truthy</p>")),
					buildBlock(pushString("<p>falsy</p>")),
				),
			),
		)),
		callStmt(ident("inner"), memExpr(ident("data"), ident("flag"))),
	))
}

func caseFunctionExpressionComponent() *Node {
	// $$renderer.component(function ($$renderer) { ... })
	return buildProgram(buildBlock(
		callStmt(
			memExpr(ident("$$renderer"), ident("component")),
			&Node{
				Type:     "FunctionExpression",
				Params:   []*Node{ident("$$renderer")},
				FuncBody: buildBlock(pushString("<aside>component</aside>")),
			},
		),
	))
}
