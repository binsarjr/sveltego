package auth

import (
	"log/slog"
	"time"
)

// Config holds all options for the Auth aggregate. Fields marked "populated by
// sub-issues" are reserved placeholders — they will be typed interfaces once
// the relevant issues land (#217–#221, #223, etc.). Do not remove them.
type Config struct {
	// BaseURL is the canonical origin of the application (e.g. "https://example.com").
	BaseURL string

	// BasePath is the URL prefix for all auth endpoints. Defaults to "/auth".
	BasePath string

	// Logger is the structured logger. Defaults to slog.Default().
	Logger *slog.Logger

	// Now returns the current time. Defaults to time.Now. Override in tests.
	Now func() time.Time

	// Storage is the persistence adapter. auth.New panics if this is nil.
	Storage Storage

	// Mailer is the email delivery adapter. A nil Mailer disables all
	// email-dependent flows (verification, password-reset, magic-link, OTP).
	// Use auth/mailer/smtp, auth/mailer/resend, or auth/mailer/sendgrid for
	// production; use auth.NewNoopMailer() in tests and development.
	Mailer Mailer

	// SMS is the SMS delivery adapter. A nil SMS disables all SMS-dependent
	// flows (OTP via SMS, SMS two-factor authentication). Use auth/sms/twilio
	// for production; use auth.NewNoopSMSSender() in tests and development.
	SMS SMSSender

	// Plugins holds optional capability extensions (populated by #224+).
	Plugins []any

	// Hasher is the password hashing implementation. If nil, NewArgon2idHasher()
	// is used as the default (Argon2id with OWASP-recommended parameters).
	Hasher Hasher

	// CSRF holds the double-submit CSRF protection configuration. If nil, CSRF
	// protection is not automatically applied; callers must wrap handlers manually.
	CSRF *CSRF

	// EnableRateLimit enables per-endpoint rate limiting (populated by a follow-up issue).
	EnableRateLimit bool
}

// Auth is the central aggregate for all authentication and authorization
// operations. Construct one per application via New and pass it into
// sveltego kit hooks. See ADR 0006 for the full method surface.
type Auth struct {
	cfg Config
}

// New constructs an Auth with the given Config, applying defaults for
// any zero-value fields. It returns a non-nil *Auth and nil error on
// success; future validation (e.g. missing BaseURL) may return an error.
func New(cfg Config) (*Auth, error) {
	if cfg.Storage == nil {
		panic("auth: Config.Storage must not be nil — auth without storage is meaningless")
	}
	if cfg.BasePath == "" {
		cfg.BasePath = "/auth"
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Hasher == nil {
		cfg.Hasher = NewArgon2idHasher()
	}
	return &Auth{cfg: cfg}, nil
}
