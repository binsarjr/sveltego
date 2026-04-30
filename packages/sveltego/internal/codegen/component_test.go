package codegen

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/parser"
)

// TestGenerate_NestedComponent_StaticAttr covers #59 acceptance:
// `<button.Comp label="OK"/>` lowers to a direct
// `button.Comp{}.Render(w, ctx, button.CompProps{Label: "OK"}, _slots)`
// call.
func TestGenerate_NestedComponent_StaticAttr(t *testing.T) {
	src := []byte(`<button.Comp label="OK" />` + "\n")
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
		"(button.Comp{}).Render(w, ctx, button.CompProps{Label: `OK`}, button.CompSlots{})",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_NestedComponent_DynamicAttr threads a Go expression
// through a component prop attribute.
func TestGenerate_NestedComponent_DynamicAttr(t *testing.T) {
	src := []byte(`<button.Comp disabled={data.Locked} />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "Disabled: data.Locked") {
		t.Errorf("expected dynamic prop forward, got:\n%s", s)
	}
}

// TestGenerate_NestedComponent_KebabAttr ensures hyphenated attribute
// names lower to PascalCase Go identifiers (`aria-label` -> `AriaLabel`).
func TestGenerate_NestedComponent_KebabAttr(t *testing.T) {
	src := []byte(`<button.Comp aria-label="x" data-id="y" />` + "\n")
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
		"AriaLabel: `x`",
		"DataId: `y`",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_NestedComponent_BooleanAttr maps a name-only attribute
// (`disabled`) to `Disabled: true`.
func TestGenerate_NestedComponent_BooleanAttr(t *testing.T) {
	src := []byte(`<button.Comp disabled />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "Disabled: true") {
		t.Errorf("expected boolean-attr lowering, got:\n%s", s)
	}
}

// TestGenerate_NestedComponent_WithDefaultSlot keeps the slot
// machinery wired when child content is present.
func TestGenerate_NestedComponent_WithDefaultSlot(t *testing.T) {
	src := []byte(`<button.Comp label="OK"><span>icon</span></button.Comp>` + "\n")
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
		"_slots := button.CompSlots{}",
		"_slots.Default = func(w *render.Writer) error",
		"`<span>`",
		"`icon`",
		"(button.Comp{}).Render(w, ctx, button.CompProps{Label: `OK`}, _slots)",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// TestGenerate_NestedComponent_InvalidExpr rejects an unparseable Go
// expression in a dynamic attribute, surfacing the existing diagnostic
// path.
func TestGenerate_NestedComponent_InvalidExpr(t *testing.T) {
	src := []byte(`<button.Comp value={1 +} />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if _, err := Generate(frag, Options{PackageName: "page"}); err == nil {
		t.Fatal("expected codegen error, got nil")
	}
}

// TestGenerate_NestedComponent_NoAttrs covers the trivial `<pkg.Comp />`
// shape so callers without props still get a valid Render call.
func TestGenerate_NestedComponent_NoAttrs(t *testing.T) {
	src := []byte(`<button.Comp />` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "(button.Comp{}).Render(w, ctx, button.CompProps{}, button.CompSlots{})") {
		t.Errorf("expected empty-prop direct call, got:\n%s", s)
	}
}

// TestGenerate_NestedComponent_ImportFlow confirms the user's
// <script> import threads into the generated file's import block so
// the dot-namespaced call `button.Comp` resolves at Go compile time.
func TestGenerate_NestedComponent_ImportFlow(t *testing.T) {
	src := []byte(`<script lang="go">
import button "myapp/components/button"
</script>
<button.Comp label="OK" />
`)
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `button "myapp/components/button"`) {
		t.Errorf("expected user import threaded, got:\n%s", s)
	}
	if !strings.Contains(s, "button.CompProps{Label: `OK`}") {
		t.Errorf("expected props mapping, got:\n%s", s)
	}
}

// TestPascalIdent guards the attribute-name to Go-field conversion.
func TestPascalIdent(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"label", "Label"},
		{"aria-label", "AriaLabel"},
		{"data_id", "DataId"},
		{"someName", "SomeName"},
		{"a", "A"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := pascalIdent(tc.in); got != tc.want {
			t.Errorf("pascalIdent(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
