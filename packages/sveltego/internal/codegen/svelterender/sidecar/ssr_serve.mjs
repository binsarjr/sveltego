// ssr-serve mode: long-running HTTP server that renders Svelte routes
// on demand. Each request body is JSON of the form:
//
//   { "route": "/posts/[id]", "source": "src/routes/posts/[id]/_page.svelte",
//     "data": { /* page data */ } }
//
// The server compiles the .svelte source via svelte/compiler in
// generate:'server' mode (cached per source path so subsequent
// requests reuse the compiled module), evaluates the resulting code
// in a Node `vm` sandbox to obtain the render function, then invokes
// it with a $$payload mock and the supplied data as `$$props`.
// Response body:
//
//   { "body": "<rendered html>", "head": "<head html>" }
//
// Errors return HTTP 500 with `{ "error": "..." }`. The server prints
// `SVELTEGO_SSR_FALLBACK_LISTEN port=NNN` on stderr after binding so
// the Go supervisor can parse the address. The OS picks the port (we
// pass 0) unless --port=NNN is provided for tests.

import { createServer } from "node:http";
import { readFile, writeFile, mkdtemp, rm } from "node:fs/promises";
import { isAbsolute, resolve as resolvePath, join as joinPath, dirname } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { compile } from "svelte/compiler";
import { render as svelteRender } from "svelte/server";

const compiledCache = new Map();
let cacheDir = null;
const sidecarDir = dirname(fileURLToPath(import.meta.url));

// Server-side shim URLs for the `$app/*` virtual modules the client
// build resolves via Vite alias. The compiled .mjs the sidecar emits per
// route imports `'$app/state'` and `'$app/navigation'`; without these
// shims Node fails to resolve the bare specifier and the render call
// throws (#460). Resolved once at module load — `pathToFileURL` is the
// canonical way to import a local file by absolute path.
const appShimURLs = {
	"$app/state": pathToFileURL(joinPath(sidecarDir, "shims", "app-state.mjs"))
		.href,
	"$app/navigation": pathToFileURL(
		joinPath(sidecarDir, "shims", "app-navigation.mjs"),
	).href,
};

