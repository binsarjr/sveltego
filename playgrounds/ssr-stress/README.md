# ssr-stress playground

End-to-end SSR coverage stress test. Each route exercises a different
Svelte 5 server-output shape so the Phase 7 corpus + Phase 8 fallback
work has concrete fixtures to measure against. Shipped via SSR Phase
7 ([#429](https://github.com/binsarjr/sveltego/issues/429), merged
2026-05-02) and exercised by the playground-smoke CI job.

## Routes

| Path | What it covers |
|---|---|
| `/` | Hello-world: minimal Render with one interpolation. |
| `/longlist` | 100-item `{#each}` — exercises emit-shape volume + escape loop. |
| `/conditional` | `{#if}/{:else}` over scalar fields. |
| `/layoutdeep/level2/level3` | 3-level layout chain with body content per level. |

## Known incomplete shapes (open carryovers)

Tracked together in [#443](https://github.com/binsarjr/sveltego/issues/443):

- **`{#snippet}` + `{@render}`** — the Phase 5 lowerer does not yet
  hoist the snippet definition into the surrounding scope. The
  generated Go references an undefined `ssvar_renderer`. The
  playground avoids snippet-heavy fixtures until #443 lands.
- **`{@const}` at template top level** — currently emits
  `if data { ... }` against a typed struct value, which is
  non-boolean in Go. Awaiting a lowerer guard for truthy-struct
  conditions.

[#456](https://github.com/binsarjr/sveltego/issues/456) wired the
deep layout chain into the children-callback ABI shipped in
[#440](https://github.com/binsarjr/sveltego/issues/440) /
[PR #453](https://github.com/binsarjr/sveltego/pull/453). The
`/layoutdeep/level2/level3` route is the end-to-end fixture: every
level emits non-trivial chrome (`<header>`, `<section>`, `<aside>`)
that wraps the inner page output in `curl` against the SSR'd HTML.

## Snapshots

`testdata/snapshots/<route>.html` — the canonical SSR HTML body for
the route. Used by smoke tests to catch regressions.

## Run

```bash
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app --no-client
./build/app  # listens on :3000
curl -s localhost:3000/
curl -s localhost:3000/longlist
curl -s localhost:3000/conditional
```

## Hydration-parity smoke (#446)

`scripts/hydration-smoke.mjs` is the Playwright harness that loads a
route in headless Chromium, waits for `window.__sveltego_hydrated`,
and fails on Svelte `hydration_mismatch` / `hydration_attribute_changed`
warnings. CI currently runs only the synthetic `--self-test` (proves
the gate fires). Live routes flip on once
[#462](https://github.com/binsarjr/sveltego/issues/462) wires
`ViteManifest` into `cmd/app/main.go` so the served HTML actually
loads the client bundle. To run the harness locally against real
routes post-#462:

```bash
cd playgrounds/ssr-stress
npm install
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app
./build/app &
npx playwright install --with-deps chromium
node ../../scripts/hydration-smoke.mjs --base http://localhost:3000 \
  --routes /,/longlist,/conditional
```

## References

- [ADR 0009](../../tasks/decisions/0009-ssr-option-b.md) — SSR Option
  B decision (build-time JS-to-Go transpile).
- [#429](https://github.com/binsarjr/sveltego/issues/429) — Phase 7
  spec and acceptance criteria.
- [#440](https://github.com/binsarjr/sveltego/issues/440),
  [#443](https://github.com/binsarjr/sveltego/issues/443) — open SSR
  carryovers exercised here.

