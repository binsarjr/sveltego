package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/binsarjr/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/runtime/router"
)

// TestServer_ServiceWorker_RegistersWhenEnabled covers issue #89: SSR
// HTML emits the auto-registration <script> for /service-worker.js when
// Config.ServiceWorker is true. The script is feature-gated on
// 'serviceWorker' in navigator and registered with scope "/" so SPA
// navigation under any sub-path is covered.
func TestServer_ServiceWorker_RegistersWhenEnabled(t *testing.T) {
	t.Parallel()
	srv, err := New(Config{
		Routes: []router.Route{{
			Pattern:  "/",
			Segments: segmentsFor("/"),
			Page:     staticPage("<h1>home</h1>"),
		}},
		Matchers:      params.DefaultMatchers(),
		Shell:         testShell,
		Logger:        quietLogger(),
		ServiceWorker: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	for _, want := range []string{
		"'serviceWorker' in navigator",
		"navigator.serviceWorker.register('/service-worker.js'",
		"scope:'/'",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("body missing %q\nbody:\n%s", want, s)
		}
	}
	// The registration must sit before </body> so it doesn't end up in <head>.
	regIdx := strings.Index(s, "navigator.serviceWorker.register")
	bodyEnd := strings.Index(s, "</body>")
	if regIdx < 0 || bodyEnd < 0 || regIdx > bodyEnd {
		t.Errorf("registration script not before </body>: regIdx=%d bodyEnd=%d", regIdx, bodyEnd)
	}
}

// TestServer_ServiceWorker_AbsentByDefault verifies that the default zero
// value (Config{ServiceWorker: false}) suppresses the registration script
// entirely — no script tag, no console hook. Users opt out by leaving
// Config.ServiceWorker false even when src/service-worker.ts exists.
func TestServer_ServiceWorker_AbsentByDefault(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, []router.Route{{
		Pattern:  "/",
		Segments: segmentsFor("/"),
		Page:     staticPage("<h1>home</h1>"),
	}})
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	for _, banned := range []string{
		"navigator.serviceWorker.register",
		"/service-worker.js",
	} {
		if strings.Contains(s, banned) {
			t.Errorf("body unexpectedly contains %q\nbody:\n%s", banned, s)
		}
	}
}