// rewriteAppAliases substitutes `'$app/state'` and `'$app/navigation'`
// import sources in the compiled JS with absolute file URLs of the
// server-side shims. The regex matches both single and double quotes
// (Svelte's compiler picks per-output) and allows the source string to
// appear either in `from '...'` (static imports) or `import('...')`
// (dynamic imports). Any other `$app/*` specifier is left alone so the
// caller surfaces a clear "unresolved $app/<name>" error rather than
// silently substituting an unrelated module.
export function rewriteAppAliases(code) {
	return code.replace(
		/(\bfrom\s*|\bimport\s*\(\s*)(['"])(\$app\/(?:state|navigation))\2/g,
		(_match, prefix, quote, specifier) => {
			const url = appShimURLs[specifier];
			return `${prefix}${quote}${url}${quote}`;
		},
	);
}

async function ensureCacheDir() {
	if (cacheDir) return cacheDir;
	// Place the cache dir inside the sidecar tree so Node's module
	// resolver can walk up to the sidecar's node_modules to find
	// `svelte`. Compiled SSR modules import `svelte/internal/server`;
	// putting the cache under /tmp would fail the resolution.
	cacheDir = await mkdtemp(joinPath(sidecarDir, ".sveltego-fallback-"));
	process.on("exit", () => {
		rm(cacheDir, { recursive: true, force: true }).catch(() => {});
	});
	return cacheDir;
}

async function loadComponent(root, source) {
	const absolute = isAbsolute(source) ? source : resolvePath(root, source);
	if (compiledCache.has(absolute)) {
		return compiledCache.get(absolute);
	}
	const src = await readFile(absolute, "utf8");
	const result = compile(src, {
		generate: "server",
		filename: absolute,
		dev: false,
		hmr: false,
	});
	if (!result || typeof result.js?.code !== "string") {
		throw new Error(`svelte/compiler produced no js.code for ${source}`);
	}
	const rewritten = rewriteAppAliases(result.js.code);
	const dir = await ensureCacheDir();
	const safe = absolute.replace(/[^a-zA-Z0-9_]/g, "_");
	const modPath = joinPath(dir, `${safe}.mjs`);
	await writeFile(modPath, rewritten, "utf8");
	const url = pathToFileURL(modPath).href;
	const mod = await import(url);
	const fn = mod.default;
	if (typeof fn !== "function") {
		throw new Error(`compiled component ${source} did not export a default function`);
	}
	compiledCache.set(absolute, fn);
	return fn;
}

function readJSONBody(req) {
	return new Promise((resolveBody, rejectBody) => {
		const chunks = [];
		let total = 0;
		const limit = 1 << 22; // 4 MiB cap; load payloads should never approach this
		req.on("data", (chunk) => {
			total += chunk.length;
			if (total > limit) {
				rejectBody(new Error(`request body too large (>${limit} bytes)`));
				req.destroy();
				return;
			}
			chunks.push(chunk);
		});
		req.on("end", () => {
			try {
				const raw = Buffer.concat(chunks).toString("utf8");
				resolveBody(raw ? JSON.parse(raw) : {});
			} catch (err) {
				rejectBody(err);
			}
		});
		req.on("error", rejectBody);
	});
}

async function handleRender(req, res, root) {
	let parsed;
	try {
		parsed = await readJSONBody(req);
	} catch (err) {
		res.writeHead(400, { "content-type": "application/json" });
		res.end(JSON.stringify({ error: String(err.message || err) }));
		return;
	}
	const { route, source, data } = parsed;
	if (typeof route !== "string" || typeof source !== "string") {
		res.writeHead(400, { "content-type": "application/json" });
		res.end(JSON.stringify({ error: "request missing route or source" }));
		return;
	}
	try {
		const fn = await loadComponent(root, source);
		const result = svelteRender(fn, { props: { data: data ?? null } });
		const payload = JSON.stringify({
			body: result.body || "",
			head: result.head || "",
		});
		res.writeHead(200, { "content-type": "application/json" });
		res.end(payload);
	} catch (err) {
		process.stderr.write(`ssr-serve render ${route} (${source}): ${err && err.stack ? err.stack : err}\n`);
		if (!res.headersSent) {
			res.writeHead(500, { "content-type": "application/json" });
		}
		res.end(JSON.stringify({ error: String(err.message || err) }));
	}
}

export async function runSSRServe(args) {
	const root = resolvePath(args.root || process.cwd());
	const port = args.port ? Number.parseInt(args.port, 10) : 0;

	const server = createServer(async (req, res) => {
		if (req.method === "POST" && req.url === "/render") {
			await handleRender(req, res, root);
			return;
		}
		if (req.method === "GET" && req.url === "/healthz") {
			res.writeHead(200, { "content-type": "text/plain" });
			res.end("ok");
			return;
		}
		res.writeHead(404, { "content-type": "text/plain" });
		res.end("not found");
	});

	await new Promise((resolveListen, rejectListen) => {
		server.once("error", rejectListen);
		server.listen(port, "127.0.0.1", () => {
			const addr = server.address();
			process.stderr.write(`SVELTEGO_SSR_FALLBACK_LISTEN port=${addr.port}\n`);
			resolveListen(undefined);
		});
	});

	const shutdown = (signal) => {
		process.stderr.write(`ssr-serve: ${signal} received, shutting down\n`);
		server.close(() => process.exit(0));
		setTimeout(() => process.exit(0), 5000).unref();
	};
	process.on("SIGTERM", () => shutdown("SIGTERM"));
	process.on("SIGINT", () => shutdown("SIGINT"));

	await new Promise(() => {});
}
