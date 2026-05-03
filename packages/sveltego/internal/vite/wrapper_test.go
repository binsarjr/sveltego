package vite

import (
	"strings"
	"testing"
)

func TestGenerateChainWrapper_singleLayoutNests(t *testing.T) {
	t.Parallel()

	src := GenerateChainWrapper(ChainWrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		StoreImport:   "../wrapper-store.svelte",
	})

	for _, want := range []string{
		`import L0 from "../../../src/routes/_layout.svelte";`,
		`import { wrapperState } from "../wrapper-store.svelte";`,
		`const Page = $derived(wrapperState.Page);`,
		`<L0 data={wrapperState.layoutData[0] ?? {}}>`,
		`<Page data={wrapperState.data} form={wrapperState.form} />`,
		`</L0>`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("wrapper missing %q:\n%s", want, src)
		}
	}
}

// TestGenerateChainWrapper_takesNoProps locks in the rune-only contract:
// the wrapper must not declare or write to any prop or rune field at the
// top level. Cross-route same-chain SPA navs reuse the wrapper instance,
// so the SPA router writes to wrapperState directly — top-level prop
// references would either trip Svelte 5's `state_referenced_locally`
// analyzer or capture stale module references on the swap.
func TestGenerateChainWrapper_takesNoProps(t *testing.T) {
	t.Parallel()
	src := GenerateChainWrapper(ChainWrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		StoreImport:   "../wrapper-store.svelte",
	})
	for _, banned := range []string{
		"$props()",
		"wrapperState.data =",
		"wrapperState.layoutData =",
		"wrapperState.form =",
		"wrapperState.Page =",
		// No static page import — page module reference flows in via the
		// wrapper-state rune, set by entry.ts before mount and rewritten
		// by the SPA router on cross-route same-chain nav (#518).
		"import Page from",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("wrapper must not contain %q (chain wrapper is rune-only):\n%s", banned, src)
		}
	}
}

func TestGenerateChainWrapper_nestedLayoutsComposeOuterToInner(t *testing.T) {
	t.Parallel()

	src := GenerateChainWrapper(ChainWrapperOptions{
		LayoutImports: []string{
			"../../../src/routes/_layout.svelte",
			"../../../src/routes/admin/_layout.svelte",
		},
		StoreImport: "../wrapper-store.svelte",
	})

	posL0Open := strings.Index(src, "<L0 ")
	posL1Open := strings.Index(src, "<L1 ")
	posPage := strings.Index(src, "<Page ")
	posL1Close := strings.Index(src, "</L1>")
	posL0Close := strings.Index(src, "</L0>")
	if !(posL0Open >= 0 && posL1Open > posL0Open && posPage > posL1Open && posL1Close > posPage && posL0Close > posL1Close) {
		t.Fatalf("nesting order broken: L0=%d L1=%d Page=%d /L1=%d /L0=%d\n%s",
			posL0Open, posL1Open, posPage, posL1Close, posL0Close, src)
	}
	if !strings.Contains(src, "wrapperState.layoutData[0] ?? {}") {
		t.Errorf("missing outer layoutData binding:\n%s", src)
	}
	if !strings.Contains(src, "wrapperState.layoutData[1] ?? {}") {
		t.Errorf("missing inner layoutData binding:\n%s", src)
	}
}

// TestGenerateChainWrapper_dynamicPageReference defends the #518
// hydration-parity contract: the wrapper renders the page through a
// $derived rune reference, NOT through `<svelte:component>`,
// `{#if}` / `{@const}` blocks, or a static import. Dynamic-component
// reactivity is what lets a cross-route same-chain SPA nav swap the
// page without unmounting the wrapper instance — that reuse is the
// mechanism that preserves layout `$state` across `/post/1 → /post/2`.
// Static imports would freeze the wrapper to a single page module per
// chain and reintroduce the bug from PR #517's deferral note.
func TestGenerateChainWrapper_dynamicPageReference(t *testing.T) {
	t.Parallel()
	src := GenerateChainWrapper(ChainWrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		StoreImport:   "../wrapper-store.svelte",
	})
	if !strings.Contains(src, "$derived(wrapperState.Page)") {
		t.Errorf("missing $derived page reference:\n%s", src)
	}
	for _, banned := range []string{"<svelte:component", "{#if wrapperState.Page", "import Page from"} {
		if strings.Contains(src, banned) {
			t.Errorf("wrapper must not use %q (cross-route layout-state regression):\n%s", banned, src)
		}
	}
}

