package codegen

import (
	"errors"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/parser"
)

// TestGenerate_SvelteHeadEmitsHeadMethod covers the page-side half of #51:
// content inside <svelte:head> lifts into a sibling Head method while the
// Render method emits the rest of the body normally.
func TestGenerate_SvelteHeadEmitsHeadMethod(t *testing.T) {
	src := []byte(`<svelte:head>
  <title>About</title>
  <meta name="description" content="about page">
</svelte:head>
<h1>About</h1>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"func (p Page) Render(w *render.Writer, ctx *kit.RenderCtx, data PageData) error {",
		"func (p Page) Head(w *render.Writer, ctx *kit.RenderCtx, data PageData) error {",
		"`<title>`",
		"`About`",
		"`<h1>`",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "svelte:head") {
		t.Errorf("expected no <svelte:head> markup in body:\n%s", s)
	}
}

// TestGenerate_NoSvelteHeadOmitsHeadMethod ensures the Head method is
// only emitted when <svelte:head> is present so existing components keep
// their compact output.
func TestGenerate_NoSvelteHeadOmitsHeadMethod(t *testing.T) {
	src := []byte(`<h1>Hello</h1>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "func (p Page) Head") {
		t.Errorf("expected no Head method, got:\n%s", s)
	}
}

// TestGenerate_SvelteHeadWithDynamicContent threads a Go expression
// through the head buffer to confirm the Head body lowers the same
// mustache pipeline as Render.
func TestGenerate_SvelteHeadWithDynamicContent(t *testing.T) {
	src := []byte(`<svelte:head>
  <title>{data.Title}</title>
</svelte:head>
<p>body</p>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"func (p Page) Head(",
		"w.WriteEscape(data.Title)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_SvelteHeadConditional confirms that {#if} inside
// <svelte:head> lowers normally — head children flow through emitChildren.
func TestGenerate_SvelteHeadConditional(t *testing.T) {
	src := []byte(`<svelte:head>
  {#if data.Indexable}
    <meta name="robots" content="index,follow">
  {/if}
</svelte:head>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"func (p Page) Head(",
		"if data.Indexable {",
		`<meta name="robots"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_SvelteHeadNestedRejected covers the spec rule that
// <svelte:head> may only appear at the template root.
func TestGenerate_SvelteHeadNestedRejected(t *testing.T) {
	src := []byte(`{#if Flag}<svelte:head><title>x</title></svelte:head>{/if}` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	_, err := Generate(frag, Options{PackageName: "page"})
	if err == nil {
		t.Fatal("expected codegen error, got nil")
	}
	var ce *CodegenError
	if !errors.As(err, &ce) {
		t.Fatalf("got %T, want *CodegenError", err)
	}
	if !strings.Contains(ce.Msg, "must appear at the template root") {
		t.Errorf("msg = %q, want substring %q", ce.Msg, "must appear at the template root")
	}
}

// TestGenerateLayout_SvelteHead covers the layout-side half of #51.
// Layouts emit a Head method just like pages so the pipeline can
// gather the chain's head buffers.
func TestGenerateLayout_SvelteHead(t *testing.T) {
	src := []byte(`<svelte:head>
  <link rel="icon" href="/favicon.png">
</svelte:head>
<slot />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := GenerateLayout(frag, LayoutOptions{PackageName: "layout"})
	if err != nil {
		t.Fatalf("GenerateLayout: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"func (l Layout) Head(w *render.Writer, ctx *kit.RenderCtx, data LayoutData) error {",
		`<link rel="icon"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_MultipleSvelteHead concatenates content across multiple
// root-level <svelte:head> blocks in source order.
func TestGenerate_MultipleSvelteHead(t *testing.T) {
	src := []byte(`<svelte:head><title>A</title></svelte:head>
<p>body</p>
<svelte:head><meta charset="utf-8"></svelte:head>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "func (p Page) Head(") {
		t.Errorf("missing Head method in:\n%s", s)
	}
	titleIdx := strings.Index(s, "`A`")
	metaIdx := strings.Index(s, `<meta charset="utf-8">`)
	if titleIdx < 0 || metaIdx < 0 || titleIdx > metaIdx {
		t.Errorf("expected title before meta in head body:\n%s", s)
	}
}
