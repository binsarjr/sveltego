package codegen

import (
	"os"
	"path/filepath"
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

// TestGenerate_HasActions_UserDeclaredFormDeduped verifies that when a
// page.server.go Load() return already contains `Form any` AND HasActions is
// true, codegen emits exactly one Form field (no compile-error duplicate).
// This is the fix for #143.
func TestGenerate_HasActions_UserDeclaredFormDeduped(t *testing.T) {
	t.Parallel()

	// Write a transient server file that declares Form any in its Load return.
	serverSrc := `package fixture

func Load() (struct {
	Title string
	Form  any
}, error) {
	return struct {
		Title string
		Form  any
	}{Title: "hello"}, nil
}
`
	dir := t.TempDir()
	serverPath := filepath.Join(dir, "page.server.go")
	if err := os.WriteFile(serverPath, []byte(serverSrc), 0o644); err != nil {
		t.Fatalf("write server file: %v", err)
	}

	src := []byte("<h1>{data.Title}</h1>")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	out, err := Generate(frag, Options{
		PackageName:    "page",
		HasActions:     true,
		ServerFilePath: serverPath,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	got := string(out)

	// Must contain exactly one Form field. go/format may align struct columns
	// with extra spaces (e.g. `Form  any` when next to `Title string`), so
	// search for the field name followed by at least one space, not the exact
	// "Form any" two-token string.
	formCount := strings.Count(got, "\tForm ")
	if formCount != 1 {
		t.Errorf("expected exactly 1 Form field in struct body, got %d:\n%s", formCount, got)
	}

	// Title field from Load must still be present.
	if !strings.Contains(got, "Title string") {
		t.Errorf("expected `Title string` in PageData:\n%s", got)
	}
}
