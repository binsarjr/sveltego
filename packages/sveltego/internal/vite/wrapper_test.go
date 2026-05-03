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
		"$effect(() => {",
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

// TestGenerateWrapper_runeSeedsInsideEffect defends Bug 2 fix: bare
// top-level `wrapperState.x = x` assignments capture only the initial
// prop value, trip Svelte 5's state_referenced_locally warning, and
// break the same-route reactive-refresh contract from #508. The seed
// must live inside a `$effect` so the rune system tracks the prop reads.
func TestGenerateWrapper_runeSeedsInsideEffect(t *testing.T) {
	t.Parallel()
	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
	})
	effectIdx := strings.Index(src, "$effect(() => {")
	if effectIdx < 0 {
		t.Fatalf("wrapper missing $effect block:\n%s", src)
	}
	// All three seed lines must sit inside the $effect body, not before it.
	for _, seed := range []string{"wrapperState.data = data;", "wrapperState.layoutData = layoutData;", "wrapperState.form = form;"} {
		seedIdx := strings.Index(src, seed)
		if seedIdx < 0 {
			t.Errorf("wrapper missing seed line %q:\n%s", seed, src)
			continue
		}
		if seedIdx < effectIdx {
			t.Errorf("seed %q sits outside $effect block (idx=%d, effect=%d)\n%s", seed, seedIdx, effectIdx, src)
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

// TestGenerateWrapper_snapshotIsReExported guards Bug 1 fix: when the
// page's `<script module>` exports `snapshot`, the wrapper must
// re-export it via its own `<script module>` block (Svelte 5 only
// recognises module-level exports there). The instance script must not
// carry the export — that path is parsed as the legacy props syntax and
// emits no ESM export, breaking the wrapper-as-snapshot bridge for
// entry.ts (vite errors with `"snapshot" is not exported by ".../wrapper.svelte"`).
func TestGenerateWrapper_snapshotIsReExported(t *testing.T) {
	t.Parallel()

	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/snapshot/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
		ModuleExports: []string{"snapshot"},
	})
	if !strings.Contains(src, `<script module lang="ts">`) {
		t.Errorf("missing <script module> block:\n%s", src)
	}
	if !strings.Contains(src, `export { snapshot } from "../../../src/routes/snapshot/_page.svelte";`) {
		t.Errorf("missing snapshot re-export-from in module block:\n%s", src)
	}
	// The snapshot must NOT be re-exported from inside the instance
	// `<script lang="ts">` — Svelte 5 reads that as the legacy props
	// syntax and emits no ESM export.
	moduleEnd := strings.Index(src, "</script>")
	if moduleEnd < 0 {
		t.Fatalf("malformed wrapper:\n%s", src)
	}
	instance := src[moduleEnd:]
	if strings.Contains(instance, "export { snapshot }") {
		t.Errorf("snapshot must not be re-exported from instance script:\n%s", src)
	}
	// Page must still be the default import for component composition.
	if !strings.Contains(src, `import Page from "../../../src/routes/snapshot/_page.svelte";`) {
		t.Errorf("missing Page default import in instance script:\n%s", src)
	}
}

// TestGenerateWrapper_multipleModuleExportsReExported covers a synthetic
// `<script module>` exporting more than just snapshot — the wrapper
// must propagate every name through the same module-block re-export
// (Bug 1 generalisation; the contract isn't snapshot-specific).
func TestGenerateWrapper_multipleModuleExportsReExported(t *testing.T) {
	t.Parallel()
	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/multi/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
		ModuleExports: []string{"alpha", "beta", "snapshot"},
	})
	if !strings.Contains(src, `export { alpha, beta, snapshot } from "../../../src/routes/multi/_page.svelte";`) {
		t.Errorf("missing combined module-export re-export:\n%s", src)
	}
}

// TestGenerateWrapper_noModuleBlockWhenNoExports keeps the wrapper free
// of an empty `<script module>` block when the page has no module-level
// exports — Svelte 5 tolerates an empty module script but it is noise
// in the generated output.
func TestGenerateWrapper_noModuleBlockWhenNoExports(t *testing.T) {
	t.Parallel()
	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
	})
	if strings.Contains(src, `<script module`) {
		t.Errorf("wrapper should not emit <script module> when ModuleExports is empty:\n%s", src)
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
