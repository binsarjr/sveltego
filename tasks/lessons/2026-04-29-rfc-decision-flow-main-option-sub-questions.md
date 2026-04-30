## 2026-04-29 — RFC decision flow (main option + sub-questions)

### Insight

- An RFC issue with N alternatives is not enough. Every option drags 2–4 follow-on questions (error recovery strategy, identifier-naming corner cases, builtin allowlist, snippet visibility). Picking only the headline option leaves codegen blocked.
- Decisions split cleanly into **Main option** (the path) + **Sub-decisions** (the corner cases). Both must be locked before code starts.
- Sub-decisions are best surfaced as a numbered list inside the parent RFC. User answers `1 a, 2 b, ...` in one shot. Cheap for both sides.
- Locked decisions live in two places: GitHub issue body (discussion record) + local `tasks/decisions/NNNN-*.md` ADR (offline + grep-able). One source of truth is a myth when contributors work without GitHub access.
- ADR file format borrows from MADR / Sun: Status, Date, linked Issue, Decision, Rationale, Locked sub-decisions, Implementation outline, References.

### Self-rules

1. When proposing alternatives in an RFC, also enumerate the sub-questions that the chosen option will force. Don't ask the user to pick A/B/C without listing the trapdoors under each.
2. After the user answers, write the locked decision to **both** the GitHub issue (prepend a `## Decision (date)` block above existing alternatives) and a local `tasks/decisions/NNNN-*.md` ADR.
3. ADR filename uses zero-padded 4-digit prefix and stable kebab title — never reuse a number, never edit an Accepted ADR in place. Supersede with a new ADR.
4. Sub-decision rationale belongs **with the sub-decision**, not in a separate doc. A future reader hitting one bullet should find the "why" without bouncing files.
5. Code samples in ADRs are signatures + 5-line illustrations, not full implementations. Signatures don't drift; example bodies do.

