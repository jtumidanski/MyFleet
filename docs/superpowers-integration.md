# Superpowers Integration — When to Use What

This document is the quick-reference companion to `CLAUDE.md`. It tells you
which command, agent, or skill to reach for in each situation.

This document owns *which* command, agent, or skill to reach for in a given
situation. `docs/agent-dispatch.md` owns *how* to dispatch any agent — model,
budget, isolation, handoff.

## The Four-Phase Workflow

| Phase | Command | What it does | Output |
|---|---|---|---|
| 1. Requirements | `/spec-task <idea>` | Interactive PRD interview | `docs/tasks/task-NNN-slug/prd.md` |
| 2. Design | `/design-task <task-folder>` | Architecture, alternatives, tradeoffs (via `superpowers:brainstorming`) | `design.md` |
| 3. Plan | `/plan-task <task-folder>` | Bite-sized TDD step-by-step plan (via `superpowers:writing-plans`) | `plan.md` + `context.md` |
| 4. Execute | `/execute-task <task-folder>` | Subagent-driven implementation (via `superpowers:subagent-driven-development`) | code + commits |

Run `/clear` between phases 1–4. Each command consumes only the prior phase's
documented artifacts.

Work after implementation — PR validation, live testing, debugging, and
follow-up fixes — is not a `/clear` boundary and does not require one: diagnose
into a durable file under the task folder, then dispatch a fresh implementer
against it rather than continuing inline. This exists because the flow can
otherwise stop at phase 4 while the work does not: a measured task in a
sibling repository spent 12.7% of its entire budget after the PR opened, at
94% main thread across three subagents and four sessions. See
[`docs/post-implementation.md`](post-implementation.md) for the mechanics of
that loop.

### Task resolution

Phase commands accept fuzzy task identifiers: `task-054-slug`, `task-054`,
`054`, and `54` all resolve to the same folder.

- Resolution is the fuzzy-identifier algorithm built into the phase commands
  themselves: glob `docs/tasks/task-*` and `.worktrees/*/docs/tasks/task-*`,
  match on the numeric prefix or the full slug. Zero matches → ask the user.
  Multiple matches → list the candidates and let the user pick.
- `tools/task-numbers.sh next` picks the number for a new task; run
  `tools/task-numbers.sh check` before planning so a number does not collide
  with an in-flight task — it also runs automatically at `SessionStart`.
- Searching for a task artifact: search across all worktrees
  (`git worktree list`) before concluding a file is missing — every worktree
  carries its own copy of `docs/tasks/` from its branch point, so a hit in one
  worktree does not mean the artifact exists in the one you're working in.
- **Never act on a bare task number without resolving it first.** `054` and
  `45` are both plausible typos for the same or different tasks; resolve
  before reading or writing anything under `docs/tasks/`.

### Artifact location override

`superpowers:brainstorming` and `superpowers:writing-plans` default to
`docs/superpowers/specs/` and `docs/superpowers/plans/`. In this project both
go under `docs/tasks/task-NNN-slug/` instead. When invoking those skills
directly, outside the phase commands, pass the task folder explicitly so
artifacts land in the right place. The override applies equally whether the
skill is reached through a phase command or invoked standalone — there is no
case where the default location is correct for this repo.

### Phase 4 context budget

`task-implementer` replaces `general-purpose` for every Phase 4
implementation dispatch. Its contracts override the plugin's
`implementer-prompt.md` where they disagree. Its contract — the 120-call
budget, the verification split, front-loaded file inventory — is owned by
[`docs/agent-dispatch.md`](agent-dispatch.md).

## Code Review

### Picking the roster

`superpowers:requesting-code-review` (or `/audit-plan`) dispatches the
appropriate subset of reviewers based on what changed, rather than requiring
it to be derived by hand on every call:

- Go files changed → `backend-guidelines-reviewer`
- `apps/web` TypeScript/React files changed → `frontend-guidelines-reviewer`
- a `plan.md` exists for the task → `plan-adherence-reviewer`
- a single commit range needs per-unit review → `task-reviewer`, see
  [docs/agent-dispatch.md](agent-dispatch.md)'s per-unit review section

All three run in parallel when both backend and frontend changed. Each
agent's own `## Scope` section is the contract for what it checks; you do not
need to restate the checklist in the dispatch prompt.

