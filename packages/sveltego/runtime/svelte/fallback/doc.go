// Package fallback owns the runtime side of ADR 0009 sub-decision 2's
// escape hatch: routes that declare `<!-- sveltego:ssr-fallback -->`
// in their `_page.svelte` opt out of the build-time JS→Go transpiler
// and route through a long-running Node sidecar at request time.
//
// The package is self-contained at runtime: codegen wires per-route
// fallback handlers into the manifest; those handlers consult the
// process-global Registry to dispatch into a sidecar Client which
// caches rendered HTML by (route, hash(load_result)).
//
// Lifecycle: the server boots the sidecar via Start exactly once when
// the manifest declares at least one fallback route. Stop is called on
// graceful shutdown. When no route is annotated, neither helper runs;
// the process never spawns Node.
package fallback
