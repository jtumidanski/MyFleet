# Context — Process Parity Phase 2 (MyFleet)

Companion to `plan.md`. Everything here was verified against source in this
worktree or in `$ATLAS`, not recalled.

---

## 1. Where things are

| Thing | Path | Note |
|---|---|---|
| This worktree | `.worktrees/task-032-process-parity-phase-2` | branch `task-032-process-parity-phase-2` |
| Atlas source (`$ATLAS`) | `~/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename` | unmerged atlas branch, pinned at `e83f59e61` |
| Canonical spec | `docs/process-parity.md` | verbatim copy of atlas's; `diff` against `$ATLAS` is **empty** as of design time |
| What applies here | `docs/tasks/task-032-process-parity-phase-2/brief.md` | MyFleet's §4 binding row |
| PRD | `docs/tasks/task-032-process-parity-phase-2/prd.md` | FR-1 … FR-11, AC-1 … AC-19 |
| Design | `docs/tasks/task-032-process-parity-phase-2/design.md` | decisions D1–D4, questions Q1–Q5 resolved |

**Re-confirm the pin before copying anything:**

```sh
export ATLAS=~/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename
diff "$ATLAS/docs/process-parity.md" docs/process-parity.md   # must be empty
```

A non-empty diff means atlas changed the spec after this plan was written. Stop
and re-sync; do not merge the two by hand.

---

## 2. Verified repository facts

Checked in this worktree, with the file and line where the fact lives:

| Fact | Value | Source |
|---|---|---|
| `make ci` targets | `lint-check vet test build fe-test fe-build manifests carfax-template` | `Makefile:51` |
| Manifest check renders both overlays | yes — `for overlay in main local`; asserts the `main` invariants; does **not** run `kubectl` | `tools/check-manifests.sh:24-62` |
| Pinned linter | `GOLANGCI_LINT_VERSION=v2.13.1` | `tools/lint.versions` |
| Lint cache layout | `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION` — same shape as atlas's | `tools/lint.sh:56-57` |
| Lint runs in workspace mode | root `go.work` active; guard never requires `go work sync` | `tools/lint.sh:1-16` |
| Lint path scoping | trailing paths restrict Go module discovery | `tools/lint.sh:29-31` |
| Prettier scope | configured at the **repo root**, `npm run format:check` with cwd `$ROOT`, covering every workspace | `tools/lint.sh:184` |
| Dockerfiles | `apps/{auth-service,fleet-service,media-service,notification-service,web}/Dockerfile` | `ls apps/*/Dockerfile` |
| Existing tools | `check-carfax-template.sh`, `check-manifests.sh`, `generate-icons.{py,sh}`, `lint.sh`, `lint.versions`, `task-numbers.sh` | `ls tools/` |
| Existing hooks | `skill-activation-prompt.{py,sh}`, `task-num-collision-detector.sh` | `ls .claude/hooks/` |
| Existing agents | `backend-guidelines-reviewer`, `frontend-guidelines-reviewer`, `plan-adherence-reviewer`, `todo-scanner` | `ls .claude/agents/` |
| Existing commands | `audit-plan`, `design-task`, `execute-task`, `plan-task`, `review-todos`, `spec-task` | `ls .claude/commands/` |
| `.gitignore` | already ignores `.cache/`; does not mention `.superpowers/` — and does not need to (`task-brief.sh` writes a self-ignoring `.gitignore`) | `.gitignore` |
| On PATH | `jq`, `kustomize`, `kubectl`, `docker`, `npm` all present; `npm` resolves to Node **v24**, not the targeted 22 | design §0 |

---

## 3. Key decisions carried from design

