# Process Parity Phase 2 (MyFleet) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Port atlas's context-discipline layer — enforcement hooks, agent trio, owner documents, a `tools/verify.sh` gate, and a rule-list `CLAUDE.md` — into MyFleet, bound to MyFleet's own paths and gates.

**Architecture:** Three kinds of work with different risk profiles, sequenced by dependency. Byte-copies (hooks, `task-brief.sh`, `doc-slice.sh`, `agent-ledger.sh`) land first and are verified by `diff`. One genuinely engineered script (`tools/verify.sh`) lands next with its own test. Then the text ports — nine owner documents, `DOCS.md`, four agents, three commands — each verified by a rule-survival grep rather than a read-through. `.claude/settings.json` wiring and the `CLAUDE.md` restructure land last, because the owner table can only point at files that already exist.

**Tech Stack:** Bash (hooks, `tools/*.sh`), Markdown (agents, commands, owner docs), JSON (`.claude/settings.json`). No Go, no TypeScript, no application code changes.

**Spec:** `docs/tasks/task-032-process-parity-phase-2/design.md` (design), `docs/tasks/task-032-process-parity-phase-2/prd.md` (PRD), `docs/process-parity.md` (canonical cross-repo spec), `docs/tasks/task-032-process-parity-phase-2/brief.md` (what applies here).

---

## Global Constraints

Every task's requirements implicitly include this section.

- **`$ATLAS`** is the atlas source worktree. Export it once per shell before any task that reads from it:
  ```sh
  export ATLAS=~/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename
  ```
  Confirm it is the pinned revision before copying anything:
  ```sh
  diff "$ATLAS/docs/process-parity.md" docs/process-parity.md   # must be empty
  ```
  If that diff is non-empty, **STOP** and report BLOCKED. Do not merge the two by hand.
- **Never write a literal `/home/<user>/...` path into any file under `docs/`.** Use `$ATLAS`, `~`, or a repo-relative path. This is enforced by `block-home-paths-in-docs.sh` from Task 12 onward and is a gitleaks hazard before that.
- **Scope:** this task touches `.claude/`, `docs/`, `tools/`, and `CLAUDE.md` only. **No file under `apps/` or `packages/` may change.** Verify with `git diff --name-only main...HEAD | grep -E '^(apps|packages)/'` → must print nothing.
- **The only `atlas-*` reference permitted anywhere outside `docs/tasks/` is in `docs/process-parity.md`.** (FR-11.1)
- **Do not delete a rule because its example does not transfer.** Find a MyFleet example. A shorter ported document is expected; a document missing a rule is a defect. (FR-7.2)
- **No ported document may reference a path that does not exist in MyFleet.** (FR-7.5, AC-17)
- **`make ci`** is `lint-check vet test build fe-test fe-build manifests carfax-template` (`Makefile:51`). Quote it verbatim wherever it appears.
- **The five container images** are `apps/auth-service`, `apps/fleet-service`, `apps/media-service`, `apps/notification-service`, `apps/web`. Build context is the **repo root** for all five.
- **The two overlays** are `deploy/k8s/overlays/main` and `deploy/k8s/overlays/local`. **Both** get a `kubectl apply --dry-run=server`, always as two separate gates.
- **`tools/verify.sh` flag surface is exactly three:** `--quick`, `--no-docker`, `-h`/`--help`. Atlas's `--base`, `--all`, `--no-ui`, `--facts` are **not** ported. Any ported document that names an atlas flag must be rebound.
- **Implementer tool-call cap is 120**, matching `CAP=120` in `.claude/hooks/turn-budget.sh`. Any document naming a different number is a defect.
- **Node target is 22.** Bootstrap only when `npm` is absent: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.
- **Commit after every task.** Never `git add -A` or `git add .` — stage the named paths only.
- **Work only in this worktree.** Prefix Bash with `cd <worktree> && ...` if there is any doubt; verify `git rev-parse --show-toplevel` ends with `/.worktrees/task-032-process-parity-phase-2` and `git branch --show-current` is `task-032-process-parity-phase-2` after each commit.

### Universal substitution key (applies to every text port)

| Atlas | MyFleet |
|---|---|
| `services/<x>` | `apps/<x>` |
| `libs/<y>` | `packages/<y>` |
| `services/atlas-ui` | `apps/web` |
| `libs/atlas-constants/` | `packages/shared-go/` |
| packet work, WZ data, IDA, service opcodes, MapleStory | JSON:API compound documents, Kafka event contracts, kustomize overlays |
| `docker buildx bake` | `docker build -f apps/<svc>/Dockerfile .` |
| `tools/toolchain.versions` | `tools/lint.versions` |
| `.github/workflows/pr-validation.yml` | `.github/workflows/pr.yml` |
| `tools/verify.sh --base <rev>` / `--all` / `--no-ui` / `--facts` | MyFleet's three flags (flagless / `--quick` / `--no-docker`) |
| `tools/task-resolve.sh <id>` / `--list` | the fuzzy task-identifier algorithm in the phase commands, plus `tools/task-numbers.sh next` / `check` and `git worktree list` |
| `tools/task-facts.sh <task>` | `git worktree list`, `git branch --show-current`, `ls docs/tasks/<id>/`, `tools/task-numbers.sh check` |
| `tools/change-surfaces.sh --base <sha>` | `git diff --name-only <base>...HEAD \| sed -n 's\|^\(apps\|packages\)/\([^/]*\)/.*\|\1/\2\|p' \| sort -u` |
| `atlas-{implementer,verifier,reviewer}` | must not appear at all |
| 86 Go modules / 14+ services | 4 services (`apps/*`) + 4 packages (`packages/*`) |
| "Atlas" (proper noun) | "MyFleet" |

---

## File Structure

**Created:**

| Path | Responsibility |
|---|---|
| `.claude/hooks/wait-loop-guard.sh` + `_test.sh` | block polling/`sleep` loops (byte-copy) |
| `.claude/hooks/block-home-paths-in-docs.sh` | deny home paths in `docs/` writes (byte-copy) |
| `.claude/hooks/turn-budget.sh` | count tool calls, warn at cap (byte-copy) |
| `.claude/hooks/turn-budget-guard.sh` | deny past cap+grace (byte-copy) |
| `.claude/hooks/fork-dispatch-guard.sh` | guard `Agent` fork dispatches (byte-copy) |
| `.claude/hooks/commit-boundary.sh` | advisory commit-boundary nudge (byte-copy) |
| `tools/task-brief.sh` | extract one plan task into a standalone brief (byte-copy) |
| `tools/doc-slice.sh` + `_test.sh` | read a slice of a large document (byte-copy) |
| `tools/agent-ledger.sh` + `_test.sh` | append agent cost/verdict rows (byte-copy) |
| `tools/verify.sh` | the pre-PR gate: `make ci` + container builds + two dry-runs |
| `tools/verify_test.sh` | argument parsing, exit codes, non-authorization text |
| `docs/verification.md` … `docs/git-workflow.md` (9) | owner documents |
| `DOCS.md` | root per-service documentation contract |
| `.claude/agents/task-implementer.md` | budget-capped implementer |
| `.claude/agents/task-verifier.md` | isolated gate runner |
| `.claude/agents/task-reviewer.md` | per-commit-range reviewer |
| `.claude/agents/service-documentation.md` | per-service doc generator |
| `.claude/commands/fix-pr-bug.md` | phase 5 procedure |
| `.claude/commands/service-doc.md` | dispatch `service-documentation` |

**Modified:**

| Path | Change |
|---|---|
| `.claude/hooks/format-on-write.sh` | *(new file, rebound not copied)* prettier → `apps/web` + `packages/*/src`; versions → `tools/lint.versions` |
| `.claude/commands/execute-task.md` | insert Steps 4a–4f; Steps 1–3 and 5 untouched |
| `.claude/settings.json` | `disableBundledSkills` + full hook wiring |
| `CLAUDE.md` | restructured into eight headings + owner table |

---

### Task 1: Byte-copy the portable hooks and tools

**Files:**
- Create: `.claude/hooks/wait-loop-guard.sh`, `.claude/hooks/wait-loop-guard_test.sh`, `.claude/hooks/block-home-paths-in-docs.sh`, `.claude/hooks/turn-budget.sh`, `.claude/hooks/turn-budget-guard.sh`, `.claude/hooks/fork-dispatch-guard.sh`, `.claude/hooks/commit-boundary.sh`
- Create: `tools/task-brief.sh`, `tools/doc-slice.sh`, `tools/doc-slice_test.sh`, `tools/agent-ledger.sh`, `tools/agent-ledger_test.sh`
- Test: `.claude/hooks/wait-loop-guard_test.sh`, `tools/doc-slice_test.sh`, `tools/agent-ledger_test.sh` (all copied, not written)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `.claude/hooks/turn-budget.sh` defines `CAP=120` — Task 8 and Task 11 must quote 120.
  - `tools/task-brief.sh PLAN_FILE TASK_NUMBER [OUTFILE]` — exit 0 written, 2 usage/no-such-plan, 3 no matching `Task <N>` heading. Default outfile `<repo-root>/.superpowers/sdd/<plan-basename>/task-<N>-brief.md`. Referenced by Task 11 Step 4b and by `commit-boundary.sh`.
  - `tools/doc-slice.sh <path> --outline | --section <pattern> | --rows <key>` — referenced by Task 6 (`slice-first.md`), Task 8 (`task-implementer`, `task-reviewer`), Task 11.
  - `tools/agent-ledger.sh append …` — referenced by Task 11 Step 4f and Task 13.

**Why `doc-slice.sh` and `agent-ledger.sh` are here** (design §3, decision D1): they are two files beyond the PRD's tools list. `slice-first.md` is *about* `doc-slice.sh`, and `/execute-task` Step 4f *is* `agent-ledger.sh`. Without them, AC-17 (no dangling paths) and FR-7.2 (don't delete the rule) cannot both be satisfied. Both are self-contained and repository-agnostic.

- [ ] **Step 1: Confirm the atlas pin, then copy**

```bash
export ATLAS=~/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename
diff "$ATLAS/docs/process-parity.md" docs/process-parity.md || { echo "PIN DRIFT — STOP"; exit 1; }

for f in wait-loop-guard.sh wait-loop-guard_test.sh block-home-paths-in-docs.sh \
         turn-budget.sh turn-budget-guard.sh fork-dispatch-guard.sh commit-boundary.sh; do
  cp "$ATLAS/.claude/hooks/$f" ".claude/hooks/$f"
done

for f in task-brief.sh doc-slice.sh doc-slice_test.sh agent-ledger.sh agent-ledger_test.sh; do
  cp "$ATLAS/tools/$f" "tools/$f"
done

chmod +x .claude/hooks/*.sh tools/task-brief.sh tools/doc-slice.sh tools/doc-slice_test.sh \
         tools/agent-ledger.sh tools/agent-ledger_test.sh
```

- [ ] **Step 2: Assert byte-identity and no `atlas-` strings (AC-1, AC-2)**

```bash
rc=0
for f in wait-loop-guard.sh wait-loop-guard_test.sh block-home-paths-in-docs.sh \
         turn-budget.sh turn-budget-guard.sh fork-dispatch-guard.sh commit-boundary.sh; do
  diff -q "$ATLAS/.claude/hooks/$f" ".claude/hooks/$f" || rc=1
done
echo "byte-identity rc=$rc"          # expect: byte-identity rc=0

grep -l 'atlas-' .claude/hooks/*.sh  # expect: no output
grep -n 'CAP=120' .claude/hooks/turn-budget.sh   # expect: one match
```

Expected: `rc=0`, no `atlas-` output, `CAP=120` present.

If `grep -l 'atlas-'` prints anything, **STOP and report BLOCKED** — atlas phase 1's claim that these files are atlas-free has regressed and this must be resolved upstream, not patched locally.

- [ ] **Step 3: Run the copied test suites (AC-8)**

```bash
.claude/hooks/wait-loop-guard_test.sh
tools/doc-slice_test.sh
tools/agent-ledger_test.sh
```

Expected: each exits 0 with `ok - …` lines and no `FAIL`.

If `doc-slice_test.sh` or `agent-ledger_test.sh` fails because it references an atlas-only fixture, **fix the fixture reference in the test only** (not the tool) and note the deviation in the commit message. `wait-loop-guard_test.sh` failing is a BLOCKED condition — FR-1.4 requires it to pass unmodified.

- [ ] **Step 4: Confirm `.superpowers/` stays out of `git status` (FR-6.2)**

