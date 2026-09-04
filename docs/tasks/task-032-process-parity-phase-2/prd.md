# Process Parity Phase 2 (MyFleet) — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-26
---

## 1. Overview

MyFleet already shares the *workflow* backbone with `atlas`, `home-hub`, and
`Harbormaster`: the four phase commands, the worktree convention, the
artifact-location override, three reviewer agents, and two guideline skills.
What it does not share is the *context-discipline* layer that `atlas` grew
afterwards — the enforcement hooks, the owner-doc set, the budget-capped
implementer, the isolated verifier, and the terse rule-list `CLAUDE.md` shape
that an owner-doc table makes possible.

This task executes `docs/process-parity.md` §6 step 2 for MyFleet: port that
layer in, bound to MyFleet's own paths and gates per the §4 MyFleet row. The
governing documents are `docs/process-parity.md` (canonical spec, a verbatim
copy of atlas `e83f59e61`) and `docs/process-parity-brief.md` (what applies here
and in what order). Both are committed in this repository, so this task is
self-contained.

The only genuine engineering is `tools/verify.sh`. Everything else is porting
text — but porting it *faithfully*, which means rebinding atlas-specific paths
and replacing atlas-specific examples rather than deleting the rules they
illustrate.

### Provenance and re-sync

The pinned copy of `docs/process-parity.md` was stale when this task began
(pinned `e75c2a168`, atlas HEAD `e83f59e61`). It was re-synced verbatim before
phase 1 proceeded. The single intervening atlas commit added a five-line
evidence note to §7 check 3 recording that `atlas-*` agent names were verified
absent from the three target repositories; it touched no portable file and
changed no MyFleet obligation.

## 2. Goals

Primary goals:

- Every §3 artifact exists in MyFleet, bound to MyFleet's paths.
- The seven §3.1 portable hooks are **byte-identical** to atlas's.
- `tools/verify.sh` exists and honors the cross-repo contract: a flagless run
  exiting 0 means the branch may be called done; `--quick` / `--no-docker` also
  exit 0 but skip slow gates and do **not** count as done.
- The agent trio is not merely present but **actually dispatched** — `/execute-task`
  is rewritten to orchestrate `task-implementer` / `task-verifier` / `task-reviewer`,
  which is what makes the turn-budget hooks and `commit-boundary.sh` meaningful.
- `CLAUDE.md` is restructured into the eight-heading rule-list shape without
  losing any rule currently carried as prose.
- §7 checks 2 and 3 pass mechanically in this repository.

Non-goals:

- Any sync mechanism between the four repositories. §2 of the spec settles this:
  each repo ends up self-contained; drift is accepted and re-harmonized
  occasionally.
- Re-creating what MyFleet already has: `tools/task-numbers.sh`,
  `task-num-collision-detector.sh`, the four phase commands (`/execute-task` is
  rewritten, not created), `/audit-plan`, `/review-todos`, the four existing
  reviewer agents, the two guideline skills, `skill-rules.json`, and the
  `skill-activation-prompt` hook.
- Porting `docs/packets/`, `docs/reverse-engineering.md`,
  `docs/adding-a-new-service.md`, or `docs/observability.md`. MyFleet's deploy
  runbook already lives at `docs/runbooks/`.
- Changing any application code under `apps/` or `packages/`. This task touches
  `.claude/`, `docs/`, `tools/`, and `CLAUDE.md` only.
- Evaluating the four cross-repo §7 checks (1, 4, 5, 6) as pass/fail. They are
  pairwise comparisons and are not evaluable from MyFleet alone.

## 3. User Stories

- As a developer running `/execute-task`, I want each plan task implemented by a
  budget-capped subagent that hands back `PARTIAL` rather than silently
  sprawling, so a runaway task is visible instead of expensive.
- As a developer, I want verification to run in a clean context via
  `task-verifier`, so a green report is not an artifact of the implementer
  grading its own work.
- As a developer finishing a branch, I want one command — `tools/verify.sh` —
  whose exit 0 authorizes calling the branch done, instead of remembering a
  prose checklist.
- As a developer, I want `CLAUDE.md` to be a short rule list that points at
  owner documents, so the context cost of every session is small and the
  procedures are still discoverable.
- As a reviewer, I want `docs/` to answer "who owns this decision" so process
  questions are resolved by a document rather than re-litigated per task.
- As a developer who just got a bug report on an open PR, I want `/fix-pr-bug`
  so phase 5 is a defined procedure rather than an improvisation.

## 4. Functional Requirements

### 4.1 Portable hooks — copied verbatim

