package server

import "testing"

func TestWriteRaw_String(t *testing.T) {
	t.Parallel()
	var p Payload
	WriteRaw(&p, "<b>bold</b>")
	if got := p.Body(); got != "<b>bold</b>" {
		t.Fatalf("WriteRaw(string) = %q, want %q", got, "<b>bold</b>")
	}
}

func TestWriteRaw_Bytes(t *testing.T) {
	t.Parallel()
	var p Payload
	WriteRaw(&p, []byte("<i>x</i>"))
	if got := p.Body(); got != "<i>x</i>" {
		t.Fatalf("WriteRaw([]byte) = %q", got)
	}
}

func TestWriteRaw_Nil(t *testing.T) {
	t.Parallel()
	var p Payload
	WriteRaw(&p, nil)
	if got := p.Body(); got != "" {
		t.Fatalf("WriteRaw(nil) wrote %q", got)
	}
}

func TestWriteRaw_AnyFallback(t *testing.T) {
	t.Parallel()
	var p Payload
	WriteRaw(&p, 42)
	if got := p.Body(); got != "42" {
		t.Fatalf("WriteRaw(int) = %q, want %q", got, "42")
	}
}

func TestTruthy_Falsy(t *testing.T) {
	t.Parallel()
	cases := []any{
		nil,
		false,
		0,
		int8(0), int16(0), int32(0), int64(0),
		uint(0), uint8(0), uint16(0), uint32(0), uint64(0),
		float32(0), float64(0),
		"",
		[]byte{},
		[]any{},
		map[string]any{},
	}
	for _, c := range cases {
		if Truthy(c) {
			t.Errorf("Truthy(%#v) = true, want false", c)
		}
	}
}

func TestTruthy_Truthy(t *testing.T) {
	t.Parallel()
	cases := []any{
		true,
		1, int64(-1),
		uint(1),
		1.5,
		"a",
		[]byte("x"),
		[]any{1},
		map[string]any{"k": 1},
		struct{}{},
	}
	for _, c := range cases {
		if !Truthy(c) {
			t.Errorf("Truthy(%#v) = false, want true", c)
		}
	}
}
