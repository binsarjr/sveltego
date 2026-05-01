# Stability — sveltego

Last updated: 2026-05-01

This is the index of per-package stability promises. Tier definitions and the breaking-change procedure live in [`CONTRIBUTING.md`](./CONTRIBUTING.md#stability-tiers) and the source RFC [#97](https://github.com/binsarjr/sveltego/issues/97).

## Per-package stability

| Package | Path | STABILITY |
|---|---|---|
| `sveltego` (core) | `packages/sveltego` | [STABILITY.md](./packages/sveltego/STABILITY.md) |
| `adapter-auto` | `packages/adapter-auto` | [STABILITY.md](./packages/adapter-auto/STABILITY.md) |
| `adapter-cloudflare` | `packages/adapter-cloudflare` | [STABILITY.md](./packages/adapter-cloudflare/STABILITY.md) |
| `adapter-docker` | `packages/adapter-docker` | [STABILITY.md](./packages/adapter-docker/STABILITY.md) |
| `adapter-lambda` | `packages/adapter-lambda` | [STABILITY.md](./packages/adapter-lambda/STABILITY.md) |
| `adapter-server` | `packages/adapter-server` | [STABILITY.md](./packages/adapter-server/STABILITY.md) |
| `adapter-static` | `packages/adapter-static` | [STABILITY.md](./packages/adapter-static/STABILITY.md) |
| `enhanced-img` | `packages/enhanced-img` | [STABILITY.md](./packages/enhanced-img/STABILITY.md) |
| `init` | `packages/init` | [STABILITY.md](./packages/init/STABILITY.md) |
| `mcp` | `packages/mcp` | [STABILITY.md](./packages/mcp/STABILITY.md) |
| `lsp` | `packages/lsp` | [STABILITY.md](./packages/lsp/STABILITY.md) |
| `playgrounds/basic` | `playgrounds/basic` | [STABILITY.md](./playgrounds/basic/STABILITY.md) |
| `benchmarks` | `benchmarks` | [STABILITY.md](./benchmarks/STABILITY.md) |

## Repo-wide phase

Pre-`v0.1`. Every export is implicitly experimental until the first release tagged from `release-please` (RFC #100). Adapter-facing API in `packages/sveltego/exports/adapter` becomes `stable` from `adapter-server v0.1` (RFC #97).
