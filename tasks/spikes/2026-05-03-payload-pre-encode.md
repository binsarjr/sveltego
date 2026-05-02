# Spike — pre-encode hydration payload stable fields (#488)

## Goal

Cut per-request CPU on the SSR hot path by pre-encoding hydration payload
fields whose value is constant across requests (`Manifest`, `AppVersion`,
`VersionPoll`, `RouteID`). Per-request marshal narrows to varying fields
(`Data`, `LayoutData`, `URL`, `Params`, `Status`, `Form`, `PageError`,
`Deps`).

Wire format must remain **byte-identical** so the client hydration entry
script and `__data.json` consumers see no change.

## Current shape

`packages/sveltego/server/pipeline.go:355` — `clientPayload`:

```go
type clientPayload struct {
    RouteID     string                `json:"routeId"`
    Data        any                   `json:"data"`
    LayoutData  []any                 `json:"layoutData,omitempty"`
    Form        any                   `json:"form"`
    URL         string                `json:"url"`
    Params      map[string]string     `json:"params"`
    Status      int                   `json:"status"`
    PageError   *clientPageError      `json:"error"`
    Manifest    []clientManifestEntry `json:"manifest,omitempty"`
    Deps        []string              `json:"deps,omitempty"`
    AppVersion  string                `json:"appVersion,omitempty"`
    VersionPoll *clientVersionPoll    `json:"versionPoll,omitempty"`
}
```

`json.Marshal` emits fields in struct-declaration order. `omitempty` skips
zero-value variants (empty slice/string/map → entire field omitted; nil
pointer → omitted; bool false → omitted).

## Splice algorithm

For each request build the JSON object directly:

```
{
  "routeId":<encodedRouteID>,             // pre-encoded per-route
  "data":<json.Marshal(data)>,             // varying — reflection unavoidable
  "layoutData":<json.Marshal(layoutDatas)>, // varying — only if non-empty
  "form":<json.Marshal(form)>,             // varying — defaults to null
  "url":<encoded URL string>,              // varying but cheap
  "params":<json.Marshal(params)>,         // varying
  "status":<itoa(status)>,                 // varying — strconv
  "error":<json.Marshal(pageError)>,       // varying — defaults to null
  "manifest":<encodedManifest>,            // pre-encoded at Server.New, only on initial render
  "deps":<json.Marshal(deps)>,             // varying — only if non-empty
  "appVersion":<encodedAppVersion>,        // pre-encoded at Server.New
  "versionPoll":<encodedVersionPoll>       // pre-encoded at Server.New
}
```

Pre-encoded slices store `,"<field>":<value>` (comma-prefixed) so they can
be appended unconditionally after non-empty preceding fields. The very
first field has no leading comma.

Implementation strategy: a single function `writePayload(buf,
payload, encoded)` writes opening `{`, then walks fields in order, emitting
either the pre-encoded slice or a per-request `json.Marshal` result.
Tracks whether a comma is needed between fields based on `omitempty`
semantics.

## Per-route encoded RouteID

The `router.Route` struct lives in `runtime/router` with its own STABILITY
surface. To avoid cross-package churn, the server keeps a
`map[string][]byte` keyed on `route.Pattern` populated at `Server.New()`
time. `writePayload` looks up by pattern and falls back to a per-request
marshal when the lookup misses (defensive — should not happen in practice).

Memory cost: ~50 bytes per route × N routes; negligible for typical apps.

## Byte-identity verification

A round-trip property test — `TestWritePayloadByteIdentical` — covers the
representative shapes:

1. Initial-render payload (all stable fields set, varying fields populated).
2. `__data.json` payload (no Manifest, no AppVersion, no VersionPoll).
3. Empty/zero payload (varying fields nil/empty).
4. Form-action override (Status=303, Form=ActionData).
5. Error boundary (PageError set, Status=500).

Each case asserts:
- `writePayload` output bytes equal `json.Marshal(payload)` bytes.
- `json.Unmarshal(writePayload output)` round-trips to a struct equal to
  the source `clientPayload`.

This is stricter than the existing hydration smoke (which only checks
field reachability via Unmarshal). With both gates green the wire format
is provably stable.

## Phase order

1. **Phase A.** Pre-encode Manifest, AppVersion, VersionPoll on Server.
2. **Phase B.** Per-route RouteID pre-encode lookup.
3. **Phase C.** `writePayload` splice writer; replace `marshalPayload` and
   `__data.json` callsites.
4. **Phase D.** Byte-identity property test + parity smoke run.
5. **Phase E.** `bench/ssr-constrained` baseline → after; document delta.

## Risks

- **JSON ordering change.** Go `encoding/json` encodes struct fields in
  declaration order; the splice writer must do the same. Property test
  catches drift.
- **Slice aliasing.** Encoded byte slices live for the Server lifetime
  and are read by every request. Writers must `Write(p)` (append-copy
  inside `bytes.Buffer` / `io.Writer`), not retain references.
- **`omitempty` parity.** Pre-encoded slices are stored as
  comma-prefixed slices; if AppVersion is empty, the slice is nil and
  the splice writer simply skips appending. Property test exercises both
  set and unset states.

## Out-of-scope (per #488)

- JSON library swap (encoding/json → goccy/jsoniter).
- Reducing payload bytes-on-wire (#315 territory).
- Devalue-style cycle/dedup.

## Acceptance gate

`bench/ssr-constrained/run.sh` shows ≥20% rps improvement at p99=100ms
ceiling. If <20%, escalate to team-lead before merge — alternative
strategy may be needed (e.g. switch to `bytes.Buffer`-backed writer with
sized pool, or pre-encode `Params` for static routes).
