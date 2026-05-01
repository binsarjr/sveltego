---
title: Errors
order: 60
summary: kit.Error, kit.Redirect, kit.Fail, _error.svelte, SafeError contract.
---

# Errors

sveltego uses idiomatic Go error returns. No panic-as-control-flow. Three sentinel constructors cover the common cases.

## Sentinels

| Constructor | Type | Used for |
|---|---|---|
| `kit.Error(code, msg)` | `*HTTPErr` | HTTP error short-circuit (404, 500, ...). |
| `kit.Redirect(code, location)` | `*RedirectErr` | Redirect (303 POST→GET, 307/308 method-preserving). |
| `kit.Fail(code, data)` | `*FailErr` | Form action validation failure with form-bound data. |

All three implement `error` and `httpStatuser`. The pipeline detects them and writes the appropriate response.

## Returning from Load

```go
func Load(ctx *kit.LoadCtx) (PageData, error) {
  user, err := authUser(ctx.Request)
  if err != nil {
    return PageData{}, kit.Redirect(303, "/login")
  }
  if !user.IsAdmin {
    return PageData{}, kit.Error(403, "admins only")
  }
  return PageData{User: user}, nil
}
```

## Error boundary

`_error.svelte` catches errors from any descendant page or layout. The nearest `_error.svelte` walking up from the failing route is rendered.

```svelte
<script lang="go">
  type PageData struct {
    Error kit.SafeError
  }
</script>

<h1>{Data.Error.Code}: {Data.Error.Message}</h1>
{#if Data.Error.ID != ""}
  <p>Reference: {Data.Error.ID}</p>
{/if}
```

## SafeError

`HandleError` produces a `kit.SafeError` — the user-facing contract:

```go
type SafeError struct {
  Code    int
  Message string
  ID      string
}
```

The pipeline never exposes the raw error to the client. Log the raw error inside `HandleError`; surface only `SafeError` in the boundary.

## Default behavior

If no `HandleError` is defined, `kit.IdentityHandleError` returns a generic 500 with no body detail. Set one in `hooks.server.go` to customize:

```go
func HandleError(ev *kit.RequestEvent, err error) kit.SafeError {
  id := correlationID(ev)
  slog.ErrorContext(ev.Request.Context(), "request failed", "err", err, "id", id)
  return kit.SafeError{Code: 500, Message: "internal error", ID: id}
}
```
