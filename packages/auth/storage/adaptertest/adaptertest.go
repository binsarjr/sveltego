// Package adaptertest provides a shared compliance suite for auth.Storage
// implementations. Any adapter can re-use it by calling Run from its own
// test file:
//
//	func TestMyAdapter_AdapterCompliance(t *testing.T) {
//	    adaptertest.Run(t, func() auth.Storage { return myadapter.New() })
//	}
package adaptertest

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/binsarjr/sveltego/auth"
)

// Run executes the full compliance suite against the Storage returned by factory.
// factory is called once per sub-test so each test gets a clean instance.
func Run(t *testing.T, factory func() auth.Storage) {
	t.Helper()
	t.Run("User", func(t *testing.T) { runUserTests(t, factory) })
	t.Run("Session", func(t *testing.T) { runSessionTests(t, factory) })
	t.Run("Account", func(t *testing.T) { runAccountTests(t, factory) })
	t.Run("Verification", func(t *testing.T) { runVerificationTests(t, factory) })
	t.Run("Cascade", func(t *testing.T) { runCascadeTest(t, factory) })
	t.Run("Concurrency", func(t *testing.T) { runConcurrencyTest(t, factory) })
}

// --- helpers ---

func newUser(id, email string) *auth.User {
	now := time.Now()
	return &auth.User{
		ID:        id,
		Email:     email,
		Name:      "Test User",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func newSession(id, userID, token string, expiry time.Time) *auth.Session {
	now := time.Now()
	return &auth.Session{
		ID:        id,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiry,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func newAccount(id, userID, provider string) *auth.Account {
	now := time.Now()
	return &auth.Account{
		ID:                id,
		UserID:            userID,
		Provider:          provider,
		ProviderAccountID: provider + "-remote-id",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func newVerification(id, userID, token string) *auth.Verification {
	return &auth.Verification{
		ID:        id,
		UserID:    userID,
		Kind:      "email",
		Token:     token,
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
}

// --- User tests ---

func runUserTests(t *testing.T, factory func() auth.Storage) {
	t.Helper()

	t.Run("CreateAndGetByID", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		u := newUser("u1", "alice@example.com")
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		got, err := s.UserByID(ctx, "u1")
		if err != nil {
			t.Fatalf("UserByID: %v", err)
		}
		if got.Email != u.Email {
			t.Errorf("email mismatch: got %q want %q", got.Email, u.Email)
		}
	})

	t.Run("CreateAndGetByEmail", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		u := newUser("u1", "alice@example.com")
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		got, err := s.UserByEmail(ctx, "alice@example.com")
		if err != nil {
			t.Fatalf("UserByEmail: %v", err)
		}
		if got.ID != "u1" {
			t.Errorf("id mismatch: got %q want %q", got.ID, "u1")
		}
	})

	t.Run("CreateConflict", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		u1 := newUser("u1", "alice@example.com")
		u2 := newUser("u2", "alice@example.com")
		if err := s.CreateUser(ctx, u1); err != nil {
			t.Fatalf("first CreateUser: %v", err)
		}
		err := s.CreateUser(ctx, u2)
		if !errors.Is(err, auth.ErrConflict) {
			t.Errorf("expected ErrConflict, got %v", err)
		}
	})

	t.Run("UserByIDNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		_, err := s.UserByID(ctx, "missing")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("UserByEmailNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		_, err := s.UserByEmail(ctx, "nobody@example.com")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("UpdateUser", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		u := newUser("u1", "alice@example.com")
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		u.Name = "Alice Updated"
		if err := s.UpdateUser(ctx, u); err != nil {
			t.Fatalf("UpdateUser: %v", err)
		}
		got, _ := s.UserByID(ctx, "u1")
		if got.Name != "Alice Updated" {
			t.Errorf("name not updated: got %q", got.Name)
		}
	})

	t.Run("UpdateUserNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		err := s.UpdateUser(ctx, newUser("missing", "x@example.com"))
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("UpdateUserEmailChange", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		u := newUser("u1", "old@example.com")
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		u.Email = "new@example.com"
		if err := s.UpdateUser(ctx, u); err != nil {
			t.Fatalf("UpdateUser: %v", err)
		}
		// old email must be gone
		if _, err := s.UserByEmail(ctx, "old@example.com"); !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("old email still resolves after update")
		}
		// new email must work
		if _, err := s.UserByEmail(ctx, "new@example.com"); err != nil {
			t.Errorf("new email not found after update: %v", err)
		}
	})

	t.Run("DeleteUser", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		u := newUser("u1", "alice@example.com")
		if err := s.CreateUser(ctx, u); err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if err := s.DeleteUser(ctx, "u1"); err != nil {
			t.Fatalf("DeleteUser: %v", err)
		}
		if _, err := s.UserByID(ctx, "u1"); !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("user still found after delete")
		}
	})

	t.Run("DeleteUserNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		err := s.DeleteUser(ctx, "missing")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// --- Session tests ---

func runSessionTests(t *testing.T, factory func() auth.Storage) {
	t.Helper()

	t.Run("CreateAndGetByToken", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		sess := newSession("s1", "u1", "tok-abc", time.Now().Add(time.Hour))
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		got, err := s.SessionByToken(ctx, "tok-abc")
		if err != nil {
			t.Fatalf("SessionByToken: %v", err)
		}
		if got.UserID != "u1" {
			t.Errorf("userID mismatch: got %q", got.UserID)
		}
	})

	t.Run("SessionByTokenNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		_, err := s.SessionByToken(ctx, "no-such-token")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("SessionByTokenExpired", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		sess := newSession("s1", "u1", "tok-exp", time.Now().Add(-time.Second))
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		_, err := s.SessionByToken(ctx, "tok-exp")
		if !errors.Is(err, auth.ErrSessionExpired) {
			t.Errorf("expected ErrSessionExpired, got %v", err)
		}
	})

	t.Run("RefreshSession", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		sess := newSession("s1", "u1", "tok-r", time.Now().Add(time.Hour))
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		newExpiry := time.Now().Add(24 * time.Hour)
		if err := s.RefreshSession(ctx, "tok-r", newExpiry); err != nil {
			t.Fatalf("RefreshSession: %v", err)
		}
		got, _ := s.SessionByToken(ctx, "tok-r")
		if !got.ExpiresAt.Equal(newExpiry) {
			t.Errorf("expiry not updated")
		}
	})

	t.Run("RefreshSessionNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		err := s.RefreshSession(ctx, "missing", time.Now().Add(time.Hour))
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("RevokeSessionIdempotent", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		sess := newSession("s1", "u1", "tok-rev", time.Now().Add(time.Hour))
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatalf("CreateSession: %v", err)
		}
		if err := s.RevokeSession(ctx, "tok-rev"); err != nil {
			t.Fatalf("first RevokeSession: %v", err)
		}
		// idempotent — second call must not error
		if err := s.RevokeSession(ctx, "tok-rev"); err != nil {
			t.Fatalf("second RevokeSession: %v", err)
		}
	})

	t.Run("RevokeAllSessions", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		for i := range 3 {
			sess := newSession(
				fmt.Sprintf("s%d", i),
				"u1",
				fmt.Sprintf("tok-%d", i),
				time.Now().Add(time.Hour),
			)
			if err := s.CreateSession(ctx, sess); err != nil {
				t.Fatalf("CreateSession %d: %v", i, err)
			}
		}
		if err := s.RevokeAllSessions(ctx, "u1"); err != nil {
			t.Fatalf("RevokeAllSessions: %v", err)
		}
		for i := range 3 {
			_, err := s.SessionByToken(ctx, fmt.Sprintf("tok-%d", i))
			if !errors.Is(err, auth.ErrNotFound) {
				t.Errorf("session tok-%d still present after RevokeAllSessions", i)
			}
		}
	})

	t.Run("RevokeAllSessionsIdempotent", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		// no sessions — must not error
		if err := s.RevokeAllSessions(ctx, "u-empty"); err != nil {
			t.Fatalf("RevokeAllSessions on empty: %v", err)
		}
	})
}

