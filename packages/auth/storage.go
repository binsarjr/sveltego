package auth

import (
	"context"
	"time"
)

// Storage is the persistence contract every auth adapter must satisfy.
// All methods receive a context for cancellation and tracing. Sentinel
// errors (ErrNotFound, ErrConflict, ErrSessionExpired) must be wrapped
// with fmt.Errorf("auth: %w", sentinel) so callers can use errors.Is.
type Storage interface {
	// CreateUser persists a new User. Returns ErrConflict if a user with
	// the same Email already exists.
	CreateUser(ctx context.Context, u *User) error

	// UserByID returns the User with the given id. Returns ErrNotFound if
	// no such user exists.
	UserByID(ctx context.Context, id string) (*User, error)

	// UserByEmail returns the User with the given email address. Returns
	// ErrNotFound if no such user exists.
	UserByEmail(ctx context.Context, email string) (*User, error)

	// UpdateUser persists changes to an existing User record. Returns
	// ErrNotFound if the user does not exist.
	UpdateUser(ctx context.Context, u *User) error

	// DeleteUser removes the User and cascades the deletion to all
	// associated Sessions, Accounts, and Verifications. Returns ErrNotFound
	// if the user does not exist.
	DeleteUser(ctx context.Context, id string) error

	// CreateSession persists a new Session.
	CreateSession(ctx context.Context, s *Session) error

	// SessionByToken returns the Session identified by the given token.
	// Returns ErrNotFound if no session with that token exists, or
	// ErrSessionExpired if the session exists but its ExpiresAt is in the past.
	SessionByToken(ctx context.Context, token string) (*Session, error)

	// RefreshSession extends the expiry of the session identified by token
	// to newExpiry. Returns ErrNotFound if no such session exists.
	RefreshSession(ctx context.Context, token string, newExpiry time.Time) error

	// RevokeSession deletes the session identified by token. Idempotent:
	// returns nil if the session does not exist.
	RevokeSession(ctx context.Context, token string) error

	// RevokeAllSessions deletes every session belonging to userID. Idempotent.
	RevokeAllSessions(ctx context.Context, userID string) error

	// CreateAccount persists a new Account linking a User to a provider.
	CreateAccount(ctx context.Context, a *Account) error

	// AccountsByUser returns all Accounts belonging to the given userID.
	// Returns an empty slice (not ErrNotFound) when the user has no accounts.
	AccountsByUser(ctx context.Context, userID string) ([]*Account, error)

	// LinkAccount is an alias of CreateAccount that documents the caller's
	// intent to associate an external provider with an existing User.
	LinkAccount(ctx context.Context, a *Account) error

	// UnlinkAccount removes the Account with the given accountID. Returns
	// ErrNotFound if the account does not exist.
	UnlinkAccount(ctx context.Context, accountID string) error

	// CreateVerification persists a new Verification record.
	CreateVerification(ctx context.Context, v *Verification) error

	// VerificationByCode returns the Verification identified by code.
	// Returns ErrNotFound if the code does not exist.
	VerificationByCode(ctx context.Context, code string) (*Verification, error)

	// ConsumeVerification marks a Verification as used by deleting it.
	// Returns ErrNotFound if the code does not exist.
	ConsumeVerification(ctx context.Context, code string) error
}
