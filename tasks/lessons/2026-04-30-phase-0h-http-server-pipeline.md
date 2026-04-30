## Phase 0h — HTTP server pipeline (2026-04-30)

### Insight

- **Issue body drift is the norm, not the exception.** Issue #20 was authored before Phase 0f/0g locked the actual runtime API. The body referenced `gen.Manifest`, `pkg/server`, `route.PageHead`, `render.Pool.Get().(*render.Writer)`, and `kit.NewLoadCtx(r, w, params)` — none of those are the shipped surface. Actual surface is `gen.Routes() []router.Route`, `packages/sveltego/server/`, no `PageHead` (deferred to v0.4 `<svelte:head>`), `render.Acquire/Release`, and `kit.NewLoadCtx(r, params)` (no writer). The brief calling out drift verbatim with file paths is what kept the implementation honest. Lesson: every issue body older than the most recent foundation phase should be re-read against the actual disk before coding.
- **`sloglint` no-raw-keys + kv-only is a real constraint, not a style preference.** First pass used `"method"`/`"path"`/`"err"` raw string keys. Linter rejected. The fix is named string constants (`logKeyMethod`, `logKeyPath`, `logKeyError`, `logKeyStatus`) at package scope; values stay alongside via kv-pairs. Snake-case enforced. Bonus: grep for `logKeyError` finds every callsite that emits an error attribute.
- **`tparallel` flags `defer ts.Close()` in a parent test that has parallel subtests.** The parent's `defer` may fire while subtests are still running, causing flaky races. Fix: `t.Cleanup(ts.Close)` runs after all subtests (including parallel ones) finish. Same root cause as the `t.Parallel` requirement on subtests in a parent that calls `t.Parallel`.
- **`gosec` G112 (slowloris) on `http.Server{Addr, Handler}` literal needs `ReadHeaderTimeout`.** A 10-second default bounds the attack surface and is invisible to well-behaved clients. Users wanting custom timeouts can construct their own `http.Server` around `Server.Handler()`.
- **In-process ServeHTTP bench is microsecond-class** (~163 ns/op, 144 B/op, 4 allocs/op on Apple M1 Pro). Issue #20's "10k req/s on 4-core M-series" target is for the full server with TCP + OS network stack; the in-process number is informational only and should not be confused with end-to-end throughput.
- **`render.Acquire/Release` cycle is opaque to the caller** (the pool is package-internal). The pool-reuse test asserts the observable contract: `Acquire()` always returns a Writer with `Len() == 0`. Direct introspection of the pool would break encapsulation.

### Self-rules

1. **Re-read every API the brief references before writing the first line of code.** When an issue body says "use `X`", verify `X` is the current shipped name. Phase 0g landed three rename rounds; Phase 0h's brief explicitly listed five API drifts because the orchestrator did this re-read. Skip the re-read and you write against ghost APIs.
2. **`sloglint` named-key constants are cheap; raw string keys are linter debt.** Define `logKey*` at package scope on the first slog call. Any package logging more than two attrs gets the constant block.
3. **Parent tests with parallel subtests must use `t.Cleanup` for setup teardown, not `defer`.** The `defer` fires when the parent function returns, which is before parallel subtests run.
4. **`http.Server` literals always set `ReadHeaderTimeout`.** `gosec` G112 is non-negotiable in this repo. 10 seconds is a sane default; document overrides.
5. **Bench files for Phase 0 work include the actual measured number in the file header comment.** Future readers should see the p50 without re-running. State the platform (Apple M1 Pro) and the date.
6. **In-pool resource cycle tests assert the observable contract, not the pool internals.** `Acquire().Len() == 0` is a contract; the sync.Pool itself is unobservable.

