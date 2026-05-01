---
title: Pure Svelte templates
date: 2026-05-01
summary: 100% pure Svelte/JS/TS in .svelte files. Server-side data is Go; codegen emits TypeScript declarations for autocomplete.
---

# Pure Svelte templates

Templates inside `.svelte` files are **100% pure Svelte/JS/TS** — the same
syntax SvelteKit uses. That means:

- `{data.user.name}` — camelCase, JavaScript expressions.
- `{data.posts.length}` — standard `.length`, not `len(...)`.
- `null` and `undefined`, not `nil`.
- `{#if data.authed}...{/if}` — JavaScript truthiness.
- `let { data } = $props();` in a `<script lang="ts">` block.

Server-side data lives in Go, in `_page.server.go` next to the template.
Codegen reads the `Load` return type and emits a sibling
`_page.svelte.d.ts` declaration so Svelte LSP autocompletes `data.*` end
to end. JSON tags drive the field names visible from the template.

See [ADR 0008](https://github.com/binsarjr/sveltego/blob/main/tasks/decisions/0008-pure-svelte-pivot.md)
for the rationale and the full pivot from the legacy Mustache-Go dialect.
