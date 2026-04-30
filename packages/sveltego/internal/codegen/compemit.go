package codegen

import (
	"fmt"
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	sveltepkg "github.com/binsarjr/sveltego/internal/parser"
)

// svelteImport records one relative import from a .svelte file's <script
// lang="go"> block that resolves to another .svelte component file.
type svelteImport struct {
	// Alias is the Go package alias from the import declaration (e.g. "button").
	Alias string
	// AbsPath is the absolute path to the child .svelte file on disk.
	AbsPath string
}

// componentRef is a queued work item carrying a .svelte abs path plus the
// component name derived from the filename.
type componentRef struct {
	AbsPath       string
	ComponentName string
}

// compNode is one vertex in the component dependency graph.
type compNode struct {
	ref      componentRef
	children []string // abs paths of direct child .svelte components
}

// componentGenResult is the output of emitting one component .svelte file.
type componentGenResult struct {
	AbsPath string
	GenPath string // abs path of the written component.gen.go
	PkgPath string // outDir-relative import suffix (e.g. ".gen/components/routes/button")
}

// emitComponentTree walks every .svelte file referenced (transitively) from
// seeds (typically each +page.svelte and +layout.svelte encountered during
// Build), discovers component imports via <script lang="go"> relative import
// declarations, detects cycles and casing collisions, and emits one
// component.gen.go per unique component.
//
// Generated packages land under <outAbs>/components/<encodedSubpath>/.
func emitComponentTree(
	projectRoot, outDir string,
	seeds []string,
) ([]componentGenResult, error) {
	outAbs := filepath.Join(projectRoot, outDir)

	// visited tracks abs paths already enqueued to prevent double-processing.
	// lowerSeen maps lowercase abs path → first abs path for casing collision detection.
	visited := map[string]bool{}
	lowerSeen := map[string]string{}

	// First pass: BFS to build the full adjacency map.
	var queue []componentRef
	nodes := map[string]*compNode{}

	// Seed the queue from each seed file's direct component imports.
	for _, seed := range seeds {
		imports, err := svelteComponentImports(seed)
		if err != nil {
			return nil, err
		}
		for _, imp := range imports {
			if err := enqueueComponent(imp.AbsPath, &queue, visited, lowerSeen); err != nil {
				return nil, err
			}
		}
	}

	// Build adjacency map by expanding the queue.
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if _, done := nodes[cur.AbsPath]; done {
			continue
		}
		imports, err := svelteComponentImports(cur.AbsPath)
		if err != nil {
			return nil, err
		}
		n := &compNode{ref: cur}
		for _, imp := range imports {
			if err := enqueueComponent(imp.AbsPath, &queue, visited, lowerSeen); err != nil {
				return nil, err
			}
			n.children = append(n.children, imp.AbsPath)
		}
		nodes[cur.AbsPath] = n
	}

	if len(nodes) == 0 {
		return nil, nil
	}

	// DFS cycle detection over the adjacency map.
	if err := detectCycles(nodes); err != nil {
		return nil, err
	}

	// Emit in topological order (dependencies before dependents) for determinism.
	emitOrder := topoSortNodes(nodes)
	var results []componentGenResult

	for _, absPath := range emitOrder {
		n := nodes[absPath]
		encoded := encodeComponentPath(projectRoot, absPath)
		pkgPath := outDir + "/components/" + encoded
		pkgName := encodedLeaf(encoded)
		genDir := filepath.Join(outAbs, "components", filepath.FromSlash(encoded))
		genFile := filepath.Join(genDir, "component.gen.go")

		src, err := os.ReadFile(absPath) //nolint:gosec // paths come from user project tree
		if err != nil {
			return nil, fmt.Errorf("codegen: read component %s: %w", absPath, err)
		}
		frag, perrs := sveltepkg.Parse(src)
		if len(perrs) > 0 {
			return nil, fmt.Errorf("codegen: parse component %s: %w", absPath, perrs)
		}

		cr, err := GenerateComponent(frag, ComponentOptions{
			PackageName:   pkgName,
			ComponentName: n.ref.ComponentName,
		})
		if err != nil {
			return nil, fmt.Errorf("codegen: generate component %s: %w", absPath, err)
		}

		if err := os.MkdirAll(genDir, 0o755); err != nil {
			return nil, fmt.Errorf("codegen: mkdir %s: %w", genDir, err)
		}
		if err := os.WriteFile(genFile, cr.Source, genFileMode); err != nil {
			return nil, fmt.Errorf("codegen: write component %s: %w", genFile, err)
		}

		results = append(results, componentGenResult{
			AbsPath: absPath,
			GenPath: genFile,
			PkgPath: pkgPath,
		})
	}

	return results, nil
}