| # | Decision | Consequence in the plan |
|---|---|---|
| **D1** | Port `tools/doc-slice.sh` and `tools/agent-ledger.sh` (with tests) beyond the PRD's two-tool list. Do **not** port `task-resolve.sh`, `task-facts.sh`, `change-surfaces.sh` — rebind their *references* to MyFleet-native commands. | Task 1 copies them; Tasks 6 and 11 rebind the three that were not ported. Without this, FR-7.2 ("don't delete the rule") and AC-17 ("no dangling paths") cannot both hold for `slice-first.md` and `/execute-task`. |
| **D2** | `tools/verify.sh` is a thin gate runner (~200 lines), not atlas's 746-line change-detection engine. Exactly three flags. Two *separate* dry-run gates. Never re-implements the manifest invariants. `VERIFY_DRY_RUN=1` short-circuit for the test. | Task 3 |
| **D3** | `format-on-write.sh` is rebound, not copied. Prettier match is a deliberate **superset** of FR-2.1: `apps/web` **and** `packages/*/src`, run from `$ROOT`. | Task 2, recorded as a deviation in Task 14's report |
| **D4** | Bootstrap Node only when `npm` is absent (FR-5.10, literally). Add a **non-fatal warning** when `npm` is present at a non-22 major — otherwise a wrong-major `fe-test` failure looks like a code defect. | Task 3, recorded as a deviation |
| **Q1** | Port `DOCS.md`, genericized. Per-service docs at `apps/<svc>/docs/`; the REST section is rebound to the hand-rolled JSON:API transport in `packages/shared-go/server`. **Scope guard: this task creates the contract only — no service documentation is written.** | Task 7 |
| **Q2** | `task-reviewer` writes `docs/tasks/<task>/reviews/<unit>.md` — atlas's own convention, adopted unchanged. It does **not** share `docs/tasks/<task>/audit.md`, which MyFleet's three existing reviewers own. Sharing one file would make per-unit reviews clobber each other. | Tasks 5, 8 |
| **Q3** | `turn-budget.sh` counters live at `${TMPDIR:-/tmp}/claude-turn-budget/<key>`, keyed on harness-issued agent/session ids, not cwd or repo path. Two concurrent worktrees produce disjoint keys; nothing is written inside the repo. **Copy verbatim; no adaptation.** | Task 1 |
| **Q4** | `disableBundledSkills: true` governs Claude Code's *bundled* skills, not plugin-provided ones. Verified by precedent: `$ATLAS/.claude/settings.json` sets it alongside `enabledPlugins` and atlas's phase commands invoke `superpowers:*` normally. | Task 12 |
| **Q5** | Repo documents state repository-scoped mechanics and **defer** to the user's global `~/.claude/CLAUDE.md` rather than restating it. One MyFleet rule atlas's `git-workflow.md` lacks: the **shared-stash hazard** across concurrent worktrees. | Task 6 |

---

## 4. Dependency order and why

```
1 byte-copies ──┬──────────────────────────────► 12 settings wiring
2 format-on-write ┘                                     │
3 verify.sh + test ──┬──► 4 verification.md ──┐         │
                     │    5 dispatch/review/sp │         │
                     │    6 five smaller docs ─┤         │
                     └──► 8 agent trio ◄───────┤         │
                          7 DOCS.md ──► 9 svc-doc        │
                          10 /fix-pr-bug ◄────┘          │
                          11 /execute-task ◄── 1, 8      │
                                                          ▼
                                        13 CLAUDE.md ──► 14 checks + gate
```

Three orderings are load-bearing and must not be reordered:

- **`CLAUDE.md` is last.** Its owner table must resolve every row to an existing
  file (FR-10.2). Writing it earlier guarantees a window where AC-13 fails, and
  a dangling row silently deletes a procedure.
- **`DOCS.md` (Task 7) before `service-documentation` (Task 9).** The agent's
  authoritative inputs are `CLAUDE.md` **and** `DOCS.md`. Ported first, it would
  point at a missing document — the exact defect this ordering prevents.
- **Hook wiring (Task 12) after the hooks are proven (Task 1 Step 3).** From
  that commit onward every session in this repository runs under the hooks,
  including the one finishing this branch.

---

## 5. Hazards

**The hook-wiring hazard.** After Task 12, `wait-loop-guard.sh` blocks `sleep`
and polling, `block-home-paths-in-docs.sh` denies any `docs/` write containing
`/home/<user>/`, `turn-budget-guard.sh` denies subagent tool calls past 125, and
`fork-dispatch-guard.sh` guards `Agent` dispatches. That is the point of the
task. It also means a misfiring hook degrades *all* future work in this repo, not
just this branch — which is why Task 12 is a single isolated commit whose
recovery is a one-commit revert.

