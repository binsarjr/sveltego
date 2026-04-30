package codegen

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/parser"
)

// TestGenerate_HasActions_AddsFormField pins the codegen behavior
// linking page.server.go's `var Actions = ...` to PageData.Form.
func TestGenerate_HasActions_AddsFormField(t *testing.T) {
	t.Parallel()
	src := []byte("<h1>{Data.Name}</h1>")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName: "page",
		HasActions:  true,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "Form any") {
		t.Errorf("expected `Form any` in PageData when HasActions=true:\n%s", got)
	}
}

func TestGenerate_NoActions_NoFormField(t *testing.T) {
	t.Parallel()
	src := []byte("<h1>hi</h1>")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{PackageName: "page"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if strings.Contains(string(out), "Form any") {
		t.Errorf("unexpected Form field when HasActions=false:\n%s", out)
	}
}