Copy from `$ATLAS/.claude/hooks/` into `.claude/hooks/`, byte-for-byte:

`wait-loop-guard.sh`, `wait-loop-guard_test.sh`, `block-home-paths-in-docs.sh`,
`turn-budget.sh`, `turn-budget-guard.sh`, `fork-dispatch-guard.sh`,
`commit-boundary.sh`

Requirements:

- FR-1.1 Each file is byte-identical to its atlas counterpart (`diff` empty).
- FR-1.2 `grep -l 'atlas-' .claude/hooks/*.sh` prints nothing.
- FR-1.3 All copied `.sh` files are executable.
- FR-1.4 `wait-loop-guard_test.sh` passes when run in this repository.
- FR-1.5 `commit-boundary.sh` names `tools/task-brief.sh` in its guidance text;
  that script must therefore exist (FR-4.2) or the guidance dangles.

### 4.2 `format-on-write.sh` — rebound, not copied

Atlas's version hardcodes `services/atlas-ui` for prettier and sources
`tools/toolchain.versions` for the pinned `golangci-lint`.

- FR-2.1 Rebind the prettier branch to `apps/web`.
- FR-2.2 Rebind the linter pin to `tools/lint.versions`, which defines
  `GOLANGCI_LINT_VERSION`. The hook must use the pinned binary only if
  `tools/lint.sh` has already bootstrapped it; the hook never bootstraps.
- FR-2.3 The hook exits 0 on every path it does not handle. A formatting hook
  must never block a write.

### 4.3 Agent trio and `/execute-task`

- FR-3.1 Port `task-implementer`, `task-verifier`, and `task-reviewer` from
  `$ATLAS/.claude/agents/`, rebinding atlas paths (`services/*`, `libs/*`,
  `services/atlas-ui`, atlas verify flags) to MyFleet's (`apps/*`,
  `packages/*`, `apps/web`, MyFleet's verify flags per §4.5).
- FR-3.2 `task-implementer` retains its 120 tool-call budget, its `PARTIAL`
  hand-back, its module-local build/test scope, and its brief-first discovery.
  The budget number must agree with what `turn-budget-guard.sh` enforces.
- FR-3.3 `task-verifier` runs the repo-wide gate in its own context, returns
  `PASS` or the first failing block, and never edits files.
- FR-3.4 `task-reviewer` reviews one commit range against its brief, writes a
  durable artifact, returns verdict-first, and does not fan out recursively.
- FR-3.5 Rewrite `.claude/commands/execute-task.md` to orchestrate the trio,
  porting atlas's structure. It must reuse the existing task worktree and never
  create a new one — MyFleet's current `/execute-task` guarantees this and the
  rewrite must not regress it.
- FR-3.6 Module-local build/test commands in the trio must be correct for
  MyFleet's `go.work` layout (`apps/*` + `packages/*`).

### 4.4 `service-documentation` and `/service-doc`

- FR-4.1 Port the `service-documentation` agent and the `/service-doc` command,
  resolving a service argument against `apps/` rather than `services/`.

> **Gap not named in the brief.** The agent's authoritative inputs are
> `CLAUDE.md` **and `DOCS.md`** — a root documentation contract that atlas has
> and MyFleet does not. Ported as-is, the agent would follow a document that
> does not exist. **Assumption taken:** port `DOCS.md` too, genericized to
> MyFleet's Go services under `apps/*` and the hand-rolled JSON:API transport in
> `packages/shared-go/server`, with per-service docs at `apps/<svc>/docs/`.
> MyFleet has no `apps/*/docs/` today, so this establishes the convention rather
> than extending one. Confirm during design (see §9).

### 4.5 `tools/verify.sh`

The one piece of real engineering. Contract, mirroring atlas:

- FR-5.1 Flagless run exits 0 ⟺ every gate that ran passed **and** the branch may
  be called done.
- FR-5.2 `--quick` and `--no-docker` exit 0 on success but skip slow gates and do
  **not** authorize "done". The output must say so.
- FR-5.3 `-h` / `--help` prints usage and exits 0. An unknown option exits 2.
- FR-5.4 Every gate runs even after an earlier one fails, so one pass gives the
  complete picture. Exit status is non-zero if any gate failed.
- FR-5.5 A final summary lists each gate as PASSED, FAILED, or SKIPPED with the
  reason for each skip.

Gates:

- FR-5.6 **`make ci`** — `lint-check vet test build fe-test fe-build manifests
  carfax-template`. Note that `make ci` already includes `manifests`, which runs
  `tools/check-manifests.sh`; that script already renders both overlays and
  asserts the `main`-overlay invariants (no PVCs, no Secrets, no ClusterRole, no
  placeholders). `verify.sh` must **not** re-implement those assertions.
