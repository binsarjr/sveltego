// Package env mirrors SvelteKit's $env/static/* and $env/dynamic/*
// namespaces with four runtime accessors: StaticPrivate, StaticPublic,
// DynamicPrivate, and DynamicPublic. Public accessors enforce a
// PUBLIC_ key prefix so leaks into client bundles are detectable;
// private accessors carry no prefix constraint and must never appear
// in client-side code.
//
// The Static* accessors panic on missing keys and are intended for
// values whose absence should fail process startup (DATABASE_URL,
// PUBLIC_API_URL). The Dynamic* accessors return "" on missing keys
// for runtime-toggleable settings.
//
// Build-time substitution of Static* calls into literal strings, and
// the codegen guard that rejects private env references in
// +page.svelte client bundles, are deferred to the v0.3 Vite
// client-bundle work. Until then Static* and Dynamic* differ only in
// missing-key behaviour.
package env