```bash
tools/task-brief.sh docs/tasks/task-032-process-parity-phase-2/plan.md 1
cat .superpowers/sdd/.gitignore    # expect: *
git status --short | grep -c superpowers   # expect: 0
```

Expected: brief written, `.gitignore` contains `*`, `git status` shows nothing under `.superpowers/`.

- [ ] **Step 5: Commit**

```bash
git add .claude/hooks/wait-loop-guard.sh .claude/hooks/wait-loop-guard_test.sh \
        .claude/hooks/block-home-paths-in-docs.sh .claude/hooks/turn-budget.sh \
        .claude/hooks/turn-budget-guard.sh .claude/hooks/fork-dispatch-guard.sh \
        .claude/hooks/commit-boundary.sh \
        tools/task-brief.sh tools/doc-slice.sh tools/doc-slice_test.sh \
        tools/agent-ledger.sh tools/agent-ledger_test.sh
git commit -m "feat(claude): port atlas portable hooks and brief/slice/ledger tools verbatim"
```

---

### Task 2: Rebind `format-on-write.sh`

**Files:**
- Create: `.claude/hooks/format-on-write.sh`

**Interfaces:**
- Consumes: `tools/lint.versions` (defines `GOLANGCI_LINT_VERSION=v2.13.1`), `.cache/tools/bin/golangci-lint-$GOLANGCI_LINT_VERSION` (populated by `tools/lint.sh`, never by this hook), `.golangci.yml` at repo root.
- Produces: a `PostToolUse Write|Edit` hook path that Task 12 wires.

This hook is **not** a byte-copy. Three changes from atlas's; everything else — the `jq` extraction, the absolute-path guard, the go.mod walk, `exit 0` on every unhandled path, the never-bootstrap rule — is unchanged.

1. `source "$ROOT/tools/toolchain.versions"` → `source "$ROOT/tools/lint.versions"`. The variable name and the cached binary path are already identical between the repos, so only the filename changes.
2. The prettier branch matches `*/apps/web/*.ts{,x}` **and** `*/packages/*/src/*.ts{,x}`, and runs from `$ROOT` rather than from a workspace directory. MyFleet's Prettier is configured at the repo root — `tools/lint.sh` runs `npm run format:check` with cwd `$ROOT`, covering every workspace. Binding literally to `apps/web` alone would leave `packages/shared-ts` and `packages/ui-components` unformatted on write, and `make lint-check` would then fail on files the hook had just touched next door. This is a deliberate superset of FR-2.1 (design §5).
3. It must fail open on every path it does not handle (FR-2.3, NFR-2). A formatting hook never blocks a write.

- [ ] **Step 1: Write the hook**

```bash
#!/usr/bin/env bash
# PostToolUse hook — format the file a Write/Edit just touched.
#
# DELIBERATELY FAIL-OPEN: a local convenience hook must never block an edit.
# Missing toolchain, missing cached binary, unparseable input, tool error — all
# exit 0 silently. `make lint-check` / CI is the enforcement point. To avoid a
# multi-minute stall on first Write, the hook never bootstraps golangci-lint
# itself; it uses the binary only if tools/lint.sh has already cached it.
set -u

[ -t 0 ] && exit 0

input="$(cat)"
fp="$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)" || exit 0
[ -z "$fp" ] && exit 0
[ -f "$fp" ] || exit 0

# Fail-open on a non-absolute path: the hook resolves nothing relative to the
# repo, and dirname-walk on a relative path can spin. First-party Write/Edit
# always pass an absolute file_path.
case "$fp" in /*) ;; *) exit 0 ;; esac

ROOT="${CLAUDE_PROJECT_DIR:-$(pwd)}"

case "$fp" in
    *.go)
        # shellcheck source=../../tools/lint.versions
        source "$ROOT/tools/lint.versions" 2>/dev/null || exit 0
        GOLANGCI="$ROOT/.cache/tools/bin/golangci-lint-${GOLANGCI_LINT_VERSION:-}"
        [ -x "$GOLANGCI" ] || exit 0
        # Format from the file's own module dir so gofumpt sees its go.mod.
        moddir="$(dirname "$fp")"
        while [ "$moddir" != "/" ] && [ ! -f "$moddir/go.mod" ]; do
            moddir="$(dirname "$moddir")"
        done
        [ -f "$moddir/go.mod" ] || exit 0
        (cd "$moddir" && "$GOLANGCI" fmt -c "$ROOT/.golangci.yml" "$fp") >/dev/null 2>&1 || true
        ;;
    */apps/web/*.ts|*/apps/web/*.tsx|*/packages/*/src/*.ts|*/packages/*/src/*.tsx)
        # Prettier is configured at the repo root and covers every workspace.
        (cd "$ROOT" && npx --no-install prettier --write "$fp") >/dev/null 2>&1 || true
        ;;
esac

exit 0
```

- [ ] **Step 2: Make it executable and verify the fail-open paths**

```bash
chmod +x .claude/hooks/format-on-write.sh

# No stdin content at all → exit 0
printf '' | .claude/hooks/format-on-write.sh; echo "empty stdin: $?"

# Unparseable JSON → exit 0
printf 'not json' | .claude/hooks/format-on-write.sh; echo "bad json: $?"

# Relative path → exit 0, no work
printf '{"tool_input":{"file_path":"apps/web/src/main.tsx"}}' \
  | .claude/hooks/format-on-write.sh; echo "relative: $?"

# Missing file → exit 0
printf '{"tool_input":{"file_path":"/nonexistent/x.go"}}' \
  | .claude/hooks/format-on-write.sh; echo "missing: $?"
```

Expected: every line prints `: 0`.

- [ ] **Step 3: Verify the rebindings landed and no atlas string survives**

```bash
grep -n 'lint.versions'  .claude/hooks/format-on-write.sh   # expect: 2 matches (shellcheck + source)
grep -n 'apps/web'       .claude/hooks/format-on-write.sh   # expect: 2 matches
grep -n 'packages/'      .claude/hooks/format-on-write.sh   # expect: 2 matches
grep -c 'atlas'          .claude/hooks/format-on-write.sh   # expect: 0
grep -c 'toolchain.versions' .claude/hooks/format-on-write.sh  # expect: 0
```

- [ ] **Step 4: Verify the Go path actually formats when the binary is cached**

```bash
tools/lint.sh --check --go packages/shared-go >/dev/null 2>&1 || true   # bootstraps the pinned binary
ls .cache/tools/bin/                                                     # expect: golangci-lint-v2.13.1
printf '{"tool_input":{"file_path":"%s/packages/shared-go/go.mod"}}' "$PWD" \
  | .claude/hooks/format-on-write.sh; echo "go.mod (unhandled ext): $?"
```

Expected: `.cache/tools/bin/golangci-lint-v2.13.1` present; the unhandled-extension call exits 0.

- [ ] **Step 5: Commit**

```bash
git add .claude/hooks/format-on-write.sh
git commit -m "feat(claude): add format-on-write hook rebound to apps/web and tools/lint.versions"
```

---

### Task 3: `tools/verify.sh` and its test

**Files:**
- Create: `tools/verify.sh`
- Test: `tools/verify_test.sh`

**Interfaces:**
- Consumes: `make ci`, `docker`, `kustomize`, `kubectl`, `nvm`. Nothing from earlier tasks.
- Produces (referenced by Tasks 4, 8, 10, 11, 13):
  - `tools/verify.sh` — flagless: every gate, exit 0 ⟺ branch may be called done.
  - `tools/verify.sh --quick` — skips container builds **and** both dry-runs; exit 0 does **not** authorize done.
  - `tools/verify.sh --no-docker` — skips container builds only; exit 0 does **not** authorize done.
  - `tools/verify.sh -h` / `--help` — usage, exit 0. Unknown option → usage on stderr, **exit 2**.
  - Non-authorization sentence, quoted verbatim by the test and by `docs/verification.md`:
    `this run does NOT authorize calling the branch done`
  - Success sentence: `All gates passed — the branch may be called done.`
  - `VERIFY_DRY_RUN=1` — records every selected gate as PASSED without executing it, then prints the summary. Exists so `verify_test.sh` can assert argument parsing and summary text in under a second.

**Shape** (design §4, decision D2): a thin gate runner, not a change-detection engine. Atlas's 746-line version is mostly change detection over 86 Go modules; MyFleet has one monolithic entry point, so change detection buys nothing. Ported in *shape*: the `step`/`skip` helpers, the `PASSED`/`FAILED`/`SKIPPED` arrays, run-everything-then-summarize, and the two-tier exit message. Not ported: `--base`, `--all`, `--no-ui`, `--facts`.

Gates 3 and 4 are **two separate `step` entries**, never one loop. The local-overlay incident is precisely a case where `main` passed and `local` did not; a combined gate reintroduces the class of bug the gate exists to catch.

`verify.sh` calls `make ci` and stops there — it does **not** re-implement the `main`-overlay invariants. `make ci`'s `manifests` target already runs `tools/check-manifests.sh`, which renders both overlays and asserts no PVCs, no Secrets, no ClusterRole, no placeholders (`tools/check-manifests.sh:24-62`). Two sources of truth for one assertion is how they drift.

- [ ] **Step 1: Write the failing test**

Create `tools/verify_test.sh`:

```bash
#!/usr/bin/env bash
# verify_test.sh — tests for tools/verify.sh's contract.
#
# What is under test is the CONTRACT, not the gates: which exit codes the flags
# produce, and that a flagged run says in words that it does not authorize
# calling the branch done. The gates themselves take many minutes and are
# short-circuited here by VERIFY_DRY_RUN=1, which records each selected gate as
# passed without executing it.
#
# Also under test, structurally: gate outcomes are recorded in exactly one place
# each. If a future edit appends to PASSED/FAILED outside step(), the summary
# starts answering from a second source of truth and these assertions fail.

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VERIFY="$HERE/verify.sh"
[ -x "$VERIFY" ] || { echo "FATAL: $VERIFY not executable" >&2; exit 2; }

fails=0
ok()  { echo "ok   - $1"; }
bad() { echo "FAIL - $1"; fails=$((fails + 1)); }

assert_exit() {
    local desc="$1" want="$2"; shift 2
    "$@" >/dev/null 2>&1
    local got=$?
    [ "$got" = "$want" ] && ok "$desc" || bad "$desc (want exit $want, got $got)"
}
assert_contains() {
    local desc="$1" needle="$2" hay="$3"
    case "$hay" in *"$needle"*) ok "$desc" ;; *) bad "$desc (missing: $needle)" ;; esac
}
assert_eq() {
    local desc="$1" want="$2" got="$3"
    [ "$want" = "$got" ] && ok "$desc" || bad "$desc (want '$want', got '$got')"
}

NONAUTH='does NOT authorize calling the branch done'
AUTH='the branch may be called done'

# --- argument parsing ------------------------------------------------------

assert_exit "--help exits 0"        0 "$VERIFY" --help
assert_exit "-h exits 0"            0 "$VERIFY" -h
assert_exit "unknown flag exits 2"  2 "$VERIFY" --bogus
assert_exit "unknown flag exits 2 (bare word)" 2 "$VERIFY" nonsense

# --- help text names both overlays and all five images ---------------------

help_out="$("$VERIFY" --help 2>&1)"
assert_contains "help names the main overlay"  'overlays/main'  "$help_out"
assert_contains "help names the local overlay" 'overlays/local' "$help_out"
assert_contains "help names --quick"           '--quick'        "$help_out"
assert_contains "help names --no-docker"       '--no-docker'    "$help_out"

# --- non-authorization semantics -------------------------------------------

quick_out="$(VERIFY_DRY_RUN=1 "$VERIFY" --quick 2>&1)"; quick_rc=$?
assert_eq       "--quick exits 0 on success" 0 "$quick_rc"
assert_contains "--quick says it does not authorize done" "$NONAUTH" "$quick_out"

nodocker_out="$(VERIFY_DRY_RUN=1 "$VERIFY" --no-docker 2>&1)"; nodocker_rc=$?
assert_eq       "--no-docker exits 0 on success" 0 "$nodocker_rc"
assert_contains "--no-docker says it does not authorize done" "$NONAUTH" "$nodocker_out"

flagless_out="$(VERIFY_DRY_RUN=1 "$VERIFY" 2>&1)"; flagless_rc=$?
assert_eq       "flagless exits 0 on success" 0 "$flagless_rc"
assert_contains "flagless authorizes done"    "$AUTH"   "$flagless_out"
case "$flagless_out" in
    *"$NONAUTH"*) bad "flagless must NOT print the non-authorization sentence" ;;
    *)            ok  "flagless does not print the non-authorization sentence" ;;
esac

# --- gate selection --------------------------------------------------------

assert_contains "flagless selects the container-build gate" 'container builds' "$flagless_out"
assert_contains "flagless selects the main dry-run"  'main overlay'  "$flagless_out"
assert_contains "flagless selects the local dry-run" 'local overlay' "$flagless_out"
assert_contains "--quick skips the container builds" 'container builds' "$quick_out"
assert_contains "--no-docker still selects the main dry-run"  'main overlay'  "$nodocker_out"
assert_contains "--no-docker still selects the local dry-run" 'local overlay' "$nodocker_out"

# --- structural: outcomes are recorded in exactly one place each ------------

assert_eq "PASSED is appended in exactly one place" "1" "$(grep -c 'PASSED+=' "$VERIFY")"
assert_eq "FAILED is appended in exactly one place" "1" "$(grep -c 'FAILED+=' "$VERIFY")"
assert_eq "that place is inside step()" "2" \
    "$(awk '/^step\(\) \{/,/^\}/' "$VERIFY" | grep -c -e 'PASSED+=' -e 'FAILED+=')"

# The two dry-runs must be two separate step() calls, never one loop — the
# local-overlay incident is exactly the case where main passed and local did not.
assert_eq "two separate dry-run step() calls" "2" "$(grep -c 'step "cluster dry-run' "$VERIFY")"

echo
if [ "$fails" -eq 0 ]; then
    echo "verify_test.sh: all assertions passed"
    exit 0
fi
echo "verify_test.sh: $fails failure(s)" >&2
exit 1
```

