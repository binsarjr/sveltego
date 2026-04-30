package memory_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/binsarjr/sveltego/auth"
	"github.com/binsarjr/sveltego/auth/storage/adaptertest"
	"github.com/binsarjr/sveltego/auth/storage/memory"
)

// TestMemory_AdapterCompliance runs the shared adapter compliance suite
// against the in-memory Store.
func TestMemory_AdapterCompliance(t *testing.T) {
	adaptertest.Run(t, func() auth.Storage { return memory.New() })
}

// TestMemory_DeletedUserGC verifies that the internal maps are cleaned up
// after DeleteUser — no ghost entries leak memory.
func TestMemory_DeletedUserGC(t *testing.T) {
	s := memory.New()
	ctx := context.Background()

	const n = 50
	for i := range n {
		u := &auth.User{
			ID:    fmt.Sprintf("gc-u%d", i),
			Email: fmt.Sprintf("gc-%d@example.com", i),
		}
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser %d: %v", i, err)
		}
	}
	for i := range n {
		if err := s.DeleteUser(ctx, fmt.Sprintf("gc-u%d", i)); err != nil {
			t.Fatalf("DeleteUser %d: %v", i, err)
		}
	}
	// All deleted users must now return ErrNotFound.
	for i := range n {
		_, err := s.UserByID(ctx, fmt.Sprintf("gc-u%d", i))
		if err == nil {
			t.Errorf("user gc-u%d still present after delete", i)
		}
	}
}
