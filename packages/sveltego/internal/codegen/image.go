package codegen

import (
	"sort"
	"strconv"
	"strings"

	"github.com/binsarjr/sveltego/internal/ast"
	"github.com/binsarjr/sveltego/internal/images"
)

// imageElementName is the sveltego built-in component for build-time
// responsive images. It is intercepted before component dispatch in
// emitElement so user-written `Image.svelte` files in src/lib/ never
// shadow it; the name is reserved.
const imageElementName = "Image"

// emitImage lowers a `<Image src="..." alt="..." width="..." />` element
// to a static `<img>` tag whose src/srcset point at the build-time
// generated variants. The src attribute is required and must be a
// static literal — dynamic `src={...}` is rejected because variant
// generation is a build-time-only operation in v1.
func emitImage(b *Builder, e *ast.Element) {
	src, ok := imageSrcLiteral(e)
	if !ok {
		b.Fail(&CodegenError{
			Pos: e.P,
			Msg: `<Image> requires a static src="..." attribute (dynamic src is not supported in v1)`,
		})
		return
	}
	res, ok := b.imageVariants[src]
	if !ok {
		b.Fail(&CodegenError{
			Pos: e.P,
			Msg: "<Image> source " + strconv.Quote(src) + " was not found in static/assets/ at build time",
		})
		return
	}
	if len(res.Variants) == 0 {
		b.Fail(&CodegenError{
			Pos: e.P,
			Msg: "<Image> source " + strconv.Quote(src) + " produced no variants",
		})
		return
	}

	sortedVariants := make([]images.Variant, len(res.Variants))
	copy(sortedVariants, res.Variants)
	sort.Slice(sortedVariants, func(i, j int) bool {
		return sortedVariants[i].Width < sortedVariants[j].Width
	})

	// The fallback `src` always points at the largest variant so non-srcset
	// browsers (and lighthouse) see a real, full-resolution URL.
	fallback := sortedVariants[len(sortedVariants)-1]

	width, height, hasWidth, hasHeight := imageDims(e, res)
	loading, decoding := imageLoadingHints(e)

	var sb strings.Builder
	sb.WriteString(`<img src="`)
	sb.WriteString(escapeAttrLiteral(fallback.URL))
	sb.WriteString(`"`)

	if len(sortedVariants) > 1 {
		sb.WriteString(` srcset="`)
		for i, v := range sortedVariants {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(escapeAttrLiteral(v.URL))
			sb.WriteString(" ")
			sb.WriteString(strconv.Itoa(v.Width))
			sb.WriteString("w")
		}
		sb.WriteString(`"`)
	}

	if hasWidth {
		sb.WriteString(` width="`)
		sb.WriteString(strconv.Itoa(width))
		sb.WriteString(`"`)
	}
	if hasHeight {
		sb.WriteString(` height="`)
		sb.WriteString(strconv.Itoa(height))
		sb.WriteString(`"`)
	}

	for _, a := range e.Attributes {
		if !imagePassthroughAttr(a) {
			continue
		}
		emitImagePassthroughStatic(&sb, &a)
	}

	hasDynamicPassthrough := false
	for _, a := range e.Attributes {
		if !imagePassthroughAttr(a) {
			continue
		}
		if _, ok := a.Value.(*ast.StaticValue); ok {
			continue
		}
		if a.Value != nil {
			hasDynamicPassthrough = true
			break
		}
	}

	if !hasDynamicPassthrough {
		sb.WriteString(` loading="`)
		sb.WriteString(loading)
		sb.WriteString(`" decoding="`)
		sb.WriteString(decoding)
		sb.WriteString(`">`)
		b.Linef("w.WriteString(%s)", quoteGo(sb.String()))
		return
	}

	b.Linef("w.WriteString(%s)", quoteGo(sb.String()))
	for _, a := range e.Attributes {
		if !imagePassthroughAttr(a) {
			continue
		}
		emitImagePassthroughDynamic(b, &a)
	}
	b.Linef("w.WriteString(%s)", quoteGo(` loading="`+loading+`" decoding="`+decoding+`">`))
}

// imageSrcLiteral returns the static src= value on an <Image> element
// and whether one was found. Dynamic src={...} forms return ok=false so
// emitImage can surface a clear diagnostic.
func imageSrcLiteral(e *ast.Element) (string, bool) {
	for i := range e.Attributes {
		a := &e.Attributes[i]
		if a.Name != "src" {
			continue
		}
		if a.Kind != ast.AttrStatic {
			return "", false
		}
		if v, ok := a.Value.(*ast.StaticValue); ok {
			return strings.TrimPrefix(v.Value, "/"), true
		}
		return "", false
	}
	return "", false
}

