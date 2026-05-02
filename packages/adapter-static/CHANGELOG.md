# Changelog — adapter-static

All notable changes to this package will be documented in this file. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Added
- `Build` now drives sveltego's `Server.Prerender` engine end-to-end and
  packages the result into a flat deploy tree under `OutputDir`. The
  adapter spawns the user binary with `SVELTEGO_PRERENDER=1` by default;
  callers can inject a custom `Runner` to drive prerender in-process.
- `BuildContext.FailOnDynamic` returns the new typed `ErrDynamicRoutes`
  when any non-prerenderable route is reported.
- `BuildContext.MainPackage`, `BuildContext.ScratchDir`, and
  `BuildContext.Stdout`/`Stderr` for build-pipeline customization.
- `_prerender_manifest.json` at the root of `OutputDir` summarizing the
  tree with per-entry SHA256 hashes. The manifest content is
  deterministic so repeat builds produce byte-identical output.
- Static `static/` directory is mirrored into `OutputDir/static/`
  (excluding the runtime `_prerendered` artifact).

### Removed
- `ErrNotImplemented` sentinel. The adapter is no longer a stub.

### References
- Closes #447 (adapter-static wiring).
- Builds on #65 (prerender engine).
