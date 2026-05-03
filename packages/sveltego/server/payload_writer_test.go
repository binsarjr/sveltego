package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
	"github.com/binsarjr/sveltego/packages/sveltego/render"
	"github.com/binsarjr/sveltego/packages/sveltego/runtime/router"
)

// TestWritePayloadByteIdenticalToJSONMarshal asserts the splice writer's
// output is byte-for-byte equal to the legacy json.Marshal(clientPayload)
// output across the representative payload shapes the runtime emits.
//
// The splice writer pre-encodes Manifest/AppVersion/VersionPoll/RouteID
// at Server.New() time and stitches them into the per-request bytes; any
// drift in field order, omitempty handling, or escape rules surfaces here
// before bench numbers are read. See #488.
func TestWritePayloadByteIdenticalToJSONMarshal(t *testing.T) {
	t.Parallel()

	type loadShape struct {
		Title string `json:"title"`
		Count int    `json:"count"`
	}

	srv := newTestServer(t, []router.Route{
		{
			Pattern:  "/",
			Segments: []router.Segment{},
			Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString("home")
				return nil
			},
		},
		{
			Pattern: "/post/[id]",
			Segments: []router.Segment{
				{Kind: router.SegmentStatic, Value: "post"},
				{Kind: router.SegmentParam, Name: "id"},
			},
			Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
				w.WriteString("post")
				return nil
			},
		},
	})

	// Force pre-encoded version-poll bytes by stamping an AppVersion + poll
	// config the test runs in isolation from a Vite manifest input.
	srv.appVersion = "abc123"
	srv.encodedAppVersion = encodeAppVersionField("abc123")
	srv.encodedVersionPoll = encodeVersionPollField("abc123", kit.VersionPollConfig{
		PollInterval: 30 * time.Second,
	})

	cases := []struct {
		name    string
		payload clientPayload
	}{
		{
			name: "initial-render-fully-populated",
			payload: clientPayload{
				RouteID:    "/post/[id]",
				Data:       loadShape{Title: "hi", Count: 7},
				LayoutData: []any{loadShape{Title: "outer", Count: 1}},
				Form:       nil,
				URL:        "https://example.test/post/42?q=1",
				Params:     map[string]string{"id": "42"},
				Status:     200,
				PageError:  nil,
				Manifest:   srv.clientManifest,
				Deps:       []string{"posts"},
				AppVersion: "abc123",
				VersionPoll: &clientVersionPoll{
					IntervalMS: 30000,
				},
			},
		},
		{
			name: "data-json-no-initial-fields",
			payload: clientPayload{
				RouteID: "/",
				Data:    loadShape{Title: "json", Count: 3},
				URL:     "https://example.test/__data.json",
				Params:  map[string]string{},
				Status:  200,
			},
		},
		{
			name: "form-action-override",
			payload: clientPayload{
				RouteID: "/",
				Data:    loadShape{Title: "form", Count: 0},
				Form:    map[string]any{"ok": true, "msg": "saved"},
				URL:     "https://example.test/?action",
				Params:  map[string]string{},
				Status:  303,
			},
		},
		{
			name: "error-boundary",
			payload: clientPayload{
				RouteID:   "/",
				Data:      nil,
				URL:       "https://example.test/missing",
				Params:    map[string]string{},
				Status:    500,
				PageError: &clientPageError{Message: "boom", Status: 500},
			},
		},
		{
			name: "data-with-script-special-chars",
			payload: clientPayload{
				RouteID: "/",
				Data:    map[string]string{"html": "<!-- evil -->", "tag": "</script>"},
				URL:     "https://example.test/",
				Params:  map[string]string{},
				Status:  200,
			},
		},
		{
			// CSRFToken populated covers the per-request token field the
			// CSRF auto-inject contract (#510 / #523) ships to the client
			// so the post-mount splicer can re-add the hidden input that
			// Svelte 5 hydration would otherwise strip on ssr-fallback
			// routes whose source `.svelte` lacks the input in its vDOM.
			name: "csrf-token-set",
			payload: clientPayload{
				RouteID:   "/login",
				Data:      map[string]any{},
				URL:       "https://example.test/login",
				Params:    map[string]string{},
				Status:    200,
				CSRFToken: "abc123-token-with-dashes_and-underscores",
			},
		},
		{
			// CSRFToken alongside AppVersion + VersionPoll pins the
			// relative emit order so the splice writer keeps matching
			// encoding/json's struct-declaration ordering (CSRFToken
			// sits between AppVersion and VersionPoll in clientPayload).
			name: "csrf-token-with-app-version-and-poll",
			payload: clientPayload{
				RouteID:    "/post/[id]",
				Data:       loadShape{Title: "hi", Count: 7},
				URL:        "https://example.test/post/42",
				Params:     map[string]string{"id": "42"},
				Status:     200,
				Manifest:   srv.clientManifest,
				AppVersion: "abc123",
				CSRFToken:  "tok-xyz",
				VersionPoll: &clientVersionPoll{
					IntervalMS: 30000,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}

			var got bytes.Buffer
			if err := srv.writePayloadJSON(&got, tc.payload); err != nil {
				t.Fatalf("writePayloadJSON: %v", err)
			}

			if !bytes.Equal(want, got.Bytes()) {
				t.Fatalf("byte mismatch:\n want: %s\n  got: %s", want, got.Bytes())
			}

			// Spliced bytes must Unmarshal back without error so the
			// client-side parser sees a valid JSON document. Field-level
			// equality after Unmarshal is intentionally not checked here:
			// json.Unmarshal collapses Data/LayoutData (typed `any`) into
			// map[string]any with sorted keys on re-Marshal, which is a
			// known encoding/json behavior unrelated to the splice writer.
			var roundtrip clientPayload
			if err := json.Unmarshal(got.Bytes(), &roundtrip); err != nil {
				t.Fatalf("Unmarshal round-trip: %v; bytes=%s", err, got.Bytes())
			}
		})
	}
}

