# Stability — packages/sveltego/runtime/svelte/fallback

Last updated: 2026-05-02 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97).
Package shipped via SSR Phase 8 ([#430](https://github.com/binsarjr/sveltego/issues/430), merged 2026-05-02).
Codifies ADR 0009 sub-decision 2's escape hatch for routes the build-time
JS-to-Go transpiler cannot lower.

## Tier legend

| Tier | Promise |
|---|---|
| `stable` | Won't break within the current major. Add-only changes; behavior changes documented in CHANGELOG. |
| `experimental` | May break in any minor release. Marked `// Experimental:` in godoc. |
| `deprecated` | Scheduled for removal. Marked `// Deprecated:` in godoc. Removed in next major. |

## Stable

(none)

## Experimental

Entire package. The fallback path is opt-in (a route declares
`<!-- sveltego:ssr-fallback -->` in its `_page.svelte`); when no route
opts in, neither helper runs and the process never spawns Node. Treat
every exported symbol as experimental until v1.0 RC.

- `fallback.Client` / `fallback.NewClient` / `fallback.ClientOptions` —
  HTTP client to the long-running Node sidecar. Transport: HTTP on
  127.0.0.1 ephemeral port; the sidecar prints
  `SVELTEGO_SSR_FALLBACK_LISTEN port=NNN` to stderr on boot, the Go
  side parses the port and dials it.
- `fallback.RenderRequest` / `fallback.RenderResponse` — JSON wire
  shape for the sidecar's `POST /render` endpoint.
- `fallback.CacheStats` — observability surface; cache layout itself
  is internal (in-memory LRU + TTL).
- `fallback.Registry` / `fallback.Default` — process-global registry
  of annotated routes. Codegen wires per-route `init()` calls that
  register routes; the supervisor only boots when
  `registry.HasRoutes()` is true.
- `fallback.Start` / `fallback.Sidecar` / `fallback.SidecarOptions` —
  one-shot sidecar boot. Reads stderr until the ready prefix, fails
  after `sidecarReadyTimeout` (30s).
- `fallback.NewSupervisor` / `fallback.Supervisor` — long-lived sidecar
  manager. Restarts up to 3× with exponential backoff (1s → 30s);
  after that, fallback routes hard-error at request time.

## Deprecated

(none)

## Internal-only (do not import even though exported)

(none)

## Decisions codified in this package

### IPC: HTTP on 127.0.0.1 ephemeral port

Chosen over Unix domain sockets:

- Browsable / debuggable with curl during dev.
- No socket-path length limits (macOS sun_path is 104 chars; nested
  worktrees and tmp dirs blow that quickly).
- Parity with the Phase 2 sidecar transport (also HTTP).

### Cache: in-memory LRU + TTL, per-Client

- Key: `route|sha256(json(data))` — semantically-equal payloads
  collide, which is intentional (deterministic Load returns produce
  cache hits).
- Defaults: 1000 entries, 60s TTL, 10s HTTP timeout.
- No metrics, no admission policy, no probabilistic TTL — the fallback
  path is itself a tail case (only annotated routes hit it); the cost
  paid down here is the sidecar round-trip, not steady-state
  contention.
- **Cluster-mode caching is out of scope for v1.** Each replica's
  cache is independent.

### Lifecycle

- Supervisor boots **only** when `registry.HasRoutes()` returns true.
  When no route declares `<!-- sveltego:ssr-fallback -->`, no Node
  process ever starts.
- Up to 3 restart attempts with exponential backoff (1s → 30s).
- After exhausting restarts, fallback routes return a 5xx; rest of the
  app keeps serving.

### Sidecar contract

- Entry point: `internal/codegen/svelterender/sidecar/ssr_serve.mjs`.
- Stdin envelope wires the registry; stderr emits the
  `SVELTEGO_SSR_FALLBACK_LISTEN port=NNN` ready line; HTTP serves
  `POST /render` with the JSON request shape mirrored by
  `fallback.RenderRequest`.

## Breaking change procedure

While `experimental`, any signature change must update STABILITY.md and
call out the change in the PR description. Promotion to `stable` is
gated on v1.0 RC.

## Cross-references

- [ADR 0009](../../../tasks/decisions/0009-ssr-option-b.md) — sub-decision 2 codifies hard-error-at-build with explicit opt-in via this package.
- `internal/codegen/svelterender/sidecar/CLAUDE.md` — sidecar's two operating modes.
- `internal/codegen/ssr.go` — `planSSR` partitions annotated vs transpiled routes.
- `server/ssr_fallback.go` — pipeline integration.
- `internal/routescan/ssr_fallback.go` — annotation detection at scan time.
