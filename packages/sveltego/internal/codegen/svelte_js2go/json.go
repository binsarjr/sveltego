package svelte_js2go

import "encoding/json"

func jsonUnmarshalReal(b []byte, v any) error {
	return json.Unmarshal(b, v)
}
