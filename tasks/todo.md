# svelte-adapter-golang — Feasibility & Plan

## Tujuan

SvelteKit adapter yang produce Go binary mandiri:
- Go = HTTP server, static file, routing, IPC ke JS runtime
- JS runtime embedded = jalankan SvelteKit `Server.respond(request)` SSR

User minta: embed struktur JS yang ada (SvelteKit bundle) ke Go.

---

## Kontrak SvelteKit Server (yg harus dipenuhi runtime JS)

`new Server(manifest).respond(request, opts) -> Promise<Response>`

Dependency runtime:
- Web standards: `Request`, `Response`, `Headers`, `URL`, `URLSearchParams`, `ReadableStream`, `TextEncoder`/`Decoder`, `globalThis.fetch`
- `crypto.getRandomValues`, `crypto.subtle` (SHA256 untuk CSP nonce)
- ES Modules + `import()` dynamic
- `Promise`, `async/await`
- `AsyncLocalStorage` (`node:async_hooks`) — **opsional**, fallback ke webcontainer mode (single-request serial) bila tak ada

---

## Kandidat Runtime — Ringkasan

| Runtime | ESM | Dynamic Import | Web APIs | CGO | Cross-compile | Perf est. | Notes |
|---|---|---|---|---|---|---|---|
| **goja** (pure Go) | parsial | ❌ | ❌ | tidak | mulus | ~200-1000 rps | bundle harus IIFE/CJS, polyfill berat |
| **v8go** | ✅ | ✅ | ❌ (polyfill) | ya | susah (prebuilt V8 per target) | 5-15k rps | binary 50-80MB, maintenance lag |
| **quickjs-go** (QuickJS-NG) | ✅ | ✅ | ❌ (polyfill) | ya | OK via zig cc | 1-3k rps | binary 5-10MB |
| **wazero+javy** | ❌ | ❌ | ❌ | tidak | mulus | ~100-500 rps | tidak fit untuk SSR |
| **subprocess Bun** | ✅ native | ✅ | ✅ native | tidak | mulus (vendor binary) | 3-8k rps | bukan true embed, +50MB binary |

---

## Jalur GOJA (pilihan user)

Realistis tapi butuh effort polyfill berat. Trade-off:

### Pro
- Pure Go → cross-compile mulus, single binary kecil, no CGO
- Mature, production-tested (k6 Grafana)
- Lifecycle goroutine bersih

### Kontra
- ES5.1 + sebagian ES6 → bundle SvelteKit harus di-down-level
- Tidak ada ESM native → bundle output harus IIFE/CJS, dynamic import jadi sync require
- Tidak ada event loop bawaan → wajib `goja_nodejs/eventloop`
- Web APIs nol → polyfill tulis sendiri (Request/Response/Headers/URL/fetch/Streams/crypto)
- `AsyncLocalStorage` tidak ada → pakai webcontainer mode SvelteKit (serialize request, kerugian throughput)
- Performa sekitar 10-50× lebih lambat dari V8

### Polyfill Wajib
1. **`Request`/`Response`/`Headers`/`URL`/`URLSearchParams`** — `globalThis` injection dari Go (bridge ke `net/http`)
2. **`ReadableStream`** — minimal impl, atau buffer-only mode
3. **`TextEncoder`/`Decoder`** — wrap `unicode/utf8`
4. **`fetch`** — bridge ke `net/http.Client`
5. **`crypto`** — bridge ke `crypto/sha256` + `crypto/rand`
6. **`process.env`** — `os.Environ()`
7. **`console.*`** — log → stdout
8. **Module loader** — bundle SvelteKit ke single CJS file, expose `require` via `goja_nodejs/require`
9. **Microtask queue** — `goja_nodejs/eventloop`

### Build Pipeline
```
SvelteKit src
  ↓ writeServer + writeClient (builder)
  ↓ esbuild bundle target=es2017, format=cjs, single file
  ↓ patch dynamic import → require
  ↓ go:embed bundle.js + manifest.js + client/ + prerendered/
  ↓ go build -o app
```

### Komponen Go Adapter
- `src/index.js` — adapter SvelteKit (Node/Bun side, panggilan dari `vite build`)
- `runtime/` — Go template:
  - `main.go` — HTTP server, embed FS, init goja
  - `bridge.go` — polyfill Web APIs
  - `eventloop.go` — wrapper goja_nodejs/eventloop
  - `fetch.go` — bridge fetch ke net/http
