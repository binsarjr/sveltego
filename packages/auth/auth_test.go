package auth_test

import (
	"testing"

	"github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/storage/memory"
)

// TestNew_DefaultsHonored verifies that New fills in zero-value Config fields
// and returns a non-nil *Auth.
func TestNew_DefaultsHonored(t *testing.T) {
	a, err := auth.New(auth.Config{Storage: memory.New()})
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("New returned nil *Auth")
	}
}

// TestNew_PanicsWithoutStorage verifies that New panics when Storage is nil.
func TestNew_PanicsWithoutStorage(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil Storage, got none")
		}
	}()
	_, _ = auth.New(auth.Config{}) //nolint:errcheck
}
