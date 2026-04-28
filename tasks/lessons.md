# Lessons — svelte-adapter-golang

## Sesi 2026-04-29 (R&D awal)

### Insight
- SvelteKit Server contract simple: `respond(Request) -> Promise<Response>`. Semua dependency = Web standards + opsional AsyncLocalStorage.
- "Webcontainer mode" SvelteKit jadi escape hatch: bila AsyncLocalStorage tak ada, runtime serialize request. Ini buka jalan untuk goja (yg gak punya ALS).
- Goja pure Go tapi bukan drop-in JS runtime modern: ESM partial, dynamic import absen, Web APIs nol.
- v8go = perf raja tapi cross-compile horror karena prebuilt V8 binding.
- subprocess Bun = paling cepat ke produksi tapi bukan "true embed".

### Aturan diri
1. Jangan klaim "embed" tanpa jelas: in-process runtime vs ship-binary. Tanya user.
2. SvelteKit bundle modern = ESM + dynamic import. Runtime tanpa keduanya butuh transpile step di adapter.
3. Polyfill Web APIs di goja = scope creep besar. Estimate effort 70%+ kerja keseluruhan.
4. Hindari klaim "production-ready" untuk PoC tahap awal — kasih tier probabilitas (PoC vs full vs prod).