- FR-5.7 **Container builds** — `docker build -f apps/<svc>/Dockerfile .` for
  each service with a Dockerfile (`auth-service`, `fleet-service`,
  `media-service`, `notification-service`, `web`), context the repo root. This
  mirrors `.github/workflows/pr.yml`, which builds images although `make ci`
  does not. Skipped by `--no-docker` and by `--quick`.
- FR-5.8 **Cluster dry-runs** — **both** of:

  ```sh
  kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
  kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
  ```

  Not present in CI, which has no cluster. Skipped by `--quick`.

- FR-5.9 **Unreachable cluster:** when no cluster is reachable, the dry-run gate
  is recorded SKIPPED with the attempted context named prominently in the
  summary, and the flagless run still exits 0. A skip that is invisible is the
  exact failure this gate exists to prevent, so it must be loud.
- FR-5.10 **Node bootstrap:** when `npm` is absent, bootstrap with
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` before the
  frontend gates. When `npm` is present, do not touch the environment.
- FR-5.11 `verify.sh` must not restate `docs/verification.md`. The script is the
  executable form of that document; rationale lives in the document.

### 4.6 `tools/task-brief.sh`

- FR-6.1 Port verbatim. Verified self-contained — its workspace resolution is
  inlined and it has no sibling-script dependency.
- FR-6.2 Its `.superpowers/sdd/` workspace must be git-ignored (the script
  writes a self-ignoring `.gitignore`; confirm this holds in MyFleet).

### 4.7 Owner documents

Port these nine into `docs/`, genericized per §5.2:

| Document | atlas lines | Owns |
|---|---|---|
| `agent-dispatch.md` | 244 | Model pinning, fan-out vs. fork, handoff decision |
| `verification.md` | 368 | Gate failures, script/CI disagreement |
| `superpowers-integration.md` | 185 | Bare task numbers, skills outside a phase command |
| `review-protocol.md` | 178 | Dispatching a reviewer, writing up a review |
| `post-implementation.md` | 160 | Phase 5, `/fix-pr-bug` |
| `codemod-vs-agents.md` | 138 | Second implementer at the same transformation |
| `slice-first.md` | 107 | Reading a large document, diff, plan, or tool result |
| `tooling-conventions.md` | 95 | Long-running processes, mechanical repo facts, shell conventions |
| `git-workflow.md` | 52 | Committing, pushing, rebasing, stray `main` commits |

- FR-7.1 Replace atlas-specific illustrations — packet work, WZ data, IDA,
  service opcodes, atlas `verify.sh` flag specifics — with MyFleet equivalents
  or neutral ones.
- FR-7.2 **Do not delete a rule because its example does not transfer.** Find a
  new example. A shorter ported document is expected; a document missing a rule
  is a defect.
- FR-7.3 `verification.md` must document MyFleet's actual gate set (§4.5),
  including the `--quick` / `--no-docker` non-authorization semantics and the
  unreachable-cluster skip.
- FR-7.4 `verification.md` (or another durable owner doc) must carry the
  **local-overlay incident**: a missing `namespace:` in
  `deploy/k8s/infra-local/kustomization.yaml` made `kubectl apply -k
  deploy/k8s/overlays/local` fail outright (`ClusterRoleBinding
  "myfleet-traefik" is invalid: subjects[0].namespace: Required value`) and
  slipped through ten reviews because only the `main` dry-run was ever run.
  This incident is the evidence for requiring both dry-runs and must survive the
  `CLAUDE.md` restructure somewhere durable.
- FR-7.5 No document may reference a path that does not exist in MyFleet.

### 4.8 `/fix-pr-bug`

- FR-8.1 Port `.claude/commands/fix-pr-bug.md`, rebinding any atlas paths, gate
  names, or verify flags to MyFleet's.

### 4.9 `.claude/settings.json`

- FR-9.1 Add `disableBundledSkills: true`.
- FR-9.2 Wire the full hook set at the same events as atlas:
  - `PreToolUse`: `Write|Edit` → `block-home-paths-in-docs.sh`; `Agent` →
    `fork-dispatch-guard.sh`; `Bash` → `wait-loop-guard.sh`; `*` →
    `turn-budget-guard.sh`
  - `PostToolUse`: `Write|Edit` → `format-on-write.sh`; `*` → `turn-budget.sh`;
    `Bash` → `commit-boundary.sh`
  - `SessionStart` → `task-num-collision-detector.sh` (already wired)
  - `UserPromptSubmit` → `skill-activation-prompt.sh` (already wired)
- FR-9.3 Preserve the existing `enabledPlugins` block.
- FR-9.4 The file must remain valid JSON and every referenced hook path must
  exist and be executable.

### 4.10 `CLAUDE.md` restructure

- FR-10.1 Rewrite into exactly these eight headings, in order, after `# MyFleet`:
  `## Never do this`, `## Evidence & grounding`, `## Development workflow`,
  `## Done means verified`, `## Dispatching agents`, `## Handing off context`,
  `## Repository conventions`, `## Where the procedures live`.