**The rebind hazard — the real risk in this task.** 1,519 lines of atlas owner
docs shrink under genericization, and "shorter" and "missing a rule" look
identical in a diff. No mechanical check catches a lost rule. The mitigations are
Task 13 Step 4's item-by-item `check` loop (21 assertions, AC-15) and the
per-document line-count delta reported in Tasks 5 and 6, where any document below
60% of its atlas line count needs a written justification.

**Home paths in `docs/`.** `plan.md`, `context.md`, and every owner document live
under `docs/`. Never write a literal `/home/<user>/...` into them — use `$ATLAS`,
`~`, or a repo-relative path. Before Task 12 this is a gitleaks hazard; after it,
the write is denied outright.

**The shared stash.** This worktree shares its stash stack with the main checkout
and every other worktree, and other sessions may push or pop concurrently. Never
bare `git stash` / `git stash pop`. Prefer a WIP commit.

---

## 6. Contracts other tasks depend on

Defined once, quoted many times. Changing one means changing every quote.

| Contract | Value | Defined | Quoted by |
|---|---|---|---|
| Implementer cap | `CAP=120` (guard denies at 125) | `.claude/hooks/turn-budget.sh` (Task 1) | Tasks 8, 11 |
| Non-authorization sentence | `this run does NOT authorize calling the branch done` | `tools/verify.sh` (Task 3) | Tasks 3 test, 4, 13 |
| Success sentence | `All gates passed — the branch may be called done.` | `tools/verify.sh` (Task 3) | Tasks 3 test, 14 |
| Verify flags | flagless / `--quick` / `--no-docker` / `-h`\|`--help`; unknown → exit 2 | `tools/verify.sh` (Task 3) | Tasks 4, 8, 10, 11 |
| Brief tool | `tools/task-brief.sh PLAN_FILE TASK_NUMBER [OUTFILE]`; exit 0/2/3 | copied (Task 1) | Tasks 8, 11 |
| Per-task review artifact | `docs/tasks/<id>/reviews/<unit>.md` | Task 5 | Task 8 |
| Whole-task audit artifact | `docs/tasks/<id>/audit.md` | already exists | Tasks 5, 14 |
| Service doc location | `apps/<svc>/docs/` | `DOCS.md` (Task 7) | Task 9 |

---

## 7. The MyFleet carve-out (FR-11.1)

Atlas's spec §7 check 3 exempts *two* files from the "no `atlas-*` references"
rule. **In MyFleet only `docs/process-parity.md` is exempt.** The
`docs/agent-dispatch.md` exemption is atlas-only — atlas is the only repository
that ever used the `atlas-*` agent names, so it is the only one that needs a
historical-cutoff note. MyFleet's ported `docs/agent-dispatch.md` must contain no
`atlas-*` reference at all.

The check, which must print nothing:

```sh
git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
  | grep -vxE 'docs/process-parity\.md'
```

---

## 8. What is explicitly out of scope

- Any change under `apps/` or `packages/`. Verify with
  `git diff --name-only main...HEAD | grep -E '^(apps|packages)/'` → nothing.
- Writing documentation for any service (`apps/*/docs/` content). Task 7 creates
  the contract; filling it is a follow-up task.
- Porting `docs/packets/`, `docs/reverse-engineering.md`,
  `docs/adding-a-new-service.md`, `docs/observability.md`. MyFleet's deploy
  runbook already lives at `docs/runbooks/`.
- Recreating what MyFleet already has: `tools/task-numbers.sh`,
  `task-num-collision-detector.sh`, the four phase commands, `/audit-plan`,
  `/review-todos`, the four existing reviewer agents, both guideline skills,
  `skill-rules.json`, the `skill-activation-prompt` hook.
- Any sync mechanism between the four repositories. Each repo ends up
  self-contained; drift is accepted and re-harmonized occasionally.
- Evaluating §7 checks 1, 4, 5 and 6. They are cross-repo pairwise comparisons,
  not evaluable from MyFleet alone. Task 14 reports MyFleet's side and says
  plainly that the comparison was not performed here. **Do not claim they pass.**
