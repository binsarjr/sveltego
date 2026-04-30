// Package auth provides the Hasher interface and Argon2id implementation for
// password hashing. The default implementation uses Argon2id (RFC 9106) with
// PHC string format encoding for forward-compatible rehash detection.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Hasher hashes passwords and verifies hashed passwords. Implement this
// interface to swap in bcrypt, scrypt, or any other KDF.
type Hasher interface {
	// Hash derives a storable hash string from a plaintext password.
	Hash(password string) (string, error)

	// Verify returns true when password matches the stored hash. It uses
	// constant-time comparison to prevent timing attacks.
	Verify(password, hash string) (bool, error)

	// Needs returns true when the stored hash was produced with parameters
	// weaker than the current configuration; the caller should re-hash after
	// a successful Verify and persist the new hash.
	Needs(hash string) bool
}

// Argon2id default parameters — OWASP minimum for interactive logins (RFC 9106 §4).
// Target: ≈50 ms on an M1 Pro. Users on slower hardware may lower Memory or Time.
const (
	defaultArgon2Time    uint32 = 3
	defaultArgon2Memory  uint32 = 64 * 1024 // 64 MiB in KiB
	defaultArgon2Threads uint8  = 4
	defaultArgon2KeyLen  uint32 = 32
	defaultArgon2SaltLen uint32 = 16
)

// Argon2idHasher implements Hasher using Argon2id. Construct via NewArgon2idHasher.
type Argon2idHasher struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

// Argon2idOption is a functional option for NewArgon2idHasher.
type Argon2idOption func(*Argon2idHasher)

// WithTime sets the Argon2id time parameter (number of passes).
func WithTime(t uint32) Argon2idOption {
	return func(h *Argon2idHasher) { h.time = t }
}

// WithMemory sets the Argon2id memory parameter in KiB (e.g. 64*1024 for 64 MiB).
func WithMemory(m uint32) Argon2idOption {
	return func(h *Argon2idHasher) { h.memory = m }
}

// WithThreads sets the Argon2id parallelism parameter.
func WithThreads(p uint8) Argon2idOption {
	return func(h *Argon2idHasher) { h.threads = p }
}

// NewArgon2idHasher constructs an Argon2idHasher with the given options.
// Without options, defaults are: time=3, memory=65536 KiB, threads=4,
// keyLen=32, saltLen=16.
func NewArgon2idHasher(opts ...Argon2idOption) *Argon2idHasher {
	h := &Argon2idHasher{
		time:    defaultArgon2Time,
		memory:  defaultArgon2Memory,
		threads: defaultArgon2Threads,
		keyLen:  defaultArgon2KeyLen,
		saltLen: defaultArgon2SaltLen,
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// Hash derives an Argon2id PHC hash from password.
// Format: $argon2id$v=19$m=<mem>,t=<time>,p=<threads>$<saltB64>$<hashB64>
func (h *Argon2idHasher) Hash(password string) (string, error) {
	salt := make([]byte, h.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: argon2id: generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, h.time, h.memory, h.threads, h.keyLen)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.memory,
		h.time,
		h.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return encoded, nil
}

// Verify returns true when password matches the stored PHC hash.
func (h *Argon2idHasher) Verify(password, hash string) (bool, error) {
	params, salt, stored, err := parseArgon2idHash(hash)
	if err != nil {
		return false, err
	}

	candidate := argon2.IDKey([]byte(password), salt, params.time, params.memory, params.threads, params.keyLen)

	if subtle.ConstantTimeCompare(stored, candidate) != 1 {
		return false, nil
	}
	return true, nil
}

// Needs returns true when the stored hash parameters are weaker than the
// current Argon2idHasher configuration, indicating the password should be
// re-hashed after the next successful login.
func (h *Argon2idHasher) Needs(hash string) bool {
	params, _, _, err := parseArgon2idHash(hash)
	if err != nil {
		// Unparseable hash — treat as needing upgrade.
		return true
	}
	return params.time < h.time ||
		params.memory < h.memory ||
		params.threads < h.threads ||
		params.keyLen < h.keyLen
}

// argon2idParams holds the decoded parameters from a PHC hash string.
type argon2idParams struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
}

var errMalformedHash = errors.New("auth: argon2id: malformed PHC hash")

func parseArgon2idHash(encoded string) (p argon2idParams, salt, hash []byte, err error) {
	// Expected: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return p, nil, nil, errMalformedHash
	}
	if parts[1] != "argon2id" {
		return p, nil, nil, fmt.Errorf("auth: argon2id: unexpected algorithm %q", parts[1])
	}

	var version int
	if _, scanErr := fmt.Sscanf(parts[2], "v=%d", &version); scanErr != nil {
		return p, nil, nil, errMalformedHash
	}
	if version != argon2.Version {
		return p, nil, nil, fmt.Errorf("auth: argon2id: unsupported version %d", version)
	}

	_, scanErr := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads)
	if scanErr != nil {
		return p, nil, nil, errMalformedHash
	}

	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return p, nil, nil, fmt.Errorf("auth: argon2id: decode salt: %w", err)
	}

	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return p, nil, nil, fmt.Errorf("auth: argon2id: decode hash: %w", err)
	}
	p.keyLen = uint32(len(hash))

	return p, salt, hash, nil
}
