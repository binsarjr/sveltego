package server

import "strings"

// Payload mirrors Svelte's $$payload object passed through compiled server
// render functions. Out collects the body, Head collects <svelte:head>
// contributions; Title is the latest <title> seen during render.
//
// Request-scoped fields the SSR templates can read (CSP nonce, CSRF
// token, route metadata) live on the sibling PageState struct — that is
// what the per-route bridge constructs from kit.RenderCtx and forwards
// into the transpiled Render function. See page_state.go.
type Payload struct {
	Out   strings.Builder
	Head  strings.Builder
	Title string
}

// Push appends a literal string to the body buffer. Mirrors $$payload.out += s
// in Svelte's compiled output.
func (p *Payload) Push(s string) {
	p.Out.WriteString(s)
}

// PushHead appends to the head buffer. Mirrors $$payload.head += s.
func (p *Payload) PushHead(s string) {
	p.Head.WriteString(s)
}

// Body returns the rendered body so far.
func (p *Payload) Body() string {
	return p.Out.String()
}

// HeadHTML returns the rendered head contributions so far.
func (p *Payload) HeadHTML() string {
	return p.Head.String()
}

// Reset clears both buffers and the title; useful for pooled reuse.
func (p *Payload) Reset() {
	p.Out.Reset()
	p.Head.Reset()
	p.Title = ""
}