// TestEncodePayloadFieldByteShape pins the encoded slice format so a
// future caller can rely on the comma-prefixed `,"name":<json>` layout
// the splice writer expects.
func TestEncodePayloadFieldByteShape(t *testing.T) {
	t.Parallel()

	got := encodePayloadField("foo", "bar")
	want := []byte(`,"foo":"bar"`)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

// TestEncodeRouteIDsCoversAllRoutes asserts every registered route ends
// up with a pre-encoded routeId entry so the splice writer's hot path
// never falls back to per-request marshal.
func TestEncodeRouteIDsCoversAllRoutes(t *testing.T) {
	t.Parallel()

	routes := []router.Route{
		{Pattern: "/"},
		{Pattern: "/blog"},
		{Pattern: "/post/[id]"},
	}
	encoded := encodeRouteIDs(routes)
	for _, r := range routes {
		raw, ok := encoded[r.Pattern]
		if !ok {
			t.Fatalf("missing pattern %q", r.Pattern)
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatalf("Unmarshal %q: %v", r.Pattern, err)
		}
		if s != r.Pattern {
			t.Fatalf("encoded %q decoded to %q", r.Pattern, s)
		}
	}
}

// TestWritePayloadHandlesEmptyServer checks the writer still produces
// valid JSON when called against a Server that has no pre-encoded
// stable bytes (e.g. a hand-constructed test server with no routes
// known to encodeRouteIDs). The fallback marshal path must engage so
// the wire format stays correct.
func TestWritePayloadHandlesEmptyServer(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	p := clientPayload{
		RouteID:   "/unregistered",
		Data:      map[string]int{"v": 1},
		URL:       "https://example.test/",
		Params:    map[string]string{},
		Status:    200,
		PageError: nil,
	}

	want, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got bytes.Buffer
	if err := srv.writePayloadJSON(&got, p); err != nil {
		t.Fatalf("writePayloadJSON: %v", err)
	}
	if !bytes.Equal(want, got.Bytes()) {
		t.Fatalf("byte mismatch:\n want: %s\n  got: %s", want, got.Bytes())
	}
}

// TestApplyInitialPayloadFieldsThenWritePayload exercises the integrated
// path the renderPage code follows: build payload, stamp initial fields,
// then encode. Output must equal a plain json.Marshal of the resulting
// struct.
func TestApplyInitialPayloadFieldsThenWritePayload(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: []router.Segment{},
		Page: func(w *render.Writer, _ *kit.RenderCtx, _ any) error {
			w.WriteString("home")
			return nil
		},
	}})
	srv.appVersion = "deadbeef"
	srv.encodedAppVersion = encodeAppVersionField("deadbeef")
	srv.versionPoll = kit.VersionPollConfig{PollInterval: 60 * time.Second}.Resolve()
	srv.encodedVersionPoll = encodeVersionPollField("deadbeef", srv.versionPoll)

	r := httptest.NewRequest("GET", "/", nil)
	route := &router.Route{Pattern: "/"}
	payload := buildClientPayload(r, nil, route, map[string]any{"hello": "world"}, nil, map[string]string{}, nil)
	srv.applyInitialPayloadFields(&payload)

	want, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var got bytes.Buffer
	if err := srv.writePayloadJSON(&got, payload); err != nil {
		t.Fatalf("writePayloadJSON: %v", err)
	}
	if !bytes.Equal(want, got.Bytes()) {
		t.Fatalf("byte mismatch:\n want: %s\n  got: %s", want, got.Bytes())
	}

	// Sanity: splice path must include the pre-encoded manifest, version, poll.
	if !bytes.Contains(got.Bytes(), []byte(`"manifest":`)) {
		t.Errorf("output missing manifest field: %s", got.Bytes())
	}
	if !bytes.Contains(got.Bytes(), []byte(`"appVersion":"deadbeef"`)) {
		t.Errorf("output missing appVersion field: %s", got.Bytes())
	}
	if !bytes.Contains(got.Bytes(), []byte(`"versionPoll":`)) {
		t.Errorf("output missing versionPoll field: %s", got.Bytes())
	}
}

