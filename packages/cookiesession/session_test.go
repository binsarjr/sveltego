package cookiesession_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/cookiesession"
)

// helpers

func makeKey(b byte) []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = b
	}
	return key
}

func makeCodec(t *testing.T, secrets ...cookiesession.Secret) cookiesession.Codec {
	t.Helper()
	if len(secrets) == 0 {
		secrets = []cookiesession.Secret{{ID: 1, Key: makeKey(0xAB)}}
	}
	c, err := cookiesession.NewCodec(secrets)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c
}

type counter struct{ Count int }

const sessionName = "sess"

func defaultOpts() cookiesession.Options {
	return cookiesession.Options{Name: sessionName}
}

// extractSetCookies pulls all Set-Cookie header values from w.
func extractSetCookies(w *httptest.ResponseRecorder) []*http.Cookie {
	resp := w.Result()
	defer resp.Body.Close()
	return resp.Cookies()
}

// applyCookiesToRequest returns a new request with the given cookies applied
// as incoming Cookie headers.
func applyCookiesToRequest(t *testing.T, r *http.Request, cookies []*http.Cookie) *http.Request {
	t.Helper()
	r2 := httptest.NewRequest(r.Method, r.URL.String(), nil)
	for _, ck := range cookies {
		r2.AddCookie(ck)
	}
	return r2
}

// Tests

