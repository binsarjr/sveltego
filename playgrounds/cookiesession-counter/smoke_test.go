// Package cookiesessioncounter provides an end-to-end smoke test for the
// cookiesession Handle middleware without requiring generated code.
package cookiesessioncounter_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/binsarjr/sveltego/cookiesession"
	"github.com/binsarjr/sveltego/exports/kit"
)

// CounterSession mirrors the session type used by the playground.
type CounterSession struct{ Count int }

func newCodec(t *testing.T) cookiesession.Codec {
	t.Helper()
	c, err := cookiesession.NewCodec([]cookiesession.Secret{
		{ID: 1, Key: []byte("smoke-test-key-must-be-32-bytes!")},
	})
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c
}

// increment simulates the "increment" form action: reads session, adds 1,
// stores back.
func incrementHandler(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
	sess, ok := cookiesession.From[CounterSession](ev)
	if ok {
		_ = sess.Update(func(s CounterSession) CounterSession {
			s.Count++
			return s
		})
	}
	return kit.NewResponse(http.StatusOK, nil), nil
}

// extractCookies pulls Set-Cookie values from a kit.Response.
func extractCookies(resp *kit.Response) []*http.Cookie {
	if resp == nil {
		return nil
	}
	var out []*http.Cookie
	for _, line := range resp.Headers["Set-Cookie"] {
		h := http.Header{"Set-Cookie": {line}}
		out = append(out, (&http.Response{Header: h}).Cookies()...)
	}
	return out
}

// applyToRequest builds a new request carrying the given cookies.
func applyToRequest(method, path string, cookies []*http.Cookie) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	for _, ck := range cookies {
		if ck.MaxAge >= 0 {
			r.AddCookie(ck)
		}
	}
	return r
}

// TestSmoke exercises the full round-trip: 3 increments via the middleware,
// verifying cookie value persists and decrypts correctly across requests.
func TestSmoke(t *testing.T) {
	codec := newCodec(t)
	mw := kit.Sequence(
		cookiesession.Handle[CounterSession](codec, "counter",
			cookiesession.WithSecure(false),
			cookiesession.WithHTTPOnly(true),
		),
		incrementHandler,
	)

	var cookies []*http.Cookie

	// 3 increment requests.
	for i := range 3 {
		r := applyToRequest(http.MethodPost, "/", cookies)
		ev := kit.NewRequestEvent(r, nil)
		resp, err := mw(ev, func(ev *kit.RequestEvent) (*kit.Response, error) {
			return incrementHandler(ev, nil)
		})
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
		newCookies := extractCookies(resp)
		if len(newCookies) > 0 {
			cookies = newCookies
		}
	}

	// Final read: counter must be 3.
	r := applyToRequest(http.MethodGet, "/", cookies)
	ev := kit.NewRequestEvent(r, nil)
	mwRead := cookiesession.Handle[CounterSession](codec, "counter")
	_, err := mwRead(ev, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, ok := cookiesession.From[CounterSession](ev)
		if !ok {
			t.Fatal("From[CounterSession]: not found")
		}
		if sess.Data().Count != 3 {
			t.Fatalf("smoke: Count=%d, want 3", sess.Data().Count)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
}

// TestSmokeCookieDecryptable verifies the cookie produced by the middleware
// is decryptable by a fresh codec with the same key (simulating a server
// restart).
func TestSmokeCookieDecryptable(t *testing.T) {
	codec := newCodec(t)
	mw := cookiesession.Handle[CounterSession](codec, "counter")

	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	ev1 := kit.NewRequestEvent(r1, nil)
	resp1, err := mw(ev1, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, _ := cookiesession.From[CounterSession](ev)
		_ = sess.Set(CounterSession{Count: 42})
		return kit.NewResponse(http.StatusOK, nil), nil
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	cookies := extractCookies(resp1)
	if len(cookies) == 0 {
		t.Fatal("no cookies after Set")
	}

	// Fresh codec same key = restart simulation.
	codec2 := newCodec(t)
	mw2 := cookiesession.Handle[CounterSession](codec2, "counter")

	r2 := applyToRequest(http.MethodGet, "/", cookies)
	ev2 := kit.NewRequestEvent(r2, nil)
	_, err = mw2(ev2, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, ok := cookiesession.From[CounterSession](ev)
		if !ok {
			t.Fatal("From after restart: not found")
		}
		if sess.Data().Count != 42 {
			t.Fatalf("after restart: Count=%d, want 42", sess.Data().Count)
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("read after restart: %v", err)
	}
}