### What a reviewer returns

Every reviewer writes its full reasoning to a durable artifact and returns a
compact verdict-first block. The contract, the verdict semantics, and the
controller's read rule are in [`docs/review-protocol.md`](review-protocol.md).
Short version: `verdict` is the first line, blocking findings are enumerated
with `file:line`, everything else is a count, and the controller opens the
artifact only when the verdict is not `APPROVED`.

All reviewers are **scoped to the change under review**: the diff is the
review surface, repo surveying is off, and anything a reviewer could not
evaluate within that surface is reported under `## Not evaluable from the
diff` rather than passed silently.

`plan-adherence-reviewer`, `backend-guidelines-reviewer`, and
`frontend-guidelines-reviewer` write to `docs/tasks/task-NNN-slug/audit.md`
(backend also writes `audit.json`); `task-reviewer` writes per-commit-range
reviews to `docs/tasks/task-NNN-slug/reviews/<unit>.md`. See
[`docs/review-protocol.md`](review-protocol.md) §Two artifacts, not one for
why they are kept apart.

Code review is mandatory before opening a PR and is a **different gate**
from verification: a green `tools/verify.sh` does not mean the branch is
correct. Every service can build, vet, test, and lint clean while the branch
carries a cross-service defect, because each service is self-consistent in
isolation. The gate cannot see a producer changing a Kafka event's shape
while a consumer still expects the old one, or a JSON:API relationship a
service resolves that never got a corresponding test. When a change crosses
a service boundary, trace the event or field into its consumers by hand and
check that a test asserts the new contract, not the old behavior.

## Maintenance Commands

| Command | What it does | Underlying agent |
|---|---|---|
| `/review-todos` | Whole-codebase TODO/FIXME scan; updates `docs/TODO.md` | `todo-scanner` |
| — | Generates/updates documentation for one service under `apps/<svc>/docs/` | `service-documentation` |

## Domain Skills

These activate via the project hook (`skill-activation-prompt.py`) when you
mention relevant keywords or work on relevant files:

- `backend-dev-guidelines` — Go service patterns
- `frontend-dev-guidelines` — React/TypeScript patterns

The hook produces a visible skill-activation banner. Heed it before
responding.

## Superpowers Skills (Self-Activating)

Reach for these explicitly when relevant; they also self-activate via Claude's
native skill matching:

- `using-superpowers` — invoke at the start of any conversation
- `brainstorming` — used inside `/design-task`
- `writing-plans` — used inside `/plan-task`
- `subagent-driven-development` — used inside `/execute-task`
- `executing-plans` — fallback for inline execution
- `systematic-debugging` — for any bug, test failure, or unexpected behavior
- `test-driven-development` — when implementing any feature or bugfix
- `verification-before-completion` — before claiming work is complete
- `using-git-worktrees` — for isolated workspaces
- `finishing-a-development-branch` — when implementation is complete and tests pass
- `requesting-code-review` — used at the end of a chunk of work
- `receiving-code-review` — when processing review feedback
- `dispatching-parallel-agents` — used by code-review orchestration
- `writing-skills` — when authoring new skills

`disableBundledSkills: true` in `.claude/settings.json` disables only Claude
Code's *bundled* skills (`dataviz`, `claude-api`, `design`, `update-config`,
…). The `superpowers` plugin is a plugin-provided skill set, not a bundled
one, so it stays enabled and every skill above and every phase command keeps
working regardless of that setting.

## When NOT to Use Superpowers

- **Trivial fixes** (typo, version bump, one-line change) — skip `/spec-task`; no PRD needed, but document the change directly via a brainstorming session before committing.
- **Documentation-only updates** that don't need a PRD — go straight to editing.

## File Locations Cheat Sheet

| Artifact | Location |
|---|---|
| PRD, design, plan, context, audit | `docs/tasks/task-NNN-slug/` |
| Audit JSON output (backend) | `docs/tasks/task-NNN-slug/audit.json` |
| Per-unit reviews | `docs/tasks/task-NNN-slug/reviews/<unit>.md` |
| Per-service docs | `apps/<service>/docs/` |
| TODO list | `docs/TODO.md` |
| Web frontend | `apps/web/` |
