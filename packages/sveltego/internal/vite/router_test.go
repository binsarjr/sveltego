package vite

import (
	"strings"
	"testing"
)

func TestGenerateRouter_emitsImportMap(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{
		Routes: map[string]string{
			"/":          "../../routes/_page.svelte",
			"/blog/[id]": "../../routes/blog/[id]/_page.svelte",
		},
	})

	if !strings.Contains(src, `"/": () => import("../../routes/_page.svelte")`) {
		t.Errorf("missing root route import:\n%s", src)
	}
	if !strings.Contains(src, `"/blog/[id]": () => import("../../routes/blog/[id]/_page.svelte")`) {
		t.Errorf("missing param route import:\n%s", src)
	}
}

func TestGenerateRouter_deterministic(t *testing.T) {
	t.Parallel()

	in := RouterOptions{
		Routes: map[string]string{
			"/z": "../routes/z/_page.svelte",
			"/a": "../routes/a/_page.svelte",
			"/m": "../routes/m/_page.svelte",
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

	src := GenerateRouter(RouterOptions{Routes: map[string]string{"/": "../routes/_page.svelte"}})
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

	src := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath: "../../routes/_page.svelte",
		RelRouterPath: "../__router/router",
	})
	for _, want := range []string{
		`import Root from "../../routes/_page.svelte"`,
		`import { startRouter } from "../__router/router"`,
		"import { hydrate, mount } from 'svelte'",
		"const appShell = document.getElementById('app');",
		"const target = appShell ?? document.body;",
		"const attach = appShell ? mount : hydrate;",
		"const component = attach(Root",
		`startRouter({ component, payload, target, chainKey: "" });`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("entry missing %q:\n%s", want, src)
		}
	}
	if strings.Contains(src, "snapshot") {
		t.Errorf("entry should not reference snapshot when HasSnapshot=false:\n%s", src)
	}
}

// TestGenerateClientEntry_snapshotImport asserts that opting in routes
// pull the `snapshot` named export out of the _page.svelte module and
// hand it to startRouter.
func TestGenerateClientEntry_snapshotImport(t *testing.T) {
	t.Parallel()

	src := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath: "../../routes/_page.svelte",
		RelRouterPath: "../__router/router",
		HasSnapshot:   true,
	})
	for _, want := range []string{
		`import Root, { snapshot } from "../../routes/_page.svelte"`,
		`startRouter({ component, payload, target, snapshot, chainKey: "" });`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("entry missing %q:\n%s", want, src)
		}
	}
}

// TestGenerateRouter_snapshotRuntime asserts the snapshot capture and
// restore plumbing is emitted: an in-memory map keyed by history id,
// capture before navigate, restore after mount on popstate.
func TestGenerateRouter_snapshotRuntime(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{
		Routes:         map[string]string{"/": "../routes/_page.svelte"},
		SnapshotRoutes: map[string]bool{"/": true},
	})
	for _, want := range []string{
		`"/": true`,
		"function captureSnapshot()",
		"function restoreSnapshot(",
		"const snapshots = new Map<number, unknown>",
		"snapshots.set(currentHistoryId, currentSnapshot.capture())",
		"snapshotRoutes[routeId] ? pageMod.snapshot ?? null : null",
		"captureSnapshot();",
		"snapshot?: Snapshot",
		"restoreSnapshot(opts.restoreId)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing snapshot wiring %q:\n%s", want, src)
		}
	}
}

// TestGenerateRouter_snapshotKeysSorted asserts the snapshotRoutes
// table is emitted in deterministic key order so generated output is
// reproducible across runs.
func TestGenerateRouter_snapshotKeysSorted(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{
		Routes: map[string]string{
			"/a": "../routes/a/_page.svelte",
			"/m": "../routes/m/_page.svelte",
			"/z": "../routes/z/_page.svelte",
		},
		SnapshotRoutes: map[string]bool{
			"/z": true,
			"/a": true,
			"/m": true,
		},
	})
	posA := strings.Index(src, `"/a": true`)
	posM := strings.Index(src, `"/m": true`)
	posZ := strings.Index(src, `"/z": true`)
	if !(posA > 0 && posA < posM && posM < posZ) {
		t.Fatalf("snapshot keys out of order: a=%d m=%d z=%d\n%s", posA, posM, posZ, src)
	}
}

// TestGenerateRouter_noSnapshotRoutes asserts that a router with no
// snapshot routes still emits the runtime hooks but a never-true table.
func TestGenerateRouter_noSnapshotRoutes(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{
		Routes: map[string]string{"/": "../routes/_page.svelte"},
	})
	if !strings.Contains(src, "const snapshotRoutes: Record<string, true> = {\n};") {
		t.Errorf("expected empty snapshotRoutes table:\n%s", src)
	}
	if !strings.Contains(src, "function captureSnapshot()") {
		t.Errorf("router still needs captureSnapshot helper:\n%s", src)
	}
}

