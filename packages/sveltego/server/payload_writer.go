package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// payloadBufPool reuses bytes.Buffer instances across the splice writer
// path. Pre-encoded stable fields (Manifest, AppVersion, VersionPoll,
// RouteID) are appended verbatim from Server-owned slices, so the
// per-request scratch buffer only sees varying fields. See #488.
var payloadBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func acquirePayloadBuf() *bytes.Buffer {
	return payloadBufPool.Get().(*bytes.Buffer)
}

func releasePayloadBuf(b *bytes.Buffer) {
	b.Reset()
	payloadBufPool.Put(b)
}

// encodePayloadField returns `,"<name>":<json.Marshal(v)>` so the slice
// can be appended after any preceding field. Panics on encode failure
// because the value is build-time constant; a Marshal error here is a
// programmer bug, not a runtime input.
func encodePayloadField(name string, v any) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("server: pre-encode payload field %q: %w", name, err))
	}
	out := make([]byte, 0, len(name)+len(raw)+4)
	out = append(out, ',', '"')
	out = append(out, name...)
	out = append(out, '"', ':')
	out = append(out, raw...)
	return out
}

// encodeAppVersionField returns the pre-encoded `,"appVersion":"<hash>"`
// slice or nil when version is empty (matches the json:"appVersion,omitempty"
// behavior). Strings are escaped via encoding/json so any future hash
// format is handled safely.
func encodeAppVersionField(version string) []byte {
	if version == "" {
		return nil
	}
	return encodePayloadField("appVersion", version)
}

// encodeVersionPollField returns the pre-encoded `,"versionPoll":{...}`
// slice or nil when no AppVersion is known (matches the per-request
// guard in applyInitialPayloadFields). Mirrors clientVersionPoll on the
// wire so the splice and per-request paths stay byte-identical.
func encodeVersionPollField(appVersion string, vp kit.VersionPollConfig) []byte {
	if appVersion == "" {
		return nil
	}
	cv := clientVersionPoll{
		IntervalMS: vp.PollInterval.Milliseconds(),
		Disabled:   vp.Disabled,
	}
	return encodePayloadField("versionPoll", cv)
}

// encodeRouteIDs precomputes the JSON-encoded route pattern strings used
// as the `routeId` field on the hot path. The map is read-only after
// construction so callers can race-load freely.
func encodeRouteIDs(routes []router.Route) map[string][]byte {
	out := make(map[string][]byte, len(routes))
	for i := range routes {
		pattern := routes[i].Pattern
		if _, ok := out[pattern]; ok {
			continue
		}
		raw, err := json.Marshal(pattern)
		if err != nil {
			panic(fmt.Errorf("server: pre-encode routeId %q: %w", pattern, err))
		}
		out[pattern] = raw
	}
	return out
}

