package routescan

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/runtime/router"
)

// specialFiles names the seven file conventions a route directory may
// own. Presence of any one materializes a ScannedRoute. The .svelte
// names retain the SvelteKit-style "+" prefix; the .go names drop it
// because Go's tooling rejects the "+" character in source filenames
// (see ADR 0003 amendment, Phase 0i-fix).
var specialFiles = map[string]struct{}{
	"+page.svelte":     {},
	"+layout.svelte":   {},
	"+error.svelte":    {},
	"+page@.svelte":    {},
	"page.server.go":   {},
	"layout.server.go": {},
	"server.go":        {},
}

// serverGoFiles is the subset of specialFiles that are user-authored Go
// source. Each one MUST start with `//go:build sveltego` so the default
// Go toolchain (`go build`, `go vet`, `golangci-lint`) skips the file
// and silently skips its containing directory when the directory name
// itself is invalid as a Go import path (e.g. `[slug]/`).
var serverGoFiles = map[string]struct{}{
	"page.server.go":   {},
	"layout.server.go": {},
	"server.go":        {},
}

// Scan walks RoutesDir and returns one ScannedRoute per directory that
// owns at least one of the seven special files. ParamsDir, when set, is
// scanned for matcher files; matcher references in route segments are
// validated against the discovered set plus the built-in names. The
// returned error is reserved for IO failures; recoverable problems land
// in ScanResult.Diagnostics.
func Scan(input ScanInput) (*ScanResult, error) {
	if input.RoutesDir == "" {
		return nil, errors.New("routescan: RoutesDir is required")
	}

	matchers, matcherDiags := discoverMatchers(input.ParamsDir)

	routes, walkDiags, err := walkRoutes(input.RoutesDir)
	if err != nil {
		return nil, err
	}

	diagnostics := append([]Diagnostic{}, matcherDiags...)
	diagnostics = append(diagnostics, walkDiags...)

	matcherDiags2 := validateMatcherRefs(routes, matchers)
	diagnostics = append(diagnostics, matcherDiags2...)

	conflictDiags := detectPatternConflicts(routes)
	diagnostics = append(diagnostics, conflictDiags...)

	sortRoutes(routes)
	sort.Slice(matchers, func(i, j int) bool { return matchers[i].Name < matchers[j].Name })
	sortDiagnostics(diagnostics)

	return &ScanResult{
		Routes:      routes,
		Matchers:    matchers,
		Diagnostics: diagnostics,
	}, nil
}

func walkRoutes(routesDir string) ([]ScannedRoute, []Diagnostic, error) {
	info, err := os.Stat(routesDir)
	if err != nil {
		return nil, nil, fmt.Errorf("routescan: stat %s: %w", routesDir, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("routescan: %s is not a directory", routesDir)
	}

	type dirInfo struct {
		path  string
		files map[string]struct{}
	}

	var (
		dirs        []dirInfo
		diagnostics []Diagnostic
	)

	walkErr := filepath.WalkDir(routesDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != routesDir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			files, readErr := readSpecialFiles(path)
			if readErr != nil {
				return readErr
			}
			if len(files) > 0 {
				dirs = append(dirs, dirInfo{path: path, files: files})
			}
			return nil
		}
		// Stray hooks.server.go inside RoutesDir is a misplaced file.
		if d.Name() == "hooks.server.go" {
			diagnostics = append(diagnostics, Diagnostic{
				Path:    path,
				Message: "hooks.server.go must live outside src/routes/",
				Hint:    "move it to src/hooks.server.go",
			})
		}
		return nil
	})
	if walkErr != nil {
		return nil, nil, fmt.Errorf("routescan: walk %s: %w", routesDir, walkErr)
	}

	dirsWithLayout := make(map[string]struct{})
	for _, di := range dirs {
		if _, ok := di.files["+layout.svelte"]; ok {
			dirsWithLayout[di.path] = struct{}{}
		}
	}

	routes := make([]ScannedRoute, 0, len(dirs))
	for _, di := range dirs {
		route, parseDiags := buildRoute(routesDir, di.path, di.files, dirsWithLayout)
		diagnostics = append(diagnostics, parseDiags...)
		if route != nil {
			diagnostics = append(diagnostics, validateRouteFiles(*route)...)
			routes = append(routes, *route)
		}
	}

	return routes, diagnostics, nil
}

func readSpecialFiles(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	files := make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, ok := specialFiles[e.Name()]; ok {
			files[e.Name()] = struct{}{}
		}
	}
	return files, nil
}

