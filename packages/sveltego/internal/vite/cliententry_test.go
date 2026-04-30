package vite

import (
	"strings"
	"testing"
)

func TestGenerateClientEntry_ImportsEnhance(t *testing.T) {
	t.Parallel()
	out := GenerateClientEntry("../../../src/routes/+page.svelte", "../../__router/router")
	if !strings.Contains(out, "import { enhance } from './enhance';") {
		t.Errorf("expected enhance import in entry.ts:\n%s", out)
	}
	if !strings.Contains(out, "(window as any).__sveltego__ = { ...payload, enhance }") {
		t.Errorf("expected enhance to be exposed on window.__sveltego__:\n%s", out)
	}
	if !strings.Contains(out, "import Page from \"../../../src/routes/+page.svelte\";") {
		t.Errorf("expected Page import:\n%s", out)
	}
}

func TestGenerateEnhanceRuntime_ShapeMatchesEnvelope(t *testing.T) {
	t.Parallel()
	out := GenerateEnhanceRuntime()
	for _, want := range []string{
		"export interface EnhanceEnvelope",
		"X-Sveltego-Action",
		"export function enhance(form: HTMLFormElement",
		"e.preventDefault();",
		"FormData(form)",
		"return {",
		"destroy()",
		"sveltego:action",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("enhance runtime missing %q", want)
		}
	}
}

func TestGenerateEnhanceRuntime_HandlesAllEnvelopeTypes(t *testing.T) {
	t.Parallel()
	out := GenerateEnhanceRuntime()
	for _, variant := range []string{"'success'", "'failure'", "'redirect'", "'error'"} {
		if !strings.Contains(out, variant) {
			t.Errorf("enhance runtime missing envelope variant %s", variant)
		}
	}
}

func TestGenerateEnhanceRuntime_Deterministic(t *testing.T) {
	t.Parallel()
	a := GenerateEnhanceRuntime()
	b := GenerateEnhanceRuntime()
	if a != b {
		t.Error("GenerateEnhanceRuntime is non-deterministic")
	}
}
