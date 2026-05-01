package server

// SpreadProps merges multiple prop maps left-to-right. Later maps override
// earlier ones. Mirrors svelte/internal/server.spread_props but ignores the
// JS property-descriptor copy because Go maps don't have getters.
func SpreadProps(maps ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// RestProps returns a copy of props with the listed keys removed. Mirrors
// svelte/internal/server.rest_props.
func RestProps(props map[string]any, rest ...string) map[string]any {
	skip := make(map[string]struct{}, len(rest))
	for _, k := range rest {
		skip[k] = struct{}{}
	}
	out := make(map[string]any, len(props))
	for k, v := range props {
		if _, ok := skip[k]; ok {
			continue
		}
		out[k] = v
	}
	return out
}

// SanitizeProps drops Svelte-internal keys ($$slots, children) before
// passing user props through. Mirrors svelte/internal/server.sanitize_props.
func SanitizeProps(props map[string]any) map[string]any {
	out := make(map[string]any, len(props))
	for k, v := range props {
		if k == "children" || k == "$$slots" {
			continue
		}
		out[k] = v
	}
	return out
}

// SanitizeSlots returns the slot-name set from props["$$slots"] plus a
// default slot when props["children"] is set. Mirrors
// svelte/internal/server.sanitize_slots.
func SanitizeSlots(props map[string]any) map[string]bool {
	out := map[string]bool{}
	if _, ok := props["children"]; ok {
		out["default"] = true
	}
	if slots, ok := props["$$slots"].(map[string]any); ok {
		for k := range slots {
			out[k] = true
		}
	}
	if slots, ok := props["$$slots"].(map[string]bool); ok {
		for k := range slots {
			out[k] = true
		}
	}
	return out
}

// Fallback returns value when non-nil, else dflt. Mirrors
// svelte/internal/shared/utils.fallback.
func Fallback(value, dflt any) any {
	if value == nil {
		return dflt
	}
	return value
}

// ExcludeFromObject returns a copy of m with the keys in exclude removed.
// Mirrors svelte/internal/shared/utils.exclude_from_object.
func ExcludeFromObject(m map[string]any, exclude ...string) map[string]any {
	skip := make(map[string]struct{}, len(exclude))
	for _, k := range exclude {
		skip[k] = struct{}{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		if _, ok := skip[k]; ok {
			continue
		}
		out[k] = v
	}
	return out
}

// EnsureArrayLike normalizes the iterable input to a []any. Mirrors
// svelte/internal/server.ensure_array_like for the {#each} block setup.
// nil → empty; []any pass-through; []string and friends are wrapped.
func EnsureArrayLike(v any) []any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []any:
		return x
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case []int:
		out := make([]any, len(x))
		for i, n := range x {
			out[i] = n
		}
		return out
	case []map[string]any:
		out := make([]any, len(x))
		for i, m := range x {
			out[i] = m
		}
		return out
	}
	return nil
}
