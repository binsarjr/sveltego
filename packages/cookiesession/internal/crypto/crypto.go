// Package crypto implements AES-256-GCM authenticated encryption for
// cookiesession wire values.
//
// Wire format: hex(nonce || ciphertext || tag)
// The nonce is 12 bytes (GCM standard), the tag is 16 bytes (GCM default).
// Cookie-level ID suffix ("&id=N") is handled by the caller via JoinID/SplitID.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	keySize   = 32 // AES-256
	nonceSize = 12 // GCM standard
)

var (
	// ErrKeyLength is returned when a key is not exactly 32 bytes.
	ErrKeyLength = errors.New("cookiesession: secret key must be exactly 32 bytes (use sveltego gen-secret)")

	// ErrNoSecrets is returned when Secrets is empty.
	ErrNoSecrets = errors.New("cookiesession: at least one secret is required")

	// ErrDecryptFailed is returned when GCM tag validation fails (tampered or wrong key).
	ErrDecryptFailed = errors.New("cookiesession: decryption failed (tampered ciphertext or wrong key)")

	// ErrSecretNotFound is returned when no secret with the requested ID exists.
	ErrSecretNotFound = errors.New("cookiesession: no secret found with the given id")
)

// Secret pairs a rotation ID with a 32-byte AES-256 key.
type Secret struct {
	ID  uint32
	Key []byte // exactly 32 bytes
}

// Secrets is a newest-first ordered list of secrets.
// Secrets[0] is always used for encryption; all entries are tried for decryption.
type Secrets []Secret

// Validate checks that the list is non-empty and every key is exactly 32 bytes.
func (s Secrets) Validate() error {
	if len(s) == 0 {
		return ErrNoSecrets
	}
	for i, sec := range s {
		if len(sec.Key) != keySize {
			return fmt.Errorf("%w (secret index %d, id %d, got %d bytes)", ErrKeyLength, i, sec.ID, len(sec.Key))
		}
	}
	return nil
}

// Encode encrypts plaintext with the head secret (Secrets[0]).
// Returns the hex-encoded ciphertext and the secret ID used.
func (s Secrets) Encode(plaintext []byte) (encoded string, id uint32, err error) {
	if err = s.Validate(); err != nil {
		return "", 0, err
	}
	head := s[0]
	encoded, err = Encrypt(head.Key, plaintext)
	if err != nil {
		return "", 0, err
	}
	return encoded, head.ID, nil
}

// Decode decrypts encoded using the secret identified by id.
// headWasUsed reports whether Secrets[0] performed the decryption.
// Falls back to head (Secrets[0]) if no secret with that ID is found and id
// matches Secrets[0].ID — callers should always supply the id from JoinID/SplitID.
func (s Secrets) Decode(encoded string, id uint32) (plaintext []byte, headWasUsed bool, err error) {
	if err = s.Validate(); err != nil {
		return nil, false, err
	}
	// Find the secret with the matching ID.
	for i, sec := range s {
		if sec.ID == id {
			plain, e := Decrypt(sec.Key, encoded)
			if e != nil {
				return nil, false, e
			}
			return plain, i == 0, nil
		}
	}
	return nil, false, fmt.Errorf("%w (id %d)", ErrSecretNotFound, id)
}

// Encrypt encrypts plaintext with the given 32-byte key using AES-256-GCM.
// It generates a fresh random 12-byte nonce on every call.
// Returns hex(nonce || ciphertext || tag).
func Encrypt(key, plaintext []byte) (string, error) {
	if len(key) != keySize {
		return "", fmt.Errorf("%w (got %d bytes)", ErrKeyLength, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cookiesession: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("cookiesession: cipher.NewGCM: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("cookiesession: rand nonce: %w", err)
	}

	// Seal appends ciphertext+tag to nonce so the layout is nonce||ct||tag.
	dst := gcm.Seal(nonce, nonce, plaintext, nil)
	return hex.EncodeToString(dst), nil
}

// Decrypt decrypts a hex-encoded value produced by Encrypt.
// Returns ErrDecryptFailed if the GCM tag is invalid (tampered or wrong key).
func Decrypt(key []byte, encoded string) ([]byte, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("%w (got %d bytes)", ErrKeyLength, len(key))
	}

	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("cookiesession: hex decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cookiesession: aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cookiesession: cipher.NewGCM: %w", err)
	}

	minLen := nonceSize + gcm.Overhead()
	if len(raw) < minLen {
		return nil, ErrDecryptFailed
	}

	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrDecryptFailed
	}
	return plain, nil
}

// JoinID appends the secret ID suffix to an encoded cookie value.
// Format: "<hex>&id=<decimal>".
func JoinID(encoded string, id uint32) string {
	return encoded + "&id=" + strconv.FormatUint(uint64(id), 10)
}

// SplitID separates a cookie value into the hex-encoded ciphertext and the
// secret ID. Returns the raw encoded string and id=0 if no suffix is present
// (for single-key deployments).
func SplitID(cookie string) (encoded string, id uint32, err error) {
	const prefix = "&id="
	idx := strings.LastIndex(cookie, prefix)
	if idx < 0 {
		// No ID suffix — caller interprets as head ID 0 or handles accordingly.
		return cookie, 0, nil
	}
	encoded = cookie[:idx]
	n, e := strconv.ParseUint(cookie[idx+len(prefix):], 10, 32)
	if e != nil {
		return "", 0, fmt.Errorf("cookiesession: invalid id suffix: %w", e)
	}
	return encoded, uint32(n), nil
}
