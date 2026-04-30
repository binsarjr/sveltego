---
title: Codegen, not interpretation
date: 2026-04-08
summary: Static decisions at build time. Zero per-request template walking.
---

# Codegen, not interpretation

Most SSR frameworks walk a template tree on every request. sveltego
compiles each `.svelte` template to a Go function so the server only
executes the path the request actually takes.

## What this buys

1. No per-request allocation for AST nodes.
2. Type errors surface at build time, not under load.
3. The Go compiler gets a chance to inline render code.

The cost is one extra step in the build pipeline. We pay it once.
