// Package memory provides an in-memory implementation of auth.Storage.
// It is suitable for tests and local development; data is lost on process exit.
package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/binsarjr/sveltego/auth"
)

// Store is a thread-safe in-memory auth.Storage implementation.
type Store struct {
	mu            sync.RWMutex
	users         map[string]*auth.User         // keyed by User.ID
	usersByEmail  map[string]string             // email -> User.ID
	sessions      map[string]*auth.Session      // keyed by Session.Token
	accounts      map[string]*auth.Account      // keyed by Account.ID
	verifications map[string]*auth.Verification // keyed by Verification.Token
}

// New returns a new empty Store implementing auth.Storage.
func New() *Store {
	return &Store{
		users:         make(map[string]*auth.User),
		usersByEmail:  make(map[string]string),
		sessions:      make(map[string]*auth.Session),
		accounts:      make(map[string]*auth.Account),
		verifications: make(map[string]*auth.Verification),
	}
}

// Ensure Store implements auth.Storage at compile time.
var _ auth.Storage = (*Store)(nil)

// --- User ---

// CreateUser persists u. Returns ErrConflict if the email is already taken.
func (s *Store) CreateUser(_ context.Context, u *auth.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.usersByEmail[u.Email]; exists {
		return fmt.Errorf("auth: %w", auth.ErrConflict)
	}
	clone := *u
	s.users[u.ID] = &clone
	s.usersByEmail[u.Email] = u.ID
	return nil
}

// UserByID returns the user with the given id. Returns ErrNotFound if absent.
func (s *Store) UserByID(_ context.Context, id string) (*auth.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	clone := *u
	return &clone, nil
}

// UserByEmail returns the user with the given email. Returns ErrNotFound if absent.
func (s *Store) UserByEmail(_ context.Context, email string) (*auth.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.usersByEmail[email]
	if !ok {
		return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	clone := *s.users[id]
	return &clone, nil
}

// UpdateUser replaces the stored record for u.ID. Returns ErrNotFound if absent.
func (s *Store) UpdateUser(_ context.Context, u *auth.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.users[u.ID]
	if !ok {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	// Remove old email index if email changed.
	if existing.Email != u.Email {
		delete(s.usersByEmail, existing.Email)
		s.usersByEmail[u.Email] = u.ID
	}
	clone := *u
	s.users[u.ID] = &clone
	return nil
}

// DeleteUser removes the user and cascades to sessions, accounts, and verifications.
// Returns ErrNotFound if the user does not exist.
func (s *Store) DeleteUser(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	delete(s.usersByEmail, u.Email)
	delete(s.users, id)
	for token, sess := range s.sessions {
		if sess.UserID == id {
			delete(s.sessions, token)
		}
	}
	for aid, acc := range s.accounts {
		if acc.UserID == id {
			delete(s.accounts, aid)
		}
	}
	for code, v := range s.verifications {
		if v.UserID == id {
			delete(s.verifications, code)
		}
	}
	return nil
}

// --- Session ---

// CreateSession persists s.
func (s *Store) CreateSession(_ context.Context, sess *auth.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *sess
	s.sessions[sess.Token] = &clone
	return nil
}

// SessionByToken returns the session for the given token. Returns ErrNotFound
// if absent, or ErrSessionExpired if the session is past its ExpiresAt.
func (s *Store) SessionByToken(_ context.Context, token string) (*auth.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[token]
	if !ok {
		return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, fmt.Errorf("auth: %w", auth.ErrSessionExpired)
	}
	clone := *sess
	return &clone, nil
}

// RefreshSession extends the expiry of the session identified by token.
// Returns ErrNotFound if absent.
func (s *Store) RefreshSession(_ context.Context, token string, newExpiry time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	sess.ExpiresAt = newExpiry
	sess.UpdatedAt = time.Now()
	return nil
}

// RevokeSession deletes the session identified by token. Idempotent.
func (s *Store) RevokeSession(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
	return nil
}

// RevokeAllSessions deletes every session belonging to userID. Idempotent.
func (s *Store) RevokeAllSessions(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, token)
		}
	}
	return nil
}

// --- Account ---

// CreateAccount persists a.
func (s *Store) CreateAccount(_ context.Context, a *auth.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *a
	s.accounts[a.ID] = &clone
	return nil
}

// AccountsByUser returns all accounts belonging to userID.
func (s *Store) AccountsByUser(_ context.Context, userID string) ([]*auth.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*auth.Account
	for _, a := range s.accounts {
		if a.UserID == userID {
			clone := *a
			out = append(out, &clone)
		}
	}
	if out == nil {
		out = []*auth.Account{}
	}
	return out, nil
}

// LinkAccount is an alias of CreateAccount documenting provider-linking intent.
func (s *Store) LinkAccount(ctx context.Context, a *auth.Account) error {
	return s.CreateAccount(ctx, a)
}

// UnlinkAccount removes the account with the given accountID.
// Returns ErrNotFound if absent.
func (s *Store) UnlinkAccount(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.accounts[accountID]; !ok {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	delete(s.accounts, accountID)
	return nil
}

// --- Verification ---

// CreateVerification persists v.
func (s *Store) CreateVerification(_ context.Context, v *auth.Verification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *v
	s.verifications[v.Token] = &clone
	return nil
}

// VerificationByCode returns the Verification identified by code.
// Returns ErrNotFound if absent.
func (s *Store) VerificationByCode(_ context.Context, code string) (*auth.Verification, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.verifications[code]
	if !ok {
		return nil, fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	clone := *v
	return &clone, nil
}

// ConsumeVerification deletes the Verification identified by code.
// Returns ErrNotFound if absent.
func (s *Store) ConsumeVerification(_ context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.verifications[code]; !ok {
		return fmt.Errorf("auth: %w", auth.ErrNotFound)
	}
	delete(s.verifications, code)
	return nil
}
