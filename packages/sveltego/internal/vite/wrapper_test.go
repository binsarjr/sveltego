package vite

import (
	"strings"
	"testing"
)

func TestGenerateWrapper_singleLayoutNests(t *testing.T) {
	t.Parallel()

	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
	})

	for _, want := range []string{
		`import L0 from "../../../src/routes/_layout.svelte";`,
		`import Page from "../../../src/routes/_page.svelte";`,
		`import { wrapperState } from "../../__router/wrapper-store.svelte";`,
		"let { data, layoutData = [], form = null } = $props();",
		"wrapperState.data = data;",
		"wrapperState.layoutData = layoutData;",
		"wrapperState.form = form;",
		`<L0 data={wrapperState.layoutData[0] ?? {}}>`,
		`<Page data={wrapperState.data} form={wrapperState.form} />`,
		`</L0>`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("wrapper missing %q:\n%s", want, src)
		}
	}
}

func TestGenerateWrapper_nestedLayoutsComposeOuterToInner(t *testing.T) {
	t.Parallel()

	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{
			"../../../src/routes/_layout.svelte",
			"../../../src/routes/admin/_layout.svelte",
		},
		PagePath:    "../../../src/routes/admin/users/_page.svelte",
		StoreImport: "../../../__router/wrapper-store.svelte",
	})

	// Outer layout opens before inner; inner closes before outer.
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

func TestGenerateWrapper_snapshotIsReExported(t *testing.T) {
	t.Parallel()

	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/snapshot/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
		HasSnapshot:   true,
	})
	if !strings.Contains(src, `import Page, { snapshot } from "../../../src/routes/snapshot/_page.svelte";`) {
		t.Errorf("missing snapshot import:\n%s", src)
	}
	if !strings.Contains(src, "export { snapshot };") {
		t.Errorf("missing snapshot re-export:\n%s", src)
	}
}

// TestGenerateWrapper_noDynamicComponentsOnFirstRender defends the
// hydration-parity contract: the wrapper must render the page through a
// STATIC `<Page>` reference, never through `{#if}`, `{@const}`, or
// `<svelte:component>` dispatch. Dynamic dispatch injects Svelte
// hydration comment markers that are absent from the SSR HTML and trips
// `svelte/e/hydration_mismatch` warnings on first paint (regression
// surfaced on basic playground's inert _layout.svelte).
func TestGenerateWrapper_noDynamicComponentsOnFirstRender(t *testing.T) {
	t.Parallel()
	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
	})
	for _, banned := range []string{"{#if", "{@const", "<svelte:component", "<PageSlot"} {
		if strings.Contains(src, banned) {
			t.Errorf("wrapper must not emit %q (hydration-mismatch hazard):\n%s", banned, src)
		}
	}
}

func TestGenerateWrapper_deterministic(t *testing.T) {
	t.Parallel()

	in := WrapperOptions{
		LayoutImports: []string{"../a.svelte", "../b.svelte"},
		PagePath:      "../page.svelte",
		StoreImport:   "../store.svelte",
	}
	a := GenerateWrapper(in)
	b := GenerateWrapper(in)
	if a != b {
		t.Fatal("GenerateWrapper non-deterministic")
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
		"data: unknown;",
		"layoutData: unknown[];",
		"form: unknown;",
		"export function _setWrapperState",
		"wrapperState.data = next.data;",
		"wrapperState.layoutData = next.layoutData;",
		"wrapperState.form = next.form;",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("wrapper-store missing %q:\n%s", want, src)
		}
	}
	for _, banned := range []string{"page: any;", "wrapperState.page", "next.page"} {
		if strings.Contains(src, banned) {
			t.Errorf("wrapper-store still references dropped page field (%q):\n%s", banned, src)
		}
	}
}

func TestGenerateClientEntry_wrapperPathSwapsRoot(t *testing.T) {
	t.Parallel()

	src := GenerateClientEntry(ClientEntryOptions{
		RelSveltePath:  "../../../src/routes/admin/_page.svelte",
		RelRouterPath:  "../../__router/router",
		RelWrapperPath: "./wrapper.svelte",
		LayoutChainKey: "abc123def4567890",
	})
	if !strings.Contains(src, `import Root from "./wrapper.svelte";`) {
		t.Errorf("expected Root import to be the wrapper, got:\n%s", src)
	}
	if !strings.Contains(src, "props: { data: payload.data, layoutData: payload.layoutData ?? [], form: payload.form ?? null }") {
		t.Errorf("expected wrapper props to forward layoutData (initial mount stays simple — page comes from the wrapper's own default import):\n%s", src)
	}
	if !strings.Contains(src, `chainKey: "abc123def4567890"`) {
		t.Errorf("expected chainKey to be forwarded to startRouter:\n%s", src)
	}
}

func TestGenerateRouter_emitsChainKeysMap(t *testing.T) {
	t.Parallel()
	src := GenerateRouter(RouterOptions{
		Routes: map[string]string{
			"/":      "../../routes/_page/wrapper.svelte",
			"/about": "../../routes/about/_page/wrapper.svelte",
			"/api":   "../../routes/api/_page.svelte",
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
}
