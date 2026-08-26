# Design — Process Parity Phase 2 (MyFleet)

Status: Approved for planning
Created: 2026-08-26
Inputs: `docs/tasks/task-032-process-parity-phase-2/prd.md`,
`docs/process-parity.md`, `docs/process-parity-brief.md`
Source repository (`$ATLAS`):
`~/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename`

---

## 0. Verified preconditions

Checked before designing, not assumed:

| Check | Result |
|---|---|
| `diff $ATLAS/docs/process-parity.md docs/process-parity.md` | empty — the pinned copy is current at `e83f59e61` |
| `$ATLAS` reachable | yes, at the path above |
| `make ci` target list | `lint-check vet test build fe-test fe-build manifests carfax-template` (`Makefile:51`) |
| `tools/check-manifests.sh` renders both overlays | yes — `for overlay in main local` (`tools/check-manifests.sh:24`); asserts the `main` invariants; does **not** run `kubectl` |
| MyFleet lint cache layout | `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION` (`tools/lint.sh:56-57`) — identical shape to atlas's |
| `tools/lint.versions` | defines `GOLANGCI_LINT_VERSION=v2.13.1` |
| `.golangci.yml` at repo root | present |
| `jq`, `kustomize`, `kubectl`, `docker`, `npm` on PATH | all present; `npm` currently resolves to Node **v24**, not v22 (see D4) |
| `.gitignore` | already ignores `.cache/`; does **not** mention `.superpowers/` — and does not need to (see D7) |

---

## 1. What this task actually is

Three different kinds of work are hiding under one PRD, and they have different
risk profiles. The plan must not treat them uniformly.

| Kind | Files | Risk | Verification |
|---|---|---|---|
| **Byte-copy** | 7 portable hooks, `tools/task-brief.sh` | near-zero | `diff` is empty; `wait-loop-guard_test.sh` passes |
| **Rebind** | `format-on-write.sh`, agent trio, `service-documentation`, `/service-doc`, `/fix-pr-bug`, `/execute-task`, 9 owner docs, `DOCS.md`, `CLAUDE.md` | **highest** — a silently dropped rule is invisible to every mechanical check | rule-by-rule mapping table (§8), AC-15 |
| **Engineering** | `tools/verify.sh` (+ its test) | moderate | `tools/verify_test.sh`, AC-5/6/7 |

The failure mode to design against is not the copy or the script. It is the
rebind: 1,519 lines of atlas owner docs shrink under genericization, and
"shorter" and "missing a rule" look identical in a diff. §8 exists solely to
make that difference mechanical.

---

## 2. Architecture at a glance

```
CLAUDE.md  (8 headings, ~90 lines, rule list)
   └── ## Where the procedures live  →  trigger → owner table
                                          │
        ┌─────────────────────────────────┴──────────────────────────────┐
        │                                                                 │
   docs/*.md (9 owner docs)                                    .claude/ (enforcement)
   agent-dispatch, verification,                        hooks: 7 verbatim + format-on-write
   superpowers-integration, review-protocol,            agents: task-{implementer,verifier,
   post-implementation, codemod-vs-agents,                      reviewer}, service-documentation
   slice-first, tooling-conventions,                    commands: /execute-task (rewritten),
   git-workflow                                                   /fix-pr-bug, /service-doc
        │                                                                 │
        └──────────────────► tools/ ◄────────────────────────────────────┘
              verify.sh (new) · task-brief.sh (port) · doc-slice.sh (port)
              agent-ledger.sh (port) · task-numbers.sh (exists) · lint.sh (exists)
```

The load-bearing relationship: `CLAUDE.md` is allowed to be short **only
because** the owner table resolves every omitted mechanic to a file. That makes
FR-10.2 (every table target exists) a structural requirement, not a lint rule —
a dangling row silently deletes a procedure.

---

## 3. Decision D1 — the ancillary-tool gap (not named in the PRD)

**The problem the PRD does not name.** Atlas's owner docs, agents, and
`/execute-task` reference six atlas-only scripts beyond the two the PRD scopes
(`verify.sh`, `task-brief.sh`). Grepped from the exact file set to be ported:

| Referenced script | Referenced by | Nature |
|---|---|---|
| `tools/doc-slice.sh` | `slice-first.md` (×4), `tooling-conventions.md`, `task-implementer.md` (×3), `task-reviewer.md`, `execute-task.md` | generic, 192 lines, no atlas coupling |
| `tools/agent-ledger.sh` | `execute-task.md` Step 4f, `CLAUDE.md` | generic, 191 lines, TSV append |
| `tools/task-resolve.sh` | `superpowers-integration.md` (×2), `execute-task.md` | atlas task-layout specific |
| `tools/task-facts.sh` | `tooling-conventions.md` (×2), `execute-task.md`, `verification.md` | atlas-specific (probes toolchain, guard suites, service/lib surfaces) |
| `tools/change-surfaces.sh` | `tooling-conventions.md`, `superpowers-integration.md` | atlas-specific (`services/` + `libs/` + audit families) |
| `tools/toolchain.versions` | `format-on-write.sh` | already rebound by FR-2.2 |

AC-17 forbids a ported document referencing a path absent from MyFleet. FR-7.2
forbids deleting the rule the reference illustrates. So each one needs a
disposition; there is no third option.

**Decision:**

- **Port verbatim: `tools/doc-slice.sh` and `tools/agent-ledger.sh`.** Both are
  self-contained and repository-agnostic. `slice-first.md` is *about*
  `doc-slice.sh` — porting the doc without the tool would leave a 107-line
  document whose entire mechanism is a dangling path. `agent-ledger.sh` is the
  mechanism for `/execute-task` Step 4f and the `CLAUDE.md` batching rule.
  Together they are 383 lines of copy — cheaper than inventing MyFleet
  substitutes and strictly better for §7 check 1's spirit (portable files stay
  byte-identical).

  *Scope note:* this is two files beyond the PRD's §4.6 tools list. It is not
  scope creep for its own sake — it is the minimum that makes FR-7.2 and AC-17
  simultaneously satisfiable for `slice-first.md` and `/execute-task`. If the
  user prefers strict PRD scope, the fallback is D1-alt below.

- **Do not port: `task-resolve.sh`, `task-facts.sh`, `change-surfaces.sh`.**
  These encode atlas's `services/` + `libs/` layout, its guard-suite taxonomy,
  and its toolchain probe. Rebind their *references* instead:

  | Atlas reference | MyFleet rebinding |
  |---|---|
  | `tools/task-resolve.sh <id>` / `--list` | the fuzzy task-identifier algorithm already specified in MyFleet's phase commands, plus `tools/task-numbers.sh next` / `check` and `git worktree list` |
  | `tools/task-facts.sh <task>` | `git worktree list`, `git branch --show-current`, `ls docs/tasks/<id>/`, `tools/task-numbers.sh check` |
  | `tools/change-surfaces.sh --base <sha>` | `git diff --name-only <base>...HEAD \| sed -n 's\|^\(apps\|packages\)/\([^/]*\)/.*\|\1/\2\|p' \| sort -u` |

  The *rules* they illustrate — "ask the tooling for a mechanical fact rather
  than deriving it", "resolve a bare task number before acting on it", "know
  which surfaces a diff touches before choosing a gate" — all survive with
  MyFleet-native examples. That is exactly the FR-7.2 contract.

- **D1-alt (rejected, recorded):** drop `slice-first.md`'s tool references and
  `/execute-task` Step 4f. Rejected because it deletes the slice mechanism and
  the cost-recording rule, which FR-7.2 forbids, and because the owner table
  would then point at a document that only exhorts.

---

## 4. Decision D2 — `tools/verify.sh` architecture

**Shape: a thin gate runner, not a change-detection engine.** Atlas's 746-line
`verify.sh` is mostly change detection over 86 Go modules — it decides *which*
modules to build. MyFleet has one monolithic entry point (`make ci` walks
`github.com/jtumidanski/myfleet/...` in one command), so change detection would
buy nothing and cost the script's whole complexity budget. MyFleet's version is
~200 lines.

**Ported from atlas verbatim in shape (not text):** the `step` / `skip`
helpers, the `PASSED`/`FAILED`/`SKIPPED` arrays, run-everything-then-summarize
(FR-5.4), and the two-tier exit message (all-passed vs. all-passed-but-skipped).
That shape is what makes FR-5.1/5.2/5.4/5.5 fall out rather than be bolted on.

