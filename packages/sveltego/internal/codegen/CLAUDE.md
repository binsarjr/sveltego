# codegen — package notes

## Tailwind coexistence contract

Scoped CSS hashing (`svelte-<hash>`) and Tailwind utility classes are
expected to coexist on the same elements. The codegen contract is:

1. **Utility classes are preserved verbatim.** A static `class="text-3xl
   font-bold"` survives the scope-class rewrite — the scope class is
   appended, never substituted.
2. **`:global()` selectors exit scope.** A `:global(.prose h2)` rule in
   the scoped `<style>` block is emitted byte-for-byte; the scope class
   is not added to its selectors.
3. **`@apply` is opaque to codegen.** The codegen pass does not parse
   `@apply text-zinc-700`. The `<style>` body is emitted verbatim and
   PostCSS / Tailwind processes it at Vite-build time.
4. **Class-token strings inside Go expressions are not rewritten.** A
   `class:bg-blue-500={Active}` keeps `bg-blue-500` as the literal token
   added when `Active` evaluates to true. The scope class is added as a
   sibling token, never injected into the directive name.

Goldens covering each rule live under
`testdata/golden/codegen/tailwind/`. Regenerate with `go test ./internal/codegen/ -args -update`.

## Vite addons

Build-time integrations (currently: Tailwind v4, Tailwind v3) are
detected by reading `package.json` once per build. See `addons.go`. The
`vite.config.gen.js` emitter (`internal/vite/config.go`) accepts the
addon set and emits the right plugin chain.

When an addon is enabled, the emitter additionally adds `src/app.css`
as a Rollup input (key `"app"`) so the manifest reader sees a hashed
CSS asset.
