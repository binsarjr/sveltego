---
title: CSP
order: 86
summary: Content-Security-Policy via kit.CSPConfig with per-request nonce.
---

# Content Security Policy

sveltego ships a CSP middleware behind `kit.CSPConfig`. Strict mode emits `Content-Security-Policy`; report-only mode emits `Content-Security-Policy-Report-Only` so violations are observed without enforcement during rollout.

## Enable

```go
cfg := kit.CSPConfig{
  Mode: kit.CSPStrict,
  Directives: map[string][]string{
    // override or extend the defaults
    "img-src": {"'self'", "data:", "https://cdn.example.com"},
  },
  ReportTo: "csp-endpoint",
}
```

Pass `cfg` to the server constructor; the pipeline inserts a per-request nonce into `Locals` and splices it into the `script-src` directive.

## Defaults

`kit.DefaultCSPDirectives()` returns:

```
default-src 'self'
script-src  'strict-dynamic'
style-src   'self' 'unsafe-inline'
img-src     'self' data:
connect-src 'self'
base-uri    'self'
form-action 'self'
```

Your `Directives` map merges over this baseline. Setting a directive to an empty slice removes it.

## Auto-injected scripts

Codegen-emitted tags inherit the per-request nonce automatically when `kit.CSPConfig.Mode` is `CSPStrict` or `CSPReportOnly`. No template change is required for:

- `<script type="module">` — the per-route entry chunk emitted before `</body>`.
- `<link rel="modulepreload">` — the head-belonging hints for transitive imports.
- `<script id="sveltego-data" type="application/json">` — the JSON hydration payload.
- The streaming `<script>__sveltego__resolve(...)</script>` chunks emitted for `kit.Streamed[T]` fields.
- The service-worker registration `<script>` (when `Config.ServiceWorker = true`).

A live `CSPStrict` response carries one nonce on the header and on every emitted tag:

```
Content-Security-Policy: script-src 'nonce-AbC123…' 'strict-dynamic'; …
```

```html
<link rel="modulepreload" nonce="AbC123…" href="/_app/shared-def.js">
<script type="module" nonce="AbC123…" src="/_app/page-abc.js"></script>
<script id="sveltego-data" nonce="AbC123…" type="application/json">{…}</script>
```

`<link rel="stylesheet">` tags are intentionally not nonce-attributed — the default `style-src` directive does not gate on nonce. CSS hashes from Vite are already content-addressed.

> **SSG note.** Prerendered routes (`Prerender: true`) cannot use a nonce: the HTML is built once at `sveltego build` time, before any per-request value exists. SSG users keep `'unsafe-inline'`, switch to per-asset hashes, or rely on `'strict-dynamic'`. SSR (the default), SPA, and Static routes all carry nonces at request time.

## Inline scripts

Use the per-request nonce on every developer-authored inline `<script>`. The nonce arrives via the standard `data` prop (populate it from your layout's `Load`):

```svelte
<script lang="ts">
  let { data } = $props();
</script>

<script nonce={data.nonce}>
  console.log("hello");
</script>
```

In `_layout.server.go`:

```go
func Load(ctx *kit.LoadCtx) (LayoutData, error) {
  nonce, _ := ctx.Locals["cspNonce"].(string)
  return LayoutData{Nonce: nonce}, nil
}
```

The `cspNonce` key is populated by the CSP middleware on every request before any `Load` runs; it returns the empty string when CSP is off, so the same template compiles uniformly across opt-in and opt-out builds. Templates that read `page.nonce` (Svelte 5 `$app/state`) receive the same value via the per-route `RenderCtx.Nonce()` accessor — no separate Load channel is needed.

## Modes

| Mode | Effect |
|---|---|
| `kit.CSPOff` | No header. Default. |
| `kit.CSPStrict` | `Content-Security-Policy` enforced. |
| `kit.CSPReportOnly` | `Content-Security-Policy-Report-Only` for observation. |

## Header determinism

Directive order is deterministic (alphabetical) so two requests with equivalent input produce byte-identical headers. This simplifies caching and snapshot tests.
