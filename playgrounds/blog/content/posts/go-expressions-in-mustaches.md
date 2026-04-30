---
title: Go expressions in mustaches
date: 2026-04-15
summary: PascalCase fields, len(), nil. Validated at codegen by go/parser.
---

# Go expressions in mustaches

Mustache expressions inside `.svelte` templates are **Go**, not
JavaScript. That means:

- `{Data.User.Name}` — PascalCase struct fields.
- `{len(Data.Posts)}` — call Go builtins directly.
- `nil`, not `null`.
- `{#if Data.Authed}...{/if}` — boolean expression rules of Go.

The codegen step pipes every mustache through `go/parser.ParseExpr` so a
typo fails the build instead of the page.
