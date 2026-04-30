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

## Inline scripts

Use the per-request nonce on every inline `<script>`:

```svelte
<script lang="go">
  // SSR template
</script>

<script{kit.NonceAttr(Ctx)}>
  console.log("hello");
</script>
```

`kit.NonceAttr(ev)` returns ` nonce="<n>"` when CSP is on, or the empty string when off — so the same template compiles uniformly across opt-in and opt-out builds.

## Modes

| Mode | Effect |
|---|---|
| `kit.CSPOff` | No header. Default. |
| `kit.CSPStrict` | `Content-Security-Policy` enforced. |
| `kit.CSPReportOnly` | `Content-Security-Policy-Report-Only` for observation. |

## Header determinism

Directive order is deterministic (alphabetical) so two requests with equivalent input produce byte-identical headers. This simplifies caching and snapshot tests.
