---
title: Snippets
order: 75
summary: Reusable template fragments via {#snippet} and {@render}.
---

# Snippets

Snippets are reusable template fragments declared inside a component. They replace slot fall-throughs from older Svelte versions and compose better with typed props.

## Declaring

```svelte
{#snippet greeting(name string)}
  <p>Hello {name}.</p>
{/snippet}
```

A snippet captures parameters by name. Inside the body, parameters are Go-typed; field access is `{name}`, not `{name.value}` or anything JS-flavored.

## Rendering

```svelte
{@render greeting("world")}
```

`{@render snippet(args...)}` invokes a snippet. Multiple invocations with different arguments produce different output.

## Snippets as props

A component can accept snippets as props:

```svelte
<script lang="go">
  type Props struct {
    Item func(p Post)
  }
  var p = $props[Props]()
</script>

{#each Data.Posts as post}
  {@render p.Item(post)}
{/each}
```

The parent passes a snippet:

```svelte
<List Item={#snippet (post Post)}
  <article>
    <h2>{post.Title}</h2>
  </article>
{/snippet} />
```

## Out of scope

`<svelte:fragment>` and named slot syntax are not part of the Svelte 5 surface and are not implemented. Use snippets for every fragment-as-prop case.
