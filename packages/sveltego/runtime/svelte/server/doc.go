// Package server mirrors Svelte's svelte/internal/server helper surface in
// pure Go. The Phase-3 transpiler emits calls to these helpers from the
// shapes Svelte's compiler produces under generate:'server'.
//
// Stability: experimental until Phase 6 lands the request pipeline. See
// STABILITY.md and ADR 0009 (tasks/decisions/0009-ssr-option-b.md).
package server
