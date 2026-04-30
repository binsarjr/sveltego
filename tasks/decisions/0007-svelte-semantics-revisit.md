# ADR 0007 — Svelte Semantics Revisit (Go-mustache vs Full-Svelte)

- **Status:** Proposed
- **Date:** 2026-04-30
- **Authors:** binsarjr, orchestrator
- **Issue:** [binsarjr/sveltego#309](https://github.com/binsarjr/sveltego/issues/309)
- **Supersedes (if Accepted):** sections of [ADR 0001](0001-parser-strategy.md) and [ADR 0002](0002-expression-syntax.md) covering expression syntax and template semantics. Status remains **Proposed** until the team converges.
- **Related:** [ADR 0003](0003-file-convention.md), [ADR 0004](0004-codegen-shape.md), [ADR 0005](0005-non-goals.md), Issue #174 (file-naming RFC).

> Working document. Not a dictation. The recommendation below is the
> authors' starting position; reviewers are asked to challenge it on the
> "Open questions" list.

## Context

### What ADR 0001 / 0002 / 0005 locked, and why

The framework treats `.svelte` files as **Go-decorated templates**, not
JavaScript. Three coupled decisions made this concrete:

- **ADR 0001** picked a hand-rolled recursive-descent Svelte parser that
  emits Go AST.
- **ADR 0002** declared mustache expressions are full Go expressions
  validated by `go/parser.ParseExpr` — `{Data.User.Name}`, `{len(Posts)}`,
  `nil` not `null`, PascalCase fields. Channel ops and `make`/`new`/`delete`
  are rejected at codegen.
- **ADR 0005** declared "no JS runtime on the server" as a hard non-goal.
  Universal load, `<script context="module">`, runtime template
  interpretation, JSDoc-driven types, `vitePreprocess` were all rejected
  for the same reason.

The pivot rationale (`tasks/lessons/2026-04-29-pivot-to-go-native-rewrite.md`)
named the SvelteKit *shape* (file convention, Load/Actions/hooks, layouts)
as the actual product — not the SvelteKit *implementation*. Codegen beats
runtime interpretation for SSR; the Go runtime defines the achievable
ceiling and a JS runtime would cap it permanently below that.

### What the user is now asking

Reopened in Bahasa, paraphrased verbatim:

> "kenapa nggk sperri svletkeit aja, klau udh nyentuh .svelte itu full
> sveltejs aja, kita ckup inject data aja lewat speerti $page dll #await
> dll gitu."

English paraphrase: _"Why don't we just do it like SvelteKit — once a file
is `.svelte`, treat it as full Svelte/JS. We just inject data via things
like `$page`, `#await`, etc."_

This re-opens both ADR 0001 (parser owns Svelte semantics) and ADR 0002
(expressions are Go). It does **not** automatically re-open ADR 0005's "no
JS runtime" rule — depending on which interpretation we adopt, the JS
runtime may or may not return.

### Re-interpretations on the table

The user's quote admits at least five readable interpretations. We
spell each out with the same `+page.svelte` to keep the comparison honest.

Reference Load function (server-side, identical across all options):

```go
// src/routes/posts/[slug]/page.server.go
//go:build sveltego
package posts_slug

func Load(ctx *kit.LoadCtx) (PageData, error) {
    return PageData{
        User:  User{Name: "Ada"},
        Posts: []Post{{Title: "Hello"}, {Title: "World"}},
    }, nil
}
```

#### Option A — Status quo (Go-mustache, codegen owns rendering)

```svelte
<!-- src/routes/posts/[slug]/+page.svelte -->
<script lang="go">
    type PageData struct {
        User  User
        Posts []Post
    }
</script>

<h1>Hi {Data.User.Name}</h1>
{#if len(Data.Posts) > 0}
    <ul>
    {#each Data.Posts as post}
        <li>{post.Title}</li>
    {/each}
    </ul>
{/if}
```

Mustache holds Go. Codegen lowers `{#if}` / `{#each}` to Go control flow.
No JS on the server. Today's shipped behaviour.

#### Option B — Full-Svelte semantics with embedded JS runtime on server

```svelte
<!-- src/routes/posts/[slug]/+page.svelte -->
<script>
    let { data } = $props();
</script>

<h1>Hi {data.user.name}</h1>
{#if data.posts.length > 0}
    <ul>
    {#each data.posts as post}
        <li>{post.title}</li>
    {/each}
    </ul>
{/if}
```

JS expressions, runes, full Svelte 5 semantics. Server runs Svelte's
official SSR via an embedded JS runtime (goja, v8go, or a Bun subprocess).
Go orchestrates: runs `Load`, marshals the result to JSON, hands it to the
runtime as `$props().data`, captures the rendered HTML.

#### Option C — CSR-only (no SSR)

Same `.svelte` source as Option B. Server returns:

```html
<!doctype html>
<html><head><link rel="modulepreload" href="/_app/entry.js"></head>
<body>
  <div id="app"></div>
  <script type="application/json" id="__sveltego_data__">
    {"user":{"name":"Ada"},"posts":[{"title":"Hello"},{"title":"World"}]}
  </script>
  <script type="module" src="/_app/entry.js"></script>
</body></html>
```

Vite-built client bundle hydrates and renders in the browser. Server-side
Go does load + JSON serialization only. No SSR.

#### Option D — Full-Svelte semantics via Go-implemented Svelte VDOM

```svelte
<!-- same source as Option B -->
```

Go-side hosts its own minimal JS-expression evaluator + Svelte runtime
walker. We translate `$state`, `$derived`, `$effect`, `{#await}`, `{#if}`
to Go evaluation. The framework re-implements (a subset of) the Svelte 5
runtime in Go.

#### Option E — Status quo with cosmetic sugar

```svelte
<!-- src/routes/posts/[slug]/+page.svelte -->
<script lang="go">
    type PageData struct {
        User  User
        Posts []Post
    }
</script>

<h1>Hi {user.name}</h1>           <!-- sugar: rewritten to {Data.User.Name} -->
{#if len(posts) > 0}              <!-- sugar: posts → Data.Posts -->
    <ul>
    {#each posts as post}
        <li>{post.title}</li>     <!-- sugar: post.title → post.Title -->
    {/each}
    </ul>
{/if}
```

Codegen heuristically rewrites lowercase-first identifiers to PascalCase
field accesses on `Data`. Opt-in (or off-by-default). A thin compatibility
layer; semantically still Option A.

## Tradeoffs matrix

Scale: `--` strongly negative · `-` negative · `~` neutral · `+` positive
· `++` strongly positive.

| Criterion | A (status quo) | B (JS runtime SSR) | C (CSR-only) | D (Go VDOM) | E (sugar) |
|---|---|---|---|---|---|
| Performance — SSR throughput vs 20-40k rps target | ++ (Go-only, codegen, no per-req walk) | -- (V8 ≈ unknown ceiling under cgo; goja ≈ 20× slower than V8 on CPU work; Bun subprocess pays IPC) | n/a (no SSR) | -- (interpreter on hot path; codegen advantage gone) | ++ (identical to A at runtime) |
| DX for Svelte devs — `{name}` "just works" | -- (must learn `Data.User.Name`, PascalCase, `nil`, `len`) | ++ (literal SvelteKit) | ++ | ++ | + (sugar fixes the surface complaint without leaving Go semantics) |
| DX for Go devs — Go type safety in templates | ++ (`go vet` sees template expressions, IDE jump-to-def works via `go/parser`) | -- (JS in templates, Go in handlers — mental context switch) | -- | - (custom evaluator means custom toolchain) | + (sugar layer must round-trip through `go/parser` to keep type checks) |
| Complexity / maintenance burden (code we must own) | + (parser + codegen, ~done) | - (binding layer, payload bridge, error mapping, sandbox, version-pin Svelte compiler) | + (smaller server; bigger client config) | -- (writing a Svelte runtime in Go = years of catch-up to upstream features) | ~ (sugar pass = one AST rewrite ~200-500 LOC) |
| Toolchain footprint — do we ship a JS runtime? | ++ (no) | -- (v8go ≈ 40-80MB binary, goja ≈ pure Go but slow, Bun ≈ ~58MB single-file binary) | ~ (Vite + bundle, no server JS) | ++ (no) | ++ (no) |
| Hydration & SPA continuity (v0.3 client SPA, #34-#42) | ~ (separate Go SSR + Vite client; payload bridge already planned) | ++ (server and client run the same compiled Svelte; perfect hydration parity) | ++ (client is the truth) | ~ (hydration bridge must mirror Go evaluator state) | ~ (same as A) |
| a11y / SEO — works without JS | ++ (HTML out of the box) | ++ (SSR HTML out of the box) | -- (CSR-only fails for crawlers without JS, slow LCP) | ++ | ++ |
| Security — XSS surface, sandboxing | ++ (Go escape on the path; no eval) | - (sandbox the JS runtime, isolate per-request VM, manage memory leaks; goja docs say single-goroutine per Runtime) | ~ (XSS lives client-side; server only emits JSON) | + (no JS eval, but our evaluator is the new attack surface) | ++ (same as A) |
| Migration cost from current code | ++ (zero) | -- (rewrite codegen + invalidate goldens + rewrite playgrounds; ~28 closed v0.1/v0.2 issues become moot or need rework) | -- (kills the Go SSR proposition; rewrite server pipeline) | -- (similar to B; plus we own the runtime) | + (additive: existing PascalCase code keeps working) |

### Notes on the matrix

- **Performance ceiling for Option B is unknown but almost certainly
  below A.** goja's author benchmarks goja ≈ 20× slower than V8 on pure
  CPU; V8-via-cgo costs a context switch per call; Bun-as-subprocess pays
  IPC plus a >50MB runtime per worker. A targeted spike (issue 1 below)
  can put real numbers behind these estimates.
- **Option C is not "free hydration"** — SEO, no-JS users, and slow-network
  TTFB suffer. The framework's identity ("Go-native rewrite of SvelteKit
  shape") explicitly includes SSR.
- **Option D is the largest unknown.** Re-implementing `$state` /
  `$derived` / `$effect` / `{#await}` / `{#each (key)}` plus the Svelte
  reactivity graph in Go is not a "spike"-sized task. We'd be permanently
  chasing upstream Svelte changes.
- **Option E is the cheapest mitigation of the original complaint.** The
  user's pain ("kenapa harus PascalCase") is largely a surface-syntax
  irritation, not a runtime requirement. A sugar layer can take it without
  reopening any of A's load-bearing decisions.

## Recommendation (challengeable)

**Authors' starting position: stay on Option A. Add Option E as an opt-in
sugar layer iff a sustained user request emerges.** Reject B, C, D for
the reasons in the matrix.

Rationale:

1. **Option A's case has not weakened.** The "no JS runtime on the
   server" line was not drawn for ideology; it was drawn because the JS
   runtime defines the throughput ceiling and the team has chosen
   throughput as a first-class goal. Nothing new in the user's question
   contradicts that — the question is about *DX*, not *throughput*.
2. **Option B reopens a settled non-goal.** ADR 0005 sub-decision 1
   retained "no JS runtime on server" as Category 1 of the canonical
   non-goals list. Reopening it requires new evidence (Spike 1).
3. **Option D is a years-long detour.** The Svelte 5 runtime is a moving
   target; cloning it in Go means the framework's primary shipped value
   is "we re-implemented Svelte." Out of proportion to the gain.
4. **Option C kills SSR.** The framework's identity is SSR-first. Crawler
   support, no-JS fallback, and TTFB are not negotiable for the audience
   we are targeting (Go web apps with SEO needs).
5. **Option E addresses 80% of the user's concern at ~0% of the
   architectural cost.** If the actual pain is "I have to write
   `Data.User.Name`," sugar handles that. If the pain is deeper (full
   Svelte semantics, runes on the server), Spike 5 will show how far
   sugar can stretch before it breaks down.

We are explicitly not declaring this final. The team should challenge:

- Are we underweighting DX? Svelte's user base values DX over raw
  throughput; if our target audience is "Svelte devs who want Go
  performance," Option B's DX advantage is large and our throughput
  argument may be over-weighted.
- Is the throughput claim still true under a Bun-subprocess model that
  amortizes the runtime cost across many requests?
- Does Option E hide a bigger problem (lossy rewrites, ambiguous
  identifiers) that turns it into a long-tail bug factory?

## Open questions for the team

Numbered for direct citation in issue comments:

1. **Throughput floor.** What is the minimum SSR throughput we'd accept
   in exchange for full-Svelte DX? "20-40k rps" is the published target;
   would 5-10k rps with literal SvelteKit semantics be a better product?
2. **Audience.** Are we primarily serving (a) Go devs who want
   SvelteKit-shape file convention, or (b) Svelte devs who want Go
   deployment? A and B point at different defaults.
3. **DX dealbreaker.** Is `{name}` over `{Data.User.Name}` a dealbreaker
   for users you've talked to? Or are they fine once the convention is
   documented?
4. **JS runtime tolerance.** Would we ship a >50MB binary (Bun-embedded)
   or a CGO dependency on V8 (v8go) if it bought us literal
   SvelteKit-pure DX? Cloudflare Workers adapter is in scope (ADR 0005);
   v8go cgo would block that path.
5. **Sugar vs semantics.** Do we treat the gap as a documentation /
   sugar issue (Option E + better docs), or as a fundamental
   architecture issue (Option B/C/D)?
6. **Hydration model.** v0.3 (#34-#42, #84, #85) is shipping a SPA
   client. Does choice of A/B/C/D/E affect the hydration payload shape we
   commit to in #34?
7. **Compatibility window.** If Option B/C/D wins, what's our story for
   the 28 closed v0.1/v0.2 issues? Migration shim, deprecation period,
   or fork the repo?
8. **Ecosystem signal.** Are there v0.1/v0.2 user complaints in the
   issue tracker pointing at this gap? (Not aware of any, but worth
   confirming before the discussion thread closes.)
9. **Performance budget for the sugar pass.** If we adopt Option E, what
   is the acceptable codegen overhead for the lowercase-first rewrite
   pass? Sub-millisecond per template feels right but should be
   measured.
10. **Scope of `$page` / `#await` shim.** The user mentioned `$page`,
    `$navigating`, `{#await}` as specific "things to inject." Some of
    these are pure conventions over the existing data flow (we can
    expose `$page.data` as a Go-side helper today). Is the request
    actually "give me a `$page`-shaped accessor" rather than "give me
    full Svelte semantics"? See Spike 2.

## Conditional next steps

If Option A holds (with optional E):

- Close this RFC as **Accepted: Option A retained, Option E spike
  filed**. Update ADR 0001/0002 with a "revisited 2026-04-30, no change"
  amendment. Maybe land Spike E.
- Pause #174 only if the wider naming question follows from this; close
  it with a parity reference if E ships.

If Option B/C/D wins:

- Open a migration plan ADR (0008+).
- File blocker issues for: codegen rewrite, golden test invalidation,
  playground rewrites, Vite/runtime version pinning, deploy adapter
  story (especially Cloudflare Workers under cgo).
- Mark this ADR **Accepted** and ADR 0001 / 0002 **Superseded by 0007**.

## Acceptance for closing this RFC

- Team picks a single option (or hybrid combination).
- ADR moves from **Proposed** → **Accepted** / **Rejected** /
  **Superseded**.
- If Option B/C/D is chosen, migration-plan ADR opens before any code
  lands.

## References

- [ADR 0001 — Parser Strategy](0001-parser-strategy.md)
- [ADR 0002 — Template Expression Syntax](0002-expression-syntax.md)
- [ADR 0003 — File Convention](0003-file-convention.md)
- [ADR 0004 — Codegen Output Shape](0004-codegen-shape.md)
- [ADR 0005 — Non-Goals](0005-non-goals.md)
- Lesson: [pivot to Go-native rewrite](../lessons/2026-04-29-pivot-to-go-native-rewrite.md)
- Lesson: [initial R&D — JS runtime survey](../lessons/2026-04-29-initial-rd.md)
- SvelteKit `$app/state` docs: <https://svelte.dev/docs/kit/$app-state>
- SvelteKit Loading data: <https://svelte.dev/docs/kit/load>
- SvelteKit page options: <https://svelte.dev/docs/kit/page-options>
- SvelteKit hydration mechanics (devalue payload): <https://www.captaincodeman.com/sveltekit-hydration-gotcha>
- goja (pure-Go ECMAScript runtime): <https://github.com/dop251/goja>
  — author benchmark: ≈ 20× slower than V8 on pure CPU work; ES 5.1
  full, ES6 ≈ 80% partial.
- v8go (V8 cgo binding): <https://github.com/rogchap/v8go> — V8 static
  library exceeds 100 MB per platform; final Go binaries typically
  40-80 MB after stripping.
- Bun runtime size: <https://github.com/oven-sh/bun/issues/14546> —
  `bun build --compile` produces ~58 MB binaries vs Go's ~7 MB
  equivalent.
- Bun blog v1.3.13: <https://bun.com/blog/bun-v1.3.13>
- File-naming RFC #174: <https://github.com/binsarjr/sveltego/issues/174>
- Auth master plan #155: <https://github.com/binsarjr/sveltego/issues/155>
