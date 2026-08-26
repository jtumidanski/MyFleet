# Task 14 — Parity checks, full gate, and the AC-19 report

Ran 2026-08-26 in `.worktrees/task-032-process-parity-phase-2`, kubectl context `bee`.

## 1. In-repo checks (checks 2 and 3 of `docs/process-parity.md` §7)

### Check 2 wording (AC-2) — literal command from the task-14 brief

```sh
grep -l 'atlas-' .claude/hooks/*.sh
```

Output:

```
.claude/hooks/wait-loop-guard_test.sh
```

This is **not** a clean "no output" pass as the brief's Step 1 predicted. The single hit is a
benign fixture string, not an agent-name reference:

```
.claude/hooks/wait-loop-guard_test.sh:62:allow 'kubectl get pods -n atlas-pr-1370'
```

`atlas-pr-1370` is an example Kubernetes namespace in a test fixture for the byte-copied
`wait-loop-guard_test.sh` (see Deviation 10 — this file cannot be edited without breaking AC-1
byte-identity). It is not one of `atlas-implementer`, `atlas-verifier`, or `atlas-reviewer`, and
it carries no operative meaning for MyFleet. The *actual* rule this repo enforces — no reference
to the old agent names — is check 3 below, and it is narrower than the brief's literal AC-2
command for exactly this reason.

### Check 3 (AC-3) — actual acceptance rule, narrowed per ruling R2

```sh
git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
  | grep -vxE 'docs/process-parity\.md'
```

Output: (none)

Passed. No operative reference to the old agent names survives outside the historical/explainer
carve-out.

## 2. Acceptance criteria (AC-1 … AC-18)

| AC | Command | Result |
|---|---|---|
| AC-1 | `diff -q` on each of the 7 portable hook files vs `$ATLAS/.claude/hooks/` | All 7 diffs empty (`wait-loop-guard.sh`, `wait-loop-guard_test.sh`, `block-home-paths-in-docs.sh`, `turn-budget.sh`, `turn-budget-guard.sh`, `fork-dispatch-guard.sh`, `commit-boundary.sh`). `rc=0`, byte-identical. |
| AC-2 | see §1 above | Literal grep is **not** empty (1 benign fixture hit, `atlas-pr-1370`); explained, not a real failure. |
| AC-3 | see §1 above | Empty. Pass. |
| AC-4 | `test -x tools/verify.sh` / `tools/task-numbers.sh` / `tools/task-brief.sh` | `AC-4 ok tools/verify.sh`, `AC-4 ok tools/task-numbers.sh`, `AC-4 ok tools/task-brief.sh` |
| AC-5 | `tools/verify.sh` flagless | `exit=0`; closing line `All gates passed — the branch may be called done.` (see §3) |
| AC-6 | `tools/verify_test.sh` | Exit 0; last line `verify_test.sh: all assertions passed` |
| AC-7 | `tools/verify.sh --help` / `--bogus` | `AC-7 help=0`; `AC-7 bogus=2` |
| AC-8 | `.claude/hooks/wait-loop-guard_test.sh` | Exit 0; `passed: 33  failed: 0` |
| AC-9 | `ls .claude/agents/{task-implementer,task-verifier,task-reviewer,service-documentation}.md` | All four exist |
| AC-10 | `ls .claude/commands/{fix-pr-bug,service-doc}.md`; `grep -c -e task-implementer -e task-verifier -e task-reviewer .claude/commands/execute-task.md` | Both files exist; grep count `13` |
| AC-11 | `jq empty`, `.disableBundledSkills`, hook count, executability | Valid JSON; `disableBundledSkills` = `true`; hook command count = `9`; all 9 hook commands executable |
| AC-12 | `grep -n '^## ' CLAUDE.md` | 8 headings in order: `Never do this`, `Evidence & grounding`, `Development workflow`, `Done means verified`, `Dispatching agents`, `Handing off context`, `Repository conventions`, `Where the procedures live` |
| AC-13 | Every `](...)` link in `CLAUDE.md` resolved via `test -e` | All 12 targets exist (9 `docs/*.md`, `DOCS.md`, `docs/runbooks/`, `docs/process-parity.md`); no `DANGLING` |
| AC-14 | `test -f docs/<name>.md` for all 9 owner docs | All 9 present: `agent-dispatch`, `verification`, `superpowers-integration`, `review-protocol`, `post-implementation`, `codemod-vs-agents`, `slice-first`, `tooling-conventions`, `git-workflow` |
| AC-16 | `grep -rl 'subjects\[0\]\.namespace: Required value' docs/ CLAUDE.md` | Present in `docs/verification.md` (durable owner doc) and 4 task-history files (`docs/tasks/task-032-process-parity-phase-2/prd.md`, `docs/tasks/task-032-process-parity-phase-2/plan.md`, `docs/tasks/task-029-go-127-migration/plan.md`, `docs/tasks/task-002-k3s-cluster-deployment/audit-plan-adherence.md`) |
| AC-17 | `grep -ohE '(tools\|scripts)/[A-Za-z0-9._/-]+' ... \| sort -u`, existence-checked individually | All MyFleet-owned tool paths exist (`tools/agent-ledger.sh`, `tools/check-manifests.sh`, `tools/doc-slice.sh`, `tools/lint.sh`, `tools/lint.versions`, `tools/task-brief.sh`, `tools/task-numbers.sh`, `tools/verify.sh`). Four names did not resolve as MyFleet paths: `scripts/ci-build.sh`, `scripts/ci-test.sh`, `scripts/lint-all.sh`, `scripts/local-up.sh`, `tools/toolchain.versions` — all five are traced to `docs/process-parity.md` / `docs/process-parity-brief.md`, describing **atlas's or home-hub's own** tooling in a cross-repo comparison table, not a MyFleet path claim. No true MISSING. |
| AC-18 | satisfied by AC-5's run, since gate 1 of `verify.sh` is `make ci` | `make ci` ran as gate 1 and PASSED (see §3) |
| scope | `git diff --name-only main...HEAD \| grep -E '^(apps\|packages)/'` | No output (grep exit 1) — `scope ok: no apps/ or packages/ change` |

