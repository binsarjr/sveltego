package cookiesession_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/binsarjr/sveltego/cookiesession"
	"github.com/binsarjr/sveltego/exports/kit"
)

// --- helpers ---

type (
	fooData struct{ Foo int }
	barData struct{ Bar string }
)

func makeHandleCodec(t *testing.T, secrets ...cookiesession.Secret) cookiesession.Codec {
	t.Helper()
	if len(secrets) == 0 {
		secrets = []cookiesession.Secret{{ID: 1, Key: makeKey(0xCD)}}
	}
	c, err := cookiesession.NewCodec(secrets)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c
}

// runHandle exercises the Handle middleware pipeline via a fake kit.HandleFn chain.
// innerFn is called with the RequestEvent after Handle runs.
func runHandle(t *testing.T, r *http.Request, mw kit.HandleFn, innerFn func(*kit.RequestEvent) (*kit.Response, error)) *kit.Response {
	t.Helper()
	var captured *kit.Response
	var capturedErr error
	resolve := func(ev *kit.RequestEvent) (*kit.Response, error) {
		return innerFn(ev)
	}
	ev := kit.NewRequestEvent(r, nil)
	resp, err := mw(ev, resolve)
	captured = resp
	capturedErr = err
	if capturedErr != nil {
		t.Fatalf("Handle returned error: %v", capturedErr)
	}
	return captured
}

// extractResponseCookies pulls Set-Cookie values from a kit.Response.
func extractResponseCookies(resp *kit.Response) []*http.Cookie {
	if resp == nil {
		return nil
	}
	// Parse Set-Cookie headers from the response.
	var cookies []*http.Cookie
	for _, line := range resp.Headers["Set-Cookie"] {
		h := http.Header{"Set-Cookie": {line}}
		resp2 := &http.Response{Header: h}
		cookies = append(cookies, resp2.Cookies()...)
	}
	return cookies
}

// applyToRequest copies cookies from a kit.Response into a new request.
func applyToRequest(t *testing.T, method, path string, cookies []*http.Cookie) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	for _, ck := range cookies {
		if ck.MaxAge >= 0 { // skip deletion cookies
			r.AddCookie(ck)
		}
	}
	return r
}

// --- tests ---

// TestHandleRoundTrip verifies cookie set → next request reads → increment → write back.
func TestHandleRoundTrip(t *testing.T) {
	codec := makeHandleCodec(t)
	mw := cookiesession.Handle[fooData](codec, "foo")

	// Request 1: handler increments from 0 to 1.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	resp1 := runHandle(t, r1, mw, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, ok := cookiesession.From[fooData](ev)
		if !ok {
			t.Fatal("From[fooData]: not found in Locals")
		}
		if err := sess.Update(func(d fooData) fooData {
			d.Foo++
			return d
		}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		return kit.NewResponse(http.StatusOK, []byte("ok")), nil
	})
	cookies1 := extractResponseCookies(resp1)
	if len(cookies1) == 0 {
		t.Fatal("expected Set-Cookie on first request")
	}

	// Request 2: read value, expect Foo == 1, then increment to 2.
	r2 := applyToRequest(t, http.MethodGet, "/", cookies1)
	resp2 := runHandle(t, r2, mw, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, ok := cookiesession.From[fooData](ev)
		if !ok {
			t.Fatal("From[fooData]: not found")
		}
		got := sess.Data()
		if got.Foo != 1 {
			t.Fatalf("request 2: Foo=%d, want 1", got.Foo)
		}
		_ = sess.Update(func(d fooData) fooData { d.Foo++; return d })
		return kit.NewResponse(http.StatusOK, []byte("ok")), nil
	})
	cookies2 := extractResponseCookies(resp2)

	// Request 3: confirm Foo == 2.
	r3 := applyToRequest(t, http.MethodGet, "/", cookies2)
	runHandle(t, r3, mw, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, ok := cookiesession.From[fooData](ev)
		if !ok {
			t.Fatal("From[fooData]: not found")
		}
		if sess.Data().Foo != 2 {
			t.Fatalf("request 3: Foo=%d, want 2", sess.Data().Foo)
		}
		return nil, nil
	})
}

// TestHandleTwoParallelNoCrosstalk verifies Handle[Foo] + Handle[Bar] do not collide.
func TestHandleTwoParallelNoCrosstalk(t *testing.T) {
	codec := makeHandleCodec(t)
	fooMW := cookiesession.Handle[fooData](codec, "foo-ck")
	barMW := cookiesession.Handle[barData](codec, "bar-ck")
	combined := kit.Sequence(fooMW, barMW)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ev := kit.NewRequestEvent(r, nil)

	var (
		gotFoo *cookiesession.Session[fooData]
		gotBar *cookiesession.Session[barData]
	)
	_, err := combined(ev, func(ev *kit.RequestEvent) (*kit.Response, error) {
		var okF, okB bool
		gotFoo, okF = cookiesession.From[fooData](ev)
		gotBar, okB = cookiesession.From[barData](ev)
		if !okF {
			t.Fatal("From[fooData] not found")
		}
		if !okB {
			t.Fatal("From[barData] not found")
		}
		return nil, nil
	})
	if err != nil {
		t.Fatalf("combined Handle: %v", err)
	}
	if gotFoo == nil || gotBar == nil {
		t.Fatal("one of the sessions is nil")
	}
	// Modifying foo should not affect bar.
	_ = gotFoo.Update(func(d fooData) fooData { d.Foo = 99; return d })
	if gotBar.Data().Bar != "" {
		t.Fatalf("bar session mutated by foo update: Bar=%q", gotBar.Data().Bar)
	}
}

