# Changelog

## 1.0.0 (2026-04-30)


### Features

* **cli:** close Phase 0i — sveltego build end-to-end + $lib alias rewrite ([2c4685d](https://github.com/binsarjr/sveltego/commit/2c4685d54309651ff744c738e15c0cb8ce6b2c87))
* **codegen:** hoist &lt;script lang="go"&gt; and infer PageData from Load() ([dd017f9](https://github.com/binsarjr/sveltego/commit/dd017f950d3586d5a321017f93629f0c0fe4b6c4))
* **kit:** real Cookies impl with secure defaults ([#111](https://github.com/binsarjr/sveltego/issues/111)) ([f97dc33](https://github.com/binsarjr/sveltego/commit/f97dc3363fa95e34dd5fb08e3bb09422f562df3d)), closes [#32](https://github.com/binsarjr/sveltego/issues/32)
* **router:** manifest emitter + built-in matchers + matcher dispatch ([549f406](https://github.com/binsarjr/sveltego/commit/549f40652bf09b936ac00ce081e060290d42280b))
* **router:** radix tree matcher with Static/Param/Optional/Rest segments ([cd31283](https://github.com/binsarjr/sveltego/commit/cd3128303256d152dbe63a6cf0e90e2d130999ed))
* **routescan:** walk src/routes/ and emit scanned route + matcher metadata ([7fa18ba](https://github.com/binsarjr/sveltego/commit/7fa18ba43c235504cb92164ecd8110bf7a9d2733))
* **server:** HTTP pipeline — match, load, render, response ([d005c09](https://github.com/binsarjr/sveltego/commit/d005c099d6b62b56f23c8047c07ade541eb364e0))
* **sveltego:** cobra CLI skeleton with build/compile/dev/check/version ([#5](https://github.com/binsarjr/sveltego/issues/5), [#6](https://github.com/binsarjr/sveltego/issues/6)) ([e9f7263](https://github.com/binsarjr/sveltego/commit/e9f7263664a6625bb2cee4a4f320c661eb47d8df))
* **sveltego:** parser foundation — lexer, ast, recursive-descent parser ([#7](https://github.com/binsarjr/sveltego/issues/7), [#8](https://github.com/binsarjr/sveltego/issues/8)) ([7888eeb](https://github.com/binsarjr/sveltego/commit/7888eebd4e3bf04daea8985a2b3d6206a8274c86))


### Bug Fixes

* **codegen:** PageData alias + pre-commit skip — close MVP via [#23](https://github.com/binsarjr/sveltego/issues/23) ([6d75338](https://github.com/binsarjr/sveltego/commit/6d75338d1f6e6101978571a7b7d9e32068174f51))
* **codegen:** user mirror, manifest adapters, wire emit; rename user .go ([a6bf618](https://github.com/binsarjr/sveltego/commit/a6bf6184c31dce5b13fe8ade95057cf500eaa22c))
* **sveltego:** require + replace test-utils for GOWORK=off CI ([9f38bcc](https://github.com/binsarjr/sveltego/commit/9f38bcc1c3e1e0c9c0ba68df6ec876e7094d9d04))

## Changelog — sveltego

All notable changes to this package will be documented in this file. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Unreleased

### Breaking changes

- **`kit.PageOptions.Templates` defaults to `"svelte"`; the legacy `"go-mustache"` value is rejected at codegen time** (#384, RFC #379 phase 5). Routes that still set `const Templates = "go-mustache"` fail compile with a migration hint. Empty Templates resolves to `"svelte"`; manifests omit the literal for routes on the default. The `kit.TemplatesGoMustache` constant is removed. Migrate `.svelte` bodies to pure Svelte/JS/TS (camelCase JSON-tag access, `$props()`, `null` instead of `nil`) before upgrading. The Mustache-Go codegen path remains in tree for now but is unreachable; follow-up GC sweeps will delete the dead files in waves (`chore(codegen): GC dead Mustache-Go code`).
- **Module path renamed from `github.com/binsarjr/sveltego` to `github.com/binsarjr/sveltego/packages/sveltego`** (#377). The on-disk layout always lived at `packages/sveltego/`, but the declared module path didn't reflect that, breaking `go install github.com/binsarjr/sveltego/cmd/sveltego@latest` from a fresh GOPATH. External imports under `github.com/binsarjr/sveltego/{cmd,exports,internal,render,runtime,server}/...` must update to `github.com/binsarjr/sveltego/packages/sveltego/{cmd,exports,internal,render,runtime,server}/...`. The pre-alpha API tier (per `STABILITY.md`) makes this acceptable, but external consumers must migrate. The dropped `replace github.com/binsarjr/sveltego/test-utils => ../../test-utils` directive (the helper moved to `packages/sveltego/internal/testutils/`) is what unblocks `go install @main` from the proxy.
