# 2026-05-03 — worktree-hijack: parallel agents collided in `phase-gap-unblock-478`

## Insight

Spawning parallel `isolation:worktree` agents from a team-lead session whose CWD was already inside a worktree (`phase-gap-unblock-478`) caused two of three sibling agents (D1 fixing #524, D2 fixing #525) to land in the **same** sibling worktree (`phase-gap-unblock-478`) instead of each getting a fresh `agent-*` tree. Result:

- D2 ran `git checkout fix/sidecar-deps-install` while D1 was mid-edit on `fix/scaffold-ootb-build` in the same checkout — D1's uncommitted edits to `packages/init/internal/scaffold/{scaffold.go,tailwind.go,scaffold_test.go}` were wiped from disk.
- D2 noticed the foreign branch + uncommitted scaffold work and stashed it as `scaffold-ootb-build-WIP-saved-by-525-agent` before completing #525, but D1 had already aborted by then and did not recover from the stash.
- A third agent (D3 fixing #526) landed in its own dedicated worktree `agent-fix-go-install-526` (not shared) and shipped #530 cleanly.
- Recovery for D1: team-lead manually created `/tmp/sveltego-d1-redo` via `git worktree add` and dispatched D1 to redo there.

The lesson `feedback_worktree_hijack.md` already warned about this exact pattern. The mitigation in the prompts ("verify pwd+branch on startup") **detected** the collision but did not **prevent** it — D1 halted on the wrong-branch heuristic, but D2 silently proceeded and stomped D1's work.

## Self-rules

1. When dispatching parallel `isolation:worktree` agents from a session whose CWD is itself inside a worktree, do not assume the spawn mechanism creates fresh, exclusive `agent-*` trees per agent. **Verify** by checking `git worktree list` immediately after spawn — if two agents land on the same path, halt one before either edits.
2. Prefer dispatching parallel agents to **manually pre-created** `/tmp/<task-name>` worktrees (via `git -C <main-repo> worktree add /tmp/<name> -b <branch> origin/main`) rather than relying on `isolation:worktree` from inside a worktree CWD. This guarantees one tree per agent.
3. When two parallel fixes touch disjoint files, sharing a worktree is technically OK — but only if both agents are aware they share. Default-assume isolation; never let an agent run `git checkout` to a foreign branch in a tree another agent might be using.
4. If recovery is needed, the contaminating agent should **stash and tag** the foreign work (D2 did this correctly with the named stash), then notify the original owner so the work isn't lost. Add the stash name to the agent's completion report for cross-agent visibility.
5. Sanity-check prompts must include a **diff check** — not just "is branch X?" but "is `git status` clean of files I didn't touch?". D1's "STOP if branch is main" check missed this case because branch matched but the tree had foreign files.