func TestGenerateChainWrapper_deterministic(t *testing.T) {
	t.Parallel()

	in := ChainWrapperOptions{
		LayoutImports: []string{"../a.svelte", "../b.svelte"},
		StoreImport:   "../store.svelte",
	}
	a := GenerateChainWrapper(in)
	b := GenerateChainWrapper(in)
	if a != b {
		t.Fatal("GenerateChainWrapper non-deterministic")
	}
}

func TestLayoutChainKey_emptyForNoLayouts(t *testing.T) {
	t.Parallel()
	if got := LayoutChainKey(nil); got != "" {
		t.Errorf("nil chain: want empty key, got %q", got)
	}
	if got := LayoutChainKey([]string{}); got != "" {
		t.Errorf("empty chain: want empty key, got %q", got)
	}
}

func TestLayoutChainKey_stableAcrossCalls(t *testing.T) {
	t.Parallel()
	chain := []string{"/abs/routes", "/abs/routes/admin"}
	a := LayoutChainKey(chain)
	b := LayoutChainKey(chain)
	if a != b {
		t.Errorf("non-deterministic: %q vs %q", a, b)
	}
	if len(a) != 16 {
		t.Errorf("expected 16-hex-char key, got %q (len %d)", a, len(a))
	}
}

func TestLayoutChainKey_distinctChainsDistinctKeys(t *testing.T) {
	t.Parallel()
	a := LayoutChainKey([]string{"/abs/routes"})
	b := LayoutChainKey([]string{"/abs/routes", "/abs/routes/admin"})
	c := LayoutChainKey([]string{"/abs/routes/admin", "/abs/routes"})
	if a == b || a == c || b == c {
		t.Errorf("expected distinct keys; got a=%q b=%q c=%q", a, b, c)
	}
}

func TestGenerateWrapperStoreModule_runeShape(t *testing.T) {
	t.Parallel()
	src := GenerateWrapperStoreModule()
	for _, want := range []string{
		"export const wrapperState = $state",
		"Page: Component<any> | null;",
		"data: unknown;",
		"layoutData: unknown[];",
		"form: unknown;",
		"export function _setWrapperState",
		"wrapperState.Page = next.Page;",
		"wrapperState.data = next.data;",
		"wrapperState.layoutData = next.layoutData;",
		"wrapperState.form = next.form;",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("wrapper-store missing %q:\n%s", want, src)
		}
	}
}

func TestGenerateClientEntry_wrapperPathSwapsRoot(t *testing.T) {
	t.Parallel()

	src := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath:  "../../../src/routes/admin/_page.svelte",
		RelRouterPath:  "../../__router/router",
		RelWrapperPath: "../__chain/abc123def4567890/wrapper.svelte",
		LayoutChainKey: "abc123def4567890",
	})
	// Wrapper is mounted as Root; the page module is imported separately
	// so entry.ts can seed it into the wrapper-state rune (#518 cross-route
	// preservation). Both imports are required.
	if !strings.Contains(src, `import Root from "../__chain/abc123def4567890/wrapper.svelte";`) {
		t.Errorf("expected Root import to be the chain wrapper, got:\n%s", src)
	}
	if !strings.Contains(src, `import Page from "../../../src/routes/admin/_page.svelte";`) {
		t.Errorf("entry must import the page module so it can seed wrapperState.Page:\n%s", src)
	}
	if !strings.Contains(src, "import { _setWrapperState } from") {
		t.Errorf("entry.ts must import _setWrapperState to seed the wrapper rune before mount:\n%s", src)
	}
	if !strings.Contains(src, "_setWrapperState({") {
		t.Errorf("entry.ts must call _setWrapperState with the payload before mount:\n%s", src)
	}
	if !strings.Contains(src, "Page,") {
		t.Errorf("_setWrapperState seed must include the Page module reference:\n%s", src)
	}
	if !strings.Contains(src, "  props: {},\n") {
		t.Errorf("wrapped mount must pass empty props (rune is the source of truth):\n%s", src)
	}
	if strings.Contains(src, "props: { data: payload.data, layoutData") {
		t.Errorf("wrapped mount must NOT pass data/layoutData props (rune-only contract):\n%s", src)
	}
	if !strings.Contains(src, `chainKey: "abc123def4567890"`) {
		t.Errorf("expected chainKey to be forwarded to startRouter:\n%s", src)
	}
}

