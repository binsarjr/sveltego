## 2026-04-29 — Issue authoring standard

### Insight

- An issue list of ~70 items doesn't speak for itself. Bullet-only checklists without context burn future contributor time looking up "what does this mean."
- Industry-standard issue body is a contract: Summary, Background, Goals, Non-Goals, Detailed Design with code, Acceptance Criteria, Testing Strategy, Out of Scope, Risks & Open Questions, Dependencies (Blocks/Blocked by), References.
- Switching repo language to English mid-project is cheap if done in one pass.

### Self-rules

1. When seeding a roadmap, write each issue as if a stranger will pick it up — context plus contract.
2. Cross-reference dependencies explicitly (Blocks / Blocked by). Don't make readers reconstruct the order.
3. Ship code samples in design sections. Words drift; signatures don't.
4. One language per repo. If switching, batch the migration in a dedicated pass.

