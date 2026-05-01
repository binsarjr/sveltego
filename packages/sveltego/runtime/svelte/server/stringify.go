package server

import "strconv"

// Stringify mirrors svelte/internal/server.stringify(value):
// strings pass through, nil renders as "", everything else uses JS-style
// String(value) coercion. Matches Svelte's compiled output for {expr}.
func Stringify(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return formatFloatJS(float64(x))
	case float64:
		return formatFloatJS(x)
	case []byte:
		return string(x)
	case Stringer:
		return x.String()
	}
	return fallbackStringify(v)
}

// Stringer mirrors fmt.Stringer without dragging in fmt for hot paths.
type Stringer interface {
	String() string
}

// formatFloatJS approximates JavaScript's Number.toString for floats:
// integral floats render without a decimal point.
func formatFloatJS(f float64) string {
	if f == float64(int64(f)) && f > -1e21 && f < 1e21 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func fallbackStringify(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if s, ok := v.(Stringer); ok {
		return s.String()
	}
	return "[object Object]"
}
