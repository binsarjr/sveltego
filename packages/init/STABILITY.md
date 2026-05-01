# Stability — init

Last updated: 2026-05-01 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Pre-`v0.1` every export is implicitly experimental; this file populates as APIs land.

## Stable

(none yet)

## Experimental

- `sveltego-init` CLI (`packages/init/cmd/sveltego-init`). Flag surface: `--ai`, `--force`, `--non-interactive`, `--module`, `--tailwind=v4|v3|none`, `--service-worker`. Invoked via `go run github.com/binsarjr/sveltego/packages/init/cmd/sveltego-init@latest`.

## Deprecated

(none yet)

## Internal-only (do not import even though exported)

- `internal/scaffold` and `internal/aitemplates` — reserved for the CLI; no compatibility promise.
