# 2026-05-03 — Four end-to-end hydration bugs surfaced only via the dummy (#495 #498 #499 #500)

## Insight

The pure-Svelte pivot (RFC #379) and SSR Option B rollout (RFC #421, ADR 0009) shipped through 9 phases of unit + integration tests, but the project had no end-to-end runnable that exercised all 4 render modes (SSR, SSG, SPA, Static) against a real browser. Once `sveltego-dummy/` came online with one sub-app per mode, four small but load-bearing bugs surfaced inside one user session — none of them caught by the existing test suite:

| Layer | Bug | Symptom |
|---|---|---|
| codegen (#495) | Manifest emitted `Options: kit.PageOptions{...}` without importing `kit` for template-only Svelte routes | `go build` fails on routes that have a `_page.svelte` but no `_page.server.go`, no layouts, no error boundaries, no non-default options |
| codegen (#498) | `sveltego prerender` invokes codegen with `NoClient: true`, which clobbered `clientKeysByPkg` | Manifest re-emitted without `ClientKey` field on every route → prerendered HTML loses `<script type="module" src="...">` → SSG pages don't hydrate |
| server (#499) | `renderEmptyShell` for SPA mode (`SSR=false`) skips Vite tags AND the JSON payload | SPA browser receives bare `<div id="app"></div>` with no JS pointing at it |
| client (#500) | Generated `entry.ts` always calls `mount(Page, { target: document.body })` | On SSR/SSG body has SSR'd markup → `mount` *appends* a duplicate component → original DOM stays handlerless, counter dead |

All four sit on the same shape: **config drift between two pipelines that compile-tested cleanly in isolation but disagreed at runtime.** The codegen pipeline's `Options` field assumed kit was imported. The prerender CLI's `NoClient` gate assumed clientKey wiring was a file-emission concern (it's path math). The server's `renderEmptyShell` predates the pure-Svelte pivot. The client entry generator confused `mount` (fresh render) with `hydrate` (attach to existing DOM).

Pattern: **two pipelines that share a contract via a struct field or string key, edited at different times, neither aware the other's invariants changed.** Tests for each pipeline pass independently; only e2e proves they agree.

## Self-rules

1. **Every render-mode invariant in CLAUDE.md gets a runnable proof.** Adding a new render mode, a new `PageOptions` field, or a new server-pipeline branch means adding (or extending) a sub-app in `sveltego-dummy/` that exercises it. CI is the second line; the dummy is the first.
2. **Gate analysis: when a feature flag controls *file emission*, do not let it gate *path mapping*.** ClientKey is a string derived from `route.Dir` + `outDir`; the file existing on disk is a separate concern. Apply the same split anywhere a "skip writing X" flag also silently strips structural metadata downstream consumers depend on.
3. **`mount` vs `hydrate` is not interchangeable.** Pick the right one based on shell shape (presence of `<div id="app">` for SPA/Static, body markup for SSR/SSG). Never default to one without inspecting the response.
4. **When the manifest emitter writes `kit.PageOptions{...}` (or any other typed value), verify the corresponding import is in scope at the same emission site.** The 2026-05-03 `usesFmt` lesson (`fmt-import-gate-stale-branch.md`) is the same shape — gate disagrees with body emit. Treat any future "missing import" bug in `manifest.go` as a third instance of this category and refactor the import gate to track *what was actually emitted*, not *what we predicted would be emitted*.
5. **End-to-end shipping definition: a render mode is "shipped" only when a sub-app of `sveltego-dummy/` proves it hydrates and the user-facing interaction (click, navigate, submit) works.** Type-check + unit test green is a precondition, not a sufficient condition.

## What the dummy now proves

- SSR (port 8081): server-rendered body + `hydrate()` attaches handlers; counter increments, single DOM copy.
- SSG (port 8082): frozen prerendered body + client bundle script + `hydrate()`; counter increments against build-time HTML.
- SPA (port 8083): empty shell + `mount()` into `<div id="app">`; counter increments after Load data flows.
- Static (port 8084): same as SPA but no Load.

Re-running `bash setup.sh && bash run-all.sh` is now the regression check for all four modes.

## References

- PR #501 — bundled fix for #495 + #498 + #499 + #500
- `tasks/lessons/2026-05-03-fmt-import-gate-stale-branch.md` — same pattern, prior instance (#485)
- `tasks/decisions/0009-ssr-option-b.md` — pipeline this work landed on
- `sveltego-dummy/` — the runnable that surfaced all four bugs
