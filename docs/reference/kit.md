---
title: kit package
order: 200
summary: Public runtime API — RequestEvent, Hooks, Cookies, Action, CSP, sentinels.
---

# kit

`github.com/binsarjr/sveltego/packages/sveltego/exports/kit` is the public runtime API. Generated code and user route handlers depend on it.

This page summarises the surface. Source is the canonical spec; godoc is generated from it. See `packages/sveltego/exports/kit/` in the repo.

## Contexts

| Type | Used in | Purpose |
|---|---|---|
| `RequestEvent` | hooks, `server.go`, actions | Request-scoped state with `URL`, `Params`, `Locals`, `Cookies`. |
| `RenderCtx` | generated render code | SSR-time context passed into templates. |
| `LoadCtx` | `Load` in `page.server.go`, `layout.server.go` | Load-time context with `Parent()` for layout chain. |

`RequestEvent.Fetch(req)` dispatches outbound HTTP through `HandleFetch`.

`LoadCtx.Header()` returns a `*HeaderWriter` for setting response headers from a loader:

```go
func Load(ctx *kit.LoadCtx) (PageData, error) {
  ctx.Header().Set("Cache-Control", "no-store")
  ctx.Header().Add("Vary", "Accept")
  return PageData{}, nil
}
```

`HeaderWriter` has three methods: `Set(key, value string)` (replace all), `Add(key, value string)` (append), `Del(key string)` (remove all).

`LoadCtx.RawParam(name string) (string, bool)` returns the un-decoded route parameter value (percent-encoding preserved). Use when the decoded value would lose a `%2F` slash inside a segment.

`LoadCtx.Speculative() bool` returns `true` when the request carries `X-Sveltego-Preload: 1` — the client prefetch header. Use to skip expensive side-effects during preloads.

## Hooks

```go
type HandleFn       func(ev *RequestEvent, resolve ResolveFn) (*Response, error)
type HandleErrorFn  func(ev *RequestEvent, err error) SafeError
type HandleFetchFn  func(ev *RequestEvent, req *http.Request) (*http.Response, error)
type RerouteFn      func(u *url.URL) string
type InitFn         func(ctx context.Context) error
```

Compose handlers with `kit.Sequence(...)`. Defaults via `kit.DefaultHooks()` and `Hooks{}.WithDefaults()`.

`InitFn` returning a non-nil error aborts startup; the server emits a `503 Service Unavailable` (or `500` when the HTTP listener could not bind) while the `Init` call is pending.

## Errors and short-circuits

```go
kit.Redirect(code int, location string, opts ...RedirectOption) error
kit.Error(code int, message ...string) error
kit.Fail(code int, data any) error
```

All three implement `error` and `httpStatuser`.

`kit.RedirectReload()` is a `RedirectOption` that signals the client to do a full document reload instead of a SPA navigation:

```go
return PageData{}, kit.Redirect(303, "/login", kit.RedirectReload())
```

`SafeError{Code, Message, ID}` is the user-facing error contract returned from `HandleError`. When `Message` is empty, the framework fills it from `http.StatusText(Code)`.

`kit.HTTPError` is an interface user-defined error types can implement to carry an HTTP status code directly to the pipeline without going through a sentinel:

```go
type HTTPError interface {
  error
  Status() int
  Public() string
}
```

The pipeline inspects returned errors for `HTTPError` before falling back to `HandleError`.

## Responses

```go
kit.NewResponse(status int, body []byte) *Response
kit.JSON(status int, body any) *Response
kit.Text(status int, body string) *Response
kit.XML(status int, body []byte) *Response
kit.NoContent() *Response
kit.MethodNotAllowed(allowed []string) *Response
```

`kit.M` is shorthand for `map[string]any`.

## Form actions