func TestGenerateClientEntry_noWrapperKeepsLegacyProps(t *testing.T) {
	t.Parallel()
	// Routes without a layout chain mount the bare _page.svelte — the
	// wrapper-state contract does not apply, so entry.ts keeps passing
	// data/form props directly.
	src := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath: "../../../src/routes/api/_page.svelte",
		RelRouterPath: "../../__router/router",
	})
	if !strings.Contains(src, "props: { data: payload.data, form: payload.form ?? null }") {
		t.Errorf("page-only mount must pass data + form props:\n%s", src)
	}
	if strings.Contains(src, "_setWrapperState") {
		t.Errorf("page-only mount must not seed wrapper-state rune:\n%s", src)
	}
}

func TestGenerateRouter_emitsChainKeysMap(t *testing.T) {
	t.Parallel()
	src := GenerateRouter(RouterOptions{
		Routes: map[string]string{
			"/":      "../../routes/_page.svelte",
			"/about": "../../routes/about/_page.svelte",
			"/api":   "../../routes/api/_page.svelte",
		},
		ChainWrappers: map[string]string{
			"deadbeef00000000": "../__chain/deadbeef00000000/wrapper.svelte",
		},
		ChainKeys: map[string]string{
			"/":      "deadbeef00000000",
			"/about": "deadbeef00000000",
			"/api":   "",
		},
	})
	if !strings.Contains(src, "const chainKeys: Record<string, string> = {") {
		t.Errorf("missing chainKeys table:\n%s", src)
	}
	if !strings.Contains(src, `"/": "deadbeef00000000"`) {
		t.Errorf("missing root chainKey entry:\n%s", src)
	}
	if !strings.Contains(src, `"/api": ""`) {
		t.Errorf("missing empty chainKey entry:\n%s", src)
	}
	if !strings.Contains(src, "import { _setWrapperState } from './wrapper-store.svelte';") {
		t.Errorf("missing wrapper-store import:\n%s", src)
	}
	if !strings.Contains(src, "_setWrapperState({") {
		t.Errorf("missing _setWrapperState call in same-chain branch:\n%s", src)
	}
	if !strings.Contains(src, "const chainWrappers: Record<string, () => Promise<{ default: any }>> = {") {
		t.Errorf("missing chainWrappers loader table:\n%s", src)
	}
	if !strings.Contains(src, `"deadbeef00000000": () => import("../__chain/deadbeef00000000/wrapper.svelte")`) {
		t.Errorf("missing wrapper loader entry:\n%s", src)
	}
}

// TestGenerateRouter_sameChainSwapsPage pins the cross-route same-chain
// preservation contract from #518: when a navigation lands on a route
// whose chainKey matches the currently mounted wrapper's, the router
// loads the destination page module, writes `Page` + payload into the
// wrapper-state rune, and skips the unmount/mount path.
func TestGenerateRouter_sameChainSwapsPage(t *testing.T) {
	t.Parallel()
	src := GenerateRouter(RouterOptions{
		Routes: map[string]string{
			"/post/[id]":      "../../routes/post/[id]/_page.svelte",
			"/post/[id]/edit": "../../routes/post/[id]/edit/_page.svelte",
		},
		ChainKeys: map[string]string{
			"/post/[id]":      "abc",
			"/post/[id]/edit": "abc",
		},
		ChainWrappers: map[string]string{
			"abc": "../__chain/abc/wrapper.svelte",
		},
	})
	for _, want := range []string{
		"const sameChain =",
		"nextChainKey === currentChainKey",
		"_setWrapperState({",
		"Page: pageMod.default",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("router missing same-chain swap plumbing %q:\n%s", want, src)
		}
	}
}
