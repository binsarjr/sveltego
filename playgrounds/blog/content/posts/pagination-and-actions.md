---
title: Pagination and actions
date: 2026-04-29
summary: List with ?page=N. Comments via Actions(). In-memory store for the demo.
---

# Pagination and actions

This playground demonstrates a few pieces working together:

- The index page paginates with `?page=N`. The `Load()` reads the query
  string off `ctx.URL.Query()`.
- The post detail page declares an `Actions` map. The default action
  appends a new comment and redirects back to the post.
- Comments live in an in-memory store keyed by post slug. No database;
  on restart the demo resets.

Real production would back this with Postgres, but the framework shape
stays the same.