```bash
chmod +x tools/verify_test.sh
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
tools/verify_test.sh; echo "exit=$?"
```

Expected: FAIL with `FATAL: .../tools/verify.sh not executable` and `exit=2`.

- [ ] **Step 3: Write `tools/verify.sh`**

```bash
#!/usr/bin/env bash
# tools/verify.sh — the pre-PR verification gate.
#
# One entry point for everything that must be clean before a branch is called
# "done". Gate 1 mirrors `make ci`, which is what CI runs; gate 2 mirrors the
# image builds in .github/workflows/pr.yml, which `make ci` does not do; gates 3
# and 4 have no CI analogue because CI has no cluster. When this script and CI
# disagree, CI is the authority and this script is the bug.
#
# Rationale, per-gate detail and what to do when a gate fails live in
# docs/verification.md. This script is the executable form of that document —
# do not restate its contents here or in CLAUDE.md.
#
# Every gate runs even after an earlier one fails, so one pass gives the
# complete picture. Exit status is non-zero if any gate failed.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

NO_DOCKER=0
QUICK=0

usage() {
    cat <<'EOF'
usage: tools/verify.sh [options]

  --no-docker   skip the container-build gate. Faster inner loop; the run then
                does NOT authorize calling the branch done.
  --quick       skip the container builds AND both cluster dry-runs. Fastest
                inner loop; the run then does NOT authorize calling the branch
                done.
  -h, --help    this message

Gates, in order:

  1. repo gate            make ci
                          (lint-check vet test build fe-test fe-build
                           manifests carfax-template)
  2. container builds     docker build -f apps/<svc>/Dockerfile .
                          for auth-service, fleet-service, media-service,
                          notification-service, web — context is the repo root
  3. cluster dry-run      kustomize build deploy/k8s/overlays/main \
                            | kubectl apply --dry-run=server -f -
  4. cluster dry-run      kustomize build deploy/k8s/overlays/local \
                            | kubectl apply --dry-run=server -f -

Gates 3 and 4 are separate on purpose: a missing namespace: in
deploy/k8s/infra-local/kustomization.yaml once broke the local overlay while
main stayed green. When no cluster is reachable, both are recorded SKIPPED with
the attempted context named, and a flagless run still exits 0.

Exit: 0 every gate that ran passed, 1 a gate failed, 2 usage error.
Only a FLAGLESS exit 0 authorizes calling the branch done.
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --no-docker) NO_DOCKER=1; shift ;;
        --quick)     QUICK=1; NO_DOCKER=1; shift ;;
        -h|--help)   usage; exit 0 ;;
        *) echo "verify.sh: unknown option $1" >&2; usage >&2; exit 2 ;;
    esac
done

# VERIFY_DRY_RUN=1 records each selected gate as passed without executing it.
# It exists for tools/verify_test.sh, which tests the CONTRACT (exit codes,
# gate selection, summary wording) rather than the gates. The selection logic
# below is the real logic — the dry run IS the real run with the work removed.
DRY="${VERIFY_DRY_RUN:-0}"

PASSED=()
FAILED=()
SKIPPED=()
LOUD_SKIPPED=()

step() {
    local label="$1"; shift
    if [ "$DRY" = "1" ]; then PASSED+=("$label"); return 0; fi
    printf '\n\033[1m── %s\033[0m\n' "$label"
    if "$@"; then
        PASSED+=("$label")
    else
        FAILED+=("$label")
        printf '\033[31m✗ %s FAILED\033[0m\n' "$label"
    fi
}

# skip: an intentional skip the caller asked for with a flag.
skip() { SKIPPED+=("$1"); }

# loud_skip: a skip the ENVIRONMENT forced. A skip that is invisible is the
# exact failure the cluster gates exist to prevent, so these get a ⚠ line of
# their own above the pass list and a closing warning.
loud_skip() { LOUD_SKIPPED+=("$1"); }

have() {
    [ "$DRY" = "1" ] && return 0
    command -v "$1" >/dev/null 2>&1
}

# ------------------------------------------------------------------ node env
#
# Bootstrap ONLY when npm is absent; when it is present, touch nothing. When it
# is present at the wrong major, warn without changing anything — a wrong-major
# Node makes an fe-test/fe-build failure look like a code defect.

node_env() {
    [ "$DRY" = "1" ] && return 0
    if ! command -v npm >/dev/null 2>&1; then
        export NVM_DIR="$HOME/.nvm"
        # shellcheck disable=SC1091
        if [ -s "$NVM_DIR/nvm.sh" ]; then
            . "$NVM_DIR/nvm.sh" && nvm use 22 >/dev/null 2>&1 || true
        fi
        command -v npm >/dev/null 2>&1 \
            || printf '\033[33mverify.sh: npm still not on PATH after nvm bootstrap; the frontend gates will fail\033[0m\n' >&2
        return 0
    fi
    local major
    major="$(node --version 2>/dev/null | sed -n 's/^v\([0-9][0-9]*\).*/\1/p')"
    if [ -n "$major" ] && [ "$major" != "22" ]; then
        printf '\033[33mverify.sh: node v%s detected; this repo targets 22 (nvm use 22)\033[0m\n' "$major" >&2
    fi
}

node_env

# ------------------------------------------------------------------ gate 1
#
# `make ci` already runs tools/check-manifests.sh via its `manifests` target,
# which renders BOTH overlays and asserts the main-overlay invariants (no PVCs,
# no Secrets, no ClusterRole, no placeholders). Do not re-implement those here.

step "make ci (lint-check vet test build fe-test fe-build manifests carfax-template)" \
    make ci

# ------------------------------------------------------------------ gate 2

SERVICES=(auth-service fleet-service media-service notification-service web)

container_builds() {
    local rc=0 svc
    for svc in "${SERVICES[@]}"; do
        printf '\n  → apps/%s\n' "$svc"
        docker build -f "apps/$svc/Dockerfile" . || rc=1
    done
    return "$rc"
}

if [ "$QUICK" -eq 1 ]; then
    skip "container builds (--quick)"
elif [ "$NO_DOCKER" -eq 1 ]; then
    skip "container builds (--no-docker)"
elif ! have docker; then
    loud_skip "container builds (docker not on PATH)"
else
    step "container builds (${#SERVICES[@]} images, context = repo root)" container_builds
fi

# ------------------------------------------------------------ gates 3 and 4

dry_run() { # dry_run <overlay>
    kustomize build "deploy/k8s/overlays/$1" | kubectl apply --dry-run=server -f -
}

cluster_reachable() {
    [ "$DRY" = "1" ] && return 0
    kubectl cluster-info --request-timeout=5s >/dev/null 2>&1
}

# The CONTEXT NAME only. Never print kubeconfig contents or credentials.
kube_context() {
    kubectl config current-context 2>/dev/null || echo "none"
}

if [ "$QUICK" -eq 1 ]; then
    skip "cluster dry-run, main overlay (--quick)"
    skip "cluster dry-run, local overlay (--quick)"
elif ! have kubectl || ! have kustomize; then
    loud_skip "cluster dry-runs, BOTH overlays (kubectl or kustomize not on PATH)"
elif ! cluster_reachable; then
    loud_skip "cluster dry-runs, BOTH overlays (no reachable cluster; context: $(kube_context))"
else
    step "cluster dry-run, main overlay"  dry_run main
    step "cluster dry-run, local overlay" dry_run local
fi

# ----------------------------------------------------------------- summary

printf '\n\033[1m════ verify.sh summary ════\033[0m\n'
for s in ${SKIPPED[@]+"${SKIPPED[@]}"};      do printf '  \033[2m− %s SKIPPED\033[0m\n' "$s"; done
for s in ${LOUD_SKIPPED[@]+"${LOUD_SKIPPED[@]}"}; do printf '  \033[33m⚠ %s SKIPPED\033[0m\n' "$s"; done
for s in ${PASSED[@]+"${PASSED[@]}"};        do printf '  \033[32m✓\033[0m %s PASSED\n' "$s"; done
for s in ${FAILED[@]+"${FAILED[@]}"};        do printf '  \033[31m✗ %s FAILED\033[0m\n' "$s"; done

if [ "${#FAILED[@]}" -gt 0 ]; then
    printf '\n\033[31m%d gate(s) FAILED — the branch is not ready.\033[0m\n' "${#FAILED[@]}"
    exit 1
fi

if [ "$QUICK" -eq 1 ] || [ "$NO_DOCKER" -eq 1 ]; then
    printf '\n\033[33mAll gates that ran passed, but slow gates were skipped by a flag:\n'
    printf 'this run does NOT authorize calling the branch done. Re-run flagless.\033[0m\n'
    exit 0
fi

if [ "${#LOUD_SKIPPED[@]}" -gt 0 ]; then
    printf '\n\033[33m⚠ %d gate(s) were skipped for lack of an environment — see the ⚠ lines above.\n' \
        "${#LOUD_SKIPPED[@]}"
    printf '  Everything that could run passed; the skipped gates were never evaluated.\033[0m\n'
fi

printf '\n\033[32mAll gates passed — the branch may be called done.\033[0m\n'
```

```bash
chmod +x tools/verify.sh
```

- [ ] **Step 4: Run the test to verify it passes (AC-4, AC-6, AC-7)**

```bash
tools/verify_test.sh; echo "exit=$?"
```

Expected: every line `ok - …`, final `verify_test.sh: all assertions passed`, `exit=0`.

- [ ] **Step 5: Verify the real gates by running the fastest real invocation**

```bash
tools/verify.sh --quick; echo "quick exit=$?"
```

Expected: `make ci` runs for real and passes; the summary shows `container builds (--quick) SKIPPED` and both `cluster dry-run … (--quick) SKIPPED`; the closing line is the non-authorization sentence; `quick exit=0`.

Do **not** run the flagless invocation here — it is deferred to Task 14, where it is the acceptance gate for the whole branch.

- [ ] **Step 6: Commit**

```bash
git add tools/verify.sh tools/verify_test.sh
git commit -m "feat(tools): add verify.sh pre-PR gate with make ci, image builds and both cluster dry-runs"
```

---

### Task 4: `docs/verification.md`

**Files:**
- Create: `docs/verification.md`
- Source: `$ATLAS/docs/verification.md` (19.4K, 368 lines)

**Interfaces:**
- Consumes: `tools/verify.sh`'s three flags and its exact sentences from Task 3.
- Produces: the owner document that Task 8 (`task-verifier`), Task 10 (`/fix-pr-bug`), Task 11 (`/execute-task` Step 4c) and Task 13 (`CLAUDE.md` owner table + `## Done means verified`) link to.

This is the largest single rewrite. Atlas's document is organized around change detection and per-guard escape hatches MyFleet's gate set does not have; rebuild it around MyFleet's four gates rather than translating section by section.

