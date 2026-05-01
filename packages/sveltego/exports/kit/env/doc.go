// Package env mirrors SvelteKit's $env/static/* and $env/dynamic/*
// namespaces with four accessors: StaticPrivate, StaticPublic,
// DynamicPrivate, and DynamicPublic. Public accessors enforce a
// PUBLIC_ key prefix so leaks into client bundles are detectable;
// private accessors carry no prefix constraint and must never appear
// in _page.svelte or _layout.svelte template expressions.
//
// The Static* accessors panic on missing keys and are intended for
// values whose absence should fail process startup (DATABASE_URL,
// PUBLIC_API_URL). The Dynamic* accessors return "" on missing keys
// for runtime-toggleable settings.
//
// Build-time substitution (v0.3, #117): codegen replaces every
// env.StaticPublic("X") call in .svelte template expressions with the
// Go string literal for X's value at build time, baking the value into
// the binary. A missing key is a fatal build error. StaticPrivate calls
// in templates are rejected by the codegen private-leak guard
// (checkPrivateEnv) because the inlined value would appear in
// server-rendered HTML delivered to browsers. Dynamic* accessors are
// always resolved at request time and are never substituted.
package env
