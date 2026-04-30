# Architecture Decision Records (ADRs)

Locked decisions for `sveltego`. Each ADR mirrors a tracked GitHub RFC issue. The local file is the working copy contributors read offline; the GitHub issue is the discussion thread.

| # | Title | GitHub Issue | Status |
|---|---|---|---|
| 0001 | Parser Strategy | [#1](https://github.com/binsarjr/sveltego/issues/1) | Accepted |
| 0002 | Template Expression Syntax | [#2](https://github.com/binsarjr/sveltego/issues/2) | Accepted |
| 0003 | File Convention (`.gen/` Layout) | [#3](https://github.com/binsarjr/sveltego/issues/3) | Accepted |
| 0004 | Codegen Output Shape | [#4](https://github.com/binsarjr/sveltego/issues/4) | Accepted |
| 0005 | Non-Goals | [#94](https://github.com/binsarjr/sveltego/issues/94) | Accepted |
| 0006 | sveltego-auth Master Plan | [#155](https://github.com/binsarjr/sveltego/issues/155) | Accepted |

## Convention

- Filename: `NNNN-kebab-title.md` (zero-padded 4 digits).
- Status values: `Proposed`, `Accepted`, `Superseded by NNNN`, `Deprecated`.
- Never edit an `Accepted` ADR in place. Supersede with a new one and link both ways.
- Every ADR carries the linked GitHub issue and date.
