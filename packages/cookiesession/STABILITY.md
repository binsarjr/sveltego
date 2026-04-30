# Stability — cookiesession

Last updated: 2026-04-30 · Version: pre-alpha

Tiers per [RFC #97](https://github.com/binsarjr/sveltego/issues/97). Pre-`v0.1` every export is implicitly experimental; this file populates as APIs land.

## Stable

(none yet)

## Experimental

- `cookiesession.Codec` — interface; Encrypt/Decrypt contract may gain options in #198.
- `cookiesession.NewCodec` — factory; signature frozen for #198.
- `cookiesession.Secret` — struct; ID+Key fields stable.
- `cookiesession.Session[T]` — generic struct; methods stable for #199 (Handle middleware).
- `cookiesession.Options` — configuration struct for Session creation.

## Deprecated

(none yet)

## Internal-only (do not import even though accessible within module)

- `internal/crypto` — AES-256-GCM primitives; consumed by Session (#198) and Handle (#199) only.
