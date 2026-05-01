---
title: Form actions
order: 40
summary: POST handlers tied to a page — default and named actions, validation, redirect.
---

# Form actions

Form actions are POST handlers attached to a page. They live in `_page.server.go` next to `Load` and run on `POST` to the page route.

## Shape

```go
package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

var Actions = kit.ActionMap{
  "default": func(ev *kit.RequestEvent) kit.ActionResult {
    if err := ev.Request.ParseForm(); err != nil {
      return kit.ActionFail(400, kit.M{"error": err.Error()})
    }
    name := ev.Request.PostForm.Get("name")
    if name == "" {
      return kit.ActionFail(422, kit.M{"name": "required"})
    }
    // ... persist
    return kit.ActionRedirect(303, "/thanks")
  },
}
```

The dispatcher reads the request's `?/<name>` query and looks up the matching key in `Actions`. Absent query → `"default"`.

`_page.server.go` does not need a `//go:build sveltego` tag — the `_` prefix already hides the file from Go's default toolchain.

## Results

| Constructor | Type | Effect |
|---|---|---|
| `kit.ActionDataResult(code, data)` | `ActionData` | Re-render with `data` exposed via the `form` prop. |
| `kit.ActionFail(code, data)` | `ActionFailData` | Re-render with failure status (typically 4xx). |
| `kit.ActionRedirect(code, url)` | `ActionRedirectResult` | Redirect (default 303). |

The `ActionResult` interface is sealed: only the three result types satisfy it. Construct via the helpers above.

## Form data inside the template

After an action runs, the page re-renders with the action's data exposed via the `form` prop alongside `data`:

```svelte
<script lang="ts">
  let { data, form } = $props();
</script>

<form method="POST">
  <input name="name" />
  <button>Submit</button>
  {#if form?.error}
    <p class="error">{form.error}</p>
  {/if}
</form>
```

Field names follow the JSON tags on whatever the action returned. `kit.M{"error": "..."}` becomes `form.error` on the client.

## Multiple actions

Name them by query parameter:

```go
var Actions = kit.ActionMap{
  "create": createAction,
  "delete": deleteAction,
}
```

Submit to `?/create` or `?/delete`:

```svelte
<form method="POST" action="?/delete">
  <input type="hidden" name="id" value={data.id} />
  <button>Delete</button>
</form>
```

## Progressive enhancement

Actions degrade gracefully without JavaScript: a plain `<form method="POST">` works. Use the standard SvelteKit `use:enhance` action on the client for AJAX-style submissions (lands with the v0.3 client bundle work — track issue #34).