func buildRoute(routesDir, dir string, files map[string]struct{}, dirsWithLayout map[string]struct{}) (*ScannedRoute, []Diagnostic) {
	rel, err := filepath.Rel(routesDir, dir)
	if err != nil {
		return nil, []Diagnostic{{
			Path:    dir,
			Message: fmt.Sprintf("cannot relativize path: %v", err),
		}}
	}
	rel = filepath.ToSlash(rel)

	var (
		segments     []router.Segment
		packageParts []string
		diagnostics  []Diagnostic
	)
	if rel != "." {
		for _, raw := range strings.Split(rel, "/") {
			seg, perr := ParseSegment(raw)
			if perr != nil {
				if errors.Is(perr, ErrGroup) {
					packageParts = append(packageParts, encodePackageName(raw, router.Segment{}, true))
					continue
				}
				diagnostics = append(diagnostics, Diagnostic{
					Path:    dir,
					Message: perr.Error(),
				})
				continue
			}
			segments = append(segments, seg)
			packageParts = append(packageParts, encodePackageName(raw, seg, false))
		}
	}

	pattern := BuildPattern(segments)
	packageName := "routes"
	if len(packageParts) > 0 {
		packageName = packageParts[len(packageParts)-1]
	}
	packagePath := ".gen/routes"
	if len(packageParts) > 0 {
		packagePath = ".gen/routes/" + strings.Join(packageParts, "/")
	}

	chain := buildLayoutChain(routesDir, dir, dirsWithLayout)
	chainPkgs := make([]string, 0, len(chain))
	for _, c := range chain {
		pkg, encErr := encodeLayoutPackagePath(routesDir, c)
		if encErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: c, Message: encErr.Error()})
			continue
		}
		chainPkgs = append(chainPkgs, pkg)
	}

	route := &ScannedRoute{
		Pattern:            pattern,
		Segments:           segments,
		Dir:                dir,
		PackageName:        packageName,
		PackagePath:        packagePath,
		LayoutChain:        chain,
		LayoutPackagePaths: chainPkgs,
		HasPage:            has(files, "+page.svelte"),
		HasLayout:          has(files, "+layout.svelte"),
		HasError:           has(files, "+error.svelte"),
		HasReset:           has(files, "+page@.svelte"),
		HasPageServer:      has(files, "page.server.go"),
		HasLayoutServer:    has(files, "layout.server.go"),
		HasServer:          has(files, "server.go"),
	}
	return route, diagnostics
}

// encodeLayoutPackagePath returns the .gen package path for a layout
// directory by re-applying ADR 0003 segment encoding. Mirrors the path
// computation used for route packages so per-layout adapter imports line
// up with what the layout emitter writes.
func encodeLayoutPackagePath(routesDir, dir string) (string, error) {
	rel, err := filepath.Rel(routesDir, dir)
	if err != nil {
		return "", fmt.Errorf("relativize layout dir: %w", err)
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return ".gen/routes", nil
	}
	var parts []string
	for _, raw := range strings.Split(rel, "/") {
		seg, perr := ParseSegment(raw)
		if perr != nil {
			if errors.Is(perr, ErrGroup) {
				parts = append(parts, encodePackageName(raw, router.Segment{}, true))
				continue
			}
			return "", perr
		}
		parts = append(parts, encodePackageName(raw, seg, false))
	}
	return ".gen/routes/" + strings.Join(parts, "/"), nil
}

func has(files map[string]struct{}, name string) bool {
	_, ok := files[name]
	return ok
}

func buildLayoutChain(routesDir, dir string, dirsWithLayout map[string]struct{}) []string {
	var chain []string
	cur := dir
	for {
		if _, ok := dirsWithLayout[cur]; ok {
			chain = append([]string{cur}, chain...)
		}
		if cur == routesDir {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return chain
}

func validateRouteFiles(r ScannedRoute) []Diagnostic {
	var ds []Diagnostic
	if r.HasPage && r.HasServer {
		ds = append(ds, Diagnostic{
			Path:    r.Dir,
			Message: fmt.Sprintf("route %q may not have both +page.svelte and server.go", r.Pattern),
			Hint:    "use either a page or an API endpoint",
		})
	}
	if r.HasPageServer && !r.HasPage && !r.HasServer {
		ds = append(ds, Diagnostic{
			Path:    r.Dir,
			Message: fmt.Sprintf("orphan page.server.go: route %q has no +page.svelte or server.go", r.Pattern),
		})
	}
	ds = append(ds, validateBuildTags(r)...)
	return ds
}

// validateBuildTags returns one Diagnostic per recognized server .go file
// in r.Dir whose first non-blank line is not `//go:build sveltego`. The
// build constraint is mandatory: without it the default Go toolchain
// either rejects the file (legacy `+`-prefix names, no longer used) or
// walks into a directory whose name is not a valid Go import path
// (`[slug]/`, `(group)/`, etc.) and fails. Severity is warning, not
// error, so users mid-migration still get a buildable .gen tree.
func validateBuildTags(r ScannedRoute) []Diagnostic {
	var ds []Diagnostic
	for name := range serverGoFiles {
		switch name {
		case "page.server.go":
			if !r.HasPageServer {
				continue
			}
		case "layout.server.go":
			if !r.HasLayoutServer {
				continue
			}
		case "server.go":
			if !r.HasServer {
				continue
			}
		}
		path := filepath.Join(r.Dir, name)
		if hasSveltegoBuildTag(path) {
			continue
		}
		ds = append(ds, Diagnostic{
			Path:    path,
			Message: "missing //go:build sveltego constraint; this file may be parsed by go build and break the build",
			Hint:    "add `//go:build sveltego` as the first line so the default Go toolchain skips it",
		})
	}
	return ds
}

// hasSveltegoBuildTag returns true when the first non-blank line of path
// is a `//go:build` directive whose expression mentions the `sveltego`
// constraint. Boolean operators (`&&`, `||`, `!`, parentheses) and other
// constraints may coexist; the directive simply has to include the
// `sveltego` token as one of its identifiers.
func hasSveltegoBuildTag(path string) bool {
	f, err := os.Open(path) //nolint:gosec // path is scanner-controlled
	if err != nil {
		return false
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		trimmed := strings.TrimSpace(scan.Text())
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//go:build") {
			return false
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "//go:build"))
		cleaned := strings.NewReplacer("&&", " ", "||", " ", "(", " ", ")", " ", "!", " ").Replace(rest)
		for _, tok := range strings.Fields(cleaned) {
			if tok == "sveltego" {
				return true
			}
		}
		return false
	}
	return false
}

