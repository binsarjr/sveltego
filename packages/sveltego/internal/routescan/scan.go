package routescan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// specialFiles names the file conventions a route directory may
// own. Presence of any one materializes a ScannedRoute. All names use
// the `_` prefix so Go's default toolchain (`go build`, `go vet`,
// `golangci-lint`) skips per-route Go files automatically — see RFC
// #379 phase 1b. Reset-suffixed names like `_page@.svelte` and
// `_page@(app).svelte` are matched separately by ParseResetFilename
// and normalized into the base (_page / _layout / _error) entry plus
// a per-route ResetTarget.
var specialFiles = map[string]struct{}{
	"_page.svelte":      {},
	"_layout.svelte":    {},
	"_error.svelte":     {},
	"_page.server.go":   {},
	"_layout.server.go": {},
	"_server.go":        {},
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
		path        string
		files       map[string]struct{}
		hasReset    bool
		resetTarget string
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
			// Mirror go/build conventions: skip testdata/ and __tests__/ subtrees.
			if path != routesDir && (d.Name() == "testdata" || d.Name() == "__tests__") {
				return fs.SkipDir
			}
			files, hasReset, resetTarget, readErr := readSpecialFiles(path)
			if readErr != nil {
				return readErr
			}
			if len(files) > 0 {
				dirs = append(dirs, dirInfo{
					path:        path,
					files:       files,
					hasReset:    hasReset,
					resetTarget: resetTarget,
				})
			}
			return nil
		}
		// Skip test and spec files per Go and JS conventions.
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") ||
			strings.HasSuffix(name, "_test.svelte") ||
			strings.HasSuffix(name, ".spec.svelte") {
			return nil
		}
		// Stray hooks.server.go inside RoutesDir is a misplaced file.
		if name == "hooks.server.go" {
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
	dirsWithError := make(map[string]struct{})
	for _, di := range dirs {
		if _, ok := di.files["_layout.svelte"]; ok {
			dirsWithLayout[di.path] = struct{}{}
		}
		if _, ok := di.files["_error.svelte"]; ok {
			dirsWithError[di.path] = struct{}{}
		}
	}

	routes := make([]ScannedRoute, 0, len(dirs))
	for _, di := range dirs {
		route, parseDiags := buildRoute(routesDir, di.path, di.files, dirsWithLayout, dirsWithError, di.hasReset, di.resetTarget)
		diagnostics = append(diagnostics, parseDiags...)
		if route != nil {
			diagnostics = append(diagnostics, validateRouteFiles(*route)...)
			routes = append(routes, *route)
		}
	}

	return routes, diagnostics, nil
}

// readSpecialFiles enumerates the special files in dir. The returned
// map carries one entry per matched filename. Reset-suffixed names
// (`_page@.svelte`, `_page@(group).svelte`, plus the _layout/_error
// equivalents) are normalized to the base name so downstream code can
// continue to consult `_page.svelte` etc; hasReset reflects whether
// any reset variant was found and resetTarget records the parsed
// @<target> portion (empty for root reset).
func readSpecialFiles(dir string) (files map[string]struct{}, hasReset bool, resetTarget string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, "", fmt.Errorf("read %s: %w", dir, err)
	}
	files = make(map[string]struct{})
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if _, ok := specialFiles[name]; ok {
			files[name] = struct{}{}
			continue
		}
		base, target, ok := ParseResetFilename(name)
		if !ok {
			continue
		}
		files[base+".svelte"] = struct{}{}
		hasReset = true
		resetTarget = target
	}
	return files, hasReset, resetTarget, nil
}