// TestHandleTamperedCookieZeroSession verifies a tampered cookie yields a zero session.
func TestHandleTamperedCookieZeroSession(t *testing.T) {
	codec := makeHandleCodec(t)
	mw := cookiesession.Handle[fooData](codec, "foo")

	// Set a valid session first.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	resp1 := runHandle(t, r1, mw, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, _ := cookiesession.From[fooData](ev)
		_ = sess.Set(fooData{Foo: 42})
		return kit.NewResponse(http.StatusOK, nil), nil
	})
	cookies1 := extractResponseCookies(resp1)

	// Tamper: flip a byte in the cookie value.
	for _, ck := range cookies1 {
		if ck.Name == "foo" {
			v := []byte(ck.Value)
			v[10] ^= 0xFF
			ck.Value = string(v)
		}
	}

	r2 := applyToRequest(t, http.MethodGet, "/", cookies1)
	runHandle(t, r2, mw, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, ok := cookiesession.From[fooData](ev)
		if !ok {
			t.Fatal("From[fooData]: not found after tamper")
		}
		if sess.Data().Foo != 0 {
			t.Fatalf("tampered cookie: Foo=%d, want 0 (zero session)", sess.Data().Foo)
		}
		return nil, nil
	})
}

// TestHandleRotation verifies old-secret cookie still decrypts after rotation.
func TestHandleRotation(t *testing.T) {
	oldKey := makeKey(0x11)
	newKey := makeKey(0x22)

	oldCodec := makeHandleCodec(t, cookiesession.Secret{ID: 1, Key: oldKey})
	mwOld := cookiesession.Handle[fooData](oldCodec, "foo")

	// Encode a session with old key.
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	resp1 := runHandle(t, r1, mwOld, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, _ := cookiesession.From[fooData](ev)
		_ = sess.Set(fooData{Foo: 7})
		return kit.NewResponse(http.StatusOK, nil), nil
	})
	cookies1 := extractResponseCookies(resp1)

	// Now app rotates: new key first, old key second.
	rotatedCodec := makeHandleCodec(t,
		cookiesession.Secret{ID: 2, Key: newKey},
		cookiesession.Secret{ID: 1, Key: oldKey},
	)
	mwRotated := cookiesession.Handle[fooData](rotatedCodec, "foo")

	r2 := applyToRequest(t, http.MethodGet, "/", cookies1)
	runHandle(t, r2, mwRotated, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, ok := cookiesession.From[fooData](ev)
		if !ok {
			t.Fatal("From[fooData]: not found after rotation")
		}
		if sess.Data().Foo != 7 {
			t.Fatalf("rotation: Foo=%d, want 7", sess.Data().Foo)
		}
		return nil, nil
	})
}

// TestHandleConcurrentRequests verifies no data race when concurrent goroutines
// each process their own request (independent *Session values).
func TestHandleConcurrentRequests(t *testing.T) {
	codec := makeHandleCodec(t)
	mw := cookiesession.Handle[fooData](codec, "foo")

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			_ = runHandle(t, r, mw, func(ev *kit.RequestEvent) (*kit.Response, error) {
				sess, _ := cookiesession.From[fooData](ev)
				_ = sess.Set(fooData{Foo: 1})
				return nil, nil
			})
		}()
	}
	wg.Wait()
}

// TestHandleCookieAttrs verifies that CookieOption values surface on Set-Cookie.
func TestHandleCookieAttrs(t *testing.T) {
	codec := makeHandleCodec(t)
	mw := cookiesession.Handle[fooData](codec, "myck",
		cookiesession.WithHTTPOnly(true),
		cookiesession.WithSecure(false),
		cookiesession.WithSameSite(http.SameSiteStrictMode),
		cookiesession.WithPath("/app"),
	)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := runHandle(t, r, mw, func(ev *kit.RequestEvent) (*kit.Response, error) {
		sess, _ := cookiesession.From[fooData](ev)
		_ = sess.Set(fooData{Foo: 1})
		return kit.NewResponse(http.StatusOK, nil), nil
	})

	found := false
	for _, line := range resp.Headers["Set-Cookie"] {
		if strings.HasPrefix(line, "myck=") {
			found = true
			if !strings.Contains(line, "HttpOnly") {
				t.Errorf("Set-Cookie missing HttpOnly: %s", line)
			}
			if strings.Contains(line, "Secure") {
				t.Errorf("Set-Cookie should not have Secure: %s", line)
			}
			if !strings.Contains(line, "SameSite=Strict") {
				t.Errorf("Set-Cookie missing SameSite=Strict: %s", line)
			}
			if !strings.Contains(line, "Path=/app") {
				t.Errorf("Set-Cookie missing Path=/app: %s", line)
			}
		}
	}
	if !found {
		t.Fatal("Set-Cookie for 'myck' not found in response")
	}
}

// TestHandleFromAbsent verifies From returns (nil, false) when middleware not installed.
func TestHandleFromAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	ev := kit.NewRequestEvent(r, nil)
	sess, ok := cookiesession.From[fooData](ev)
	if ok || sess != nil {
		t.Fatal("From[fooData] should return (nil, false) when middleware not installed")
	}
}
