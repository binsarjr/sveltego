---
layout: home
title: sveltego
order: 0
summary: SvelteKit-shape framework for Go. SSR via codegen, no JS server runtime.

hero:
  name: sveltego
  text: SvelteKit shape, in pure Go
  tagline: Server-side rendering via codegen. Go expressions in templates. No JS runtime on the server.
  actions:
    - theme: brand
      text: Quickstart
      link: /guide/quickstart
    - theme: alt
      text: Reference
      link: /reference/kit
    - theme: alt
      text: GitHub
      link: https://github.com/binsarjr/sveltego

features:
  - title: Pure Go server
    details: .svelte templates compile to .gen/*.go. No goja, v8go, or Bun on the request path.
  - title: Familiar conventions
    details: _page.svelte, page.server.go, _layout.svelte, _error.svelte, hooks.server.go. Same shape as SvelteKit.
  - title: Go expressions
    details: '{Data.User.Name}, {len(Data.Posts)}, nil not null. Validated at codegen via go/parser.'
  - title: Svelte 5 runes
    details: $props, $state, $derived, $effect, $bindable. No legacy reactivity.
  - title: Codegen, not interpretation
    details: Static decisions at build time. Targeted 20–40k rps for mid-complexity SSR.
  - title: AI-native docs
    details: llms.txt, llms-full.txt, copy-for-LLM buttons, project-aware AGENTS.md.
---

## What is sveltego?

sveltego is a rewrite of SvelteKit's shape in pure Go. The file conventions, the data-loading model, the error boundaries, the form actions, the hooks pipeline — all of it lands in Go without a JS runtime on the server.

`.svelte` templates compile to Go source. The `<script>` block hosts Go directly; mustache expressions (`{...}`) are Go expressions, validated at codegen via `go/parser`. Vite is retained at build time only, for the client hydration bundle.

## Why a rewrite?

Adapters layered on top of SvelteKit-the-JS-server inherit every limitation of the chosen runtime. The SvelteKit *shape* (file convention, mental model) is what users want — not the SvelteKit *implementation*. A native Go target unlocks goroutines, `context.Context`, and the Go standard library on every request path.

See the [Quickstart](/guide/quickstart) to scaffold your first app, or the [migration guide](/guide/migration) if you are coming from SvelteKit.
