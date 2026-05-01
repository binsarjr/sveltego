# ssr-stress playground

End-to-end SSR coverage stress test. Each route exercises a different
Svelte 5 server-output shape so the Phase 7 corpus + Phase 8 fallback
work has concrete fixtures to measure against.

## Routes

| Path | What it covers |
|---|---|
| `/` | Hello-world: minimal Render with one interpolation. |
| `/longlist` | 100-item `{#each}` — exercises emit-shape volume + escape loop. |
| `/conditional` | `{#if}/{:else}` over scalar fields. |
| `/layoutdeep/level2/level3` | 3-level layout chain with body content per level. |

## Known incomplete shapes

- `{#snippet}` + `{@render}`: the Phase 5 lowerer does not currently
  hoist the snippet definition into the surrounding scope. The
  generated Go references an undefined `ssvar_renderer`. Tracked as a
  Phase 5/8 follow-up.
- `{@const}` at template top level: emits `if data { ... }` against a
  typed struct value, which is non-boolean in Go. Awaiting a lowerer
  guard for truthy-struct conditions.

## Snapshots

`testdata/snapshots/<route>.html` — the canonical SSR HTML body for
the route. Used by smoke tests to catch regressions.

## Run

```bash
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app --no-client
./build/app  # listens on :3000
curl -s localhost:3000/
```
