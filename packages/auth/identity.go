package auth

import "time"

// User represents an authenticated principal. Fields mirror the better-auth
// shape (ADR 0006) so storage adapters can target a common schema. Storage
// logic lives in the storage sub-packages (#217–#219).
type User struct {
	// ID is the unique identifier for the user (UUIDv7 by default).
	ID string

	// Email is the user's primary email address.
	Email string

	// EmailVerified indicates whether the email has been confirmed.
	EmailVerified bool

	// Name is the user's display name.
	Name string

	// Image is the URL of the user's avatar.
	Image string

	// CreatedAt is when the user record was created.
	CreatedAt time.Time

	// UpdatedAt is when the user record was last modified.
	UpdatedAt time.Time
}

// Account links a User to an external OAuth provider or a credential method
// (e.g. "email" for password-based logins). One User may have many Accounts.
type Account struct {
	// ID is the unique identifier for the account record.
	ID string

	// UserID is the foreign key back to User.ID.
	UserID string

	// Provider is the provider identifier (e.g. "email", "google", "github").
	Provider string

	// ProviderAccountID is the provider-side user identifier.
	ProviderAccountID string

	// AccessToken is the OAuth access token (populated for OAuth accounts).
	AccessToken string

	// RefreshToken is the OAuth refresh token.
	RefreshToken string

	// ExpiresAt is when the OAuth access token expires.
	ExpiresAt *time.Time

	// CreatedAt is when the account record was created.
	CreatedAt time.Time

	// UpdatedAt is when the account record was last modified.
	UpdatedAt time.Time
}

// Session represents an active authentication session. DB-backed sessions
// store a hashed token; stateless sessions encode this struct into a signed
// encrypted cookie (see ADR 0006 §Session Strategies, #220–#221).
type Session struct {
	// ID is the unique session identifier.
	ID string

	// UserID is the foreign key to User.ID.
	UserID string

	// Token is the raw session token (only present immediately after creation;
	// stored as a hash in the DB).
	Token string

	// ExpiresAt is when this session becomes invalid.
	ExpiresAt time.Time

	// FreshUntil is when the session is considered "fresh" for sensitive ops.
	// After this time, re-authentication may be required.
	FreshUntil time.Time

	// IPAddress is the IP that created the session (for display in session list).
	IPAddress string

	// UserAgent is the User-Agent header at session creation time.
	UserAgent string

	// CreatedAt is when the session was created.
	CreatedAt time.Time

	// UpdatedAt is when the session was last refreshed.
	UpdatedAt time.Time
}

// Verification is a short-lived record used for email verification,
// magic links, OTP codes, and password-reset flows.
type Verification struct {
	// ID is the unique identifier for the verification record.
	ID string

	// UserID is the foreign key to User.ID.
	UserID string

	// Kind identifies the verification type (e.g. "email", "magic-link", "otp", "password-reset").
	Kind string

	// Token is the raw verification token (stored hashed in the DB).
	Token string

	// ExpiresAt is when the verification record becomes invalid.
	ExpiresAt time.Time

	// CreatedAt is when the verification record was created.
	CreatedAt time.Time
}
