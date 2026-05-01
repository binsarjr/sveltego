# Stability — create-sveltego

Last updated: 2026-05-01 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Pre-`v0.1` every export is implicitly experimental; this file populates as APIs land.

## Stable

(none yet)

## Experimental

- npm CLI: `create-sveltego [flags] <dir>`. The wrapper passes every flag
  through to the underlying `sveltego-init` Go binary verbatim, so any
  flag the binary accepts is in scope.
- Wrapper-only flags / env vars (`--no-binary-download`,
  `SVELTEGO_NO_BINARY_DOWNLOAD`, `SVELTEGO_INIT_LOCAL_PATH`,
  `SVELTEGO_VERSION`, `SVELTEGO_CACHE_DIR`).

## Deprecated

(none yet)

## Internal-only (do not import even though exported)

(none yet)
