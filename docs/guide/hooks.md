---
title: Hooks
order: 50
summary: Handle, HandleError, HandleFetch, Reroute, Init — request lifecycle hooks.
---

# Hooks

`src/hooks._server.go` exports five optional hooks. Generated code wires the missing ones to identity defaults so you only write what you need.

```go
//go:build sveltego

package hooks

import (
  "context"
  "net/http"
  "net/url"

  "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
  // pre: read cookie, set ev.Locals["user"]
  res, err := resolve(ev)
  // post: mutate response headers
  return res, err
}

func HandleError(ev *kit.RequestEvent, err error) kit.SafeError {
  return kit.SafeError{Code: 500, Message: "something went wrong"}
}

func HandleFetch(ev *kit.RequestEvent, req *http.Request) (*http.Response, error) {
  return http.DefaultClient.Do(req)
}

func Reroute(u *url.URL) string {
  return ""
}

func Init(ctx context.Context) error {
  return nil
}
```

## Handle

`Handle` wraps the entire request pipeline. Call `resolve(ev)` to advance to route resolution. Return without calling `resolve` to short-circuit.

Compose multiple handlers with `kit.Sequence`:

```go
var Handle = kit.Sequence(authHandle, requestIDHandle, telemetryHandle)
```

`Sequence` runs handlers left-to-right; each one's `resolve` advances to the next. Returning early from any handler short-circuits the rest.

## HandleError

Called whenever `Handle`, `Load`, render, or a `_server.go` handler returns an error. Returns a `kit.SafeError` — the user-facing contract: `Code`, `Message`, `ID`. Logs the raw error; the response body never echoes internal detail. When `Message` is empty, the framework substitutes `http.StatusText(Code)`.

## HandleFetch

Intercepts outbound HTTP from `Load` and `_server.go`. Use it to add auth headers, redirect to local services, or apply request-scoped retries.

`RequestEvent.Fetch(req)` dispatches through `HandleFetch`; bypass it (e.g. `http.DefaultClient.Do`) and the hook is silently skipped.

## Reroute

Runs before route matching. Return a non-empty path to rewrite the URL used for lookup. `ev.URL` is preserved as the original; `ev.MatchPath` carries the rewrite. Empty string = no rewrite.

```go
func Reroute(u *url.URL) string {
  if u.Path == "/old-path" {
    return "/new-path"
  }
  return ""
}
```

## Init

Runs once at server start, before the first request. Use for warmup: open DB pools, prime caches, register metrics. Returning a non-nil error aborts startup.

## Defaults

`kit.DefaultHooks()` returns identity defaults for every field. `kit.Hooks{}.WithDefaults()` fills in the missing ones — idempotent, called by generated wiring code.
