# 2026-05-02 — Mustache-Go atomic delete (#468 PR2)

PR1 (#477, sha 4dff070) retired the runtime LayoutChain. PR #483
(sha abc1178b) closed the SSR-transpile chain-mate gap (#478). The
Mustache-Go pipeline is now confirmed dead in production for all 4
playgrounds.

This PR is the atomic delete.

## Confidence matrix — explicit inventory (12 files, ~3,222 LOC)

| File | LOC | External refs | Verdict |
|---|---:|---|---|
| `element.go` | 376 | none | DELETE |
| `slot.go` | 571 | exports `GenerateComponent`, `ComponentOptions`, `ComponentResult`, `Diagnostic`, `DiagnosticSeverity` — only consumed by other dead-12 files (`compemit.go`) and dying tests | DELETE |
| `snippets.go` | 82 | none | DELETE |
| `special.go` | 36 | none | DELETE |
| `head.go` | 118 | exports `extractHeadChildren` — only consumed by dying `codegen.go` mustache entry points + dying `build.go:emitLayout` | DELETE |
| `style.go` | 190 | exports `extractStyle`, `applyScopeClass`, `emitStyleBlock` — only consumed by dying `codegen.go` entry points | DELETE |
| `runes.go` | 723 | exports `analyzeRunes` — consumed by `script.go:extractScripts`; `extractScripts` itself is consumed only by dying `codegen.go`+`slot.go`+`compemit.go` | DELETE |
| `compemit.go` | 483 | exports `emitComponentTree` — only `build.go:313` callsite (also dying) | DELETE |
| `component.go` | 213 | none | DELETE |
| `expr.go` | 169 | none | DELETE |
| `text.go` | 44 | none | DELETE |
| `block.go` | 217 | none | DELETE |
| **Σ** | **3,222** | | |

External (`packages/sveltego/cmd/sveltego/*`, `runtime/router/`,
`internal/devserver/`) reach into codegen via `Build`,
`BuildOptions`, `EmitLinksFile`, `CollectLinkRoutes`, `SortLinkRoutes`,
`LinkRoute`, `GenerateManifest`, `ManifestOptions` — none touching the 12
dead files. `svelte_js2go/` does not import the parent codegen package.

## Cascade — orphans discovered through call-graph analysis

Once the 12 are gone, the following are also unreachable in production
code (only mustache-pipeline call paths remain):

- `codegen.go` mustache entries — `Generate`, `GenerateLayout`,
  `GenerateErrorPage` plus their internal helpers
  (`emitLayoutDataAlias`, `emitRuneStmts`, `assignLHSName`,
  `rejectRootConst`, `emitChildren`, `emitNode`,
  `(*Builder).emitSpanComment`, `mergeImports`).
- `pagedata.go` whole file — `inferPageData`, `inferLayoutData`,
  `inferDataShape`, `pageDataResult`, `pageDataField`,
  `emitPageDataStruct`, `emitPropsStruct`, etc. SSR transpile uses
  `typegen` instead.
- `provenance.go` whole file — `headerComment`, `spanComment`,
  `provenanceVersion`. svelte_js2go has its own header path
  (`runtime_companion.go`).
- `script.go` partial — keep `detectSnapshotInSvelte`,
  `detectSnapshotExport`, `stripJSComments`,
  `snapshotExportRegexp`, `scriptModuleBlockRegexp`, `formatNode`.
  Drop `extractScripts`, `collectScripts`, `scriptOutput`,
  `formatImportSpec`, `rewriteRunes`, `restoreRunesBytes`,
  `runePrefix`, `runeRegexp`.
- `emit.go` partial — `Builder` and helpers (`Line`, `Linef`,
  `Indent`, `Dedent`, `Bytes`, `Fail`, `Err`, `quoteGo`) are
  load-bearing inside `manifest.go`'s adapter emitters; KEEP. Drop
  the mustache-only fields on Builder (`hasChildren`, `keyCounter`,
  `nestDepth`, `componentMode`, `slots`, `provenance`, `srcPath`,
  `imageVariants`).
- `image.go` whole file — only callers were `element.go:emitElement`
  and `emit.go` doc comments. SSR transpile has no equivalent
  built-in `<Image>` lowering yet — surface as a follow-up if
  needed.
- `imagescan.go`, `image_test.go`, `image_build_test.go` —
  `imagescan.go` `buildImageVariants` is plumbed into
  `BuildOptions.ImageWidths` and the dying `Options.ImageVariants`
  field. Once mustache emit dies, the variants map flows nowhere.
  Drop the lot, plus the `ImageWidths` BuildOptions field if no
  surviving caller passes it.

## Manifest + build trims

- `manifest.go`:
  - `emitLayoutAdapters`: drop the `!li.hasSSR` branch (legacy
    `Layout{}.Render` adapter).
  - `emitErrorAdapters`: drop the `!ei.hasSSR` branch (legacy
    `ErrorPage{}.Render` adapter).
  - `emitLayoutHeadAdapters`: drop entirely (SSR pipeline emits head
    bytes via `payload.HeadHTML()`).
  - `emitHeadAdapters`: drop entirely (`Page{}.Head` is mustache-only;
    Svelte routes carry head via Vite).
  - `usesFmt` accounting: drop the legacy mustache branches; emit
    `fmt` import only when other paths still need it (today: the
    SSR adapters all use `fmt` via `%v` formatting? — verify on
    cleanup).
  - `hasSSR` fields on `layoutImport` / `errorImport`: every adapter is
    SSR now. Drop the field; collapse the wrappers.
  - `pageHeads` map + `LayoutHeads` map: drop. No mustache emitter
    populates them; SSR bridge handles head via Payload.
- `build.go`:
  - `emitErrorPage` (lines 697-730), `emitLayout` (lines 732-793) —
    delete, plus the callsites at 224 and 293 plus the
    `ssrErrorPkgs` / `ssrLayoutPkgs` skip-gates around them
    (`ssrLayoutPlans` / `ssrErrorPlans` are still computed for the
    SSR transpile dispatch — keep those, but the `pkgs` sets become
    one-shot inputs to dispatch only).
  - `emitComponentTree` callsite at 313 — delete; component-tree
    emit was a mustache-pipeline concept.
  - `Options.ImageVariants` plumbing — see above.
- `codegen_test.go`'s 11 mustache tests, `csrf_inject_test.go` (4),
  `element_test.go` (10+), `error_test.go` (3), `head_test.go` (7),
  `layout_test.go` (4), `pagedata_test.go` (1), `provenance_test.go`
  (10), `runes_test.go` (15+), `snippets_test.go` (7),
  `style_test.go` (10), `component_test.go` (9), `compemit_test.go`
  (10), `image_test.go` (full), `image_build_test.go` (full) —
  delete in full.

## Testdata trees

DELETE (mustache fixtures + goldens):

- `testdata/codegen/{errorpage,layout,pagedata,runes,scripts,tailwind}/`
- `testdata/codegen/*.svelte` (the four root-level
  `7[5-7]-svelte-options-runes-*.svelte`)
- `testdata/component/`
- `testdata/golden/codegen/`
- `testdata/golden/component/`
- `testdata/golden/errorpage/`
- `testdata/golden/layout/`

KEEP:

- `testdata/golden/{links,assets,hooks,manifest}/` — used by
  `link_emit_test.go` / `assets_emit_test.go` / `hooks_emit_test.go`
  / `manifest_test.go`.
- `testdata/page-options/` — used by `manifest_test.go`.

## Build_test.go survivors

`build_test.go` mixes Build()-level tests with mustache-output
assertions. Triage:

- `TestBuild_HappyPath` (85), `TestBuild_Determinism` (224),
  `TestBuild_EmitsLayoutChain` (254),
  `TestBuild_EmitsLayoutServer` (323),
  `TestBuild_EmitsSvelteHead` (413),
  `TestBuild_PageDataNamedType` (742),
  `TestBuild_LayoutDataNamedType` (806) — these inspect mustache
  outputs (`page.gen.go`, `layout.gen.go`) plus svelte fixtures the
  Vite path doesn't actually compile here. Need surgical
  conversion or deletion. Conservative initial pass: keep Build()
  smoke (HappyPath asserts the manifest + a wire file, not page
  body) but DELETE the mustache-output assertions; otherwise the
  test fixtures' `_page.svelte` get left without a Mustache emitter.

  Reality check before each: does the fixture have a corresponding
  `_page.server.go` shape? If yes, the SSR transpile may still
  produce useful output; assertions will need rewriting against the
  new artifacts.

## Phase plan (≤5 files per commit)

1. **Phase A — delete the 12 dead files** + cascade-orphan files in
   the same atomic commit. Group: `element.go`, `slot.go`,
   `snippets.go`, `special.go`, `head.go`, `style.go`, `runes.go`,
   `compemit.go`, `component.go`, `expr.go`, `text.go`, `block.go`,
   plus `pagedata.go`, `provenance.go`, `image.go`, `imagescan.go`,
   `image_test.go`, `image_build_test.go`, `pagedata_test.go`,
   `provenance_test.go`. (Many of these are ALREADY orphan in
   non-test code paths once the entry points die; doing them in
   the same commit prevents a half-state where the package
   doesn't compile because `script.go` calls `analyzeRunes` from a
   deleted file.)
2. **Phase B — trim `codegen.go`, `script.go`, `emit.go`,
   `manifest.go`**. Remove mustache `Generate*`, helpers,
   adapter branches.
3. **Phase C — trim `build.go`** (delete `emitLayout`,
   `emitErrorPage`, `emitComponentTree` callsite + helpers; drop
   `ImageVariants` plumbing).
4. **Phase D — delete tests + testdata**. `codegen_test.go`,
   `csrf_inject_test.go`, `element_test.go`, `error_test.go`,
   `head_test.go`, `layout_test.go`, `runes_test.go`,
   `snippets_test.go`, `style_test.go`, `component_test.go`,
   `compemit_test.go` plus the fixture trees.
5. **Phase E — fix surviving test fallout**. `build_test.go` needs
   triage; keep what verifies surviving Build() paths, drop the
   mustache-output assertions.

Each phase: gofumpt, goimports, go vet, go test -race ./..., golangci-lint
run, sveltego compile per playground. Fix any breakage IMMEDIATELY
before moving on.

Single PR for the atomic close (#468 needs a single commit shape per
the issue body's atomic-retirement criterion). Phases here are
internal scaffolding so an interrupted run can resume cleanly.

## Expected LOC delta

Source files (drop):
- 12 inventory: 3,222
- pagedata.go: 320
- provenance.go: 70
- image.go: 320
- imagescan.go: 100

Trim (estimate):
- codegen.go: 17,200 → ~3,000 (drop 14,200)
- script.go: 8,300 → ~3,000 (drop 5,300)
- emit.go: 4,200 → ~2,500 (drop 1,700)
- manifest.go: 49,400 → ~38,000 (drop ~11,000 in adapter trims, head emit, hasSSR plumbing)
- build.go: 33,500 → ~28,500 (drop ~5,000 in emitLayout, emitErrorPage, ImageVariants plumbing)

Tests (drop):
- ~2,800 LOC across 13 test files
- + image_test (~200 LOC), image_build_test (~150)

Testdata: ~1.0M of fixture/golden bytes.

**Conservative net source delta: ~9,500–12,000 LOC** if we
include the trimmed cascades. If we ship the strict
inventory-only PR, it's ~3,222 LOC plus the manifest/build trims
required for compilation — around **5,000–6,000 LOC**. The issue
body's atomic-retirement language supports the larger cascade
delete, but if we hit a snag on a cascade the strict-inventory
fallback is achievable in one PR.

## Risk register

- `imageElementName` / `<Image>` is a documented user-facing
  feature. With mustache pipeline gone, `<Image>` in a `.svelte`
  file falls through to the Vite Svelte plugin which doesn't know
  about it. **Action**: file a follow-up issue to re-implement
  `<Image>` in the SSR transpile path or document the
  feature-removal. For now, the playgrounds don't use `<Image>` —
  verified by running `grep -rn '<Image' playgrounds/` from main.
- CSRF auto-injection (`shouldInjectCSRF`/`emitCSRFHiddenInput`)
  was a mustache pipeline feature. Pure-Svelte authors have to
  emit the hidden input themselves. **Action**: same as Image —
  follow-up issue. The playground `<form>` cases use
  `enhance.ts` for client-side handling; CSRF token injection
  shifts to that layer if needed.
- `build_test.go` is the largest unknown. Some Build()-level
  assertions verify mustache outputs that no longer exist. The
  conservative move is to delete the failing assertions in this
  PR and file a follow-up to expand the SSR-transpile build
  smoke if coverage shrinks too much.
