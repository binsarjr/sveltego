package server

// WriteRaw appends v to the payload body buffer without HTML escape.
// It is the codegen target for `{@html expr}` (issue #445): mirrors
// Svelte 5's compiled-server behavior where `$.html(value)` bypasses
// the escape table and writes the raw string into the body.
//
// Trusted content invariant: callers are responsible for sanitizing
// untrusted input before handing it to WriteRaw. Mirrors Svelte's
// `{@html}` semantics — sveltego does not bundle a sanitizer because
// the appropriate strategy is data-shape specific (markdown renderer,
// rich-text whitelist, OG embed, …) and belongs at the data-load
// layer rather than the render layer.
//
// Stringify covers the value-to-string conversion so callers can pass
// through untyped expressions without a per-shape cast — matches the
// codegen pattern where compiled output emits the bare expression and
// the helper handles the type dispatch.
func WriteRaw(p *Payload, v any) {
	if p == nil {
		return
	}
	switch s := v.(type) {
	case nil:
		return
	case string:
		p.Out.WriteString(s)
	case []byte:
		p.Out.Write(s)
	default:
		p.Out.WriteString(Stringify(v))
	}
}

// Truthy reports whether v is truthy under JavaScript-like semantics,
// the predicate Svelte's compiled-server output expects from `{#if x}`
// when x is not statically a bool. Issue #443 wires this in for
// `{@const}` lowering where the binding's Go type is a struct field
// (string, []T, *T, numeric, …) rather than a bool.
//
// Falsy values: nil, false, 0 / 0.0, "", empty slice/map. Anything
// else is truthy.
func Truthy(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int8:
		return x != 0
	case int16:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case uint:
		return x != 0
	case uint8:
		return x != 0
	case uint16:
		return x != 0
	case uint32:
		return x != 0
	case uint64:
		return x != 0
	case float32:
		return x != 0
	case float64:
		return x != 0
	case []byte:
		return len(x) > 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	}
	return true
}
