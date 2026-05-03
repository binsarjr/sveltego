package vite

import (
	"strings"
	"testing"
)

func TestGenerateClientEntry_ImportsEnhance(t *testing.T) {
	t.Parallel()
	out := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath: "../../../src/routes/_page.svelte",
		RelRouterPath: "../../__router/router",
	})
	if !strings.Contains(out, "import { enhance } from './enhance';") {
		t.Errorf("expected enhance import in entry.ts:\n%s", out)
	}
	if !strings.Contains(out, "(window as any).__sveltego__ = { ...payload, enhance }") {
		t.Errorf("expected enhance to be exposed on window.__sveltego__:\n%s", out)
	}
	if !strings.Contains(out, "import Root from \"../../../src/routes/_page.svelte\";") {
		t.Errorf("expected Root (page) import:\n%s", out)
	}
	if !strings.Contains(out, "(window as any).__sveltego_hydrated = true") {
		t.Errorf("expected __sveltego_hydrated marker after mount (#446):\n%s", out)
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

// TestGenerateClientEntry_CSRFAutoInject covers issue #510 on the SPA
// path: the per-route entry must walk every <form method="post"> the
// client mounts and splice a hidden _csrf_token input populated from
// payload.csrfToken. Without this, SPA / Static routes that render
// forms entirely in the browser would POST without the field and
// trigger the framework's 403 CSRF rejection.
func TestGenerateClientEntry_CSRFAutoInject(t *testing.T) {
	t.Parallel()
	out := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath: "../../../src/routes/_page.svelte",
		RelRouterPath: "../../__router/router",
	})
	for _, want := range []string{
		"(payload as any).csrfToken",
		"target.querySelectorAll('form')",
		"_csrf_token",
		"input.type = 'hidden';",
		"f.insertBefore(input, f.firstChild);",
		"window as any).__sveltego_csrf__",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("client entry missing %q for CSRF auto-inject:\n%s", want, out)
		}
	}
}
