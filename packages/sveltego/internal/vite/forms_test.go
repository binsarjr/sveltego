package vite

import (
	"strings"
	"testing"
)

// TestGenerateFormsModule_ExportsEnhance asserts the public-facing
// $app/forms module surface stays stable: `enhance` is exported as a
// Svelte action, the envelope shape mirrors SvelteKit's ActionResult,
// and the request carries the X-Sveltego-Action header so the server
// returns JSON instead of HTML.
func TestGenerateFormsModule_ExportsEnhance(t *testing.T) {
	t.Parallel()
	out := GenerateFormsModule()
	for _, want := range []string{
		"export function enhance(form: HTMLFormElement, callback?: SubmitHandler)",
		"export interface EnhanceEnvelope",
		"export type SubmitHandler",
		"const ACTION_HEADER = 'X-Sveltego-Action'",
		"credentials: 'same-origin'",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("forms module missing %q:\n%s", want, out)
		}
	}
}

// TestGenerateFormsModule_Deterministic guards against per-call
// non-determinism — the build must be reproducible.
func TestGenerateFormsModule_Deterministic(t *testing.T) {
	t.Parallel()
	a := GenerateFormsModule()
	b := GenerateFormsModule()
	if a != b {
		t.Error("GenerateFormsModule is non-deterministic")
	}
}

// TestGenerateEnhanceRuntime_ShimMatchesForms documents the back-compat
// alias: GenerateEnhanceRuntime returns the same bytes as
// GenerateFormsModule so existing callers (and downstream snapshots)
// don't drift while they migrate to the new name.
func TestGenerateEnhanceRuntime_ShimMatchesForms(t *testing.T) {
	t.Parallel()
	if GenerateEnhanceRuntime() != GenerateFormsModule() {
		t.Error("GenerateEnhanceRuntime should return GenerateFormsModule output")
	}
}