## 3. Flagless gate (`tools/verify.sh`, AC-5/AC-18)

Cluster was reachable (`kubectl config current-context` = `bee`). All four gates ran for real —
**no `⚠ SKIPPED` lines were recorded**. This is a clean pass, not a pass-with-gap.

```
verify.sh: node v24 detected; this repo targets 22 (nvm use 22)

── make ci (lint-check vet test build fe-test fe-build manifests carfax-template)
... [full make ci output: lint.sh 0 issues x6, prettier check clean, eslint clean,
     go vet clean, go test -race all packages ok, go build ok, fe-test/fe-build ok] ...

── container builds (5 images, context = repo root)
... [5 images built] ...

── cluster dry-run, main overlay
... [server dry-run apply, all resources "configured"/"created (server dry run)"] ...

── cluster dry-run, local overlay
... [server dry-run apply, all resources "configured"/"created (server dry run)",
     including namespace/myfleet, ClusterRole/ClusterRoleBinding, PVCs, Secrets] ...

════ verify.sh summary ════
  ✓ make ci (lint-check vet test build fe-test fe-build manifests carfax-template) PASSED
  ✓ container builds (5 images, context = repo root) PASSED
  ✓ cluster dry-run, main overlay PASSED
  ✓ cluster dry-run, local overlay PASSED

All gates passed — the branch may be called done.
```

`AC-5 flagless exit=0`.

The `node v24 detected` warning is Deviation 4 (design D4) exercising as expected — Node in this
environment is v24, not v22.

## 4. AC-19 — not evaluable here

> `docs/process-parity.md` §7 checks 1, 4, 5 and 6 are pairwise comparisons between repositories.
> They are **not evaluable from MyFleet alone**, and this report does not claim they pass. What
> follows is MyFleet's side of each, for whoever performs the comparison.

