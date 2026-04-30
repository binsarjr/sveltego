package env

import (
	"strings"
	"testing"
)

func TestStaticPrivate(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		t.Setenv("SVELTEGO_TEST_DB_URL", "postgres://localhost/x")
		if got := StaticPrivate("SVELTEGO_TEST_DB_URL"); got != "postgres://localhost/x" {
			t.Fatalf("StaticPrivate = %q, want postgres://localhost/x", got)
		}
	})

	t.Run("empty value still present", func(t *testing.T) {
		t.Setenv("SVELTEGO_TEST_EMPTY", "")
		if got := StaticPrivate("SVELTEGO_TEST_EMPTY"); got != "" {
			t.Fatalf("StaticPrivate = %q, want empty", got)
		}
	})

	t.Run("missing panics", func(t *testing.T) {
		assertPanics(t, "required key", func() {
			StaticPrivate("SVELTEGO_TEST_DEFINITELY_UNSET_KEY")
		})
	})
}

func TestStaticPublic(t *testing.T) {
	t.Run("present with prefix", func(t *testing.T) {
		t.Setenv("PUBLIC_API_URL", "https://api.example.com")
		if got := StaticPublic("PUBLIC_API_URL"); got != "https://api.example.com" {
			t.Fatalf("StaticPublic = %q, want https://api.example.com", got)
		}
	})

	t.Run("missing prefix panics", func(t *testing.T) {
		assertPanics(t, "PUBLIC_ prefix", func() {
			StaticPublic("SECRET_KEY")
		})
	})

	t.Run("missing value panics", func(t *testing.T) {
		assertPanics(t, "required key", func() {
			StaticPublic("PUBLIC_SVELTEGO_TEST_UNSET")
		})
	})
}

func TestDynamicPrivate(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		t.Setenv("SVELTEGO_TEST_DYN_PRIV", "v1")
		if got := DynamicPrivate("SVELTEGO_TEST_DYN_PRIV"); got != "v1" {
			t.Fatalf("DynamicPrivate = %q, want v1", got)
		}
	})

	t.Run("missing returns empty", func(t *testing.T) {
		if got := DynamicPrivate("SVELTEGO_TEST_DEFINITELY_UNSET_KEY"); got != "" {
			t.Fatalf("DynamicPrivate = %q, want empty", got)
		}
	})
}

func TestDynamicPublic(t *testing.T) {
	t.Run("present with prefix", func(t *testing.T) {
		t.Setenv("PUBLIC_FEATURE_FLAG", "on")
		if got := DynamicPublic("PUBLIC_FEATURE_FLAG"); got != "on" {
			t.Fatalf("DynamicPublic = %q, want on", got)
		}
	})

	t.Run("missing returns empty", func(t *testing.T) {
		if got := DynamicPublic("PUBLIC_SVELTEGO_TEST_UNSET"); got != "" {
			t.Fatalf("DynamicPublic = %q, want empty", got)
		}
	})

	t.Run("missing prefix panics", func(t *testing.T) {
		assertPanics(t, "PUBLIC_ prefix", func() {
			DynamicPublic("SECRET_KEY")
		})
	})
}

func assertPanics(t *testing.T, wantSubstr string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q, got none", wantSubstr)
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value not a string: %#v", r)
		}
		if !strings.Contains(msg, wantSubstr) {
			t.Fatalf("panic = %q, want substring %q", msg, wantSubstr)
		}
	}()
	fn()
}
