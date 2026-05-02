## 2026-05-02 — Reproduce both halves before assuming bug coupling

### Insight

- Issue #460 was titled "SSR sidecar respects vite `$app/*` aliases" with a
  hypothesis that the `stableStringify: cycle in AST` error was caused by
  unresolved `$app/*` imports. Two separate failures got bound into one
  symptom by the reporter's mental model: **alias resolution** and **AST
  serialisation cycle detection** are independent subsystems that both
  happened to break in the same code path.
- The cycle wasn't real. Acorn 8.16 emits `ImportSpecifier` where
  `imported === local` (same Identifier object) on a non-rename
  `import { name }`. ESTree allows this DAG. The legacy `WeakSet` cycle
  detector was add-only — it mis-flagged any DAG as a cycle. Fix was a
  one-line semantic swap (WeakSet → enter/exit ancestor stack), with
  *zero* connection to alias resolution.
- The alias problem was real but lived in the runtime fallback sidecar
  (`ssr_serve.mjs`) — a different file, different mode, different
  lifecycle. Build-time SSR mode (`--mode=ssr`) doesn't execute imports
  at all; it just serialises an AST. So criterion-1 of the issue
  ("resolves in build-time SSR sidecar") was meaningless under the
  literal reading and only made sense once you separated the two bugs.
- Reproducing each half *separately* — a 5-case sweep over import
  shapes (named / renamed / default / shorthand / no-imports) — surfaced
  that only the no-rename `ImportSpecifier` tripped the cycle, instantly
  decoupling the cycle from any alias-related cause. Walking the
  appstate fixture's compiled output with two detectors side-by-side
  (WeakSet false-positive vs ancestor-stack negative) confirmed the
  diagnosis before a single line of fix code was written.
- A third bug (`page.*` rune lowering in `svelte_js2go`) appeared once
  the first two were diagnosed — it's what blocks dropping the
  `<!-- sveltego:ssr-fallback -->` annotation in #463's playground
  fixture. That's a substantial separate feature filed as #467 and acked
  out of scope. Without the decoupling step, "fix #460" would have
  ballooned into a multi-day rewrite of the codegen lowerer instead of a
  surgical 7-file PR.

### Self-rules

1. **When an issue's hypothesis names two coupled causes ("X breaks
   because Y is unresolved"), reproduce both halves separately before
   assuming the coupling.** Construct the smallest possible repro for
   each named cause in isolation. If one half reproduces without the
   other, the coupling is wrong and the bugs are separate. Diagnose
   each separately, scope each separately, and ack the split with the
   team-lead before writing code.
2. **Use a sweep, not a single repro, when diagnosing pattern-shaped
   bugs.** A single failing case proves the bug exists; a sweep over
   adjacent shapes (renames, optional fields, shorthand vs longhand,
   empty vs populated) tells you the *boundary* of the bug. The
   boundary often points at the real root cause and disqualifies wrong
   hypotheses by construction. For #460 this meant:
   `import { x }` (cycles), `import { x as y }` (clean),
   `import x from` (clean), shorthand prop (clean), no imports (clean)
   — only the first shape trips it, so it can't be about the module
   being `$app/*` because the same shape with `'svelte'` would also
   cycle (and does).
3. **A "WeakSet of seen objects" is a visited-node detector, not a
   cycle detector. Cycles require ancestor tracking (add on enter,
   remove on exit). Any DAG-shaped input — and ESTree ASTs, JSON refs,
   shared object literals are all DAGs in the wild — produces false
   positives.** When implementing or reviewing a cycle check, ask: "is
   the same node legitimately reachable via two non-ancestor paths?"
   If yes, you need an ancestor stack, not a seen-set.
4. **The acceptance criteria of a misdiagnosed issue may need
   reframing, not just satisfying.** If the issue says "X must Y" but
   the underlying mechanism makes "Y" meaningless, surface the gap to
   the team-lead before quietly checking the box. PR #470's body
   reframed criterion 1 ("resolves in build-time SSR sidecar" →
   "AST emit no longer trips on shared-Identifier shape") with a
   one-line rationale; the team-lead acked the reframe rather than
   the literal reading.
5. **Diagnose before code, even under the per-task time cap.** The
   five minutes spent reproducing the bug, sweeping adjacent shapes,
   and walking the fixture output paid back tenfold by avoiding the
   wrong fix (writing a vite-config emitter for a sidecar that doesn't
   use vite). Five-iter / 60-min caps are budgets for execution, not
   excuses to skip diagnosis. If the cap is tight, *trim the fix*, not
   the diagnosis.
