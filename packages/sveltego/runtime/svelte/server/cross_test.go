package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type crossFile struct {
	Helper        string      `json:"helper"`
	SvelteVersion string      `json:"svelte_version"`
	Cases         []crossCase `json:"cases"`
}

type crossCase struct {
	Name string `json:"name"`
	In   any    `json:"in,omitempty"`
	Args []any  `json:"args,omitempty"`
	Want any    `json:"want"`
}

// TestCrossCheckFixtures asserts every Go helper produces byte-equal output
// vs the captured Svelte fixtures. Drives the ≥50-pair acceptance bar in
// issue #426.
func TestCrossCheckFixtures(t *testing.T) {
	matches, err := filepath.Glob("testdata/cross/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("no fixtures found")
	}

	total := 0
	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var f crossFile
			if err := json.Unmarshal(data, &f); err != nil {
				t.Fatal(err)
			}
			for _, c := range f.Cases {
				total++
				want, _ := c.Want.(string)
				got := runHelper(t, f.Helper, c)
				if got != want {
					t.Errorf("%s/%s: got %q, want %q", f.Helper, c.Name, got, want)
				}
			}
		})
	}
	if total < 50 {
		t.Errorf("fixture count = %d, acceptance bar is ≥50", total)
	}
}

func runHelper(t *testing.T, helper string, c crossCase) string {
	t.Helper()
	switch helper {
	case "EscapeHTML":
		return EscapeHTML(c.In)
	case "EscapeHTMLAttr":
		return EscapeHTMLAttr(c.In)
	case "Stringify":
		return Stringify(c.In)
	case "Attr":
		if len(c.Args) != 3 {
			t.Fatalf("Attr expects 3 args, got %d", len(c.Args))
		}
		name, _ := c.Args[0].(string)
		isBool, _ := c.Args[2].(bool)
		return Attr(name, c.Args[1], isBool)
	case "Clsx":
		return Clsx(normalizeJSONArgs(c.Args)...)
	case "MergeStyles":
		return MergeStyles(normalizeJSONArgs(c.Args)...)
	case "SpreadAttributes":
		if len(c.Args) != 1 {
			t.Fatalf("SpreadAttributes expects 1 arg")
		}
		props, ok := c.Args[0].(map[string]any)
		if !ok {
			t.Fatalf("SpreadAttributes arg must be object")
		}
		return SpreadAttributes(props)
	}
	t.Fatalf("unknown helper: %s", helper)
	return ""
}

// normalizeJSONArgs flattens encoding/json's number-as-float64 default for
// ints we expect to round-trip cleanly. JSON strings/bools/objects pass
// through; integral floats become ints to match Stringify expectations.
func normalizeJSONArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = normalizeJSONValue(a)
	}
	return out
}

func normalizeJSONValue(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int(x)
		}
	case []any:
		for i, e := range x {
			x[i] = normalizeJSONValue(e)
		}
	case map[string]any:
		for k, e := range x {
			x[k] = normalizeJSONValue(e)
		}
	}
	return v
}
