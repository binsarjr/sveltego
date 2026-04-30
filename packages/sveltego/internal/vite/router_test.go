package vite

import (
	"strings"
	"testing"
)

func TestGenerateRouter_emitsImportMap(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{
		Routes: map[string]string{
			"/":          "../../routes/+page.svelte",
			"/blog/[id]": "../../routes/blog/[id]/+page.svelte",
		},
	})

	if !strings.Contains(src, `"/": () => import("../../routes/+page.svelte")`) {
		t.Errorf("missing root route import:\n%s", src)
	}
	if !strings.Contains(src, `"/blog/[id]": () => import("../../routes/blog/[id]/+page.svelte")`) {
		t.Errorf("missing param route import:\n%s", src)
	}
}

func TestGenerateRouter_deterministic(t *testing.T) {
	t.Parallel()

	in := RouterOptions{
		Routes: map[string]string{
			"/z": "../routes/z/+page.svelte",
			"/a": "../routes/a/+page.svelte",
			"/m": "../routes/m/+page.svelte",
		},
	}
	a := GenerateRouter(in)
	b := GenerateRouter(in)
	if a != b {
		t.Fatalf("non-deterministic output across calls")
	}
	// Keys must appear in sorted order so codegen output is reproducible.
	posA := strings.Index(a, `"/a"`)
	posM := strings.Index(a, `"/m"`)
	posZ := strings.Index(a, `"/z"`)
	if !(posA < posM && posM < posZ) {
		t.Errorf("expected sorted key order; got positions a=%d m=%d z=%d", posA, posM, posZ)
	}
}

// TestGenerateRouter_includesRuntime asserts the generated module carries
// the SPA runtime (click listener, popstate, navigate, manifest matcher),
// not just the import table.
func TestGenerateRouter_includesRuntime(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{"/": "../routes/+page.svelte"}})
	want := []string{
		"export function startRouter",
		"export function shouldNotIntercept",
		"export function matchManifest",
		"document.addEventListener('click'",
		"window.addEventListener('popstate'",
		"new AbortController()",
		"history.replaceState",
		"history[opts.replace ? 'replaceState' : 'pushState']",
		"'/__data.json'",
		"'x-sveltego-data': '1'",
	}
	for _, s := range want {
		if !strings.Contains(src, s) {
			t.Errorf("router missing %q:\n%s", s, src)
		}
	}
}

// TestGenerateRouter_optOutMatrix checks the shouldNotIntercept decision
// matrix mentioned in the issue spec covers every documented case:
// data-sveltego-reload, download, target=_blank, rel=external. Modifier
// keys are handled in onClick (separate path).
func TestGenerateRouter_optOutMatrix(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	cases := []string{
		"data-sveltego-reload",
		"hasAttribute('download')",
		"target !== '' && target !== '_self'",
		"rel.includes('external')",
		"e.metaKey || e.ctrlKey || e.shiftKey || e.altKey",
		"e.button !== 0",
	}
	for _, s := range cases {
		if !strings.Contains(src, s) {
			t.Errorf("router missing opt-out clause %q:\n%s", s, src)
		}
	}
}

// TestGenerateRouter_optInAttrs verifies the SvelteKit-style opt-in
// modifiers (replacestate, noscroll) are wired through navigate.
func TestGenerateRouter_optInAttrs(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"data-sveltego-replacestate",
		"data-sveltego-noscroll",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing opt-in %q", want)
		}
	}
}

// TestGenerateRouter_scrollRestore asserts the SPA router takes manual
// control of scroll restoration and saves/restores per-history-entry
// offsets so back/forward returns to where the user left off.
func TestGenerateRouter_scrollRestore(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"history.scrollRestoration = 'manual'",
		"const scrolls = new Map<number, [number, number]>",
		"function saveScroll()",
		"scrolls.get(opts.restoreId)",
		"window.scrollTo(saved[0], saved[1])",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing scroll restore plumbing %q", want)
		}
	}
}

// TestGenerateClientEntry_importsRouter asserts the per-route entry now
// boots the SPA router after the initial mount.
func TestGenerateClientEntry_importsRouter(t *testing.T) {
	t.Parallel()

	src := GenerateClientEntry("../../routes/+page.svelte", "../__router/router")
	for _, want := range []string{
		`import Page from "../../routes/+page.svelte"`,
		`import { startRouter } from "../__router/router"`,
		"const component = mount(Page",
		"startRouter({ component, payload, target: document.body })",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("entry missing %q:\n%s", want, src)
		}
	}
}