// imageDims returns the width/height pair to emit on the <img>. User-
// supplied static values win; otherwise the intrinsic dimensions of the
// source image flow through. Returning has-flags lets the caller skip
// the attribute entirely when neither source provided a value (e.g.
// pre-decode failure paths).
func imageDims(e *ast.Element, res images.Result) (w, h int, hasW, hasH bool) {
	for i := range e.Attributes {
		a := &e.Attributes[i]
		if a.Kind != ast.AttrStatic {
			continue
		}
		switch a.Name {
		case "width":
			if v, ok := a.Value.(*ast.StaticValue); ok {
				if n, err := strconv.Atoi(v.Value); err == nil {
					w = n
					hasW = true
				}
			}
		case "height":
			if v, ok := a.Value.(*ast.StaticValue); ok {
				if n, err := strconv.Atoi(v.Value); err == nil {
					h = n
					hasH = true
				}
			}
		}
	}
	if !hasW && res.IntrinsicWidth > 0 {
		w = res.IntrinsicWidth
		hasW = true
	}
	if !hasH && res.IntrinsicHeight > 0 {
		h = res.IntrinsicHeight
		hasH = true
	}
	return w, h, hasW, hasH
}

// imageLoadingHints returns the loading and decoding attribute values.
// Default is loading=lazy, decoding=async. The `eager` boolean prop
// flips loading to "eager" for above-the-fold imagery; SvelteKit's
// enhanced-img calls this `priority`, but `eager` mirrors the actual
// HTML attribute value and reads cleaner in templates.
func imageLoadingHints(e *ast.Element) (loading, decoding string) {
	loading = "lazy"
	decoding = "async"
	for i := range e.Attributes {
		a := &e.Attributes[i]
		if a.Name == "eager" || a.Name == "priority" {
			loading = "eager"
		}
	}
	return loading, decoding
}

// imagePassthroughAttr reports whether attr a should flow through to
// the emitted <img>. We pass alt, sizes, class, id, title, style, role,
// aria-* and data-*; src/width/height/loading/decoding/eager/priority
// are handled explicitly above. Dynamic class/style/etc. are not yet
// supported; treat them as static-only for v1 to keep the lowering
// simple. Class and style directives on <Image> are similarly ignored.
func imagePassthroughAttr(a ast.Attribute) bool {
	switch a.Name {
	case "src", "width", "height", "loading", "decoding", "eager", "priority":
		return false
	}
	switch a.Kind {
	case ast.AttrEventHandler, ast.AttrBind, ast.AttrUse,
		ast.AttrClassDirective, ast.AttrStyleDirective, ast.AttrLet:
		return false
	}
	return true
}

// emitImagePassthroughStatic appends a static attribute (and the
// surrounding space + name=) to the in-progress static head buffer.
// Dynamic forms are deferred to emitImagePassthroughDynamic.
func emitImagePassthroughStatic(sb *strings.Builder, a *ast.Attribute) {
	if a.Value == nil {
		sb.WriteByte(' ')
		sb.WriteString(a.Name)
		return
	}
	if v, ok := a.Value.(*ast.StaticValue); ok {
		sb.WriteByte(' ')
		sb.WriteString(a.Name)
		sb.WriteString(`="`)
		sb.WriteString(escapeAttrLiteral(v.Value))
		sb.WriteString(`"`)
	}
}

// emitImagePassthroughDynamic emits the codegen lines required to
// stream a dynamic alt={...} or sizes={...} value into the open tag.
// Because the static head is already flushed by the caller, this
// appends to the running output via b.Line directly.
func emitImagePassthroughDynamic(b *Builder, a *ast.Attribute) {
	switch v := a.Value.(type) {
	case *ast.DynamicValue:
		b.Linef("w.WriteString(%s)", quoteGo(" "+a.Name+`="`))
		b.Linef("w.WriteEscapeAttr(%s)", v.Expr)
		b.Linef("w.WriteString(%s)", quoteGo(`"`))
	case *ast.InterpolatedValue:
		b.Linef("w.WriteString(%s)", quoteGo(" "+a.Name+`="`))
		for _, part := range v.Parts {
			switch p := part.(type) {
			case *ast.Text:
				if p.Value != "" {
					b.Linef("w.WriteString(%s)", quoteGo(escapeAttrLiteral(p.Value)))
				}
			case *ast.Mustache:
				b.Linef("w.WriteEscapeAttr(%s)", p.Expr)
			}
		}
		b.Linef("w.WriteString(%s)", quoteGo(`"`))
	}
}

// collectImageSources walks a parsed fragment and returns every static
// src referenced by an `<Image>` element. The result is deduplicated and
// sorted so build-time variant generation is deterministic.
func collectImageSources(frag *ast.Fragment) []string {
	if frag == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var visit func([]ast.Node)
	visit = func(nodes []ast.Node) {
		for _, n := range nodes {
			switch v := n.(type) {
			case *ast.Element:
				if v.Name == imageElementName {
					if src, ok := imageSrcLiteral(v); ok {
						seen[src] = struct{}{}
					}
				}
				visit(v.Children)
			case *ast.IfBlock:
				visit(v.Then)
				for _, eb := range v.Elifs {
					visit(eb.Body)
				}
				visit(v.Else)
			case *ast.EachBlock:
				visit(v.Body)
				visit(v.Else)
			case *ast.AwaitBlock:
				visit(v.Pending)
				visit(v.Then)
				visit(v.Catch)
			case *ast.KeyBlock:
				visit(v.Body)
			case *ast.SnippetBlock:
				visit(v.Body)
			}
		}
	}
	visit(frag.Children)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
