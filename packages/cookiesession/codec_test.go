package cookiesession

import (
	"strings"
	"testing"
	"time"
)

func makeTestCodec(t *testing.T) Codec {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0xAB
	}
	c, err := NewCodec([]Secret{{ID: 1, Key: key}})
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c
}

func TestSplitChunksSingle(t *testing.T) {
	wire := strings.Repeat("x", 100)
	chunks := splitChunks(wire)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short wire, got %d", len(chunks))
	}
	if chunks[0] != wire {
		t.Fatal("chunk content mismatch")
	}
}

func TestSplitChunksMultiple(t *testing.T) {
	wire := strings.Repeat("a", maxChunkBytes*2+500)
	chunks := splitChunks(wire)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0] != wire[:maxChunkBytes] {
		t.Fatal("chunk[0] mismatch")
	}
	if chunks[1] != wire[maxChunkBytes:maxChunkBytes*2] {
		t.Fatal("chunk[1] mismatch")
	}
	if chunks[2] != wire[maxChunkBytes*2:] {
		t.Fatal("chunk[2] mismatch")
	}
}

func TestJoinChunksRoundTrip(t *testing.T) {
	original := strings.Repeat("z", maxChunkBytes*3+17)
	chunks := splitChunks(original)
	joined := joinChunks(chunks)
	if joined != original {
		t.Fatalf("join(split(x)) != x: got %d bytes, want %d", len(joined), len(original))
	}
}

func TestChunkName(t *testing.T) {
	if got := chunkName("sess", 0); got != "sess.0" {
		t.Fatalf("chunkName(sess,0) = %q, want sess.0", got)
	}
	if got := chunkName("sess", 7); got != "sess.7" {
		t.Fatalf("chunkName(sess,7) = %q, want sess.7", got)
	}
}

func TestMetaName(t *testing.T) {
	if got := metaName("sess"); got != "sess.meta" {
		t.Fatalf("metaName = %q, want sess.meta", got)
	}
}

func TestEncodeDecodeChunkMeta(t *testing.T) {
	for _, n := range []int{1, 2, 10, 100} {
		encoded := encodeChunkMeta(n)
		decoded, err := decodeChunkMeta(encoded)
		if err != nil {
			t.Fatalf("decodeChunkMeta(%q): %v", encoded, err)
		}
		if decoded != n {
			t.Fatalf("round-trip chunk count: got %d, want %d", decoded, n)
		}
	}
}

func TestDecodeChunkMetaInvalid(t *testing.T) {
	for _, bad := range []string{"0", "-1", "abc", "", "1.5"} {
		_, err := decodeChunkMeta(bad)
		if err == nil {
			t.Errorf("decodeChunkMeta(%q): expected error, got nil", bad)
		}
	}
}

func TestEncodeDecodePayloadRoundTrip(t *testing.T) {
	c := makeTestCodec(t)
	type item struct {
		Count int
		Name  string
	}
	original := payload[item]{
		Data:    item{Count: 42, Name: "hello"},
		Expires: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	wire, err := encodePayload(c, original)
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	got, err := decodePayload[item](c, wire)
	if err != nil {
		t.Fatalf("decodePayload: %v", err)
	}
	if got.Data != original.Data {
		t.Fatalf("data mismatch: got %+v, want %+v", got.Data, original.Data)
	}
	if !got.Expires.Equal(original.Expires) {
		t.Fatalf("expires mismatch: got %v, want %v", got.Expires, original.Expires)
	}
}

func TestDecodePayloadTampered(t *testing.T) {
	c := makeTestCodec(t)
	type item struct{ Count int }
	wire, err := encodePayload(c, payload[item]{Data: item{Count: 1}})
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	// Flip a hex char in the middle of the wire string (before &id= suffix).
	runes := []rune(wire)
	mid := len(runes) / 3
	if runes[mid] == 'f' {
		runes[mid] = '0'
	} else {
		runes[mid] = 'f'
	}
	tampered := string(runes)
	_, err = decodePayload[item](c, tampered)
	if err == nil {
		t.Fatal("expected error on tampered wire, got nil")
	}
}
