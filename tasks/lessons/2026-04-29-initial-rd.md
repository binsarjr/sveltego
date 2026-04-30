## 2026-04-29 — Initial R&D

### Insight

- SvelteKit's `Server.respond(Request) → Promise<Response>` is a small contract — Web standards plus optional `AsyncLocalStorage`.
- "Webcontainer mode" was the escape hatch we considered to avoid `AsyncLocalStorage`: serialize requests in runtimes without ALS. It works but caps throughput.
- goja is pure Go but not a drop-in modern JS runtime — partial ESM, no dynamic import, zero Web APIs.
- v8go is the perf king but cross-compile is painful (prebuilt V8 bindings per target).
- subprocess Bun is fastest path to production but is not "true embed" — you ship a 50MB+ runtime alongside the Go binary.

### Self-rules

1. Don't claim "embed" without distinguishing in-process runtime vs ship-binary. Ask the user.
2. Modern SvelteKit bundles use ESM + dynamic import. Runtimes lacking either need a transpile step in the adapter.
3. Web API polyfills in goja are scope creep. Estimate ~70% of total effort.
4. Avoid "production-ready" claims for early PoCs — tier probabilities (PoC vs full vs production).

