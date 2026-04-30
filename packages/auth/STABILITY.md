# Stability — auth

Last updated: 2026-04-30 · Version: pre-alpha (v0.0.x)

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97) and
[ADR 0006](../../tasks/decisions/0006-auth-master-plan.md). Every public
symbol below is **experimental** until the package reaches v0.6 stable.
Breaking changes require only a changelog entry and a minor-version bump
while the major version is 0.

## Stable

(none yet)

## Experimental

- `Auth` — central aggregate; construct via `New`.
- `Config` — all fields; populated incrementally by sub-issues #217–#234.
- `New(Config) (*Auth, error)` — constructor; defaults enforced.
- `User` — identity record.
- `Account` — provider link record.
- `Session` — active session record.
- `Verification` — short-lived verification record.
- `ErrNotFound`, `ErrConflict`, `ErrInvalidCredentials`, `ErrSessionExpired`,
  `ErrRateLimited`, `ErrCSRFInvalid`, `ErrEmailNotVerified`, `Err2FARequired`,
  `ErrMailerSend`, `ErrSMSSend` — sentinel errors.
- `Mailer` — interface for email delivery adapters; nil disables email flows.
- `Email` — message struct passed to Mailer.Send.
- `NoopMailer` — in-memory recording adapter for tests/dev; construct via `NewNoopMailer`.
- `NewNoopMailer() *NoopMailer` — constructor.
- `SMSSender` — interface for SMS delivery adapters; nil disables SMS flows.
- `SMSRecord` — holds the To/Body of a recorded NoopSMSSender call.
- `NoopSMSSender` — in-memory recording adapter for tests/dev; construct via `NewNoopSMSSender`.
- `NewNoopSMSSender() *NoopSMSSender` — constructor.
- `Hasher` — interface for pluggable password hashing backends.
- `Argon2idHasher` — Argon2id (RFC 9106) implementation of Hasher; construct via `NewArgon2idHasher`.
- `NewArgon2idHasher(...Argon2idOption) *Argon2idHasher` — constructor.
- `Argon2idOption` — functional option type for `NewArgon2idHasher`.
- `WithTime(uint32)`, `WithMemory(uint32)`, `WithThreads(uint8)` — Argon2idOption constructors.
- `CSRF` — double-submit cookie + trusted-origin CSRF protection; construct via `NewCSRF`.
- `NewCSRF(...CSRFOption) *CSRF` — constructor.
- `CSRFOption` — functional option type for `NewCSRF`.
- `WithCSRFCookieName`, `WithCSRFHeaderName`, `WithCSRFFieldName`, `WithTrustedOrigins`,
  `WithCSRFTokenSize`, `WithCSRFAllowInsecure` — CSRFOption constructors.

### Subpackage: `auth/mailer/smtp`

- `Mailer` — net/smtp STARTTLS adapter; construct via `New`.
- `New(host, port, username, password, ...Option) *Mailer` — constructor.
- `WithTLSConfig`, `WithTimeout`, `WithFrom` — functional options.

### Subpackage: `auth/mailer/resend`

- `Mailer` — Resend HTTP API adapter; construct via `New`.
- `New(apiKey, ...Option) *Mailer` — constructor.
- `WithHTTPClient`, `WithBaseURL`, `WithFrom` — functional options.

### Subpackage: `auth/mailer/sendgrid`

- `Mailer` — SendGrid v3 HTTP API adapter; construct via `New`.
- `New(apiKey, ...Option) *Mailer` — constructor.
- `WithHTTPClient`, `WithBaseURL`, `WithFrom` — functional options.

### Subpackage: `auth/sms/twilio`

- `Sender` — Twilio Programmable SMS adapter; construct via `New`.
- `New(accountSID, authToken, fromNumber, ...Option) *Sender` — constructor.
- `WithHTTPClient`, `WithBaseURL` — functional options.

### Subpackage: `auth/storage/sql`

- `Store` — `database/sql`-backed auth.Storage implementation; construct via `New`.
- `New(db *sql.DB, dialect Dialect) *Store` — constructor.
- `Dialect` — enum identifying the target database engine (Postgres, MySQL, SQLite).
- `Schema(dialect Dialect) string` — returns the DDL to create the auth tables for the given dialect.

### Subpackage: `auth/storage/pgx`

- `Store` — native pgx v5 auth.Storage implementation for PostgreSQL; construct via `New`.
- `New(pool *pgxpool.Pool) *Store` — constructor.

## Deprecated

(none yet)

## Internal-only (do not import even though exported)

(none yet)
