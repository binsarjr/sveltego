---
layout: home
title: sveltego
order: 0
summary: SvelteKit-shape framework for Go. Pure-Svelte templates, Go-only server, no JS runtime at request time.

hero:
  name: sveltego
  text: SvelteKit shape, in pure Go
  tagline: Pure-Svelte templates, Go-only server. Hybrid SSG + SPA runtime, no JS engine on the request path.
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
    details: Go-only on the request path. No goja, v8go, or Bun. Node runs only at build time for SSG prerender.
  - title: Familiar conventions
    details: _page.svelte, _page.server.go, _layout.svelte, _error.svelte, hooks.server.go. Same shape as SvelteKit.
  - title: 100% pure Svelte templates
    details: '{data.user.name}, {data.posts.length}, null not nil. Standard Svelte runes; Svelte LSP works without a fork.'
  - title: Svelte 5 runes
    details: $props, $state, $derived, $effect, $bindable. No legacy reactivity.
  - title: Codegen, not interpretation
    details: Go AST → .svelte.d.ts for IDE autocomplete. Build-time SSG for prerendered routes; SPA for the rest.
  - title: AI-native docs
    details: llms.txt, llms-full.txt, copy-for-LLM buttons, project-aware AGENTS.md.
---

## What is sveltego?

sveltego is a rewrite of SvelteKit's shape in pure Go. The file conventions, the data-loading model, the error boundaries, the form actions, the hooks pipeline — all of it lands in Go without a JS runtime on the server.

`.svelte` files are 100% pure Svelte/JS/TS — runes, JS expressions, lowercase props (`{data.user.name}`). Server-side Go files own data loading; codegen reads their Go AST and emits sibling `.svelte.d.ts` declaration files so Svelte LSP autocompletes `data.*` end to end. Vite produces the client bundle. The runtime is hybrid: build-time static prerender (SSG) for routes opted in via `kit.PageOptions{Prerender: true}`, client-side render (SPA) for everything else. The deployed Go binary has no JS engine. See [ADR 0008](https://github.com/binsarjr/sveltego/blob/main/tasks/decisions/0008-pure-svelte-pivot.md).

## Why a rewrite?

Adapters layered on top of SvelteKit-the-JS-server inherit every limitation of the chosen runtime. The SvelteKit *shape* (file convention, mental model) is what users want — not the SvelteKit *implementation*. A native Go target unlocks goroutines, `context.Context`, and the Go standard library on every request path.

See the [Quickstart](/guide/quickstart) to scaffold your first app, or the [migration guide](/guide/migration) if you are coming from SvelteKit.
