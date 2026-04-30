package parser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/test-utils/golden"
)

func parseOK(t *testing.T, src string) *ast.Fragment {
	t.Helper()
	frag, errs := Parse([]byte(src))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	return frag
}

func TestEmpty(t *testing.T) {
	frag := parseOK(t, "")
	if len(frag.Children) != 0 {
		t.Fatalf("expected zero children, got %d", len(frag.Children))
	}
}

func TestTextOnly(t *testing.T) {
	frag := parseOK(t, "hello world")
	if len(frag.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(frag.Children))
	}
	tx, ok := frag.Children[0].(*ast.Text)
	if !ok {
		t.Fatalf("expected *ast.Text, got %T", frag.Children[0])
	}
	if tx.Value != "hello world" {
		t.Fatalf("text mismatch: %q", tx.Value)
	}
}

func TestSimpleElement(t *testing.T) {
	frag := parseOK(t, "<div>hi</div>")
	el := frag.Children[0].(*ast.Element)
	if el.Name != "div" || el.Component {
		t.Fatalf("element header wrong: %+v", el)
	}
	if len(el.Children) != 1 {
		t.Fatalf("expected one text child, got %d", len(el.Children))
	}
	if el.Children[0].(*ast.Text).Value != "hi" {
		t.Fatalf("inner text: %q", el.Children[0].(*ast.Text).Value)
	}
}

func TestComponentDetection(t *testing.T) {
	cases := map[string]bool{
		"<Button />":     true,
		"<Form.Field />": true,
		"<svelte:head>":  false,
		"<div>":          false,
	}
	for src, want := range cases {
		full := src
		if !strings.HasSuffix(src, "/>") {
			full = src + "</" + strings.TrimPrefix(strings.TrimSuffix(src, ">"), "<") + ">"
		}
		frag, _ := Parse([]byte(full))
		if len(frag.Children) == 0 {
			t.Fatalf("no children for %q", src)
		}
		el, ok := frag.Children[0].(*ast.Element)
		if !ok {
			t.Fatalf("not an element for %q", src)
		}
		if el.Component != want {
			t.Fatalf("%q: Component=%v want %v", src, el.Component, want)
		}
	}
}

func TestAttributes(t *testing.T) {
	frag := parseOK(t, `<a href="/x" target='_blank' disabled class={Theme} on:click={Handle} bind:value={Name} use:enhance class:active={Ok} style:color="red">x</a>`)
	el := frag.Children[0].(*ast.Element)
	if len(el.Attributes) != 9 {
		t.Fatalf("expected 9 attrs, got %d", len(el.Attributes))
	}
	wantKinds := []ast.AttrKind{
		ast.AttrStatic, ast.AttrStatic, ast.AttrStatic,
		ast.AttrStatic,
		ast.AttrEventHandler, ast.AttrBind, ast.AttrUse,
		ast.AttrClassDirective, ast.AttrStyleDirective,
	}
	for i, want := range wantKinds {
		if got := el.Attributes[i].Kind; got != want {
			t.Fatalf("attr %d kind=%s want %s", i, got, want)
		}
	}
}

func TestMustache(t *testing.T) {
	frag := parseOK(t, "{Data.User.Name}")
	m := frag.Children[0].(*ast.Mustache)
	if m.Expr != "Data.User.Name" {
		t.Fatalf("expr: %q", m.Expr)
	}
}

func TestIfElseElseif(t *testing.T) {
	frag := parseOK(t, "{#if A}a{:else if B}b{:else}c{/if}")
	ib := frag.Children[0].(*ast.IfBlock)
	if ib.Cond != "A" {
		t.Fatalf("cond: %q", ib.Cond)
	}
	if len(ib.Elifs) != 1 || ib.Elifs[0].Cond != "B" {
		t.Fatalf("elif: %+v", ib.Elifs)
	}
	if len(ib.Else) != 1 {
		t.Fatalf("else children: %d", len(ib.Else))
	}
}

func TestEachWithIndexAndKey(t *testing.T) {
	frag := parseOK(t, "{#each Items as item, i (item.ID)}{i}{/each}")
	eb := frag.Children[0].(*ast.EachBlock)
	if eb.Iter != "Items" || eb.Item != "item" || eb.Index != "i" || eb.Key != "item.ID" {
		t.Fatalf("each parts: %+v", eb)
	}
}

func TestEachElseFallback(t *testing.T) {
	frag := parseOK(t, "{#each Items as item}{item}{:else}empty{/each}")
	eb := frag.Children[0].(*ast.EachBlock)
	if len(eb.Else) != 1 {
		t.Fatalf("expected else fallback, got %+v", eb)
	}
}

func TestAwait(t *testing.T) {
	frag := parseOK(t, "{#await Promise}loading{:then v}{v}{:catch e}{e}{/await}")
	ab := frag.Children[0].(*ast.AwaitBlock)
	if ab.Expr != "Promise" || ab.ThenVar != "v" || ab.CatchVar != "e" {
		t.Fatalf("await: %+v", ab)
	}
	if len(ab.Pending) != 1 || len(ab.Then) != 1 || len(ab.Catch) != 1 {
		t.Fatalf("await branches: %+v", ab)
	}
}

func TestKey(t *testing.T) {
	frag := parseOK(t, "{#key Data.Id}{Data.Id}{/key}")
	kb := frag.Children[0].(*ast.KeyBlock)
	if kb.Key != "Data.Id" {
		t.Fatalf("key: %q", kb.Key)
	}
}

func TestSnippet(t *testing.T) {
	frag := parseOK(t, "{#snippet card(p Post)}<div>{p.Name}</div>{/snippet}")
	sb := frag.Children[0].(*ast.SnippetBlock)
	if sb.Name != "card" || sb.Params != "p Post" {
		t.Fatalf("snippet: %+v", sb)
	}
}

