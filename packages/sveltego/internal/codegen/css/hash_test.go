package css

import "testing"

// TestHash_UpstreamGolden compares the Go port against values produced by
// running upstream Svelte's `hash` function on Node. Inputs cover ASCII,
// CR-stripping, LF, and non-ASCII (UTF-16 surrogate-aware) cases.
func TestHash_UpstreamGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"", "45h"},
		{"a", "3ksa"},
		{"hello", "1r2ynjb"},
		{"hello\rworld", "1jdft95"},
		{"hello\nworld", "zvkcy5"},
		{"div { color: red; }", "bcpeq7"},
		{"p { color: red; }", "14l9336"},
		{"h1 { color: green; }", "1uh457l"},
		{"/path/to/Component.svelte", "v3zdnq"},
		{"src/routes/+page.svelte", "1uha8ag"},
		{"日本語", "23kt70"},
		{"café", "b4tvgk"},
	}
	for _, tc := range cases {
		got := Hash(tc.in)
		if got != tc.want {
			t.Errorf("Hash(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHash_CRDoesNotAffect(t *testing.T) {
	t.Parallel()
	if Hash("ab") != Hash("a\rb") {
		t.Errorf("CR not stripped: Hash(ab)=%q Hash(a\\rb)=%q", Hash("ab"), Hash("a\rb"))
	}
}

func TestScopeClass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		filename, css, want string
	}{
		{"Component.svelte", "ignored", "svelte-lsmn3l"},
		{"", "div { color: red; }", "svelte-bcpeq7"},
		{"(unknown)", "div { color: red; }", "svelte-bcpeq7"},
	}
	for _, tc := range cases {
		got := ScopeClass(tc.filename, tc.css)
		if got != tc.want {
			t.Errorf("ScopeClass(%q, %q) = %q, want %q", tc.filename, tc.css, got, tc.want)
		}
	}
}
