package auth_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/binsarjr/sveltego/auth"
)

func TestArgon2id_RoundTrip(t *testing.T) {
	h := auth.NewArgon2idHasher()
	password := "correct-horse-battery-staple"

	hashed, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash error: %v", err)
	}
	if !strings.HasPrefix(hashed, "$argon2id$") {
		t.Errorf("unexpected hash prefix: %q", hashed[:10])
	}

	ok, err := h.Verify(password, hashed)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if !ok {
		t.Error("Verify returned false for correct password")
	}
}

func TestArgon2id_VerifyRejectsTampered(t *testing.T) {
	h := auth.NewArgon2idHasher()
	hashed, err := h.Hash("secret")
	if err != nil {
		t.Fatalf("Hash error: %v", err)
	}

	ok, err := h.Verify("wrong-password", hashed)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if ok {
		t.Error("Verify returned true for wrong password")
	}
}

func TestArgon2id_NeedsUpgrade(t *testing.T) {
	// Hash with weak params (time=1, memory=16KiB, threads=1).
	weak := auth.NewArgon2idHasher(
		auth.WithTime(1),
		auth.WithMemory(16*1024),
		auth.WithThreads(1),
	)
	hashed, err := weak.Hash("password")
	if err != nil {
		t.Fatalf("Hash error: %v", err)
	}

	// Default hasher has stronger params — must report upgrade needed.
	strong := auth.NewArgon2idHasher()
	if !strong.Needs(hashed) {
		t.Error("Needs returned false for hash produced with weaker params")
	}

	// Same-params hasher must not flag upgrade.
	if weak.Needs(hashed) {
		t.Error("Needs returned true for hash produced with same params")
	}
}

func TestArgon2id_DifferentSalts(t *testing.T) {
	h := auth.NewArgon2idHasher()
	a, err := h.Hash("password")
	if err != nil {
		t.Fatalf("first Hash error: %v", err)
	}
	b, err := h.Hash("password")
	if err != nil {
		t.Fatalf("second Hash error: %v", err)
	}
	if a == b {
		t.Error("two hashes of the same password must differ (different salts)")
	}
}

func TestArgon2id_Concurrent(t *testing.T) {
	h := auth.NewArgon2idHasher(
		auth.WithTime(1),
		auth.WithMemory(16*1024),
		auth.WithThreads(1),
	)
	var wg sync.WaitGroup
	const n = 4
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = h.Hash("concurrent-password")
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Hash error: %v", i, err)
		}
	}
}
