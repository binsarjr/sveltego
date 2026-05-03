package csrfinject

import (
	"strings"
	"testing"
)

// TestRewrite_PostFormGetsHiddenInput covers the dominant case the
// runtime sidecar fallback produces: a POST form rendered by Svelte's
// server compiler must gain a hidden `_csrf_token` input bound to the
// per-request token immediately after the open tag.
func TestRewrite_PostFormGetsHiddenInput(t *testing.T) {
	t.Parallel()
	const token = "abc.def-XYZ_123"
	in := `<form method="post" action="?/login"><input name="email"/></form>`
	out := Rewrite(in, token)
	want := `<form method="post" action="?/login"><input type="hidden" name="_csrf_token" value="abc.def-XYZ_123"><input name="email"/></form>`
	if out != want {
		t.Fatalf("unexpected output:\nwant %s\n got %s", want, out)
	}
}

// TestRewrite_GetFormUnchanged: GET forms stay bookmarkable and must
// not receive the hidden input.
func TestRewrite_GetFormUnchanged(t *testing.T) {
	t.Parallel()
	in := `<form method="get"><input name="q"/></form>`
	if out := Rewrite(in, "tok"); out != in {
		t.Fatalf("GET form should be unchanged; got %q", out)
	}
}

// TestRewrite_NoMethodUnchanged: HTML defaults to GET when no method
// attribute is present; no injection happens.
func TestRewrite_NoMethodUnchanged(t *testing.T) {
	t.Parallel()
	in := `<form><input name="q"/></form>`
	if out := Rewrite(in, "tok"); out != in {
		t.Fatalf("method-less form should be unchanged; got %q", out)
	}
}

// TestRewrite_MethodCaseInsensitive verifies POST detection ignores
// case (matches HTML semantics).
func TestRewrite_MethodCaseInsensitive(t *testing.T) {
	t.Parallel()
	for _, m := range []string{"POST", "post", "Post", "pOsT"} {
		m := m
		t.Run(m, func(t *testing.T) {
			t.Parallel()
			in := `<form method="` + m + `"></form>`
			out := Rewrite(in, "tok")
			if !strings.Contains(out, `name="_csrf_token"`) {
				t.Fatalf("method=%s must inject hidden input; got %q", m, out)
			}
		})
	}
}

// TestRewrite_NocsrfStrippedAndSkipped: the per-form opt-out attribute
// is removed from the rendered HTML and the hidden input is not added.
func TestRewrite_NocsrfStrippedAndSkipped(t *testing.T) {
	t.Parallel()
	in := `<form method="post" nocsrf action="/x"><input name="a"/></form>`
	out := Rewrite(in, "tok")
	if strings.Contains(out, "_csrf_token") {
		t.Fatalf("nocsrf form should not gain CSRF input:\n%s", out)
	}
	if strings.Contains(out, "nocsrf") {
		t.Fatalf("nocsrf attribute must be stripped from rendered HTML:\n%s", out)
	}
	want := `<form method="post" action="/x"><input name="a"/></form>`
	if out != want {
		t.Fatalf("unexpected output:\nwant %s\n got %s", want, out)
	}
}

// TestRewrite_AlreadyInjectedIdempotent: a form that already starts
// with the hidden input is left alone — covers double-runs (build-time
// pass already injected; runtime sidecar would otherwise inject again).
func TestRewrite_AlreadyInjectedIdempotent(t *testing.T) {
	t.Parallel()
	in := `<form method="post"><input type="hidden" name="_csrf_token" value="prev"><input name="a"/></form>`
	out := Rewrite(in, "tok")
	if out != in {
		t.Fatalf("already-injected form should be unchanged; got %s", out)
	}
	hits := strings.Count(out, "_csrf_token")
	if hits != 1 {
		t.Fatalf("expected exactly 1 _csrf_token, got %d:\n%s", hits, out)
	}
}

// TestRewrite_UserAlreadyDeclaredHidden: a user who manually placed
// the hidden input as the first child of a POST form is respected; we
// do not double-inject. This is the explicit acceptance criterion.
func TestRewrite_UserAlreadyDeclaredHidden(t *testing.T) {
	t.Parallel()
	in := `<form method="post" action="/x"><input type='hidden' name='_csrf_token' value='manual'><button>Go</button></form>`
	out := Rewrite(in, "framework-token")
	if strings.Contains(out, "framework-token") {
		t.Fatalf("framework token should not have been spliced in; got:\n%s", out)
	}
	hits := strings.Count(out, "_csrf_token")
	if hits != 1 {
		t.Fatalf("expected exactly 1 _csrf_token, got %d:\n%s", hits, out)
	}
}

