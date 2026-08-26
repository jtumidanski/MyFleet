---
description: Phase 4 — invoke superpowers:subagent-driven-development to implement a planned task in its existing worktree
argument-hint: Task identifier — accepts "task-001-bucket-replication", "task-001", "001", or "1"
---

You are starting Phase 4 of the MyFleet four-phase development workflow. Argument: **$ARGUMENTS**

## Process

### Step 1 — Resolve the task

Same fuzzy-match algorithm as `/design-task` Step 1:

1. Glob `docs/tasks/task-*` (main) and `.worktrees/*/docs/tasks/task-*` (sibling worktrees).
2. Match `$ARGUMENTS` against folder names — exact, number-only (`1`/`001`/`task-1`/`task-001`), or slug fragment.
3. Zero matches → ask for correction. Multiple matches → list and let the user pick.
4. If the task lives only on main with no worktree, stop and tell the user the task needs a worktree.
5. Resolve to `<worktree>/docs/tasks/<id>/`.

### Step 2 — Ensure we're in the right worktree

Run `pwd`. If it does NOT match `<worktree>`, `cd <worktree>` yourself and continue from there. Do NOT ask the user to re-run the command — per CLAUDE.md's "Worktree Discipline" rule, cd into the task worktree yourself.

Do NOT create a new worktree — the worktree was created by `/spec-task` and must be reused so phase artifacts stay co-located.

### Step 3 — Validate inputs

Confirm `<worktree>/docs/tasks/<id>/plan.md` AND `context.md` exist. If either is missing, tell the user to complete `/plan-task` first.

### Step 4 — Invoke subagent-driven-development

Use the Skill tool to invoke `superpowers:subagent-driven-development` (default). Pass:

- Plan path: `<worktree>/docs/tasks/<id>/plan.md`
- Context path: `<worktree>/docs/tasks/<id>/context.md`
- Project conventions: `<worktree>/CLAUDE.md`
- **Worktree absolute path** (`<worktree>`) for every dispatched implementer subagent. Subagent prompts MUST enforce cwd-discipline — every Bash call prefixed with `cd <worktree> && ...`, post-commit branch verification, no destructive git ops, no `git add -A` / `git add .`.

If the user explicitly requests inline mode this session (rare), invoke `superpowers:executing-plans` instead.

### Step 4a — Model discipline for every dispatch

Name an explicit `model` on every subagent dispatch. Leaving it unspecified
inherits Opus, which costs a large multiple of Sonnet's per-turn cost for work
that does not need it. `task-verifier` is pinned to `haiku` — it runs one
command and quotes the output. `task-implementer` defaults to `sonnet`; a plan
task tagged `model: opus` in `plan.md` may opt in, but that is the only
exception. Never use Fable for background or review work. The full job → model
table lives in [`docs/agent-dispatch.md`](../../docs/agent-dispatch.md).

### Step 4b — Brief-first

Produce each implementer's brief with the repo's own script, not by hand:

```sh
tools/task-brief.sh docs/tasks/<id>/plan.md <N>
```

It extracts the plan's `Task <N>` section verbatim and writes it to a brief
file, printing the path it wrote. Exit 3 means the plan has no heading
matching `Task <N>` — fix the plan; do not fall back to hand-assembling the
brief from the full `plan.md`. That fallback is exactly the context bloat the
brief exists to prevent.

Before dispatching, confirm the generated brief carries its own file
inventory — the `**Files:**` and `**Interfaces:**` blocks. A brief without
them makes the implementer re-derive scope it should have been handed, which
inflates its context before a single edit happens. If a task's plan section is
missing that inventory, add it to the brief file yourself, once, in your own
context, before dispatching.

### Step 4c — Verification runs outside the implementer

`task-implementer` never runs `tools/verify.sh` itself — only module-local
build/test. After an implementer reports `DONE` or `DONE_WITH_CONCERNS`,
dispatch `task-verifier` (`model: haiku`) with the worktree absolute path and
the command to run, defaulting to:

```sh
tools/verify.sh --quick
```

Running the gate in `task-verifier`'s own clean context instead of inside the
implementer costs a fraction of the tokens, and the build/vet/lint output
never lands in the implementer's window.

