# bench/payload-spike

Reproducible hydration-payload comparison between sveltego and SvelteKit. Closes [#315](https://github.com/binsarjr/sveltego/issues/315).

## Run

```sh
bench/payload-spike/run.sh
```

The script:

1. Generates two minimal apps inline at `bench/payload-spike/.run/{sveltego-app,sveltekit-app}/` — same routes, same data shape, same template output.
2. Builds both (sveltego compile + Vite + Go build; SvelteKit Vite + adapter-node).
3. Boots each server, curls `/`, `/list`, `/post/1`.
4. Computes plain / gzip -9 / brotli -q 11 byte counts.
5. Estimates transfer time at 3G / 4G / fiber.
6. Writes the report to `tasks/spikes/2026-05-03-hydration-payload-size.md`.

The `.run/` tree is gitignored — `tasks/spikes/2026-05-03-hydration-payload-size.md` is the only commit-relevant artifact the script produces.

## Requirements

- Go 1.25+ (matches the repo's toolchain pin).
- Node 20+ and npm in `$PATH`.
- `curl`, `gzip`, `brotli`, `awk`, `python3`.

The repo already exercises this toolchain via the existing playgrounds; no new dependencies were added for the spike.

## Pinned upstream versions

The script hard-codes the SvelteKit / Svelte / vite / adapter-node versions so future runs reproduce the same byte counts. Bumping any of them is a deliberate doc-update event:

| Package | Version |
|---|---|
| @sveltejs/kit | 2.59.0 |
| svelte | 5.55.5 |
| @sveltejs/adapter-node | 5.5.4 |
| @sveltejs/vite-plugin-svelte | 7.0.0 |
| vite | 8.0.10 |

## Compromises documented in the report

- The sveltego app uses the `<!-- sveltego:ssr-fallback -->` annotation so SSR runs through the Node sidecar (the basic playground's path today). The build-time JS-to-Go transpile route emits the same wire shape, so the comparison is representative either way.
- Both apps render with a bare layout — no shared chrome, no Tailwind. The diff is meant to isolate framework bytes; styling overhead applies to both stacks identically.
- Detail-page body is three lorem paragraphs; list page is 20 rows. Adjust the constants in `run.sh` if a heavier fixture is needed.
