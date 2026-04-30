# Stability — dashboard

Last updated: 2026-04-30 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Pre-`v0.1` every export is implicitly experimental; this file populates as APIs land.

The `dashboard` playground is a demonstration app, not a published API. None of its types or functions are exported under a stability tier; consumers are expected to copy patterns into their own apps rather than depend on this module.

## Stable

(none — playground is illustrative only)

## Experimental

(none)

## Deprecated

(none)

## Internal-only (do not import even though exported)

- Everything under `playgrounds/dashboard/src/lib`. The in-memory store is illustrative; replace with a real persistence layer in production code.