func detectPatternConflicts(routes []ScannedRoute) []Diagnostic {
	if len(routes) == 0 {
		return nil
	}
	// Group routes by URL shape (param names erased). Two siblings
	// `[a]` and `[b]` produce different Pattern strings but the same
	// set of matching URLs, so they conflict.
	type entry struct {
		pattern string
		dir     string
	}
	byShape := make(map[string][]entry, len(routes))
	for _, r := range routes {
		if !(r.HasPage || r.HasServer || r.HasReset) {
			continue
		}
		shape := patternShape(r.Segments)
		byShape[shape] = append(byShape[shape], entry{pattern: r.Pattern, dir: r.Dir})
	}

	var ds []Diagnostic
	for _, entries := range byShape {
		if len(entries) <= 1 {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].dir < entries[j].dir })
		for i := 1; i < len(entries); i++ {
			ds = append(ds, Diagnostic{
				Path: entries[i].dir,
				Message: fmt.Sprintf(
					"route conflict: %q and %q both match the same URLs",
					entries[0].pattern, entries[i].pattern,
				),
			})
		}
	}
	return ds
}

// patternShape returns the URL-shape of segments with parameter names
// erased. Static values, kinds, and matcher tags are preserved because
// they meaningfully partition the URL space.
func patternShape(segments []router.Segment) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteByte('/')
		switch s.Kind {
		case router.SegmentStatic:
			b.WriteString(s.Value)
		case router.SegmentParam:
			b.WriteString("[*]")
			if s.Matcher != "" {
				b.WriteByte('=')
				b.WriteString(s.Matcher)
			}
		case router.SegmentOptional:
			b.WriteString("[[*]]")
			if s.Matcher != "" {
				b.WriteByte('=')
				b.WriteString(s.Matcher)
			}
		case router.SegmentRest:
			b.WriteString("[...*]")
			if s.Matcher != "" {
				b.WriteByte('=')
				b.WriteString(s.Matcher)
			}
		}
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

func validateMatcherRefs(routes []ScannedRoute, matchers []DiscoveredMatcher) []Diagnostic {
	known := make(map[string]struct{}, len(matchers)+len(builtinMatchers))
	for name := range builtinMatchers {
		known[name] = struct{}{}
	}
	for _, m := range matchers {
		known[m.Name] = struct{}{}
	}

	var ds []Diagnostic
	for _, r := range routes {
		for _, seg := range r.Segments {
			if seg.Matcher == "" {
				continue
			}
			if _, ok := known[seg.Matcher]; ok {
				continue
			}
			ds = append(ds, Diagnostic{
				Path: r.Dir,
				Message: fmt.Sprintf(
					"route %q references unknown matcher %q on parameter %q",
					r.Pattern, seg.Matcher, seg.Name,
				),
				Hint: "add src/params/" + seg.Matcher + ".go with `func Match(s string) bool` or use a built-in (int, uuid, slug)",
			})
		}
	}
	return ds
}

func sortRoutes(routes []ScannedRoute) {
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Pattern != routes[j].Pattern {
			return routes[i].Pattern < routes[j].Pattern
		}
		return routes[i].Dir < routes[j].Dir
	})
}

func sortDiagnostics(ds []Diagnostic) {
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Path != ds[j].Path {
			return ds[i].Path < ds[j].Path
		}
		if ds[i].Pos.Line != ds[j].Pos.Line {
			return ds[i].Pos.Line < ds[j].Pos.Line
		}
		if ds[i].Pos.Col != ds[j].Pos.Col {
			return ds[i].Pos.Col < ds[j].Pos.Col
		}
		return ds[i].Message < ds[j].Message
	})
}