**Flag surface — exactly three, no more:**

| Flag | Effect |
|---|---|
| *(none)* | every gate; exit 0 ⟺ branch may be called done (FR-5.1) |
| `--quick` | skips container builds **and** cluster dry-runs; exit 0 does not authorize done (FR-5.2) |
| `--no-docker` | skips container builds only; exit 0 does not authorize done |
| `-h`/`--help` | usage, exit 0. Unknown option → usage to stderr, **exit 2** (FR-5.3) |

Atlas's `--base`, `--all`, `--no-ui`, `--facts` are deliberately **not** ported.
`--facts` in particular is worth naming as a rejected option: it is valuable in
atlas because selection is non-obvious, and it is noise in MyFleet where the
gate list is fixed and printable from `--help`.

**Gate table:**

| # | Gate | Command | Skipped by |
|---|---|---|---|
| 1 | Repo gate | `make ci` (which is `lint-check vet test build fe-test fe-build manifests carfax-template`) | never |
| 2 | Container builds | `docker build -f apps/<svc>/Dockerfile .` for `auth-service`, `fleet-service`, `media-service`, `notification-service`, `web` — context repo root | `--quick`, `--no-docker` |
| 3 | Cluster dry-run, `main` | `kustomize build deploy/k8s/overlays/main \| kubectl apply --dry-run=server -f -` | `--quick`; unreachable cluster |
| 4 | Cluster dry-run, `local` | `kustomize build deploy/k8s/overlays/local \| kubectl apply --dry-run=server -f -` | `--quick`; unreachable cluster |

Gates 3 and 4 are **two separate `step` entries**, never one loop that
short-circuits. The local-overlay incident (FR-7.4) is precisely a case where
`main` passed and `local` did not; a single combined gate reintroduces the class
of bug the gate exists to catch.

**No re-implementation of the manifest invariants.** Gate 1 already runs
`tools/check-manifests.sh`, which renders both overlays and asserts no PVCs, no
Secrets, no ClusterRole, no `REPLACE_ME` (verified at
`tools/check-manifests.sh:24-62`). `verify.sh` calls `make ci` and stops there
(FR-5.6). Two sources of truth for the same assertion is how they drift.

**Reachability probe (FR-5.9).** Before gates 3–4:
`kubectl cluster-info --request-timeout=5s >/dev/null 2>&1`. On failure, both
gates are recorded SKIPPED with the reason
`no reachable cluster (context: <kubectl config current-context>)`, and the run
still exits 0. The context **name** is printed; no kubeconfig content, no
credentials (NFR-3). The skip is emphasized in the summary — a `⚠` line above
the pass line, not a dim `−` line among the others — because an invisible skip
here is the exact failure mode the gate exists to prevent.

**Idempotence (NFR-5).** No gate writes a tracked file. `make ci`'s lint runs in
`--check` mode; `check-manifests.sh` writes only to `/tmp`; docker writes to the
image store. Nothing to clean up.

**Test.** Add `tools/verify_test.sh` (modelled on atlas's `verify_test.sh`,
adapted): asserts `--help` → 0, unknown flag → 2, and that `--quick` /
`--no-docker` output contains the non-authorization sentence. It must not
execute the real gates — it exercises argument parsing and the summary text via
a `VERIFY_DRY_RUN=1` short-circuit that the script honors before the first gate.
This turns AC-6 and AC-7 from a human read into a command.

---

## 5. Decision D3 — `format-on-write.sh` rebinding

Three changes to the atlas file; everything else byte-identical, including the
fail-open structure and the never-bootstrap rule (FR-2.3, NFR-2).

1. `source "$ROOT/tools/toolchain.versions"` → `source "$ROOT/tools/lint.versions"`.
   The variable name `GOLANGCI_LINT_VERSION` and the cached path
   `$ROOT/.cache/tools/bin/golangci-lint-$VERSION` are **already identical**
   between the repos, so only the filename changes.
