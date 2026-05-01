package server

import (
	"reflect"
	"sort"
	"testing"
)

func TestSpreadProps(t *testing.T) {
	a := map[string]any{"x": 1, "y": 2}
	b := map[string]any{"y": 99, "z": 3}
	got := SpreadProps(a, b)
	want := map[string]any{"x": 1, "y": 99, "z": 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SpreadProps = %v, want %v", got, want)
	}
}

func TestRestProps(t *testing.T) {
	in := map[string]any{"a": 1, "b": 2, "c": 3}
	got := RestProps(in, "b")
	want := map[string]any{"a": 1, "c": 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RestProps = %v, want %v", got, want)
	}
}

func TestSanitizeProps(t *testing.T) {
	in := map[string]any{"a": 1, "children": "skip", "$$slots": "skip"}
	got := SanitizeProps(in)
	want := map[string]any{"a": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SanitizeProps = %v, want %v", got, want)
	}
}

func TestSanitizeSlots(t *testing.T) {
	in := map[string]any{
		"children": func() {},
		"$$slots":  map[string]any{"header": true, "footer": true},
	}
	got := SanitizeSlots(in)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"default", "footer", "header"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("SanitizeSlots keys = %v, want %v", keys, want)
	}
}

func TestFallback(t *testing.T) {
	if got := Fallback(nil, "x"); got != "x" {
		t.Errorf("Fallback(nil,x) = %v", got)
	}
	if got := Fallback("y", "x"); got != "y" {
		t.Errorf("Fallback(y,x) = %v", got)
	}
}

func TestExcludeFromObject(t *testing.T) {
	got := ExcludeFromObject(map[string]any{"a": 1, "b": 2}, "b")
	want := map[string]any{"a": 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExcludeFromObject = %v, want %v", got, want)
	}
}

func TestEnsureArrayLike(t *testing.T) {
	if got := EnsureArrayLike(nil); got != nil {
		t.Errorf("nil input = %v", got)
	}
	if got := EnsureArrayLike([]string{"a", "b"}); !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Errorf("[]string = %v", got)
	}
	if got := EnsureArrayLike([]any{1, 2}); !reflect.DeepEqual(got, []any{1, 2}) {
		t.Errorf("[]any = %v", got)
	}
}