- [ ] **Step 1: Read the source in slices, not whole**

```bash
tools/doc-slice.sh "$ATLAS/docs/verification.md" --outline
```

Read the outline, then pull only the sections whose *rules* must survive. Do not paste 368 lines into context.

- [ ] **Step 2: Write `docs/verification.md`**

Required content, each item a defect if absent:

1. **The four gates**, in `tools/verify.sh` order, with the exact command each runs:
   - gate 1 `make ci` → `lint-check vet test build fe-test fe-build manifests carfax-template`
   - gate 2 `docker build -f apps/<svc>/Dockerfile .` for the five images, context the repo root
   - gate 3 `kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -`
   - gate 4 `kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -`
2. **Non-authorization semantics (FR-7.3).** Only a flagless exit 0 authorizes calling the branch done. `--quick` and `--no-docker` exit 0 on success and do not. Quote the script's sentence verbatim: `this run does NOT authorize calling the branch done`.
3. **The unreachable-cluster skip (FR-5.9).** When no cluster is reachable both dry-run gates are recorded SKIPPED with the attempted context named, and the flagless run still exits 0. State plainly that a run in that state has not verified the manifests and say what to do about it (re-run against a reachable context before the PR merges).
4. **The local-overlay incident (FR-7.4), verbatim**, as the stated evidence for why gates 3 and 4 are two gates rather than one:

   > A missing `namespace:` in `deploy/k8s/infra-local/kustomization.yaml` made `kubectl apply -k deploy/k8s/overlays/local` fail outright (`ClusterRoleBinding "myfleet-traefik" is invalid: subjects[0].namespace: Required value`) and slipped through ten reviews because only the `main` dry-run was ever run.

   Also record why `--dry-run=server` is safe to point at the shared `bee` context: it validates against the API server without persisting anything, and it needs the `traefik.io` CRDs, which bee has.