// --- Account tests ---

func runAccountTests(t *testing.T, factory func() auth.Storage) {
	t.Helper()

	t.Run("CreateAndList", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		a := newAccount("a1", "u1", "google")
		if err := s.CreateAccount(ctx, a); err != nil {
			t.Fatalf("CreateAccount: %v", err)
		}
		list, err := s.AccountsByUser(ctx, "u1")
		if err != nil {
			t.Fatalf("AccountsByUser: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 account, got %d", len(list))
		}
	})

	t.Run("AccountsByUserEmpty", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		list, err := s.AccountsByUser(ctx, "nobody")
		if err != nil {
			t.Fatalf("AccountsByUser: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("expected empty slice, got %d", len(list))
		}
	})

	t.Run("LinkAccount", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		a := newAccount("a1", "u1", "github")
		if err := s.LinkAccount(ctx, a); err != nil {
			t.Fatalf("LinkAccount: %v", err)
		}
		list, _ := s.AccountsByUser(ctx, "u1")
		if len(list) != 1 {
			t.Errorf("expected 1 account after LinkAccount, got %d", len(list))
		}
	})

	t.Run("UnlinkAccount", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		a := newAccount("a1", "u1", "google")
		if err := s.CreateAccount(ctx, a); err != nil {
			t.Fatalf("CreateAccount: %v", err)
		}
		if err := s.UnlinkAccount(ctx, "a1"); err != nil {
			t.Fatalf("UnlinkAccount: %v", err)
		}
		list, _ := s.AccountsByUser(ctx, "u1")
		if len(list) != 0 {
			t.Errorf("expected 0 accounts after unlink, got %d", len(list))
		}
	})

	t.Run("UnlinkAccountNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		err := s.UnlinkAccount(ctx, "missing")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// --- Verification tests ---

func runVerificationTests(t *testing.T, factory func() auth.Storage) {
	t.Helper()

	t.Run("CreateAndGet", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		v := newVerification("v1", "u1", "code-abc")
		if err := s.CreateVerification(ctx, v); err != nil {
			t.Fatalf("CreateVerification: %v", err)
		}
		got, err := s.VerificationByCode(ctx, "code-abc")
		if err != nil {
			t.Fatalf("VerificationByCode: %v", err)
		}
		if got.UserID != "u1" {
			t.Errorf("userID mismatch: got %q", got.UserID)
		}
	})

	t.Run("VerificationByCodeNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		_, err := s.VerificationByCode(ctx, "no-code")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("ConsumeVerification", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		v := newVerification("v1", "u1", "code-xyz")
		if err := s.CreateVerification(ctx, v); err != nil {
			t.Fatalf("CreateVerification: %v", err)
		}
		if err := s.ConsumeVerification(ctx, "code-xyz"); err != nil {
			t.Fatalf("ConsumeVerification: %v", err)
		}
		_, err := s.VerificationByCode(ctx, "code-xyz")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("verification still present after consume")
		}
	})

	t.Run("ConsumeVerificationNotFound", func(t *testing.T) {
		s := factory()
		ctx := context.Background()
		err := s.ConsumeVerification(ctx, "no-code")
		if !errors.Is(err, auth.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// --- Cascade test ---

func runCascadeTest(t *testing.T, factory func() auth.Storage) {
	t.Helper()

	s := factory()
	ctx := context.Background()

	u := newUser("u1", "cascade@example.com")
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	sess := newSession("s1", "u1", "tok-cascade", time.Now().Add(time.Hour))
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	acc := newAccount("a1", "u1", "google")
	if err := s.CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	ver := newVerification("v1", "u1", "code-cascade")
	if err := s.CreateVerification(ctx, ver); err != nil {
		t.Fatalf("CreateVerification: %v", err)
	}

	if err := s.DeleteUser(ctx, "u1"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	if _, err := s.SessionByToken(ctx, "tok-cascade"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("session survived DeleteUser cascade")
	}
	if list, _ := s.AccountsByUser(ctx, "u1"); len(list) != 0 {
		t.Errorf("accounts survived DeleteUser cascade: %d remaining", len(list))
	}
	if _, err := s.VerificationByCode(ctx, "code-cascade"); !errors.Is(err, auth.ErrNotFound) {
		t.Errorf("verification survived DeleteUser cascade")
	}
}

// --- Concurrency test ---

func runConcurrencyTest(t *testing.T, factory func() auth.Storage) {
	t.Helper()

	s := factory()
	ctx := context.Background()

	const goroutines = 100
	const opsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(g int) {
			defer wg.Done()
			for op := range opsPerGoroutine {
				id := fmt.Sprintf("u-%d-%d", g, op)
				email := fmt.Sprintf("user-%d-%d@example.com", g, op)
				u := newUser(id, email)
				_ = s.CreateUser(ctx, u)
				_, _ = s.UserByID(ctx, id)
				_, _ = s.UserByEmail(ctx, email)
				tok := fmt.Sprintf("tok-%d-%d", g, op)
				sess := newSession(id+"s", id, tok, time.Now().Add(time.Hour))
				_ = s.CreateSession(ctx, sess)
				_, _ = s.SessionByToken(ctx, tok)
				_ = s.RevokeSession(ctx, tok)
				_ = s.DeleteUser(ctx, id)
			}
		}(g)
	}
	wg.Wait()
}
