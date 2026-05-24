# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MyFleet is a multi-service project: one or more **Go** backend services plus a **React/TypeScript** web UI. The repository is currently unscaffolded — service layout, module structure, and build tooling will be decided during the first task. Update this file (build commands, service paths, conventions) once those are settled.

## Workflow Rules

When asked to understand or plan something, DO NOT start implementing code changes. Wait for explicit approval before making any edits. Planning and implementation are separate phases.

## Build & Verification

Once a build system is in place, this section should enumerate the commands required before claiming a branch is "done". As a placeholder, the general expectation for any Go service is `go test -race ./...`, `go vet ./...`, and `go build ./...` clean; for the React UI, `npm run build` and `npm test` clean. Add Docker-build verification once a container build is wired up.

## Code Patterns

When refactoring shared types or creating common libraries, prefer straightforward moves over re-exporting type aliases. Keep abstractions clean — don't break service boundaries by having one layer call another's internals directly.

## Development Workflow

The canonical flow for any non-trivial change is four phases. **`/spec-task` creates a dedicated worktree at `.worktrees/task-NNN-slug/` on a `task-NNN-slug` branch; all subsequent phases run inside that worktree** so docs, code, and the eventual PR are one unit. Each phase is a separate slash command, invoked from a fresh (`/clear`'d) session so the next phase consumes only the prior phase's documented artifacts:

1. `/spec-task <idea>` — run from the main repo. Interactive PRD interview that creates the worktree + branch and commits the PRD. Output: `<worktree>/docs/tasks/task-NNN-slug/prd.md`.
2. `cd .worktrees/task-NNN-slug`, `/clear`, then `/design-task <task-id>` — invokes `superpowers:brainstorming`. Output: `design.md` (committed on the task branch).
3. `/clear`, then `/plan-task <task-id>` — invokes `superpowers:writing-plans`. Output: `plan.md` + `context.md` (committed).
4. `/clear`, then `/execute-task <task-id>` — invokes `superpowers:subagent-driven-development`. Reuses the existing worktree; never creates a new one.

Phase commands accept fuzzy task identifiers: `task-001-slug`, `task-001`, `001`, or `1` all resolve to the same folder. They search both `docs/tasks/` (main) and `.worktrees/*/docs/tasks/` to locate the task.

Task numbers are assigned by `tools/task-numbers.sh next` (single source of truth). A SessionStart hook runs `tools/task-numbers.sh check` and surfaces any task-number collisions before they compound.

Skip `/spec-task` only for trivial fixes that don't warrant a PRD; document those directly via a brainstorming session.

### Artifact Location Override

Both `superpowers:brainstorming` and `superpowers:writing-plans` default to `docs/superpowers/specs/` and `docs/superpowers/plans/`. **In this project, both go under `docs/tasks/task-NNN-slug/` instead.** When invoking those skills directly (outside the phase commands), pass the task folder explicitly so artifacts land in the right place.

### Code Review Pattern

Code review uses three modular reviewer agents, dispatched in parallel:

- `plan-adherence-reviewer` — verifies plan tasks were actually implemented
- `backend-guidelines-reviewer` — Go DOM-* / SUB-* / SEC-* checklist (when Go files changed)
- `frontend-guidelines-reviewer` — React/TS FE-* checklist (when frontend files changed)

Invoke via `superpowers:requesting-code-review` (it dispatches the appropriate subset), or invoke an individual agent directly for ad-hoc checks. Each agent writes findings to `docs/tasks/task-NNN-slug/audit.md`.

The backend and frontend reviewer checklists are sourced from the `backend-dev-guidelines` and `frontend-dev-guidelines` skills in `.claude/skills/`. The `skill-activation-prompt` hook (wired in `.claude/settings.json`) auto-suggests those skills based on file/intent triggers configured in `.claude/skills/skill-rules.json`.

## Design/Plan Output Style

- When producing design.md or plan.md documents, write the full document directly to the file. Do NOT walk through sections interactively or ask for per-section approval. The user will read the committed file.

## Worktree Discipline

- Tasks live in git worktrees (siblings of the main repo under `.worktrees/`). Before planning/designing/executing a task, verify cwd is the correct worktree; if not, `cd` into it yourself rather than asking the user.
- When searching for task PRDs/plans/designs, search across all worktrees (`git worktree list`) before concluding a file is missing.
- Never edit files in the main repo when a task worktree exists for that work.

## Code Review Before PR

- Always run the code-review step (`/audit-plan` or `superpowers:requesting-code-review`) before opening a PR. Do not skip even when the task plan looks complete.

## Verification Over Memory

- For API contracts, configuration values, and service-to-service interactions, verify against local source rather than citing values from memory or general knowledge.
- When uncertain about behavior, read the source rather than speculating.
