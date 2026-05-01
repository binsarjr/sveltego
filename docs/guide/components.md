---
title: Components
order: 70
summary: Svelte 5 runes — $props, $state, $derived, $effect, $bindable. Pure Svelte/JS/TS.
---

# Components

sveltego targets **Svelte 5 only**. Legacy Svelte 4 reactivity (`$:`, store auto-subscriptions, `export let`) is not supported. Use runes.

`.svelte` files are 100% pure Svelte/JS/TS — the same syntax SvelteKit uses. The `data` prop arrives from `_page.server.go`'s `Load` (Go side); JSON tags drive the Go-to-TypeScript field mapping, so a Go field `User User \`json:"user"\`` shows up as `data.user` in the template.

## Props

```svelte
<script lang="ts">
  let { name, count } = $props();
</script>

<p>Hello {name}, {count} unread.</p>
```

For typed props, use a TypeScript type:

```svelte
<script lang="ts">
  type Props = { name: string; count: number };
  let { name, count }: Props = $props();
</script>
```

For pages, codegen emits a sibling `_page.svelte.d.ts` describing `data`'s shape (built from your `_page.server.go` `Load` return type), so Svelte LSP autocompletes `data.*` end to end with no manual type annotation.

## State

```svelte
<script lang="ts">
  let count = $state(0);
</script>

<button onclick={() => count++}>
  Clicked {count} times
</button>
```

`$state(initial)` creates a reactive cell. Reads and writes look like normal variables; the Svelte compiler rewrites them into reactivity primitives.

## Derived

```svelte
<script lang="ts">
  let count = $state(0);
  let doubled = $derived(count * 2);
</script>

<p>{doubled}</p>
```

`$derived(expr)` recomputes when its dependencies change. Pure expressions only — no I/O.

## Effects

```svelte
<script lang="ts">
  let count = $state(0);
  $effect(() => {
    console.log('count changed to', count);
  });
</script>
```

`$effect(fn)` runs after the DOM updates and re-runs when its dependencies change. Use for side effects that must observe state.

## Bindable

```svelte
<script lang="ts">
  let { value = $bindable() } = $props();
</script>

<input bind:value />
```

`$bindable()` declares a two-way binding. The parent component supplies and updates the bound value through the same prop.

## Template expressions

Inside `{...}` you write standard JavaScript / TypeScript expressions over the `data` prop and any local state:

```svelte
{data.user.name}
{data.posts.length}
{data.title.toUpperCase()}
{#if data.posts && data.posts.length > 0}
  ...
{/if}
{#each data.posts as post}
  <li>{post.title}</li>
{/each}
```

Field names use camelCase by convention (driven by JSON tags on the Go side). `null` not `nil`. Empty slices serialize as JSON `[]` so `data.posts.length` is always defined.

See [ADR 0008](https://github.com/binsarjr/sveltego/blob/main/tasks/decisions/0008-pure-svelte-pivot.md) for the rationale behind pure-Svelte templates.
