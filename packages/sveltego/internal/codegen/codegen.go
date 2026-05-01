package codegen

import (
	"errors"
	"fmt"
	"go/format"
	"sort"
	"strings"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/packages/sveltego/internal/images"
)

// Options configures Generate.
type Options struct {
	// PackageName is written verbatim into the generated `package` clause.
	PackageName string
	// ServerFilePath optionally points at a sibling _page.server.go whose
	// Load() inline struct return is used to infer PageData fields.
	ServerFilePath string
	// HasActions is true when the sibling page.server.go declares
	// `var Actions kit.ActionMap`. The page's PageData gains a `Form any`
	// field so action result data can be threaded through the render.
	HasActions bool
	// Filename is the .svelte source path used to seed the CSS scope
	// hash so client and server agree on the `svelte-<hash>` class name.
	// Empty falls back to hashing the CSS body, matching upstream's
	// default cssHash rule.
	Filename string
	// Provenance, when true (the default in [Build]), emits per-span
	// // gen: source=<path>:<line> kind=<kind> comments so LLMs and
	// humans can trace generated code back to its .svelte source line.
	// --no-provenance sets this to false (keeps the file-level banner).
	Provenance bool
	// SourceContent holds the raw .svelte source bytes used to compute
	// the SHA-256 template hash in the file-level banner. When nil the
	// banner omits the hash line.
	SourceContent []byte
	// GeneratedAt pins the timestamp in the file-level banner. Zero uses
	// time.Now() so tests can pass a fixed value for deterministic output.
	GeneratedAt time.Time
	// ImageVariants maps each <Image src=...> path the build pipeline
	// resolved to its generated variant set. The codegen-time lookup
	// rejects unknown sources with a clear diagnostic.
	ImageVariants map[string]images.Result
	// MirrorImportPath is the Go import path of the user-source mirror
	// (`<module>/.gen/usersrc/<encoded>`) for this route. Set by the
	// build driver when a sibling page.server.go exists. Generate emits
	// `type PageData = <usersrc-alias>.PageData` whenever the server file
	// declares a named PageData type, preserving type identity between
	// the user-authored type and the manifest's adapter assertion. Empty
	// preserves the inline-struct alias behavior.
	MirrorImportPath string
}

// LayoutOptions configures GenerateLayout.
type LayoutOptions struct {
	// PackageName is written verbatim into the generated `package` clause.
	PackageName string
	// ServerFilePath optionally points at a sibling _layout.server.go whose
	// Load() inline struct return is used to infer LayoutData fields.
	ServerFilePath string
	// Filename is the .svelte source path used to seed the CSS scope
	// hash so client and server agree on the `svelte-<hash>` class name.
	Filename string
	// Provenance mirrors Options.Provenance for layout files.
	Provenance bool
	// SourceContent mirrors Options.SourceContent for layout files.
	SourceContent []byte
	// GeneratedAt mirrors Options.GeneratedAt for layout files.
	GeneratedAt time.Time
	// ImageVariants mirrors Options.ImageVariants for layout files.
	ImageVariants map[string]images.Result
	// MirrorImportPath mirrors Options.MirrorImportPath for layouts. When
	// the layout server file declares a named LayoutData type, the
	// emitter aliases to `<usersrc>.LayoutData` instead of synthesizing
	// an inline struct, so the manifest's runtime type assertion sees
	// the user-authored type.
	MirrorImportPath string
}

// ErrorPageOptions configures GenerateErrorPage.
type ErrorPageOptions struct {
	// PackageName is written verbatim into the generated `package` clause.
	PackageName string
	// Filename is the .svelte source path used to seed the CSS scope hash.
	Filename string
	// Provenance mirrors Options.Provenance for error page files.
	Provenance bool
	// SourceContent mirrors Options.SourceContent for error page files.
	SourceContent []byte
	// GeneratedAt mirrors Options.GeneratedAt for error page files.
	GeneratedAt time.Time
}

