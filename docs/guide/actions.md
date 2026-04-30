---
title: Form actions
order: 40
summary: POST handlers tied to a page — default and named actions, validation, redirect.
---

# Form actions

Form actions are POST handlers attached to a page. They live in `page.server.go` next to `Load` and run on `POST` to the page route.

## Shape

```go
//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

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

## Results

| Constructor | Type | Effect |
|---|---|---|
| `kit.ActionDataResult(code, data)` | `ActionData` | Re-render with `data` exposed via `Form`. |
| `kit.ActionFail(code, data)` | `ActionFailData` | Re-render with failure status (typically 4xx). |
| `kit.ActionRedirect(code, url)` | `ActionRedirectResult` | Redirect (default 303). |

The `ActionResult` interface is sealed: only the three result types satisfy it. Construct via the helpers above.

## Form data inside the template

After an action runs, the page re-renders with `Form` populated. Access in the template as `{Form.error}`, `{Form.name}`, etc., depending on what the action returned.

```svelte
<script lang="go">
  type PageData struct{}
</script>

<form method="POST">
  <input name="name" />
  <button>Submit</button>
  {#if Form != nil}
    <p class="error">{Form.error}</p>
  {/if}
</form>
```

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
  <input type="hidden" name="id" value="{Data.ID}" />
  <button>Delete</button>
</form>
```

## Progressive enhancement

Actions degrade gracefully without JavaScript: a plain `<form method="POST">` works. Client-side enhancement (without full reload) lands with the v0.3 client bundle work — track issue #34.
