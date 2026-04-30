---
title: Svelte 5 runes only
date: 2026-04-22
summary: $props, $state, $derived, $effect, $bindable. Skip Svelte 4 reactivity.
---

# Svelte 5 runes only

We target Svelte 5 explicitly. Runes are first-class:

- `$props()` for component inputs.
- `$state` / `$derived` for client reactivity.
- `$effect` for side effects.
- `$bindable` for two-way binding from a parent.

Legacy `$:` reactivity and store autoload from Svelte 4 are out of
scope. Less syntax, fewer footguns, simpler codegen.
