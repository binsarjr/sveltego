# adapter-cloudflare

Cloudflare Workers deploy adapter. **Currently a stub** — `Build` returns `ErrNotImplemented`.

## Why blocked

Cloudflare's Go runtime is too restricted for sveltego today:

- No full `net/http` server; only a `fetch`-style entry.
- ~1MB compressed script size cap (typical sveltego binary is 8–12MB).
- Restricted stdlib (no `os.Open`, no goroutines beyond the fetch handler scope).

Two paths forward, neither in scope yet:

- **TinyGo + WASI shim** — compile to WASM, run under Workers' WASM runtime. Requires reflect-free codegen.
- **Pages Functions (Node)** — out of scope per [ADR 0005](../../tasks/decisions/0005-non-goals.md) (Node-only platform).

## Workaround

Deploy with `--target=docker` and put Cloudflare in front as a CDN. Workers KV / R2 are still attachable from any origin.

Track:

- https://github.com/binsarjr/sveltego/issues?q=adapter-cloudflare

Status: pre-alpha (stub). See repo root [`README.md`](../../README.md) and [`STABILITY.md`](./STABILITY.md).
