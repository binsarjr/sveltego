package codegen

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/parser"
)

func TestAnalyzeRunes_Props(t *testing.T) {
	t.Parallel()

	body := "let { Title, Limit = 10 } = $props()\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if !ana.HasProps {
		t.Fatalf("HasProps = false, want true")
	}
	if len(ana.Props) != 2 {
		t.Fatalf("Props len = %d, want 2; %#v", len(ana.Props), ana.Props)
	}
	if ana.Props[0].Name != "Title" || ana.Props[0].Type != "string" {
		t.Errorf("prop[0] = %+v", ana.Props[0])
	}
	if ana.Props[1].Name != "Limit" || ana.Props[1].Type != "int" || ana.Props[1].Default != "10" {
		t.Errorf("prop[1] = %+v", ana.Props[1])
	}
}

func TestAnalyzeRunes_PropsRest(t *testing.T) {
	t.Parallel()

	body := "let { A, ...Rest } = $props()\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if ana.RestField != "Rest" {
		t.Errorf("RestField = %q, want %q", ana.RestField, "Rest")
	}
	if len(ana.Props) != 2 || !ana.Props[1].Rest || ana.Props[1].Type != "map[string]any" {
		t.Errorf("rest prop missing/wrong: %#v", ana.Props)
	}
}

func TestAnalyzeRunes_Bindable(t *testing.T) {
	t.Parallel()

	body := "let { Value = $bindable(0), Label } = $props()\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if len(ana.Props) != 2 {
		t.Fatalf("len = %d, want 2", len(ana.Props))
	}
	if !ana.Props[0].Bindable {
		t.Errorf("Value not marked bindable: %+v", ana.Props[0])
	}
	if ana.Props[0].Default != "0" {
		t.Errorf("Value default = %q", ana.Props[0].Default)
	}
}

func TestAnalyzeRunes_State(t *testing.T) {
	t.Parallel()

	body := "let count = $state(5)\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if len(ana.Stmts) != 1 {
		t.Fatalf("Stmts len = %d, want 1", len(ana.Stmts))
	}
	if ana.Stmts[0].Body != "count := 5" {
		t.Errorf("body = %q", ana.Stmts[0].Body)
	}
}

func TestAnalyzeRunes_Derived(t *testing.T) {
	t.Parallel()

	body := "let count = $state(2)\nlet total = $derived(count * 3)\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if len(ana.Stmts) != 2 {
		t.Fatalf("Stmts len = %d, want 2", len(ana.Stmts))
	}
	if ana.Stmts[1].Kind != runeDerived || !strings.Contains(ana.Stmts[1].Body, "count * 3") {
		t.Errorf("derived stmt = %+v", ana.Stmts[1])
	}
}

func TestAnalyzeRunes_DerivedBy(t *testing.T) {
	t.Parallel()

	body := "let total = $derived.by(func() int { return 42 })\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if len(ana.Stmts) != 1 {
		t.Fatalf("Stmts len = %d", len(ana.Stmts))
	}
	if ana.Stmts[0].Kind != runeDerivedBy {
		t.Errorf("kind = %v", ana.Stmts[0].Kind)
	}
	if !strings.HasSuffix(strings.TrimSpace(ana.Stmts[0].Body), ")()") {
		t.Errorf("derived.by IIFE shape missing: %q", ana.Stmts[0].Body)
	}
}

func TestAnalyzeRunes_Effect(t *testing.T) {
	t.Parallel()

	body := "$effect(func() { _ = 1 })\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if len(ana.Stmts) != 1 {
		t.Fatalf("Stmts len = %d", len(ana.Stmts))
	}
	if ana.Stmts[0].Kind != runeEffect {
		t.Errorf("kind = %v", ana.Stmts[0].Kind)
	}
	if ana.Stmts[0].Body != "" {
		t.Errorf("effect body should be empty, got %q", ana.Stmts[0].Body)
	}
}

func TestAnalyzeRunes_DeclsCoexist(t *testing.T) {
	t.Parallel()

	body := "import \"strconv\"\n\nfunc helper() string { return strconv.Itoa(1) }\nlet count = $state(0)\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if len(ana.Imports) != 1 {
		t.Errorf("imports = %v", ana.Imports)
	}
	if len(ana.Decls) != 1 {
		t.Errorf("decls = %v", ana.Decls)
	}
	if len(ana.Stmts) != 1 || !strings.Contains(ana.Stmts[0].Body, "count := 0") {
		t.Errorf("stmts = %v", ana.Stmts)
	}
}

func TestAnalyzeRunes_TypeAnnotation(t *testing.T) {
	t.Parallel()

	body := "let { Foo, Bar }: { Foo string; Bar int } = $props()\n"
	rewritten, _ := rewriteRunes(body)
	ana, err := analyzeRunes(rewritten, ast.Pos{Line: 1, Col: 1})
	if err != nil {
		t.Fatalf("analyzeRunes: %v", err)
	}
	if len(ana.Props) != 2 {
		t.Fatalf("len = %d", len(ana.Props))
	}
	if ana.Props[0].Type != "string" || ana.Props[1].Type != "int" {
		t.Errorf("annotations not honored: %+v", ana.Props)
	}
}

func TestGenerate_PropsEndToEnd(t *testing.T) {
	t.Parallel()

	src := []byte("<script lang=\"go\">\n\tlet { Greeting = \"hi\" } = $props()\n</script>\n<p>{props.Greeting}</p>\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want := []string{
		"type Props struct {",
		"Greeting string",
		"func defaultProps(p *Props)",
		"var props Props",
		"defaultProps(&props)",
		"props.Greeting",
	}
	for _, w := range want {
		if !strings.Contains(string(out), w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
}

func TestGenerate_StateLowering(t *testing.T) {
	t.Parallel()

	src := []byte("<script lang=\"go\">\n\tlet count = $state(7)\n</script>\n<p>{count}</p>\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(string(out), "count := 7") {
		t.Errorf("missing `count := 7`:\n%s", out)
	}
}

func TestGenerate_EffectStripped(t *testing.T) {
	t.Parallel()

	src := []byte("<script lang=\"go\">\n\t$effect(func() { _ = 1 })\n</script>\n<p>hi</p>\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if strings.Contains(string(out), "$effect(") {
		t.Errorf("$effect not stripped:\n%s", out)
	}
}
