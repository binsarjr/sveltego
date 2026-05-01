---
title: Snippets
order: 75
summary: Reusable template fragments via {#snippet} and {@render}.
---

# Snippets

Snippets are reusable template fragments declared inside a component. They replace slot fall-throughs from older Svelte versions and compose better with typed props. Standard Svelte 5 syntax — sveltego does not modify it.

## Declaring

```svelte
{#snippet greeting(name)}
  <p>Hello {name}.</p>
{/snippet}
```

A snippet captures parameters by name. Inside the body, parameters are JavaScript values.

## Rendering

```svelte
{@render greeting('world')}
```

`{@render snippet(args...)}` invokes a snippet. Multiple invocations with different arguments produce different output.

## Snippets as props

A component can accept snippets as props:

```svelte
<script lang="ts">
  let { item, data } = $props();
</script>

{#each data.posts as post}
  {@render item(post)}
{/each}
```

The parent passes a snippet:

```svelte
<List {data}>
  {#snippet item(post)}
    <article>
      <h2>{post.title}</h2>
    </article>
  {/snippet}
</List>
```

## Out of scope

`<svelte:fragment>` and named slot syntax are not part of the Svelte 5 surface and are not implemented. Use snippets for every fragment-as-prop case.
