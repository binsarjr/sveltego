package server

import (
	"sort"
	"strings"
)

// Attr renders one HTML attribute. Mirrors svelte/internal/shared/attributes.attr:
//   - hidden becomes boolean unless value == "until-found"
//   - translate normalizes true/false to "yes"/"no"
//   - nil/false-with-isBoolean returns ""
//   - non-boolean attributes are wrapped name="escaped-value"
//   - boolean attributes render as name=""
//
// Output always has a leading space when non-empty so concatenation in
// compiled output produces the right shape.
func Attr(name string, value any, isBoolean bool) string {
	if name == "hidden" {
		if s, ok := value.(string); !(ok && s == "until-found") {
			isBoolean = true
		}
	}

	if value == nil {
		return ""
	}
	if isBoolean && isJSFalsy(value) {
		return ""
	}

	normalized := value
	if name == "translate" {
		switch v := value.(type) {
		case bool:
			if v {
				normalized = "yes"
			} else {
				normalized = "no"
			}
		}
	}

	if isBoolean {
		return ` ` + name + `=""`
	}
	return ` ` + name + `="` + EscapeHTMLAttrString(Stringify(normalized)) + `"`
}

// Clsx mirrors svelte/internal/shared/attributes.clsx. For a string/scalar:
// returns the value or "" if nil. For a map[string]any: includes keys whose
// values are JS-truthy, joined by single space. For a slice/array: each
// element is recursively flattened into class tokens. Designed to match
// the lukeed/clsx behavior Svelte vendors.
func Clsx(args ...any) string {
	switch len(args) {
	case 0:
		return ""
	case 1:
		return clsxValue(args[0])
	}
	var b strings.Builder
	for _, a := range args {
		appendClsx(&b, a)
	}
	return b.String()
}

func clsxValue(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return Stringify(x)
	case []any:
		var b strings.Builder
		appendClsx(&b, x)
		return b.String()
	case []string:
		var b strings.Builder
		for _, s := range x {
			if s == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(s)
		}
		return b.String()
	case map[string]any:
		var b strings.Builder
		appendClsx(&b, x)
		return b.String()
	case map[string]bool:
		var b strings.Builder
		appendClsx(&b, x)
		return b.String()
	}
	return ""
}

func appendClsx(b *strings.Builder, v any) {
	if v == nil {
		return
	}
	switch x := v.(type) {
	case string:
		if x == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(x)
	case []any:
		for _, e := range x {
			appendClsx(b, e)
		}
	case []string:
		for _, e := range x {
			if e == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(e)
		}
	case map[string]any:
		keys := sortedKeys(x)
		for _, k := range keys {
			if isJSTruthy(x[k]) {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(k)
			}
		}
	case map[string]bool:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if x[k] {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(k)
			}
		}
	case bool:
		// JS clsx ignores booleans
	default:
		s := Stringify(x)
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
}

// MergeStyles concatenates style fragments. Mirrors how Svelte composes
// style strings from spread + literal style attribute. Each non-empty
// fragment is normalized to end with a single semicolon and joined with
// a space.
func MergeStyles(args ...any) string {
	var b strings.Builder
	for _, a := range args {
		s := strings.TrimSpace(Stringify(a))
		if s == "" {
			continue
		}
		s = strings.TrimRight(s, ";")
		if s == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s)
		b.WriteByte(';')
	}
	return b.String()
}

// SpreadAttributes mirrors svelte/internal/server.attributes for the common
// spread case: it walks props in deterministic order, drops invalid names,
// drops on* handlers and $$-prefixed keys, normalizes class via Clsx, and
// emits each via Attr. Returns a leading-space-prefixed attribute string.
func SpreadAttributes(props map[string]any) string {
	if len(props) == 0 {
		return ""
	}
	keys := sortedKeys(props)
	var b strings.Builder
	for _, name := range keys {
		if len(name) >= 2 && name[0] == '$' && name[1] == '$' {
			continue
		}
		if !isValidAttrName(name) {
			continue
		}
		lower := strings.ToLower(name)
		if len(lower) > 2 && strings.HasPrefix(lower, "on") {
			continue
		}
		v := props[name]
		if name == "class" {
			v = clsxValue(v)
		}
		b.WriteString(Attr(lower, v, isBooleanAttribute(lower)))
	}
	return b.String()
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func isJSFalsy(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case bool:
		return !x
	case string:
		return x == ""
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	}
	return false
}

func isJSTruthy(v any) bool {
	return !isJSFalsy(v)
}

// isValidAttrName mirrors Svelte's INVALID_ATTR_NAME_CHAR_REGEX: rejects
// whitespace, quotes, /, =, > and the unicode noncharacter ranges. Empty
// names are rejected as well.
func isValidAttrName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch r {
		case ' ', '\t', '\n', '\r', '\f', '\'', '"', '>', '/', '=':
			return false
		}
		if r >= 0xfdd0 && r <= 0xfdef {
			return false
		}
		if r >= 0xfffe {
			low := r & 0xffff
			if low == 0xfffe || low == 0xffff {
				return false
			}
		}
	}
	return true
}