- FR-10.2 `## Where the procedures live` is a trigger → owner table, and **every
  target file in it must exist**.
- FR-10.3 These rules currently carried as prose must survive, not be lost:
  - the `make ci` target list;
  - the Node/nvm bootstrap;
  - the manifest render and dual `--dry-run=server` requirement, including the
    local-overlay incident (FR-7.4);
  - the worktree discipline rules (never edit the main repo when a task worktree
    exists; search all worktrees before concluding a file is missing);
  - the artifact-location override (`docs/tasks/task-NNN-slug/`, not
    `docs/superpowers/`);
  - code-review-before-PR;
  - the design/plan output style (write the full document, don't interview
    section by section);
  - verification-over-memory;
  - the four-phase flow and fuzzy task-identifier resolution;
  - container build context is the repo root for every service, `apps/web`
    included.
- FR-10.4 Repo-specific content stays repo-specific: project overview, build
  commands, deployment specifics, domain conventions.

### 4.11 The MyFleet carve-out

- FR-11.1 In MyFleet, **only `docs/process-parity.md`** is exempt from the "no
  `atlas-*` references" rule. The `docs/agent-dispatch.md` exemption is
  atlas-only: MyFleet never used the `atlas-*` names, so it needs no
  historical-cutoff note. The ported `docs/agent-dispatch.md` must therefore
  contain no `atlas-*` agent references at all.

## 5. API Surface

Not applicable. This task adds no HTTP endpoints and modifies no service
contract. The analogous surface is the command/agent surface:

| Surface | Kind | Change |
|---|---|---|
| `/execute-task` | command | rewritten to orchestrate the trio |
| `/fix-pr-bug` | command | new |
| `/service-doc` | command | new |
| `task-implementer` | agent | new |
| `task-verifier` | agent | new |
| `task-reviewer` | agent | new |
| `service-documentation` | agent | new |
| `tools/verify.sh` | CLI | new; flags `--quick`, `--no-docker`, `-h`/`--help` |
| `tools/task-brief.sh` | CLI | new; `PLAN_FILE TASK_NUMBER [OUTFILE]`, exits 0/2/3 |

## 6. Data Model

Not applicable — no entities, fields, relationships, or migrations. The only
persistent state introduced is the git-ignored `.superpowers/sdd/` brief
workspace (FR-6.2) and the tool-call counters `turn-budget.sh` maintains.

## 7. Service Impact

No service under `apps/` or `packages/` changes behavior. Impact is confined to:

| Area | Change |
|---|---|
| `.claude/hooks/` | +7 verbatim, +1 rebound (`format-on-write.sh`) |
| `.claude/agents/` | +4 (trio + `service-documentation`) |
| `.claude/commands/` | +2 (`/fix-pr-bug`, `/service-doc`), 1 rewritten (`/execute-task`) |
| `.claude/settings.json` | `disableBundledSkills` + full hook wiring |
| `tools/` | +`verify.sh`, +`task-brief.sh` |
| `docs/` | +9 owner documents, +`DOCS.md` at root (assumption, §4.4) |
| `CLAUDE.md` | restructured |

Indirect impact worth calling out: once the hooks are wired, **every future
session in this repository is subject to them** — `wait-loop-guard.sh` blocks
polling and `sleep` loops, `block-home-paths-in-docs.sh` rejects absolute home
paths under `docs/`, and `turn-budget-guard.sh` caps implementer tool calls.
That is the point of the task, but it means a hook that misfires degrades all
work, not just this branch. Hooks must fail open where they are advisory.

## 8. Non-Functional Requirements

- NFR-1 **Hook latency.** `PostToolUse` `*` hooks run on every tool call.
  `turn-budget.sh` must stay cheap; it is already in production use in atlas at
  this size, so verbatim copying preserves its measured cost.
- NFR-2 **Fail-open vs. fail-closed.** `turn-budget-guard.sh`,
  `wait-loop-guard.sh`, and `block-home-paths-in-docs.sh` are deliberately
  blocking. `format-on-write.sh` and `commit-boundary.sh` are advisory and must
  never block a write or a commit.
