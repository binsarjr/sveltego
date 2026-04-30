package codegen

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/parser"
	"github.com/binsarjr/sveltego/test-utils/golden"
)

// TestGenerateComponent_Fixtures walks testdata/codegen/component/*.svelte
// fixtures through GenerateComponent and snapshots the result against
// testdata/golden/component/<name>.gen.go.golden. The component name is
// derived from the fixture filename (the segment before the first dash,
// PascalCased) so naming pressure stays on the fixture file.
func TestGenerateComponent_Fixtures(t *testing.T) {
	t.Parallel()
	root := "testdata/component"
	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".svelte") {
			return nil
		}
		matches = append(matches, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(matches) < 4 {
		t.Fatalf("expected >= 4 component fixtures, found %d", len(matches))
	}
	sort.Strings(matches)
	for _, path := range matches {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("rel %s: %v", path, err)
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ".svelte")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			frag, errs := parser.Parse(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			res, err := GenerateComponent(frag, ComponentOptions{
				PackageName:   "comp",
				ComponentName: "Comp",
			})
			if err != nil {
				t.Fatalf("GenerateComponent: %v", err)
			}
			golden.EqualString(t, "component/"+name+".gen.go", string(res.Source))
		})
	}
}

// TestGenerateComponent_DefaultSlot covers #49 acceptance:
// "Default slot renders caller content."
//
// The lowering produces a Slots struct with a Default field whose
// closure body is invoked when present and falls back to the template's
// inline content when the caller did not provide one.
func TestGenerateComponent_DefaultSlot(t *testing.T) {
	src := []byte(`<div class="card"><slot>nothing here</slot></div>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := GenerateComponent(frag, ComponentOptions{
		PackageName:   "card",
		ComponentName: "Card",
	})
	if err != nil {
		t.Fatalf("GenerateComponent: %v", err)
	}
	out := string(res.Source)
	for _, want := range []string{
		"type CardSlots struct {",
		"Default func(w *render.Writer) error",
		"if slots.Default != nil {",
		"if err := slots.Default(w); err != nil {",
		"} else {",
		"`nothing here`",
		"func (c Card) Render(w *render.Writer, ctx *kit.RenderCtx, props CardProps, slots CardSlots)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if len(res.Diagnostics) != 1 {
		t.Fatalf("expected 1 deprecation diagnostic, got %d", len(res.Diagnostics))
	}
	if res.Diagnostics[0].Severity != DiagInfo {
		t.Errorf("severity = %v, want Info", res.Diagnostics[0].Severity)
	}
	if !strings.Contains(res.Diagnostics[0].Message, "<slot> is legacy") {
		t.Errorf("message = %q, want substring %q", res.Diagnostics[0].Message, "<slot> is legacy")
	}
}

// TestGenerateComponent_NamedSlot covers #49 acceptance:
// "Named slot routes correct child elements." plus
// "Multiple slots in same component coexist."
func TestGenerateComponent_NamedSlot(t *testing.T) {
	src := []byte(`<header><slot name="header"/></header><main><slot/></main>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := GenerateComponent(frag, ComponentOptions{
		PackageName:   "card",
		ComponentName: "Card",
	})
	if err != nil {
		t.Fatalf("GenerateComponent: %v", err)
	}
	out := string(res.Source)
	for _, want := range []string{
		"Default func(w *render.Writer) error",
		"Header  func(w *render.Writer) error",
		"if slots.Header != nil {",
		"if slots.Default != nil {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestGenerateComponent_SlotProps covers #50 acceptance:
// "Slot prop visible in caller's content." The receiver-side outlet
// `<slot item={item}/>` lowers to a closure parameter named `item`.
func TestGenerateComponent_SlotProps(t *testing.T) {
	src := []byte(`<slot item={item}/>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := GenerateComponent(frag, ComponentOptions{
		PackageName:   "list",
		ComponentName: "List",
	})
	if err != nil {
		t.Fatalf("GenerateComponent: %v", err)
	}
	out := string(res.Source)
	for _, want := range []string{
		"Default func(item any, w *render.Writer) error",
		"if err := slots.Default(item, w); err != nil {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestGenerateComponent_NamedSlotWithProp covers #50 acceptance:
// "Named slot with prop works."
func TestGenerateComponent_NamedSlotWithProp(t *testing.T) {
	src := []byte(`<slot name="row" row={row}/>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := GenerateComponent(frag, ComponentOptions{
		PackageName:   "list",
		ComponentName: "List",
	})
	if err != nil {
		t.Fatalf("GenerateComponent: %v", err)
	}
	out := string(res.Source)
	for _, want := range []string{
		"Row func(row any, w *render.Writer) error",
		"if err := slots.Row(row, w); err != nil {",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// TestGenerate_ComponentCaller_DefaultSlot covers the caller-side half
// of #49: lifting a component's body into the Default slot closure on
// the call site.
func TestGenerate_ComponentCaller_DefaultSlot(t *testing.T) {
	src := []byte(`<Card><p>Body content.</p></Card>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src2 := string(out)
	for _, want := range []string{
		"_slots := card.CardSlots{}",
		"_slots.Default = func(w *render.Writer) error {",
		"`<p>`",
		"`Body content.`",
		"if err := (card.Card{}).Render(w, ctx, card.CardProps{}, _slots); err != nil {",
	} {
		if !strings.Contains(src2, want) {
			t.Errorf("missing %q in:\n%s", want, src2)
		}
	}
}

// TestGenerate_ComponentCaller_NamedSlot covers the caller-side half
// of #49 (named slot routing) and the let-binding from #50.
func TestGenerate_ComponentCaller_NamedSlot(t *testing.T) {
	src := []byte(`<Card><h1 slot="header" let:item>{item}</h1></Card>` + "\n")
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
		"_slots.Header = func(item any, w *render.Writer) error {",
		"`<h1>`",
		"w.WriteEscape(item)",
		"`</h1>`",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_ComponentCaller_LetAlias covers the let-binding alias
// form `let:item={alias}` — the closure parameter keeps the slot prop
// key, the body sees the user-chosen local.
func TestGenerate_ComponentCaller_LetAlias(t *testing.T) {
	src := []byte(`<List let:item={post}>{post.Title}</List>` + "\n")
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
		"_slots.Default = func(item any, w *render.Writer) error {",
		"post := item",
		"_ = post",
		"w.WriteEscape(post.Title)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_SvelteBody covers #52: <svelte:body> emits no markup.
func TestGenerate_SvelteBody(t *testing.T) {
	src := []byte(`<svelte:body onresize={Handler} />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "svelte:body") {
		t.Errorf("expected no svelte:body markup, got:\n%s", s)
	}
	if strings.Contains(s, "<body") || strings.Contains(s, "</body>") {
		t.Errorf("expected no body element markup, got:\n%s", s)
	}
}

// TestGenerate_SvelteWindowAndDocument covers #52 for the other two
// special elements.
func TestGenerate_SvelteWindowAndDocument(t *testing.T) {
	cases := []string{
		`<svelte:window onscroll={Scroll} />` + "\n",
		`<svelte:document onkeydown={Key} />` + "\n",
	}
	for _, src := range cases {
		frag, errs := parser.Parse([]byte(src))
		if len(errs) > 0 {
			t.Fatalf("parse %q: %v", src, errs)
		}
		out, err := Generate(frag, Options{PackageName: "page"})
		if err != nil {
			t.Fatalf("Generate %q: %v", src, err)
		}
		s := string(out)
		for _, banned := range []string{"svelte:window", "svelte:document", "<window", "<document"} {
			if strings.Contains(s, banned) {
				t.Errorf("expected no %q markup, got:\n%s", banned, s)
			}
		}
	}
}

// TestGenerate_SvelteBodyMisplacedDiagnostic covers #52: the element
// must be at the template root. Wrapping it in a {#if} surfaces a
// codegen error.
func TestGenerate_SvelteBodyMisplacedDiagnostic(t *testing.T) {
	src := []byte(`{#if Flag}<svelte:body />{/if}` + "\n")
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

// TestGenerate_SvelteBodyRejectsChildren validates the no-children rule
// on the special elements.
func TestGenerate_SvelteBodyRejectsChildren(t *testing.T) {
	src := []byte(`<svelte:body>nope</svelte:body>` + "\n")
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
	if !strings.Contains(ce.Msg, "may not have children") {
		t.Errorf("msg = %q, want substring %q", ce.Msg, "may not have children")
	}
}

// TestGenerate_SvelteComponentDispatch covers #53: the dynamic
// dispatcher emits a Render call against the user expression.
func TestGenerate_SvelteComponentDispatch(t *testing.T) {
	src := []byte(`<svelte:component this={Picked} />` + "\n")
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
		"if err := Picked.Render(w); err != nil {",
		"return err",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_SvelteComponentRequiresThis ensures the missing-this
// case surfaces a clear diagnostic instead of compiling to an
// orphaned method call.
func TestGenerate_SvelteComponentRequiresThis(t *testing.T) {
	src := []byte(`<svelte:component />` + "\n")
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
	if !strings.Contains(ce.Msg, "this={expr}") {
		t.Errorf("msg = %q, want substring %q", ce.Msg, "this={expr}")
	}
}

// TestSlotFieldName guards the slot-name to PascalCase conversion that
// is the bridge between Svelte syntax and Go identifiers.
func TestSlotFieldName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "Default"},
		{"default", "Default"},
		{"header", "Header"},
		{"row-item", "RowItem"},
		{"row_item", "RowItem"},
		{"someName", "SomeName"},
	}
	for _, tc := range cases {
		if got := slotFieldName(tc.in); got != tc.want {
			t.Errorf("slotFieldName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
