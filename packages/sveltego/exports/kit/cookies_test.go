package kit_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/exports/kit"
)

func ptrBool(b bool) *bool { return &b }

func TestCookies_Get_ReadsIncoming(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	c := kit.NewCookies(req)

	v, ok := c.Get("session")
	if !ok || v != "abc" {
		t.Errorf("Get(session) = (%q,%v), want (abc,true)", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Error("Get(missing) ok=true, want false")
	}
}

func TestCookies_Set_DefaultsApplied(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := kit.NewCookies(req)
	c.Set("k", "v", kit.CookieOpts{})

	rw := httptest.NewRecorder()
	c.Apply(rw)
	got := rw.Header().Get("Set-Cookie")

	wants := []string{"k=v", "Path=/", "HttpOnly", "SameSite=Lax"}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("Set-Cookie %q missing %q", got, w)
		}
	}
	if strings.Contains(got, "Secure") {
		t.Errorf("Set-Cookie %q should not have Secure for plain HTTP", got)
	}
}

func TestCookies_Set_SecureFromTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	c := kit.NewCookies(req)
	c.Set("k", "v", kit.CookieOpts{})

	rw := httptest.NewRecorder()
	c.Apply(rw)
	got := rw.Header().Get("Set-Cookie")

	if !strings.Contains(got, "Secure") {
		t.Errorf("Set-Cookie %q missing Secure on HTTPS", got)
	}
}

func TestCookies_Set_ExplicitOptsWin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	c := kit.NewCookies(req)
	c.Set("k", "v", kit.CookieOpts{
		Path:     "/admin",
		Domain:   "example.com",
		MaxAge:   2 * time.Hour,
		HttpOnly: ptrBool(false),
		Secure:   ptrBool(false),
		SameSite: http.SameSiteStrictMode,
	})

	rw := httptest.NewRecorder()
	c.Apply(rw)
	got := rw.Header().Get("Set-Cookie")

	wants := []string{"k=v", "Path=/admin", "Domain=example.com", "Max-Age=7200", "SameSite=Strict"}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("Set-Cookie %q missing %q", got, w)
		}
	}
	if strings.Contains(got, "HttpOnly") {
		t.Errorf("Set-Cookie %q has HttpOnly when explicitly disabled", got)
	}
	if strings.Contains(got, "Secure") {
		t.Errorf("Set-Cookie %q has Secure when explicitly disabled", got)
	}
}

func TestCookies_SetExposed_DefaultNoHttpOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := kit.NewCookies(req)
	c.SetExposed("ui-pref", "dark", kit.CookieOpts{})

	rw := httptest.NewRecorder()
	c.Apply(rw)
	got := rw.Header().Get("Set-Cookie")

	if strings.Contains(got, "HttpOnly") {
		t.Errorf("SetExposed Set-Cookie %q should not have HttpOnly", got)
	}
}

func TestCookies_SetExposed_HttpOnlyOverride(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := kit.NewCookies(req)
	c.SetExposed("k", "v", kit.CookieOpts{HttpOnly: ptrBool(true)})

	rw := httptest.NewRecorder()
	c.Apply(rw)
	got := rw.Header().Get("Set-Cookie")

	if !strings.Contains(got, "HttpOnly") {
		t.Errorf("SetExposed with HttpOnly=true missing HttpOnly: %q", got)
	}
}

func TestCookies_Multiple_SeparateHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := kit.NewCookies(req)
	c.Set("a", "1", kit.CookieOpts{})
	c.Set("b", "2", kit.CookieOpts{})
	c.Set("c", "3", kit.CookieOpts{})

	rw := httptest.NewRecorder()
	c.Apply(rw)

	hdrs := rw.Header().Values("Set-Cookie")
	if len(hdrs) != 3 {
		t.Fatalf("Set-Cookie count = %d, want 3", len(hdrs))
	}
	for i, name := range []string{"a=1", "b=2", "c=3"} {
		if !strings.Contains(hdrs[i], name) {
			t.Errorf("hdr[%d] %q missing %q", i, hdrs[i], name)
		}
	}
}

func TestCookies_Delete_EmitsExpired(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := kit.NewCookies(req)
	c.Delete("session", kit.CookieOpts{})

	rw := httptest.NewRecorder()
	c.Apply(rw)
	got := rw.Header().Get("Set-Cookie")

	wants := []string{"session=", "Path=/", "Max-Age=0"}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("Delete Set-Cookie %q missing %q", got, w)
		}
	}
}

func TestCookies_Delete_RoundTrip(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "abc"})
	c := kit.NewCookies(req)

	if v, ok := c.Get("session"); !ok || v != "abc" {
		t.Fatalf("seed Get = (%q,%v)", v, ok)
	}
	c.Delete("session", kit.CookieOpts{Path: "/admin", Domain: "example.com"})

	rw := httptest.NewRecorder()
	c.Apply(rw)
	got := rw.Header().Get("Set-Cookie")

	if !strings.Contains(got, "Path=/admin") {
		t.Errorf("Delete didn't honor opts.Path: %q", got)
	}
	if !strings.Contains(got, "Domain=example.com") {
		t.Errorf("Delete didn't honor opts.Domain: %q", got)
	}
}

func TestCookies_Apply_NoCookiesNoHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := kit.NewCookies(req)

	rw := httptest.NewRecorder()
	c.Apply(rw)

	if hdrs := rw.Header().Values("Set-Cookie"); len(hdrs) != 0 {
		t.Errorf("Set-Cookie count = %d, want 0", len(hdrs))
	}
}

func TestCookies_NilSafe(t *testing.T) {
	var c *kit.Cookies
	if v, ok := c.Get("x"); ok || v != "" {
		t.Errorf("nil Get = (%q,%v)", v, ok)
	}
	c.Set("x", "y", kit.CookieOpts{})
	c.SetExposed("x", "y", kit.CookieOpts{})
	c.Delete("x", kit.CookieOpts{})
	c.Apply(httptest.NewRecorder())
}

func TestCookies_NewCookies_NilRequest(t *testing.T) {
	c := kit.NewCookies(nil)
	if c == nil {
		t.Fatal("NewCookies(nil) = nil, want non-nil")
	}
	if _, ok := c.Get("anything"); ok {
		t.Error("nil-request jar should have no incoming cookies")
	}
	c.Set("k", "v", kit.CookieOpts{})
	rw := httptest.NewRecorder()
	c.Apply(rw)
	if rw.Header().Get("Set-Cookie") == "" {
		t.Error("Set still queues when request was nil")
	}
}
