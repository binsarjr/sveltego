// SSG mode placeholder. The pure-Svelte SSG pipeline (RFC #379 phase 4,
// #383) wires Vite's SSR output into svelte/server.render() and writes
// HTML under static/_prerendered/. Until that lands the Go orchestrator
// only invokes EnsureNode(); no SSG manifest is dispatched here yet.
//
// Keeping the dispatcher entry shape stable means the Phase 4 wiring is
// a body-only change inside this file — the sidecar contract (one
// binary, two modes, --manifest input) does not move.

export async function runSSG(_args) {
	process.stderr.write(
		"svelterender-sidecar: ssg mode is not yet implemented (RFC #379 phase 4 / #383)\n",
	);
	process.exit(2);
}
