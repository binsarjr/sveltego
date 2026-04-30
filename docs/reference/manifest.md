---
title: Manifest
order: 220
summary: The manifest.gen.go contract — registered routes, layouts, hooks, page options.
---

# Manifest

`manifest.gen.go` is the registry the runtime consults at request time. Codegen emits it; user code does not edit it.

## What it registers

- **Routes.** Each `+page.svelte` and `+server.go` produces an entry with: pattern, render function, optional `Load`, optional `Actions`, resolved `PageOptions`.
- **Layouts.** Each `+layout.svelte` produces an entry with: pattern, render function, optional `Load`, link to parent.
- **Error boundaries.** Each `+error.svelte` registers under its mount point.
- **Param matchers.** Functions exported from `src/params/<name>.go` map to matcher names usable in patterns (`[id=hex]`).
- **Hooks.** `Handle`, `HandleError`, `HandleFetch`, `Reroute`, `Init` from `hooks.server.go`. Missing fields are filled with `kit.Identity*` defaults.
- **Page options.** Resolved per-route values for `Prerender`, `SSR`, `CSR`, `TrailingSlash` after layout cascade.

## Why a manifest

Runtime route lookup is a map index, not a tree walk. Every static decision (which Load runs, what TrailingSlash mode applies, which matcher validates a param) is resolved at codegen so the request path stays branch-free.

## Lifecycle

1. `sveltego compile` (or the `compile` step inside `build`) emits `.gen/manifest.gen.go`.
2. `sveltego build` then runs `go build`, which links the manifest into the binary.
3. The server, on `Init`, reads the manifest registrations and constructs the router.

## Why you can't edit it

The file is overwritten on every codegen run. Source of truth lives in:

- `page.server.go` for `Load`, `Actions`, page options.
- `layout.server.go` for layout `Load` and cascaded options.
- `hooks.server.go` for hooks.
- File paths under `src/routes/` for patterns.

Edit those; rebuild; the manifest follows.

## Inspecting

`sveltego routes` prints the route table without compiling Go. Use it to verify a new route is registered before you reach for `go build`.
