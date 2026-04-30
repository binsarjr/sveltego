# CLAUDE.md — packages/auth

## Where things go

This package follows the ADR 0006 layout exactly. Each top-level file covers
one concern: `auth.go` (aggregate + Config + New), `identity.go` (core types),
`errors.go` (sentinels), `session.go` (token format — #220), `password.go`
(Hasher — #222), `csrf.go` (#233), `hook.go` (kit integration — #228).
Sub-packages live under named directories:

```
storage/{memory,sql,pgx,mongo,redis}/
email/{smtp,resend,sendgrid,noop}/
sms/{twilio,noop}/
provider/{google,github,...,generic}/
totp/  webauthn/  rbac/  org/  admin/  plugin/  kitform/
```

Do not create new top-level files or sub-packages without a corresponding
issue and an ADR 0006 amendment. If a feature doesn't map to an existing
file slot, open an issue first.

## Before changing public surface

Read ADR 0006 (`tasks/decisions/0006-auth-master-plan.md`) and
`STABILITY.md` before adding, renaming, or removing any exported symbol.
All symbols are **experimental** at v0.0.x, but breaking changes still
require a changelog entry. Run `go vet ./...` + `gofumpt -l .` +
`goimports -l -local github.com/binsarjr/sveltego .` after every edit.
No JS in tests — this is a pure-Go module.
