package sveltejs2go

// allProgrammaticCases enumerates the 30 priority emit shapes from
// issue #425. The hello-world and each-list shapes come from the
// sidecar fixtures (TestTranspile_SidecarFixtures); the remainder
// live here as synthesized AST trees so the test suite doesn't need
// the Node sidecar in CI.
func allProgrammaticCases() []programmaticCase {
	return []programmaticCase{
		// 1) string literal append → payload.Push.
		{name: "string-literal", root: caseStringLiteral},
		// 2) escape_html(expr) interpolation.
		{name: "escape-interp", root: caseEscapeInterp},
		// 3) tagged template literal — covered by hello-world fixture; here we add
		//    the corner case where the tagged template wraps a single literal.
		{name: "tagged-template-literal", root: caseTaggedTemplate},
		// 4) multi-line template literal split.
		{name: "multi-line-template", root: caseMultiLineTemplate},
		// 5) if/else lowering.
		{name: "if-else", root: caseIfElse},
		// 6) else if chain.
		{name: "else-if-chain", root: caseElseIfChain},
		// 7) for-of loop.
		{name: "for-of", root: caseForOf},
		// 8) nested for/if.
		{name: "nested-for-if", root: caseNestedForIf},
		// 9) ternary in template literal.
		{name: "ternary-in-template", root: caseTernaryInTemplate},
		// 10) `&&` / `||` short-circuit.
		{name: "logical-and", root: caseLogicalAnd},
		// 11) optional chaining `?.`.
		{name: "optional-chain", root: caseOptionalChain},
		// 12) nullish coalescing `??`.
		{name: "nullish-coalesce", root: caseNullishCoalesce},
		// 13) static attribute literal.
		{name: "static-attr", root: caseStaticAttr},
		// 14) attr(name, value) helper.
		{name: "attr-helper", root: caseAttrHelper},
		// 15) class: directive → clsx.
		{name: "class-directive", root: caseClassDirective},
		// 16) style: directive → merge_styles.
		{name: "style-directive", root: caseStyleDirective},
		// 17) spread_attributes.
		{name: "spread-attrs", root: caseSpreadAttrs},
		// 18) component invocation — covered by hello-world fixture.
		//     Here exercise a nested component closure.
		{name: "component-nested", root: caseComponentNested},
		// 19) children/default slot.
		{name: "slot-default", root: caseSlotDefault},
		// 20) named slots.
		{name: "slot-named", root: caseSlotNamed},
		// 21) <svelte:head>.
		{name: "svelte-head", root: caseSvelteHead},
		// 22) <svelte:component this={...}>.
		{name: "svelte-element-dynamic", root: caseSvelteElement},
		// 23) {#if} lowering.
		{name: "if-block", root: caseIfBlock},
		// 24) {#each} with index.
		{name: "each-with-index", root: caseEachWithIndex},
		// 25) {#each} with destructuring — emitted similarly; we
		//     synthesize the typical lowering.
		{name: "each-destructure", root: caseEachDestructure},
		// 26) {#key} (no-op server) — Svelte emits it as a normal
		//     block; we capture that no special handling is needed.
		{name: "key-block", root: caseKeyBlock},
		// 27) {@html raw} — deferred per Phase 4 quirks.
		//     Tested separately in TestTranspile_HTMLDeferred.
		// 28) {@const name = expr} → Go local.
		{name: "atconst", root: caseAtConst},
		// 29) {#snippet} → Go closure.
		{name: "snippet-closure", root: caseSnippet},
		// 30) {@render snippetCall(args)} → invoke closure.
		{name: "render-snippet", root: caseRenderSnippet},
	}
}

func caseStringLiteral() *Node {
	return buildProgram(buildBlock(pushString("<p>hello</p>")))
}

func caseEscapeInterp() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<h1>", "</h1>"},
			[]*Node{escapeOf(memExpr(ident("data"), ident("name")))},
		),
	))
}

