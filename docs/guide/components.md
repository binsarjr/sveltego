---
title: Components
order: 70
summary: Svelte 5 runes — $props, $state, $derived, $effect, $bindable.
---

# Components

sveltego targets **Svelte 5 only**. Legacy Svelte 4 reactivity (`$:`, store auto-subscriptions, `export let`) is not supported. Use runes.

## Props

```svelte
<script lang="go">
  type Props struct {
    Name  string
    Count int
  }
  var p = $props[Props]()
</script>

<p>Hello {p.Name}, {p.Count} unread.</p>
```

`$props[T]()` returns a typed value. Field access in the template (`{p.Name}`) is a Go expression: PascalCase, no JS pattern destructuring at the prop boundary.

## State

```svelte
<script lang="go">
  var count = $state(0)
</script>

<button onclick={func() { count++ }}>
  Clicked {count} times
</button>
```

`$state[T](initial)` creates a reactive cell. Reads and writes are via the cell value; the runtime tracks dependencies for re-render on the client.

## Derived

```svelte
<script lang="go">
  var count = $state(0)
  var doubled = $derived(count * 2)
</script>

<p>{doubled}</p>
```

`$derived(expr)` recomputes when its dependencies change. Pure expressions only — no I/O.

## Effects

```svelte
<script lang="go">
  var count = $state(0)
  $effect(func() {
    log("count changed to", count)
  })
</script>
```

`$effect(fn)` runs after the DOM updates and re-runs when its dependencies change. Use for side effects that must observe state.

## Bindable

```svelte
<script lang="go">
  type Props struct {
    Value string
  }
  var p = $bindable[Props]()
</script>

<input bind:value={p.Value} />
```

`$bindable[T]()` declares a two-way binding. The parent component supplies and updates the bound value through the same field.

## Mustache expressions

Inside `{...}` you write Go:

```svelte
{Data.User.Name}
{len(Data.Posts)}
{strings.ToUpper(Data.Title)}
{#if Data.Posts != nil && len(Data.Posts) > 0}
  ...
{/if}
{#each Data.Posts as post}
  <li>{post.Title}</li>
{/each}
```

Validated at codegen via `go/parser.ParseExpr`. Type errors surface at `sveltego build`, not runtime.
