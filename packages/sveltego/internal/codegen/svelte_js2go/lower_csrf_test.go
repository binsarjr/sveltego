package sveltejs2go

import (
	"strings"
	"testing"
)

// Each case here drives Transpile end-to-end with CSRFAutoInject set so
// the assertions cover the splice + emitter integration, not just the
// in-memory AST mutation.

// TestCSRFInject_PostFormGetsHiddenInput verifies the dominant case:
// `<form method="post" action="?/login">` in a static quasi gains a
// hidden _csrf_token input bound to pageState.CSRFToken immediately
// after the open tag.
func TestCSRFInject_PostFormGetsHiddenInput(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		propsDestructure("data"),
		pushString(`<form method="post" action="?/login"><input name="email"/></form>`),
	))
	got, err := TranspileNode(prog, "/login", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	out := string(got)
	want := []string{
		`<form method=\"post\" action=\"?/login\">`,
		`<input type=\"hidden\" name=\"_csrf_token\" value=\"`,
		`pageState.CSRFToken`,
		`\">`,
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("expected %q in output; got:\n%s", w, out)
		}
	}
	// Order matters: the CSRF input must follow the open tag and
	// precede the user-authored <input name="email">.
	csrfIdx := strings.Index(out, `\"_csrf_token\"`)
	emailIdx := strings.Index(out, `name=\"email\"`)
	if csrfIdx < 0 || emailIdx < 0 || csrfIdx > emailIdx {
		t.Errorf("CSRF input must precede user-authored input; csrf=%d email=%d\n%s",
			csrfIdx, emailIdx, out)
	}
}

// TestCSRFInject_GetFormSkipped: GET forms are bookmarkable and skip
// CSRF. The generated output must not reference _csrf_token.
func TestCSRFInject_GetFormSkipped(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="get"><input name="q"/></form>`),
	))
	got, err := TranspileNode(prog, "/search", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if strings.Contains(string(got), "_csrf_token") {
		t.Errorf("GET form should not get CSRF input:\n%s", got)
	}
}

// TestCSRFInject_NoMethodSkipped: a form with no method attribute
// defaults to GET per HTML and gets no CSRF input.
func TestCSRFInject_NoMethodSkipped(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form><input name="q"/></form>`),
	))
	got, err := TranspileNode(prog, "/search", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if strings.Contains(string(got), "_csrf_token") {
		t.Errorf("form without method should not get CSRF input:\n%s", got)
	}
}

// TestCSRFInject_MethodCaseInsensitive ensures method matching is
// case-insensitive (matches HTML semantics).
func TestCSRFInject_MethodCaseInsensitive(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"POST", "post", "Post", "pOsT"} {
		method := method
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			prog := buildProgram(buildBlock(
				pushString(`<form method="` + method + `"></form>`),
			))
			got, err := TranspileNode(prog, "/p", Options{
				PackageName:        "gen",
				EmitPageStateParam: true,
				CSRFAutoInject:     true,
			})
			if err != nil {
				t.Fatalf("TranspileNode: %v", err)
			}
			if !strings.Contains(string(got), "_csrf_token") {
				t.Errorf("method=%s must inject CSRF input:\n%s", method, got)
			}
		})
	}
}

// TestCSRFInject_DisabledByOption verifies the option gate: with
// CSRFAutoInject:false (or default zero), no injection happens even
// when a POST form is present.
func TestCSRFInject_DisabledByOption(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="post"></form>`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		// CSRFAutoInject deliberately omitted.
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if strings.Contains(string(got), "_csrf_token") {
		t.Errorf("CSRFAutoInject=false should suppress injection:\n%s", got)
	}
}

// TestCSRFInject_NocsrfOptOut verifies the per-form opt-out: a
// `nocsrf` attribute on a POST form skips injection AND strips the
// marker attribute from the rendered HTML.
func TestCSRFInject_NocsrfOptOut(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="post" nocsrf><input name="email"/></form>`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	out := string(got)
	if strings.Contains(out, "_csrf_token") {
		t.Errorf("nocsrf form should skip CSRF injection:\n%s", out)
	}
	if strings.Contains(out, "nocsrf") {
		t.Errorf("nocsrf attribute should be stripped from rendered HTML:\n%s", out)
	}
}

// TestCSRFInject_TemplateLiteralWithInterpolation: the form lives in a
// quasi adjacent to an interpolation. The splice must keep the
// interpolation in place and inject the hidden input only into the
// quasi that holds the form open tag.
func TestCSRFInject_TemplateLiteralWithInterpolation(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{`<form method="post" action="?/save"><input name="title" value="`, `"/></form>`},
			[]*Node{escapeOf(memExpr(ident("data"), ident("title")))},
		),
	))
	got, err := TranspileNode(prog, "/save", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, "_csrf_token") {
		t.Errorf("expected CSRF input alongside interpolated form:\n%s", out)
	}
	if !strings.Contains(out, "pageState.CSRFToken") {
		t.Errorf("expected pageState.CSRFToken interpolation:\n%s", out)
	}
	// Original interpolation must survive.
	if !strings.Contains(out, "server.EscapeHTML(data.title)") {
		t.Errorf("user interpolation must survive splice:\n%s", out)
	}
}

