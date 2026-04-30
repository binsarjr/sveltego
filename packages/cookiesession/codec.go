package cookiesession

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// maxChunkBytes is the maximum encoded payload length stored in a single
// cookie. RFC 6265 allows 4096 bytes per cookie (name + value + attributes);
// we leave a 96-byte buffer for the cookie header envelope.
const maxChunkBytes = 4000

// payload is the JSON structure stored inside each encrypted session cookie.
type payload[T any] struct {
	Data    T         `json:"data"`
	Expires time.Time `json:"expires"`
}

// encodePayload JSON-marshals p and encrypts it with c.
// Returns the encrypted wire string (hex with &id= suffix from Codec).
func encodePayload[T any](c Codec, p payload[T]) (string, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("cookiesession: marshal: %w", err)
	}
	wire, err := c.Encrypt(raw)
	if err != nil {
		return "", err
	}
	return wire, nil
}

// decodePayload decrypts wire and JSON-unmarshals into a payload[T].
func decodePayload[T any](c Codec, wire string) (payload[T], error) {
	raw, err := c.Decrypt(wire)
	if err != nil {
		return payload[T]{}, err
	}
	var p payload[T]
	if err := json.Unmarshal(raw, &p); err != nil {
		return payload[T]{}, fmt.Errorf("cookiesession: unmarshal: %w", err)
	}
	return p, nil
}

// splitChunks splits a wire string into chunks of at most maxChunkBytes each.
// Returns a slice of chunk strings. If the wire fits in one chunk, returns a
// single-element slice.
func splitChunks(wire string) []string {
	if len(wire) <= maxChunkBytes {
		return []string{wire}
	}
	var chunks []string
	for len(wire) > 0 {
		n := maxChunkBytes
		if n > len(wire) {
			n = len(wire)
		}
		chunks = append(chunks, wire[:n])
		wire = wire[n:]
	}
	return chunks
}

// joinChunks concatenates ordered chunk values into a single wire string.
func joinChunks(chunks []string) string {
	return strings.Join(chunks, "")
}

// chunkName returns the cookie name for the i-th chunk (0-based).
func chunkName(base string, i int) string {
	return base + "." + strconv.Itoa(i)
}

// metaName returns the cookie name for the chunk-count metadata cookie.
func metaName(base string) string {
	return base + ".meta"
}

// encodeChunkMeta serialises chunkCount into the meta cookie value.
func encodeChunkMeta(chunkCount int) string {
	return strconv.Itoa(chunkCount)
}

// decodeChunkMeta parses the chunk count from a meta cookie value.
func decodeChunkMeta(meta string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(meta))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("cookiesession: invalid chunk meta %q", meta)
	}
	return n, nil
}
