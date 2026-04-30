// Package ai embeds the AI-assistant rule templates that ship to user
// projects via `sveltego init --ai`. The templates teach AI agents about
// sveltego's Go-only SSR conventions, expression syntax, and file layout.
package ai

import "embed"

// FS holds the four template files exposed under their on-disk paths
// relative to this package: AGENTS.md, CLAUDE.md, .cursorrules, and
// .github/copilot-instructions.md.
//
//go:embed AGENTS.md CLAUDE.md .cursorrules .github/copilot-instructions.md
var FS embed.FS

// Files lists every template path inside [FS] in the order callers
// should write them to a fresh project.
var Files = []string{
	"AGENTS.md",
	"CLAUDE.md",
	".cursorrules",
	".github/copilot-instructions.md",
}