**Check 1 — the seven §3.1 hook files are byte-identical across all four repositories.**
MyFleet's side: `diff -q` against `$ATLAS` (`atlas-ms/atlas` worktree
`task-266-process-parity-agent-rename`) for all 7 files (`wait-loop-guard.sh`,
`wait-loop-guard_test.sh`, `block-home-paths-in-docs.sh`, `turn-budget.sh`,
`turn-budget-guard.sh`, `fork-dispatch-guard.sh`, `commit-boundary.sh`) returned empty — MyFleet
is byte-identical to atlas for all 7. `home-hub` and `Harbormaster` were not compared here (out
of scope for this worktree).

**Check 4 — each `.claude/settings.json` wires the same hook set at the same events.**
MyFleet's side: 9 hook commands wired across 4 events (`PostToolUse`, `PreToolUse`,
`SessionStart`, `UserPromptSubmit`) — `block-home-paths-in-docs.sh`, `fork-dispatch-guard.sh`,
`wait-loop-guard.sh`, `turn-budget-guard.sh`, `format-on-write.sh`, `turn-budget.sh`,
`commit-boundary.sh`, `task-num-collision-detector.sh`, `skill-activation-prompt.sh`; all 9
executable. Whether the other repositories wire the identical set at the identical events was not
checked here.

**Check 5 — each `CLAUDE.md` carries the same eight section headings, and ends with a
`## Where the procedures live` table whose every target file exists.**
MyFleet's side: 8 headings present in order (§2, AC-12), and the closing
`## Where the procedures live` table lists 15 triggers whose targets (owner docs, `DOCS.md`,
`docs/runbooks/`, `backend-dev-guidelines`/`backend-guidelines-reviewer`,
`frontend-dev-guidelines`/`frontend-guidelines-reviewer`, `docs/process-parity.md`) all resolve
(the skill/agent names are not file-path links and were not path-checked). Whether the other
repositories' `CLAUDE.md` carry the identical headings was not checked here.

**Check 6 — each repository has the nine §3.5 owner documents, and every `docs/` link in every
`CLAUDE.md` resolves.**
MyFleet's side: all 9 owner documents present (§2, AC-14) and all `docs/` links in `CLAUDE.md`
resolve (§2, AC-13). Whether the other repositories carry the same nine was not checked here.

## 5. Deviations from the PRD, with reasons

1. `tools/doc-slice.sh` and `tools/agent-ledger.sh` ported beyond the PRD's tools list (design
   D1) — the minimum that makes FR-7.2 and AC-17 simultaneously satisfiable.
2. `tools/task-resolve.sh` also ported (ruling R1, beyond D1) — `tools/agent-ledger.sh:74`
   hard-calls it and `agent-ledger_test.sh` copies both into its fixture, so without it Task 1's
   test fails and `/execute-task` Step 4f ships broken. It is repo-agnostic and no document cites
   it (consistent with the AC-17 sweep finding no reference to it at all).
3. `format-on-write.sh`'s prettier match is a superset of FR-2.1, covering `packages/*/src` as
   well as `apps/web` (design D3).
4. `verify.sh` warns on a non-22 Node major, which FR-5.10 does not require (design D4). This
   environment has Node v24, and the warning path was exercised in §3's run.
