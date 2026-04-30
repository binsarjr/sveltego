package ai

import (
	"strings"
	"testing"
)

// anchorStrings names canonical substrings every template must carry.
// Drift on any of these is a content regression that the snapshot test
// catches before the templates ship out via `sveltego init --ai`.
var anchorStrings = []string{
	"sveltego",
	"//go:build sveltego",
	"PageData",
	"kit.Redirect",
	"kit.ActionMap",
	"page.server.go",
	"+page.svelte",
	"$props",
	"https://sveltego.dev",
	"github.com/binsarjr/sveltego",
}

// crossLinkStrings names the per-tool entry points each template must
// reference. Each template should mention every other; the test enforces
// the cross-link contract from #73.
var crossLinkStrings = map[string][]string{
	"AGENTS.md":                       {"CLAUDE.md", ".cursorrules", "copilot-instructions.md"},
	"CLAUDE.md":                       {"AGENTS.md", ".cursorrules", "copilot-instructions.md"},
	".cursorrules":                    {"AGENTS.md", "CLAUDE.md"},
	".github/copilot-instructions.md": {"AGENTS.md", "CLAUDE.md"},
}

func TestFiles_ListMatchesEmbed(t *testing.T) {
	for _, name := range Files {
		if _, err := FS.ReadFile(name); err != nil {
			t.Errorf("FS missing template %q: %v", name, err)
		}
	}
}

func TestTemplates_NonEmpty(t *testing.T) {
	for _, name := range Files {
		body, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		if len(body) == 0 {
			t.Errorf("template %q is empty", name)
		}
	}
}

func TestTemplates_CarryAnchorStrings(t *testing.T) {
	for _, name := range Files {
		body, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		text := string(body)
		for _, anchor := range anchorStrings {
			if !strings.Contains(text, anchor) {
				t.Errorf("template %q missing anchor %q", name, anchor)
			}
		}
	}
}

func TestTemplates_CrossLink(t *testing.T) {
	for name, links := range crossLinkStrings {
		body, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		text := string(body)
		for _, link := range links {
			if !strings.Contains(text, link) {
				t.Errorf("template %q missing cross-link to %q", name, link)
			}
		}
	}
}

func TestTemplates_NameAntiPatterns(t *testing.T) {
	// The templates must explicitly name the patterns agents should avoid
	// so a tool grepping for the rejected form (e.g. `+page.server.go`,
	// `export let`) finds the rejection context, not a permissive
	// example. The list mirrors the "Don't" / "Anti-patterns" section.
	rejectedTokens := []string{
		"+page.server.go",
		"export let",
	}
	for _, name := range Files {
		body, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		text := string(body)
		for _, tok := range rejectedTokens {
			if !strings.Contains(text, tok) {
				t.Errorf("template %q does not name rejected token %q (the rejection list lost coverage)", name, tok)
			}
		}
	}
}
