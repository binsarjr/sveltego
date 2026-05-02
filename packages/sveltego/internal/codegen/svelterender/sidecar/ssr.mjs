// SSR mode: compile each .svelte route via svelte/compiler in
// generate:'server' mode, parse the compiled JS with Acorn, and write
// the ESTree AST as JSON. Output is byte-deterministic — the same input
// produces the same file on every run, on every host.
//
// Manifest format (JSON, fed via --manifest=<path>):
//
//   {
//     "root":  "<absolute project root>",
//     "outDir": "<absolute path; usually <root>/.gen/svelte_js2go>",
//     "jobs": [
//       { "route": "/", "source": "src/routes/_page.svelte" },
//       { "route": "/about", "source": "src/routes/about/_page.svelte" }
//     ]
//   }
//
// Output per job: <outDir>/<route-slug>/ast.json containing
//   { "schema": "<version>", "route": "<route>", "ast": <ESTree Program> }
//
// route-slug = route with leading '/' stripped, '/' replaced by '__',
// and the special root route "/" mapped to "_root". Slugging keeps the
// directory tree filesystem-safe across platforms.

import { readFile, writeFile, mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { parse as acornParse } from "acorn";
import { compile } from "svelte/compiler";

export const SCHEMA_VERSION = "ssr-json-ast/v1";

export function routeSlug(route) {
	if (route === "/" || route === "") return "_root";
	const trimmed = route.replace(/^\/+/, "").replace(/\/+$/, "");
	return trimmed.split("/").join("__");
}

export function compileServerJS(source, filename) {
	const result = compile(source, {
		generate: "server",
		filename,
		dev: false,
		hmr: false,
	});
	if (!result || typeof result.js?.code !== "string") {
		throw new Error(
			`svelte/compiler returned no js.code for ${filename}`,
		);
	}
	return result.js.code;
}

export function parseToAST(jsSource) {
	return acornParse(jsSource, {
		sourceType: "module",
		ecmaVersion: "latest",
		locations: false,
	});
}

// stableStringify emits JSON with sorted object keys at every level so
// minor changes in Acorn's property emission order can never produce a
// different golden file. Arrays preserve order (semantics depend on
// position). null and primitives pass through.
//
// Cycle detection tracks ancestors on the current walk path, not every
// node ever seen. ESTree ASTs are DAGs in the wild — Acorn shares the
// same Identifier object between `imported` and `local` on a non-rename
// `ImportSpecifier` (`import { page }` from '$app/state'), and any
// `WeakSet`-add-only check would mis-flag that as a cycle. A genuine
// cycle is a node reachable from itself via parent→child edges, which
// is what an enter/exit ancestor stack catches.
export function stableStringify(value) {
	const ancestors = new WeakSet();
	const walk = (node) => {
		if (node === null || typeof node !== "object") return node;
		if (ancestors.has(node)) {
			throw new Error("stableStringify: cycle in AST");
		}
		ancestors.add(node);
		try {
			if (Array.isArray(node)) return node.map(walk);
			const out = {};
			for (const key of Object.keys(node).sort()) {
				out[key] = walk(node[key]);
			}
			return out;
		} finally {
			ancestors.delete(node);
		}
	};
	return JSON.stringify(walk(value), null, 2) + "\n";
}

async function processJob(job, ctx) {
	const sourcePath = resolve(ctx.root, job.source);
	const source = await readFile(sourcePath, "utf8");
	const js = compileServerJS(source, job.source);
	const ast = parseToAST(js);
	const slug = routeSlug(job.route);
	const outPath = join(ctx.outDir, slug, "ast.json");
	await mkdir(dirname(outPath), { recursive: true });
	const payload = {
		ast,
		route: job.route,
		schema: SCHEMA_VERSION,
	};
	await writeFile(outPath, stableStringify(payload), "utf8");
	return { route: job.route, output: outPath };
}

export async function runSSR(args) {
	if (!args.manifest) {
		process.stderr.write(
			"svelterender-sidecar ssr: --manifest=<path> is required\n",
		);
		process.exit(2);
	}
	const manifestPath = resolve(args.manifest);
	const raw = await readFile(manifestPath, "utf8");
	const manifest = JSON.parse(raw);
	if (!manifest || !Array.isArray(manifest.jobs)) {
		throw new Error(
			`ssr manifest at ${manifestPath} missing 'jobs' array`,
		);
	}
	const root = resolve(manifest.root || args.root || process.cwd());
	const outDir = resolve(manifest.outDir || args.out || join(root, ".gen", "svelte_js2go"));
	const ctx = { root, outDir };
	const results = [];
	for (const job of manifest.jobs) {
		if (!job || typeof job.route !== "string" || typeof job.source !== "string") {
			throw new Error(
				`ssr manifest job missing 'route' or 'source': ${JSON.stringify(job)}`,
			);
		}
		results.push(await processJob(job, ctx));
	}
	process.stdout.write(stableStringify({ schema: SCHEMA_VERSION, results }));
}
