package codegen

import (
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/internal/parser"
)

// TestCSRFInjection_PostFormGetsHiddenInput verifies the codegen-time
// auto-injection of the _csrf_token hidden input into POST forms (#123).
func TestCSRFInjection_PostFormGetsHiddenInput(t *testing.T) {
	t.Parallel()
	src := []byte(`<form method="POST"><input name="email" /></form>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := GenerateComponent(frag, ComponentOptions{
		PackageName:   "page",
		ComponentName: "Page",
	})
	if err != nil {
		t.Fatalf("GenerateComponent: %v", err)
	}
	out := string(res.Source)
	if !strings.Contains(out, `<input type="hidden" name="_csrf_token" value="`) {
		t.Errorf("expected hidden CSRF input in output:\n%s", out)
	}
	if !strings.Contains(out, "ctx.CSRFToken()") {
		t.Errorf("expected ctx.CSRFToken() reference in output:\n%s", out)
	}
}

// TestCSRFInjection_GetFormSkipped confirms only POST forms get the
// hidden input — GET forms are bookmarkable and don't need CSRF.
func TestCSRFInjection_GetFormSkipped(t *testing.T) {
	t.Parallel()
	src := []byte(`<form method="GET"><input name="q" /></form>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := GenerateComponent(frag, ComponentOptions{
		PackageName:   "page",
		ComponentName: "Page",
	})
	if err != nil {
		t.Fatalf("GenerateComponent: %v", err)
	}
	out := string(res.Source)
	if strings.Contains(out, "_csrf_token") {
		t.Errorf("did not expect CSRF input on GET form:\n%s", out)
	}
}

// TestCSRFInjection_NoCSRFAttributeOptsOut covers the per-form opt-out:
// any <form nocsrf> drops both the hidden input and the marker attribute
// from the rendered HTML.
func TestCSRFInjection_NoCSRFAttributeOptsOut(t *testing.T) {
	t.Parallel()
	src := []byte(`<form method="POST" nocsrf><input name="email" /></form>` + "\n")
	frag, errs := parser.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	res, err := GenerateComponent(frag, ComponentOptions{
		PackageName:   "page",
		ComponentName: "Page",
	})
	if err != nil {
		t.Fatalf("GenerateComponent: %v", err)
	}
	out := string(res.Source)
	if strings.Contains(out, "_csrf_token") {
		t.Errorf("nocsrf form should skip CSRF injection:\n%s", out)
	}
	if strings.Contains(out, "nocsrf") {
		t.Errorf("nocsrf attribute should be stripped from output:\n%s", out)
	}
}

// TestCSRFInjection_MethodCaseInsensitive ensures method matching is
// case-insensitive (matches HTML semantics).
func TestCSRFInjection_MethodCaseInsensitive(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"POST", "post", "Post", "pOsT"} {
		method := method
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			src := []byte(`<form method="` + method + `"></form>` + "\n")
			frag, errs := parser.Parse(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			res, err := GenerateComponent(frag, ComponentOptions{
				PackageName:   "page",
				ComponentName: "Page",
			})
			if err != nil {
				t.Fatalf("GenerateComponent: %v", err)
			}
			out := string(res.Source)
			if !strings.Contains(out, "_csrf_token") {
				t.Errorf("method=%s should inject CSRF input:\n%s", method, out)
			}
		})
	}
}