// TestCSRFInject_MultiplePostForms covers the case where a single
// quasi holds two POST forms; both must get hidden inputs.
func TestCSRFInject_MultiplePostForms(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="post" action="?/a"></form><form method="post" action="?/b"></form>`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	out := string(got)
	hits := strings.Count(out, "_csrf_token")
	if hits != 2 {
		t.Errorf("expected 2 CSRF inputs (one per POST form); got %d:\n%s", hits, out)
	}
}

// TestCSRFInject_NonFormElementSkipped: a fake "<form-like>" element
// that isn't actually <form> must not be rewritten.
func TestCSRFInject_NonFormElementSkipped(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<formal-attire method="post"></formal-attire>`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if strings.Contains(string(got), "_csrf_token") {
		t.Errorf("non-form element should not be rewritten:\n%s", got)
	}
}

// TestCSRFInject_SelfClosingFormSkipped: a self-closing form (rare but
// HTML-legal) should not be rewritten — there's no body to inject into.
func TestCSRFInject_SelfClosingFormSkipped(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="post" action="?/x" />`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if strings.Contains(string(got), "_csrf_token") {
		t.Errorf("self-closing form should not be rewritten:\n%s", got)
	}
}

// TestCSRFInject_QuotedGtInAttribute: a `>` byte inside a quoted
// attribute value must not be mistaken for the open-tag close.
func TestCSRFInject_QuotedGtInAttribute(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="post" action="?/x>y"></form>`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, "_csrf_token") {
		t.Errorf("quoted > must not abort scan:\n%s", out)
	}
	// The action attribute's value must survive intact.
	if !strings.Contains(out, "?/x>y") {
		t.Errorf("attribute value must be preserved:\n%s", out)
	}
}

// TestCSRFInject_Idempotent: running the pre-pass twice on the same
// AST yields the same output as one run — second pass must detect the
// already-injected marker and skip.
func TestCSRFInject_Idempotent(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="post" action="?/x"></form>`),
	))
	// Apply twice.
	injectCSRF(prog)
	injectCSRF(prog)
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		// Note: CSRFAutoInject NOT set — the AST was already mutated
		// in-place above, so we drive the rest of the pipeline without
		// triggering a third injection.
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	hits := strings.Count(string(got), "_csrf_token")
	if hits != 1 {
		t.Errorf("idempotent injection should yield 1 CSRF input; got %d:\n%s", hits, got)
	}
}

// TestCSRFInject_PreservesUnrelatedQuasis: a template literal with
// multiple quasis where only the last one holds a form — quasis before
// the form must pass through unchanged.
func TestCSRFInject_PreservesUnrelatedQuasis(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		propsDestructure("data"),
		pushTemplate(
			[]string{
				`<h1>Welcome `,
				`</h1><form method="post" action="?/save"></form>`,
			},
			[]*Node{escapeOf(memExpr(ident("data"), ident("name")))},
		),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	out := string(got)
	if !strings.Contains(out, "<h1>Welcome ") {
		t.Errorf("leading quasi must be preserved:\n%s", out)
	}
	if !strings.Contains(out, "server.EscapeHTML(data.name)") {
		t.Errorf("interpolation must be preserved:\n%s", out)
	}
	if !strings.Contains(out, "_csrf_token") {
		t.Errorf("form quasi must get CSRF input:\n%s", out)
	}
}

// TestCSRFInject_UpperCaseFormTag: HTML tag names are case-insensitive;
// `<FORM METHOD="POST">` must inject too.
func TestCSRFInject_UpperCaseFormTag(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<FORM METHOD="POST"></FORM>`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if !strings.Contains(string(got), "_csrf_token") {
		t.Errorf("uppercase FORM with uppercase METHOD must inject:\n%s", got)
	}
}

// TestCSRFInject_PutMethodSkipped covers a non-POST verb that some
// frameworks accept (PUT). Only POST gets the hidden input — PUT,
// DELETE, etc. require a different transport (XHR / use:enhance) and
// are out of scope for the static-form CSRF helper.
func TestCSRFInject_PutMethodSkipped(t *testing.T) {
	t.Parallel()
	prog := buildProgram(buildBlock(
		pushString(`<form method="put" action="?/x"></form>`),
	))
	got, err := TranspileNode(prog, "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("TranspileNode: %v", err)
	}
	if strings.Contains(string(got), "_csrf_token") {
		t.Errorf("PUT form should not be rewritten:\n%s", got)
	}
}

// TestCSRFInject_DeterministicAcrossRuns: byte-identical output across
// repeated transpiles. The pre-pass mutates the AST in place, so the
// second run starts from a different state — re-Decode the program
// per run to drive both from a fresh tree.
func TestCSRFInject_DeterministicAcrossRuns(t *testing.T) {
	t.Parallel()
	build := func() *Node {
		return buildProgram(buildBlock(
			propsDestructure("data"),
			pushString(`<form method="post" action="?/x"><input name="email"/></form>`),
		))
	}
	first, err := TranspileNode(build(), "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("first TranspileNode: %v", err)
	}
	second, err := TranspileNode(build(), "/p", Options{
		PackageName:        "gen",
		EmitPageStateParam: true,
		CSRFAutoInject:     true,
	})
	if err != nil {
		t.Fatalf("second TranspileNode: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("non-deterministic output across runs\nfirst:\n%s\n\nsecond:\n%s", first, second)
	}
}
