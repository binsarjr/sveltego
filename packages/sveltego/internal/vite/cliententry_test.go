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

// TestGenerateClientEntry_CSRFAutoInject covers #510 (SPA path) and
// #541 (SPA-nav re-fire). The per-route entry must walk every
// <form method="post"> the client mounts and splice a hidden
// _csrf_token input populated from window.__sveltego__.csrfToken (the
// router rewrites the global on every nav, so reading from the global
// keeps the value fresh after navigation). The splicer is registered
// via afterNavigate so it re-runs on each SPA navigation; without it
// the second-and-later page rendered via the client router has no
// hidden input and the user's first POST returns 403.
func TestGenerateClientEntry_CSRFAutoInject(t *testing.T) {
	t.Parallel()
	out := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath: "../../../src/routes/_page.svelte",
		RelRouterPath: "../../__router/router",
	})
	for _, want := range []string{
		"(window as any).__sveltego__?.csrfToken",
		"target.querySelectorAll('form')",
		"_csrf_token",
		"input.type = 'hidden';",
		"f.insertBefore(input, f.firstChild);",
		"window as any).__sveltego_csrf__",
		"afterNavigate(__sveltegoInjectCSRF)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("client entry missing %q for CSRF auto-inject:\n%s", want, out)
		}
	}
}
