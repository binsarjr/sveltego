package codegen

import (
	"bufio"
	"bytes"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// userSourceFile describes one user-authored .go file under src/routes/
// that the build pipeline must mirror into .gen/usersrc/<encoded>/ so the
// wire glue can import it via a Go-valid path. UserPath is the absolute
// source path; MirrorPath is the absolute destination; PackageName is
// the encoded segment used as the mirror's package clause.
type userSourceFile struct {
	UserPath    string
	MirrorPath  string
	PackageName string
	// HasActions is set after the user file is parsed; the wire emitter
	// only emits the Actions adapter when the symbol exists.
	HasActions bool
}

// mirrorUserSource copies one user .go file into the mirror tree. The
// build constraint header (`//go:build sveltego`) is stripped so the
// mirror compiles under the default Go toolchain; the package clause is
// rewritten to the encoded segment so the directory's import path
// agrees with its package identifier. Other source bytes are preserved
// verbatim. The function also reports whether the file declares an
// exported `Actions` function; the wire emitter consults this flag.
func mirrorUserSource(in *userSourceFile) error {
	src, err := os.ReadFile(in.UserPath) //nolint:gosec // path comes from scanner walk
	if err != nil {
		return fmt.Errorf("codegen: read %s: %w", in.UserPath, err)
	}

	stripped := stripBuildConstraint(src)

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, in.UserPath, stripped, parser.AllErrors|parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("codegen: parse %s: %w", in.UserPath, err)
	}
	in.HasActions = hasActionsVar(parsed)

	rewritten, err := rewritePackageClause(stripped, in.PackageName)
	if err != nil {
		return fmt.Errorf("codegen: rewrite package clause for %s: %w", in.UserPath, err)
	}

	formatted, err := format.Source(rewritten)
	if err != nil {
		return fmt.Errorf("codegen: format mirror %s: %w", in.MirrorPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(in.MirrorPath), 0o755); err != nil {
		return fmt.Errorf("codegen: mkdir %s: %w", filepath.Dir(in.MirrorPath), err)
	}
	if err := os.WriteFile(in.MirrorPath, formatted, genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", in.MirrorPath, err)
	}
	return nil
}

// stripBuildConstraint drops a leading `//go:build` line and any blank
// lines immediately following it, leaving the rest of src untouched.
// Other build constraints earlier in the file are preserved; only the
// first `//go:build` is removed because that is the user-mandated
// `//go:build sveltego` marker.
func stripBuildConstraint(src []byte) []byte {
	var out bytes.Buffer
	scan := bufio.NewScanner(bytes.NewReader(src))
	scan.Buffer(make([]byte, 0, 64*1024), 1<<20)
	dropped := false
	for scan.Scan() {
		line := scan.Text()
		trimmed := strings.TrimSpace(line)
		if !dropped {
			if trimmed == "" {
				out.WriteString(line)
				out.WriteByte('\n')
				continue
			}
			if strings.HasPrefix(trimmed, "//go:build") {
				dropped = true
				continue
			}
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// rewritePackageClause replaces the file's package clause with `package
// <name>`, preserving every other byte. parser.ParseFile is used to
// locate the clause's byte range so neighbouring whitespace and
// comments survive.
func rewritePackageClause(src []byte, name string) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "mirror.go", src, parser.PackageClauseOnly)
	if err != nil {
		return nil, err
	}
	startOff := fset.Position(f.Package).Offset
	endOff := fset.Position(f.Name.End()).Offset
	if startOff < 0 || endOff > len(src) || startOff > endOff {
		return nil, fmt.Errorf("invalid package clause offsets %d..%d", startOff, endOff)
	}
	var out bytes.Buffer
	out.Write(src[:startOff])
	fmt.Fprintf(&out, "package %s", name)
	out.Write(src[endOff:])
	return out.Bytes(), nil
}

// hasActionsVar reports whether the parsed file declares a top-level
// `var Actions ...` (any type or initializer). Form actions are
// authored as `var Actions = kit.ActionMap{...}` per spec; the wire
// emitter consults this flag to decide whether to reference the symbol
// or emit a nil-returning stub.
func hasActionsVar(f *goast.File) bool {
	for _, decl := range f.Decls {
		gd, ok := decl.(*goast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*goast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name != nil && name.Name == "Actions" {
					return true
				}
			}
		}
	}
	return false
}

// emitWire writes one wire.gen.go per route with a +page.server.go (or
// other server .go file). The wire file lives next to page.gen.go in
// the encoded gen directory; it imports the user-source mirror by an
// alias because the mirror's package name and the gen package name
// often collide.
func emitWire(genRoot, modulePath string, route mirrorRoute) error {
	importPath := modulePath + "/" + genRoot + "/usersrc/" + route.encodedSubpath

	var b Builder
	b.Line("// Code generated by sveltego. DO NOT EDIT.")
	b.Linef("package %s", route.packageName)
	b.Line("")
	b.Line("import (")
	b.Indent()
	b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
	b.Line("")
	b.Linef(`usersrc %q`, importPath)
	b.Dedent()
	b.Line(")")
	b.Line("")

	b.Line("// Load wraps the user-authored Load() so the manifest can reference")
	b.Line("// it through the gen package. The wrapper widens any concrete")
	b.Line("// PageData return type to `any` for router.LoadHandler.")
	b.Line("func Load(ctx *kit.LoadCtx) (any, error) { return usersrc.Load(ctx) }")

	b.Line("")
	if route.hasActions {
		b.Line("// Actions wraps the user-authored `var Actions kit.ActionMap` so the")
		b.Line("// manifest can reference it through the gen package as router.ActionsHandler.")
		b.Line("func Actions() any { return usersrc.Actions }")
	} else {
		b.Line("// Actions is emitted as a nil-returning stub because the user's")
		b.Line("// page.server.go does not declare an Actions variable. The manifest")
		b.Line("// references this symbol unconditionally when HasPageServer is set.")
		b.Line("func Actions() any { return nil }")
	}

	if err := b.Err(); err != nil {
		return err
	}
	out, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("codegen: format wire source: %w", err)
	}

	if err := os.MkdirAll(route.wireDir, 0o755); err != nil {
		return fmt.Errorf("codegen: mkdir %s: %w", route.wireDir, err)
	}
	target := filepath.Join(route.wireDir, "wire.gen.go")
	if err := os.WriteFile(target, out, genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return nil
}

// mirrorRoute carries everything emitWire needs for one route. The
// fields collapse the project root, encoded subpath, and Actions
// detection so the wire emitter does no path arithmetic of its own.
type mirrorRoute struct {
	encodedSubpath string // routes/posts/_slug_
	packageName    string // _slug_
	wireDir        string // <abs>/.gen/routes/posts/_slug_
	hasActions     bool
}

// emitLayoutWire writes wire_layout.gen.go beside layout.gen.go in the
// encoded gen directory. The file declares LayoutLoad, a LoadHandler
// adapter that re-exports the user-authored Load() from the layout
// server mirror at .gen/layoutsrc/<encoded>/layout_server.go. The
// layoutsrc tree is a sibling of usersrc so a route directory that owns
// both +page.server.go and +layout.server.go does not produce two Load
// declarations in the same Go package.
func emitLayoutWire(genRoot, modulePath string, route mirrorRoute) error {
	importPath := modulePath + "/" + genRoot + "/layoutsrc/" + route.encodedSubpath

	var b Builder
	b.Line("// Code generated by sveltego. DO NOT EDIT.")
	b.Linef("package %s", route.packageName)
	b.Line("")
	b.Line("import (")
	b.Indent()
	b.Line(`"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"`)
	b.Line("")
	b.Linef(`usersrc %q`, importPath)
	b.Dedent()
	b.Line(")")
	b.Line("")

	b.Line("// LayoutLoad wraps the user-authored Load() from layout.server.go")
	b.Line("// so the manifest can reference it through the gen package. The")
	b.Line("// wrapper widens any concrete LayoutData return to `any` for")
	b.Line("// router.LayoutLoadHandler.")
	b.Line("func LayoutLoad(ctx *kit.LoadCtx) (any, error) { return usersrc.Load(ctx) }")

	if err := b.Err(); err != nil {
		return err
	}
	out, err := format.Source(b.Bytes())
	if err != nil {
		return fmt.Errorf("codegen: format layout wire source: %w", err)
	}

	if err := os.MkdirAll(route.wireDir, 0o755); err != nil {
		return fmt.Errorf("codegen: mkdir %s: %w", route.wireDir, err)
	}
	target := filepath.Join(route.wireDir, "wire_layout.gen.go")
	if err := os.WriteFile(target, out, genFileMode); err != nil {
		return fmt.Errorf("codegen: write %s: %w", target, err)
	}
	return nil
}
