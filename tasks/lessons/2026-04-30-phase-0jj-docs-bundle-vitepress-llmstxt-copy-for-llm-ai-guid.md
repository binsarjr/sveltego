## Phase 0jj — docs bundle: Vitepress + llms.txt + Copy-for-LLM + AI guide (2026-04-30)

### Insight

- **Vitepress closeBundle hook is the right place to emit auxiliary text artifacts** (`llms.txt`, raw `.md`, `llms-full.txt`). `buildEnd` from the Rollup hook spec works too but `closeBundle` runs after Vite has fully written `outDir`, so subsequent `writeFile` calls are guaranteed to land in the same directory the rest of the build wrote into. Writing to the resolved `cfg.build.outDir` (not a hardcoded `dist/`) keeps the plugin portable across Vitepress upgrades that may relocate the output root.
- **`tinyglobby` is the canonical Vitepress glob library** (matches the example in spec #70) and ships zero transitive cost. No `globby`, no `fast-glob`. One devDep, no surprises.
- **Frontmatter parsing in a docs plugin should stay hand-rolled, not import gray-matter.** Five lines of `split('
').forEach` over `key: value` covers every field we declare (title, order, summary). Reaching for gray-matter pulls in stricter YAML semantics we don't need and a transitive surface we have to audit.
- **Vue file extension matters for Vitepress 1.x theme components.** `.vue` SFCs work out of the box; a `.svelte` or `.tsx` file in the theme folder will not be picked up by the default theme extension layer. The component lives in `theme/components/CopyForLLM.vue` and gets injected via `Layout` slot in `theme/index.ts`.
- **Raw-md serving in dev needs middleware, not file-system passthrough.** Vitepress' default dev server transforms every `.md` request into HTML via `vue-router`. The plugin must `configureServer` and intercept before the default handler fires. Production is simpler — just write the raw files alongside the HTML at `closeBundle`.
- **Copy-for-LLM clipboard fallback is mandatory.** `navigator.clipboard.writeText` requires a secure context; on local previews over plain HTTP the API is gated. The component falls back to `document.execCommand('copy')` via a hidden textarea so the button works on `localhost` builds and over LAN proxies during demos.
- **Keep docs scope strictly disjoint from packages/sveltego/**.** Touching any Go package would conflict with the 9 parallel agents working on the codegen/runtime tree. The only repo-root files allowed beyond `docs/**` are `.gitignore` (to ignore Vitepress' `dist/` and `cache/`) and `tasks/lessons.md` (append-only journal).

### Self-rules

1. **Docs site outputs go in `closeBundle`, not `buildEnd`.** Writing through `cfg.build.outDir` keeps the plugin tied to Vitepress' actual output path; never hardcode `dist/`.
2. **Reach for `tinyglobby` for any new Vitepress plugin that walks the source tree.** Matches Vitepress's own ecosystem choice and avoids a heavier dep tree.
3. **Hand-roll frontmatter parsing in plugins that only consume known keys.** Five-line split-on-colon beats importing gray-matter when the schema is `{title, order, summary}`. Save the dep budget for things that earn it.
4. **Vitepress theme components live in `.vue`; Layout slots are the injection seam.** No `.svelte` in Vitepress, no JSX without explicit setup. Use `h(DefaultTheme.Layout, null, { 'doc-before': () => h(Component) })` to inject content above the page body without overriding the default theme wholesale.
5. **Always provide a clipboard-API fallback for "Copy" buttons in static docs.** `localhost` over plain HTTP is the most common build-test path; failing silently there guarantees a "doesn't work" bug report. Hidden-textarea + `document.execCommand('copy')` handles every legacy path.
6. **Concurrency-disjoint scope is the entire job in a parallel-agent phase.** When 9 other agents touch `packages/sveltego/**`, the docs phase touches `docs/**` plus `.gitignore` plus `tasks/lessons.md` — period. No code refactors, no doc reflows in `README.md`. Cross-doc consistency happens in a follow-up serial commit if needed.

