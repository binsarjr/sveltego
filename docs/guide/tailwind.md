---
title: Tailwind CSS
order: 75
summary: Add Tailwind CSS to a sveltego project — v4 quickstart, content globs, scoped-CSS coexistence, and common pitfalls.
---

# Tailwind CSS

Tailwind integrates with sveltego through Vite's plugin system. The server binary does not need Tailwind at runtime; Vite handles everything at build time.

## Quickstart (Tailwind v4, preferred)

```sh
sveltego init --tailwind
```

The `--tailwind` flag (tracked in [#207](https://github.com/binsarjr/sveltego/issues/207)) wires up the Vite plugin and inserts the `@source` directive automatically. Until that flag ships, follow the manual steps below.

### Manual setup

```sh
npm install -D @tailwindcss/vite
```

`vite.config.ts`:

```ts
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [tailwindcss()],
});
```

`src/app.css`:

```css
@import "tailwindcss";
@source "../src/**/*.{svelte,go}";
```

Import the stylesheet in your root layout:

```svelte
<!-- src/routes/+layout.svelte -->
<script lang="go">
  import "../app.css";
</script>

<slot />
```

## Content globs

sveltego routes are `.svelte` templates plus `.go` server files. Both can contain class strings that Tailwind must scan.

**Tailwind v4** uses the `@source` directive inside CSS:

```css
@import "tailwindcss";
@source "../src/**/*.{svelte,go}";
```

**Tailwind v3** uses the `content` array in `tailwind.config.js`:

```js
/** @type {import('tailwindcss').Config} */
export default {
  content: ['./src/**/*.{svelte,go}'],
  theme: { extend: {} },
  plugins: [],
};
```

The `.go` glob matters: class names assembled inside `Load` or action handlers are invisible to Tailwind's scanner unless the source files are included.

## Tailwind v3 fallback

If you need v3 (e.g. a plugin that hasn't been ported to v4):

```sh
npm install -D tailwindcss postcss autoprefixer
npx tailwindcss init -p
```

`vite.config.ts` with PostCSS:

```ts
import { defineConfig } from 'vite';

export default defineConfig({
  // Vite picks up postcss.config.js automatically; no explicit plugin needed.
});
```

`postcss.config.js`:

```js
export default {
  plugins: {
    tailwindcss: {},
    autoprefixer: {},
  },
};
```

`tailwind.config.js` (content array as shown above).

## Coexistence with sveltego scoped CSS

sveltego's codegen (Phase 0dd / [#54](https://github.com/binsarjr/sveltego/issues/54)) applies a per-component scope hash to selectors in `<style>` blocks. Tailwind utility classes pass straight through — they live in the global stylesheet imported in the layout, not inside a component `<style>` block.

| Where | Scope hash applied? | Notes |
|---|---|---|
| `<style>` block in `.svelte` | Yes | Selectors get a `[data-sveltego-HASH]` attribute selector suffix. |
| Tailwind utility class in markup | No | Class names are global; Tailwind's stylesheet is not hashed. |
| `@apply` inside `<style>` | Yes | The expanded rule inherits the scope hash just like any other rule in the block. |

**`@apply` caveat.** When you use `@apply` inside a scoped `<style>` block, the resulting CSS rule is scoped, but the utility still resolves against the global Tailwind configuration. That is the expected behavior, but it means the rule will only match elements inside the component, not siblings. If you need a global override, write it in `app.css` instead.

**Ordering.** The Tailwind stylesheet is injected before component styles by default. Component `<style>` declarations therefore take precedence over utilities with equal specificity — standard CSS cascade applies.

## Common pitfalls

### Dynamic class strings are purged

Tailwind's scanner looks for complete class names as static strings. A class built at runtime will be purged in production:

```go
// Bad — "text-red-500" never appears as a literal; Tailwind purges it.
color := "red"
cls := "text-" + color + "-500"
```

```go
// Good — use a full literal string; branch on it in Go.
var cls string
if danger {
    cls = "text-red-500"
} else {
    cls = "text-gray-700"
}
```

Alternatively, add the class to the safelist in your config:

```css
/* Tailwind v4 — safelist in CSS */
@source unsafe "../src/**/*.go";
```

```js
// Tailwind v3 — safelist in tailwind.config.js
safelist: ['text-red-500', 'text-orange-500'],
```

### Dev vs production purge

In development (Vite dev server), Tailwind v4's `@tailwindcss/vite` plugin scans on demand and does not purge. In the production build (`sveltego build` → `vite build`), only classes found in the `@source` glob are emitted. Run a production build before shipping to catch any missing classes.

### Dark mode

```css
/* Tailwind v4 — variant strategy */
@import "tailwindcss";
@variant dark (&:where(.dark, .dark *));
```

```js
// Tailwind v3 — darkMode in config
darkMode: 'class',
```

sveltego has no built-in dark-mode helper. Toggle a `dark` class on `<html>` from a client-side `<script>` or from a layout server action that sets a cookie — either approach works.

## FAQ

**Can I use Tailwind v3 and v4 at the same time?**
No. They use different plugin architectures. Pick one per project.

**How do I opt out of Tailwind after running `sveltego init --tailwind`?**
Remove `@tailwindcss/vite` from `vite.config.ts` and `@import "tailwindcss"` from your CSS. No sveltego-specific cleanup is needed.

**Does the scope hash affect Tailwind's JIT output?**
No. The scope hash is written into the generated HTML attributes and the component `<style>` block CSS, not into utility class names. JIT output is unaffected.

## Migration from SvelteKit + Tailwind

If you have an existing SvelteKit project with Tailwind, the Vite plugin wiring is identical. The only change required is updating the content glob to include `.go` files:

```diff
- content: ['./src/**/*.{svelte,ts}'],
+ content: ['./src/**/*.{svelte,go}'],
```

For v4 projects using `@source`, swap the extension list the same way. Everything else — your `tailwind.config.js`, `postcss.config.js`, and component markup — carries over unchanged.
