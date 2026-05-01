package server

// Element renders a dynamic HTML element. Mirrors svelte/internal/server.element:
// emits <!----> sentinels, opens the tag, runs attributes, runs children,
// closes the tag (unless void). Raw-text elements (script/style) skip the
// inner empty-comment marker.
//
// attrs and children may be nil for the no-op cases.
func Element(p *Payload, tag string, attrs func(*Payload), children func(*Payload)) {
	p.Push(EmptyComment)
	if tag != "" {
		p.Push("<" + tag)
		if attrs != nil {
			attrs(p)
		}
		p.Push(">")
		if !IsVoidElement(tag) {
			if children != nil {
				children(p)
			}
			if !IsRawTextElement(tag) {
				p.Push(EmptyComment)
			}
			p.Push("</" + tag + ">")
		}
	}
	p.Push(EmptyComment)
}

// Head writes a head fragment with the Svelte-emitted hash marker. Mirrors
// svelte/internal/server.head: pushes <!--hash-->, runs fn against the head
// buffer, then pushes the empty-comment terminator.
func Head(p *Payload, hash string, fn func(*Payload)) {
	p.PushHead("<!--" + hash + "-->")
	if fn != nil {
		// Temporarily swap Out for Head so the callback's Push lands in head.
		// Compiled output uses $$payload.head which is just another Builder.
		var inner Payload
		fn(&inner)
		p.Head.WriteString(inner.Out.String())
		if inner.Title != "" {
			p.Title = inner.Title
		}
	}
	p.PushHead(EmptyComment)
}