2. Prettier branch: atlas matches `*/services/atlas-ui/*.ts{,x}` and runs
   `npx prettier` from `services/atlas-ui`. MyFleet's Prettier is configured at
   the **repo root** (`tools/lint.sh:184` runs `npm run format:check` with cwd
   `$ROOT`, covering every workspace). So the rebinding is:
   match `*/apps/web/*.ts{,x}` **and** `*/packages/*/src/*.ts{,x}`, and run
   `npx --no-install prettier --write "$fp"` from `$ROOT`.

   *Why the wider match:* FR-2.1 says "rebind the prettier branch to
   `apps/web`". Binding literally to `apps/web` alone would leave
   `packages/shared-ts` and `packages/ui-components` — both real TS workspaces
   with tests in `make fe-test` — unformatted on write, and `make lint-check`
   would then fail on files the hook had just touched next door. The root
   Prettier config makes the wider match free. This is a superset of FR-2.1, not
   a departure from it; called out here so the plan states it explicitly.
3. Everything else — the `jq` extraction, the absolute-path guard, the go.mod
   walk, `exit 0` on every unhandled path — is unchanged.

`jq` is on PATH (§0), and the hook already fails open when it is not.

---

## 6. Decision D4 — Node bootstrap and the version wrinkle

FR-5.10: bootstrap `nvm use 22` when `npm` is absent; do not touch the
environment when it is present.

**The wrinkle:** on this machine `npm` *is* present but resolves to Node
**v24.19.0**, while the repo targets 22. Implemented literally, `verify.sh` would
leave a wrong-major Node in place and any resulting `fe-test`/`fe-build` failure
would look like a code defect.

**Decision:** honor FR-5.10 exactly — bootstrap only on absence — and add a
**non-fatal warning** when `npm` is present and `node --version` is not major 22:

```
verify.sh: node vNN detected; this repo targets 22 (nvm use 22)
```

The warning changes no exit status and touches no environment. This is the
smallest change that keeps FR-5.10's contract while preventing a misattributed
failure. It is a deliberate addition to the FR and is recorded here so the plan
does not silently invent it.

---

## 7. Open questions from PRD §9 — resolved

### Q1 — `DOCS.md` → **port it, genericized** (PRD assumption confirmed)

`$ATLAS/DOCS.md` is 261 lines requiring per-service `README.md`,
`docs/domain.md`, `docs/kafka.md`, `docs/rest.md`, `docs/storage.md`, with
optional `saga.md`/`state.md`/`migrations.md`. The prerequisite question was
whether that artifact set even applies here: it does — `packages/shared-go`,
`apps/fleet-service`, `apps/notification-service`, and `apps/media-service` all
carry Kafka in their `go.mod`, and every service is REST + Postgres.

Genericization:
- `Atlas Microservice Documentation Contract` → MyFleet's.
- REST section rebound to the hand-rolled JSON:API transport in
  `packages/shared-go/server` (not a generic REST description) — a MyFleet
  service's `rest.md` documents resource types, relationships, and included
  compound documents, because that is what the transport actually is.
- Per-service docs live at `apps/<svc>/docs/`.
- Atlas-only sections with no MyFleet analogue are dropped **as sections**, not
  as rules; nothing survives that references an absent concept.

**Explicit scope guard:** this task creates the *contract* and the agent that
enforces it. It does **not** write documentation for the four services. No
`apps/*/docs/` content is produced here. That is a follow-up task, and the plan
must say so rather than leave an implementer to discover the ambiguity.

### Q2 — `task-reviewer` artifact location → **`docs/tasks/<task>/reviews/<unit>.md`**

Atlas's convention, read from the agent definition itself: *"If the artifact
path is not given, derive it as `docs/tasks/<task>/reviews/<unit>.md`."* Adopt
it unchanged. It does **not** collide with MyFleet's existing reviewers, which
write `docs/tasks/<task>/audit.md` — and that separation is correct, not
incidental: `audit.md` is the pre-PR guideline/plan-adherence verdict for the
whole task; `reviews/<unit>.md` is one commit range's review, of which there are
many per task. Sharing one file would make the per-unit reviews clobber each
other.

### Q3 — `turn-budget.sh` counter storage → **worktree-safe, verified**

Read from the source: counters live at
`${TMPDIR:-/tmp}/claude-turn-budget/<key>` where `<key>` is `agent-<agent_id>`
for subagents and `session-<session_id>` for the controller, both sanitized to
`[A-Za-z0-9._-]`. The key derives from harness-issued ids, not from cwd or repo
path, so two concurrent worktrees produce disjoint keys. Files older than a day
are pruned. Nothing is written inside the repository. Copy verbatim; no
adaptation needed.

