// Node ESM loader hook for `.svelte` files. Registered by the
// `--mode=ssr-serve` sidecar at boot via `module.register()`. The hook
// runs on the loader thread; the parent thread passes the project root
// + sidecar directory through the `data` argument so the hook can
// resolve `$app/*` shims and `$lib/*` imports without re-deriving them.
//
// Three responsibilities:
//
//  1. resolve(): rewrite bare `$app/state`, `$app/navigation`, and
//     `$lib/<rest>` specifiers to absolute file:// URLs before Node's
//     default resolver sees them. Without this Node would 404 on the
//     bare specifier.
//  2. resolve(): when the parent module imports a `.svelte` file by
//     relative path (`./Foo.svelte` or `../Bar.svelte`), resolve it to
//     an absolute file:// URL the load() hook can recognise.
//  3. load(): for any `.svelte` URL, compile the source via
//     `svelte/compiler` in `generate: 'server'` mode, rewrite the
//     compiled JS so any `$app/*` / `$lib/*` imports it emits also
//     route through file:// URLs, then return the JS as the module
//     source. Compiled output is cached by absolute path so repeat
//     resolutions during a request avoid re-compilation.
//
// The hook never uses a temp dir on disk — it returns module source
// directly via `load()`'s `source` field, which the V8 module loader
// evaluates as if it had been read from a file. That keeps the cache
// in-memory and avoids the cleanup-on-exit dance the entry-file path
// in `ssr_serve.mjs` does for its `mkdtemp`-based cache.

import { readFile } from "node:fs/promises";
import { fileURLToPath, pathToFileURL } from "node:url";
import { isAbsolute, join as joinPath, resolve as resolvePath } from "node:path";
import { compile } from "svelte/compiler";

let projectRoot = null;
let sidecarDir = null;
let sidecarRootURL = null;
let appShimURLs = null;
const compiledCache = new Map();

// initialize() runs once on the loader thread when the parent calls
// `register(specifier, parentURL, { data })`. The `data` object is
// structured-cloned across the thread boundary; only plain JSON-shaped
// values survive intact.
export function initialize(data) {
	projectRoot = data.projectRoot;
	sidecarDir = data.sidecarDir;
	// A trailing-slash file:// URL anchored at the sidecar dir. Used as
	// the parentURL when re-resolving bare `svelte` / `svelte/...`
	// specifiers so Node's package resolver walks the sidecar's
	// vendored `node_modules` instead of the project root (which has
	// no Svelte runtime installed).
	sidecarRootURL = pathToFileURL(sidecarDir + "/").href;
	appShimURLs = {
		"$app/state": pathToFileURL(joinPath(sidecarDir, "shims", "app-state.mjs")).href,
		"$app/navigation": pathToFileURL(joinPath(sidecarDir, "shims", "app-navigation.mjs")).href,
		"$app/forms": pathToFileURL(joinPath(sidecarDir, "shims", "app-forms.mjs")).href,
	};
}

function resolveAlias(specifier) {
	if (specifier === "$app/state" || specifier === "$app/navigation" || specifier === "$app/forms") {
		return appShimURLs[specifier];
	}
	if (specifier.startsWith("$lib/")) {
		const rest = specifier.slice("$lib/".length);
		return pathToFileURL(joinPath(projectRoot, "src", "lib", rest)).href;
	}
	return null;
}

// resolve() intercepts every module specifier the runtime is about to
// load. The default resolver is invoked via `nextResolve` for anything
// the hook doesn't claim.
export async function resolve(specifier, context, nextResolve) {
	const aliased = resolveAlias(specifier);
	if (aliased) {
		return { url: aliased, shortCircuit: true, format: aliased.endsWith(".svelte") ? "module" : undefined };
	}
	// Relative `.svelte` import from another module: resolve it against
	// the parent's URL so the load() hook sees an absolute file:// URL
	// it can claim.
	if (specifier.endsWith(".svelte")) {
		let url;
		if (specifier.startsWith("file://")) {
			url = specifier;
		} else if (isAbsolute(specifier)) {
			url = pathToFileURL(specifier).href;
		} else if (context.parentURL && context.parentURL.startsWith("file://")) {
			const parentPath = fileURLToPath(context.parentURL);
			const abs = resolvePath(joinPath(parentPath, ".."), specifier);
			url = pathToFileURL(abs).href;
		} else {
			return nextResolve(specifier, context);
		}
		return { url, shortCircuit: true, format: "module" };
	}
	// Bare `svelte` / `svelte/...` specifier: re-resolve from the
	// sidecar's own root so Node walks the sidecar's vendored
	// `node_modules`. Without this, a child .svelte compiled by load()
	// inherits a parentURL inside the project tree, where `svelte` is
	// not installed (issue #512 root cause for project-rooted children).
	if (specifier === "svelte" || specifier.startsWith("svelte/")) {
		return nextResolve(specifier, { ...context, parentURL: sidecarRootURL });
	}
	return nextResolve(specifier, context);
}

// rewriteImports turns every bare `$app/*` and `$lib/*` import in the
// compiled JS into an absolute file:// URL Node can resolve directly.
// Mirrors the entry-level rewrite passes in ssr_serve.mjs but reaches
// any `.svelte` the loader compiles, not just the entry.
function rewriteImports(code) {
	return code.replace(
		/(\bfrom\s*|\bimport\s*\(\s*)(['"])((?:\$app\/(?:state|navigation|forms))|(?:\$lib\/[^'"]+))\2/g,
		(_match, prefix, quote, specifier) => {
			const url = resolveAlias(specifier);
			if (!url) return _match;
			return `${prefix}${quote}${url}${quote}`;
		},
	);
}

// load() returns the compiled module source for `.svelte` URLs. Node
// caches the resulting module by URL itself, so the in-process
// `compiledCache` here is only an optimisation for repeated resolves
// of the same path within a single boot.
export async function load(url, context, nextLoad) {
	if (!url.endsWith(".svelte") || !url.startsWith("file://")) {
		return nextLoad(url, context);
	}
	const absolute = fileURLToPath(url);
	let source = compiledCache.get(absolute);
	if (!source) {
		const src = await readFile(absolute, "utf8");
		const result = compile(src, {
			generate: "server",
			filename: absolute,
			dev: false,
			hmr: false,
		});
		if (!result || typeof result.js?.code !== "string") {
			throw new Error(`svelte/compiler produced no js.code for ${absolute}`);
		}
		source = rewriteImports(result.js.code);
		compiledCache.set(absolute, source);
	}
	return {
		format: "module",
		shortCircuit: true,
		source,
	};
}
