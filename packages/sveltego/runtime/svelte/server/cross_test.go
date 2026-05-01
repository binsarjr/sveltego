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
		// Template-level fixtures use a different schema; they are
		// driven by TestCrossCheckTemplates below.
		if filepath.Base(path) == "templates.json" {
			continue
		}
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

// templateCase is one full-template render fixture: a sequence of
// payload-mutating ops (push, escape, attr, clsx, merge_styles,
// spread_attributes) plus the Svelte 5 server-rendered HTML the chain
// MUST produce byte-for-byte. Drives the ≥30 representative-template
// cross-check from issue #429.
type templateCase struct {
	Name     string       `json:"name"`
	Ops      []templateOp `json:"ops"`
	Expected string       `json:"expected"`
}

type templateOp struct {
	Op   string `json:"op"`
	Args []any  `json:"args"`
}

type templateFile struct {
	Kind          string         `json:"kind"`
	SvelteVersion string         `json:"svelte_version"`
	Cases         []templateCase `json:"cases"`
}

// TestCrossCheckTemplates drives ≥30 full-template render fixtures.
// Each fixture's `expected` is a snapshot of the body Svelte's
// `svelte/server` produces for the equivalent template; the Go side
// plays back the same op sequence through Payload + helpers and must
// reach byte-equality. Phase 7 (#429) cross-check bar.
func TestCrossCheckTemplates(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "cross", "templates.json"))
	if err != nil {
		t.Fatalf("read templates fixture: %v", err)
	}
	var f templateFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("parse templates fixture: %v", err)
	}
	if len(f.Cases) < 30 {
		t.Fatalf("templates corpus has %d entries; acceptance bar is ≥30", len(f.Cases))
	}
	for _, tc := range f.Cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			var p Payload
			for _, op := range tc.Ops {
				runTemplateOp(t, &p, op)
			}
			got := p.Body()
			if got != tc.Expected {
				t.Fatalf("template body mismatch\n--- want:\n%s\n--- got:\n%s", tc.Expected, got)
			}
		})
	}
}

func runTemplateOp(t *testing.T, p *Payload, op templateOp) {
	t.Helper()
	switch op.Op {
	case "push":
		s, _ := op.Args[0].(string)
		p.Push(s)
	case "escape_html":
		p.Push(EscapeHTML(op.Args[0]))
	case "escape_html_attr":
		p.Push(EscapeHTMLAttr(op.Args[0]))
	case "stringify":
		p.Push(Stringify(op.Args[0]))
	case "attr":
		if len(op.Args) != 3 {
			t.Fatalf("attr op expects 3 args, got %d", len(op.Args))
		}
		name, _ := op.Args[0].(string)
		isBool, _ := op.Args[2].(bool)
		p.Push(Attr(name, op.Args[1], isBool))
	case "clsx":
		p.Push(Clsx(normalizeJSONArgs(op.Args)...))
	case "merge_styles":
		p.Push(MergeStyles(normalizeJSONArgs(op.Args)...))
	case "spread_attributes":
		props, ok := op.Args[0].(map[string]any)
		if !ok {
			t.Fatalf("spread_attributes arg must be object")
		}
		p.Push(SpreadAttributes(props))
	default:
		t.Fatalf("unknown template op: %s", op.Op)
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
