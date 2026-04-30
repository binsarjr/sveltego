# ADR 0006 — sveltego-auth Master Plan

- **Status:** Accepted
- **Date:** 2026-04-30
- **Authors:** binsarjr, orchestrator
- **Issue:** [binsarjr/sveltego#155](https://github.com/binsarjr/sveltego/issues/155)
- **Milestone:** v0.6 — Authentication

## Context

Authentication is the next major capability the sveltego ecosystem needs after form actions ([#30]) and the kit hooks group ([#26], [#27], [#80], [#81]) shipped in v0.2. The framework is a Go-native rewrite of SvelteKit's shape (ADR 0001, ADR 0002), so the auth surface must be Go-native too — no JS runtime on the request path, no embedded interpreters, no `node_modules`.

### Why a first-party library

Three options were on the table at the start of v0.6 planning:

1. **Recommend an existing Go auth library** (e.g. `keratin/authn`, `volatiletech/authboss`, `markbates/goth`).
2. **Federate to ory/kratos or authelia** — run a separate identity service, talk to it over HTTP/gRPC.
3. **Build a first-party library** at `packages/auth/` that targets the framework's own primitives.

The first two were rejected for the reasons captured in *Alternatives Considered* below. The decision: ship a first-party library that mirrors the DX bar set by [`better-auth/better-auth`](https://github.com/better-auth/better-auth) — a single import that covers email-password, magic link, OAuth, 2FA, passkey, RBAC, organizations, anonymous sessions, and admin — but written in idiomatic Go and integrated with `kit.HandleFn`, `kit.Cookies`, `kit.Locals`, and `kitform.Action`.

### What better-auth's DX bar looks like

The reference DX (translated to Go shape) is:

```go
auth, err := auth.New(auth.Config{
    BaseURL: "https://example.com",
    Secret:  os.Getenv("AUTH_SECRET"),
    Storage: pgxstore.New(pool),
    Mailer:  smtp.New(smtpCfg),
    Plugins: []auth.Plugin{
        emailpassword.Plugin(),
        magiclink.Plugin(),
        totp.Plugin(),
        passkey.Plugin(),
        oauth.Google(googleCfg),
        oauth.GitHub(githubCfg),
    },
})
```

One config object, one `New`, plugins compose. Sign-up to a new app should fit in 30 lines including imports. Anything more is a regression against the DX target.

### Sveltego primitives we leverage

The library does not invent its own request layer. It rides on what `kit` already exposes:

- **`kit.HandleFn`** ([#26]) — `auth.Mount` returns a `kit.HandleFn` that owns `/auth/*` routes; `auth.PopulateLocals` returns a `kit.HandleFn` that loads the session and writes `User` and `Session` into `kit.Locals` for downstream pages.
- **`kit.Cookies`** ([#28]) — session and CSRF tokens use the existing typed cookie surface; no parallel cookie helper.
- **`kit.Locals`** ([#27]) — typed accessors `auth.UserFrom(locals)` / `auth.SessionFrom(locals)` so pages and `Load()` functions read auth state without importing storage.
- **`kit.RedirectError` / `kit.HTTPError`** ([#283]) — auth flows surface 302 redirects and typed 401/403 the same way every other handler does.
- **`kitform.Action`** ([#30]) — `kitform.SignInEmail(auth)`, `kitform.SignUpEmail(auth)`, `kitform.RequestReset(auth)`, `kitform.ConfirmReset(auth)`, `kitform.VerifyTOTP(auth)` ship as `kit.ActionMap` entries so a sign-in page is one line of `Actions`.
- **`kit.RouteID` / `kit.Reroute`** — `auth.Require(role|perm)` gates by route or by predicate; reroute lets unauthenticated users hit `/login` without losing their original target.

The result: `packages/auth/` is composable, not parallel. Same hook chain, same cookies, same form-action shape.

## Decision

Ship a single Go module at `packages/auth/` (precedent: `packages/lsp`, `packages/mcp`). Import path `github.com/binsarjr/sveltego/packages/auth`.

The public surface centres on three identifiers:

- `auth.Auth` — the live aggregate, holding storage, mailer, sms, plugins, hooks, sessions, hasher, csrf, limiter.
- `auth.Config` — input struct for `New`. Ships defaults; only `BaseURL`, `Secret`, and `Storage` are required.
- `auth.New(cfg auth.Config) (*auth.Auth, error)` — constructor that validates config, runs each plugin's `Setup`, returns the aggregate.

Subpackages:

- `auth/storage/{memory,sql,pgx,mongo,redis}` — storage adapters; `memory` is the canonical reference, `sql` covers Postgres/MySQL/SQLite via `database/sql`, `pgx` is the high-performance Postgres path, `mongo` and `redis` round out the matrix. `redis` is **secondary-cache** only — it requires a primary durable adapter (`pgx`, `sql`, `mongo`) underneath. See [#252].
- `auth/oauth/{google,github,apple,discord,microsoft,facebook,twitter,gitlab,bitbucket,twitch,linkedin,spotify,reddit,generic}` — per-provider plugins on a shared OIDC base. See [#232]–[#240].
- `auth/totp` — TOTP 2FA + backup codes ([#230]).
- `auth/passkey` — WebAuthn / passkey via `go-webauthn/webauthn` ([#231]).
- `auth/admin` — list / ban / impersonate / revoke ([#250]).
- `auth/org` — organizations + members + invitations + teams ([#249]).
- `auth/plugin` — `Plugin` interface; concrete plugins import this, not the parent module, to keep the dependency arrow pointing inward.
- `auth/kitform` — `kit.Action` adapters for each public flow ([#247]).
- `auth/kithook` — `kit.HandleFn` adapters: `Mount`, `PopulateLocals`, `Require` ([#246]).
- `auth/storage/adaptertest` — conformance suite every adapter re-runs ([#217]).

Stability tier markings live in `packages/auth/STABILITY.md` per [#255]. The v0.6 release is **experimental**; v1.0 promotes the sub-surfaces that have not changed shape for two minor versions.

## Architecture

### Package layout

The shape from [#155], expanded with the per-file responsibility every implementation phase will respect:

```
packages/auth/
  auth.go              Auth aggregate, Config, New, lifecycle (Close)
  identity.go          User, Account, Session, Verification value types
  session.go           Token format, encode/decode, freshness, rotation
  password.go          Hasher interface + argon2id default
  csrf.go              Double-submit token issue + verify
  hook.go              kit.HandleFn glue (re-exported via auth/kithook)
  errors.go            Typed error sentinels (ErrUnauthenticated, ErrInvalidCredentials, …)
  storage/
    storage.go         Storage interface (typed namespaces; see below)
    memory/            In-memory reference impl + conformance harness
    sql/               database/sql adapter (Postgres, MySQL, SQLite drivers)
    pgx/               Native Postgres on jackc/pgx
    mongo/             go.mongodb.org/mongo-driver
    redis/             Secondary-cache wrapper (sessions, blocklist, rate-limit)
    adaptertest/       Conformance suite reused by every adapter
  email/
    mailer.go          Mailer interface
    smtp/              net/smtp + gomail
    resend/            Resend HTTP API
    sendgrid/          Sendgrid HTTP API
    noop/              Drops mail; logs at DEBUG; for tests
  sms/
    sms.go             SMSSender interface
    twilio/            Twilio HTTP API
    noop/              Drops SMS; logs at DEBUG
  oauth/
    oauth.go           Provider interface, OIDC base, PKCE S256, signed state
    google/, github/, apple/, discord/, microsoft/, facebook/, twitter/,
    gitlab/, bitbucket/, twitch/, linkedin/, spotify/, reddit/, generic/
  totp/
    totp.go            Enroll, Confirm, Verify, BackupCodes
  passkey/
    passkey.go         BeginRegistration / FinishRegistration / BeginLogin / FinishLogin
  rbac/
    rbac.go            AccessControl, Role, Permission, Require predicate
  org/
    org.go             Organization, Member, Invitation, Team
  admin/
    admin.go           ListUsers, BanUser, ImpersonateUser, RevokeSessions
  plugin/
    plugin.go          Plugin interface, Hook event slots
  kithook/
    kithook.go         Mount, PopulateLocals, Require — kit.HandleFn adapters
  kitform/
    kitform.go         Action helpers for sign-in/up/reset/2FA-verify/passkey
  ratelimit/
    limiter.go         Limiter interface; memory + redis impls
  audit/
    audit.go           Audit-log Plugin sample (also serves as plugin reference)
  STABILITY.md         Per-surface tier markings
  examples/
    full/              End-to-end app for acceptance test
```

The dependency arrow always points inward: `kithook` and `kitform` depend on the core `auth` aggregate; the core depends on storage, mailer, sms, plugin interfaces — not on concrete adapters. Concrete adapters (`storage/pgx`, `email/smtp`, `oauth/google`) live in their own subpackages so a user who imports only `email-password + memory` does not pull in `pgx`, Twilio, or Google's OAuth client.

### Composition: Plugin interface + slot-based hooks

Plugins extend the aggregate without modifying core. The interface ([#251]):

```go
type Plugin interface {
    Name() string
    Setup(*Auth) error
    Hooks() []Hook
    HTTPRoutes() []Route
}

type Hook struct {
    Event  Event   // BeforeSignIn, AfterSignIn, BeforeSignUp, AfterSignUp,
                   // OnSessionRevoke, OnPasswordChange, OnEmailVerify, …
    Order  int
    Run    func(ctx *HookCtx) error
}

type Route struct {
    Method  string
    Path    string                 // relative to /auth, e.g. "/sign-in/email"
    Handler kit.HandleFn
}
```

Each plugin's `Setup` runs once at `New` time and may register storage migrations, sentinel cookies, or extra config validators. `Hooks()` returns event-slot subscriptions; the aggregate sorts by `Order`, runs them sequentially, and aborts on the first non-nil error (with a typed `HookAbortError` so the caller can distinguish hook failures from storage failures). `HTTPRoutes()` is what `Mount` consumes to wire `/auth/*` endpoints.

Built-in plugins compose this way too — there is no privileged path. `emailpassword.Plugin()` is the same shape as a third-party `audit.Plugin()`. This is the *open-closed* property the better-auth plugin model gets right and we mirror.

### Session strategy

Two strategies, configurable at `auth.Config.Session.Strategy`:

1. **DB-backed (default)** ([#220]). 32-byte random token; client stores the raw token; storage stores its `sha256` digest, never the raw token. Lookup hashes the cookie value, queries `Sessions().Get(hashed)`. 7-day expiry with 1-day freshness window. Rotation on privilege escalation (sign-in after sign-up, MFA passed, password changed). Cost: one storage round-trip per request that touches auth.

2. **Stateless encrypted cookie** ([#221]). AES-256-GCM with a key derived from `Config.Secret`. Cookie carries `User.ID`, `Session.ID`, `IssuedAt`, `Expires`, `MFA`, `Roles`. No DB read on the request path. Optional Redis blocklist for revocation; without the blocklist, a stolen cookie is valid until expiry — the trade-off is documented and `auth.New` warns when stateless is paired with no blocklist.

Both strategies expose the same `Session` value type and the same `auth.SessionFrom(locals)` accessor. Switching strategies is a config flip, never an API rewrite. Stateless is appropriate for read-heavy edge deployments (Cloudflare adapter, ADR 0005); DB-backed is the safe default everywhere else.

### Storage interface

The interface lives at `packages/auth/storage/storage.go` ([#217]):

```go
type Storage interface {
    Users() UserStore
    Accounts() AccountStore
    Sessions() SessionStore
    Verifications() VerificationStore
    Passkeys() PasskeyStore        // optional; nil-able for adapters that don't implement
    TOTP() TOTPStore               // optional
    Orgs() OrgStore                // optional
    Tx(ctx context.Context, fn func(Storage) error) error
    Close() error
}

type UserStore interface {
    Create(ctx context.Context, u User) error
    Get(ctx context.Context, id string) (User, error)
    GetByEmail(ctx context.Context, email string) (User, error)
    Update(ctx context.Context, u User) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, q ListQuery) ([]User, error)
}

type SessionStore interface {
    Create(ctx context.Context, s Session) error
    Get(ctx context.Context, hashedToken string) (Session, error)
    ListByUser(ctx context.Context, userID string) ([]Session, error)
    Update(ctx context.Context, s Session) error
    Revoke(ctx context.Context, id string) error
    RevokeAllByUser(ctx context.Context, userID string) error
    PurgeExpired(ctx context.Context, before time.Time) (int, error)
}

type AccountStore interface {
    Create(ctx context.Context, a Account) error
    Get(ctx context.Context, provider, providerAccountID string) (Account, error)
    ListByUser(ctx context.Context, userID string) ([]Account, error)
    Delete(ctx context.Context, id string) error
}

type VerificationStore interface {
    Create(ctx context.Context, v Verification) error
    Consume(ctx context.Context, token string) (Verification, error)
    PurgeExpired(ctx context.Context, before time.Time) (int, error)
}
```

`Tx` is best-effort: adapters that cannot transactionalise (`memory`, plain Redis) return `auth.ErrTxUnsupported` which higher-level flows treat as a hint to fall back to compensating writes. The conformance suite at `auth/storage/adaptertest` exercises every method with the same fixtures and runs against every adapter under build tag `-tags=integration` via `testcontainers-go`. A new adapter passes when `adaptertest.Run(t, factory)` returns clean.

Typed namespaces beat a single `Create[T]` generic surface here because per-table indexes and queries (`GetByEmail`, `ListByUser`) deserve to live with the adapter. Generic dispatch hides them behind a runtime cast, costs us optimised queries, and makes adapter authors guess at the right indexes.

## Integration with sveltego

Authentication mounts into the existing `kit.HandleFn` chain. A user wires it once, in `hooks.server.go`:

```go
//go:build sveltego

package src

import (
    "myapp/src/lib/server/authmod"
    "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
    "github.com/binsarjr/sveltego/packages/auth/kithook"
)

var Auth = authmod.New()

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
    return kit.Sequence(
        kithook.Mount(Auth),
        kithook.PopulateLocals(Auth),
    )(ev, resolve)
}
```

`kithook.Mount(Auth)` matches `/auth/*` and routes to plugin-supplied handlers. `kithook.PopulateLocals(Auth)` runs on every request, loads the session if any, writes `User` and `Session` into `Locals` so pages and `Load()` see them via `auth.UserFrom(ev.Locals)` / `auth.SessionFrom(ev.Locals)`.

Protected routes use `kithook.Require`:

```go
func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
    return kit.Sequence(
        kithook.Mount(Auth),
        kithook.PopulateLocals(Auth),
        kithook.Require(Auth, "/dashboard/*", kithook.SignedIn),
        kithook.Require(Auth, "/admin/*", kithook.HasRole("admin")),
    )(ev, resolve)
}
```

`kithook.SignedIn` and `kithook.HasRole` are predicates returning `bool`; users compose their own. `Require` issues `kit.RedirectError(303, "/login?next=...")` for anonymous users and `kit.HTTPError(403)` for authenticated-but-unauthorised.

Form actions ([#247]):

```go
//go:build sveltego

package login

import (
    "myapp/src"
    "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
    "github.com/binsarjr/sveltego/packages/auth/kitform"
)

var Actions = kit.ActionMap{
    "default": kitform.SignInEmail(src.Auth, kitform.SignInOptions{
        RedirectTo: "/dashboard",
    }),
}
```

The action handler reads the form, calls `Auth.SignInEmail(ctx, email, password)`, sets the session cookie, and returns `kit.Redirect(RedirectTo)`. Field validation errors come back as `kit.ActionFailure` with the existing tagged-error-map shape ([#21] / ADR 0005's form-validation note). No new pattern, no parallel surface.

## Plugin model — built-ins

Each line is the one-sentence responsibility:

- **`emailpassword`** — sign-up, sign-in, sign-out, change password ([#223]).
- **`magiclink`** — email-delivered single-use sign-in token ([#228]).
- **`otp`** — six-digit code over email or SMS, single-use, short TTL ([#229]).
- **`totp`** — RFC 6238 TOTP enroll + verify, backup codes ([#230]).
- **`passkey`** — WebAuthn registration and assertion via `go-webauthn/webauthn` ([#231]).
- **`oauth-*`** (Google, GitHub, Apple, Discord, Microsoft, Facebook, Twitter, GitLab, Bitbucket, Twitch, LinkedIn, Spotify, Reddit) — per-provider OAuth2/OIDC on a shared base with PKCE S256 ([#232]–[#239]).
- **`oauth-generic`** — bring-your-own OIDC issuer ([#240]).
- **`csrf`** — double-submit token middleware on every state-changing auth endpoint ([#244]).
- **`ratelimit`** — sliding-window per-IP and per-account on auth endpoints ([#245]).
- **`anonymous`** — issue a session for an unauthenticated user; link on later sign-up ([#242]).
- **`accountlink`** — attach multiple OAuth accounts to one User by verified-email match ([#241]).
- **`multisession`** — list / revoke / revoke-others for the current user ([#243]).
- **`admin`** — operator surface: list users, ban, impersonate, revoke sessions ([#250]).
- **`org`** — organizations, members, roles, invitations, teams ([#249]).
- **`audit`** — sample plugin that logs every auth event via `slog` ([#251]).

Plugins are opt-in. The minimum viable app pulls `emailpassword` and `csrf`; everything else is additive.

## Security defaults

Every default below is what `auth.New` enables when the user does not override:

- **Argon2id** for password hashing (`time=2`, `memory=64MiB`, `threads=2`) ([#222]). Cost knobs are tunable; the default targets ~50ms/hash on a 2 vCPU host.
- **Double-submit CSRF** on every `POST/PUT/PATCH/DELETE` to `/auth/*` and on every form action posted via `kitform` ([#244]). Trusted-origin allowlist for cross-origin admin tooling.
- **Session-fixation guard** — every successful sign-in rotates the session token. Anonymous-to-authenticated transitions issue a new token and revoke the prior one.
- **Rate limiting** on `/auth/sign-in`, `/auth/sign-up`, `/auth/reset`, `/auth/verify-otp`, `/auth/totp/verify` ([#245]). Memory backend default; Redis backend recommended for multi-instance deployments.
- **Secure cookies** — `Secure`, `HttpOnly`, `SameSite=Lax`. `__Host-` prefix on the session cookie when `BaseURL` scheme is `https`.
- **Token rotation on privilege escalation** — issuing a TOTP, enrolling a passkey, changing a password, or completing email verification all rotate the session token and invalidate the prior cookie.
- **Optional IP pinning** — `Config.Session.IPPinning = true` binds a session to its issuing `/24` IPv4 or `/64` IPv6 range. Off by default because mobile networks rotate IPs aggressively; opt-in for high-stakes deployments.
- **Constant-time compares** everywhere a token, OTP, or HMAC is checked. Property tests in `auth/internal/cttest` enforce.

## Mailer + SMSSender

```go
type Mailer interface {
    Send(ctx context.Context, msg Email) error
}

type Email struct {
    To       string
    From     string
    Subject  string
    HTML     []byte
    Text     []byte
    Headers  map[string]string
    Tag      string   // for provider-side analytics
}

type SMSSender interface {
    Send(ctx context.Context, msg SMS) error
}

type SMS struct {
    To       string
    From     string
    Body     string
}
```

Built-in adapters ([#226], [#227]):

- **Mailer**: `email/smtp` (net/smtp + STARTTLS), `email/resend` (Resend HTTP API), `email/sendgrid` (Sendgrid HTTP API), `email/noop` (drops mail, DEBUG-logs subject + recipient — for tests).
- **SMSSender**: `sms/twilio` (Twilio HTTP API), `sms/noop`.

Templates are package-default (HTML + text), overridable via `Config.Templates`. The `noop` mailer is the test default; `auth.New` warns when running with `noop` and `BaseURL` scheme is `https`.

## RBAC

```go
type Permission string

type Role struct {
    Name        string
    Permissions []Permission
}

type AccessControl struct {
    Roles map[string]Role
}

func (ac *AccessControl) HasPermission(role string, perm Permission) bool

func Require(perm Permission) kit.HandleFn
```

`Require(perm)` is the predicate-style helper for `kithook.Require`. `org.Member` carries a `[]Role` slice — a user is a member of N organizations and holds a role per organization, not globally. `auth.UserFrom(locals).GlobalRoles` is for app-level roles (e.g. `admin`); `org.MemberFrom(locals, orgID).Roles` is for tenant-scoped roles. Two slots, no overlap. Spec at [#248].

## Migration path from "no auth"

Target: install `+` 1 sign-in form `+` 1 protected route in **<30 LOC** of user code, plus the `import` block.

```go
// hooks.server.go
//go:build sveltego
package src

import (
    "myapp/src/lib/server/authmod"
    "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
    "github.com/binsarjr/sveltego/packages/auth/kithook"
)

var Auth = authmod.New()

func Handle(ev *kit.RequestEvent, resolve kit.ResolveFn) (*kit.Response, error) {
    return kit.Sequence(
        kithook.Mount(Auth),
        kithook.PopulateLocals(Auth),
        kithook.Require(Auth, "/dashboard/*", kithook.SignedIn),
    )(ev, resolve)
}
```

```go
// src/lib/server/authmod/auth.go
//go:build sveltego
package authmod

import (
    "os"
    "github.com/binsarjr/sveltego/packages/auth"
    memstore "github.com/binsarjr/sveltego/packages/auth/storage/memory"
    "github.com/binsarjr/sveltego/packages/auth/email/noop"
    emailpassword "github.com/binsarjr/sveltego/packages/auth/plugin/emailpassword"
)

func New() *auth.Auth {
    a, err := auth.New(auth.Config{
        BaseURL: os.Getenv("BASE_URL"),
        Secret:  os.Getenv("AUTH_SECRET"),
        Storage: memstore.New(),
        Mailer:  noop.New(),
        Plugins: []auth.Plugin{emailpassword.Plugin()},
    })
    if err != nil { panic(err) }
    return a
}
```

```go
// src/routes/login/page.server.go
//go:build sveltego
package login

import (
    "myapp/src"
    "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
    "github.com/binsarjr/sveltego/packages/auth/kitform"
)

var Actions = kit.ActionMap{
    "default": kitform.SignInEmail(src.Auth, kitform.SignInOptions{RedirectTo: "/dashboard"}),
}
```

That is the full surface for a working sign-in. Sign-up adds three more lines (a `register` page with `kitform.SignUpEmail`). Promoting `memstore` to `pgxstore.New(pool)` is one line in `authmod`.

## Bench targets

Targets enforced via the bench harness ([#254]) and the regression gate ([#105]):

- **Sign-in (email/password) under default argon2id cost**: p50 < 50ms, p99 < 80ms on a 2 vCPU host. The argon2 verify dominates — anything beyond 80ms means the cost is too high or threading is wrong.
- **Session lookup, memory storage**: p50 < 1ms, p99 < 2ms.
- **Session lookup, Postgres via pgx**: p50 < 5ms, p99 < 10ms (single-connection, warm pool).
- **Stateless cookie decode + verify**: p50 < 30µs on commodity hardware.
- **CSRF token verify**: p50 < 5µs.
- **Allocation budget on the hot session-read path**: ≤ 8 allocations per request. Enforced by `benchstat` regression gate.

Bench harness compares against a parallel TypeScript run of `better-auth` for sanity, but the regression gate is internal-only — we do not block on the upstream library's perf. Numbers above the threshold open a `priority:p1` issue automatically.

## Out of scope (v0.6)

Carried forward from [#155] and consistent with ADR 0005:

- **SAML and Enterprise SSO** — separate RFC for v0.7. The plugin interface is forward-compatible; adding a `saml` plugin later does not require a v0.6 rev.
- **Stripe / billing** — separate plugin proposal, not an auth concern.
- **JWT API token issuance** — separate plugin in v0.7. Sessions are first-class; API tokens are a different problem (long-lived, scope-bounded, revocation-by-id) and bolting them onto the session strategy would muddy both.
- **Wire compatibility with the `better-auth` JavaScript client SDK** — sveltego ships its own Svelte client. The shape is similar; the wire is not stable across implementations.
- **Multi-tenant per-database isolation** — use the `org` plugin's scoping inside one database. Per-DB tenancy is a deployment concern, not a framework primitive.

## Open questions

These do not block v0.6 acceptance but are listed so future ADRs can resolve them:

1. **Passkey storage format** — `webauthn.Credential` carries vendor-specific blobs; storing as `[]byte` opaque is forward-compatible but opaque to admin tooling. Decide during [#231]: opaque blob with a separate metadata table, or a typed JSON column?
2. **Multi-tenant org boundaries** — what's the data-isolation contract when two organizations share a `User` row? The current draft says "users are global, memberships are scoped"; revisit during [#249] if this leaks personal data across orgs.
3. **GDPR-required user-deletion cascade** — `Storage.Users().Delete` must cascade to sessions, accounts, passkeys, TOTP, audit log. Spec the cascade order in `adaptertest` and require every adapter to pass; deferred to [#217] / [#250] to lock the test.
4. **Argon2 cost auto-tuning** — should `auth.New` benchmark argon2 at startup and adjust cost down if the host is below target? Auto-tuning is convenient but makes the password hash non-portable across deploys. Default off; revisit if support tickets converge here.
5. **PKCE state location** — query parameter (cleaner, survives provider quirks better) vs cookie (easier CSRF posture). Decide during [#232] with a short spike, not in this ADR.

## Issue map (v0.6)

Every issue in the v0.6 milestone maps to one or more ADR sections. Implementation order is the build sequence captured in [#155]; storage and sessions land first, OAuth and passkey can run in parallel after that.

| Issue | Title (short) | ADR section |
|---|---|---|
| [#155] | Master plan (this ADR) | (this ADR) |
| [#216] | Package scaffold + module layout | Architecture / Package layout |
| [#217] | Storage interface + memory adapter | Architecture / Storage interface |
| [#218] | database/sql adapter (Postgres + MySQL + SQLite) | Architecture / Storage interface |
| [#219] | pgx native Postgres adapter | Architecture / Storage interface |
| [#220] | Session token format + DB-backed strategy | Architecture / Session strategy |
| [#221] | Stateless encrypted-cookie session strategy | Architecture / Session strategy |
| [#222] | Argon2id Hasher + pluggable interface | Security defaults |
| [#223] | Email/password sign-up/sign-in/sign-out/change-password | Plugin model — built-ins |
| [#224] | Email verification flow | Plugin model — built-ins |
| [#225] | Password reset flow | Plugin model — built-ins |
| [#226] | Mailer interface + SMTP/Resend/Sendgrid/noop | Mailer + SMSSender |
| [#227] | SMSSender interface + Twilio/noop | Mailer + SMSSender |
| [#228] | Magic-link plugin | Plugin model — built-ins |
| [#229] | OTP via email + SMS | Plugin model — built-ins |
| [#230] | TOTP 2FA + backup codes | Plugin model — built-ins |
| [#231] | WebAuthn / passkey plugin | Plugin model — built-ins |
| [#232] | OAuth/OIDC provider system + PKCE + signed state | Plugin model — built-ins |
| [#233] | OAuth — Google | Plugin model — built-ins |
| [#235] | OAuth — GitHub | Plugin model — built-ins |
| [#236] | OAuth — Apple | Plugin model — built-ins |
| [#237] | OAuth — Discord | Plugin model — built-ins |
| [#238] | OAuth — Microsoft / Azure AD | Plugin model — built-ins |
| [#239] | OAuth — Facebook / Twitter / GitLab / Bitbucket / Twitch / LinkedIn / Spotify / Reddit | Plugin model — built-ins |
| [#240] | Generic OIDC provider | Plugin model — built-ins |
| [#241] | Account linking + multi-provider per user | Plugin model — built-ins |
| [#242] | Anonymous sessions + link-on-signup | Plugin model — built-ins |
| [#243] | Multi-session list / revoke / revoke-others | Plugin model — built-ins |
| [#244] | CSRF double-submit + trusted origins | Security defaults |
| [#245] | Rate-limiter interface + memory + Redis | Security defaults |
| [#246] | kit.Handle integration (Mount, PopulateLocals, Require) | Integration with sveltego |
| [#247] | kitform helpers for sign-in/up/reset/2FA | Integration with sveltego |
| [#248] | RBAC primitives (AccessControl, Role, Permission) | RBAC |
| [#249] | Organization + members + invitations + teams | RBAC, Open questions #2 |
| [#250] | Admin plugin (list, ban, impersonate, revoke) | Plugin model — built-ins |
| [#251] | Plugin interface + sample audit-log plugin | Architecture / Composition |
| [#252] | Redis secondary-storage for cache + blocklist | Architecture / Package layout, Session strategy |
| [#253] | Docs section + minimal example app | Migration path |
| [#254] | Bench harness vs upstream better-auth | Bench targets |
| [#255] | STABILITY.md + per-surface tier markings | Decision |

## Alternatives considered

### Embed Authelia

Authelia is a mature, battle-tested identity provider with first-class 2FA, passkey, and OIDC. Federating sveltego apps to it gives us those features for free. Rejected: it's a separate Go binary running its own HTTP server, the integration story is reverse-proxy headers (`Remote-User`, `Remote-Groups`), and the DX is "configure two services" rather than "import one library." For apps that already run Authelia, our `oauth-generic` plugin pairs with it cleanly — Authelia stays a great upstream, we just don't embed it.

### Federate to ory/kratos

Kratos is the reference Go-native identity server with a clean self-service flow API. Rejected for the same reasons as Authelia plus one: Kratos's API is opinionated about its own flow state machine, which doesn't compose with `kitform.Action`. Embedding Kratos as a library (rather than service) is not on its supported surface; we would be carrying a fork. As an external service for enterprise deployments, Kratos remains a great fit and our `oauth-generic` pairs with it.

### In-house, lighter (sessions + email/password only)

Ship sessions, email/password, and CSRF, leave OAuth and passkey to community plugins. Rejected: the DX bar is "one import covers the modern auth surface." A lighter library forces every app to pick three or four dependencies and stitch them together — exactly the JS world's pre-better-auth experience. The plugin interface keeps the library composable, so "lighter" is a config choice (don't import `oauth-google`), not an architectural one.

## References

- [`better-auth/better-auth`](https://github.com/better-auth/better-auth) — TypeScript shape we mirror.
- [`go-webauthn/webauthn`](https://github.com/go-webauthn/webauthn) — passkey backend.
- [`pquerna/otp`](https://github.com/pquerna/otp) — TOTP.
- [`golang.org/x/crypto/argon2`](https://pkg.go.dev/golang.org/x/crypto/argon2) — password hashing.
- [`golang.org/x/oauth2`](https://pkg.go.dev/golang.org/x/oauth2) — OAuth2 client.
- [Authelia](https://www.authelia.com/) — alternative considered.
- [Ory Kratos](https://www.ory.sh/kratos/) — alternative considered.
- [Lucia Auth](https://lucia-auth.com/) — better-auth's predecessor; informs the typed-session idiom.
- ADR 0001 — Parser Strategy.
- ADR 0002 — Template Expression Syntax.
- ADR 0003 — File Convention.
- ADR 0005 — Non-Goals.
- Issue [#155] — master plan tracking issue (closed by the PR landing this ADR).

[#21]: https://github.com/binsarjr/sveltego/issues/21
[#26]: https://github.com/binsarjr/sveltego/issues/26
[#27]: https://github.com/binsarjr/sveltego/issues/27
[#28]: https://github.com/binsarjr/sveltego/issues/28
[#30]: https://github.com/binsarjr/sveltego/issues/30
[#80]: https://github.com/binsarjr/sveltego/issues/80
[#81]: https://github.com/binsarjr/sveltego/issues/81
[#105]: https://github.com/binsarjr/sveltego/issues/105
[#155]: https://github.com/binsarjr/sveltego/issues/155
[#216]: https://github.com/binsarjr/sveltego/issues/216
[#217]: https://github.com/binsarjr/sveltego/issues/217
[#218]: https://github.com/binsarjr/sveltego/issues/218
[#219]: https://github.com/binsarjr/sveltego/issues/219
[#220]: https://github.com/binsarjr/sveltego/issues/220
[#221]: https://github.com/binsarjr/sveltego/issues/221
[#222]: https://github.com/binsarjr/sveltego/issues/222
[#223]: https://github.com/binsarjr/sveltego/issues/223
[#224]: https://github.com/binsarjr/sveltego/issues/224
[#225]: https://github.com/binsarjr/sveltego/issues/225
[#226]: https://github.com/binsarjr/sveltego/issues/226
[#227]: https://github.com/binsarjr/sveltego/issues/227
[#228]: https://github.com/binsarjr/sveltego/issues/228
[#229]: https://github.com/binsarjr/sveltego/issues/229
[#230]: https://github.com/binsarjr/sveltego/issues/230
[#231]: https://github.com/binsarjr/sveltego/issues/231
[#232]: https://github.com/binsarjr/sveltego/issues/232
[#233]: https://github.com/binsarjr/sveltego/issues/233
[#235]: https://github.com/binsarjr/sveltego/issues/235
[#236]: https://github.com/binsarjr/sveltego/issues/236
[#237]: https://github.com/binsarjr/sveltego/issues/237
[#238]: https://github.com/binsarjr/sveltego/issues/238
[#239]: https://github.com/binsarjr/sveltego/issues/239
[#240]: https://github.com/binsarjr/sveltego/issues/240
[#241]: https://github.com/binsarjr/sveltego/issues/241
[#242]: https://github.com/binsarjr/sveltego/issues/242
[#243]: https://github.com/binsarjr/sveltego/issues/243
[#244]: https://github.com/binsarjr/sveltego/issues/244
[#245]: https://github.com/binsarjr/sveltego/issues/245
[#246]: https://github.com/binsarjr/sveltego/issues/246
[#247]: https://github.com/binsarjr/sveltego/issues/247
[#248]: https://github.com/binsarjr/sveltego/issues/248
[#249]: https://github.com/binsarjr/sveltego/issues/249
[#250]: https://github.com/binsarjr/sveltego/issues/250
[#251]: https://github.com/binsarjr/sveltego/issues/251
[#252]: https://github.com/binsarjr/sveltego/issues/252
[#253]: https://github.com/binsarjr/sveltego/issues/253
[#254]: https://github.com/binsarjr/sveltego/issues/254
[#255]: https://github.com/binsarjr/sveltego/issues/255
[#283]: https://github.com/binsarjr/sveltego/issues/283