func TestRoundTripSmall(t *testing.T) {
	codec := makeCodec(t)
	opts := defaultOpts()

	// Request 1: set session.
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	s1, err := cookiesession.NewSession[counter](r1, w1, codec, opts)
	if err != nil {
		t.Fatalf("NewSession (req1): %v", err)
	}
	if err := s1.Set(counter{Count: 42}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Request 2: read session back.
	cookies := extractSetCookies(w1)
	if len(cookies) == 0 {
		t.Fatal("no Set-Cookie from request 1")
	}
	r2 := applyCookiesToRequest(t, r1, cookies)
	w2 := httptest.NewRecorder()
	s2, err := cookiesession.NewSession[counter](r2, w2, codec, opts)
	if err != nil {
		t.Fatalf("NewSession (req2): %v", err)
	}
	got := s2.Data()
	if got.Count != 42 {
		t.Fatalf("round-trip: got Count=%d, want 42", got.Count)
	}
}

func TestRoundTripLarge(t *testing.T) {
	codec := makeCodec(t)
	opts := defaultOpts()

	// Build a payload > 4000 bytes when JSON+encrypted.
	// A string of 6000 'x' chars should produce a chunked payload.
	type bigData struct {
		Payload string
	}
	bigVal := bigData{Payload: strings.Repeat("x", 6000)}

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	s1, err := cookiesession.NewSession[bigData](r1, w1, codec, opts)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := s1.Set(bigVal); err != nil {
		t.Fatalf("Set large: %v", err)
	}

	// Verify multiple Set-Cookie headers were emitted.
	cookies := extractSetCookies(w1)
	hasMeta := false
	chunkCount := 0
	for _, ck := range cookies {
		if ck.Name == sessionName+".meta" {
			hasMeta = true
		}
		if strings.HasPrefix(ck.Name, sessionName+".") && ck.Name != sessionName+".meta" {
			chunkCount++
		}
	}
	if !hasMeta {
		t.Fatal("expected .meta cookie for large payload")
	}
	if chunkCount < 2 {
		t.Fatalf("expected >=2 chunk cookies, got %d", chunkCount)
	}

	// Read back on request 2.
	r2 := applyCookiesToRequest(t, r1, cookies)
	w2 := httptest.NewRecorder()
	s2, err := cookiesession.NewSession[bigData](r2, w2, codec, opts)
	if err != nil {
		t.Fatalf("NewSession (req2): %v", err)
	}
	got := s2.Data()
	if got.Payload != bigVal.Payload {
		t.Fatalf("large round-trip mismatch: got %d chars, want %d",
			len(got.Payload), len(bigVal.Payload))
	}
}

func TestTamperedCookieReturnsError(t *testing.T) {
	codec := makeCodec(t)
	opts := defaultOpts()

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	s1, err := cookiesession.NewSession[counter](r1, w1, codec, opts)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := s1.Set(counter{Count: 7}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	cookies := extractSetCookies(w1)
	// Tamper: flip a char in the session cookie value.
	for _, ck := range cookies {
		if ck.Name == sessionName {
			runes := []rune(ck.Value)
			if runes[5] == 'f' {
				runes[5] = '0'
			} else {
				runes[5] = 'f'
			}
			ck.Value = string(runes)
		}
	}

	r2 := applyCookiesToRequest(t, r1, cookies)
	w2 := httptest.NewRecorder()
	_, err = cookiesession.NewSession[counter](r2, w2, codec, opts)
	if err == nil {
		t.Fatal("expected error on tampered cookie, got nil")
	}
}

func TestRotatedSecretDecodeSucceeds(t *testing.T) {
	oldSecret := cookiesession.Secret{ID: 1, Key: makeKey(0x11)}
	newSecret := cookiesession.Secret{ID: 2, Key: makeKey(0x22)}

	// Encode with old codec (single secret).
	oldCodec := makeCodec(t, oldSecret)
	opts := defaultOpts()

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	s1, err := cookiesession.NewSession[counter](r1, w1, oldCodec, opts)
	if err != nil {
		t.Fatalf("NewSession (old): %v", err)
	}
	if err := s1.Set(counter{Count: 99}); err != nil {
		t.Fatalf("Set (old): %v", err)
	}

	// Decode with new codec that has both secrets (new first, old second).
	newCodec := makeCodec(t, newSecret, oldSecret)
	cookies := extractSetCookies(w1)
	r2 := applyCookiesToRequest(t, r1, cookies)
	w2 := httptest.NewRecorder()
	s2, err := cookiesession.NewSession[counter](r2, w2, newCodec, opts)
	if err != nil {
		t.Fatalf("NewSession (rotated): %v", err)
	}
	if s2.Data().Count != 99 {
		t.Fatalf("rotated secret: got Count=%d, want 99", s2.Data().Count)
	}
}

func TestConcurrentUpdate(t *testing.T) {
	codec := makeCodec(t)
	opts := defaultOpts()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	s, err := cookiesession.NewSession[counter](r, w, codec, opts)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Concurrent reads via Data() are always safe.
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Data()
		}()
	}
	wg.Wait()

	// Sequential updates are fine.
	for range 5 {
		if err := s.Update(func(c counter) counter {
			c.Count++
			return c
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if s.Data().Count != 5 {
		t.Fatalf("after 5 Updates: got Count=%d, want 5", s.Data().Count)
	}
}

func TestDestroyClearsAndEmitsMaxAgeZero(t *testing.T) {
	codec := makeCodec(t)
	opts := defaultOpts()

	// First set a session.
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	s1, err := cookiesession.NewSession[counter](r1, w1, codec, opts)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := s1.Set(counter{Count: 3}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Now destroy.
	cookies := extractSetCookies(w1)
	r2 := applyCookiesToRequest(t, r1, cookies)
	w2 := httptest.NewRecorder()
	s2, err := cookiesession.NewSession[counter](r2, w2, codec, opts)
	if err != nil {
		t.Fatalf("NewSession (req2): %v", err)
	}
	if err := s2.Destroy(); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	// The response should contain a deletion cookie for the session.
	deleteCookies := extractSetCookies(w2)
	found := false
	for _, ck := range deleteCookies {
		if ck.Name == sessionName && ck.MaxAge == -1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected deletion cookie (MaxAge=-1) for %q, got %v", sessionName, deleteCookies)
	}

	// Data after destroy should be zero value.
	if s2.Data().Count != 0 {
		t.Fatalf("Data after Destroy: got Count=%d, want 0", s2.Data().Count)
	}
	if !s2.IsDirty() {
		t.Fatal("IsDirty should be true after Destroy")
	}
}

func TestExpiredPayloadTreatedAsEmpty(t *testing.T) {
	codec := makeCodec(t)
	opts := defaultOpts()
	opts.MaxAge = 1 * time.Millisecond // very short TTL

	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	s1, err := cookiesession.NewSession[counter](r1, w1, codec, opts)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if err := s1.Set(counter{Count: 9}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	cookies := extractSetCookies(w1)
	r2 := applyCookiesToRequest(t, r1, cookies)
	w2 := httptest.NewRecorder()
	s2, err := cookiesession.NewSession[counter](r2, w2, codec, opts)
	if err != nil {
		t.Fatalf("NewSession after expiry: %v", err)
	}
	// Data should be zero.
	if s2.Data().Count != 0 {
		t.Fatalf("expired session: got Count=%d, want 0", s2.Data().Count)
	}
}

func TestUpdateFuncMutation(t *testing.T) {
	codec := makeCodec(t)
	opts := defaultOpts()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	s, err := cookiesession.NewSession[counter](r, w, codec, opts)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if err := s.Update(func(c counter) counter {
		c.Count = 100
		return c
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if s.Data().Count != 100 {
		t.Fatalf("Update: got Count=%d, want 100", s.Data().Count)
	}
	if !s.IsDirty() {
		t.Fatal("IsDirty should be true after Update")
	}
}
