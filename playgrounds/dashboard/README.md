# dashboard

Reference playground demonstrating cookie-session auth, multi-route protection
via the `Handle` hook, CRUD with form actions, and a JSON polling endpoint
backed by `_server.go`. Closes [#63](https://github.com/binsarjr/sveltego/issues/63).

## Features demonstrated

| Concept | Where |
|---|---|
| `Handle` hook for cookie-session auth | `src/hooks.server.go` |
| `kit.Cookies.Get/Set/Delete` | `src/hooks.server.go`, `src/routes/login/_page.server.go` |
| `kit.ActionMap` (default + named actions) | `src/routes/login/_page.server.go`, `src/routes/dashboard/_page.server.go` |
| `RequestEvent.BindForm` | every `Action` in this app |
| `kit.Redirect` (Load redirect) | `src/routes/_page.server.go` |
| `kit.ActionRedirect` (post-action redirect) | `src/routes/login/_page.server.go` |
| `kit.SafeError` via `kit.Fail` | login validation errors |
| `Locals[user]` (request-scoped state) | hooks → Load |
| `_server.go` REST endpoint (JSON) | `src/routes/api/metrics/_server.go` |
| Dynamic route param `[id]` | `src/routes/dashboard/items/[id]/` |
| `bcrypt` password hashing | `src/lib/store.go` |

## Layout

```
playgrounds/dashboard/
├── app.html                          # %sveltego.head% / %sveltego.body% shell
├── go.mod                            # require + replace points at packages/sveltego
├── cmd/app/main.go                   # boots server with gen.Routes() + gen.Hooks()
├── src/
│   ├── hooks.server.go               # cookie session → Locals.User; redirect-to-login on /dashboard
│   ├── lib/store.go                  # in-memory user + items store, bcrypt
│   └── routes/
│       ├── _layout.svelte            # nav + slot
│       ├── _page.svelte              # welcome / link to dashboard
│       ├── page.server.go            # Load (Locals.User), Actions{logout}
│       ├── login/_page.svelte        # login form
│       ├── login/page.server.go      # Actions{default: login}
│       ├── dashboard/_page.svelte    # items list + chart panel
│       ├── dashboard/page.server.go  # Load (items) + Actions{create, delete}
│       ├── dashboard/items/[id]/_page.svelte    # edit form
│       ├── dashboard/items/[id]/page.server.go  # Load (item) + Actions{update, delete}
│       └── api/metrics/server.go     # GET → JSON metrics for polling chart
```

## Auth flow

1. `POST /login?/default` runs `Actions["default"]` in `login/page.server.go`:
   - Binds `username/password` via `ev.BindForm`.
   - Verifies bcrypt hash via `store.Verify`.
   - Calls `store.IssueSession(user)` to mint an opaque session token.
   - Sets `session=<token>` cookie via `ev.Cookies.Set`.
   - Returns `kit.ActionRedirect(303, "/dashboard")`.
2. Every subsequent request: `Handle` reads the `session` cookie, looks up the
   user, sets `ev.Locals["user"]`. Requests targeted at `/dashboard*` without a
   session short-circuit with a 303 redirect to `/login`.
3. `POST /?/logout` deletes the cookie + session and redirects to `/login`.

## CRUD flow

`/dashboard` lists items in a table with inline create + delete forms. Each row
links to `/dashboard/items/<id>` for an edit page with update + delete forms.
All mutations use the kit Actions API so each landing renders the form's result
under `Data.Form` (extending the `Form any` field that codegen attaches to
`PageData` when `var Actions` is present).

The store is in-memory (`map[string]Item`) protected by a `sync.RWMutex`.
Restart loses state — replace with a real persistence layer in production.

## Polling chart

`/api/metrics` returns synthetic time-series JSON:

```json
{
  "ts": ["...", "..."],
  "values": [42, 51, ...]
}
```

The dashboard's chart panel renders a server-side ASCII bar chart from the
latest sample, plus a `<meta http-equiv="refresh" content="5">` chunk that
auto-refreshes the page every 5 seconds. True client-side polling (a
`fetch('/api/metrics').then(...)` loop) is JS — deferred to
[#34](https://github.com/binsarjr/sveltego/issues/34) (Vite client bundle).

## Run

```bash
cd playgrounds/dashboard
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego compile
go run github.com/binsarjr/sveltego/packages/sveltego/cmd/sveltego build --out ./build/app
./build/app                               # listens on :3000
```

Default credentials: `admin` / `password123`. The login form rejects anything
else with a `Form.Error` render-back.

```bash
curl -i http://localhost:3000/dashboard          # 303 → /login
curl -i -c /tmp/c -d 'username=admin&password=password123' http://localhost:3000/login?/default
curl -i -b /tmp/c http://localhost:3000/dashboard
curl -s -b /tmp/c http://localhost:3000/api/metrics | head -c 120
```

## References

- [#63](https://github.com/binsarjr/sveltego/issues/63) — original spec.
- [#26](https://github.com/binsarjr/sveltego/issues/26) — Handle hook.
- [#27](https://github.com/binsarjr/sveltego/issues/27) — form actions.
- [#78](https://github.com/binsarjr/sveltego/issues/78) — kit.Cookies.
- [ADR 0003](../../tasks/decisions/0003-file-convention.md) — file convention.

## Known limitations

- Layout-chain rendering ([#24](https://github.com/binsarjr/sveltego/issues/24))
  is v0.2-scope. The dashboard does not use a `(group)/` route group; all auth
  enforcement is hoisted to the `Handle` hook.
- Client-side fetch polling is a JS feature ([#34](https://github.com/binsarjr/sveltego/issues/34)).
  This playground demonstrates the SSR side (the JSON endpoint + a meta-refresh
  fallback chart panel). Once the Vite client bundle lands, a `<script>` block
  can replace the meta-refresh with a real `setInterval` polling loop.
- Sessions are in-memory. Restarting the server forces re-login.
