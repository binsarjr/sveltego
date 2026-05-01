// Build-time Node sidecar entry point. Three modes:
//
//   --mode=ssg        prerender pure-Svelte routes to static HTML (ADR 0008).
//   --mode=ssr        compile each route via svelte/compiler generate:'server'
//                     then parse the resulting JS via Acorn and write the
//                     ESTree JSON AST to .gen/svelte_js2go/<route>/ast.json
//                     (ADR 0009, RFC #421).
//   --mode=ssr-serve  long-running HTTP server that renders Svelte routes
//                     on demand. Used by Go's runtime/svelte/fallback for
//                     routes annotated with `<!-- sveltego:ssr-fallback -->`
//                     (ADR 0009 sub-decision 2 / Phase 8 #430).
//
// Build-time modes (ssg / ssr) are one-shot. ssr-serve is the only
// long-running mode; the deployed Go binary plus static/ remains the
// production deployable.

import { runSSG } from "./ssg.mjs";
import { runSSR } from "./ssr.mjs";
import { runSSRServe } from "./ssr_serve.mjs";

function parseArgs(argv) {
	const args = { mode: "", manifest: "", out: "", root: "", port: "" };
	for (const raw of argv.slice(2)) {
		const eq = raw.indexOf("=");
		if (eq < 0) continue;
		const key = raw.slice(0, eq);
		const val = raw.slice(eq + 1);
		switch (key) {
			case "--mode":
				args.mode = val;
				break;
			case "--manifest":
				args.manifest = val;
				break;
			case "--out":
				args.out = val;
				break;
			case "--root":
				args.root = val;
				break;
			case "--port":
				args.port = val;
				break;
		}
	}
	return args;
}

async function main() {
	const args = parseArgs(process.argv);
	if (!args.mode) {
		process.stderr.write(
			"svelterender-sidecar: --mode=ssg|ssr|ssr-serve is required\n",
		);
		process.exit(2);
	}
	switch (args.mode) {
		case "ssg":
			await runSSG(args);
			return;
		case "ssr":
			await runSSR(args);
			return;
		case "ssr-serve":
			await runSSRServe(args);
			return;
		default:
			process.stderr.write(
				`svelterender-sidecar: unknown --mode=${args.mode} (want ssg, ssr, or ssr-serve)\n`,
			);
			process.exit(2);
	}
}

main().catch((err) => {
	process.stderr.write(
		`svelterender-sidecar: ${err && err.stack ? err.stack : err}\n`,
	);
	process.exit(1);
});
