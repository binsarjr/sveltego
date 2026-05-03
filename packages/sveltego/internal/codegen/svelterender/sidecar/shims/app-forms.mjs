// Server-side shim for `$app/forms` consumed by the runtime SSR
// fallback sidecar (`--mode=ssr-serve`). The `enhance` action is a
// client-only DOM Svelte action; on the server it must exist as an
// importable symbol but never run. The exported function returns the
// inert `{ destroy }` shape Svelte's action contract expects so any
// SSR-safe call site receives a valid handle without touching the DOM.

export function enhance(_form, _callback) {
	return {
		destroy() {},
	};
}
