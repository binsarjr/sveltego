# Hydration payload size — sveltego vs SvelteKit

Spike for [#315](https://github.com/binsarjr/sveltego/issues/315). Apples-to-apples on-the-wire byte comparison between sveltego and SvelteKit, same input fixture.

**Generated:** 2026-05-02T23:49:31Z

**Reproduce:** `bench/payload-spike/run.sh` (see [README](../../bench/payload-spike/README.md)).

## Setup

- Two minimal apps generated inline by the bench script — same routes, same data, same template shape.
- Three pages compared: root greeting, list of 20 posts, single post detail.
- Both servers boot locally; `curl` captures the raw HTML response.
- Compression: `gzip -9` and `brotli -q 11`.
- Transfer-time estimates assume an empty pipe; ignore TCP/TLS handshake and queueing.

### Pinned upstream versions

| Package | Version |
|---|---|
| @sveltejs/kit | 2.59.0 |
| svelte | 5.55.5 |
| @sveltejs/adapter-node | 5.5.4 |
| @sveltejs/vite-plugin-svelte | 7.0.0 |
| vite | 8.0.10 |

Bumping any of these requires re-running the spike and updating this report.

### Compromises

- Sveltego routes carry the `<!-- sveltego:ssr-fallback -->` annotation so SSR runs through the Node sidecar (the path the basic playground exercises today). The build-time JS-to-Go transpile route emits the same wire shape; this spike does not switch between them.
- Both apps render with a bare layout chain (no shared chrome). Tailwind / styling is omitted from both sides so the diff isolates the framework's own bytes.
- Detail-page body uses three repetitions of the standard lorem paragraph; list page uses 20 rows. Numbers scale linearly — adjust if a heavier fixture is needed for follow-up work.

## Byte counts

Sizes in bytes. "plain" is the raw HTTP response body; "gz" is gzip -9; "br" is brotli -q 11.

### Root (`/`)

| stack | plain | gz | br |
|---|---:|---:|---:|
| sveltego  | 942  | 551  | 396  |
| sveltekit | 926 | 558 | 423 |

### List (`/list`, 20 posts)

| stack | plain | gz | br |
|---|---:|---:|---:|
| sveltego  | 8436  | 962  | 725  |
| sveltekit | 8247 | 954 | 733 |

### Detail (`/post/1`)

| stack | plain | gz | br |
|---|---:|---:|---:|
| sveltego  | 1622  | 620  | 473  |
| sveltekit | 1587 | 615 | 477 |

## Transfer estimates (brotli, ms)

Time to push the body across the wire at the given throughput. Lower is better. Numbers ignore TCP/TLS handshake.

| route | stack | 3G (1.6 Mbps) | 4G (12 Mbps) | fiber (100 Mbps) |
|---|---|---:|---:|---:|
| root | sveltego | 2.0 | 0.3 | 0.0 |
| root | sveltekit | 2.1 | 0.3 | 0.0 |
| list | sveltego | 3.6 | 0.5 | 0.1 |
| list | sveltekit | 3.7 | 0.5 | 0.1 |
| detail | sveltego | 2.4 | 0.3 | 0.0 |
| detail | sveltekit | 2.4 | 0.3 | 0.0 |

## Diff observations

Eyeballed the captured responses under `bench/payload-spike/.run/results/`. Patterns that explain the byte counts:

1. **Both stacks duplicate `data` once** — once in the rendered HTML, once in the hydration bridge. That's the unavoidable SSR-then-hydrate cost; it scales linearly with `Load()` return size and dominates the list page (the 20-row payload is the lion's share of both responses).
2. **Sveltego's hydration JSON carries per-request boilerplate that repeats on every route.** The bridge ships `routeId`, `data`, `form`, `url`, `params`, `status`, `error`, `manifest`, `appVersion`, `versionPoll` — and three of those (`manifest`, `appVersion`, `versionPoll`) are byte-identical for every page. On `/` (the smallest payload) these three fields are **257 of 370 JSON bytes (69%)**. SvelteKit ships none of that inline — its router manifest and version-poll config land in `start.<hash>.js` and the kit chunks, hashed-cached, downloaded once per visit.
3. **Sveltego puts `<link rel="modulepreload">` and `<script type="module" src="...">` inline in `<head>` on every render.** SvelteKit instead emits a single inline `<script>` block at the bottom of `<body>` that does `Promise.all([import(start), import(app)])`. Both ship the same hashed chunks; the wire shapes balance out to within ~30 bytes per page.
4. **SvelteKit's hydration markers are heavier than sveltego's.** The wrapping `<!--[--><!--[0--><!--[--><!--[-->...<!--]--><!--]--><!--]-->` (used by Svelte 5 to track block boundaries for hydration) costs ~40-60 bytes per page that sveltego's stripped-down `<!--[-->...<!--]--><!--]-->` does not pay. This is the only line where sveltego wins on raw HTML markup.
5. **Whitespace/JSON minification is already tight.** Sveltego's JSON has no spaces and quoted keys; SvelteKit emits an ad-hoc JS object literal (unquoted keys, also no spaces). Equivalent on the wire. No low-hanging fruit here.
6. **No data echoed in two formats.** Sveltego does not emit a duplicate JS literal alongside the JSON — only one bridge. SvelteKit does not double either. Concern raised in the issue background ("data echoed twice between HTML and JSON") does not apply to either stack today.

The compressors collapse most of (1) and (2): per-route data is highly repeating, and `manifest`/`appVersion`/`versionPoll` show up identically every request, so gzip/brotli amortise them. That's why the brotli numbers across all three pages are within 27 bytes of each other.

## Verdict

**Sveltego is competitive with SvelteKit today — within ~6% on plain bytes, within ~3% on brotli, faster on the smallest page.** No payload regression to chase.

The single actionable observation: **`manifest` + `appVersion` + `versionPoll` are inlined into every hydration payload but are byte-identical across routes.** They're 69% of the JSON on the smallest page, and they persist after compression on the first request. Move them out of the per-page `<script type="application/json">` and into a hashed JS chunk imported once per session (mirroring SvelteKit's `start.<hash>.js`) and the empty-page payload halves. Tracked as follow-up work for whichever post-MVP perf milestone owns hydration tuning.

## Raw responses

Captured HTML for each route is preserved under `bench/payload-spike/.run/results/` (gitignored).
