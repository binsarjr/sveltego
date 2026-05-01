package typegen

import (
	"fmt"
	goast "go/ast"
)

// mapType renders one Go AST type expression as its TypeScript
// equivalent. The resolver instance is needed because nested struct
// types and named-struct references are looked up lazily and recorded
// for hoisting into the emitted `.d.ts`.
//
// Unmapped types degrade to `unknown` accompanied by a diagnostic so
// the build still emits and the developer sees the offending field
// in the build log.
func (r *structResolver) mapType(expr goast.Expr) (string, []Diagnostic) {
	switch t := expr.(type) {
	case *goast.Ident:
		return r.mapIdent(t), nil

	case *goast.StarExpr:
		inner, diags := r.mapType(t.X)
		return inner + " | null", diags

	case *goast.ArrayType:
		inner, diags := r.mapType(t.Elt)
		return inner + "[]", diags

	case *goast.MapType:
		// `map[string]T` → `Record<string, T>`. Non-string keys also
		// degrade to `Record<...>` but the build emits a diagnostic
		// because the JSON wire form coerces every key to a string.
		key, kdiags := r.mapType(t.Key)
		val, vdiags := r.mapType(t.Value)
		diags := append(kdiags, vdiags...) //nolint:gocritic // appendAssign: order matches argument order
		return fmt.Sprintf("Record<%s, %s>", key, val), diags

	case *goast.SelectorExpr:
		return mapSelector(t)

	case *goast.IndexExpr:
		return r.mapIndex(t)

	case *goast.IndexListExpr:
		// Multi-parameter generics — only `kit.Streamed[T]`,
		// `kit.Form[T]` matter for now and they are single-parameter.
		// Anything else falls through to a generic stringification.
		return r.mapIndex(&goast.IndexExpr{X: t.X, Index: t.Indices[0]})

	case *goast.StructType:
		// Inline anonymous struct: render as a TS object literal.
		nested := &structResolver{file: r.file, filePath: r.filePath, named: r.named, namedKeys: r.namedKeys}
		fields, diags := nested.fieldsFromStruct(t)
		// Keep any new named types collected during the recursive walk.
		r.namedKeys = nested.namedKeys
		r.named = nested.named
		return renderInlineObject(fields), diags

	case *goast.InterfaceType:
		// `interface{}` / `any` → `unknown` is closer to Go semantics
		// than TS's `any`, but Svelte / SvelteKit conventions use
		// `unknown` for opaque shapes. Pick `unknown` for type safety.
		return "unknown", nil
	}
	return "unknown", []Diagnostic{{
		Path:    r.filePath,
		Message: fmt.Sprintf("unsupported Go type %T; mapped to unknown", expr),
	}}
}

func (r *structResolver) mapIdent(t *goast.Ident) string {
	switch t.Name {
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"uintptr", "byte", "rune",
		"float32", "float64":
		return "number"
	case "any":
		return "unknown"
	case "error":
		return "string"
	}
	// Identifier may be a top-level struct in the same file. Record it
	// for emission and reference by name.
	if findStructDecl(r.file, t.Name) != nil {
		r.recordNamedType(t.Name)
		return t.Name
	}
	return "unknown"
}

// mapSelector handles `pkg.Type` selectors. The only well-known
// mapping is `time.Time` → ISO string. Everything else falls back to
// `unknown` with the full Go selector spelled out in the diagnostic.
func mapSelector(t *goast.SelectorExpr) (string, []Diagnostic) {
	pkg, ok := t.X.(*goast.Ident)
	if !ok || t.Sel == nil {
		return "unknown", []Diagnostic{{Message: "selector with non-ident receiver mapped to unknown"}}
	}
	switch pkg.Name + "." + t.Sel.Name {
	case "time.Time":
		return "string", nil
	}
	return "unknown", []Diagnostic{{
		Message: fmt.Sprintf("selector %s.%s has no TS mapping; defaulted to unknown", pkg.Name, t.Sel.Name),
	}}
}

// mapIndex handles instantiated generics. Only `kit.Streamed[T]` and
// `kit.Form[T]` carry framework semantics; other generics fall back
// to the inner type so a `slices.Slice[T]` or `iter.Seq[T]` does not
// break the build, but a diagnostic flags the gap.
func (r *structResolver) mapIndex(t *goast.IndexExpr) (string, []Diagnostic) {
	sel, ok := t.X.(*goast.SelectorExpr)
	if !ok {
		inner, diags := r.mapType(t.Index)
		return inner, append(diags, Diagnostic{
			Path:    r.filePath,
			Message: "non-selector generic; mapped to inner type",
		})
	}
	pkg, ok := sel.X.(*goast.Ident)
	if !ok || sel.Sel == nil {
		inner, diags := r.mapType(t.Index)
		return inner, diags
	}
	inner, diags := r.mapType(t.Index)
	switch pkg.Name + "." + sel.Sel.Name {
	case "kit.Streamed":
		return "Promise<" + inner + "[]>", diags
	case "kit.Form":
		// Form payload mirrors the user-supplied data shape; the action
		// machinery surfaces it as the same TypeScript object.
		return inner + " | null", diags
	}
	return inner, append(diags, Diagnostic{
		Path:    r.filePath,
		Message: fmt.Sprintf("generic %s.%s has no TS mapping; defaulted to inner type", pkg.Name, sel.Sel.Name),
	})
}

func renderInlineObject(fields []Field) string {
	if len(fields) == 0 {
		return "{}"
	}
	out := "{ "
	for i, f := range fields {
		if i > 0 {
			out += "; "
		}
		out += f.Name + ": " + f.TSType
	}
	out += " }"
	return out
}
