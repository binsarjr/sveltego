package typegen

import (
	"path/filepath"
	"testing"
)

func TestBuildShape_Nested(t *testing.T) {
	t.Parallel()
	shape, _, err := BuildShape(filepath.Join("testdata", "nested", "_page.server.go"), KindPage)
	if err != nil {
		t.Fatalf("BuildShape: %v", err)
	}
	if shape.RootType != "PageData" {
		t.Errorf("RootType = %q, want PageData", shape.RootType)
	}
	root, ok := shape.Types["PageData"]
	if !ok {
		t.Fatalf("PageData missing from shape: %#v", shape)
	}
	user, ok := root.Lookup("user")
	if !ok {
		t.Fatalf("PageData.user missing: %#v", root)
	}
	if user.GoName != "User" {
		t.Errorf("user.GoName = %q, want User", user.GoName)
	}
	if user.NamedType != "User" {
		t.Errorf("user.NamedType = %q, want User", user.NamedType)
	}
	posts, ok := root.Lookup("posts")
	if !ok {
		t.Fatalf("PageData.posts missing")
	}
	if !posts.Slice {
		t.Errorf("posts.Slice = false, want true")
	}
	userType, ok := shape.Types["User"]
	if !ok {
		t.Fatalf("User type missing from shape")
	}
	name, ok := userType.Lookup("name")
	if !ok {
		t.Fatalf("User.name missing")
	}
	if name.GoName != "Name" {
		t.Errorf("user.name.GoName = %q, want Name", name.GoName)
	}
}

func TestBuildShape_JSONTagToGoName(t *testing.T) {
	t.Parallel()
	// jsontag fixture: `Renamed string \`json:"display_name"\`` —
	// JSON tag display_name maps to Go field Renamed.
	shape, _, err := BuildShape(filepath.Join("testdata", "jsontag", "_page.server.go"), KindPage)
	if err != nil {
		t.Fatalf("BuildShape: %v", err)
	}
	root := shape.Types["PageData"]
	renamed, ok := root.Lookup("display_name")
	if !ok {
		t.Fatalf("display_name JSON tag missing")
	}
	if renamed.GoName != "Renamed" {
		t.Errorf("display_name.GoName = %q, want Renamed", renamed.GoName)
	}
}

func TestBuildShape_PointerField(t *testing.T) {
	t.Parallel()
	shape, _, err := BuildShape(filepath.Join("testdata", "pointers", "_page.server.go"), KindPage)
	if err != nil {
		t.Fatalf("BuildShape: %v", err)
	}
	root := shape.Types["PageData"]
	for _, f := range root.Fields {
		if !f.Pointer {
			continue
		}
		if f.GoType == "" {
			t.Errorf("pointer field %q missing GoType", f.GoName)
		}
	}
}
