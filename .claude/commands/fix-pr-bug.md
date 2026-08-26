---
description: Phase 5 — diagnose a post-implementation bug into a durable file, then fix it in a fresh context
argument-hint: Task identifier plus a short bug slug — e.g. "032 verify-flag-mismatch"
---

You are running Phase 5 of the MyFleet workflow: a bug found after
implementation — in PR validation, live testing, or regression — fixed
without carrying a debugging conversation into the fix.

Argument: **$ARGUMENTS** (a task identifier, then a short bug slug).

The full rationale and the loop this command mechanizes live in
[`docs/post-implementation.md`](../../docs/post-implementation.md). Read it if
anything below is ambiguous.

## Step 1 — Resolve the task

Same fuzzy-match algorithm as `/design-task` Step 1:

1. Glob `docs/tasks/task-*` (main) and `.worktrees/*/docs/tasks/task-*`
   (sibling worktrees).
2. Match the task identifier against folder names — exact, number-only
   (`1`/`001`/`task-1`/`task-001`), or slug fragment.
3. Zero matches → ask for correction. Multiple matches → list them and let
   the user pick.
4. Resolve to `<worktree>/docs/tasks/<id>/`.

Run `pwd`. If it does NOT match `<worktree>`, `cd <worktree>` yourself and
continue from there. Do NOT ask the user to re-run the command — per
CLAUDE.md's "Worktree Discipline" rule, cd into the task worktree yourself.
Do NOT create a new worktree.

If a prior `docs/tasks/<task>/bug-*.md` already describes this symptom, read
that instead of reproducing from scratch.

## Step 2 — Reproduce, in your own context

Reproduction is interactive and stays here. Confirm which environment and
build you are actually looking at first — see
[`docs/runbooks/k3s-deployment.md`](../../docs/runbooks/k3s-deployment.md) for
the `bee` cluster and
[`docs/runbooks/local-debugging.md`](../../docs/runbooks/local-debugging.md)
for the local stack. Check pod logs (`kubectl logs`) and cluster state before
anything else.

## Step 3 — Write the diagnosis to disk

Write `docs/tasks/<task>/bug-<slug>.md` using the template in
`docs/post-implementation.md`: reproduced / observed / expected / root cause,
then a `## Fix` file inventory, then `## Not yet answered`.

**Write it before dispatching anything.** This file is the boundary — after
it, everything must be resumable from repository state plus this file. If the
root cause is not established, say so and name what is ruled out; do not
guess one.

The `## Fix` inventory is what removes the fix agent's discovery phase. You
already know the paths from reproducing; without them the agent pays to
rediscover what you just found, at its own context depth.

## Step 4 — Dispatch a fresh implementer

```text
subagent_type: task-implementer
model: sonnet
```

Brief: the bug file path. Add only what the file cannot carry — the worktree
absolute path, and any ruling you made after writing it. Do not restate the
file. This dispatch is subject to the same 120-tool-call `task-implementer`
budget as any other dispatch — see
[`docs/agent-dispatch.md`](../../docs/agent-dispatch.md) §The implementer
budget.

**Run code review before pushing the fix, even for a one-line change.**
Dispatch `task-reviewer` (`model: sonnet`) against the fix commit range with
the bug file as the requirement, and add the guideline reviewers
(`backend-guidelines-reviewer` / `frontend-guidelines-reviewer`) via
`superpowers:requesting-code-review` if the fix touches Go or frontend code.

## Step 5 — Verify in a clean context

Dispatch `task-verifier` (`model: haiku`) to run the gate. A flagless
`tools/verify.sh` exit 0 is required before the fix is pushed — only a
flagless exit 0 authorizes calling the branch done; `--quick` and
`--no-docker` exit 0 on success but do not (see
[`docs/verification.md`](../../docs/verification.md)). Launch it backgrounded
and keep going; do not poll it — `.claude/hooks/wait-loop-guard.sh` refuses
the poll.

`make ci` and `.github/workflows/pr.yml` are the same gate the pushed branch
will re-run; a flagless local pass before pushing is what makes that CI run a
confirmation rather than a discovery.

## Step 6 — Push and ledger

Push the fix to the **same branch** as the PR under investigation — never a
second branch; the clean PR branch comes from a rebase at PR time, not from
juggling parallel branches (see
[`docs/git-workflow.md`](../../docs/git-workflow.md)).

```sh
tools/agent-ledger.sh append <task> --unit "bug-<slug>" --agent-type task-implementer \
  --model sonnet --status <status> --commit <sha>
```

Add a reviewer row with `--verdict` if `task-reviewer` ran. Then update the
bug file with the outcome — the commit that fixed it, and whether live
testing confirmed it.

## Step 7 — Decide whether to continue here

Ask the handoff question: does the next bug depend on this conversation, or
only on repository state? An unrelated bug is a fresh unit — run
`/fix-pr-bug` again against its own bug file rather than accumulating.

Past ~150k context, stop starting new investigations in this session: write
the remaining leads into the task folder and say so as your final output. A
handoff the same context then works past is not a handoff.

## Important rules

- Never reproduce inside a subagent; never implement the fix inside this
  context.
- Never dispatch without a bug file on disk.
- Never poll a background process.
- Never commit to `main`; the fix lands on the task branch.
- Never claim the bug fixed without a flagless `tools/verify.sh` pass and,
  where applicable, a live re-test.
