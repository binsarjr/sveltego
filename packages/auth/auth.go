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

	// Storage is the persistence adapter (populated by #217–#219).
	// Type will become a typed interface once the storage package lands.
	Storage any

	// Mailer is the email delivery adapter (populated by the email-provider issues).
	Mailer any

	// SMS is the SMS delivery adapter (populated by the sms-provider issues).
	SMS any

	// Plugins holds optional capability extensions (populated by #224+).
	Plugins []any

	// EnableCSRF enables double-submit CSRF protection on mutating auth endpoints.
	// Defaults to true once csrf.go lands (#233).
	EnableCSRF bool

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
	if cfg.BasePath == "" {
		cfg.BasePath = "/auth"
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Auth{cfg: cfg}, nil
}
