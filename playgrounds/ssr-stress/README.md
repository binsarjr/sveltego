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

[#440](https://github.com/binsarjr/sveltego/issues/440) tracks the
deeper layout-chain carryover (children-callback ABI) — the
`/layoutdeep/...` route is the test fixture for that work; today the
slot-only path renders, but per-level body content beyond the first
level depends on #440.

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

CI runs `scripts/hydration-smoke.mjs` against `/`, `/longlist`, and
`/conditional` after building the playground with the client bundle.
The script loads each route in headless Chromium, waits for
`window.__sveltego_hydrated`, and fails on Svelte `hydration_mismatch`
/ `hydration_attribute_changed` warnings. To run locally:

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