// detectCycles runs a DFS over the adjacency map and returns an error on the
// first back-edge found. The error message contains the full cycle path using
// file basenames for readability.
func detectCycles(nodes map[string]*compNode) error {
	inStack := map[string]bool{}
	dfsVisited := map[string]bool{}
	stackPath := []string{}

	var dfs func(path string) error
	dfs = func(path string) error {
		if inStack[path] {
			cycleStart := -1
			for i, p := range stackPath {
				if p == path {
					cycleStart = i
					break
				}
			}
			cycleSegments := stackPath[cycleStart:]
			names := make([]string, len(cycleSegments)+1)
			for i, p := range cycleSegments {
				names[i] = filepath.Base(p)
			}
			names[len(cycleSegments)] = filepath.Base(path)
			return fmt.Errorf("codegen: component cycle detected: %s", strings.Join(names, " → "))
		}
		if dfsVisited[path] {
			return nil
		}
		dfsVisited[path] = true
		inStack[path] = true
		stackPath = append(stackPath, path)
		if n, ok := nodes[path]; ok {
			for _, child := range n.children {
				if err := dfs(child); err != nil {
					return err
				}
			}
		}
		stackPath = stackPath[:len(stackPath)-1]
		delete(inStack, path)
		return nil
	}

	// Iterate in sorted order for deterministic error messages.
	keys := sortedKeys(nodes)
	for _, path := range keys {
		if err := dfs(path); err != nil {
			return err
		}
	}
	return nil
}

// topoSortNodes returns the abs paths in topological order (dependencies
// emitted before their dependents). Assumes no cycles (detectCycles passed).
func topoSortNodes(nodes map[string]*compNode) []string {
	visited := map[string]bool{}
	var order []string

	var visit func(path string)
	visit = func(path string) {
		if visited[path] {
			return
		}
		visited[path] = true
		if n, ok := nodes[path]; ok {
			// Sort children for determinism.
			children := make([]string, len(n.children))
			copy(children, n.children)
			stableSort(children)
			for _, child := range children {
				visit(child)
			}
		}
		order = append(order, path)
	}

	keys := sortedKeys(nodes)
	for _, k := range keys {
		visit(k)
	}
	return order
}

// enqueueComponent adds absPath to the work queue if not already visited,
// after checking for casing collisions.
func enqueueComponent(
	absPath string,
	queue *[]componentRef,
	visited map[string]bool,
	lowerSeen map[string]string,
) error {
	if visited[absPath] {
		return nil
	}
	lower := strings.ToLower(absPath)
	if prev, ok := lowerSeen[lower]; ok && prev != absPath {
		return fmt.Errorf("codegen: casing collision: %q and %q map to the same encoded path", prev, absPath)
	}
	lowerSeen[lower] = absPath
	visited[absPath] = true
	compName := componentNameFromPath(absPath)
	*queue = append(*queue, componentRef{AbsPath: absPath, ComponentName: compName})
	return nil
}

// svelteComponentImports reads a .svelte file from disk, parses it, and
// returns all imports that resolve to sibling .svelte component files.
func svelteComponentImports(svelteAbsPath string) ([]svelteImport, error) {
	src, err := os.ReadFile(svelteAbsPath) //nolint:gosec // paths come from user project tree
	if err != nil {
		return nil, fmt.Errorf("codegen: read %s: %w", svelteAbsPath, err)
	}
	return svelteImportsFromSource(svelteAbsPath, src)
}

// svelteImportsFromSource extracts component imports from already-read source
// bytes. Used in tests and for seed files whose bytes were already read by
// the main emit pass.
func svelteImportsFromSource(svelteAbsPath string, src []byte) ([]svelteImport, error) {
	frag, perrs := sveltepkg.Parse(src)
	if len(perrs) > 0 {
		// Non-fatal here: parse errors surface again in the generate pass.
		return nil, nil
	}

	scripts := collectScripts(frag.Children)
	if len(scripts) == 0 {
		return nil, nil
	}

	dir := filepath.Dir(svelteAbsPath)
	var out []svelteImport

	for _, s := range scripts {
		if s.Lang != "go" {
			continue
		}
		rawImps, err := extractRawImports(s.Body)
		if err != nil {
			continue // invalid Go, surfaced later by generate pass
		}
		for _, imp := range rawImps {
			if !isRelativePath(imp.path) {
				continue
			}
			absTarget, err := resolveComponentPath(dir, imp.path)
			if err != nil || absTarget == "" {
				continue
			}
			out = append(out, svelteImport{
				Alias:   imp.alias,
				AbsPath: absTarget,
			})
		}
	}
	return out, nil
}

