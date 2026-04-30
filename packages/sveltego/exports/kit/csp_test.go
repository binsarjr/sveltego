package kit

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNonce_ReadsFromLocals(t *testing.T) {
	t.Parallel()
	ev := NewRequestEvent(httptest.NewRequest("GET", "/", nil), nil)
	if got := Nonce(ev); got != "" {
		t.Fatalf("Nonce on empty Locals = %q, want empty", got)
	}
	SetNonce(ev, "abc")
	if got := Nonce(ev); got != "abc" {
		t.Fatalf("Nonce after SetNonce = %q, want abc", got)
	}
}

func TestNonce_NilEvent(t *testing.T) {
	t.Parallel()
	if got := Nonce(nil); got != "" {
		t.Fatalf("Nonce(nil) = %q", got)
	}
	SetNonce(nil, "x")
}

func TestNonceAttr_FormatsAttribute(t *testing.T) {
	t.Parallel()
	ev := NewRequestEvent(httptest.NewRequest("GET", "/", nil), nil)
	if got := NonceAttr(ev); got != "" {
		t.Fatalf("NonceAttr without nonce = %q, want empty", got)
	}
	SetNonce(ev, "deadbeef")
	if got := NonceAttr(ev); got != ` nonce="deadbeef"` {
		t.Fatalf("NonceAttr = %q", got)
	}
}

func TestBuildCSPHeader_DefaultsAndNonce(t *testing.T) {
	t.Parallel()
	got := BuildCSPHeader(CSPConfig{Mode: CSPStrict}, "n0")
	for _, want := range []string{
		"default-src 'self'",
		"script-src 'nonce-n0' 'strict-dynamic'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"connect-src 'self'",
		"base-uri 'self'",
		"form-action 'self'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestBuildCSPHeader_DeterministicOrder(t *testing.T) {
	t.Parallel()
	a := BuildCSPHeader(CSPConfig{Mode: CSPStrict}, "n0")
	b := BuildCSPHeader(CSPConfig{Mode: CSPStrict}, "n0")
	if a != b {
		t.Fatalf("non-deterministic CSP output:\n%s\n%s", a, b)
	}
}

func TestBuildCSPHeader_OverrideMerges(t *testing.T) {
	t.Parallel()
	cfg := CSPConfig{
		Mode: CSPStrict,
		Directives: map[string][]string{
			"img-src":     {"'self'", "https://cdn.example.com"},
			"connect-src": {},
		},
	}
	got := BuildCSPHeader(cfg, "n0")
	if !strings.Contains(got, "img-src 'self' https://cdn.example.com") {
		t.Errorf("user img-src override not applied: %q", got)
	}
	if strings.Contains(got, "connect-src") {
		t.Errorf("empty-value connect-src should remove directive: %q", got)
	}
}

func TestBuildCSPHeader_ReportTo(t *testing.T) {
	t.Parallel()
	got := BuildCSPHeader(CSPConfig{Mode: CSPStrict, ReportTo: "csp-endpoint"}, "n0")
	if !strings.HasSuffix(got, "; report-to csp-endpoint") {
		t.Errorf("report-to suffix missing: %q", got)
	}
}

func TestCSPHeaderName_ModeSwitch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		mode CSPMode
		want string
	}{
		{CSPOff, ""},
		{CSPStrict, "Content-Security-Policy"},
		{CSPReportOnly, "Content-Security-Policy-Report-Only"},
	}
	for _, c := range cases {
		if got := CSPHeaderName(c.mode); got != c.want {
			t.Errorf("CSPHeaderName(%d) = %q, want %q", c.mode, got, c.want)
		}
	}
}

func TestDefaultCSPDirectives_ReturnsCopy(t *testing.T) {
	t.Parallel()
	a := DefaultCSPDirectives()
	a["script-src"] = append(a["script-src"], "'unsafe-inline'")
	b := DefaultCSPDirectives()
	for _, v := range b["script-src"] {
		if v == "'unsafe-inline'" {
			t.Fatal("DefaultCSPDirectives leaked mutation across calls")
		}
	}
}

func TestCSPTemplate_BuildMatchesBuildCSPHeader(t *testing.T) {
	t.Parallel()
	cfg := CSPConfig{
		Mode: CSPStrict,
		Directives: map[string][]string{
			"img-src": {"'self'", "https://cdn.example.com"},
		},
		ReportTo: "csp-endpoint",
	}
	tpl := NewCSPTemplate(cfg)
	for _, nonce := range []string{"abc", "deadbeef", "z9z9z9z9"} {
		if got, want := tpl.Build(nonce), BuildCSPHeader(cfg, nonce); got != want {
			t.Errorf("Build(%q) = %q, want %q", nonce, got, want)
		}
	}
}

func TestBuildCSPHeader_CachedAcrossCalls(t *testing.T) {
	t.Parallel()
	cfg := CSPConfig{
		Mode:       CSPStrict,
		Directives: map[string][]string{"img-src": {"'self'", "data:"}},
		ReportTo:   "endpoint-1",
	}
	first := BuildCSPHeader(cfg, "n1")
	second := BuildCSPHeader(cfg, "n2")
	// Differ only by nonce splice; everything else must match.
	if len(first) != len(second) {
		t.Fatalf("length differs: %d vs %d (%q vs %q)", len(first), len(second), first, second)
	}
	for i := 0; i < len(first); i++ {
		if first[i] != second[i] {
			// First diff position should fall inside the script-src
			// nonce token. Allow it; everything else must agree.
			before := first[:i]
			if !strings.Contains(before, "script-src 'nonce-") {
				t.Fatalf("diff before nonce slot at %d: %q vs %q", i, first, second)
			}
			break
		}
	}
}

func TestBuildCSPHeader_ZeroAllocOnCacheHit(t *testing.T) {
	// AllocsPerRun must not run in parallel — it disables GC.
	cfg := CSPConfig{Mode: CSPStrict, ReportTo: "alloc-test"}
	BuildCSPHeader(cfg, "warmup") // prime cache
	allocs := testing.AllocsPerRun(100, func() {
		_ = BuildCSPHeader(cfg, "n0")
	})
	// Hot path: cspCacheKey builds one string, sync.Map.Load is
	// alloc-free on hit, Build does one string concat. Budget 3.
	if allocs > 3 {
		t.Errorf("hot-path allocs = %.1f, want <= 3", allocs)
	}
}
