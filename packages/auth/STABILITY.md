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
  `ErrRateLimited`, `ErrCSRFInvalid`, `ErrEmailNotVerified`, `Err2FARequired`
  — sentinel errors.

## Deprecated

(none yet)

## Internal-only (do not import even though exported)

(none yet)
