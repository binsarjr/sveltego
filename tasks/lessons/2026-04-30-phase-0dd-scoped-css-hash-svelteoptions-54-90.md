## Phase 0dd — scoped CSS hash + svelte:options (#54 + #90) (2026-04-30)

### Insight

- **Upstream Svelte's CSS hash is DJB2-XOR over UTF-16 code units, base36, processed in reverse.** Found in `packages/svelte/src/utils.js` at `function hash(str)`: strip `
`, init `5381`, fold from end with `((h << 5) - h) ^ charCodeAt(i)`, finalize via `>>> 0`, emit `toString(36)`. Default `cssHash` wraps as `svelte-${hash(filename === '(unknown)' ? css : filename ?? css)}`. The output length is NOT fixed at 5 chars — base36 of a 32-bit value is 1–7 chars depending on magnitude. The issue body's "5-char base36" was a heuristic, not a contract; matching upstream byte-for-byte is what matters. Verified with Node-driven golden vectors hardcoded into the Go test (avoids JS toolchain dep in CI).