func caseTaggedTemplate() *Node {
	// Single-quasi template with no expressions — exercises the
	// degenerate split.
	return buildProgram(buildBlock(
		callStmt(
			memExpr(ident("$$renderer"), ident("push")),
			tplLit([]string{"<section>only static</section>"}, nil),
		),
	))
}

func caseMultiLineTemplate() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<div>\n  <p>line one</p>\n  <p>", "</p>\n  <p>line three</p>\n</div>"},
			[]*Node{escapeOf(memExpr(ident("data"), ident("body")))},
		),
	))
}

func caseIfElse() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			memExpr(ident("data"), ident("loggedIn")),
			buildBlock(pushString("<p>welcome back</p>")),
			buildBlock(pushString("<p>please sign in</p>")),
		),
	))
}

func caseElseIfChain() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			binary("==", memExpr(ident("data"), ident("status")), strLit("ok")),
			buildBlock(pushString("<p>ok</p>")),
			ifStmt(
				binary("==", memExpr(ident("data"), ident("status")), strLit("warn")),
				buildBlock(pushString("<p>warn</p>")),
				buildBlock(pushString("<p>error</p>")),
			),
		),
	))
}

func caseForOf() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		forOf(
			letDecl("item", nil),
			memExpr(ident("data"), ident("items")),
			buildBlock(pushTemplate(
				[]string{"<li>", "</li>"},
				[]*Node{escapeOf(memExpr(ident("item"), ident("title")))},
			)),
		),
	))
}

func caseNestedForIf() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		forOf(
			letDecl("group", nil),
			memExpr(ident("data"), ident("groups")),
			buildBlock(
				ifStmt(
					memExpr(ident("group"), ident("visible")),
					buildBlock(forOf(
						letDecl("item", nil),
						memExpr(ident("group"), ident("items")),
						buildBlock(pushTemplate(
							[]string{"<li>", "</li>"},
							[]*Node{escapeOf(memExpr(ident("item"), ident("name")))},
						)),
					)),
					nil,
				),
			),
		),
	))
}

func caseTernaryInTemplate() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<span class=\"", "\"></span>"},
			[]*Node{conditional(
				memExpr(ident("data"), ident("active")),
				strLit("on"),
				strLit("off"),
			)},
		),
	))
}

func caseLogicalAnd() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			logical("&&",
				memExpr(ident("data"), ident("ready")),
				memExpr(ident("data"), ident("visible")),
			),
			buildBlock(pushString("<p>ready</p>")),
			nil,
		),
	))
}

func caseOptionalChain() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(chain(optionalMember(
				memExpr(ident("data"), ident("user")),
				ident("name"),
			)))},
		),
	))
}

func caseNullishCoalesce() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(logical("??",
				memExpr(ident("data"), ident("name")),
				strLit("guest"),
			))},
		),
	))
}

func caseStaticAttr() *Node {
	// `<button class="btn">click</button>` — entirely static template.
	return buildProgram(buildBlock(
		pushString(`<button class="btn">click</button>`),
	))
}

func caseAttrHelper() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<input", " />"},
			[]*Node{helperCall("attr",
				strLit("value"),
				memExpr(ident("data"), ident("value")),
				boolLit(false),
			)},
		),
	))
}

func caseClassDirective() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<span class="`, `"></span>`},
			[]*Node{helperCall("clsx",
				strLit("base"),
				memExpr(ident("data"), ident("classes")),
			)},
		),
	))
}

func caseStyleDirective() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<div style="`, `"></div>`},
			[]*Node{helperCall("merge_styles",
				strLit("color: red"),
				memExpr(ident("data"), ident("style")),
			)},
		),
	))
}

func caseSpreadAttrs() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{"<div", "></div>"},
			[]*Node{helperCall("spread_attributes",
				memExpr(ident("data"), ident("attrs")),
			)},
		),
	))
}

func caseComponentNested() *Node {
	// $$renderer.component(($$renderer) => { ... }) wrapping a push.
	return buildProgram(buildBlock(
		callStmt(
			memExpr(ident("$$renderer"), ident("component")),
			arrowFn(
				[]*Node{ident("$$renderer")},
				buildBlock(pushString("<section>nested</section>")),
			),
		),
	))
}

