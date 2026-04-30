package codegen

import (
	"strings"
	"testing"
)

// TestEmitPageDataStruct_AliasForm pins the type-alias emit shape for #109.
// The alias `=` is load-bearing: it preserves type identity between the
// user's anonymous struct literal returned by Load() and the named
// PageData symbol referenced by the manifest adapter. A new named type
// (no `=`) would force a value conversion at the wire boundary.
func TestEmitPageDataStruct_AliasForm(t *testing.T) {
	t.Parallel()

	t.Run("non-empty fields emit alias with body", func(t *testing.T) {
		t.Parallel()
		var b Builder
		emitPageDataStruct(&b, []pageDataField{
			{Name: "Greeting", Type: "string"},
			{Name: "Count", Type: "int"},
		})
		got := string(b.Bytes())
		if !strings.Contains(got, "type PageData = struct {") {
			t.Errorf("expected alias form `type PageData = struct {`, got:\n%s", got)
		}
		if strings.Contains(got, "type PageData struct {") {
			t.Errorf("alias `=` missing; new-type form would break runtime assertion (#109):\n%s", got)
		}
		if !strings.Contains(got, "Greeting string") {
			t.Errorf("missing Greeting field:\n%s", got)
		}
		if !strings.Contains(got, "Count int") {
			t.Errorf("missing Count field:\n%s", got)
		}
	})

	t.Run("empty fields emit alias to zero-field struct", func(t *testing.T) {
		t.Parallel()
		var b Builder
		emitPageDataStruct(&b, nil)
		got := string(b.Bytes())
		if !strings.Contains(got, "type PageData = struct{}") {
			t.Errorf("expected `type PageData = struct{}`, got:\n%s", got)
		}
	})
}