// Generate lowers frag to Go source for a Page.Render method. The returned
// bytes pass through go/format so the caller receives canonically
// formatted Go.
func Generate(frag *ast.Fragment, opts Options) ([]byte, error) {
	if frag == nil {
		return nil, errors.New("codegen: nil fragment")
	}
	if opts.PackageName == "" {
		return nil, errors.New("codegen: empty package name")
	}

	svelteOpts, err := extractSvelteOptions(frag)
	if err != nil {
		return nil, err
	}
	scripts, err := extractScripts(frag)
	if err != nil {
		return nil, err
	}
	if err := validateRunesOption(svelteOpts, scripts); err != nil {
		return nil, err
	}
	style, err := extractStyle(frag, opts.Filename)
	if err != nil {
		return nil, err
	}
	applyScopeClass(frag.Children, style.ScopeClass)
	pageData, err := inferPageData(opts.ServerFilePath)
	if err != nil {
		return nil, err
	}
	mirrorAlias := ""
	if pageData.HasNamedType && opts.MirrorImportPath != "" {
		mirrorAlias = "usersrc"
	}
	if opts.HasActions && !pageData.HasNamedType {
		// Remove any user-declared Form field before injecting the contract field.
		// This allows page.server.go to declare `Form any` in its Load return
		// without causing a duplicate-field compile error. See #143. The
		// named-type branch leaves Form to the user's authored declaration.
		pageData.Fields = dropField(pageData.Fields, "Form")
		pageData.Fields = append(pageData.Fields, pageDataField{Name: "Form", Type: "any"})
	}

	pageDataImports := pageData.Imports
	if mirrorAlias != "" {
		pageDataImports = append(pageDataImports, fmt.Sprintf("%s %q", mirrorAlias, opts.MirrorImportPath))
	}
	imports := mergeImports(scripts.Imports, pageDataImports)
	headChildren, bodyChildren := extractHeadChildren(frag.Children)

	var b Builder
	b.provenance = opts.Provenance
	b.srcPath = opts.Filename
	b.imageVariants = opts.ImageVariants
	if opts.Filename != "" {
		ts := opts.GeneratedAt
		if ts.IsZero() {
			ts = time.Now()
		}
		b.Line(headerComment(opts.Filename, opts.SourceContent, provenanceVersion, ts))
	} else {
		b.Line("// Code generated by sveltego. DO NOT EDIT.")
	}
	b.Linef("package %s", opts.PackageName)
	b.Line("")
	b.Line("import (")
	b.Indent()
	for _, imp := range imports {
		b.Line(imp)
	}
	b.Dedent()
	b.Line(")")
	b.Line("")

	for _, decl := range scripts.Decls {
		b.Line(decl)
		b.Line("")
	}

	b.Line("type Page struct{}")
	b.Line("")
	emitPageDataStruct(&b, pageData.Fields, mirrorAlias)
	b.Line("")
	if scripts.HasProps {
		emitPropsStruct(&b, scripts.Props)
		b.Line("")
	}
	b.Line("func (p Page) Render(w *render.Writer, ctx *kit.RenderCtx, data PageData) error {")
	b.Indent()
	b.Line("_ = ctx")
	b.Line("_ = data")
	if scripts.HasProps {
		b.Line("var props Props")
		b.Line("defaultProps(&props)")
		b.Line("_ = props")
	}
	emitRuneStmts(&b, scripts.RuneStmts)
	rejectRootConst(&b, bodyChildren)
	rejectNestedHead(&b, bodyChildren)
	emitChildren(&b, bodyChildren)
	emitStyleBlock(&b, style)
	b.Line("return nil")
	b.Dedent()
	b.Line("}")

	if len(headChildren) > 0 {
		b.Line("")
		emitPageHead(&b, scripts, headChildren)
	}

	if err := b.Err(); err != nil {
		return nil, err
	}

	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("codegen: format generated source: %w", err)
	}
	return restoreRunesBytes(out), nil
}

