package auth

import "errors"

// Sentinel errors returned by Auth methods. Callers should use errors.Is
// to test for these values rather than comparing strings.
var (
	// ErrNotFound is returned when a requested record does not exist.
	ErrNotFound = errors.New("auth: not found")

	// ErrConflict is returned when a uniqueness constraint would be violated
	// (e.g. email already registered).
	ErrConflict = errors.New("auth: conflict")

	// ErrInvalidCredentials is returned when the supplied password or token
	// does not match the stored credential.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")

	// ErrSessionExpired is returned when the session token exists but has
	// passed its ExpiresAt time.
	ErrSessionExpired = errors.New("auth: session expired")

	// ErrRateLimited is returned when a caller has exceeded the per-endpoint
	// rate limit.
	ErrRateLimited = errors.New("auth: rate limited")

	// ErrCSRFInvalid is returned when the CSRF double-submit token does not
	// match the cookie value.
	ErrCSRFInvalid = errors.New("auth: CSRF token invalid")

	// ErrEmailNotVerified is returned when an operation requires a verified
	// email but the user's EmailVerified flag is false.
	ErrEmailNotVerified = errors.New("auth: email not verified")

	// Err2FARequired is returned when the account has two-factor authentication
	// enabled and the caller has not yet completed the second factor.
	Err2FARequired = errors.New("auth: two-factor authentication required")

	// ErrMailerSend is the sentinel wrapped by all Mailer implementations on
	// delivery failure. Wrap with fmt.Errorf("auth/mailer: <adapter>: %w: %w",
	// ErrMailerSend, originalErr) so callers can use errors.Is(err, ErrMailerSend).
	ErrMailerSend = errors.New("auth: mailer send failed")

	// ErrSMSSend is the sentinel wrapped by all SMSSender implementations on
	// delivery failure. Wrap with fmt.Errorf("auth/sms: <adapter>: %w: %w",
	// ErrSMSSend, originalErr) so callers can use errors.Is(err, ErrSMSSend).
	ErrSMSSend = errors.New("auth: sms send failed")
)
