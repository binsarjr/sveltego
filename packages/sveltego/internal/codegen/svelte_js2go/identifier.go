package sveltejs2go

import "strings"

// mangleIdent rewrites a JS identifier into a Go-legal identifier.
// Svelte's compiled output uses `$$prefix` style for emitter-internal
// names ($$index, $$length, $$payload, $$props, $$renderer); Go does
// not allow `$` in identifiers, so we substitute the leading run of
// dollar signs with a fixed marker. The mapping is round-trippable for
// debugging but the inverse isn't needed at runtime.
//
// Examples:
//
//	$$index    → ssvar_index
//	$$length   → ssvar_length
//	$$payload  → ssvar_payload
//	data       → data       (untouched)
//	each_array → each_array (untouched)
//
// The "ssvar_" prefix is short enough to keep generated lines readable
// and unlikely to collide with user identifiers (Svelte template
// authors don't write Go identifiers with that prefix).
func mangleIdent(name string) string {
	if name == "" {
		return name
	}
	if !strings.ContainsRune(name, '$') {
		return name
	}
	// Strip the leading $$ run; keep any trailing $ as a literal-safe
	// underscore to remain a valid identifier.
	trimmed := strings.TrimLeft(name, "$")
	if trimmed == "" {
		trimmed = "anon"
	}
	rest := strings.ReplaceAll(trimmed, "$", "_")
	return "ssvar_" + rest
}