// GenerateLayout lowers frag to Go source for a Layout.Render method.
// The signature mirrors Generate but accepts a `children` callback that
// any <slot /> element in the template lowers to. LayoutData is emitted
// as an empty type alias today; Phase 0k-B introduces real data via
// _layout.server.go.
func GenerateLayout(frag *ast.Fragment, opts LayoutOptions) ([]byte, error) {
	if frag == nil {
		return nil, errors.New("codegen: nil fragment")
	}
	if opts.PackageName == "" {
		return nil, errors.New("codegen: empty package name")
	}

	svelteOpts, err := extractSvelteOptions(frag)
	if err != nil {
		return nil, err
	}
	scripts, err := extractScripts(frag)
	if err != nil {
		return nil, err
	}
	if err := validateRunesOption(svelteOpts, scripts); err != nil {
		return nil, err
	}
	style, err := extractStyle(frag, opts.Filename)
	if err != nil {
		return nil, err
	}
	applyScopeClass(frag.Children, style.ScopeClass)
	layoutData, err := inferLayoutData(opts.ServerFilePath)
	if err != nil {
		return nil, err
	}
	mirrorAlias := ""
	if layoutData.HasNamedType && opts.MirrorImportPath != "" {
		mirrorAlias = "usersrc"
	}

	layoutDataImports := layoutData.Imports
	if mirrorAlias != "" {
		layoutDataImports = append(layoutDataImports, fmt.Sprintf("%s %q", mirrorAlias, opts.MirrorImportPath))
	}
	imports := mergeImports(scripts.Imports, layoutDataImports)
	headChildren, bodyChildren := extractHeadChildren(frag.Children)

	var b Builder
	b.hasChildren = true
	b.provenance = opts.Provenance
	b.srcPath = opts.Filename
	b.imageVariants = opts.ImageVariants
	if opts.Filename != "" {
		ts := opts.GeneratedAt
		if ts.IsZero() {
			ts = time.Now()
		}
		b.Line(headerComment(opts.Filename, opts.SourceContent, provenanceVersion, ts))
	} else {
		b.Line("// Code generated by sveltego. DO NOT EDIT.")
	}
	b.Linef("package %s", opts.PackageName)
	b.Line("")
	b.Line("import (")
	b.Indent()
	for _, imp := range imports {
		b.Line(imp)
	}
	b.Dedent()
	b.Line(")")
	b.Line("")

	for _, decl := range scripts.Decls {
		b.Line(decl)
		b.Line("")
	}

	b.Line("type Layout struct{}")
	b.Line("")
	emitLayoutDataAlias(&b, layoutData.Fields, mirrorAlias)
	b.Line("")
	if scripts.HasProps {
		emitPropsStruct(&b, scripts.Props)
		b.Line("")
	}
	b.Line("func (l Layout) Render(w *render.Writer, ctx *kit.RenderCtx, data LayoutData, children func(*render.Writer) error) error {")
	b.Indent()
	b.Line("_ = ctx")
	b.Line("_ = data")
	b.Line("_ = children")
	if scripts.HasProps {
		b.Line("var props Props")
		b.Line("defaultProps(&props)")
		b.Line("_ = props")
	}
	emitRuneStmts(&b, scripts.RuneStmts)
	rejectRootConst(&b, bodyChildren)
	rejectNestedHead(&b, bodyChildren)
	emitChildren(&b, bodyChildren)
	emitStyleBlock(&b, style)
	b.Line("return nil")
	b.Dedent()
	b.Line("}")

	if len(headChildren) > 0 {
		b.Line("")
		emitLayoutHead(&b, scripts, headChildren)
	}

	if err := b.Err(); err != nil {
		return nil, err
	}

	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("codegen: format generated source: %w", err)
	}
	return restoreRunesBytes(out), nil
}

