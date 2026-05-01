package typegen

// Shape exposes the per-route data type information SSR
// property-access lowering (Phase 5, #427) needs to translate a
// JS-style member chain into a Go field chain. RootType is the
// struct identifier that backs `data` for the route ("PageData" or
// "LayoutData" for the typegen-driven cases). Types holds every
// referenced struct (root included) keyed by Go type name, with each
// entry's fields carrying both JSON tag and Go field identifier.
//
// The map is closed under the route — only types reachable from the
// root appear. Anonymous nested structs do not produce an entry; the
// lowerer treats them as opaque (unknown) so a route trying to lower
// `data.someInline.thing` hard-errors with a useful suggestion.
//
// Shape is consumed read-only; callers must not mutate the returned
// values.
type Shape struct {
	RootType string
	Types    map[string]ShapeType
}

// ShapeType is one struct's fields, indexed by JSON-tag name. Lookup
// returns the matching field plus whether it was found; callers use
// Found = false to surface a hard build error in strict modes.
type ShapeType struct {
	Name   string
	Fields []Field
}

// Lookup finds the field in t whose JSON tag (Field.Name) matches
// jsonTag. Returns the zero Field and false when the tag is not
// declared on the struct.
func (t ShapeType) Lookup(jsonTag string) (Field, bool) {
	for _, f := range t.Fields {
		if f.Name == jsonTag {
			return f, true
		}
	}
	return Field{}, false
}

// BuildShape parses srcPath and assembles a [Shape] for SSR lowering.
// kind selects between page (PageData) and layout (LayoutData)
// shapes; it controls the resolved root type identifier and the file
// the walker associates with diagnostics.
//
// Returns the Shape, any non-fatal walker diagnostics, and an error
// only when the source file cannot be parsed at all. A missing Load
// function returns a Shape with the requested RootType and an empty
// Types map — the lowerer then strict-errors on every member access,
// which is the desired behaviour for routes that declare no data.
func BuildShape(srcPath string, kind Kind) (Shape, []Diagnostic, error) {
	_, _, dataIdent, typeIdent := kindFilenames(kind)
	_ = dataIdent

	rootShape, diags, err := walkServerFile(srcPath, typeIdent)
	if err != nil {
		return Shape{}, diags, err
	}

	types := map[string]ShapeType{
		typeIdent: {Name: typeIdent, Fields: rootShape.Fields},
	}
	for _, nt := range rootShape.NamedTypes {
		if _, ok := types[nt.Name]; ok {
			continue
		}
		types[nt.Name] = ShapeType(nt)
	}
	return Shape{RootType: typeIdent, Types: types}, diags, nil
}
