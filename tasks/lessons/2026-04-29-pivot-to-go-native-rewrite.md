## 2026-04-29 — Pivot to Go-native rewrite

### Insight

- All JS runtimes bond CPU to a JS engine. Even when the throughput is "OK" (Bun subprocess), the concurrency model is foreign to Go: no goroutines, no `context.Context`, IPC overhead per request.
- Adapters layered on top of SvelteKit-the-JS-server inherit every limitation of the chosen runtime. Going faster than the runtime is impossible.
- The SvelteKit *shape* (file convention, Load/Actions/hooks, layouts) is what users want — not the SvelteKit *implementation*.
- Codegen `.svelte` → Go source is feasible: Svelte 5 templates have a tractable subset, and the `<script>` block can host Go directly when we declare expressions are Go-native.
- Once expressions are Go, we can run `go/parser.ParseExpr` at codegen for validation — type errors surface at build, not runtime.

### Self-rules

1. When the user says "I want X performance," check whether the chosen runtime can ever reach it. If not, propose a different architecture before more polyfill work.
2. Performance ceilings are hard. The runtime defines the max throughput; nothing above it is recoverable via code.
3. Familiar shape (file convention, mental model) is the actual product. Don't conflate it with the upstream implementation.
4. Codegen beats runtime interpretation for SSR every time — static decisions cost nothing per request.

### Decisions captured

- Repo: `binsarjr/sveltego` (private at start).
- Build tool: pure Go. No Node/Bun runtime on the server. Vite stays at build time for the client bundle.
- Expressions: Go-native (PascalCase fields, `nil`, `len()`). No JS-to-Go translator.
- Target: Svelte 5 (runes) only. Skip Svelte 4 legacy syntax.
- Performance target: 20–40k rps for mid-complexity SSR.

