# Stability — adapter-fastly

Last updated: 2026-04-30 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Pre-`v0.1` every export is implicitly experimental; this file populates as APIs land.

## Stable

(none yet)

## Experimental

- `adapterfastly.Build` — orchestrates TinyGo compilation; returns ErrTinyGoMissing when TinyGo is absent
- `adapterfastly.BuildContext`
- `adapterfastly.Doc`
- `adapterfastly.Name`
- `adapterfastly.ErrTinyGoMissing`

## Deprecated

(none yet)

## Internal-only (do not import even though exported)

- `internal/fsutil` — file-copy helpers, not part of the public API
