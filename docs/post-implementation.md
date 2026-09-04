# Phase 5 — After Implementation

The four-phase flow ends at `/execute-task`. The work does not: PR validation,
live-environment testing, bug reproduction, regression investigation, and
follow-up fixes all happen afterwards, and nothing told those sessions to
delegate. This document is the missing phase.

It introduces no new context-clearing rule and requires nothing of the user. It
generalizes the handoff principle the execute loop already applies — see
[`docs/agent-dispatch.md`](agent-dispatch.md) §Context handoff — to the work
that follows implementation.

---

## Why

Main-thread tokens are the expensive kind: the context grows monotonically and
every turn re-reads all of it. A turn at 200k+ costs several times the same
investigation inside a fresh subagent starting near 40k. A post-PR session that
stays solo and inline pays the main-thread rate for work that a dispatched
`task-implementer` or `task-verifier` would do at a fraction of the cost.

The failure mode this document targets is specific: a diagnosis gets written to
a file (the habit is usually already right — bug artifacts are cheap and
people write them), but the fix is then implemented inline in the same
long-running session instead of being dispatched. The artifact habit is not
the gap; the delegation habit is.

A second, related cost is rediscovery: a post-PR session re-greps the repo to
relocate code the same branch wrote hours earlier, or re-reads `plan.md`
end-to-end to re-establish what a task said, because nothing captured it in a
retrievable form. The bug file (below) exists to prevent exactly that.

---

## The loop

**Reproduce inline. Diagnose into a file. Delegate the fix. Verify fresh.**

### 1. Reproduce — stay in your own context

Reproduction is interactive: the operator is in the loop with a live UI or API
client, a running cluster, round-trip latency matters more than tokens, and
each step depends on what the last one showed. Do this yourself. Do not
delegate it.

Check pod logs (`kubectl logs`) and the cluster state first — see
[`docs/runbooks/k3s-deployment.md`](runbooks/k3s-deployment.md) for the `bee`
cluster and [`docs/runbooks/local-debugging.md`](runbooks/local-debugging.md)
for the local stack. Confirm which environment and which build you are
actually looking at before anything else; the wrong environment sends the
whole investigation down the wrong path.

### 2. Write the diagnosis to `docs/tasks/<task>/bug-<slug>.md`

Before dispatching anything. This is the boundary: everything after it must be
resumable from repository state plus this file.

```markdown
# bug: <one-line symptom>

**Reproduced:** <environment, build/commit, exact steps>
**Observed:** <what happens, with the log line / response body / error verbatim>
**Expected:** <what should happen, and where that is specified — PRD/FR, plan task>
**Root cause:** <what you established, with file:line>  — or: "not yet established; <what is ruled out>"

## Fix

- `apps/fleet-service/internal/fleet/resource.go:42` — <what changes here>
- `apps/fleet-service/internal/fleet/builder_test.go` — <the test that must fail before and pass after>

## Not yet answered

- <anything the fix agent must decide, and what it should do if unsure>
```

The `## Fix` section is a `### Files` inventory by another name, and it does the
same job: it removes the implementer's discovery phase — the phase that
inflates context before a single edit happens. You already know these paths
from reproducing; the fix agent would otherwise pay to rediscover them, at its
own context depth, on top of what you already paid at yours.

If the root cause is not established, say so explicitly and name what is ruled
out. An honest "not yet established" is a fine brief; a guessed root cause is
not.

### 3. Delegate the fix to a fresh agent

```text
subagent_type: task-implementer
model: sonnet
brief: docs/tasks/<task>/bug-<slug>.md
```

The bug file is the brief. Do not restate it in the dispatch prompt — add only
what the file cannot carry: the worktree path, and any ruling you have made
since writing it. The dispatch is subject to the same 120-tool-call
implementer budget as any other `task-implementer` dispatch — see
[`docs/agent-dispatch.md`](agent-dispatch.md) §The implementer budget.

This is the step most likely to be skipped. It is also where the saving is:
the fix agent starts near its own small context instead of inheriting the
reproduction session's accumulated history.

### 4. Verify in a clean context

`task-verifier` (`model: haiku`) runs `tools/verify.sh` for the gate —
flagless if a reachable cluster context is available, `--quick` otherwise, and
a flagless run must still happen before the branch is called done (see
[`docs/verification.md`](verification.md)). `task-reviewer` (`model: sonnet`)
if the fix crosses a service boundary or touches a contract — the gate
(`make ci`, `.github/workflows/pr.yml`) cannot see a seam defect.

Launch the gate backgrounded and keep going; do not poll it.
`.claude/hooks/wait-loop-guard.sh` will refuse the poll anyway.

### 5. Ledger it

```sh
tools/agent-ledger.sh append <task> --unit "bug-<slug>" --agent-type task-implementer \
  --model sonnet --status <status> --commit <sha>
```

So the next audit can answer "what did the post-PR phase actually cost" without
reconstructing transcripts.

---

## The `/fix-pr-bug` procedure

`/fix-pr-bug` mechanizes steps 2–5 above for a single bug, once reproduction
(step 1) has established what is wrong:

1. Write the diagnosis to `docs/tasks/<task>/bug-<slug>.md` in the shape
   above — reproduced, observed, expected, root cause, `## Fix` file list,
   `## Not yet answered`.
2. Dispatch a fresh `task-implementer` (`model: sonnet`, 120-tool-call budget)
   with the bug file as its brief; add only the worktree path and anything
   ruled out since the file was written.
3. On completion, dispatch `task-verifier` for `tools/verify.sh` in a clean
   context; add `task-reviewer` if the fix crossed a service boundary or
   touched a contract.
4. Push the fix on the same branch as the PR under investigation — see
   [`docs/git-workflow.md`](git-workflow.md); never on a second branch.
5. Append one `tools/agent-ledger.sh` line for the fix unit, batched with
   whatever gate-log or review-artifact read the verdict came from (see
   [`docs/tooling-conventions.md`](tooling-conventions.md)).

If root cause is not established, `/fix-pr-bug` still writes the bug file with
an honest "not yet established" and the implementer's first job is narrowing
it — it does not block on a guessed diagnosis.

---

## When to hand off your own context

The same question as every other durable boundary: **does the next unit depend
materially on this conversation's history, or only on repository state plus a
short written diagnosis?**

After a bug file is written, the answer is almost always "repository state".
Once you have written the diagnosis, the reproduction conversation that
produced it is no longer load-bearing.

Concretely, in a debugging session:

- **After each bug file is written and its fix dispatched**, ask the question.
  If the next bug is unrelated to the one you just fixed, it is a fresh unit —
  dispatch it against its own bug file rather than continuing to accumulate.
- **Past ~150k tokens, stop starting new investigations in this context.**
  Write the remaining leads into the task folder and hand off. This is the
  same backstop `docs/agent-dispatch.md` sets for a controller (~150k tokens,
  or 4 completed plan tasks, whichever comes first); the `/execute-task`
  ceiling applies here for the same reason it applies there: the marker on
  disk is meaningless if the session that wrote it keeps going.

---

## What does not change

- **Reproduction stays inline.** An over-delegated interactive debugging session
  is worse than an expensive one.
- **The bug-file habit is already right** — this document adds the delegation
  step after it, not a new artifact format.
- **`/execute-task`'s ceiling and ledger are unchanged.** Phase 5 borrows them;
  it does not redefine them.