// GenerateErrorPage lowers a _error.svelte fragment to Go source for an
// ErrorPage.Render method. The render parameter binds to kit.SafeError so
// templates reference {data.Code}, {data.Message}, {data.ID} directly.
func GenerateErrorPage(frag *ast.Fragment, opts ErrorPageOptions) ([]byte, error) {
	if frag == nil {
		return nil, errors.New("codegen: nil fragment")
	}
	if opts.PackageName == "" {
		return nil, errors.New("codegen: empty package name")
	}

	svelteOpts, err := extractSvelteOptions(frag)
	if err != nil {
		return nil, err
	}
	scripts, err := extractScripts(frag)
	if err != nil {
		return nil, err
	}
	if err := validateRunesOption(svelteOpts, scripts); err != nil {
		return nil, err
	}
	style, err := extractStyle(frag, opts.Filename)
	if err != nil {
		return nil, err
	}
	applyScopeClass(frag.Children, style.ScopeClass)

	imports := mergeImports(scripts.Imports, nil)

	var b Builder
	b.provenance = opts.Provenance
	b.srcPath = opts.Filename
	if opts.Filename != "" {
		ts := opts.GeneratedAt
		if ts.IsZero() {
			ts = time.Now()
		}
		b.Line(headerComment(opts.Filename, opts.SourceContent, provenanceVersion, ts))
	} else {
		b.Line("// Code generated by sveltego. DO NOT EDIT.")
	}
	b.Linef("package %s", opts.PackageName)
	b.Line("")
	b.Line("import (")
	b.Indent()
	for _, imp := range imports {
		b.Line(imp)
	}
	b.Dedent()
	b.Line(")")
	b.Line("")

	for _, decl := range scripts.Decls {
		b.Line(decl)
		b.Line("")
	}

	b.Line("type ErrorPage struct{}")
	b.Line("")
	if scripts.HasProps {
		emitPropsStruct(&b, scripts.Props)
		b.Line("")
	}
	b.Line("func (e ErrorPage) Render(w *render.Writer, ctx *kit.RenderCtx, data kit.SafeError) error {")
	b.Indent()
	b.Line("_ = ctx")
	b.Line("_ = data")
	if scripts.HasProps {
		b.Line("var props Props")
		b.Line("defaultProps(&props)")
		b.Line("_ = props")
	}
	emitRuneStmts(&b, scripts.RuneStmts)
	rejectRootConst(&b, frag.Children)
	emitChildren(&b, frag.Children)
	emitStyleBlock(&b, style)
	b.Line("return nil")
	b.Dedent()
	b.Line("}")

	if err := b.Err(); err != nil {
		return nil, err
	}

	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("codegen: format generated source: %w", err)
	}
	return restoreRunesBytes(out), nil
}

// emitLayoutDataAlias writes the layout's LayoutData type alias.
// Mirrors emitPageDataStruct's three-shape contract: when mirrorAlias is
// non-empty, emits `type LayoutData = <alias>.LayoutData` so the
// manifest's runtime assertion against the user-authored type
// succeeds; otherwise emits the inline-struct alias form.
func emitLayoutDataAlias(b *Builder, fields []pageDataField, mirrorAlias string) {
	if mirrorAlias != "" {
		b.Linef("type LayoutData = %s.LayoutData", mirrorAlias)
		return
	}
	if len(fields) == 0 {
		b.Line("type LayoutData = struct{}")
		return
	}
	b.Line("type LayoutData = struct {")
	b.Indent()
	for _, fd := range fields {
		b.Linef("%s %s", fd.Name, fd.Type)
	}
	b.Dedent()
	b.Line("}")
}

