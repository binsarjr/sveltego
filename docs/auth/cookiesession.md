---
title: cookiesession
order: 10
summary: Encrypted, stateless, type-safe sessions stored in browser cookies. AES-256-GCM, chunked for >4 KB payloads, secret rotation built in.
---

# cookiesession

`github.com/binsarjr/sveltego/cookiesession` stores session data in signed and encrypted browser cookies. No database, no server-side state. Rotation lets you retire old secrets without logging users out.

See [STABILITY.md](https://github.com/binsarjr/sveltego/blob/main/packages/cookiesession/STABILITY.md) for the current API tier. All exported symbols are **experimental** — signatures may change before v1.0.

## Quickstart

Install:

```bash
go get github.com/binsarjr/sveltego/cookiesession
```

Wire the middleware in `src/hooks.server.go`:

```go
//go:build sveltego

package hooks

import (
  "github.com/binsarjr/sveltego/cookiesession"
  "github.com/binsarjr/sveltego/exports/kit"
)

type Session struct{ UserID string }

var codec = must(cookiesession.NewCodec([]cookiesession.Secret{
  {ID: 1, Key: loadKey("SESSION_SECRET")}, // 32-byte key from env
}))

// Handle installs the session middleware. Compose with your own hooks
// using kit.Sequence.
var Handle = cookiesession.Handle[Session](codec, "sess",
  cookiesession.WithHTTPOnly(true),
  cookiesession.WithSameSite(0), // defaults to Lax
)

func must[T any](v T, err error) T {
  if err != nil { panic(err) }
  return v
}

func loadKey(env string) []byte {
  // In production: os.Getenv(env) decoded from hex or base64.
  // Key must be exactly 32 bytes.
  panic("replace with real key loading")
}
```

Read the session in a Load function:

```go
//go:build sveltego

package routes

import (
  "github.com/binsarjr/sveltego/cookiesession"
  "github.com/binsarjr/sveltego/exports/kit"
)

type Session struct{ UserID string }

func Load(ctx *kit.LoadCtx) (struct{ UserID string }, error) {
  sess, ok := cookiesession.FromCtx[Session](ctx)
  if !ok {
    return struct{ UserID string }{}, nil
  }
  return struct{ UserID string }{UserID: sess.Data().UserID}, nil
}
```

Mutate the session in a form action:

```go
var Actions = kit.ActionMap{
  "login": func(ev *kit.RequestEvent) kit.ActionResult {
    sess, ok := cookiesession.From[Session](ev)
    if !ok {
      return kit.ActionFail(500, nil)
    }
    _ = sess.Set(Session{UserID: "u-123"})
    return kit.ActionRedirect(303, "/dashboard")
  },
}
```

For a complete working example see the [cookiesession-counter playground](https://github.com/binsarjr/sveltego/tree/main/playgrounds/cookiesession-counter).

## API reference

### `Codec`

```go
type Codec interface {
  Encrypt(plaintext []byte) (cookie string, err error)
  Decrypt(cookie string) (plaintext []byte, err error)
}
```

`Codec` encrypts and decrypts cookie payloads. `Encrypt` always uses the first (newest) secret; `Decrypt` tries every secret in order. Pass a `Codec` to `Handle[T]` and `NewSession[T]`.

### `NewCodec`

```go
func NewCodec(secrets []Secret) (Codec, error)
```

Returns a `Codec` backed by `secrets`. Secrets must be newest-first. Returns an error if the slice is empty or any key is not exactly 32 bytes.

### `Secret`

```go
type Secret struct {
  ID  uint32
  Key []byte // must be exactly 32 bytes
}
```

Pairs a rotation ID with a 32-byte AES-256 key. Use a unique monotonically increasing `ID` for each key generation.

### `Session[T]`

```go
type Session[T any] struct { /* unexported */ }
```

Request-scoped session value. Do not share across goroutines. Methods:

| Method | Description |
|---|---|
| `Data() T` | Returns the current payload (safe for concurrent reads). |
| `Set(v T) error` | Replaces the payload and flushes the cookie. |
| `Update(fn func(T) T) error` | Applies fn to a copy of Data and calls Set. |
| `Refresh(d ...time.Duration) error` | Resets expiry to now+d (or opts.MaxAge) and flushes. |
| `Destroy() error` | Clears the session and emits deletion cookies. |
| `Expires() time.Time` | Returns the expiry stamped in the payload (zero = no expiry). |
| `NeedsSync() bool` | Reports whether the session has unflushed changes. |
| `IsDirty() bool` | Alias for `NeedsSync`. |

### `NewSession[T]`

```go
func NewSession[T any](r *http.Request, w http.ResponseWriter, codec Codec, opts Options) (*Session[T], error)
```

Creates a `Session[T]` and immediately loads from request cookies. If no cookie is found, the session starts empty. A decode failure (tampered or wrong key) is returned as an error; the session is still usable with a zero `T`.

### `Options`

```go
type Options struct {
  Name     string
  MaxAge   time.Duration
  Path     string
  Domain   string
  Secure   *bool
  SameSite http.SameSite
}
```

Configuration for session cookie attributes. `Name` is required. `Secure` defaults to `r.TLS != nil` when nil. `SameSite` defaults to `Lax` when zero. `Path` defaults to `/`.

### `Handle[T]`

```go
func Handle[T any](codec Codec, name string, opts ...CookieOption) kit.HandleFn
```

Returns a `kit.HandleFn` middleware. For each request it:

1. Reads cookie chunks from the incoming request.
2. Decrypts via `codec` (secret rotation handled transparently).
3. Decodes to `*Session[T]`; on failure starts an empty session and logs at Debug.
4. Stashes the session in `ev.Locals` under a typed key unique to `T`.
5. Calls the next handler.
6. Forwards any `Set-Cookie` headers from session mutations to the `kit.Response`.
7. Sets `Sveltego-Cookie-Session-Sync: 1` when the session is dirty.

Compose with `kit.Sequence`:

```go
var Handle = kit.Sequence(
  cookiesession.Handle[Session](codec, "sess"),
  myAuthHandle,
)
```

Two parallel handles `Handle[Foo]` + `Handle[Bar]` do not collide because the Locals key is type-qualified.

### `CookieOption` (functional options)

| Option | Default | Description |
|---|---|---|
| `WithMaxAge(seconds int)` | 0 (session cookie) | Sets `Max-Age` attribute. |
| `WithSecure(bool)` | auto (`r.TLS != nil`) | Overrides automatic HTTPS detection. |
| `WithHTTPOnly(bool)` | `true` | Sets `HttpOnly` attribute. |
| `WithSameSite(http.SameSite)` | `Lax` | Sets `SameSite` attribute. |
| `WithDomain(string)` | (host-only) | Sets `Domain` attribute. |
| `WithPath(string)` | `/` | Sets `Path` attribute. |

### `From[T]`

```go
func From[T any](ev *kit.RequestEvent) (*Session[T], bool)
```

Retrieves the `*Session[T]` stashed by `Handle[T]`. Returns `(nil, false)` when the middleware was not installed for this type. Use in Handle-level code and form actions.

### `FromCtx[T]`

```go
func FromCtx[T any](ctx *kit.LoadCtx) (*Session[T], bool)
```

Retrieves the `*Session[T]` from a Load-level context. The pipeline shares `RequestEvent.Locals` with `LoadCtx.Locals` so values stashed by `Handle[T]` are visible without additional wiring.

### `MustFrom[T]`

```go
func MustFrom[T any](ev *kit.RequestEvent) *Session[T]
```

Like `From[T]` but panics with a clear message when the middleware is not installed. Use only in code paths where the middleware is guaranteed present; the panic is intentional because its absence is a programming error, not a runtime condition.

## Threat model

### What is protected

- **Cookie tampering.** Each cookie value is AES-256-GCM ciphertext. The GCM authentication tag detects any bit flip. A tampered cookie yields a decode error; the middleware starts a fresh empty session.
- **Secret compromise + rotation.** Multiple secrets can coexist in the codec. Retire compromised keys by removing them from the `Secrets` slice (after rotating — see the playbook below).
- **Type confusion between parallel sessions.** `Handle[Foo]` and `Handle[Bar]` use different Locals keys. `From[Foo]` never retrieves a `*Session[Bar]`.

### What is NOT protected

- **XSS reading session data.** `HttpOnly=true` (the default) prevents JavaScript from reading the cookie. If you set `WithHTTPOnly(false)`, XSS can exfiltrate the session cookie and the ciphertext (though it cannot forge a new one without the key).
- **Replay within session lifetime.** A cookie captured at time T is valid until its `Expires` timestamp. Rotating secrets does not invalidate existing issued cookies. Mitigate with short `MaxAge` values or server-side revocation (which this library does not provide).
- **Length side-channel on chunked payloads.** Payload size is visible from the number of `Set-Cookie` chunks. Do not store secrets whose length is sensitive in session data.
- **Client-side JSON inspection.** Attackers who hold the AES key (via server compromise) can decode any session. The cookie is encrypted, not merely signed; decryption requires the key, not just possession of the ciphertext.
- **Cross-site request forgery.** `SameSite=Lax` (the default) mitigates most CSRF. For stricter requirements set `WithSameSite(http.SameSiteStrictMode)` or add an explicit CSRF token.

## Secret rotation playbook

Rotate secrets on a regular cadence (e.g., every 90 days) or immediately when a key is suspected compromised.

**Step 1 — generate a new 32-byte key.**

```bash
openssl rand -hex 16   # 16 bytes hex = 32 hex chars; use the raw bytes
# Or: sveltego gen-secret  (when the CLI command lands)
```

Store the new key in your secrets manager.

**Step 2 — deploy with new key first, old key second.**

```go
cookiesession.NewCodec([]cookiesession.Secret{
  {ID: 2, Key: newKey}, // new key at index 0 → used for Encrypt
  {ID: 1, Key: oldKey}, // old key → used for Decrypt fallback
})
```

`Encrypt` always uses the first secret. `Decrypt` tries every secret in order, identified by the `&id=N` suffix embedded in each cookie value.

**Step 3 — wait one full session lifetime.**

All old-key cookies expire. No session encrypted with the old key remains valid.

**Step 4 — remove the old key.**

```go
cookiesession.NewCodec([]cookiesession.Secret{
  {ID: 2, Key: newKey},
})
```

Deploy again. Done.

**When to rotate:**
- Regular cadence (recommended: 90 days for general use, 30 days for high-assurance).
- Immediately on suspected key compromise or server breach.
- After any member of the team with key access leaves.

## Cross-references

- [ADR 0006 — auth master plan](https://github.com/binsarjr/sveltego/issues/155) — cookiesession is one layer in the full auth track.
- [STABILITY.md](https://github.com/binsarjr/sveltego/blob/main/packages/cookiesession/STABILITY.md) — API stability tier.
- [cookiesession-counter playground](https://github.com/binsarjr/sveltego/tree/main/playgrounds/cookiesession-counter) — minimal working example.