// TestRewrite_QuotedGreaterThanInAttr: a `>` inside a quoted attribute
// value must not be mistaken for the open-tag close.
func TestRewrite_QuotedGreaterThanInAttr(t *testing.T) {
	t.Parallel()
	in := `<form method="post" action="?/x>y"><input name="a"/></form>`
	out := Rewrite(in, "tok")
	if !strings.Contains(out, `<form method="post" action="?/x>y"><input type="hidden" name="_csrf_token"`) {
		t.Fatalf("quoted > inside attribute confused the scanner:\n%s", out)
	}
}

// TestRewrite_MultipleForms: every POST form in the document gets one
// hidden input; GET forms in the same document remain untouched.
func TestRewrite_MultipleForms(t *testing.T) {
	t.Parallel()
	in := `<form method="get"><input name="q"/></form>` +
		`<div><form method="post" action="/a"></form></div>` +
		`<form method="POST" action="/b"></form>` +
		`<form><input/></form>`
	out := Rewrite(in, "tok")
	hits := strings.Count(out, `name="_csrf_token"`)
	if hits != 2 {
		t.Fatalf("expected 2 hidden inputs (one per POST form); got %d:\n%s", hits, out)
	}
}

// TestRewrite_NoFormsNoOp: an HTML chunk that mentions neither <form
// nor <input cycles through Rewrite without allocating a new string.
func TestRewrite_NoFormsNoOp(t *testing.T) {
	t.Parallel()
	in := `<div class="hero"><p>nothing to see here</p></div>`
	out := Rewrite(in, "tok")
	if out != in {
		t.Fatalf("non-form HTML should be returned unchanged; got %q", out)
	}
}

// TestRewrite_SelfClosingFormSkipped: `<form .../>` is malformed for
// interactive forms; treat as non-form so we never inject into a
// childless self-close.
func TestRewrite_SelfClosingFormSkipped(t *testing.T) {
	t.Parallel()
	in := `<form method="post"/><p>after</p>`
	if out := Rewrite(in, "tok"); strings.Contains(out, "_csrf_token") {
		t.Fatalf("self-closing form should not get CSRF input:\n%s", out)
	}
}

// TestRewrite_TokenHTMLEscaped: a token containing characters that
// would otherwise close the attribute is HTML-escaped before insertion.
func TestRewrite_TokenHTMLEscaped(t *testing.T) {
	t.Parallel()
	in := `<form method="post"></form>`
	out := Rewrite(in, `a"b<c>d&e'f`)
	if !strings.Contains(out, `value="a&quot;b&lt;c&gt;d&amp;e&#39;f">`) {
		t.Fatalf("token not properly escaped:\n%s", out)
	}
}

// TestRewrite_FormAttributesPreserved: attribute order, quoting, and
// case are preserved on the open tag (only nocsrf is ever stripped).
func TestRewrite_FormAttributesPreserved(t *testing.T) {
	t.Parallel()
	in := `<form METHOD='POST' Action="?/x" data-foo='bar baz' class="hero"><input/></form>`
	out := Rewrite(in, "tok")
	if !strings.Contains(out, `<form METHOD='POST' Action="?/x" data-foo='bar baz' class="hero">`) {
		t.Fatalf("form attributes mutated:\n%s", out)
	}
}

// TestRewrite_NestedFormInsideListBlock simulates a form rendered
// inside a Svelte {#each} or {#if} block: the runtime view is a flat
// HTML string with the form somewhere in the middle of other markup.
// The pass must still inject (string-level scanner does not care about
// Svelte block structure — that's the whole point of injecting at the
// rendered-HTML layer).
func TestRewrite_NestedFormInsideListBlock(t *testing.T) {
	t.Parallel()
	in := `<ul>` +
		`<li>one<form method="post" action="/a"></form></li>` +
		`<li>two<form method="post" action="/b"></form></li>` +
		`</ul>`
	out := Rewrite(in, "tok")
	hits := strings.Count(out, `name="_csrf_token"`)
	if hits != 2 {
		t.Fatalf("expected 2 injected inputs for forms inside list; got %d:\n%s", hits, out)
	}
}

// TestRewrite_EmptyInput is a trivial guard against an obvious panic
// path; an empty string is a no-op.
func TestRewrite_EmptyInput(t *testing.T) {
	t.Parallel()
	if out := Rewrite("", "tok"); out != "" {
		t.Fatalf("empty input should be empty output, got %q", out)
	}
}

// TestRewrite_TokenWithoutSpecialCharsNotReencoded covers the fast-
// path branch of htmlAttrEscape: a token with no `<&"'>` characters
// is returned verbatim.
func TestRewrite_TokenWithoutSpecialCharsNotReencoded(t *testing.T) {
	t.Parallel()
	in := `<form method="post"></form>`
	out := Rewrite(in, "abc-DEF_123.~")
	if !strings.Contains(out, `value="abc-DEF_123.~">`) {
		t.Fatalf("safe token should not be re-encoded:\n%s", out)
	}
}