5. `tools/verify_test.sh` and the `VERIFY_DRY_RUN` short-circuit, which no FR names (design D2).
6. `step()` was refactored to a single `PASSED+=`/`FAILED+=` site (ruling R4) — the plan's
   transcribed `step()` had two `PASSED+=` lines but the plan's own test asserted exactly one;
   only the single-site form satisfies all three structural assertions
   (`PASSED is appended in exactly one place`, `FAILED is appended in exactly one place`,
   `that place is inside step()` — all three passed in §2's AC-6 run). Behaviourally identical.
7. `verify_test.sh`'s gate-selection assertions were tightened beyond the plan's text (ruling
   R5) — the plan's needles matched both the PASSED and SKIPPED summary lines, so the
   "--quick skips X" assertions could not fail, and nothing asserted that `--quick` skips the two
   dry-runs. The tightened assertions (`--quick skips the main dry-run (SKIPPED)`, etc.) ran and
   passed in §2's AC-6 output.
8. `DOCS.md` does NOT prescribe documenting compound documents / an `included` member (ruling
   R7), contrary to design Q1's phrasing — `packages/shared-go/server/jsonapi.go`'s `Document`
   has no `Included` field and `"included"` appears in no Go file, so prescribing it would
   violate `DOCS.md`'s own "document only what exists" rule.
9. `task-implementer`'s module-local gate is `go build ./... && go test ./...`, without
   `go vet`, `-race`, or `tools/lint.sh` (ruling R8), contrary to the plan's Task 8 block —
   atlas's implementer contract explicitly forbids those three and so does
   `docs/agent-dispatch.md:75`, which the plan's block contradicted.
10. Task 1's `atlas-` grep was narrowed to `atlas-(implementer|verifier|reviewer)` (ruling R2) —
    the byte-copied `wait-loop-guard_test.sh` and `doc-slice_test.sh` carry benign
    `atlas-<something>` fixture strings (e.g. `atlas-pr-1370`, see §1), and AC-1 byte-identity
    forbids editing them, so the broad grep and AC-1 were mutually unsatisfiable. This is the
    exact deviation that explains §1's AC-2 result above.
11. The container-build-context rule sits under `## Done means verified` rather than the plan's
    assigned `## Repository conventions` (ruling R9) — the rule survives with accurate wording.
12. A lost rule was restored to `docs/tooling-conventions.md` (ruling R6): "any such wrapper must
    pass `tools/*.sh` output through unfiltered", dropped because its example cited the unported
    `task-resolve.sh`.
13. Per-document shrink: measured live with `wc -l` on every ported owner document plus `DOCS.md`
    against its `$ATLAS` source (percentages are MyFleet lines / atlas lines):
    `docs/agent-dispatch.md` 249/244 = 102%, `docs/verification.md` 188/368 = **51%**,
    `docs/superpowers-integration.md` 177/185 = 96%, `docs/review-protocol.md` 224/178 = 126%,
    `docs/post-implementation.md` 186/160 = 116%, `docs/codemod-vs-agents.md` 117/138 = 85%,
    `docs/slice-first.md` 109/107 = 102%, `docs/tooling-conventions.md` 107/95 = 113%,
    `docs/git-workflow.md` 69/52 = 133%, `DOCS.md` 361/261 = 138%. One document,
    `docs/verification.md`, falls below the plan's 60% floor (plan.md:871: "Any document below
    60% of its atlas line count needs a one-line written justification"), and no such
    justification was recorded at commit time (`67d1802`) — the plan gave Task 4 no shrink step
    (only Tasks 5 and 6 had one), so the requirement was never triggered for it. The
    justification, recorded here: the plan directs `docs/verification.md` to be rebuilt around
    MyFleet's four gates rather than translated section by section, because atlas's 368 lines are
    organised around change detection and per-guard escape hatches that MyFleet's gate set does
    not have. The shrink is dropped machinery, not dropped rules — and that was verified, not
    inferred from the document's length: Task 4's review (`.superpowers/sdd/plan/task-4-report.md`,
    "Ten required content items — mapping") confirmed all ten required content items present with
    file:line citations, and separately found and fixed a Critical accuracy defect in the
    document. No owner document was changed to produce this finding.

Additionally: five text-port tasks each lost at least one rule during transcription, and every
one was caught by review and restored (deviations 8, 9, 12 above are examples of this) — that is
the process working, and it belongs in the record.

## 6. What was skipped and why

Nothing was skipped. The flagless `tools/verify.sh` run in §3 found the `bee` cluster reachable
and ran both server dry-runs for real; no `⚠ SKIPPED` line was recorded.