// mergeImports unions framework imports with script + pagedata imports,
// dedupes by full import path, and returns a stable-sorted slice ready to
// emit between `import (` and `)`.
func mergeImports(scriptImports, pageDataImports []string) []string {
	set := map[string]struct{}{
		`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`: {},
		`"github.com/binsarjr/sveltego/packages/sveltego/render"`:      {},
	}
	for _, imp := range scriptImports {
		set[imp] = struct{}{}
	}
	for _, imp := range pageDataImports {
		set[imp] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// emitRuneStmts writes the lowered $state / $derived / $derived.by
// statements at the top of Render's body. $effect entries lower to a
// trailing comment marker so the generated source still records that an
// effect was elided; the marker carries no runtime cost.
func emitRuneStmts(b *Builder, stmts []runeStmt) {
	for _, s := range stmts {
		switch s.Kind {
		case runeEffect:
			b.Line("// $effect elided on SSR")
		default:
			if s.Body == "" {
				continue
			}
			b.Line(s.Body)
			if name := assignLHSName(s.Body); name != "" {
				b.Linef("_ = %s", name)
			}
		}
	}
}

// assignLHSName returns the LHS identifier of a `name := expr` line, or
// "" when src does not start with a short-var-decl. Used to emit a
// trailing `_ = name` so the Go compiler accepts vars that appear only
// in template mustaches the codegen pass has not yet observed.
func assignLHSName(src string) string {
	src = strings.TrimSpace(src)
	idx := strings.Index(src, ":=")
	if idx <= 0 {
		return ""
	}
	head := strings.TrimSpace(src[:idx])
	if !isIdent(head) {
		return ""
	}
	return head
}

// rejectRootConst latches a CodegenError when {@const} appears as a
// direct child of the template root. Svelte requires {@const} to live
// inside a block (e.g. {#each}, {#if}); a root-level declaration has no
// surrounding scope to attach to and is rejected at codegen time.
func rejectRootConst(b *Builder, children []ast.Node) {
	for _, c := range children {
		if cn, ok := c.(*ast.Const); ok {
			b.Fail(&CodegenError{
				Pos: cn.P,
				Msg: "{@const} not allowed at template root; place inside {#each}, {#if}, or another block",
			})
			return
		}
	}
}

// emitChildren walks a sibling list and dispatches by node kind. Adjacent
// Text nodes are merged before iteration so the emitted code has one
// WriteString per run.
func emitChildren(b *Builder, children []ast.Node) {
	merged := mergeAdjacentText(children)
	for _, child := range merged {
		emitNode(b, child)
	}
}

func emitNode(b *Builder, n ast.Node) {
	if b.provenance {
		b.emitSpanComment(n)
	}
	switch v := n.(type) {
	case *ast.Text:
		emitText(b, v)
	case *ast.Element:
		emitElement(b, v)
	case *ast.Mustache:
		emitMustache(b, v)
	case *ast.IfBlock:
		emitIfBlock(b, v)
	case *ast.EachBlock:
		emitEachBlock(b, v)
	case *ast.AwaitBlock:
		emitAwaitBlock(b, v)
	case *ast.KeyBlock:
		emitKeyBlock(b, v)
	case *ast.SnippetBlock:
		emitSnippetBlock(b, v)
	case *ast.RawHTML:
		emitRawHTML(b, v)
	case *ast.Const:
		emitConst(b, v)
	case *ast.Render:
		emitRender(b, v)
	case *ast.Script:
		// extracted to package scope by extractScripts
	case *ast.Style:
		// TODO: <style> extraction (#16)
	case *ast.Comment:
	}
}

// emitSpanComment writes a // gen: source=... kind=... comment before the
// span when b.provenance is true. It is a no-op for node kinds that produce
// no runtime output (Script, Style, Comment).
func (b *Builder) emitSpanComment(n ast.Node) {
	var kind string
	switch n.(type) {
	case *ast.Text:
		kind = "text"
	case *ast.Element:
		kind = "element"
	case *ast.Mustache:
		kind = "mustache"
	case *ast.IfBlock:
		kind = "if"
	case *ast.EachBlock:
		kind = "each"
	case *ast.AwaitBlock:
		kind = "await"
	case *ast.KeyBlock:
		kind = "key"
	case *ast.SnippetBlock:
		kind = "snippet"
	case *ast.RawHTML:
		kind = "rawhtml"
	case *ast.Const:
		kind = "const"
	case *ast.Render:
		kind = "render"
	default:
		// Script, Style, Comment produce no runtime output; skip.
		return
	}
	b.Line(spanComment(b.srcPath, n.Pos().Line, kind))
}