// TestGenerateRouter_onLeave verifies the OnLeave hook surface from #172:
// pages register cleanup callbacks via onLeave(fn), callbacks fire once
// on the next navigation commit, and the registry is cleared after
// firing so a remount does not see stale callbacks from a prior visit.
func TestGenerateRouter_onLeave(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"export function onLeave(fn: () => void)",
		"leaveCallbacks.push(fn)",
		"function fireLeaveCallbacks()",
		"leaveCallbacks = [];",
		"fireLeaveCallbacks();",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing onLeave plumbing %q", want)
		}
	}
	// Callbacks must fire BEFORE the new component mounts so the outgoing
	// page sees a consistent DOM during cleanup. fireLeaveCallbacks() must
	// appear before the unmount/mount block.
	idxFire := strings.Index(src, "fireLeaveCallbacks();")
	idxMount := strings.Index(src, "mounted = mount(pageMod.default")
	if idxFire < 0 || idxMount < 0 || idxFire >= idxMount {
		t.Fatalf("fireLeaveCallbacks must precede mount; got fire=%d mount=%d", idxFire, idxMount)
	}
}

// TestGenerateRouter_redirectFollow verifies #181: fetchSPA detects 3xx,
// follows internal Locations via goto, falls back to location.assign for
// external targets and X-Sveltego-Reload responses, and caps the
// redirect chain so a server cannot loop the client forever.
func TestGenerateRouter_redirectFollow(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"export async function fetchSPA",
		"redirect: 'manual'",
		"x-sveltego-reload",
		"location.assign(loc)",
		"location.assign(target.href)",
		"matchManifest(target.pathname)",
		"await goto(target)",
		"MAX_REDIRECTS",
		"function isRedirect(status: number)",
		"status === 301",
		"status === 302",
		"status === 303",
		"status === 307",
		"status === 308",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing redirect-follow plumbing %q", want)
		}
	}
}

// TestGenerateRouter_windowSurface pins the window.__sveltego_router__
// shape so siblings (enhance runtime, user code) can call goto and
// matchManifest without joining the router import graph.
func TestGenerateRouter_windowSurface(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{}})
	for _, want := range []string{
		"__sveltego_router__",
		"goto,",
		"fetchSPA,",
		"onLeave,",
		"matchManifest,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing window surface %q", want)
		}
	}
}

// TestGenerateEnhance_redirectFollowsSPA pins the enhance runtime: when
// the action envelope reports a redirect with an internal target and the
// router exposes goto, we SPA-navigate; otherwise we fall back to a full
// load. This is the form-action POST -> _server.go 303 path from #181.
func TestGenerateEnhance_redirectFollowsSPA(t *testing.T) {
	t.Parallel()

	src := GenerateEnhanceRuntime()
	for _, want := range []string{
		"__sveltego_router__",
		"router.matchManifest",
		"router.goto",
		"window.location.href = result.location",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("enhance runtime missing redirect-follow plumbing %q", want)
		}
	}
}

// TestGenerateStateModule_emitsSurface pins the public $app/state surface.
// page exposes the nine fields documented in #312; navigating and updated
// expose `current` getters powered by Svelte 5 runes.
func TestGenerateStateModule_emitsSurface(t *testing.T) {
	t.Parallel()

	src := GenerateStateModule()
	for _, want := range []string{
		"export const page",
		"export const navigating",
		"export const updated",
		"$state<Page>",
		"$state<Navigation | null>(null)",
		"export function _setPage",
		"export function _setNavigating",
		"export function _setUpdated",
		"export function _startVersionPoller",
		"VERSION_ENDPOINT = '/_app/version.json'",
		"get url()",
		"get params()",
		"get route()",
		"get status()",
		"get error()",
		"get data()",
		"get form()",
		"get state()",
		"history.state",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("state module missing %q:\n%s", want, src)
		}
	}
}

// TestGenerateRouter_importsStateSetters confirms the router calls into
// the state module when starting and committing navigations so $app/state
// stays in sync with the active page.
func TestGenerateRouter_importsStateSetters(t *testing.T) {
	t.Parallel()

	src := GenerateRouter(RouterOptions{Routes: map[string]string{"/": "../routes/_page.svelte"}})
	for _, want := range []string{
		"import { _setPage, _setNavigating",
		"_setPage(initial.payload)",
		"_setNavigating({",
		"_setNavigating(null)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing %q:\n%s", want, src)
		}
	}
}

// TestGenerateClientEntry_seedsState asserts the per-route entry primes
// $app/state from the hydration payload before mount so user scripts that
// read page.url / page.params on first render see real values, not the
// state module's initial defaults.
func TestGenerateClientEntry_seedsState(t *testing.T) {
	t.Parallel()

	src := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath: "../../routes/_page.svelte",
		RelRouterPath: "../__router/router",
	})
	for _, want := range []string{
		`import { _setPage, _startVersionPoller } from "../__router/state.svelte"`,
		"_setPage(payload);",
		"_startVersionPoller(payload.appVersion, payload.versionPoll)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("entry missing %q:\n%s", want, src)
		}
	}
}

// TestGenerateConfig_aliasesAppState pins the vite alias map: user code
// imports `$app/state`, `$app/navigation`, and `$lib/*` directly,
// mirroring SvelteKit. `$lib` resolves to `<projectRoot>/src/lib` so
// shared components and modules import as `$lib/Button.svelte` instead
// of fragile relative paths.
func TestGenerateConfig_aliasesAppState(t *testing.T) {
	t.Parallel()

	src := GenerateConfig(ConfigOptions{})
	for _, want := range []string{
		`"$app/state": path.resolve(__dirname, ".gen/client/__router/state.svelte")`,
		`"$app/navigation": path.resolve(__dirname, ".gen/client/__router/navigation")`,
		`"$lib": path.resolve(__dirname, "src/lib")`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("config missing alias %q:\n%s", want, src)
		}
	}
}