```go
type ActionFn  = func(ev *RequestEvent) ActionResult
type ActionMap = map[string]ActionFn

kit.ActionDataResult(code int, data any) ActionResult
kit.ActionFail(code int, data any) ActionResult
kit.ActionRedirect(code int, location string) ActionResult
```

Result types: `ActionData`, `ActionFailData`, `ActionRedirectResult`. The interface is sealed.

## Cookies

```go
type Cookies struct{ /* ... */ }
type CookieOpts struct {
  Path     string
  Domain   string
  MaxAge   time.Duration
  Expires  time.Time
  HttpOnly *bool
  Secure   *bool
  SameSite http.SameSite
}

(c *Cookies) Get(name string) (string, bool)
(c *Cookies) Set(name, value string, opts CookieOpts)
(c *Cookies) SetExposed(name, value string, opts CookieOpts)
(c *Cookies) Delete(name string, opts CookieOpts)
(c *Cookies) Apply(w http.ResponseWriter)
```

Secure defaults: `HttpOnly=true`, `SameSite=Lax`, `Path="/"`, `Secure=request-scheme`.

## CSP

```go
type CSPMode int
const (
  CSPOff CSPMode = iota
  CSPStrict
  CSPReportOnly
)

type CSPConfig struct {
  Mode       CSPMode
  Directives map[string][]string
  ReportTo   string
}

kit.DefaultCSPDirectives() map[string][]string
kit.BuildCSPHeader(cfg CSPConfig, nonce string) string
kit.CSPHeaderName(mode CSPMode) string
kit.Nonce(ev *RequestEvent) string
kit.NonceAttr(ev *RequestEvent) string
```

## Streaming

```go
kit.Stream[T](fn func() (T, error)) *Streamed[T]
(s *Streamed[T]) Wait(ctx context.Context, timeout time.Duration) (T, error)
(s *Streamed[T]) IsResolved() bool
kit.DefaultStreamTimeout = 30 * time.Second
```

## Page options

```go
type TrailingSlash uint8
const (
  TrailingSlashDefault TrailingSlash = iota
  TrailingSlashNever
  TrailingSlashAlways
  TrailingSlashIgnore
)

type PageOptions struct {
  Prerender     bool
  SSR           bool
  CSR           bool
  SSROnly       bool
  TrailingSlash TrailingSlash
}
```

`PageOptions.Merge(override PageOptionsOverride) PageOptions` cascades layout values into pages.

`SSROnly = true` blocks the `__data.json` scrape endpoint used by the SPA client for prefetch. Use on pages where only the server-rendered HTML response is intended (e.g. print views, webhook acknowledgement pages).

## Link

```go
kit.Link(pattern string, params map[string]string) (string, error)
```

Runtime fallback. Codegen-emitted typed helpers under `<module>/.gen/links` are preferred — they fail at compile time when the route is renamed.

## Sitemap and robots

```go
kit.NewSitemap(baseURL string) *SitemapBuilder
(b *SitemapBuilder) Add(path string, lastMod time.Time, freq ChangeFreq, priority float64) *SitemapBuilder
(b *SitemapBuilder) Bytes() []byte

kit.NewRobots() *RobotsBuilder
(r *RobotsBuilder) UserAgent(name string) *RobotsBuilder
(r *RobotsBuilder) Allow(path string) *RobotsBuilder
(r *RobotsBuilder) Disallow(path string) *RobotsBuilder
(r *RobotsBuilder) Sitemap(url string) *RobotsBuilder
(r *RobotsBuilder) String() string
```

## Subpackages

- `kit/env` — `StaticPrivate`, `StaticPublic`, `DynamicPrivate`, `DynamicPublic`. Public accessors enforce a `PUBLIC_` key prefix.
- `kit/params` — built-in matchers `Int`, `UUID`, `Slug`, plus `DefaultMatchers()`.

## Stability

This package is the project's primary stability surface. Breaking changes follow the procedure in RFC #97. Until v1.0 the surface is marked experimental; treat tagged releases as gates.