5. **The `main`-overlay invariants** — renders with no PersistentVolumeClaims, no Secrets, no ClusterRole, no placeholder values — and the fact that `tools/check-manifests.sh` (run by `make ci`'s `manifests` target) is the single place they are asserted. `verify.sh` must never re-implement them.
6. **Node bootstrap.** `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`, run only when `npm` is absent. Plus the wrong-major warning and why it is a warning and not an action.
7. **Atlas's script-vs-CI rule, kept:** when `tools/verify.sh` and CI disagree, **CI is the authority and the script is the bug.** MyFleet's analogue is `.github/workflows/pr.yml`, which builds the images although `make ci` does not — that is precisely why gate 2 exists locally.
8. **Individual `make` targets** for narrowing a failure: `make vet`, `make test`, `make build`, `make fe-test`, `make fe-build`, `make lint-check` (check-only, what CI runs), `make lint` (fixes what it can).
9. **Module-local gates** for iterating inside one module without paying for the whole repo:
   ```sh
   cd apps/<svc> && go build ./... && go vet ./... && go test -race ./...
   tools/lint.sh --check --go apps/<svc>
   npm run -w apps/web test
   ```
   Note that `golangci-lint` runs in workspace mode with the root `go.work` active, so a module-local lint needs no `go work sync`.
10. **What to do when a gate fails** — one short subsection per gate, naming the likely cause and the narrowing command.

Do **not** restate the script's usage text; link to `tools/verify.sh --help`.

- [ ] **Step 3: Verify the document (FR-7.3, FR-7.4, AC-16, AC-17)**

```bash
grep -c 'does NOT authorize calling the branch done' docs/verification.md   # expect: >= 1
grep -c 'subjects\[0\].namespace: Required value'    docs/verification.md   # expect: >= 1
grep -c 'lint-check vet test build fe-test fe-build manifests carfax-template' docs/verification.md  # >= 1
grep -c 'overlays/local'  docs/verification.md   # expect: >= 1
grep -c 'overlays/main'   docs/verification.md   # expect: >= 1
grep -c 'nvm use 22'      docs/verification.md   # expect: >= 1
grep -c 'pr.yml'          docs/verification.md   # expect: >= 1
grep -c 'atlas'           docs/verification.md   # expect: 0
grep -nE '\-\-base|\-\-all|\-\-no-ui|\-\-facts|buildx bake' docs/verification.md  # expect: no output

# every tools/ and scripts/ path named must exist
grep -ohE '(tools|scripts)/[A-Za-z0-9._/-]+' docs/verification.md | sort -u \
  | while read -r p; do [ -e "$p" ] || echo "MISSING: $p"; done   # expect: no output
```

- [ ] **Step 4: Commit**

```bash
git add docs/verification.md
git commit -m "docs: add verification.md owner document for the MyFleet gate set"
```

---

### Task 5: `agent-dispatch.md`, `review-protocol.md`, `superpowers-integration.md`

**Files:**
- Create: `docs/agent-dispatch.md` (source `$ATLAS/docs/agent-dispatch.md`, 244 lines)
- Create: `docs/review-protocol.md` (source `$ATLAS/docs/review-protocol.md`, 178 lines)
- Create: `docs/superpowers-integration.md` (source `$ATLAS/docs/superpowers-integration.md`, 185 lines)

**Interfaces:**
- Consumes: the universal substitution key; MyFleet's agent roster.
- Produces: three owner-table targets for Task 13, and the documents Task 8's agents and Task 11's `/execute-task` Step 4a link to.

**MyFleet's agent roster** — this is the authoritative list these three documents must use:

| Agent | Purpose |
|---|---|
| `task-implementer` | one plan task, 120-call budget *(Task 8)* |
| `task-verifier` | isolated `tools/verify.sh` run *(Task 8)* |
| `task-reviewer` | one commit range against its brief *(Task 8)* |
| `plan-adherence-reviewer` | every plan task actually implemented → `docs/tasks/<id>/audit.md` |
| `backend-guidelines-reviewer` | Go DOM-* / SUB-* / SEC-* checklist → `audit.md` |
| `frontend-guidelines-reviewer` | React/TS FE-* checklist → `audit.md` |
| `todo-scanner` | TODO/FIXME inventory → `docs/TODO.md` |
| `service-documentation` | per-service docs under `apps/<svc>/docs/` *(Task 9)* |

- [ ] **Step 1: `docs/agent-dispatch.md`**

Port with the substitution key. **Zero `atlas-*` references** — MyFleet never used those names, so it gets no historical-cutoff note (FR-11.1). This is the one place MyFleet's carve-out differs from atlas's.

Rules that must survive:
- The `model` pin follows the job, not the `subagent_type`; unspecified inherits Opus, which costs a large multiple of Sonnet per turn. Never use Fable for background or review workflows.
- Never dispatch an agent without an explicit `model`.
- Fan out with fresh-context agents — a named agent type plus an explicit brief. Fork only to continue an interactive debugging thread, and say why inline. *(enforced by `fork-dispatch-guard.sh`)*
- Per-unit review is `task-reviewer`, never a bare `general-purpose` dispatch.
- The handoff decision: at every durable boundary — a commit landing, a gate returning, a fan-out reporting — ask whether the next unit depends materially on this conversation's history or only on repository state. Dependency is the signal, not size. If it is resumable from repo state, hand off. Handing off means delegating to a fresh agent with a brief, not clearing. Write the diagnosis down first, one paragraph into the task folder.
- Never spend inference turns polling a process or waiting on a child agent. *(enforced by `wait-loop-guard.sh`)*
- The agent roster table above.

- [ ] **Step 2: `docs/review-protocol.md`**

Port with the substitution key, plus the one place MyFleet is genuinely richer than atlas — the two-artifact split (design §7, Q2):

- `docs/tasks/<id>/audit.md` — the **pre-PR** verdict for the whole task, written by `plan-adherence-reviewer`, `backend-guidelines-reviewer` and `frontend-guidelines-reviewer`, dispatched by `superpowers:requesting-code-review` or `/audit-plan`.
- `docs/tasks/<id>/reviews/<unit>.md` — **one commit range's** review, written by `task-reviewer`, of which there are many per task.

State explicitly that they do not share a file and why: sharing one would make the per-unit reviews clobber each other.

Other rules that must survive: verdict-first reports; a reviewer never fans out recursively; a reviewer's default is FAIL until file:line evidence proves PASS; code review is a separate gate from `tools/verify.sh` — a green gate cannot see a cross-service seam defect, so when a change crosses a service boundary, trace the event into its consumers by hand and confirm a test asserts the new contract.

- [ ] **Step 3: `docs/superpowers-integration.md`**

Port with the substitution key. Rebind bare-task-number resolution to MyFleet's mechanism (there is no `tools/task-resolve.sh` here):

- Fuzzy identifiers: `task-001-slug`, `task-001`, `001`, and `1` all resolve to the same folder. Glob `docs/tasks/task-*` and `.worktrees/*/docs/tasks/task-*`; zero matches → ask; multiple → list and let the user pick.
- Task numbers come from `tools/task-numbers.sh next` (single source of truth). `tools/task-numbers.sh check` detects collisions and runs at `SessionStart`.
- Never act on a bare task number without resolving it first.

Rules that must survive:
- **The artifact-location override.** `superpowers:brainstorming` and `superpowers:writing-plans` default to `docs/superpowers/specs/` and `docs/superpowers/plans/`. In MyFleet both go under `docs/tasks/task-NNN-slug/` instead. When invoking those skills outside a phase command, pass the task folder explicitly.
- The four-phase flow and which skill each phase invokes.
- Using a superpowers skill outside a phase command: still honor the override, still write to the task folder.
- `disableBundledSkills: true` disables Claude Code's *bundled* skills, not plugin-provided ones; the `superpowers` plugin stays enabled and the phase commands keep working.

- [ ] **Step 4: Verify all three**

```bash
for f in docs/agent-dispatch.md docs/review-protocol.md docs/superpowers-integration.md; do
  echo "--- $f"
  grep -nE 'atlas' "$f"                     # expect: no output
  grep -nE 'services/|libs/|buildx bake' "$f"   # expect: no output
  grep -ohE '(tools|scripts|docs|apps|packages)/[A-Za-z0-9._/-]+' "$f" | sort -u \
    | while read -r p; do case "$p" in *'<'*|*'>'*) continue;; esac; [ -e "$p" ] || echo "MISSING: $p"; done
done

grep -c 'reviews/' docs/review-protocol.md      # expect: >= 1
grep -c 'audit.md' docs/review-protocol.md      # expect: >= 1
grep -c 'docs/tasks/task-NNN-slug' docs/superpowers-integration.md  # expect: >= 1
grep -c 'task-numbers.sh' docs/superpowers-integration.md           # expect: >= 1
grep -c 'model' docs/agent-dispatch.md          # expect: >= 1
```

- [ ] **Step 5: Report the shrink**

```bash
for f in agent-dispatch review-protocol superpowers-integration; do
  a=$(wc -l < "$ATLAS/docs/$f.md"); m=$(wc -l < "docs/$f.md")
  printf '%-28s atlas=%s myfleet=%s (%s%%)\n' "$f" "$a" "$m" "$(( m * 100 / a ))"
done
```

Any document below 60% of its atlas line count needs a one-line written justification in the commit message. A shorter document is expected; a document missing a rule is a defect.

- [ ] **Step 6: Commit**

```bash
git add docs/agent-dispatch.md docs/review-protocol.md docs/superpowers-integration.md
git commit -m "docs: add agent-dispatch, review-protocol and superpowers-integration owner documents"
```

---

### Task 6: `slice-first.md`, `tooling-conventions.md`, `git-workflow.md`, `post-implementation.md`, `codemod-vs-agents.md`

**Files:**
- Create: `docs/slice-first.md` (source `$ATLAS/docs/slice-first.md`, 107 lines)
- Create: `docs/tooling-conventions.md` (source `$ATLAS/docs/tooling-conventions.md`, 95 lines)
- Create: `docs/git-workflow.md` (source `$ATLAS/docs/git-workflow.md`, 52 lines)
- Create: `docs/post-implementation.md` (source `$ATLAS/docs/post-implementation.md`, 160 lines)
- Create: `docs/codemod-vs-agents.md` (source `$ATLAS/docs/codemod-vs-agents.md`, 138 lines)

**Interfaces:**
- Consumes: `tools/doc-slice.sh` and `tools/agent-ledger.sh` from Task 1; `tools/verify.sh` flags from Task 3.
- Produces: five owner-table targets for Task 13. `post-implementation.md` is the document Task 10's `/fix-pr-bug` links to.

- [ ] **Step 1: `docs/slice-first.md`**

Ports nearly intact — `tools/doc-slice.sh` exists here (Task 1). Keep atlas's measured evidence (diff sizes, stream fractions) as-is and attribute it explicitly to atlas measurements; it is evidence, not an example, and re-measuring it in MyFleet is out of scope. Rebind the `--rows` example from atlas service names to MyFleet's:

```sh
tools/doc-slice.sh docs/verification.md --outline
tools/doc-slice.sh docs/verification.md --section 'Cluster dry-run'
tools/doc-slice.sh docs/TODO.md --rows fleet-service --rows media-service
```

Rules that must survive: slice a large artifact before reading it whole; outline first, then pull the one section; never paste a whole plan/diff/log into context when a slice answers the question.

- [ ] **Step 2: `docs/tooling-conventions.md`**

Rebind the tool table per the substitution key. The rows for `task-facts.sh` and `change-surfaces.sh` become MyFleet-native commands (those two scripts are deliberately not ported — they encode atlas's `services/` + `libs/` layout and guard-suite taxonomy):

| Want | MyFleet command |
|---|---|
| which task, which worktree, which branch | `git worktree list`; `git branch --show-current`; `ls docs/tasks/<id>/`; `tools/task-numbers.sh check` |
| which surfaces a diff touched | `git diff --name-only <base>...HEAD \| sed -n 's\|^\(apps\|packages\)/\([^/]*\)/.*\|\1/\2\|p' \| sort -u` |
| the next task number | `tools/task-numbers.sh next` |
| the pinned linter version | `tools/lint.versions` |
| a slice of a large document | `tools/doc-slice.sh` |

Rules that must survive:
- **Ask the tooling for a mechanical fact rather than deriving it**; find tracking docs with `Glob`/`Grep` rather than assuming a path.
- Never spend inference turns polling a long-running process. Start it in the background and let the harness notify you.
- Use repo-relative paths or placeholders in committed files; never a literal home/absolute path. *(enforced under `docs/` by `block-home-paths-in-docs.sh`)*
- Preserve existing line endings; never normalize CRLF→LF as a side effect.
- Batch a gate-log or review-artifact read with the ledger append recording its verdict into the same tool call.
- **Defer to the global config, do not restate it.** Where a convention is genuinely global rather than repository-scoped — which editing tool to reach for, shell-output proxies — this document stays silent. An echo is a second copy that will drift.

- [ ] **Step 3: `docs/git-workflow.md`**

Port with the substitution key, plus the one MyFleet-specific rule atlas's does not carry (design §7, Q5) — **the shared-stash hazard**:

> MyFleet runs many concurrent worktrees off one `.git`, so the stash stack is shared and another session may push or pop it while you work. Never use bare `git stash` / `git stash pop`. Prefer a temporary WIP commit to set work aside. If you must stash: `git stash push -u -m "<unique-tag>"`, immediately capture the SHA with `git stash list --format='%H %gs'`, restore with `git stash apply <sha>` (not `pop`), then drop the entry after re-finding its current `stash@{n}` by tag.

Rules that must survive: never commit or push directly to `main`; branch first; keep triage and fix on the same branch and produce the clean PR branch by rebase at PR time; what to do about a stray `main` commit, a push that didn't build, and a `gh` 401. Interactive git flags (`-i`) are not available in this environment.

- [ ] **Step 4: `docs/post-implementation.md`**

Port with the substitution key. This is the **phase 5** owner document — what happens after the PR is open. Rebind every gate reference to MyFleet's (`tools/verify.sh`, `make ci`, `.github/workflows/pr.yml`). It must describe the `/fix-pr-bug` procedure that Task 10 implements, and the two must agree.

- [ ] **Step 5: `docs/codemod-vs-agents.md`**

Straight substitution — the atlas source has no atlas-tool references. The rule it owns: before dispatching a *second* implementer at the same transformation, stop and write the codemod instead. Rebind any example to a MyFleet-shaped one (e.g. a mechanical rename across `apps/*` and `packages/*`, or a JSON:API field rename across handlers).

- [ ] **Step 6: Verify all five**

```bash
for f in slice-first tooling-conventions git-workflow post-implementation codemod-vs-agents; do
  echo "--- docs/$f.md"
  grep -nE 'services/|libs/|buildx bake|--base |--facts|--no-ui' "docs/$f.md"   # expect: no output
  grep -nE 'atlas-(implementer|verifier|reviewer)' "docs/$f.md"                 # expect: no output
  grep -ohE '(tools|scripts|docs|apps|packages)/[A-Za-z0-9._/-]+' "docs/$f.md" | sort -u \
    | while read -r p; do case "$p" in *'<'*|*'>'*) continue;; esac; [ -e "$p" ] || echo "MISSING: $p"; done
  a=$(wc -l < "$ATLAS/docs/$f.md"); m=$(wc -l < "docs/$f.md")
  printf '  lines: atlas=%s myfleet=%s (%s%%)\n' "$a" "$m" "$(( m * 100 / a ))"
done

grep -c 'doc-slice.sh'    docs/slice-first.md         # expect: >= 1
grep -c 'stash apply'     docs/git-workflow.md        # expect: >= 1
grep -c 'task-numbers.sh' docs/tooling-conventions.md # expect: >= 1
```

Note: the bare word `atlas` is permitted in `slice-first.md` only where it attributes a measurement to atlas. Every other occurrence in these five files is a defect. Check with `grep -n atlas docs/*.md` and justify each hit.

- [ ] **Step 7: Commit**

```bash
git add docs/slice-first.md docs/tooling-conventions.md docs/git-workflow.md \
        docs/post-implementation.md docs/codemod-vs-agents.md
git commit -m "docs: add slice-first, tooling-conventions, git-workflow, post-implementation and codemod-vs-agents owner documents"
```

---

### Task 7: `DOCS.md` — the per-service documentation contract

**Files:**
- Create: `DOCS.md` (repo root; source `$ATLAS/DOCS.md`, 261 lines)

**Interfaces:**
- Consumes: nothing.
- Produces: the contract `service-documentation` (Task 9) follows. Without it that agent points at a missing document.

**Scope guard, stated so no implementer has to discover it:** this task creates the *contract*. It does **not** write documentation for any service. No `apps/*/docs/` content is produced here or anywhere in this plan. That is a follow-up task.

Genericization (design §7, Q1):
- Title and framing → MyFleet, not Atlas.
- Per-service docs live at `apps/<svc>/docs/`. Required set: `README.md`, `docs/domain.md`, `docs/rest.md`, `docs/storage.md`, and `docs/kafka.md` for services that produce or consume events. Optional: `docs/migrations.md`.
- **The REST section is rebound to the hand-rolled JSON:API transport in `packages/shared-go/server`**, not a generic REST description. A MyFleet service's `rest.md` documents resource types, attribute sets, relationships, and included compound documents — because that is what the transport actually is.
- Kafka applies: `packages/shared-go`, `apps/fleet-service`, `apps/notification-service` and `apps/media-service` all carry Kafka in their `go.mod`. Verify this before writing the section rather than trusting the design note.
- Atlas-only sections with no MyFleet analogue are dropped **as sections**, not as rules. Nothing survives that references an absent concept.

- [ ] **Step 1: Confirm the Kafka claim against source**

```bash
grep -l 'kafka' apps/*/go.mod packages/*/go.mod
ls packages/shared-go/server/
```

Expected: the Kafka list matches the four modules named above (adjust the `DOCS.md` text to whatever this actually prints — the source is the authority, not the design note). `packages/shared-go/server/` exists.

- [ ] **Step 2: Write `DOCS.md`**

Follow the atlas structure: a short statement of the contract, the required file set with a per-file description of what belongs in it, an explicit "code is the single source of truth" rule, and an explicit "document only what exists; never infer intent or future behavior" rule.

- [ ] **Step 3: Verify**

```bash
grep -c 'apps/<svc>/docs' DOCS.md            # expect: >= 1
grep -c 'packages/shared-go/server' DOCS.md  # expect: >= 1
grep -ci 'json:api' DOCS.md                  # expect: >= 1
grep -ci 'atlas' DOCS.md                     # expect: 0
grep -nE 'services/|libs/' DOCS.md           # expect: no output
ls apps/*/docs 2>/dev/null                   # expect: no output — this task writes no service docs
```

- [ ] **Step 4: Commit**

```bash
git add DOCS.md
git commit -m "docs: add DOCS.md per-service documentation contract"
```

---

### Task 8: The agent trio

**Files:**
- Create: `.claude/agents/task-implementer.md` (source `$ATLAS/.claude/agents/task-implementer.md`)
- Create: `.claude/agents/task-verifier.md` (source `$ATLAS/.claude/agents/task-verifier.md`)
- Create: `.claude/agents/task-reviewer.md` (source `$ATLAS/.claude/agents/task-reviewer.md`)

**Interfaces:**
- Consumes: `tools/verify.sh` flags (Task 3), `tools/task-brief.sh` and `tools/doc-slice.sh` (Task 1), `CAP=120` from `.claude/hooks/turn-budget.sh` (Task 1), `docs/agent-dispatch.md` / `docs/review-protocol.md` / `docs/verification.md` (Tasks 4–5).
- Produces: three agent names that Task 11's `/execute-task` dispatches and Task 5's roster table lists.

**Module-local gate for MyFleet** (FR-3.6) — this exact block replaces atlas's `cd <worktree>/services/atlas-<svc>/atlas.com/<svc> && …`:

```sh
cd <worktree>/apps/<svc> && go build ./... && go vet ./... && go test -race ./...
tools/lint.sh --check --go apps/<svc>     # module-scoped lint
npm run -w apps/web test                  # frontend module-local
```

For a `packages/` change, the same three Go commands from that package's module root. `golangci-lint` runs in workspace mode with the root `go.work` active, so a module-local lint needs no `go work sync`.

- [ ] **Step 1: `.claude/agents/task-implementer.md`**

Port with the substitution key. **Must retain, unchanged in substance:**
- The **120 tool-call budget** and the `PARTIAL` hand-back — the implementer stops and reports `PARTIAL` rather than sprawling. The number must be exactly 120, matching `CAP=120` in `.claude/hooks/turn-budget.sh`; `turn-budget-guard.sh` denies further calls at CAP+5.
- **Module-local build/test scope only.** The implementer never runs the repo-wide gate — that is `task-verifier`'s job in its own context. Rebind atlas's forbidden-command list to MyFleet's: the implementer must not run `tools/verify.sh` (any flag, including `--quick`), `tools/lint.sh` unscoped, or `make ci`.
- **Brief-first discovery** — read `tools/task-brief.sh`'s output, not the whole `plan.md`; use `tools/doc-slice.sh` for anything large.
- The cwd discipline: prefix every Bash call with `cd <worktree> && …`, verify the branch after committing, no destructive git ops, never `git add -A` / `git add .`.
- Replace atlas's `libs/atlas-constants/` check with: check `packages/shared-go/` before defining a new domain type, alias, or numeric constant.

- [ ] **Step 2: `.claude/agents/task-verifier.md`**

Port with the substitution key. **Rebind the verify invocation**: atlas's `tools/verify.sh --quick --base <last-gated-commit>` becomes plain `tools/verify.sh --quick` — MyFleet's script has no `--base`, and the whole paragraph about a shared-lib fan-out warning goes with it (there is no change detection to fan out). Everything else stands:
- Default command `tools/verify.sh --quick`; run exactly what the controller named.
- Step 1 is always `git branch --show-current` and `git rev-parse --show-toplevel`; if the toplevel is not the worktree it was given, report `ERROR` and verify nothing.
- **It never edits anything** — no `Edit`, no `Write`, no git mutation, no `go mod tidy`, no formatting. A verifier that fixes what it measures destroys the signal and skips review.
- It runs nothing else — no exploratory greps, no reading source to explain a failure. Quote the failure; do not diagnose it.
- Report format: PASS / FAIL / ERROR blocks, under 30 lines, verbatim output, the **first** failed gate's block only (all failures named, one quoted), 20-head/20-tail elision past 40 lines.
- Never report PASS for a run that did not complete. An unrun gate is `ERROR`.
- Keep `model: haiku` and `tools: Bash, Read`.

Add one MyFleet-specific line: a `--quick` PASS does not authorize calling the branch done, and the verifier must say so when the controller asks whether the branch is ready.

- [ ] **Step 3: `.claude/agents/task-reviewer.md`**

Port with the substitution key. **Must retain:**
- Reviews **one commit range** against its brief — not the whole branch.
- **Verdict first**, then evidence.
- **Does not fan out recursively** — a reviewer never dispatches another agent.
- Writes a **durable artifact**. Artifact path, unchanged from atlas's convention: if not given, derive it as `docs/tasks/<task>/reviews/<unit>.md`. State explicitly that this is *not* `docs/tasks/<task>/audit.md`, which MyFleet's `plan-adherence-reviewer` / guideline reviewers own, and that sharing one file would make per-unit reviews clobber each other.
- Rebind the `tools/doc-slice.sh` usage examples to MyFleet documents.

- [ ] **Step 4: Verify all three (AC-9)**

```bash
ls .claude/agents/task-implementer.md .claude/agents/task-verifier.md .claude/agents/task-reviewer.md

for f in .claude/agents/task-*.md; do
  echo "--- $f"
  grep -nE 'atlas' "$f"                                  # expect: no output
  grep -nE 'services/|libs/|--base |--facts|--no-ui|--all' "$f"   # expect: no output
  head -1 "$f" | grep -q '^---' || echo "MISSING frontmatter: $f"
  grep -q '^name:' "$f" || echo "MISSING name: $f"
  grep -q '^model:' "$f" || echo "MISSING model: $f"
done

grep -c '120' .claude/agents/task-implementer.md         # expect: >= 1
grep -c 'PARTIAL' .claude/agents/task-implementer.md     # expect: >= 1
grep -c 'apps/<svc>' .claude/agents/task-implementer.md  # expect: >= 1
grep -c 'reviews/' .claude/agents/task-reviewer.md       # expect: >= 1
grep -c 'tools/verify.sh --quick' .claude/agents/task-verifier.md  # expect: >= 1

# the cap in the agent must agree with the hook (FR-3.2)
grep -o 'CAP=[0-9]*' .claude/hooks/turn-budget.sh        # expect: CAP=120

# every path any agent names must exist
grep -ohE '(tools|docs|apps|packages)/[A-Za-z0-9._/-]+' .claude/agents/task-*.md | sort -u \
  | while read -r p; do case "$p" in *'<'*|*'>'*) continue;; esac; [ -e "$p" ] || echo "MISSING: $p"; done
```

- [ ] **Step 5: Commit**

```bash
git add .claude/agents/task-implementer.md .claude/agents/task-verifier.md .claude/agents/task-reviewer.md
git commit -m "feat(claude): add task-implementer, task-verifier and task-reviewer agents"
```

---

### Task 9: `service-documentation` agent and `/service-doc`

**Files:**
- Create: `.claude/agents/service-documentation.md` (source `$ATLAS/.claude/agents/service-documentation.md`)
- Create: `.claude/commands/service-doc.md` (source `$ATLAS/.claude/commands/service-doc.md`)

**Interfaces:**
- Consumes: `DOCS.md` (Task 7), `CLAUDE.md`.
- Produces: `/service-doc <svc>` → dispatches `service-documentation`.

- [ ] **Step 1: Write `.claude/agents/service-documentation.md`**

Port with three rebindings; everything else is unchanged:
1. "Atlas Documentation Agent" → "MyFleet Documentation Agent"; `description` names MyFleet.
2. Argument resolution: accepts a service name (`fleet-service`) or a service path (`apps/fleet-service`). **Resolve to a path under `apps/`, not `services/`.**
3. The two examples in the `description` frontmatter become MyFleet ones (`/service-doc fleet-service`; "Re-document media-service from the current code.").

Keep unchanged: authoritative inputs are `CLAUDE.md` and `DOCS.md`; code is the single source of truth; document only what exists in code; never infer intent or future behavior; never modify code; operate only within the target service directory; output updated doc files only, no commentary; if a required doc file cannot be produced from the available code, ask a single targeted question and stop. Keep `model: sonnet` and `tools: Read, Grep, Glob, Write, Edit, Bash`.

- [ ] **Step 2: Write `.claude/commands/service-doc.md`**

```markdown
---
description: Generate or update documentation for one MyFleet service — dispatches the service-documentation agent
argument-hint: Service name or path (e.g., "fleet-service" or "apps/fleet-service")
---

Dispatch the `service-documentation` agent against: **$ARGUMENTS**.

The agent treats code as the single source of truth, follows `DOCS.md`, and operates only within the target service directory under `apps/`. It outputs only updated doc files — no commentary, no analysis.
```

- [ ] **Step 3: Verify**

```bash
grep -c 'apps/' .claude/agents/service-documentation.md   # expect: >= 1
grep -nE 'services/|atlas' .claude/agents/service-documentation.md  # expect: no output
grep -nE 'services/|atlas' .claude/commands/service-doc.md          # expect: no output
grep -c 'DOCS.md' .claude/agents/service-documentation.md # expect: >= 1
test -f DOCS.md && echo "DOCS.md exists"                  # expect: DOCS.md exists
head -1 .claude/commands/service-doc.md | grep -q '^---' && echo "frontmatter ok"
```

The `DOCS.md` existence check is the point of Task 7: an agent whose authoritative input is a missing file is the defect this ordering prevents.

- [ ] **Step 4: Commit**

```bash
git add .claude/agents/service-documentation.md .claude/commands/service-doc.md
git commit -m "feat(claude): add service-documentation agent and /service-doc command"
```

---

### Task 10: `/fix-pr-bug`

**Files:**
- Create: `.claude/commands/fix-pr-bug.md` (source `$ATLAS/.claude/commands/fix-pr-bug.md`, 4.3K)

**Interfaces:**
- Consumes: `docs/post-implementation.md` (Task 6), `tools/verify.sh` (Task 3).
- Produces: the phase-5 command the owner table (Task 13) points at.

- [ ] **Step 1: Port with the substitution key**

Rebindings required:
- Every gate reference → `tools/verify.sh` (flagless before the fix is pushed), `make ci`, `.github/workflows/pr.yml`.
- Atlas verify flags → MyFleet's three.
- `services/` / `libs/` → `apps/` / `packages/`.
- Any `task-resolve.sh` reference → MyFleet's fuzzy identifier algorithm.
- The document must link `docs/post-implementation.md` as its owner and must not restate it.

Rules that must survive: keep triage and fix on the same branch; produce the clean PR branch by rebase at PR time; run code review before pushing the fix, even for a one-line change; a flagless `tools/verify.sh` exit 0 before the fix is pushed.

- [ ] **Step 2: Verify**

```bash
grep -nE 'atlas|services/|libs/|--base |--facts|--no-ui|buildx bake' .claude/commands/fix-pr-bug.md  # expect: no output
grep -c 'post-implementation.md' .claude/commands/fix-pr-bug.md   # expect: >= 1
grep -c 'tools/verify.sh' .claude/commands/fix-pr-bug.md          # expect: >= 1
head -1 .claude/commands/fix-pr-bug.md | grep -q '^---' && echo "frontmatter ok"
grep -ohE '(tools|docs|apps|packages)/[A-Za-z0-9._/-]+' .claude/commands/fix-pr-bug.md | sort -u \
  | while read -r p; do case "$p" in *'<'*|*'>'*) continue;; esac; [ -e "$p" ] || echo "MISSING: $p"; done
```

- [ ] **Step 3: Commit**

```bash
git add .claude/commands/fix-pr-bug.md
git commit -m "feat(claude): add /fix-pr-bug phase-5 command"
```

---

### Task 11: Rewrite `/execute-task` to orchestrate the trio

**Files:**
- Modify: `.claude/commands/execute-task.md` (currently 2.8K; source for the inserted steps is `$ATLAS/.claude/commands/execute-task.md`, Steps 4a–4f at lines 87–350)

**Interfaces:**
- Consumes: `task-implementer` / `task-verifier` / `task-reviewer` (Task 8), `tools/task-brief.sh` and `tools/agent-ledger.sh` (Task 1), `tools/verify.sh` flags (Task 3), `docs/agent-dispatch.md` and `docs/verification.md` (Tasks 4–5).
- Produces: the orchestration that makes the turn-budget hooks and `commit-boundary.sh` meaningful (FR-3.5, the PRD's stated reason the trio must be *dispatched*, not merely present).

**This is an insertion, not a replacement** (design §9). MyFleet's current Steps 1, 2, 3 and 5 and the `## Important Rules` block already match atlas's near-verbatim and **must not be touched** — that text is what guarantees the worktree is reused and never created, and leaving it alone makes FR-3.5's no-regression requirement mechanically safe rather than a matter of care.

| Step | Action |
|---|---|
| 1–3 | **unchanged** — MyFleet's fuzzy-resolve prose algorithm stays (there is no `tools/task-resolve.sh` here) |
| 4 | keep the existing `subagent-driven-development` invocation; append the six sub-steps below |
| 4a | model discipline for every dispatch → `docs/agent-dispatch.md` |
| 4b | the brief carries its file inventory; produce it with `tools/task-brief.sh` |
| 4c | verification runs **outside** the implementer, via `task-verifier` |
| 4d | handle `PARTIAL` |
| 4e | hand off your own context |
| 4f | record what each agent cost via `tools/agent-ledger.sh append …` |
| 5 | **unchanged** — already names `plan-adherence-reviewer` |
| Important Rules | **unchanged** |

- [ ] **Step 1: Read the atlas source in slices**

```bash
tools/doc-slice.sh "$ATLAS/.claude/commands/execute-task.md" --outline
tools/doc-slice.sh "$ATLAS/.claude/commands/execute-task.md" --section 'Step 4a'
tools/doc-slice.sh "$ATLAS/.claude/commands/execute-task.md" --section 'Step 4b'
tools/doc-slice.sh "$ATLAS/.claude/commands/execute-task.md" --section 'Step 4c'
tools/doc-slice.sh "$ATLAS/.claude/commands/execute-task.md" --section 'Step 4d'
tools/doc-slice.sh "$ATLAS/.claude/commands/execute-task.md" --section 'Step 4e'
tools/doc-slice.sh "$ATLAS/.claude/commands/execute-task.md" --section 'Step 4f'
```

- [ ] **Step 2: Insert Steps 4a–4f after the existing Step 4**

Content requirements per sub-step:

- **4a — Model discipline.** Every dispatch names an explicit `model`; unspecified inherits Opus at a large multiple of Sonnet's per-turn cost. `task-verifier` is `haiku`. Never Fable for background or review work. Owner: `docs/agent-dispatch.md`.
- **4b — Brief-first.** Produce each implementer's brief with:
  ```sh
  tools/task-brief.sh docs/tasks/<id>/plan.md <N>
  ```
  Exit 3 means no `Task <N>` heading — fix the plan, do not hand-assemble the brief from `plan.md`, which is exactly the context bloat the brief prevents. Confirm the brief carries its own file inventory (the `**Files:**` and `**Interfaces:**` blocks) before dispatching; a brief without them makes the implementer re-derive scope.
- **4c — Verification runs outside the implementer.** After an implementer reports DONE, dispatch `task-verifier` with the worktree path and `tools/verify.sh --quick`. The implementer never runs the repo-wide gate itself — the same run costs a fraction of the tokens in a clean context and its output never lands in the implementer's window. State explicitly: a `--quick` PASS is a per-task gate, **not** authorization to call the branch done; that requires a flagless `tools/verify.sh` exit 0 at the end (`docs/verification.md`).
- **4d — Handle `PARTIAL`.** The implementer's cap is **120** tool calls, matching `CAP=120` in `.claude/hooks/turn-budget.sh`; `turn-budget-guard.sh` denies further calls at 125. A `PARTIAL` hand-back is a signal the task was mis-sized, not a failure to retry blindly: read what landed, commit it if it is coherent, and either split the remainder into a follow-up dispatch with a narrowed brief or amend the plan.
- **4e — Hand off your own context.** At every durable boundary ask whether the next unit depends on this conversation's history or only on repository state. If repo state suffices, write the one-paragraph diagnosis into the task folder and delegate to a fresh agent with a brief. Owner: `docs/agent-dispatch.md`.
- **4f — Record what each agent cost.**
  ```sh
  tools/agent-ledger.sh append …
  ```
  Batch the ledger append with the read of the gate log or review artifact whose verdict it records — same tool call, not two.

- [ ] **Step 3: Verify the rewrite (AC-10) and prove the no-regression guarantee**

```bash
# the trio is dispatched by name
grep -c 'task-implementer' .claude/commands/execute-task.md   # expect: >= 1
grep -c 'task-verifier'    .claude/commands/execute-task.md   # expect: >= 1
grep -c 'task-reviewer'    .claude/commands/execute-task.md   # expect: >= 1

# all six sub-steps present
for s in 4a 4b 4c 4d 4e 4f; do
  grep -q "Step $s" .claude/commands/execute-task.md || echo "MISSING Step $s"
done

# the worktree guarantee is intact, byte for byte
grep -c 'NEVER create a new one' .claude/commands/execute-task.md   # expect: 1
grep -c 'Do NOT create a new worktree' .claude/commands/execute-task.md  # expect: 1
git diff main -- .claude/commands/execute-task.md | grep '^-' | grep -iE 'worktree'
# expect: NO removed line mentions worktree — Steps 1-3 and the Rules block are untouched

# cap agreement and no atlas leakage
grep -c '120' .claude/commands/execute-task.md   # expect: >= 1
grep -nE 'atlas|services/|libs/|task-resolve\.sh|--base |--facts' .claude/commands/execute-task.md  # expect: no output

grep -ohE '(tools|docs)/[A-Za-z0-9._/-]+' .claude/commands/execute-task.md | sort -u \
  | while read -r p; do case "$p" in *'<'*|*'>'*) continue;; esac; [ -e "$p" ] || echo "MISSING: $p"; done
```

If any `^-` line mentioning `worktree` appears in the diff, **revert and redo the insertion** — the no-regression guarantee has been broken.

- [ ] **Step 4: Commit**

```bash
git add .claude/commands/execute-task.md
git commit -m "feat(claude): orchestrate the task agent trio from /execute-task"
```

---

### Task 12: Wire `.claude/settings.json`

**Files:**
- Modify: `.claude/settings.json`

**Interfaces:**
- Consumes: every hook from Tasks 1 and 2 — all must exist and be executable before this lands.
- Produces: the enforcement layer. **From this commit onward every session in this repository runs under these hooks**, including the session finishing this branch.

**Hazard, stated plainly** (design §11): `wait-loop-guard.sh` blocks `sleep` and polling loops, `block-home-paths-in-docs.sh` rejects absolute home paths under `docs/`, and `turn-budget-guard.sh` denies subagent tool calls past 125. That is the point of the task. It also means a misfiring hook degrades all work, not just this branch. This is therefore **one isolated commit**: if a hook misfires, the recovery is to revert this single commit, not to unpick the port.

- [ ] **Step 1: Verify every hook exists and is executable before wiring**

```bash
for h in block-home-paths-in-docs fork-dispatch-guard wait-loop-guard turn-budget-guard \
         format-on-write turn-budget commit-boundary task-num-collision-detector \
         skill-activation-prompt; do
  f=".claude/hooks/$h.sh"
  [ -x "$f" ] && echo "ok   $f" || echo "FAIL $f (missing or not executable)"
done
```

Expected: nine `ok` lines. Any `FAIL` is a BLOCKED condition — do not wire a path that does not resolve.

- [ ] **Step 2: Write `.claude/settings.json`**

```json
{
  "disableBundledSkills": true,
  "permissions": {
    "allow": [],
    "deny": [],
    "ask": []
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/block-home-paths-in-docs.sh"
          }
        ]
      },
      {
        "matcher": "Agent",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/fork-dispatch-guard.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/wait-loop-guard.sh"
          }
        ]
      },
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/turn-budget-guard.sh"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/format-on-write.sh"
          }
        ]
      },
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/turn-budget.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/commit-boundary.sh"
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/task-num-collision-detector.sh"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "$CLAUDE_PROJECT_DIR/.claude/hooks/skill-activation-prompt.sh"
          }
        ]
      }
    ]
  },
  "enabledPlugins": {
    "superpowers@claude-plugins-official": true
  }
}
```

Note what is preserved: `enabledPlugins` is untouched (FR-9.3), and both pre-existing hook entries (`SessionStart`, `UserPromptSubmit`) keep their commands exactly.

- [ ] **Step 3: Verify (AC-11)**

```bash
jq empty .claude/settings.json && echo "valid JSON"
jq -r '.disableBundledSkills' .claude/settings.json                       # expect: true
jq -r '.enabledPlugins["superpowers@claude-plugins-official"]' .claude/settings.json  # expect: true
jq -r '[.hooks[][].hooks[].command] | length' .claude/settings.json       # expect: 9

# every wired path resolves to an executable file
jq -r '[.hooks[][].hooks[].command] | .[]' .claude/settings.json \
  | sed "s|\$CLAUDE_PROJECT_DIR|$PWD|" \
  | while read -r c; do [ -x "$c" ] && echo "ok   $c" || echo "FAIL $c"; done
```

Expected: `valid JSON`, `true`, `true`, `9`, and nine `ok` lines with no `FAIL`.

- [ ] **Step 4: Smoke-test the two hooks most likely to misfire**

```bash
# block-home-paths-in-docs: allows a repo-relative docs write
printf '{"tool_input":{"file_path":"docs/x.md","content":"see docs/verification.md"}}' \
  | .claude/hooks/block-home-paths-in-docs.sh; echo "allow: rc=$? (expect empty output, rc=0)"

# and denies a home path under docs/
# (the fixture is assembled at runtime so this plan file never carries the literal)
HOMEISH="/$(echo home)/someone/repo"
printf '{"tool_input":{"file_path":"docs/x.md","content":"%s"}}' "$HOMEISH" \
  | .claude/hooks/block-home-paths-in-docs.sh | jq -r '.hookSpecificOutput.permissionDecision'
# expect: deny

# turn-budget is cheap and writes outside the repo
.claude/hooks/turn-budget.sh </dev/null >/dev/null 2>&1; echo "turn-budget rc=$?"
git status --short | wc -l    # expect: 0 — no hook wrote a tracked file
```

- [ ] **Step 5: Commit — this one alone, nothing else staged**

```bash
git add .claude/settings.json
git commit -m "feat(claude): wire the full hook set and disable bundled skills"
git show --stat HEAD    # expect: exactly one file changed
```

---

### Task 13: Restructure `CLAUDE.md`

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: all nine owner documents (Tasks 4–6), `DOCS.md` (Task 7), every agent and command (Tasks 8–11), `tools/verify.sh` (Task 3).
- Produces: the eight-heading rule list plus the owner table.

**This is last on purpose.** The owner table is a trigger → owner table and **every target in it must exist** (FR-10.2). Writing it earlier guarantees a window where AC-13 fails. Confirm before starting:

```bash
for f in docs/agent-dispatch.md docs/verification.md docs/superpowers-integration.md \
         docs/review-protocol.md docs/post-implementation.md docs/codemod-vs-agents.md \
         docs/slice-first.md docs/tooling-conventions.md docs/git-workflow.md \
         DOCS.md docs/process-parity.md; do
  [ -f "$f" ] && echo "ok   $f" || echo "FAIL $f"
done
ls -d docs/runbooks    # expect: docs/runbooks
```

All must be `ok` before Step 1.

- [ ] **Step 1: Write `CLAUDE.md` with exactly these eight headings, in order (FR-10.1)**

After `# MyFleet`:
`## Never do this`, `## Evidence & grounding`, `## Development workflow`, `## Done means verified`, `## Dispatching agents`, `## Handing off context`, `## Repository conventions`, `## Where the procedures live`.

**The rule-survival map (FR-10.3, AC-15).** Every left-hand row must land where the right-hand column says. This is the highest-risk deliverable in the plan: a lost rule is invisible to every other mechanical check.

| Rule (currently prose in `CLAUDE.md`) | Lands in |
|---|---|
| `make ci` target list | `## Done means verified` + `docs/verification.md` |
| Node/nvm bootstrap | `docs/verification.md` (mechanics); `## Done means verified` references it |
| Manifest render + **dual** `--dry-run=server` | `## Done means verified` (one line) + `docs/verification.md` (full) |
| Local-overlay `namespace:` incident | `docs/verification.md`, verbatim, as the evidence for the dual dry-run |
| Never edit the main repo when a task worktree exists | `## Never do this` |
| Search all worktrees before concluding a file is missing | `## Development workflow` |
| Artifact-location override (`docs/tasks/task-NNN-slug/`) | `## Development workflow` + `docs/superpowers-integration.md` |
| Code review before PR | `## Done means verified` |
| Design/plan output style — write the file, don't interview | `## Development workflow` |
| Verification over memory | `## Evidence & grounding` |
| Four-phase flow + fuzzy task-identifier resolution | `## Development workflow` + `docs/superpowers-integration.md` |
| Container build context is the repo root for every service, `apps/web` included | `## Repository conventions` |
| Asked to understand or plan? Do not implement | `## Development workflow` |
| Prefer straightforward moves over re-exported type aliases; no cross-boundary internals | `## Repository conventions` |
| `/audit-plan` + the three reviewer agents, `skill-rules.json` triggers | `## Dispatching agents` + `docs/review-protocol.md` |
| Task numbers from `tools/task-numbers.sh next` | `## Development workflow` |
| Deploy runbooks | `## Where the procedures live` → `docs/runbooks/` |

**New rules the ported layer adds**, each one line:
- `## Never do this`: never commit or push directly to `main`; never invent a value, name, output, or behavior; never claim verified from a flagged or partial run; never open a PR without code review; never dispatch an agent without an explicit `model`; never land a placeholder comment or stubbed handler; never spend inference turns polling a process or waiting on a child agent *(enforced)*.
- `## Evidence & grounding`: unverified is "unknown / unverified," never a plausible guess; quote actual tool output before concluding; sweep, don't spot-check; finish producible work rather than declaring a "follow-up" for a prerequisite you can produce yourself.
- `## Done means verified`: before calling a branch done, ready for PR, or invoking `superpowers:finishing-a-development-branch`, the **flagless** `tools/verify.sh` must exit 0; `--quick`/`--no-docker` also exit 0 but do not count.
- `## Dispatching agents`: the model pin follows the job; fan out with fresh-context agents, fork only to continue an interactive debugging thread; per-unit review is `task-reviewer`, never a bare `general-purpose` dispatch; read `docs/agent-dispatch.md` before dispatching and `docs/review-protocol.md` before dispatching a reviewer.
- `## Handing off context`: the whole section, per `docs/agent-dispatch.md`.
- `## Repository conventions`: check `packages/shared-go/` before defining a new domain type, alias, or numeric constant; repo-relative paths in committed files, never a literal home path *(enforced under `docs/`)*; preserve existing line endings; ask the tooling for a mechanical fact; slice a large artifact before reading it whole; batch a gate-log read with the ledger append recording its verdict; never bare `git stash`/`git stash pop` — the stack is shared across worktrees.

**The owner table.** MyFleet's rows — atlas's last four (`docs/packets/`, `docs/reverse-engineering.md`, `docs/adding-a-new-service.md`, `docs/observability.md`) are dropped per the PRD non-goals:

```markdown
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
```

Keep repo-specific content repo-specific (FR-10.4): the project overview line under `# MyFleet`, the build commands, deployment specifics and domain conventions stay in `CLAUDE.md` rather than being exiled to an owner doc.

- [ ] **Step 2: Verify the headings (AC-12)**

```bash
grep -n '^## ' CLAUDE.md
```

Expected, in this exact order:
```
## Never do this
## Evidence & grounding
## Development workflow
## Done means verified
## Dispatching agents
## Handing off context
## Repository conventions
## Where the procedures live
```

`## Where the procedures live` must be the last heading in the file.

- [ ] **Step 3: Verify every link resolves (AC-13, FR-10.2)**

```bash
grep -oE '\]\(([^)]+)\)' CLAUDE.md | sed 's/^](//; s/)$//' \
  | while read -r p; do [ -e "$p" ] || echo "DANGLING: $p"; done
```

Expected: no output. A dangling row silently deletes a procedure — this is a hard stop, not a warning.

- [ ] **Step 4: Verify the rule-survival map item by item (AC-15)**

Run each check; every one must find its rule somewhere in `CLAUDE.md` **or** in a document the owner table points at. Check them one at a time, not in aggregate.

```bash
check() { # check <label> <pattern> <file...>
  if grep -qE "$2" "${@:3}"; then echo "ok   - $1"; else echo "FAIL - $1"; fi
}
D="CLAUDE.md docs/verification.md docs/superpowers-integration.md docs/review-protocol.md \
   docs/agent-dispatch.md docs/tooling-conventions.md docs/git-workflow.md docs/slice-first.md \
   docs/post-implementation.md docs/codemod-vs-agents.md"

# shellcheck disable=SC2086
check "make ci target list"          'lint-check vet test build fe-test fe-build manifests carfax-template' $D
check "node/nvm bootstrap"           'nvm use 22' $D
check "dual dry-run"                 'overlays/local' $D
check "local-overlay incident"       'subjects\[0\]\.namespace: Required value' $D
check "never edit main repo"         'main repo when a task worktree exists' CLAUDE.md
check "search all worktrees"         'git worktree list|all worktrees' $D
check "artifact-location override"   'docs/tasks/task-NNN-slug' $D
check "code review before PR"        'code review before opening a PR|before opening a PR' $D
check "design/plan output style"     'directly to (the )?file|do not walk sections' $D
check "verification over memory"     'read the (local )?source|repo source' $D
check "four-phase flow"              '/spec-task' $D
check "fuzzy task identifiers"       'task-001|fuzzy' $D
check "container build context"      'context is the repo root|context = repo root' $D
check "plan? do not implement"       'Do not implement|DO NOT start implementing' $D
check "no re-exported type aliases"  're-export' $D
check "no cross-boundary internals"  "another layer's internals" $D
check "reviewer agents"              'plan-adherence-reviewer' $D
check "task numbers single source"   'tools/task-numbers.sh next' $D
check "deploy runbooks"              'docs/runbooks' CLAUDE.md
check "flagless verify authorizes"   'flagless' $D
check "shared-stash hazard"          'git stash apply|stash push -u' $D
```

Expected: 21 `ok` lines, zero `FAIL`. Any `FAIL` means the rule was lost in the restructure — restore it before committing.

- [ ] **Step 5: Verify no atlas leakage and no dead paths in CLAUDE.md**

```bash
grep -nE 'atlas|services/|libs/' CLAUDE.md    # expect: no output
grep -ohE '(tools|docs|apps|packages)/[A-Za-z0-9._/-]+' CLAUDE.md | sort -u \
  | while read -r p; do case "$p" in *'<'*|*'>'*) continue;; esac; [ -e "$p" ] || echo "MISSING: $p"; done
wc -l CLAUDE.md    # informational — expect roughly 80-110 lines
```

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: restructure CLAUDE.md into an eight-heading rule list with an owner table"
```

---

### Task 14: Parity checks, full gate, and the AC-19 report

**Files:**
- Create: `docs/tasks/task-032-process-parity-phase-2/parity-report.md`

**Interfaces:**
- Consumes: everything.
- Produces: the evidence that AC-1 … AC-19 hold, and the honest statement of what was not evaluable here.

- [ ] **Step 1: Run the two in-repo parity checks (AC-2, AC-3)**

```bash
grep -l 'atlas-' .claude/hooks/*.sh    # AC-2 — expect: no output

git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
  | grep -vxE 'docs/process-parity\.md'   # AC-3 — expect: no output
```

Both must print nothing. `docs/process-parity.md` is the **only** exempt file in MyFleet — the `docs/agent-dispatch.md` exemption in the spec is atlas-only, because atlas is the only repo that ever used those names (FR-11.1).

- [ ] **Step 2: Re-run every mechanical acceptance check**

```bash
export ATLAS=~/source/atlas-ms/atlas/.worktrees/task-266-process-parity-agent-rename

# AC-1 — the seven portable hooks are byte-identical
rc=0
for f in wait-loop-guard.sh wait-loop-guard_test.sh block-home-paths-in-docs.sh \
         turn-budget.sh turn-budget-guard.sh fork-dispatch-guard.sh commit-boundary.sh; do
  diff -q "$ATLAS/.claude/hooks/$f" ".claude/hooks/$f" || rc=1
done
echo "AC-1 rc=$rc"                                        # expect: 0

# AC-4 — the three tools exist and are executable
for t in tools/verify.sh tools/task-numbers.sh tools/task-brief.sh; do
  [ -x "$t" ] && echo "AC-4 ok $t" || echo "AC-4 FAIL $t"
done

# AC-7 — help and unknown flag
tools/verify.sh --help >/dev/null; echo "AC-7 help=$?"    # expect: 0
tools/verify.sh --bogus >/dev/null 2>&1; echo "AC-7 bogus=$?"  # expect: 2

# AC-6 — flagged runs exit 0 and disclaim authorization
tools/verify_test.sh; echo "AC-6/7 test exit=$?"          # expect: 0

# AC-8 — the copied hook test
.claude/hooks/wait-loop-guard_test.sh; echo "AC-8 exit=$?"   # expect: 0

# AC-9 — the four agents
ls .claude/agents/task-implementer.md .claude/agents/task-verifier.md \
   .claude/agents/task-reviewer.md .claude/agents/service-documentation.md

# AC-10 — the two new commands, and execute-task dispatching the trio by name
ls .claude/commands/fix-pr-bug.md .claude/commands/service-doc.md
grep -c -e task-implementer -e task-verifier -e task-reviewer .claude/commands/execute-task.md

# AC-11 — settings.json
jq empty .claude/settings.json && echo "AC-11 valid JSON"
jq -r '.disableBundledSkills' .claude/settings.json
jq -r '[.hooks[][].hooks[].command] | length' .claude/settings.json   # expect: 9
jq -r '[.hooks[][].hooks[].command] | .[]' .claude/settings.json \
  | sed "s|\$CLAUDE_PROJECT_DIR|$PWD|" \
  | while read -r c; do [ -x "$c" ] || echo "AC-11 FAIL $c"; done

# AC-12 — the eight headings in order
grep -n '^## ' CLAUDE.md

# AC-13 — every CLAUDE.md link resolves
grep -oE '\]\(([^)]+)\)' CLAUDE.md | sed 's/^](//; s/)$//' \
  | while read -r p; do [ -e "$p" ] || echo "AC-13 DANGLING: $p"; done

# AC-14 — all nine owner documents
for f in agent-dispatch verification superpowers-integration review-protocol \
         post-implementation codemod-vs-agents slice-first tooling-conventions git-workflow; do
  [ -f "docs/$f.md" ] && echo "AC-14 ok docs/$f.md" || echo "AC-14 FAIL docs/$f.md"
done

# AC-16 — the incident text survives somewhere durable
grep -rl 'subjects\[0\]\.namespace: Required value' docs/ CLAUDE.md

# AC-17 — no ported document references an absent path
grep -ohE '(tools|scripts)/[A-Za-z0-9._/-]+' CLAUDE.md DOCS.md docs/*.md \
    .claude/agents/*.md .claude/commands/*.md | sort -u \
  | while read -r p; do case "$p" in *'<'*|*'>'*) continue;; esac; [ -e "$p" ] || echo "AC-17 MISSING: $p"; done

# scope: no application code changed
git diff --name-only main...HEAD | grep -E '^(apps|packages)/' || echo "scope ok: no apps/ or packages/ change"
```

Expected: `AC-1 rc=0`; every `ok` line present; no `FAIL`, `DANGLING`, or `MISSING`; `AC-7 help=0`, `AC-7 bogus=2`; `AC-11` prints `true` and `9`; `scope ok`.

- [ ] **Step 3: Run the flagless gate (AC-5, AC-18)**

```bash
tools/verify.sh; echo "AC-5 flagless exit=$?"
```

Expected: `exit=0`, and the closing line `All gates passed — the branch may be called done.` This run also satisfies AC-18, since gate 1 is `make ci`.

If the cluster is unreachable, the two dry-run gates are recorded with `⚠ … SKIPPED (no reachable cluster; context: <name>)` and the run still exits 0 (FR-5.9). That is a **pass with a named gap**, not a clean pass — record the context name in the report and say plainly that the manifests were not dry-run.

- [ ] **Step 4: Write `docs/tasks/task-032-process-parity-phase-2/parity-report.md`**

Sections:

1. **In-repo checks (checks 2 and 3).** The commands from Step 1 and their output. Both must have printed nothing; say so with the command quoted.
2. **Acceptance criteria.** One row per AC-1 … AC-18 with the command run and its result. Quote actual output — never paraphrase a count from memory.
3. **AC-19 — not evaluable here.** State plainly:

   > `docs/process-parity.md` §7 checks 1, 4, 5 and 6 are pairwise comparisons between repositories. They are **not evaluable from MyFleet alone**, and this report does not claim they pass. What follows is MyFleet's side of each, for whoever performs the comparison.

   Then report MyFleet's side of each of the four: which files MyFleet carries, at what content, with the `diff` results against `$ATLAS` where a byte-identity claim is in scope.
4. **Deviations from the PRD, each with its reason.** At minimum:
   - `tools/doc-slice.sh` and `tools/agent-ledger.sh` ported beyond the PRD's tools list (design D1) — the minimum that makes FR-7.2 and AC-17 simultaneously satisfiable.
   - `format-on-write.sh`'s prettier match is a superset of FR-2.1, covering `packages/*/src` as well as `apps/web` (design D3).
   - `verify.sh` warns on a non-22 Node major, which FR-5.10 does not require (design D4).
   - `tools/verify_test.sh` and the `VERIFY_DRY_RUN` short-circuit, which no FR names (design D2).
   - Any per-document shrink below 60% of its atlas line count, with the justification.
5. **What was skipped and why**, if the flagless run recorded any `⚠` skip.

- [ ] **Step 5: Commit**

```bash
git add docs/tasks/task-032-process-parity-phase-2/parity-report.md
git commit -m "docs(task-032): record parity check results and the AC-19 non-evaluable report"
```

- [ ] **Step 6: Code review before PR**

Run `superpowers:requesting-code-review`. It dispatches `plan-adherence-reviewer`; the backend and frontend guideline reviewers do not apply here, since no Go or TypeScript file changed. Findings land in `docs/tasks/task-032-process-parity-phase-2/audit.md`.

Do not open a PR before this step completes.

---

## Self-Review

**Spec coverage** — every PRD functional requirement mapped to a task:

| FR | Task |
|---|---|
| FR-1.1 … FR-1.5 (portable hooks) | 1 |
| FR-2.1 … FR-2.3 (`format-on-write.sh`) | 2 |
| FR-3.1 … FR-3.4 (agent trio) | 8 |
| FR-3.5, FR-3.6 (`/execute-task`) | 11, 8 |
| FR-4.1 (`service-documentation`, `/service-doc`) | 9; `DOCS.md` in 7 |
| FR-5.1 … FR-5.11 (`tools/verify.sh`) | 3 |
| FR-6.1, FR-6.2 (`tools/task-brief.sh`) | 1 |
| FR-7.1 … FR-7.5 (owner documents) | 4, 5, 6 |
| FR-8.1 (`/fix-pr-bug`) | 10 |
| FR-9.1 … FR-9.4 (`settings.json`) | 12 |
| FR-10.1 … FR-10.4 (`CLAUDE.md`) | 13 |
| FR-11.1 (the MyFleet carve-out) | 5 (agent-dispatch), 14 (the check) |
| AC-1 … AC-19 | 14, plus per-task verification steps |
| Design D1 (ancillary tools) | 1, 6, 11 |
| Design D2 (`verify.sh` shape) | 3 |
| Design D3 (`format-on-write.sh`) | 2 |
| Design D4 (Node wrinkle) | 3 |
| Design Q1 (`DOCS.md`) | 7 |
| Design Q2 (`reviews/<unit>.md`) | 5, 8 |
| Design Q3 (turn-budget storage) | 1 — verified in design; copy verbatim, no adaptation |
| Design Q4 (`disableBundledSkills`) | 12 — verified in design; adopt as specified |
| Design Q5 (global-config overlap) | 6 |

**Gaps deliberately left, and why:**
- Design §13's non-decisions stand: no `apps/*/docs/` content, no `apps/`/`packages/` change, no cross-repo §7 comparison. Task 14 reports rather than asserts.
- No committed path-checking tool. The AC-17 sweep is an inline command in Tasks 4–14 rather than a new `tools/` script, because the design scopes exactly two new tools plus two ports.

**Type/name consistency:** `tools/verify.sh`'s two summary sentences (`this run does NOT authorize calling the branch done`, `All gates passed — the branch may be called done.`) are defined once in Task 3 and quoted by Task 3's test, Task 4's `verification.md` check, and Task 13's rule-survival check. The cap `120` is defined by `.claude/hooks/turn-budget.sh` (Task 1) and quoted by Tasks 8 and 11. The review artifact paths `docs/tasks/<id>/audit.md` and `docs/tasks/<id>/reviews/<unit>.md` are defined in Task 5 and reused in Task 8.
