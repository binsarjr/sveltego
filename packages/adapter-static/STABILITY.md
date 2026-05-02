# Stability — adapter-static

Last updated: 2026-05-02 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Pre-`v0.1` every export is implicitly experimental.

## Stable

(none yet)

## Experimental

- `adapterstatic.Build` — drives prerender + packaging end-to-end (#447)
- `adapterstatic.BuildContext`
- `adapterstatic.Runner` — pluggable prerender driver
- `adapterstatic.RunInfo`
- `adapterstatic.ErrDynamicRoutes`
- `adapterstatic.PrerenderManifestFilename`
- `adapterstatic.DefaultMainPackage`
- `adapterstatic.Doc`
- `adapterstatic.Name`

## Deprecated

- `adapterstatic.ErrNotImplemented` — removed when the adapter shipped
  in #447. Callers that wrapped the sentinel must update to the new
  validation-error surface.

## Internal-only (do not import even though exported)

(none yet)
