package csrffallback

import (
	"strings"
	"testing"
)

func TestLower_PostFormGetsHiddenInput(t *testing.T) {
	src := []byte(`<script lang="ts">
  let { data, form } = $props();
</script>

<form method="post" action="?/login">
  <input name="username">
  <button>Submit</button>
</form>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if !got.Mutated {
		t.Fatalf("expected Mutated=true; output=%q", got.Content)
	}
	want := `<input type="hidden" name="_csrf_token" value={` + CSRFValueExpr + `}>`
	if !strings.Contains(string(got.Content), want) {
		t.Fatalf("hidden input not spliced; got=%q", got.Content)
	}
	// Form open tag itself preserved.
	if !strings.Contains(string(got.Content), `<form method="post" action="?/login">`) {
		t.Fatalf("original form open tag missing; got=%q", got.Content)
	}
}

func TestLower_GetFormUntouched(t *testing.T) {
	src := []byte(`<form method="get" action="/search">
  <input name="q">
</form>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if got.Mutated {
		t.Fatalf("expected Mutated=false on GET form; got=%q", got.Content)
	}
	if string(got.Content) != string(src) {
		t.Fatalf("input mutated; got=%q want=%q", got.Content, src)
	}
}

func TestLower_NoCSRFOptOutStripsAttrAndSkipsSplice(t *testing.T) {
	src := []byte(`<form method="post" nocsrf action="/upload">
  <input type="file" name="file">
</form>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if !got.Mutated {
		t.Fatalf("nocsrf opt-out should mutate (strip attribute); got=%q", got.Content)
	}
	if strings.Contains(string(got.Content), "nocsrf") {
		t.Fatalf("nocsrf attribute not stripped; got=%q", got.Content)
	}
	if strings.Contains(string(got.Content), `name="_csrf_token"`) {
		t.Fatalf("hidden input spliced into nocsrf form; got=%q", got.Content)
	}
}

func TestLower_FormStringsInScriptIgnored(t *testing.T) {
	src := []byte(`<script>
  // String literal that looks like a form open tag must NOT be rewritten.
  const html = '<form method="post">';
  const tag = "<form method=\"post\">";
</script>

<p>nothing here</p>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if got.Mutated {
		t.Fatalf("script content rewrote; got=%q", got.Content)
	}
}

func TestLower_FormStringsInStyleIgnored(t *testing.T) {
	src := []byte(`<style>
  /* form[method="post"] selector should not trigger splice */
  form[method="post"] { display: block; }
</style>

<p>nothing here</p>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if got.Mutated {
		t.Fatalf("style content rewrote; got=%q", got.Content)
	}
}

func TestLower_MultiplePostFormsAllSpliced(t *testing.T) {
	src := []byte(`<form method="post" action="?/a"><input name="x"></form>
<form method="post" action="?/b"><input name="y"></form>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	count := strings.Count(string(got.Content), `name="_csrf_token"`)
	if count != 2 {
		t.Fatalf("want 2 csrf inputs spliced; got %d in %q", count, got.Content)
	}
}

func TestLower_IdempotentWhenInputAlreadyPresent(t *testing.T) {
	src := []byte(`<form method="post" action="?/x">
  <input type="hidden" name="_csrf_token" value="static">
  <input name="username">
</form>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if got.Mutated {
		t.Fatalf("idempotent splice should leave Mutated=false; got=%q", got.Content)
	}
	if c := strings.Count(string(got.Content), `name="_csrf_token"`); c != 1 {
		t.Fatalf("idempotency violated; got %d hidden inputs in %q", c, got.Content)
	}
}

func TestLower_DynamicMethodIsSkipped(t *testing.T) {
	src := []byte(`<form method={kind} action="?/x">
  <input name="x">
</form>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if got.Mutated {
		t.Fatalf("dynamic method should not match POST; got=%q", got.Content)
	}
}

func TestLower_MustacheActionTolerated(t *testing.T) {
	src := []byte(`<form method="post" action={'?/' + slug}>
  <input name="x">
</form>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if !got.Mutated {
		t.Fatalf("expected splice for POST form with mustache action; got=%q", got.Content)
	}
}

func TestLower_RelativeImportsRewrittenToAbsolute(t *testing.T) {
	src := []byte(`<script>
  import Foo from './Foo.svelte';
  import { bar } from "../shared/util";
  import baz from "$lib/baz";
</script>

<Foo />
`)
	got, err := Lower(LowerOptions{Source: "/abs/routes/login/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	out := string(got.Content)
	if !strings.Contains(out, `from '/abs/routes/login/Foo.svelte'`) {
		t.Fatalf("./Foo.svelte not rewritten to absolute; got=%q", out)
	}
	if !strings.Contains(out, `from "/abs/routes/shared/util"`) {
		t.Fatalf("../shared/util not rewritten to absolute; got=%q", out)
	}
	if !strings.Contains(out, `from "$lib/baz"`) {
		t.Fatalf("$lib alias incorrectly rewritten; got=%q", out)
	}
}

func TestLower_EmptyInputReturnsCleanResult(t *testing.T) {
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: nil})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	if got.Mutated {
		t.Fatalf("empty input mutated; got=%q", got.Content)
	}
	if len(got.Content) != 0 {
		t.Fatalf("empty input changed; got=%q", got.Content)
	}
}

func TestLower_PreservesSurroundingContent(t *testing.T) {
	src := []byte(`<h1>before</h1>
<form method="post" action="?/login">
  <input name="x">
</form>
<p>after</p>
`)
	got, err := Lower(LowerOptions{Source: "/abs/_page.svelte", SourceContent: src})
	if err != nil {
		t.Fatalf("Lower returned err: %v", err)
	}
	out := string(got.Content)
	if !strings.HasPrefix(out, "<h1>before</h1>\n") {
		t.Fatalf("prefix lost; got=%q", out)
	}
	if !strings.HasSuffix(out, "<p>after</p>\n") {
		t.Fatalf("suffix lost; got=%q", out)
	}
}