### Q4 — `disableBundledSkills: true` → **safe, verified by precedent**

`$ATLAS/.claude/settings.json` sets `disableBundledSkills: true` **and**
`"enabledPlugins": {"superpowers@claude-plugins-official": true}`
simultaneously, and atlas's four phase commands invoke `superpowers:*` skills as
their normal operating mode. The setting governs Claude Code's *bundled* skills,
not plugin-provided ones. Adopt as specified (FR-9.1), keeping
`enabledPlugins` untouched (FR-9.3).

### Q5 — overlap with the user's global `~/.claude/CLAUDE.md` → **repo docs defer**

The global file carries RTK (the bash-output proxy) and a file-edit override.
Neither is a git or shell *convention* in the sense `git-workflow.md` and
`tooling-conventions.md` own, so the overlap is small. Rule for the port: the
repo documents state repository-scoped mechanics (branch naming, worktree-shared
stash discipline, `make`/`tools/` entry points) and **do not** restate or
contradict global preferences about which tool to reach for. Where a topic is
genuinely global — e.g. "use `Edit`/`Write` for file content" — the repo document
stays silent rather than echoing it, because an echo is a second copy that will
drift.

One MyFleet-specific rule `git-workflow.md` **must** carry, which atlas's does
not: the shared-stash hazard. MyFleet runs many concurrent worktrees off one
`.git`, the stash stack is shared, and bare `git stash` / `git stash pop` can
pop another session's work. Use a WIP commit; if stashing, `git stash push -u -m
"<tag>"`, capture the SHA, `git stash apply <sha>`, then drop by tag.

---

## 8. `CLAUDE.md` restructure — the rule-survival map

This is the highest-risk deliverable and AC-15 demands item-by-item proof. The
plan must carry this table and check each row after the rewrite.

| FR-10.3 rule (current location) | Lands in |
|---|---|
| `make ci` target list | `## Done means verified` (as the `verify.sh` gate description) + `docs/verification.md` |
| Node/nvm bootstrap | `docs/verification.md` (mechanics); `## Done means verified` references it |
| Manifest render + **dual** `--dry-run=server` | `## Done means verified` (one line) + `docs/verification.md` (full) |
| Local-overlay `namespace:` incident (FR-7.4) | `docs/verification.md` — verbatim incident text, as the stated evidence for the dual-dry-run rule |
| Never edit main repo when a task worktree exists | `## Never do this` |
| Search all worktrees before concluding a file is missing | `## Development workflow` |
| Artifact-location override (`docs/tasks/task-NNN-slug/`) | `## Development workflow` + `docs/superpowers-integration.md` |
| Code review before PR | `## Done means verified` |
| Design/plan output style (write the file, don't interview) | `## Development workflow` |
| Verification over memory | `## Evidence & grounding` |
| Four-phase flow + fuzzy task-identifier resolution | `## Development workflow` + `docs/superpowers-integration.md` |
| Container build context is the repo root for every service incl. `apps/web` | `## Repository conventions` |
| *(existing)* "asked to understand or plan? do not implement" | `## Development workflow` |
| *(existing)* prefer straightforward moves over re-exported type aliases; no cross-boundary internals | `## Repository conventions` |
| *(existing)* `/audit-plan` + three reviewer agents, `skill-rules.json` triggers | `## Dispatching agents` + `docs/review-protocol.md` |
| *(existing)* task numbers from `tools/task-numbers.sh next` | `## Development workflow` |
| *(existing)* deploy runbooks | `## Where the procedures live` → `docs/runbooks/` |

**Owner table (FR-10.2)** — every target must exist when the table lands. Atlas's
last four rows (`docs/packets/`, `docs/reverse-engineering.md`,
`docs/adding-a-new-service.md`, `docs/observability.md`) are **dropped**, per
PRD non-goals. MyFleet-specific rows added: `docs/runbooks/` for deploy, both
guideline skills, and `docs/process-parity.md`.

**Sequencing constraint:** `CLAUDE.md` is rewritten **last**, after all nine
owner docs and `DOCS.md` exist. Writing the table first guarantees a window
where AC-13 fails.

---

## 9. `/execute-task` rewrite — insertion, not replacement

MyFleet's current `/execute-task` and atlas's share Steps 1, 2, 3, and 5
near-verbatim (fuzzy resolve → worktree check → validate `plan.md` + `context.md`
→ invoke `subagent-driven-development` → completion handoff). Atlas adds
Steps 4a–4f. So the rewrite is an **insertion into MyFleet's existing skeleton**,
which is what keeps FR-3.5's no-regression guarantee (never create a worktree)
mechanically safe — that text is not touched.

| Step | Atlas content | MyFleet binding |
|---|---|---|
| 1–3 | atlas fuzzy-resolve via `task-resolve.sh` | **keep MyFleet's existing prose algorithm** (D1: no `task-resolve.sh`) |
| 4a | model discipline for every dispatch | as-is; owner `docs/agent-dispatch.md` |
| 4b | brief carries its file inventory; `tools/task-brief.sh` | as-is — the script is ported (FR-6.1) |
| 4c | verification runs outside the implementer, via `task-verifier` | rebind atlas verify flags → MyFleet's `--quick` / `--no-docker` / flagless |
| 4d | handle `PARTIAL` | as-is; the 120 cap must match `turn-budget.sh`'s `CAP=120` (FR-3.2) |
| 4e | hand off your own context | as-is |
| 4f | `tools/agent-ledger.sh append …` | as-is — the script is ported (D1) |
| 5 | completion handoff | keep MyFleet's, which already names `plan-adherence-reviewer` |

**Module-local build/test for the trio (FR-3.6).** MyFleet's `go.work` spans
`apps/*` + `packages/*`, so a module-local gate is:

```sh
cd apps/<svc> && go build ./... && go vet ./... && go test -race ./...
tools/lint.sh --check --go apps/<svc>        # module-scoped lint
npm run -w apps/web test                      # frontend module-local
```

Verified against `tools/lint.sh`'s usage: trailing paths restrict Go module
discovery, and golangci-lint runs in workspace mode with the root `go.work`
active — so a module-local lint needs no `go work sync`.

---

## 10. Owner-doc genericization — per-document plan

Nine documents, 1,519 atlas lines. Expected MyFleet total: ~1,150–1,300. The
substitution key, applied uniformly:

| Atlas illustration | MyFleet substitute |
|---|---|
| packet work, WZ data, IDA, service opcodes | JSON:API compound documents, Kafka event contracts, kustomize overlays |
| `services/<x>` / `libs/<y>` | `apps/<x>` / `packages/<y>` |
| `services/atlas-ui` | `apps/web` |
| `docker buildx bake` | `docker build -f apps/<svc>/Dockerfile .` |
| atlas `verify.sh` flags (`--base`, `--all`, `--no-ui`, `--facts`) | MyFleet's three flags (D2) |
| `tools/toolchain.versions` | `tools/lint.versions` |
| 86 Go modules / 14+ services | 4 services + 4 packages |
| `atlas-{implementer,verifier,reviewer}` | must not appear at all (FR-11.1) |

Per-document notes where the port is more than substitution:

- **`verification.md`** (368 → largest rewrite). Atlas's is organized around
  change detection and per-guard escape hatches that MyFleet's gate set does not
  have. Rebuild around MyFleet's four gates (D2), the `--quick`/`--no-docker`
  non-authorization semantics (FR-7.3), the unreachable-cluster skip (FR-5.9),
  and — verbatim — the local-overlay incident (FR-7.4). Keep atlas's rule that
  when the script and CI disagree, **CI is the authority and the script is the
  bug**; MyFleet's analogue is `.github/workflows/pr.yml`, which builds images
  although `make ci` does not.
- **`agent-dispatch.md`** (244). Must contain **zero** `atlas-*` references
  (FR-11.1) — MyFleet gets no historical-cutoff note because MyFleet never used
  those names. Rebind the agent roster to MyFleet's seven agents.
- **`superpowers-integration.md`** (185). Rebind bare-task-number resolution to
  MyFleet's fuzzy algorithm and `tools/task-numbers.sh` (D1); keep the
  artifact-location override rule.
- **`review-protocol.md`** (178). Add the `audit.md` vs `reviews/<unit>.md`
  split (Q2) — the one place MyFleet has a genuinely richer reviewer set than
  atlas.
- **`slice-first.md`** (107). Ports intact given D1's `doc-slice.sh`. Atlas's
  measured evidence (diff sizes, stream fractions) is kept as-is and attributed
  to atlas measurements — it is evidence, not an example, and re-measuring it in
  MyFleet is out of scope.
- **`tooling-conventions.md`** (95). Table rows for `task-facts.sh` /
  `change-surfaces.sh` rebound per D1; the "ask the tooling for a mechanical
  fact" rule survives with MyFleet commands.
- **`git-workflow.md`** (52). Plus the shared-stash rule (Q5).
- **`post-implementation.md`** (160), **`codemod-vs-agents.md`** (138). Straight
  substitution; `codemod-vs-agents.md` had zero atlas-tool references.

---

## 11. Implementation sequencing

Ordered by dependency, not by file count. Later stages depend on earlier ones
existing; the ordering is what keeps every intermediate commit self-consistent.

1. **Byte-copies** — 7 hooks + `task-brief.sh` + `doc-slice.sh` +
   `agent-ledger.sh`; `chmod +x`; run `wait-loop-guard_test.sh`; `diff` each of
   the 7 against `$ATLAS`. *(AC-1, AC-2, AC-8)*
2. **`format-on-write.sh`** rebound (D3).
3. **`tools/verify.sh` + `tools/verify_test.sh`** (D2, D4). Independent of every
   document; gates AC-4/5/6/7.
4. **Owner documents** — nine, plus `DOCS.md` (D1, Q1, §10). Independent of each
   other; the natural parallel batch.
5. **Agents** — trio + `service-documentation` (§9, Q2). Depends on 4 (they link
   owner docs) and on 3 (they name verify flags).
6. **Commands** — `/fix-pr-bug`, `/service-doc`, `/execute-task` rewrite.
   Depends on 1 (`task-brief.sh`, `agent-ledger.sh`) and 5 (agent names).
7. **`.claude/settings.json`** — wiring. Depends on 1 and 2; every referenced
   path must exist and be executable first. *(AC-11)*
8. **`CLAUDE.md`** — last (§8 sequencing constraint). *(AC-12, AC-13, AC-15)*
9. **Parity checks + `make ci`** — AC-3, AC-14, AC-16, AC-17, AC-18, and the
   AC-19 report.

**Hook-wiring hazard (PRD §7's indirect impact).** From step 7 onward, every
session in this repository — including the one finishing this very task — runs
under the new hooks. `wait-loop-guard.sh` blocks `sleep`/polling and
`turn-budget-guard.sh` denies tool calls past CAP+5 for subagents. That is
intended, but it means step 7 must come *after* the hooks are proven (step 1's
test run) and *before* only the two lowest-risk steps. If a hook misfires, the
recovery is to revert that one `settings.json` entry, not to unpick the port —
which is why wiring is a single isolated commit.

---

## 12. Risks

| Risk | Mitigation |
|---|---|
| A rule is lost in the `CLAUDE.md` restructure — invisible to every mechanical check | §8's row-by-row map, checked item by item per AC-15 |
| An owner doc references an absent path | `grep -ohE '(tools\|scripts)/[A-Za-z0-9._/-]+' docs/*.md .claude/**/*.md \| sort -u`, then `test -e` each — run as a gate, not a read-through (AC-17) |
| A ported doc silently shrinks because a rule went with its example | Per-document line-count delta reported with a one-line reason; a >40% drop needs a stated justification |
| `verify.sh` flagless run cannot reach a cluster on the machine that runs it | FR-5.9 loud skip; AC-5 is still satisfiable (exit 0), and the summary says which context was tried |
| Wired hooks degrade unrelated future work | Step 7 is one isolated commit; advisory hooks fail open (NFR-2) |
| `--quick`'s exit 0 gets mistaken for "done" | The non-authorization sentence is asserted by `verify_test.sh`, not just written |

---

## 13. What this design does not decide

- The content of `apps/*/docs/` for any service (Q1 scope guard).
- Any change under `apps/` or `packages/` — none is in scope.
- The four cross-repo §7 checks (1, 4, 5, 6). MyFleet's side is reported; the
  pairwise comparison is not evaluable here and must not be claimed (AC-19).
- Whether atlas should adopt anything back from MyFleet's port (e.g. the
  shared-stash rule). Out of scope; §2 of the spec accepts drift.