func buildRoute(routesDir, dir string, files map[string]struct{}, dirsWithLayout, dirsWithError map[string]struct{}, hasReset bool, resetTarget string) (*ScannedRoute, []Diagnostic) {
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
	if hasReset {
		truncated, truncErr := truncateChainForReset(routesDir, chain, resetTarget)
		if truncErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: dir, Message: truncErr.Error()})
		}
		chain = truncated
	}
	chainPkgs := make([]string, 0, len(chain))
	chainServers := make([]string, 0, len(chain))
	for _, c := range chain {
		pkg, encErr := encodeLayoutPackagePath(routesDir, c)
		if encErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: c, Message: encErr.Error()})
			continue
		}
		chainPkgs = append(chainPkgs, pkg)
		serverPath := filepath.Join(c, "_layout.server.go")
		if _, err := os.Stat(serverPath); err == nil {
			chainServers = append(chainServers, serverPath)
		} else {
			chainServers = append(chainServers, "")
		}
	}

	hasPage := has(files, "_page.svelte")
	ssrFallback := false
	if hasPage {
		fb, fbErr := detectSSRFallbackAnnotation(filepath.Join(dir, "_page.svelte"))
		if fbErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: dir, Message: fbErr.Error()})
		}
		ssrFallback = fb
	}
	route := &ScannedRoute{
		Pattern:            pattern,
		Segments:           segments,
		Dir:                dir,
		PackageName:        packageName,
		PackagePath:        packagePath,
		LayoutChain:        chain,
		LayoutPackagePaths: chainPkgs,
		LayoutServerFiles:  chainServers,
		HasPage:            hasPage,
		HasLayout:          has(files, "_layout.svelte"),
		HasError:           has(files, "_error.svelte"),
		HasReset:           hasReset,
		ResetTarget:        resetTarget,
		HasPageServer:      has(files, "_page.server.go"),
		HasLayoutServer:    has(files, "_layout.server.go"),
		HasServer:          has(files, "_server.go"),
		SSRFallback:        ssrFallback,
	}
	if boundaryDir := nearestErrorDir(routesDir, dir, dirsWithError); boundaryDir != "" {
		pkgPath, encErr := encodeLayoutPackagePath(routesDir, boundaryDir)
		if encErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: boundaryDir, Message: encErr.Error()})
		} else {
			route.ErrorBoundaryDir = boundaryDir
			route.ErrorBoundaryPackagePath = pkgPath
			route.ErrorBoundaryLayoutDepth = layoutsAtOrAbove(chain, boundaryDir)
		}
	}
	return route, diagnostics
}

// nearestErrorDir walks from dir up to routesDir and returns the first
// directory that owns a _error.svelte. Empty when none cover the route.
func nearestErrorDir(routesDir, dir string, dirsWithError map[string]struct{}) string {
	cur := dir
	for {
		if _, ok := dirsWithError[cur]; ok {
			return cur
		}
		if cur == routesDir {
			return ""
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// layoutsAtOrAbove returns the count of LayoutChain entries whose
// directory equals or is an ancestor of boundaryDir. Layouts past this
// prefix are inside the broken subtree and abort on error.
func layoutsAtOrAbove(chain []string, boundaryDir string) int {
	count := 0
	for _, layoutDir := range chain {
		if layoutDir == boundaryDir || isAncestor(layoutDir, boundaryDir) {
			count++
		}
	}
	return count
}

// isAncestor reports whether a is a strict ancestor of b. Both must be
// cleaned absolute paths sharing a separator-aligned prefix.
func isAncestor(a, b string) bool {
	if a == b {
		return false
	}
	prefix := a
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(b, prefix)
}

// truncateChainForReset trims a layout chain (ordered ancestor->self)
// to the suffix that survives a layout-reset suffix. An empty
// resetTarget yields an empty chain (root reset). A "(group)" target
// drops every entry above the nearest matching group ancestor and
// keeps the matched layout plus everything beneath it; it returns an
// error when no ancestor of the route is the named group.
func truncateChainForReset(routesDir string, chain []string, resetTarget string) ([]string, error) {
	if resetTarget == "" {
		return nil, nil
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("layout reset %q has no ancestor layout to truncate at", resetTarget)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		rel, relErr := filepath.Rel(routesDir, chain[i])
		if relErr != nil {
			return chain, fmt.Errorf("relativize layout dir %s: %w", chain[i], relErr)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			continue
		}
		parts := strings.Split(rel, "/")
		leaf := parts[len(parts)-1]
		if leaf == resetTarget {
			return chain[i:], nil
		}
	}
	return chain, fmt.Errorf("layout reset %q does not match any ancestor group of this route", resetTarget)
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
			Message: fmt.Sprintf("route %q may not have both _page.svelte and _server.go", r.Pattern),
			Hint:    "use either a page or an API endpoint",
		})
	}
	if r.HasPageServer && !r.HasPage && !r.HasServer {
		ds = append(ds, Diagnostic{
			Path:    r.Dir,
			Message: fmt.Sprintf("orphan _page.server.go: route %q has no _page.svelte or _server.go", r.Pattern),
		})
	}
	return ds
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
				Hint: "add src/params/" + seg.Matcher + "/" + seg.Matcher + ".go with `func Match(s string) bool` or use a built-in (int, uuid, slug)",
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