- `bundler.js` — invoke esbuild dengan plugin polyfill

### Risiko Utama
1. **Top-level await** di SvelteKit `index.js` (`await server.init`) — perlu wrapper IIFE async + manual drain event loop
2. **`globalThis.fetch` patching** dalam `page/render.js` — set per-request, harus thread-safe vs serialized requests
3. **Streaming responses** — goja String/Bytes ↔ Go []byte, perlu zero-copy bridge
4. **Bundle size** — SvelteKit Server + deps ~500KB-2MB minified, OK
5. **Cookie / Set-Cookie multi-value** — Headers polyfill harus support `getSetCookie()`
6. **Crypto.subtle async** — goja Promise + Go goroutine bridge

### Probabilitas Sukses
- **PoC "Hello World" SSR**: tinggi (~80%) — 1-2 minggu
- **Full SvelteKit kompatibel** (form actions, load fn, hooks, streaming): sedang (~50%) — 1-2 bulan
- **Production-grade** (perf, edge cases, websocket): rendah-sedang (~30%) — 3-6 bulan

---

## Plan Eksekusi (jika approve goja)

### Fase 0 — Setup repo (DIY, no code yet)
- [ ] init `package.json` adapter
- [ ] init Go module di `runtime/`
- [ ] sketch `tsconfig.json`, `.gitignore`

### Fase 1 — Bridge minimal
- [ ] Go: load goja, register `console`, `process.env`
- [ ] Go: implement `URL`, `Headers`, `Request`, `Response` polyfill
- [ ] Go: implement `crypto.getRandomValues` + `crypto.subtle.digest`
- [ ] Go: implement minimal `fetch` (bridge ke `net/http`)
- [ ] uji dengan JS skrip dummy

### Fase 2 — Bundler
- [ ] adapter `src/index.js` — terima Builder SvelteKit
- [ ] esbuild bundle SvelteKit Server → single CJS
- [ ] patch `import()` → `require()`
- [ ] generate manifest.js + emit ke build dir

### Fase 3 — Runtime integration
- [ ] Go HTTP handler → bangun `Request` JS → panggil `server.respond` → konversi `Response` JS → `http.ResponseWriter`
- [ ] static file server (sirv-style) di Go
- [ ] prerendered routes lookup
- [ ] uji golden path: routing, layout, slot

### Fase 4 — Edge cases
- [ ] form actions (POST + redirect)
- [ ] cookies / Set-Cookie
- [ ] streaming responses (ReadableStream)
- [ ] hooks (handle, handleFetch, handleError)
- [ ] error boundary

### Fase 5 — Perf + harden
- [ ] benchmark vs adapter-bun
- [ ] worker pool (multi-goja-runtime), karena AsyncLocalStorage tidak ada → satu runtime per goroutine
- [ ] precompress assets
- [ ] graceful shutdown

---

## Rekomendasi

User memilih goja. Goja **possible** untuk SvelteKit tapi effort polyfill ~70% dari total kerja. Saran:

1. **Mulai dari hello-world SSR** (Fase 1+2+3 minimal). Buktikan goja bisa jalankan SvelteKit Server.respond untuk satu route static. Jika lolos, lanjut ke fitur lain.
2. **Plan B**: jika polyfill terlalu menyiksa atau perf jelek, fallback ke **subprocess Bun** dengan binary embed via `go:embed`. Bukan "true embed JS engine in Go" tapi tetap "single Go binary ship".
3. **Plan C**: **v8go** untuk perf, terima cross-compile pain.

Pertanyaan untuk user sebelum lanjut:
1. Cross-compile single binary itu hard requirement? (kalau ya, goja > v8go > quickjs)
2. Target perf? (kalau >5k rps, goja kurang)
3. Boleh worker pool multi-runtime, atau strict single goroutine?
4. Streaming response wajib di MVP atau buffer-only OK?

---

## Status

- [x] Pelajari kontrak SvelteKit Server
- [x] Survei runtime
- [ ] PoC minimal (tunggu approval pendekatan)
- [x] Laporan feasibility (file ini)
