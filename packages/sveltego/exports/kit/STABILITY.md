# Stability — packages/sveltego/exports/kit

Last updated: 2026-04-30 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97).

## Tier legend

| Tier | Promise |
|---|---|
| `stable` | Won't break within the current major. Add-only changes; behavior changes documented in CHANGELOG. |
| `experimental` | May break in any minor release. Marked `// Experimental:` in godoc. |
| `deprecated` | Scheduled for removal. Marked `// Deprecated:` in godoc. Removed in next major. |

## Stable

- `kit.Cookies` — cookie jar; set, get, delete, apply to response writer
- `kit.CookieOpts` — options struct for Set/SetExposed/Delete
- `kit.NewCookies` — constructor
- `kit.RequestEvent` — request-scoped event passed to load, actions, hooks
- `kit.NewRequestEvent` — constructor
- `kit.RenderCtx` — render-phase context (params, locals, header writer)
- `kit.LoadCtx` — load-phase context (params, parent data, header writer, speculative flag)
- `kit.NewRenderCtx` — constructor
- `kit.NewLoadCtx` — constructor
- `kit.Hooks` — struct collecting all hook functions
- `kit.DefaultHooks` — returns a Hooks with all identity implementations
- `kit.SafeError` — public-facing error wrapper (status + message)
- `kit.Redirect` — returns a redirect sentinel error
- `kit.Error` — returns an HTTP error sentinel
- `kit.Fail` — returns an action-fail sentinel

## Experimental

- `kit.HeaderWriter` — lazy header accumulator; shape may change
- `kit.Response` — SSR response envelope; fields subject to change
- `kit.NewResponse` — constructor
- `kit.HandleFn` — hook handler function type
- `kit.ResolveFn` — resolve continuation passed to HandleFn
- `kit.HandleErrorFn` — error hook function type
- `kit.HandleFetchFn` — fetch-intercept hook function type
- `kit.RerouteFn` — reroute hook function type
- `kit.InitFn` — server-init hook function type
- `kit.IdentityHandle` — no-op HandleFn
- `kit.IdentityHandleError` — no-op HandleErrorFn
- `kit.IdentityHandleFetch` — no-op HandleFetchFn
- `kit.IdentityReroute` — no-op RerouteFn
- `kit.IdentityInit` — no-op InitFn
- `kit.Sequence` — composes multiple HandleFn values left-to-right
- `kit.ActionFn` — action handler function type
- `kit.ActionMap` — map of action name → ActionFn
- `kit.ActionResult` — sealed interface for action return values
- `kit.ActionData` — successful action result
- `kit.ActionFailData` — failed action result
- `kit.ActionRedirectResult` — redirect action result
- `kit.ActionDataResult` — constructor for ActionData
- `kit.ActionFail` — constructor for ActionFailData
- `kit.ActionRedirect` — constructor for ActionRedirectResult
- `kit.BindError` — error returned by BindForm / BindMultipart
- `kit.DefaultMaxFormMemory` — 32 MiB default for form parsing
- `kit.CSPMode` — enumeration of CSP enforcement modes
- `kit.CSPConfig` — CSP configuration struct
- `kit.CSPTemplate` — pre-compiled CSP template
- `kit.NewCSPTemplate` — constructor
- `kit.Nonce` — reads CSP nonce from event locals
- `kit.SetNonce` — writes CSP nonce into event locals
- `kit.NonceAttr` — formats nonce as an HTML attribute string
- `kit.DefaultCSPDirectives` — returns the default CSP directive map
- `kit.BuildCSPHeader` — builds a CSP header value string
- `kit.CSPHeaderName` — returns the header name for the given CSP mode
- `kit.TrailingSlash` — enumeration of trailing-slash policies
- `kit.PageOptions` — per-route rendering options (now includes `Templates`: "go-mustache" default, "svelte" opt-in for RFC #379 phase 3)
- `kit.PageOptionsOverride` — partial options for merging into PageOptions
- `kit.DefaultPageOptions` — returns sensible defaults
- `kit.TemplatesGoMustache` — string constant for the Mustache-Go template pipeline
- `kit.TemplatesSvelte` — string constant for the pure-Svelte template pipeline
- `kit.M` — type alias for `map[string]any`
- `kit.JSON` — builds a JSON response
- `kit.Text` — builds a plain-text response
- `kit.XML` — builds an XML response
- `kit.NoContent` — builds a 204 response
- `kit.MethodNotAllowed` — builds a 405 response
- `kit.Link` — builds a URL from a pattern and params map
- `kit.ErrLinkPattern` — sentinel for malformed link patterns
- `kit.ErrLinkParam` — sentinel for missing link params
- `kit.Asset` — resolves a static asset path to its hashed URL
- `kit.RegisterAssets` — installs the source-path → hashed-URL table consumed by `Asset`
- `kit.DefaultAssetsImmutablePrefix` — URL prefix under which fingerprinted assets are served
- `kit.RobotsBuilder` — builder for robots.txt output
- `kit.NewRobots` — constructor
- `kit.ChangeFreq` — enumeration of sitemap change frequencies
- `kit.SitemapEntry` — single sitemap entry
- `kit.SitemapBuilder` — builder for sitemap XML output
- `kit.NewSitemap` — constructor
- `kit.StaticConfig` — configuration for the static file handler
- `kit.DefaultStaticImmutablePrefix` — default immutable asset path prefix
- `kit.DefaultStaticMaxAge` — default cache max-age for immutable assets
- `kit.Streamed[T]` — generic deferred-value wrapper for streaming SSR
- `kit.StreamedAny` — type-erased interface for Streamed[T]
- `kit.Stream[T]` — constructor without context propagation
- `kit.StreamCtx[T]` — constructor with context propagation
- `kit.DefaultStreamTimeout` — default timeout for Wait / WaitAny
- `kit.ErrStreamTimeout` — sentinel returned on stream timeout
- `kit.RedirectErr` — concrete type for redirect sentinels
- `kit.HTTPErr` — concrete type for HTTP error sentinels
- `kit.FailErr` — concrete type for action-fail sentinels
- `kit.RedirectOption` — functional option for Redirect
- `kit.RedirectReload` — option that adds an HX-Refresh style reload header
- `kit.HTTPError` — interface implemented by redirect / HTTP error sentinels

## Deprecated

(none)

## Internal-only (do not import even though exported)

(none — all exports in this package are intended for application authors)

## Breaking change procedure

Any `stable` symbol change requires an RFC issue describing the change and migration path, a PR that adds the new API alongside the old, at least one minor cycle with `// Deprecated:` godoc, and removal in the next major bump with a CHANGELOG entry. See [RFC #97](https://github.com/binsarjr/sveltego/issues/97) for the full procedure; CHANGELOG entries are generated by release-please.

## How to mark new symbols

Every exported symbol added in a PR **must** add a corresponding row to this file in the same PR. Place the row under `## Experimental` unless you have explicit approval to mark it `stable`. If the symbol is intentionally internal (e.g. used only by generated code), place it under `## Internal-only`.
