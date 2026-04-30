package auth_test

import (
	"testing"

	"github.com/binsarjr/sveltego/auth"
)

// TestNew_DefaultsHonored verifies that New fills in zero-value Config fields
// and returns a non-nil *Auth.
func TestNew_DefaultsHonored(t *testing.T) {
	a, err := auth.New(auth.Config{})
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("New returned nil *Auth")
	}
}