- NFR-3 **No secrets.** `verify.sh` must not print kubeconfig contents, cluster
  credentials, or registry tokens. It names the *context* on a dry-run skip, not
  the credentials.
- NFR-4 **Verify runtime.** Flagless `verify.sh` will be slow — `make ci` plus
  five container builds plus two dry-runs. `--quick` exists precisely so the
  inner loop is not gated on that; the help text must make the tradeoff explicit.
- NFR-5 **Idempotence.** Re-running `verify.sh` changes no tracked file.
- NFR-6 **Determinism of the parity checks.** §7 checks 2 and 3 are scripted
  one-liners, not judgment calls.

## 9. Open Questions

1. **`DOCS.md` (§4.4).** Porting `service-documentation` requires a root
   documentation contract MyFleet lacks. The assumption taken is to port and
   genericize atlas's `DOCS.md` with per-service docs at `apps/<svc>/docs/`.
   Confirm in design; the alternative is to rebind the agent to a lighter
   contract, but *not* to ship an agent pointing at a missing document.
2. **Trio review artifact location.** `task-reviewer` writes a durable artifact.
   MyFleet's existing reviewers write to `docs/tasks/task-NNN-slug/audit.md`.
   Decide in design whether `task-reviewer` shares that file, uses a sibling
   (e.g. `reviews/`), or is per-commit-range — atlas's convention should be
   checked before inventing one.
3. **`turn-budget.sh` counter storage.** Where it persists counters must not
   collide across concurrent worktrees. Verify atlas's mechanism is
   worktree-safe before relying on it here.
4. **`disableBundledSkills: true` interaction.** MyFleet enables the
   `superpowers` plugin. Confirm the setting disables only Claude Code's bundled
   skills and not plugin-provided ones, since the four phase commands depend on
   `superpowers:*` skills.
5. **Overlap between ported `git-workflow.md` / `tooling-conventions.md` and the
   user's global `~/.claude/CLAUDE.md`.** Where they conflict, the repo document
   should defer rather than contradict.

## 10. Acceptance Criteria

Mechanical, in-repo:

- [ ] AC-1 `diff` against `$ATLAS` is empty for each of the seven §3.1 portable
      hooks.
- [ ] AC-2 `grep -l 'atlas-' .claude/hooks/*.sh` prints nothing.
- [ ] AC-3 This prints nothing:
      ```sh
      git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
        | grep -vxE 'docs/process-parity\.md'
      ```
- [ ] AC-4 `tools/verify.sh`, `tools/task-numbers.sh`, and `tools/task-brief.sh`
      all exist and are executable.
- [ ] AC-5 Flagless `tools/verify.sh` exits 0.
- [ ] AC-6 `tools/verify.sh --quick` and `--no-docker` exit 0 and their output
      states that they do not authorize "done".
- [ ] AC-7 `tools/verify.sh --help` exits 0; an unknown flag exits 2.
- [ ] AC-8 `.claude/hooks/wait-loop-guard_test.sh` passes.
- [ ] AC-9 `.claude/agents/` contains `task-implementer`, `task-verifier`,
      `task-reviewer`, and `service-documentation`.
- [ ] AC-10 `.claude/commands/` contains `fix-pr-bug.md` and `service-doc.md`,
      and `execute-task.md` dispatches the trio by name.
- [ ] AC-11 `.claude/settings.json` is valid JSON, sets
      `disableBundledSkills: true`, wires all nine hook entries, and every
      referenced hook path exists and is executable.
- [ ] AC-12 `CLAUDE.md` carries the eight §5.3 headings in order and ends with a
      `## Where the procedures live` table.
- [ ] AC-13 Every `docs/` link in `CLAUDE.md` resolves to an existing file.
- [ ] AC-14 All nine §3.5 owner documents exist under `docs/`.
- [ ] AC-15 Every FR-10.3 rule is present somewhere in `CLAUDE.md` or in a
      document its table points to — checked item by item, not in aggregate.
- [ ] AC-16 The local-overlay `namespace:` incident text is present in a durable
      document.
- [ ] AC-17 No ported document references a path absent from MyFleet.
- [ ] AC-18 `make ci` still passes (the restructure must not break the build).

Reported, not asserted:

- [ ] AC-19 §7 checks 1, 4, 5, and 6 are cross-repo pairwise comparisons and are
      **not evaluable from MyFleet alone**. Report MyFleet's side of each and say
      plainly that the comparison was not performed here. Do not claim these
      pass.
