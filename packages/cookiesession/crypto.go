// Package cookiesession provides encrypted, stateless, type-safe sessions
// stored in browser cookies using AES-256-GCM authenticated encryption.
//
// This file exposes the public Codec API. Lower-level AES-256-GCM primitives
// live in internal/crypto and are accessible to sibling packages within this
// module (Session, Handle) but not to external callers.
package cookiesession

import (
	"fmt"

	"github.com/binsarjr/sveltego/cookiesession/internal/crypto"
)

// Codec encrypts and decrypts cookie session payloads.
// Encrypt always uses the newest (head) secret; Decrypt tries all secrets.
type Codec interface {
	Encrypt(plaintext []byte) (cookie string, err error)
	Decrypt(cookie string) (plaintext []byte, err error)
}

// codec is the concrete implementation backed by a Secrets list.
type codec struct {
	secrets crypto.Secrets
}

// NewCodec returns a Codec backed by the provided secrets.
// secrets must be newest-first; the first entry is used for encryption.
// Returns an error if secrets fails validation (empty or any key != 32 bytes).
func NewCodec(secrets []Secret) (Codec, error) {
	cs := make(crypto.Secrets, len(secrets))
	for i, s := range secrets {
		cs[i] = crypto.Secret{ID: s.ID, Key: s.Key}
	}
	if err := cs.Validate(); err != nil {
		return nil, err
	}
	return &codec{secrets: cs}, nil
}

// Encrypt encrypts plaintext and returns a cookie-safe hex string with an
// &id=N suffix identifying which secret was used.
func (c *codec) Encrypt(plaintext []byte) (string, error) {
	encoded, id, err := c.secrets.Encode(plaintext)
	if err != nil {
		return "", fmt.Errorf("cookiesession: encrypt: %w", err)
	}
	return crypto.JoinID(encoded, id), nil
}

// Decrypt decodes a cookie value produced by Encrypt.
// The &id=N suffix is used to select the correct decryption key.
// Returns ErrDecryptFailed if the ciphertext has been tampered with.
func (c *codec) Decrypt(cookie string) ([]byte, error) {
	encoded, id, err := crypto.SplitID(cookie)
	if err != nil {
		return nil, fmt.Errorf("cookiesession: decrypt split: %w", err)
	}
	plain, _, err := c.secrets.Decode(encoded, id)
	if err != nil {
		return nil, fmt.Errorf("cookiesession: decrypt: %w", err)
	}
	return plain, nil
}

// Secret pairs a rotation ID with a 32-byte AES-256 key.
// The Key must be exactly 32 bytes; use `sveltego gen-secret` to generate one.
type Secret struct {
	ID  uint32
	Key []byte
}