// writePayloadJSON renders payload p as the canonical clientPayload JSON
// document into dst. Stable fields (RouteID, Manifest, AppVersion,
// VersionPoll) are spliced from pre-encoded slices owned by s; varying
// fields (Data, LayoutData, URL, Params, Status, Form, PageError, Deps)
// are marshaled per-request. The output is byte-identical to
// `json.Marshal(p)` so existing wire-format consumers are untouched.
//
// The presence of p.Manifest / p.AppVersion / p.VersionPoll mirrors
// json.Marshal's `omitempty` behavior: when the caller has stamped
// initial-render fields via applyInitialPayloadFields the encoded
// stable bytes are spliced in; otherwise (e.g. __data.json path) the
// fields are skipped entirely.
func (s *Server) writePayloadJSON(dst *bytes.Buffer, p clientPayload) error {
	dst.WriteByte('{')

	encodedRouteID := s.encodedRouteIDs[p.RouteID]
	dst.WriteString(`"routeId":`)
	if len(encodedRouteID) > 0 {
		dst.Write(encodedRouteID)
	} else {
		raw, err := json.Marshal(p.RouteID)
		if err != nil {
			return fmt.Errorf("marshal routeId: %w", err)
		}
		dst.Write(raw)
	}

	dst.WriteString(`,"data":`)
	if err := encodeJSONValue(dst, p.Data); err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	if len(p.LayoutData) > 0 {
		dst.WriteString(`,"layoutData":`)
		if err := encodeJSONValue(dst, p.LayoutData); err != nil {
			return fmt.Errorf("marshal layoutData: %w", err)
		}
	}

	dst.WriteString(`,"form":`)
	if err := encodeJSONValue(dst, p.Form); err != nil {
		return fmt.Errorf("marshal form: %w", err)
	}

	dst.WriteString(`,"url":`)
	if err := encodeJSONString(dst, p.URL); err != nil {
		return fmt.Errorf("marshal url: %w", err)
	}

	dst.WriteString(`,"params":`)
	if err := encodeStringMap(dst, p.Params); err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	dst.WriteString(`,"status":`)
	var statusBuf [20]byte
	dst.Write(strconv.AppendInt(statusBuf[:0], int64(p.Status), 10))

	dst.WriteString(`,"error":`)
	if p.PageError == nil {
		dst.WriteString(`null`)
	} else if err := encodeJSONValue(dst, p.PageError); err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	if len(p.Manifest) > 0 {
		if len(s.encodedManifest) > 0 {
			dst.Write(s.encodedManifest)
		} else {
			dst.WriteString(`,"manifest":`)
			if err := encodeJSONValue(dst, p.Manifest); err != nil {
				return fmt.Errorf("marshal manifest: %w", err)
			}
		}
	}

	if len(p.Deps) > 0 {
		dst.WriteString(`,"deps":`)
		if err := encodeJSONValue(dst, p.Deps); err != nil {
			return fmt.Errorf("marshal deps: %w", err)
		}
	}

	if p.AppVersion != "" {
		if len(s.encodedAppVersion) > 0 {
			dst.Write(s.encodedAppVersion)
		} else {
			dst.WriteString(`,"appVersion":`)
			if err := encodeJSONString(dst, p.AppVersion); err != nil {
				return fmt.Errorf("marshal appVersion: %w", err)
			}
		}
	}

	if p.VersionPoll != nil {
		if len(s.encodedVersionPoll) > 0 {
			dst.Write(s.encodedVersionPoll)
		} else {
			dst.WriteString(`,"versionPoll":`)
			if err := encodeJSONValue(dst, p.VersionPoll); err != nil {
				return fmt.Errorf("marshal versionPoll: %w", err)
			}
		}
	}

	dst.WriteByte('}')
	return nil
}

// encodeJSONValue writes v as JSON into dst, mirroring encoding/json's
// nil handling: a nil interface emits "null", a nil typed slice/map
// emits "null" too (matching json.Marshal default behavior).
func encodeJSONValue(dst *bytes.Buffer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dst.Write(raw)
	return nil
}

// encodeJSONString writes s as a JSON string literal directly into dst,
// avoiding the json.Marshal allocation that returns a fresh []byte.
// Falls back to json.Marshal when the string contains any byte that
// requires escaping beyond simple printable ASCII so encoding rules stay
// identical to the standard library (HTML-safe escapes, surrogate pair
// handling, etc.).
func encodeJSONString(dst *bytes.Buffer, s string) error {
	if !needsJSONStringEscape(s) {
		dst.Grow(len(s) + 2)
		dst.WriteByte('"')
		dst.WriteString(s)
		dst.WriteByte('"')
		return nil
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	dst.Write(raw)
	return nil
}

// needsJSONStringEscape returns true when s contains any byte that the
// standard library encoder would escape in a JSON string literal: ASCII
// control chars, quote, backslash, the HTML-safe trio (<, >, &), or any
// byte ≥ 0x7f (multibyte UTF-8 lead bytes are not escaped per se but
// json.Marshal validates them and the fast-path here only handles pure
// ASCII to stay obviously correct).
func needsJSONStringEscape(s string) bool {
	for i := range len(s) {
		c := s[i]
		if c < 0x20 || c >= 0x7f || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			return true
		}
	}
	return false
}

// encodeStringMap writes a map[string]string as a JSON object directly
// into dst, avoiding both the reflection-driven map walk in
// encoding/json and the intermediate []byte allocation. Keys are
// emitted in sorted order to match json.Marshal's deterministic output.
// Falls back to json.Marshal when any key or value contains characters
// that require escaping beyond plain ASCII.
func encodeStringMap(dst *bytes.Buffer, m map[string]string) error {
	if m == nil {
		dst.WriteString(`null`)
		return nil
	}
	if len(m) == 0 {
		dst.WriteString(`{}`)
		return nil
	}
	for k, v := range m {
		if needsJSONStringEscape(k) || needsJSONStringEscape(v) {
			return encodeJSONValue(dst, m)
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	dst.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			dst.WriteByte(',')
		}
		dst.WriteByte('"')
		dst.WriteString(k)
		dst.WriteString(`":"`)
		dst.WriteString(m[k])
		dst.WriteByte('"')
	}
	dst.WriteByte('}')
	return nil
}
