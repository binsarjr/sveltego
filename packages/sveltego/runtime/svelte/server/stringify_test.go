package server

import "testing"

type stringerImpl struct{ s string }

func (s stringerImpl) String() string { return s.s }

func TestStringify(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"empty-string", "", ""},
		{"string", "hello", "hello"},
		{"int-zero", 0, "0"},
		{"int", 42, "42"},
		{"int-negative", -7, "-7"},
		{"int64-large", int64(1_000_000_000_000), "1000000000000"},
		{"uint", uint(99), "99"},
		{"float-integral", 3.0, "3"},
		{"float-fractional", 3.14, "3.14"},
		{"float-negative", -2.5, "-2.5"},
		{"bool-true", true, "true"},
		{"bool-false", false, "false"},
		{"bytes", []byte("hi"), "hi"},
		{"stringer", stringerImpl{"sg"}, "sg"},
		{"unknown-struct", struct{ X int }{1}, "[object Object]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Stringify(tc.in); got != tc.want {
				t.Errorf("Stringify(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
