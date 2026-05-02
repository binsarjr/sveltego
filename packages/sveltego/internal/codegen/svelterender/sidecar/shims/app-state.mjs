// Server-side shim for `$app/state` consumed by the runtime SSR
// fallback sidecar (`--mode=ssr-serve`). The compiled .mjs the sidecar
// emits per route imports `'$app/state'`; without this shim Node fails
// to resolve the bare specifier and the render call throws.
//
// On the client these symbols are runes backed by the SPA router. On
// the SSR fallback path they are static objects populated per render
// via _setPage / _setNavigating / _setUpdated. The Go fallback supervisor
// will extend the render-request envelope to carry the page snapshot in
// a follow-up; until then defaults ship to keep imports resolvable and
// `svelte/server` rendering of `page.url.pathname` etc. from crashing.

let pageState = {
	url: new URL("http://localhost/"),
	params: {},
	route: { id: null },
	status: 200,
	error: null,
	data: null,
	form: null,
	state: {},
};

let navigatingState = { current: null };
let updatedState = { current: false };

export const page = new Proxy(
	{},
	{
		get(_target, prop) {
			return pageState[prop];
		},
		ownKeys() {
			return Reflect.ownKeys(pageState);
		},
		getOwnPropertyDescriptor(_target, prop) {
			if (prop in pageState) {
				return {
					configurable: true,
					enumerable: true,
					value: pageState[prop],
				};
			}
			return undefined;
		},
		has(_target, prop) {
			return prop in pageState;
		},
	},
);

export const navigating = new Proxy(
	{},
	{
		get(_target, prop) {
			return navigatingState[prop];
		},
	},
);

export const updated = new Proxy(
	{},
	{
		get(_target, prop) {
			return updatedState[prop];
		},
	},
);

export function _setPage(next) {
	pageState = next;
}

export function _setNavigating(next) {
	navigatingState = next;
}

export function _setUpdated(next) {
	updatedState = next;
}
