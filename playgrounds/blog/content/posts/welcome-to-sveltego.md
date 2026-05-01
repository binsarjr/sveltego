---
title: Welcome to sveltego
date: 2026-04-01
summary: A blog rendered by Go, scaffolded with the SvelteKit shape.
---

# Welcome

This blog is rendered by **sveltego**: a pure-Go rewrite of the SvelteKit
shape. Templates compile to Go source, expressions are Go, and there is
no JavaScript runtime on the server.

## Highlights

- File convention you already know: `_page.svelte`, `page.server.go`.
- Server-side `Load()` returns Go structs.
- Form `Actions()` for things like comments.
- Markdown rendered through `goldmark` and sanitized with `bluemonday`.

Browse to a post to read more, then leave a comment.