func TestAtTags(t *testing.T) {
	cases := map[string]ast.Node{
		"{@html Data.Raw}":          &ast.RawHTML{Expr: "Data.Raw"},
		"{@const total := a + b}":   &ast.Const{Stmt: "total := a + b"},
		"{@render card(Data.Item)}": &ast.Render{Expr: "card(Data.Item)"},
	}
	for src, want := range cases {
		frag := parseOK(t, src)
		got := frag.Children[0]
		switch w := want.(type) {
		case *ast.RawHTML:
			if g, ok := got.(*ast.RawHTML); !ok || g.Expr != w.Expr {
				t.Fatalf("%q: got %#v", src, got)
			}
		case *ast.Const:
			if g, ok := got.(*ast.Const); !ok || g.Stmt != w.Stmt {
				t.Fatalf("%q: got %#v", src, got)
			}
		case *ast.Render:
			if g, ok := got.(*ast.Render); !ok || g.Expr != w.Expr {
				t.Fatalf("%q: got %#v", src, got)
			}
		}
	}
}

func TestScriptGoLang(t *testing.T) {
	frag := parseOK(t, `<script lang="go">var X = 1</script>`)
	s := frag.Children[0].(*ast.Script)
	if s.Lang != "go" || s.Body != "var X = 1" {
		t.Fatalf("script: %+v", s)
	}
}

func TestScriptUnsupportedLangIsError(t *testing.T) {
	frag, errs := Parse([]byte(`<script lang="ts">type X = number;</script>`))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d (%v)", len(errs), errs)
	}
	s := frag.Children[0].(*ast.Script)
	if s.Lang != "ts" {
		t.Fatalf("lang: %q", s.Lang)
	}
}

func TestScriptModule(t *testing.T) {
	frag := parseOK(t, `<script module>export const snapshot = {};</script>`)
	s := frag.Children[0].(*ast.Script)
	if !s.Module {
		t.Fatalf("expected Module=true: %+v", s)
	}
	if s.Lang != "" {
		t.Fatalf("expected default lang for module script: %q", s.Lang)
	}
	if s.Body != `export const snapshot = {};` {
		t.Fatalf("body mismatch: %q", s.Body)
	}
}

func TestScriptModuleAcceptsTSLang(t *testing.T) {
	frag := parseOK(t, `<script module lang="ts">export const snapshot: any = {};</script>`)
	s := frag.Children[0].(*ast.Script)
	if !s.Module || s.Lang != "ts" {
		t.Fatalf("expected module + lang=ts: %+v", s)
	}
}

func TestScriptModuleRejectsGoLang(t *testing.T) {
	_, errs := Parse([]byte(`<script module lang="go">var X = 1</script>`))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d (%v)", len(errs), errs)
	}
}

func TestScriptRegularRejectsModuleAttr(t *testing.T) {
	// `<script module>` is the canonical Svelte 5 module-context form;
	// the legacy `<script context="module">` is intentionally not
	// accepted. A bare module attribute on a regular Go script is
	// treated as module + default lang, which is the same path the
	// JS-only snapshot block goes through.
	frag := parseOK(t, `<script module>console.log(1)</script>`)
	s := frag.Children[0].(*ast.Script)
	if !s.Module {
		t.Fatalf("expected module=true: %+v", s)
	}
}

func TestStyle(t *testing.T) {
	frag := parseOK(t, `<style>a > b { color: red; }</style>`)
	st := frag.Children[0].(*ast.Style)
	if !strings.Contains(st.Body, "color: red") {
		t.Fatalf("style: %q", st.Body)
	}
}

func TestComment(t *testing.T) {
	frag := parseOK(t, `<!-- hi --><div></div>`)
	if _, ok := frag.Children[0].(*ast.Comment); !ok {
		t.Fatalf("not a comment: %T", frag.Children[0])
	}
}

func TestSelfClosingVoid(t *testing.T) {
	frag := parseOK(t, `<br /><img src="x" />`)
	if len(frag.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(frag.Children))
	}
}

func TestVoidElementNoClosing(t *testing.T) {
	frag := parseOK(t, `<input type="text"><div>x</div>`)
	if len(frag.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(frag.Children))
	}
	if frag.Children[0].(*ast.Element).Name != "input" {
		t.Fatalf("first not input: %+v", frag.Children[0])
	}
}

func TestMultiErrorRecovery(t *testing.T) {
	src := `<div></span><p></strong>`
	_, errs := Parse([]byte(src))
	if len(errs) < 2 {
		t.Fatalf("expected ≥ 2 errors for input with two mistakes, got %d (%v)", len(errs), errs)
	}
}

func TestErrorPositions(t *testing.T) {
	src := "hello\n{@bogus x}"
	_, errs := Parse([]byte(src))
	if len(errs) == 0 {
		t.Fatalf("expected error")
	}
	e := errs[0]
	if e.Pos.Line != 2 {
		t.Fatalf("line: %d", e.Pos.Line)
	}
}

func TestFixtures(t *testing.T) {
	matches, err := filepath.Glob("testdata/parser/*.svelte")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) < 30 {
		t.Fatalf("expected >= 30 fixtures, found %d", len(matches))
	}
	sort.Strings(matches)
	for _, path := range matches {
		name := strings.TrimSuffix(filepath.Base(path), ".svelte")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			frag, errs := Parse(src)
			rendered := Dump(frag)
			if len(errs) > 0 {
				rendered += "\n--errors--\n" + DumpErrors(errs)
			}
			golden.EqualString(t, "parser/"+name, rendered)
		})
	}
}