func caseSlotDefault() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		callStmt(
			memExpr(ident("$"), ident("slot")),
			ident("$$payload"),
			ident("$$props"),
			strLit("default"),
		),
	))
}

func caseSlotNamed() *Node {
	return buildProgram(buildBlock(
		propsDestructure("$$props"),
		callStmt(
			memExpr(ident("$"), ident("slot")),
			ident("$$payload"),
			ident("$$props"),
			strLit("header"),
			objExpr([2]*Node{ident("title"), strLit("welcome")}),
		),
	))
}

func caseSvelteHead() *Node {
	return buildProgram(buildBlock(
		callStmt(
			memExpr(ident("$"), ident("head")),
			strLit("h1"),
			arrowFn(
				[]*Node{ident("$$payload")},
				buildBlock(pushString("<title>About</title>")),
			),
		),
	))
}

func caseSvelteElement() *Node {
	return buildProgram(buildBlock(
		propsDestructure("data"),
		callStmt(
			memExpr(ident("$"), ident("element")),
			ident("$$payload"),
			memExpr(ident("data"), ident("tag")),
			arrowFn([]*Node{ident("$$payload")}, buildBlock()),
			arrowFn([]*Node{ident("$$payload")}, buildBlock(pushString("inner"))),
		),
	))
}

func caseIfBlock() *Node {
	// {#if data.show} ... {/if} → IfStatement at the top of the
	// render body (same shape as caseIfElse minus the alt branch).
	return buildProgram(buildBlock(
		propsDestructure("data"),
		ifStmt(
			memExpr(ident("data"), ident("show")),
			buildBlock(pushString("<p>shown</p>")),
			nil,
		),
	))
}

func caseEachWithIndex() *Node {
	// Mirror Svelte's each-array lowering pattern.
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
					[]string{"<li>", " (", ")</li>"},
					[]*Node{
						escapeOf(memExpr(ident("item"), ident("title"))),
						escapeOf(ident("$$index")),
					},
				),
			),
		),
	))
}

func caseEachDestructure() *Node {
	// {#each data.items as { name, count }} — Svelte lowers to an
	// each-array loop with let { name, count } = each_array[$$index].
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
				letDecl("entry", computedMember(ident("each_array"), ident("$$index"))),
				pushTemplate(
					[]string{"<li>", " x ", "</li>"},
					[]*Node{
						escapeOf(memExpr(ident("entry"), ident("name"))),
						escapeOf(memExpr(ident("entry"), ident("count"))),
					},
				),
			),
		),
	))
}

func caseKeyBlock() *Node {
	// {#key data.token} ... {/key} — Svelte server-side wraps in a
	// regular block. We just verify the inner content emits.
	return buildProgram(buildBlock(
		propsDestructure("data"),
		pushString("<div>keyed</div>"),
	))
}

func caseAtConst() *Node {
	// {@const total = data.a + data.b} → const total := ...
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("total", binary("+",
			memExpr(ident("data"), ident("a")),
			memExpr(ident("data"), ident("b")),
		)),
		pushTemplate(
			[]string{"<p>", "</p>"},
			[]*Node{escapeOf(ident("total"))},
		),
	))
}

func caseSnippet() *Node {
	// {#snippet card(item)} ... {/snippet} → const card = func(...) {...}
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("card", arrowFn(
			[]*Node{ident("item")},
			buildBlock(pushTemplate(
				[]string{"<article>", "</article>"},
				[]*Node{escapeOf(memExpr(ident("item"), ident("name")))},
			)),
		)),
	))
}

func caseRenderSnippet() *Node {
	// {@render card(data.item)} → call closure.
	return buildProgram(buildBlock(
		propsDestructure("data"),
		constDecl("card", arrowFn(
			[]*Node{ident("item")},
			buildBlock(pushString("<article></article>")),
		)),
		callStmt(ident("card"), memExpr(ident("data"), ident("item"))),
	))
}