**A `--quick` PASS is a per-task gate, not authorization to call the branch
done.** `--quick` skips the container-build gate and both cluster dry-runs.
Only a flagless `tools/verify.sh` run, exit 0, at the end of the branch —
per [`docs/verification.md`](../../docs/verification.md) — authorizes "done".
Run that flagless pass once, in `superpowers:finishing-a-development-branch`,
never per task.

On a verdict: **PASS** → ledger it and move to the per-task review. **FAIL** →
the quoted failing block becomes a review finding; feed it into the existing
fix loop rather than fixing it yourself in the controller session. **ERROR**
(wrong tree, command not found, timeout) → the gate did not run; resolve the
cause and re-dispatch. Never treat `ERROR` as `PASS`.

The per-task review agent is `task-reviewer` (`model: sonnet`), dispatched
for the task's commit range once `task-verifier` has passed — never a bare
`general-purpose` dispatch for this. It writes its verdict to
`docs/tasks/<id>/reviews/<unit>.md`; read that artifact only when the verdict
is not `APPROVED`, since `CHANGES_REQUIRED` already carries the fix brief in
its `blocking` lines. `task-reviewer` is distinct from the whole-branch
`plan-adherence-reviewer` invoked in Step 5, which writes
`docs/tasks/<id>/audit.md`.

### Step 4d — Handle `PARTIAL`

`task-implementer`'s tool-call cap is **120**, matching `CAP=120` in
`.claude/hooks/turn-budget.sh`; `.claude/hooks/turn-budget-guard.sh` denies
further tool calls once the count reaches **125**. `PARTIAL` is the designed
outcome when that cap is reached with work remaining — the implementer
commits what already works and hands back the rest. It is a signal the task
was mis-sized, not a failure to retry blindly, and not something to re-dispatch
the same agent into its now-large context to "finish."

On `PARTIAL`:

1. Read what landed. If it is coherent, it is already committed — leave it.
2. Ledger the `PARTIAL` status, the commit range, and what remains.
3. Write a narrowed continuation brief for just the remaining work (its own
   file inventory, not the whole original brief) and dispatch a **fresh**
   `task-implementer` with it, or amend the plan if the remaining scope no
   longer fits the task as written.
4. Verification and review still run once over the whole task's commit
   range, not once per segment.

Two `PARTIAL`s on one task means the plan under-decomposed it — rule on the
split, ledger the ruling, and carry it forward.

### Step 4e — Hand off your own context

Steps 4a-4d bound every subagent's context; nothing bounds yours, and you are
the one context that survives the entire plan — every implementer report,
every review, every ledger update accumulates in it. At every durable
boundary (a task completing, a gate reconciling), ask whether the next unit
depends on this conversation's history or only on repository state. If repo
state suffices, write a one-paragraph diagnosis of what remains into the task
folder and tell the user to `/clear` and re-run `/execute-task <task-id>` —
resumption reads the ledger and the task folder, not this context. See
[`docs/agent-dispatch.md`](../../docs/agent-dispatch.md) §Context handoff for
the cost rationale.

### Step 4f — Record what each agent cost

When you reconcile an agent (implementer, verifier, reviewer), append one line
to the task ledger:

```sh
tools/agent-ledger.sh append <task> --unit "Task <N>" --agent-type <type> \
  --model <model> --status <status> --commit <sha>
```

Reviewer rows add `--verdict <verdict> --caused-fix <yes|no>`; a Step 4e
handoff adds `--kind handoff --context-tokens <n>`. Pass only what you
actually know — an omitted field stays `-`; do not estimate a count you did
not measure. Batch the append with the read of the gate log or review
artifact whose verdict it records — same tool call, not two.

### Step 5 — On completion

After all plan tasks complete and verify, the chosen skill hands off to `superpowers:finishing-a-development-branch`. Honor that handoff. Then suggest:

> All plan tasks complete. Recommend running `superpowers:requesting-code-review` next, which dispatches the plan-adherence-reviewer agent (plus any guideline reviewers once they're defined).

## Important Rules

- The worktree was created by `/spec-task`. NEVER create a new one here.
- Never start implementation outside the task worktree.
- Follow plan steps exactly; stop and ask when blocked rather than guessing.
- Run the verification commands the plan specifies; don't claim completion based on assumption.
