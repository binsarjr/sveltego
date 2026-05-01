// Package aitemplates embeds the AI-assistant rule templates that ship
// to user projects via `sveltego-init --ai`. Templates live alongside
// this package so packages/init has no cross-module dependency that
// would block `go run github.com/binsarjr/sveltego/packages/init/...@latest`.
//
// The canonical, human-edited copies still live at templates/ai/ at
// the repo root. A repo-level check keeps the two trees byte-equal.
package aitemplates

import "embed"

//go:embed files/AGENTS.md files/CLAUDE.md files/.cursorrules files/.github/copilot-instructions.md
var raw embed.FS

// FS exposes the four template files under their ship-paths
// (AGENTS.md, CLAUDE.md, .cursorrules, .github/copilot-instructions.md)
// by stripping the internal "files/" prefix.
var FS = stripPrefixFS{inner: raw}

// Files lists every template path inside FS in the order callers
// should write them to a fresh project.
var Files = []string{
	"AGENTS.md",
	"CLAUDE.md",
	".cursorrules",
	".github/copilot-instructions.md",
}
