// Server-side shim for `$app/navigation` consumed by the runtime SSR
// fallback sidecar (`--mode=ssr-serve`). All exports are inert no-ops:
// navigation is a client-side concern. The shim exists so user code that
// imports `goto`, `invalidate`, etc. at module scope doesn't crash the
// initial server render — the imported symbols only get called from
// event handlers that never fire during SSR.

export function goto(_url, _opts) {
	return Promise.resolve();
}

export function invalidate(_dep) {
	return Promise.resolve();
}

export function invalidateAll() {
	return Promise.resolve();
}

export function preloadCode(_url) {
	return Promise.resolve();
}

export function preloadData(_url) {
	return Promise.resolve();
}

export function pushState(_url, _state) {}

export function replaceState(_url, _state) {}

export function beforeNavigate(_callback) {}

export function afterNavigate(_callback) {}

export function onNavigate(_callback) {}

export function disableScrollHandling() {}
