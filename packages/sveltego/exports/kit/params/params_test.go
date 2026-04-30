package params

import "testing"

func TestInt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0", true},
		{"42", true},
		{"-1", true},
		{"+1", true},
		{"007", true},
		{"abc", false},
		{"12abc", false},
		{"abc12", false},
		{"3.14", false},
		{" 5", false},
		{"5 ", false},
		{"0x10", false},
		{"9999999999", true},
	}
	for _, tc := range cases {
		if got := Int.Match(tc.in); got != tc.want {
			t.Errorf("Int.Match(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestUUID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"00000000-0000-0000-0000-000000000000", true},
		{"abcdef01-2345-6789-abcd-ef0123456789", true},
		{"ABCDEF01-2345-6789-ABCD-EF0123456789", true},
		{"AbCdEf01-2345-6789-aBcD-EF0123456789", true},
		{"00000000-0000-0000-0000-00000000000", false},   // too short
		{"00000000-0000-0000-0000-0000000000000", false}, // too long
		{"00000000_0000_0000_0000_000000000000", false},  // wrong delim
		{"0000000-00000-0000-0000-000000000000", false},  // hyphen position off
		{"gggggggg-0000-0000-0000-000000000000", false},  // non-hex
		{"00000000-0000-0000-0000-00000000000g", false},
		{"not-a-uuid", false},
	}
	for _, tc := range cases {
		if got := UUID.Match(tc.in); got != tc.want {
			t.Errorf("UUID.Match(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSlug(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"a", true},
		{"hello", true},
		{"hello-world", true},
		{"my-slug-123", true},
		{"123", true},
		{"a-b-c-d", true},
		{"-leading", false},
		{"trailing-", false},
		{"double--hyphen", false},
		{"Hello", false}, // uppercase
		{"hello_world", false},
		{"hello world", false},
		{"hello!", false},
		{"-", false},
	}
	for _, tc := range cases {
		if got := Slug.Match(tc.in); got != tc.want {
			t.Errorf("Slug.Match(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDefaultMatchers(t *testing.T) {
	t.Parallel()
	m := DefaultMatchers()
	if len(m) != 3 {
		t.Fatalf("want 3 entries, got %d", len(m))
	}
	for _, name := range []string{"int", "uuid", "slug"} {
		if _, ok := m[name]; !ok {
			t.Errorf("missing %q", name)
		}
	}
	// caller-owned: mutation must not bleed into subsequent calls.
	delete(m, "int")
	fresh := DefaultMatchers()
	if _, ok := fresh["int"]; !ok {
		t.Errorf("DefaultMatchers shares state across calls")
	}
}

func BenchmarkInt(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Int.Match("12345")
	}
}

func BenchmarkSlug(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Slug.Match("hello-world-123")
	}
}
