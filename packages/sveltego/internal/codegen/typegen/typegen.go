// Package typegen reads a route's `_page.server.go` (or
// `_layout.server.go`) Go AST, extracts the Load function's return
// shape, and emits a sibling `_page.svelte.d.ts` (or
// `_layout.svelte.d.ts`) declaring the `PageData` (or `LayoutData`)
// interface and the matching `data` value.
//
// Phase 2 of RFC #379 (ADR 0008): the .d.ts files give pure-Svelte
// templates IDE autocompletion via Svelte LSP without parsing Go
// expressions inside templates. This package runs in parallel to the
// legacy Mustache-Go codegen pipeline; Phase 5 (#384) drops the legacy
// path.
package typegen

import (
	"fmt"
	"os"
	"path/filepath"
)

// EmitOptions configures one [EmitForRoute] call. RouteDir is the
// absolute filesystem path to the route directory (the one that owns
// `_page.svelte` / `_page.server.go`). Kind selects between the page
// and layout shapes.
type EmitOptions struct {
	RouteDir string
	Kind     Kind
}

// Kind selects between page-shape and layout-shape emission. The
// distinction drives the source filename, the output filename, and the
// generated identifier names.
type Kind int

const (
	// KindPage emits `_page.svelte.d.ts` from `_page.server.go`,
	// declaring `PageData` and `data: PageData`.
	KindPage Kind = iota
	// KindLayout emits `_layout.svelte.d.ts` from `_layout.server.go`,
	// declaring `LayoutData` and `data: LayoutData`.
	KindLayout
)

// Diagnostic is a non-fatal warning surfaced to the build driver. Path
// is the source `.go` file the warning concerns; Message is a human
// sentence ready to print as-is. Diagnostics never replace errors —
// they accompany a successful emit.
type Diagnostic struct {
	Path    string
	Message string
}

func (d Diagnostic) String() string {
	return d.Path + ": " + d.Message
}

// EmitForRoute generates a `.d.ts` next to the route's `.svelte` file
// based on the route's server-side Go AST. Returns the absolute path
// of the written file (empty when no source `.server.go` exists),
// any non-fatal diagnostics, and an error.
//
// When the source file is absent the call is a no-op: a route that
// does not load server data still has a typed default of `{}`, but
// emitting an empty `.d.ts` would mislead the LSP into thinking
// `data` is an empty object rather than `unknown`. The build driver
// decides whether to treat the absence as an error.
func EmitForRoute(opts EmitOptions) (string, []Diagnostic, error) {
	srcName, outName, dataIdent, typeIdent := kindFilenames(opts.Kind)
	srcPath := filepath.Join(opts.RouteDir, srcName)
	if _, err := os.Stat(srcPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("typegen: stat %s: %w", srcPath, err)
	}

	shape, diags, err := walkServerFile(srcPath, typeIdent)
	if err != nil {
		return "", diags, err
	}
	out := emitDeclaration(shape, dataIdent, typeIdent)

	target := filepath.Join(opts.RouteDir, outName)
	if err := os.WriteFile(target, []byte(out), 0o600); err != nil {
		return "", diags, fmt.Errorf("typegen: write %s: %w", target, err)
	}
	return target, diags, nil
}

func kindFilenames(k Kind) (src, out, dataIdent, typeIdent string) {
	switch k {
	case KindLayout:
		return "_layout.server.go", "_layout.svelte.d.ts", "data", "LayoutData"
	default:
		return "_page.server.go", "_page.svelte.d.ts", "data", "PageData"
	}
}