// TestEncodeJSONValueNilHandling pins the nil-interface and nil-pointer
// behavior so the splice writer matches encoding/json on the form field
// (any) and pageError (*clientPageError).
func TestEncodeJSONValueNilHandling(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   any
		want string
	}{
		{"nil-interface", nil, "null"},
		{"nil-pointer", (*clientPageError)(nil), "null"},
		{"empty-string", "", `""`},
		{"empty-map", map[string]string{}, "{}"},
		{"nil-map", map[string]string(nil), "null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got bytes.Buffer
			if err := encodeJSONValue(&got, tc.in); err != nil {
				t.Fatalf("encodeJSONValue: %v", err)
			}
			if got.String() != tc.want {
				t.Fatalf("got %q, want %q", got.String(), tc.want)
			}
		})
	}
}

// TestVersionPollBytesMatchClientStruct asserts the pre-encoded
// versionPoll bytes Unmarshal back to the same wire shape the per-request
// path would have produced. Guards against future drift if
// clientVersionPoll grows fields.
func TestVersionPollBytesMatchClientStruct(t *testing.T) {
	t.Parallel()

	vp := kit.VersionPollConfig{PollInterval: 45 * time.Second}.Resolve()
	encoded := encodeVersionPollField("v1", vp)
	if encoded == nil {
		t.Fatalf("encoded is nil for non-empty appVersion")
	}
	want, err := json.Marshal(map[string]any{
		"versionPoll": clientVersionPoll{
			IntervalMS: vp.PollInterval.Milliseconds(),
			Disabled:   vp.Disabled,
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	// want is `{"versionPoll":{...}}`; encoded is `,"versionPoll":{...}`.
	// Compare the body slice past the leading delimiter on each side.
	wantBody := want[1 : len(want)-1]
	encodedBody := encoded[1:]
	if !bytes.Equal(wantBody, encodedBody) {
		t.Fatalf("versionPoll bytes diverge:\n want: %s\n  got: %s", wantBody, encodedBody)
	}
}

// TestPayloadBufPoolReusesScratchBuffers ensures the writer scratch pool
// returns reset buffers so leftover bytes from a previous request don't
// bleed into the next response. Reflection-free smoke test.
func TestPayloadBufPoolReusesScratchBuffers(t *testing.T) {
	t.Parallel()

	first := acquirePayloadBuf()
	first.WriteString("dirty")
	releasePayloadBuf(first)

	second := acquirePayloadBuf()
	defer releasePayloadBuf(second)
	if second.Len() != 0 {
		t.Fatalf("acquired buffer not reset: contained %q", second.String())
	}

	// reflect.TypeOf to avoid a direct pointer comparison that the race
	// detector flags as benign — we only care the pool returns a usable
	// *bytes.Buffer, not pointer identity.
	if got := reflect.TypeOf(second).String(); got != "*bytes.Buffer" {
		t.Fatalf("unexpected scratch type: %s", got)
	}
}
