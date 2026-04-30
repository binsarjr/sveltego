# adapter-static

Static-site (SSG) deploy adapter. **Currently a stub** — `Build` returns `ErrNotImplemented` because the adapter depends on prerender mode (issue #65), which has not shipped yet.

The exported types (`BuildContext`, `Build`, `Doc`, `ErrNotImplemented`) are stable so callers can wire the adapter today and have it start working when #65 lands.

## Workaround

Until #65 ships:

- Use `--target=server` and place a CDN (Cloudflare, Bunny, Fastly) in front of the binary.
- Or render selected routes manually via `httptest` and ship the bytes to S3 / Pages / Netlify Edge.

Track:

- [#65 — prerender mode](https://github.com/binsarjr/sveltego/issues/65)

Status: pre-alpha (stub). See repo root [`README.md`](../../README.md) and [`STABILITY.md`](./STABILITY.md).
