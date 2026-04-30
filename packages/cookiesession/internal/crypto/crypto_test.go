package crypto_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/cookiesession/internal/crypto"
)

func makeKey(b byte) []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = b
	}
	return key
}

// TestRoundTrip verifies 1 KiB plaintext survives encrypt→decrypt unchanged.
func TestRoundTrip(t *testing.T) {
	key := makeKey(0xAB)
	plaintext := bytes.Repeat([]byte("Hello sveltego! "), 64) // 1024 bytes

	encoded, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := crypto.Decrypt(key, encoded)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(plaintext))
	}
}

// TestEmptyPlaintext verifies empty byte slices are accepted.
func TestEmptyPlaintext(t *testing.T) {
	key := makeKey(0x01)

	encoded, err := crypto.Encrypt(key, []byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}
	got, err := crypto.Decrypt(key, encoded)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes", len(got))
	}
}

// TestLargePlaintext verifies a 128 KiB payload.
func TestLargePlaintext(t *testing.T) {
	key := makeKey(0xCC)
	plaintext := bytes.Repeat([]byte{0xFF}, 128*1024)

	encoded, err := crypto.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt large: %v", err)
	}
	got, err := crypto.Decrypt(key, encoded)
	if err != nil {
		t.Fatalf("Decrypt large: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("large round-trip mismatch")
	}
}

// TestTamperDetection verifies that any single-byte mutation causes decrypt to fail.
func TestTamperDetection(t *testing.T) {
	key := makeKey(0x77)
	encoded, err := crypto.Encrypt(key, []byte("secret payload"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Flip the last hex char.
	runes := []rune(encoded)
	last := runes[len(runes)-1]
	if last == 'f' {
		runes[len(runes)-1] = '0'
	} else {
		runes[len(runes)-1] = 'f'
	}
	tampered := string(runes)

	if _, err = crypto.Decrypt(key, tampered); err == nil {
		t.Fatal("expected error on tampered ciphertext, got nil")
	}
}

// TestNonceUniqueness verifies 1000 successive Encrypt calls yield distinct outputs.
func TestNonceUniqueness(t *testing.T) {
	key := makeKey(0x55)
	plaintext := []byte("same plaintext every time")

	seen := make(map[string]struct{}, 1000)
	for i := range 1000 {
		enc, err := crypto.Encrypt(key, plaintext)
		if err != nil {
			t.Fatalf("Encrypt[%d]: %v", i, err)
		}
		if _, dup := seen[enc]; dup {
			t.Fatalf("nonce collision at iteration %d", i)
		}
		seen[enc] = struct{}{}
	}
}

// TestKeyLengthValidation verifies that a 31-byte key is rejected with a message
// mentioning "32 bytes".
func TestKeyLengthValidation(t *testing.T) {
	shortKey := make([]byte, 31)

	_, err := crypto.Encrypt(shortKey, []byte("x"))
	if err == nil {
		t.Fatal("Encrypt with 31-byte key should fail")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("error should mention '32 bytes', got: %v", err)
	}

	_, err = crypto.Decrypt(shortKey, "deadbeef")
	if err == nil {
		t.Fatal("Decrypt with 31-byte key should fail")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Fatalf("error should mention '32 bytes', got: %v", err)
	}
}

// TestSecretsValidation verifies empty slice and bad key lengths are rejected.
func TestSecretsValidation(t *testing.T) {
	// Empty slice.
	if err := crypto.Secrets(nil).Validate(); err == nil {
		t.Fatal("empty Secrets should fail Validate")
	}

	// Key too short.
	bad := crypto.Secrets{{ID: 1, Key: make([]byte, 16)}}
	if err := bad.Validate(); err == nil {
		t.Fatal("16-byte key should fail Validate")
	}
	if !strings.Contains(bad.Validate().Error(), "32 bytes") {
		t.Fatalf("validate error should mention '32 bytes'")
	}
}

// TestSecretsEncodeDecode verifies round-trip via Secrets helpers.
func TestSecretsEncodeDecode(t *testing.T) {
	secrets := crypto.Secrets{
		{ID: 2, Key: makeKey(0x22)},
		{ID: 1, Key: makeKey(0x11)},
	}
	plaintext := []byte("cookiesession payload")

	encoded, id, err := secrets.Encode(plaintext)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if id != 2 {
		t.Fatalf("expected id 2 (head), got %d", id)
	}

	got, headWasUsed, err := secrets.Decode(encoded, id)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !headWasUsed {
		t.Fatal("head should have been used when id matches head")
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("round-trip via Secrets mismatch")
	}
}

// TestRotationAcceptsOldKey verifies that an old (non-head) secret can still decrypt.
func TestRotationAcceptsOldKey(t *testing.T) {
	oldSecret := crypto.Secret{ID: 1, Key: makeKey(0x11)}
	newSecret := crypto.Secret{ID: 2, Key: makeKey(0x22)}

	// Encrypt with old key directly.
	encoded, err := crypto.Encrypt(oldSecret.Key, []byte("old session data"))
	if err != nil {
		t.Fatalf("Encrypt with old key: %v", err)
	}

	// Build a Secrets with the new key as head and old as fallback.
	secrets := crypto.Secrets{newSecret, oldSecret}

	got, headWasUsed, err := secrets.Decode(encoded, oldSecret.ID)
	if err != nil {
		t.Fatalf("Decode with old id: %v", err)
	}
	if headWasUsed {
		t.Fatal("headWasUsed should be false when using old secret")
	}
	if !bytes.Equal(got, []byte("old session data")) {
		t.Fatal("rotation decode mismatch")
	}
}

// TestRotationRejectsUnknownKey verifies an unknown id returns ErrSecretNotFound.
func TestRotationRejectsUnknownKey(t *testing.T) {
	secrets := crypto.Secrets{
		{ID: 1, Key: makeKey(0x11)},
	}
	encoded, _, err := secrets.Encode([]byte("data"))
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	_, _, err = secrets.Decode(encoded, 99) // unknown id
	if err == nil {
		t.Fatal("expected error for unknown id, got nil")
	}
}

// TestJoinSplitID verifies the wire suffix helper round-trips correctly.
func TestJoinSplitID(t *testing.T) {
	encoded := "deadbeef01020304"
	cookie := crypto.JoinID(encoded, 42)

	gotEncoded, gotID, err := crypto.SplitID(cookie)
	if err != nil {
		t.Fatalf("SplitID: %v", err)
	}
	if gotEncoded != encoded {
		t.Fatalf("SplitID encoded: got %q, want %q", gotEncoded, encoded)
	}
	if gotID != 42 {
		t.Fatalf("SplitID id: got %d, want 42", gotID)
	}
}

// TestSplitIDNoSuffix verifies SplitID is tolerant of cookies without the &id= suffix.
func TestSplitIDNoSuffix(t *testing.T) {
	enc, id, err := crypto.SplitID("deadbeef")
	if err != nil {
		t.Fatalf("SplitID no suffix: %v", err)
	}
	if enc != "deadbeef" || id != 0 {
		t.Fatalf("SplitID no suffix: enc=%q id=%d", enc, id)
	}
}

// FuzzDecrypt confirms Decrypt never panics on arbitrary input.
func FuzzDecrypt(f *testing.F) {
	f.Add([]byte("notvalidhex"))
	f.Add([]byte("deadbeef"))
	f.Add([]byte(""))
	key := makeKey(0xAA)
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = crypto.Decrypt(key, string(data))
	})
}
