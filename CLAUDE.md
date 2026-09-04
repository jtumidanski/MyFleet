# MyFleet

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Never do this

- Never commit or push directly to `main`.
- Never invent a value, name, output, or behavior — verify against source or tool output instead.
- Never claim a branch is verified from a flagged (`--quick`/`--no-docker`) or partial run; see "Done means verified."
- Never open a PR without code review.
- Never dispatch an agent without an explicit `model`.
- Never land a placeholder comment or a stubbed handler.
- Never spend inference turns polling a process or waiting on a child agent *(enforced by `.claude/hooks/wait-loop-guard.sh`)*.
- Never edit files in the main repo when a task worktree exists for that work.

## Evidence & grounding

- For API contracts, configuration values, and service-to-service interactions, verify against local source rather than citing values from memory or general knowledge; when uncertain about behavior, read the repo source rather than speculating.
- Unverified is "unknown / unverified," never a plausible guess.
- Quote actual tool output before concluding a check passed or failed.
- Sweep a check across every match, don't spot-check a sample.
- Finish producible work rather than declaring a "follow-up" for a prerequisite you can produce yourself.

## Development workflow

When asked to understand or plan something, DO NOT start implementing code changes. Wait for explicit approval before making any edits — planning and implementation are separate phases.

The canonical flow for any non-trivial change is four phases. `/spec-task` creates a dedicated worktree at `.worktrees/task-NNN-slug/` on a `task-NNN-slug` branch; all subsequent phases run inside that worktree so docs, code, and the eventual PR are one unit. Each phase is a separate slash command, invoked from a fresh (`/clear`'d) session so the next phase consumes only the prior phase's documented artifacts:

1. `/spec-task <idea>` — run from the main repo. Interactive PRD interview that creates the worktree + branch and commits the PRD.
2. `cd .worktrees/task-NNN-slug`, `/clear`, then `/design-task <task-id>` — invokes `superpowers:brainstorming`.
3. `/clear`, then `/plan-task <task-id>` — invokes `superpowers:writing-plans`.
4. `/clear`, then `/execute-task <task-id>` — invokes `superpowers:subagent-driven-development`. Reuses the existing worktree; never creates a new one.

Phase commands accept fuzzy task identifiers: `task-001-slug`, `task-001`, `001`, or `1` all resolve to the same folder. Skip `/spec-task` only for trivial fixes that don't warrant a PRD.

Task numbers are assigned by `tools/task-numbers.sh next` (single source of truth); see `docs/superpowers-integration.md` for the full phase-command and skill-invocation mechanics, including the `docs/tasks/task-NNN-slug/` artifact-location override for `superpowers:brainstorming` and `superpowers:writing-plans`.

Tasks live in git worktrees (siblings of the main repo under `.worktrees/`). Before planning/designing/executing a task, verify cwd is the correct worktree; if not, `cd` into it yourself rather than asking the user.

When searching for task PRDs/plans/designs, search across all worktrees (`git worktree list`) before concluding a file is missing.

When producing `design.md` or `plan.md` documents, write the full document directly to the file. Do NOT walk through sections interactively or ask for per-section approval — the user reads the committed file.

## Done means verified

Before calling a branch done, ready for PR, or invoking `superpowers:finishing-a-development-branch`, the **flagless** `tools/verify.sh` must exit 0. `--quick`/`--no-docker` exit 0 too but do not count as done — see `docs/verification.md`.

`make ci` runs `lint-check vet test build fe-test fe-build manifests carfax-template`; render and dry-run the deploy manifests against **both** `deploy/k8s/overlays/main` and `deploy/k8s/overlays/local` (rendering alone missed a real `overlays/local` namespace bug in the past — see `docs/verification.md` for the incident and the node/nvm bootstrap). Container build context is the repo root for every service, `apps/web` included: `docker build -f apps/<service>/Dockerfile .`.

Always run the code-review step (`/audit-plan` or `superpowers:requesting-code-review`) before opening a PR — do not skip even when the task plan looks complete.

## Dispatching agents

The model pin follows the job. Fan out with fresh-context agents; fork only to continue an interactive debugging thread, and say why inline. Per-unit review is `task-reviewer`, never a bare `general-purpose` dispatch. Read `docs/agent-dispatch.md` before dispatching and `docs/review-protocol.md` before dispatching a reviewer.

Code review uses three modular reviewer agents, dispatched in parallel: `plan-adherence-reviewer`, `backend-guidelines-reviewer`, and `frontend-guidelines-reviewer`, triggered by `.claude/skills/skill-rules.json`.

## Handing off context

Before a session runs out of usable context, hand off rather than pushing through degraded reasoning: write a short diagnosis and next steps into the task folder so the handoff is lossless even though the reasoning behind it does not survive in the transcript. A handoff the same context then works past is not a handoff. See `docs/agent-dispatch.md` for the full context-handoff mechanics, the `--kind handoff --context-tokens <n>` ledger record, and how `/execute-task` Steps 4d-4e implement `PARTIAL` handling and controller handoff as concrete instances of this rule.

## Repository conventions

- Check `packages/shared-go/` before defining a new domain type, alias, or numeric constant.
- When refactoring shared types or creating common libraries, prefer straightforward moves over re-exporting type aliases. Keep abstractions clean — don't break service boundaries by having one layer call another layer's internals directly.
- Use repo-relative paths in committed files, never a literal home path.
- Preserve existing line endings.
- Ask the tooling for a mechanical fact rather than deriving it by hand — see `docs/tooling-conventions.md`.
- Slice a large artifact before reading it whole — see `docs/slice-first.md`.
- Batch a gate-log read with the ledger append recording its verdict.
- Never bare `git stash` / `git stash pop` — the stash stack is shared across worktrees; see `docs/git-workflow.md`.

## Where the procedures live

Load the owning document before acting in its area; it holds the mechanics this file omits.

| Trigger | Owner |
|---|---|
| Dispatching any agent, or deciding whether to hand off | [docs/agent-dispatch.md](docs/agent-dispatch.md) |
| Dispatching a reviewer, or writing up a review | [docs/review-protocol.md](docs/review-protocol.md) |
| A `tools/verify.sh` gate failed, or script and CI disagree | [docs/verification.md](docs/verification.md) |
| A bare task number, or a superpowers skill outside a phase command | [docs/superpowers-integration.md](docs/superpowers-integration.md) |
| Committing, pushing, rebasing; a stray `main` commit, a shared-worktree stash | [docs/git-workflow.md](docs/git-workflow.md) |
| A long-running process, a mechanical repo fact, shell/editing conventions | [docs/tooling-conventions.md](docs/tooling-conventions.md) |
| About to read a large document, diff, plan, or tool result | [docs/slice-first.md](docs/slice-first.md) |
| The PR is open and something is wrong (phase 5, `/fix-pr-bug`) | [docs/post-implementation.md](docs/post-implementation.md) |
| About to dispatch a second implementer at the same transformation | [docs/codemod-vs-agents.md](docs/codemod-vs-agents.md) |
| Writing or updating a service's documentation | [DOCS.md](DOCS.md), `service-documentation` agent, `/service-doc` |
| Deploying, or a wedged cluster | [docs/runbooks/](docs/runbooks/) |
| Writing or changing a Go service | `backend-dev-guidelines` skill, `backend-guidelines-reviewer` agent |
| Writing or changing the web frontend | `frontend-dev-guidelines` skill, `frontend-guidelines-reviewer` agent |
| A cross-repository process-parity question | [docs/process-parity.md](docs/process-parity.md) |
