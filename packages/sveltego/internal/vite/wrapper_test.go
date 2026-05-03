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
		`<L0 data={wrapperState.layoutData[0] ?? {}}>`,
		`<Page data={wrapperState.data} form={wrapperState.form} />`,
		`</L0>`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("wrapper missing %q:\n%s", want, src)
		}
	}
}

// TestGenerateWrapper_takesNoProps defends Bug 2 fix: any top-level
// reference to `data`/`layoutData`/`form` props in the wrapper trips
// Svelte 5's `state_referenced_locally` analyzer. Moving the seed into
// `$effect` defers it past first render and breaks hydration. The fix
// keeps the wrapper as a pure rune consumer — entry.ts owns the
// `_setWrapperState` seed BEFORE mount runs (see
// TestGenerateClientEntry_wrapperPathSwapsRoot).
func TestGenerateWrapper_takesNoProps(t *testing.T) {
	t.Parallel()
	src := GenerateWrapper(WrapperOptions{
		LayoutImports: []string{"../../../src/routes/_layout.svelte"},
		PagePath:      "../../../src/routes/_page.svelte",
		StoreImport:   "../../__router/wrapper-store.svelte",
	})
	for _, banned := range []string{
		"$props()",
		"$effect(() => {",
		"wrapperState.data =",
		"wrapperState.layoutData =",
		"wrapperState.form =",
	} {
		if strings.Contains(src, banned) {
			t.Errorf("wrapper must not contain %q (top-level prop refs trip state_referenced_locally):\n%s", banned, src)
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
	// Wrapper takes zero props — entry.ts seeds the rune store before
	// mount via _setWrapperState (#508 hydration parity).
	if !strings.Contains(src, "import { _setWrapperState } from") {
		t.Errorf("entry.ts must import _setWrapperState to seed the wrapper rune before mount:\n%s", src)
	}
	if !strings.Contains(src, "_setWrapperState({") {
		t.Errorf("entry.ts must call _setWrapperState with the payload before mount:\n%s", src)
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