// rawImport is a minimally parsed import: alias (empty when absent) and the
// path without surrounding quotes.
type rawImport struct {
	alias string
	path  string
}

// extractRawImports parses Go import declarations from a script body and
// returns each import's alias and unquoted path.
func extractRawImports(body string) ([]rawImport, error) {
	fileSrc := "package _x\n" + body + "\n"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", fileSrc, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []rawImport
	for _, decl := range f.Decls {
		gen, ok := decl.(*goast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		for _, spec := range gen.Specs {
			is, ok := spec.(*goast.ImportSpec)
			if !ok {
				continue
			}
			path := strings.Trim(is.Path.Value, `"`)
			alias := ""
			if is.Name != nil {
				alias = is.Name.Name
			}
			out = append(out, rawImport{alias: alias, path: path})
		}
	}
	return out, nil
}

// isRelativePath reports whether path starts with "./" or "../".
func isRelativePath(path string) bool {
	return strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../")
}

// resolveComponentPath resolves a relative import path from baseDir to the
// absolute path of the target .svelte file on disk. Returns "" when no file
// is found, without error (missing components are not fatal at discovery time;
// the generate pass surfaces missing-file errors).
//
// Resolution order:
//  1. path + ".svelte" — most common: `import button "./button"`.
//  2. path (if it already ends in ".svelte") — explicit extension.
//  3. path + "/index.svelte" — directory components.
func resolveComponentPath(baseDir, relPath string) (string, error) {
	relPath = strings.TrimRight(relPath, "/")
	joined := filepath.Join(baseDir, filepath.FromSlash(relPath))

	// Explicit .svelte extension.
	if strings.HasSuffix(relPath, ".svelte") {
		if _, err := os.Stat(joined); err == nil {
			return filepath.Clean(joined), nil
		}
		return "", nil
	}

	// Append .svelte.
	candidate := joined + ".svelte"
	if _, err := os.Stat(candidate); err == nil {
		return filepath.Clean(candidate), nil
	}

	// Directory component: path/index.svelte.
	candidate = filepath.Join(joined, "index.svelte")
	if _, err := os.Stat(candidate); err == nil {
		return filepath.Clean(candidate), nil
	}

	return "", nil
}

// encodeComponentPath converts the absolute path of a .svelte component to a
// slash-separated, lowercase, filesystem-safe relative path string used as
// the sub-directory under <gen>/components/.
//
// The encoding:
//  1. Strips projectRoot prefix.
//  2. Strips leading "src/" if present.
//  3. Removes ".svelte" suffix.
//  4. Lowercases every segment.
//  5. Replaces bracket/paren characters (route param syntax) with underscores.
//
// Example:
//
//	projectRoot=/app  absPath=/app/src/routes/foo/Button.svelte → "routes/foo/button"
func encodeComponentPath(projectRoot, absPath string) string {
	rel, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		base := filepath.Base(absPath)
		return strings.ToLower(strings.TrimSuffix(base, ".svelte"))
	}
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "src/")
	rel = strings.TrimSuffix(rel, ".svelte")
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		parts[i] = encodeSegment(p)
	}
	return strings.Join(parts, "/")
}

// encodeSegment lowercases seg and replaces bracket/paren characters with
// underscores so the result is a valid Go package-name component.
func encodeSegment(seg string) string {
	var b strings.Builder
	b.Grow(len(seg))
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		switch c {
		case '[', ']', '(', ')':
			b.WriteByte('_')
		default:
			if c >= 'A' && c <= 'Z' {
				b.WriteByte(c + ('a' - 'A'))
			} else {
				b.WriteByte(c)
			}
		}
	}
	return b.String()
}

// encodedLeaf returns the last slash-separated segment of an encoded path,
// used as the Go package name for the component.
func encodedLeaf(encoded string) string {
	idx := strings.LastIndexByte(encoded, '/')
	if idx < 0 {
		return encoded
	}
	return encoded[idx+1:]
}

// componentNameFromPath derives the PascalCase component type name from the
// .svelte file's base name. "button.svelte" → "Button", "my-card.svelte" →
// "MyCard".
func componentNameFromPath(absPath string) string {
	base := filepath.Base(absPath)
	name := strings.TrimSuffix(base, ".svelte")
	return pascalIdent(name)
}

// sortedKeys returns the keys of a map[string]*compNode in sorted order.
func sortedKeys(m map[string]*compNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	stableSort(keys)
	return keys
}

// stableSort sorts a string slice in place using insertion sort (correct for
// small slices; avoids importing sort for a trivial use).
func stableSort(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
