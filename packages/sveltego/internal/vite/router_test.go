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

// TestGenerateRouter_navigationAPI asserts the public $app/navigation
// surface is exported from the generated router module so navigation.ts
// can re-export it (#85).
func TestGenerateRouter_navigationAPI(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"export async function goto",
		"export async function invalidate",
		"export async function invalidateAll",
		"export async function preloadData",
		"export async function preloadCode",
		"export function pushState",
		"export function replaceState",
		"export function beforeNavigate",
		"export function afterNavigate",
		"export function onNavigate",
		"export type Payload",
		"export type GotoOpts",
		"export type Navigation",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing navigation export %q", want)
		}
	}
}

// TestGenerateRouter_invalidateDeps verifies the dep registry plumbing
// so invalidate(tag) can match payloads carrying ctx.Depends() output.
func TestGenerateRouter_invalidateDeps(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"const depRegistry = new Map<string, Set<string>>()",
		"function recordDeps(",
		"deps?: string[]",
		"if (stale.includes(activeKey))",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing dep-registry plumbing %q", want)
		}
	}
}

// TestGenerateRouter_prefetch verifies the data-sveltego-prefetch
// triggers (hover, tap, viewport) and Save-Data opt-out (#40).
func TestGenerateRouter_prefetch(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"function installPrefetch()",
		"PREFETCH_HOVER_DELAY_MS = 150",
		"a.dataset.sveltegoPrefetch",
		"document.body.dataset.sveltegoPrefetch",
		"document.addEventListener('mouseover'",
		"document.addEventListener('focusin'",
		"document.addEventListener('pointerdown'",
		"document.addEventListener('touchstart'",
		"new IntersectionObserver(",
		`a[data-sveltego-prefetch="viewport"]`,
		"saveDataEnabled()",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing prefetch plumbing %q", want)
		}
	}
}

// TestGenerateRouter_prefetchSpeculativeHeader pins the X-Sveltego-Preload
// hint on prefetch __data.json fetches so server-side LoadCtx.Speculative
// can fire (#40, follow-up to ctx.Speculative shipped in v0.5).
func TestGenerateRouter_prefetchSpeculativeHeader(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"x-sveltego-preload",
		"speculative = false",
		"if (speculative) headers['x-sveltego-preload']",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing speculative-header plumbing %q", want)
		}
	}
}

// TestGenerateRouter_prefetchCacheBound asserts preloadData honors the
// LRU bound called for in #40 acceptance criteria.
func TestGenerateRouter_prefetchCacheBound(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"PREFETCH_CACHE_MAX = 30",
		"if (cache.size >= PREFETCH_CACHE_MAX)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing LRU bound %q", want)
		}
	}
}

// TestGenerateNavigationModule asserts the public navigation.ts shim
// re-exports every $app/navigation symbol from the router module so
// downstream code has a stable import path (#85).
func TestGenerateNavigationModule(t *testing.T) {
	t.Parallel()

	src := GenerateNavigationModule()
	for _, want := range []string{
		"goto,",
		"invalidate,",
		"invalidateAll,",
		"preloadData,",
		"preloadCode,",
		"pushState,",
		"replaceState,",
		"beforeNavigate,",
		"afterNavigate,",
		"onNavigate,",
		"from './router'",
		"export type { GotoOpts, Navigation }",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("navigation.ts missing %q:\n%s", want, src)
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
