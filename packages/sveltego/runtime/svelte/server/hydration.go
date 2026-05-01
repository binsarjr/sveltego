package server

// Hydration markers mirror svelte/internal/server/hydration.js exactly.
// Their byte values are part of Svelte's hydration protocol: any change
// here means the client runtime can no longer pair anchors with server
// output, so these constants are load-bearing.
const (
	EmptyComment  = "<!---->"
	BlockOpen     = "<!--[-->"
	BlockOpenElse = "<!--[!-->"
	BlockClose    = "<!--]-->"
)
