# Go 1.27 Toolchain Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse every Go version-declaring surface in the repo — module directives, builder images, CI inputs, and documentation — onto Go 1.27, raising the golangci-lint pin first so the linter can type-check the 1.27 standard library.

**Architecture:** This is a version bump, not a feature adoption. The only design decisions are ordering and atomicity. The pinned linter (`golangci-lint v2.12.2`) cannot type-check Go 1.27 stdlib source, so Task 1 raises the pin to `v2.13.1` and absorbs its three gofumpt findings *before* anything else moves — that commit is independently green on the current toolchain. Tasks 2–5 then sweep the version surfaces in the order tooling forces (`go` directives → `make tidy` → Dockerfiles → CI → docs), and Task 6 adds the Renovate mechanism that keeps those surfaces from drifting apart again. Tasks 7–9 verify: full `make ci`, container builds, and manifest renders.

**Tech Stack:** Go 1.27 (workspace of 6 modules via `go.work`), golangci-lint v2.13.1 (gofumpt + goimports formatters, `default: standard` analyzer group), Docker multi-stage builds on `golang:1.27-alpine` / `alpine:3.24`, GitHub Actions (`actions/setup-go@v7`), kustomize, Renovate (`custom.regex` manager).

**Spec:** [design.md](design.md) — read it before starting. PRD: [prd.md](prd.md). Probe baseline: [probe-results.md](probe-results.md).

## Global Constraints

Copied verbatim from the spec. Every task's requirements implicitly include these.

- **Go directive is exactly `go 1.27.0`** in all seven files (`go.work` + six `go.mod`). Not `1.27`, not `1.27.1`.
- **No `toolchain` directive** may exist in `go.work` or any `go.mod`. If `go mod tidy` / `go work sync` inserts one, remove it before commit (FR-3).
- **Builder image tag is `golang:1.27-alpine`** — the floating minor tag, not `1.27.0-alpine` (design OQ-3).
- **Runtime image stays `alpine:3.24`.** No Dockerfile runtime-stage edit (design §6).
- **CI `go-version` is exactly `'1.27'`** (single-quoted string) at all three sites. Comments adjacent to those lines must remain **byte-identical** (FR-9).
- **`GOLANGCI_LINT_VERSION=v2.13.1`** in `tools/lint.versions`.
- **Fix, never suppress.** Any new linter finding is fixed in the source. No `//nolint`, no `.golangci.yml` exclusion, no narrowing of the `standard` group, no rev-gating (FR-12, FR-13).
- **No Go 1.27 language or stdlib feature adoption.** Mechanical boundary, checked in Task 7: outside the three gofumpt files, `git diff main -- '*.go'` must be empty; inside them, `git diff -w main -- <those three files>` must be empty (design §5).
- **No dependency version changes.** Only checksum bookkeeping may appear in `go.sum` / `go.work.sum` (FR-4).
- **`apps/web/Dockerfile`, everything under `deploy/k8s`, and everything under `docs/tasks/` stay unmodified** (FR-8, FR-17, design §8). The `1.25`/`1.26` strings in ten `docs/tasks/task-0NN-*/` files are historical records and are deliberately left stale.
- **Commit order is mandatory.** Task 1 (lint pin) must be the first commit on the branch. Never land a CI `go-version` bump before the lint pin — that produces a commit where CI is red by construction (design §3, ordering option C, explicitly forbidden).

## Environment notes

- Work in `/home/tumidanski/source/MyFleet/.worktrees/task-029-go-127-migration` on branch `task-029-go-127-migration`. Never edit the main checkout.
- The local toolchain is already `go1.27.0` (`go version` confirms). The main repo checkout shows a dirty `go.work.sum`; that is pre-existing noise outside this worktree and is not yours to clean (design §4).
- Node is not always on `PATH`. Before any `make ci` / `make fe-*` target:
  ```sh
  export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
  ```

---

### Task 1: Raise the golangci-lint pin and absorb its gofumpt findings

The linter pin is a **prerequisite** of the toolchain bump, not a companion to it. `v2.12.2` fails all six modules under a local `go1.27.0` (`packages/dto-go` reports `internal/poll/splice_linux.go:237:21: unknown field rfd in struct literal of type splicePipe (typecheck)`; the larger modules panic inside `goanalysis/runner_loadingpackage.go`). This commit lands `v2.13.1` and its three whitespace fixes while the `go` directives are still `1.25.0`, so it is independently green on both the old and new toolchain.

**Files:**
- Modify: `tools/lint.versions:5`
- Modify: `apps/fleet-service/internal/fuel/builder.go` (whitespace only, around line 19–25)
- Modify: `apps/fleet-service/internal/maintenancerecord/builder.go` (whitespace only, around line 19–27)
- Modify: `apps/media-service/internal/processing/worker_test.go` (whitespace only, around line 39–40)
- Create (throwaway, not committed): `/tmp/task029-kustomize-local-before.yaml`, `/tmp/task029-kustomize-main-before.yaml`

**Interfaces:**
- Consumes: nothing.
- Produces: a working tree where `tools/lint.sh --check --go` passes tree-wide under `golangci-lint v2.13.1`. Task 7 depends on this remaining true after the directive bump. Also produces the two pre-change kustomize baseline files that Task 9 diffs against.

- [ ] **Step 1: Confirm a clean start and capture the manifest baseline**

The kustomize baseline must be captured *before* any file changes so Task 9 can prove the renders are byte-identical.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-029-go-127-migration
git status --short          # expected: no output
git branch --show-current   # expected: task-029-go-127-migration
go version                  # expected: go version go1.27.0 linux/amd64
kustomize build deploy/k8s/overlays/local > /tmp/task029-kustomize-local-before.yaml
kustomize build deploy/k8s/overlays/main  > /tmp/task029-kustomize-main-before.yaml
wc -l /tmp/task029-kustomize-*-before.yaml
```

Expected: `git status --short` prints nothing, and both baseline files are non-empty. If the worktree is dirty, stop and report — do not stash someone else's work.

- [ ] **Step 2: Reproduce the failure that motivates this task**

Run the pinned linter under the current pin so the "before" state is observed rather than assumed:

```bash
tools/lint.sh --check --go packages/dto-go
```

Expected: FAIL, with `unknown field rfd in struct literal of type splicePipe (typecheck)` pointing into the Go 1.27 stdlib's `internal/poll/splice_linux.go`. This is the failing "test" for this task.

- [ ] **Step 3: Bump the lint pin**

`tools/lint.versions` is a shell-sourced key=value file read by both local runs and CI. Change line 5 only; leave the four comment lines above it untouched.

```diff
-GOLANGCI_LINT_VERSION=v2.12.2
+GOLANGCI_LINT_VERSION=v2.13.1
```

- [ ] **Step 4: Verify the pin fixes type-checking and surfaces exactly three gofumpt findings**

```bash
tools/lint.sh --check --go
```

Expected: FAIL, but for a completely different reason than Step 2 — no typecheck errors, no panics, exactly 3 `gofumpt` findings across 6 modules:

| Module | Expected findings |
|---|---|
| `apps/auth-service` | 0 |
| `apps/fleet-service` | 2 — `internal/fuel/builder.go:19`, `internal/maintenancerecord/builder.go:26` |
| `apps/media-service` | 1 — `internal/processing/worker_test.go:39` |
| `apps/notification-service` | 0 |
| `packages/dto-go` | 0 |
| `packages/shared-go` | 0 |

**If you see any finding from an analyzer (staticcheck, govet, errcheck, …) rather than `gofumpt`, stop and report.** The probe measured zero analyzer findings; an analyzer finding means the newer vendored `honnef.co/go/tools v0.8.0` caught something real, which is a scope question for the user (FR-12 forbids suppressing it) — not something to silently fix or ignore.

- [ ] **Step 5: Apply the formatter fixes automatically**

Do **not** hand-edit these. The rule at work is a newer-gofumpt rule about runs of single-line function bodies whose alignment column differs from the declaration that follows; let the formatter decide the exact bytes.

```bash
tools/lint.sh --go
```

For orientation, this is the shape of the change in `apps/fleet-service/internal/fuel/builder.go` — a blank line inserted plus the re-alignment that follows from it:

```diff
-func (b *Builder) SetPricePerGallon(price float64) *Builder  { b.m.pricePerGallon = price; return b }
+func (b *Builder) SetPricePerGallon(price float64) *Builder { b.m.pricePerGallon = price; return b }
+
 func (b *Builder) SetCreatedByUserID(userID string) *Builder { b.m.createdByUserID = userID; return b }
```

**Anti-goal, recorded so the implementation does not drift into it:** the alignment churn in these builder files is tempting bait for "while we're here, let's restructure these builders." Do not. Whitespace, nothing else.

- [ ] **Step 6: Verify the fixes are whitespace-only**

```bash
git diff --stat
git diff -w -- apps/fleet-service/internal/fuel/builder.go \
               apps/fleet-service/internal/maintenancerecord/builder.go \
               apps/media-service/internal/processing/worker_test.go
```

Expected: `--stat` lists exactly those three `.go` files plus `tools/lint.versions`, and the whitespace-ignoring diff prints **nothing**. Any non-empty `-w` diff means an identifier, expression, or statement changed — revert and re-run fix mode.

- [ ] **Step 7: Verify lint is clean tree-wide**

```bash
tools/lint.sh --check --go
```

Expected: `lint.sh: OK` and `0 issues.` per module, all six.

- [ ] **Step 8: Verify the code still compiles and tests pass**

```bash
make vet && make test
```

Expected: both pass. These run under the local `go1.27.0` against the still-`1.25.0` directives — the probe captured this green, so a failure here is not caused by this task's edits and must be root-caused before proceeding.

- [ ] **Step 9: Commit**

```bash
git add tools/lint.versions \
        apps/fleet-service/internal/fuel/builder.go \
        apps/fleet-service/internal/maintenancerecord/builder.go \
        apps/media-service/internal/processing/worker_test.go
git commit -m "chore(lint): pin golangci-lint v2.13.1 for Go 1.27 type-checking

v2.12.2's vendored go/types cannot parse Go 1.27 stdlib source and fails
all six modules under a local go1.27.0 toolchain. v2.13.1 type-checks
cleanly; its newer gofumpt wants a blank line between runs of single-line
function bodies with differing alignment columns, which accounts for the
three whitespace-only source edits."
```

---

### Task 2: Bump the seven Go directives and tidy

The directive change is the formality, not the hazard — the probe confirmed the workspace already tests green under `go1.27.0` with the tree unchanged. What needs care is that `make tidy` produces *only* checksum bookkeeping.

**Files:**
- Modify: `go.work:1`
- Modify: `apps/auth-service/go.mod:3`, `apps/fleet-service/go.mod:3`, `apps/media-service/go.mod:3`, `apps/notification-service/go.mod:3`
- Modify: `packages/dto-go/go.mod:3`, `packages/shared-go/go.mod:3`
- Modify (tooling-generated): `go.work.sum`, and per-module `go.sum` as `make tidy` dictates

**Interfaces:**
- Consumes: Task 1's lint pin — without it, Step 5 of this task fails on every module.
- Produces: a workspace whose minimum toolchain is Go 1.27.0. Task 8's container builds depend on this: a builder image older than 1.27 now fails loudly, which is what makes a Dockerfile tag typo detectable.

- [ ] **Step 1: Change all seven directives**

Every one of the seven files currently has the identical line `go 1.25.0`. This is the one place in this plan where a repository-wide mechanical sweep via shell is the right tool:

```bash
sed -i 's/^go 1\.25\.0$/go 1.27.0/' go.work apps/*/go.mod packages/*/go.mod
```

- [ ] **Step 2: Verify all seven changed and nothing else did**

```bash
grep -H '^go ' go.work apps/*/go.mod packages/*/go.mod
git diff --stat
```

Expected: seven lines, each `go 1.27.0`, from `go.work`, the four `apps/*/go.mod`, and the two `packages/*/go.mod`. `--stat` shows exactly seven files, one insertion and one deletion each.

- [ ] **Step 3: Tidy**

```bash
make tidy
```

- [ ] **Step 4: Verify no `toolchain` directive was inserted (FR-3)**

```bash
grep -n '^toolchain' go.work apps/*/go.mod packages/*/go.mod
```

Expected: no output (grep exits 1). If a `toolchain` line appeared, delete it and re-run this check. A `toolchain` line would only add a second version string that could drift from the `go` directive, which the PRD's non-goals explicitly reject in favour of one authoritative surface. (Omitting it does not block older toolchains — under the default `GOTOOLCHAIN=auto` the `go` directive alone triggers a silent auto-download; a hard `requires go >= 1.27.0` error only occurs under `GOTOOLCHAIN=local`, which this project does not set.)

- [ ] **Step 5: Verify the checksum churn is bookkeeping only (FR-4)**

The probe measured this precisely: **+12 lines, all additions, all `/go.mod` hash lines** for modules already in the graph (`github.com/alecthomas/units`, `github.com/creack/pty`, `github.com/google/gofuzz`, `github.com/modern-go/concurrent`, and similar). Make the check mechanical rather than eyeball-based:

```bash
# Any removed line at all is a red flag.
git diff -- go.work.sum '**/go.sum' | grep '^-[^-]' || echo "OK: no removed checksum lines"

# Every added line should be a /go.mod hash, not an h1: module-content hash.
git diff -- go.work.sum '**/go.sum' | grep '^+[^+]' | grep -v '/go.mod ' || echo "OK: all additions are /go.mod hashes"

# Dependency versions must not move.
git diff -- '**/go.mod' | grep -E '^[+-]\s+[a-z0-9.]+\.[a-z]+/' || echo "OK: no require-block changes"
```

Expected: all three print their `OK:` message. Anything else means something other than bookkeeping happened — root-cause it, do not commit through it.

- [ ] **Step 6: Verify the workspace still builds, vets, tests, and lints**

```bash
make vet && make build && make test && make lint-check
```

Expected: all four pass. `make test` is `go test -race` across the workspace; the probe captured it green on 1.27 pre-change, so any failure now is attributable to the directive bump and must be root-caused, not retried.

- [ ] **Step 7: Commit**

```bash
git add go.work go.work.sum apps/*/go.mod apps/*/go.sum packages/*/go.mod packages/*/go.sum
git commit -m "chore(go): declare go 1.27.0 across the workspace

Seven directives move from 1.25.0 to 1.27.0. No toolchain directive is
added, so a contributor on older Go gets an explicit version error rather
than a silent auto-download. go.work.sum churn is additive /go.mod hash
bookkeeping only; no dependency version changes."
```

---

### Task 3: Move the four service builder images to `golang:1.27-alpine`

**Files:**
- Modify: `apps/auth-service/Dockerfile:1`, `apps/fleet-service/Dockerfile:1`, `apps/media-service/Dockerfile:1`, `apps/notification-service/Dockerfile:1`

**Interfaces:**
- Consumes: nothing from prior tasks (the tag change is independent), but lands after Task 2 per design §3's forced ordering.
- Produces: the images Task 8 builds.

- [ ] **Step 1: Change line 1 of all four service Dockerfiles**

All four are byte-identical on line 1 today (`FROM golang:1.26-alpine AS build`):

```bash
sed -i 's|^FROM golang:1\.26-alpine AS build$|FROM golang:1.27-alpine AS build|' \
  apps/auth-service/Dockerfile apps/fleet-service/Dockerfile \
  apps/media-service/Dockerfile apps/notification-service/Dockerfile
```

Note the tag is the **floating minor** `1.27-alpine`, not `1.27.0-alpine`. That is a deliberate decision (design OQ-3): it picks up Go patch releases on the next image rebuild without routing every security rollup through a Renovate PR gated behind `minimumReleaseAge: 7 days`.

- [ ] **Step 2: Verify the builder changed and the runtime did not (FR-5, FR-6)**

```bash
grep -n '^FROM' apps/auth-service/Dockerfile apps/fleet-service/Dockerfile \
                apps/media-service/Dockerfile apps/notification-service/Dockerfile
git status --short apps/web/Dockerfile
```

Expected: each service shows `FROM golang:1.27-alpine AS build` on line 1 and `FROM alpine:3.24` on line 31. `alpine:3.24` is correct and stays — 3.24 is the current series, the floating tag already tracks the latest patch, and `golang:1.27-alpine` resolves to the `alpine3.24` variant, so builder and runtime stay on the same Alpine series. `git status --short apps/web/Dockerfile` must print nothing.

- [ ] **Step 3: Commit**

```bash
git add apps/auth-service/Dockerfile apps/fleet-service/Dockerfile \
        apps/media-service/Dockerfile apps/notification-service/Dockerfile
git commit -m "chore(docker): build services on golang:1.27-alpine

Keeps the floating minor tag so Go patch releases land on the next image
rebuild. 1.27-alpine resolves to alpine3.24, matching the runtime stage."
```

---

### Task 4: Move all three CI `setup-go` inputs to `'1.27'`

The comments around these lines document hard-won cache-race findings (`main.yml` cites an observed run ID). They must survive byte-identical.

**Files:**
- Modify: `.github/workflows/pr.yml:15`, `.github/workflows/pr.yml:50`
- Modify: `.github/workflows/main.yml:44`

**Interfaces:**
- Consumes: Task 1's lint pin. This is the ordering constraint that matters most — CI on 1.27 with `golangci-lint v2.12.2` is red by construction.
- Produces: nothing later tasks read.

- [ ] **Step 1: Change the three `go-version` inputs**

All three lines are identical (`          go-version: '1.26'`), and `1.26` appears nowhere else in either file:

```bash
sed -i "s/^\( *\)go-version: '1\.26'$/\1go-version: '1.27'/" \
  .github/workflows/pr.yml .github/workflows/main.yml
```

- [ ] **Step 2: Verify all three sites and only those lines changed (FR-9)**

```bash
grep -rn "go-version" .github/workflows/
git diff -- .github/workflows/
```

Expected from `grep`: exactly three `go-version: '1.27'` hits — `pr.yml:15`, `pr.yml:50`, `main.yml:44` — plus one prose occurrence at `main.yml:46` (`its key from OS/arch/go-version/file-hash only`) which is a comment and must be untouched. No `'1.26'` anywhere. Expected from `git diff`: three changed lines total, no comment lines in the diff.

- [ ] **Step 3: Confirm no cache key needs busting (FR-10, design OQ-4)**

This is a verification, not an edit. Read the keys:

```bash
grep -n 'key:\|restore-keys:' .github/workflows/pr.yml .github/workflows/main.yml
```

Expected: `go-build-*` and `go-lint-*` key on `hashFiles('**/go.sum', 'go.work.sum')`; `lint-tools-*` keys on `hashFiles('tools/lint.versions')`. **None embeds the Go version, and none should be changed.** Two things happen for free: the `go.work.sum` churn from Task 2 busts the two build caches once (the expected one-run slowdown), and Task 1's `tools/lint.versions` edit self-busts the lint-tooling cache so no stale `golangci-lint-v2.12.2` binary survives. Restoring 1.26 build objects into a 1.27 job is not a correctness hazard — Go's build cache is content-addressed on a key that includes the toolchain build ID, so stale entries are inert misses, and `GOMODCACHE` is toolchain-independent.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/pr.yml .github/workflows/main.yml
git commit -m "ci: run Go jobs on 1.27

Lands after the golangci-lint pin bump: v2.12.2 cannot type-check the 1.27
stdlib, so the reverse order would produce a commit where CI is red by
construction. Cache keys are unchanged by design — none embeds the Go
version, and go.work.sum/lint.versions churn busts what needs busting."
```

---

### Task 5: Correct every stale Go-version claim in documentation

Three files. `architecture-overview.md` is doubly wrong — it claims Go 1.25 *and* cites CI as `'1.25'` when CI was already 1.26, so both the version and the citation need correcting. `scaffolding-checklist.md` names a runtime base (`alpine:3.20`) that no service has used for some time.

**Files:**
- Modify: `README.md:87`
- Modify: `.claude/skills/backend-dev-guidelines/resources/architecture-overview.md:18-19`
- Modify: `.claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md:70,73`

**Interfaces:**
- Consumes: the post-change tree, since the skill's file:line citations must be re-verified against it (FR-15).
- Produces: nothing later tasks read.

- [ ] **Step 1: Update the README prerequisite (FR-14)**

`README.md:87`. This line is not cosmetic — because no `toolchain` directive is added, a contributor on older Go isn't hard-blocked at all; under the default `GOTOOLCHAIN=auto` the `go` directive alone triggers a silent download of 1.27. This README line is what makes the new floor discoverable without relying on the build to enforce it.

```diff
-- Go 1.25+
+- Go 1.27+
```

- [ ] **Step 2: Update the architecture overview (FR-15)**

`.claude/skills/backend-dev-guidelines/resources/architecture-overview.md:18-19`:

```diff
-- Go 1.25 (`go.work:1` declares `go 1.25.0`; CI pins `go-version: '1.25'`
+- Go 1.27 (`go.work:1` declares `go 1.27.0`; CI pins `go-version: '1.27'`
   at `.github/workflows/pr.yml:15,50`)
```

- [ ] **Step 3: Re-verify the cited file:line references (FR-15)**

The citations must be true of the post-change tree, not merely plausible:

```bash
sed -n '1p' go.work
sed -n '15p;50p' .github/workflows/pr.yml
```

Expected: `go 1.27.0`, and both workflow lines reading `          go-version: '1.27'`. If a line number has shifted, update the citation to the real number rather than leaving a stale one.

- [ ] **Step 4: Update the scaffolding checklist (FR-16)**

`.claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md`, lines 70 and 73. Both values are wrong today; the builder is two minors stale and the runtime base is a whole series stale:

```diff
-- Builder stage: `FROM golang:1.25-alpine AS build`; copy `go.work`,
+- Builder stage: `FROM golang:1.27-alpine AS build`; copy `go.work`,
```

```diff
-- Runtime stage: `FROM alpine:3.20`; create and switch to a non-root user
+- Runtime stage: `FROM alpine:3.24`; create and switch to a non-root user
```

- [ ] **Step 5: Sweep for any remaining version claim outside the archives (FR-17)**

```bash
grep -rn '1\.25\|1\.26' --include='*.md' . | grep -v node_modules
```

Expected: **hits only under `docs/tasks/task-0NN-*/`** — roughly ten files of plans, contexts, and audits from tasks 001–020. Those are records of what was true when that work happened; rewriting them would falsify the record and mislead future archaeology. Leave them. A hit anywhere else is a miss in this task and must be fixed here. (Ignore incidental matches that are not Go versions, e.g. an unrelated `1.25` in a numeric table — read each hit rather than pattern-matching blindly.)

- [ ] **Step 6: Commit**

```bash
git add README.md .claude/skills/backend-dev-guidelines/resources/architecture-overview.md \
        .claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md
git commit -m "docs: state Go 1.27 and the actual container bases

architecture-overview.md cited CI as '1.25' when CI was already 1.26, so
both the version and the citation are corrected against the post-change
tree. scaffolding-checklist.md's alpine:3.20 runtime base has been wrong
independently of this migration; services run alpine:3.24."
```

---

### Task 6: Teach Renovate to move the Go toolchain surfaces as one PR

This is the anti-drift mechanism, and the one place the PRD's phrasing ("gains a package rule") is insufficient. **`packageRules` alone cannot do this.** Renovate's `github-actions` manager parses `uses:` references and `container:`/`services:` images — it does *not* parse arbitrary action inputs, so `go-version: '1.27'` is invisible to it, and a `packageRules` entry cannot group a dependency Renovate never extracted. Making it visible requires a `customManagers` regex manager, and because this repo sets an explicit `enabledManagers` allowlist, custom managers are **disabled** unless `"custom.regex"` is added to it. Omitting that is a silent no-op: the config validates, Renovate runs, and nothing is ever grouped.

**Files:**
- Modify: `renovate.json` — `enabledManagers` (lines 10–15), a new top-level `customManagers` array, one appended `packageRules` entry

**Interfaces:**
- Consumes: nothing.
- Produces: nothing later tasks read. Behavioural confirmation lands post-merge (Step 5).

- [ ] **Step 1: Add `custom.regex` to `enabledManagers`**

Carry this as its own step rather than folding it into "update renovate.json" — it is the single easiest thing to forget and the failure mode is silent.

```diff
   "enabledManagers": [
     "gomod",
     "npm",
     "dockerfile",
-    "github-actions"
+    "github-actions",
+    "custom.regex"
   ],
```

- [ ] **Step 2: Add the `customManagers` array**

Insert as a new top-level key. Place it immediately after the closing `]` of `enabledManagers` (i.e. between `enabledManagers` and `packageRules`):

```json
  "customManagers": [
    {
      "customType": "regex",
      "description": "actions/setup-go's go-version input is a plain YAML value the github-actions manager does not extract.",
      "managerFilePatterns": ["/^\\.github/workflows/[^/]+\\.ya?ml$/"],
      "matchStrings": ["go-version:\\s*['\"](?<currentValue>[0-9.]+)['\"]"],
      "datasourceTemplate": "golang-version",
      "depNameTemplate": "go",
      "versioningTemplate": "docker"
    }
  ],
```

Two details that are load-bearing:

- **`managerFilePatterns`, not `fileMatch`.** `fileMatch` is the deprecated spelling; current Renovate wants `managerFilePatterns`.
- **The regex is anchored on `go-version:` followed by a quoted value.** The literal string `go-version` also appears in prose at `.github/workflows/main.yml:46` ("its key from OS/arch/go-version/file-hash only"). That occurrence has no `:`-then-quote following it, so it does not match — Step 4 asserts this rather than trusting it.

- [ ] **Step 3: Append the grouping rule**

Append as the **last** entry of the `packageRules` array — after the in-repo-module exclusion rule — so it wins Renovate's last-rule-wins merge:

```json
    {
      "description": "Go toolchain surfaces — builder image, CI input, and the go.mod directive — move as one PR so they cannot drift apart the way 1.25/1.26 did.",
      "matchDatasources": ["golang-version", "docker"],
      "matchPackageNames": ["go", "golang"],
      "groupName": "Go toolchain"
    }
```

Notes: the rule sets **no `automerge` and no `minimumReleaseAge` override**, so both inherit from the top-level config and major Go releases continue to require review (FR-18). `matchPackageNames` includes `go` as well as `golang` because Renovate's `gomod` manager *does* extract the `go` directive as a `golang-version` dependency named `go` — so the same rule sweeps the `go.work`/`go.mod` directive into the group for free, a third surface at no extra cost. The existing "Separate major upgrades" rule sets only `automerge`/`labels` and no `groupName`, so it composes with this rule rather than fighting it.

- [ ] **Step 4: Verify the regex matches what it should and nothing it shouldn't**

```bash
grep -nP "go-version:\s*['\"][0-9.]+['\"]" .github/workflows/*.yml
grep -n 'go-version' .github/workflows/main.yml
```

Expected: the first command matches exactly three lines (`pr.yml:15`, `pr.yml:50`, `main.yml:44`), all showing `1.27`. The second shows those plus the prose comment at `main.yml:46` — confirming the prose line is **not** in the first command's output.

- [ ] **Step 5: Validate the config and verify the existing rules survived (FR-19)**

```bash
npx --yes --package renovate renovate-config-validator --strict
python3 -m json.tool renovate.json > /dev/null && echo "OK: valid JSON"
grep -n 'allowedVersions\|myfleet/packages' renovate.json
```

Expected: the validator reports the config as valid; JSON parses; and the TypeScript `<7` ceiling and the in-repo module exclusion (`"enabled": false` for `github.com/jtumidanski/myfleet/packages/**`) are both present and unmodified.

**Understand what this validation does and does not prove.** The validator reports "Validating as global config", which is a looser schema check than repo-config validation — it proves *syntax*, not *behaviour*. The regex manager's real proof is the next Renovate run's Dependency Dashboard showing a `go` dependency at `1.27`. That confirmation lands **post-merge and must not block the PR**; note it in the PR description as a follow-up to eyeball.

- [ ] **Step 6: Commit**

```bash
git add renovate.json
git commit -m "chore(renovate): group the Go toolchain surfaces into one PR

setup-go's go-version input is invisible to the github-actions manager, so
a custom.regex manager extracts it — and custom.regex must be added to the
enabledManagers allowlist or the manager is a silent no-op. The grouping
rule sweeps in the golang builder image and the gomod go directive, so a
future 1.28 bump arrives as one PR instead of three that drift apart."
```

---

### Task 7: Full CI gate and scope-discipline audit

Layered verification, because the layers fail differently and a single `make ci` would not tell you which one broke.

**Files:** none modified. This task only runs commands.

**Interfaces:**
- Consumes: Tasks 1–6, all committed.
- Produces: the evidence the PR description cites.

- [ ] **Step 1: Run the full gate**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build` all pass. `make test` is `go test -race` across the workspace, and no test may be skipped or removed relative to the pre-change baseline.

- [ ] **Step 2: Audit scope discipline — no Go feature adoption crept in**

The compiler side of this migration was already de-risked (the tree tested green on `go1.27.0` before any change), so the *only* thing the directive bump changes is what the language permits — exactly the surface a well-meaning implementer starts exercising. Two commands, no judgement calls:

```bash
# Outside the three gofumpt files, no .go file may have changed at all.
git diff main --stat -- '*.go'

# Inside those three, the change must be whitespace-only.
git diff -w main -- apps/fleet-service/internal/fuel/builder.go \
                    apps/fleet-service/internal/maintenancerecord/builder.go \
                    apps/media-service/internal/processing/worker_test.go
```

Expected: the first lists **only** those three files; the second prints **nothing**.

- [ ] **Step 3: Run the acceptance-criteria greps**

```bash
grep -H '^go ' go.work apps/*/go.mod packages/*/go.mod            # all: go 1.27.0
grep -n '^toolchain' go.work apps/*/go.mod packages/*/go.mod      # no output
grep -n 'FROM golang' apps/*/Dockerfile                           # 4x golang:1.27-alpine
grep -rn "go-version" .github/workflows/                          # 3x '1.27' + 1 prose comment
grep -n 'GOLANGCI_LINT_VERSION' tools/lint.versions               # v2.13.1
grep -n 'Go 1\.27' README.md                                      # the prerequisite line
git diff main --stat -- apps/web deploy/k8s docs/tasks .golangci.yml  # no output
```

Expected: as annotated. The last command proving empty is what backs the PRD's "explicitly unmodified" list — `apps/web`, `deploy/k8s`, the historical task archives, and `.golangci.yml` (whose `standard` group membership did not change between v2.12.2 and v2.13.1, as the zero analyzer findings in Task 1 demonstrated).

---

### Task 8: Build all four container images

This is the only check that exercises `golang:1.27-alpine` as an actual image rather than a string. Because the `go` directive is now `1.27.0`, a builder image older than 1.27 fails loudly here — so a typo in the tag surfaces at this step and nowhere else.

**Files:** none modified.

**Interfaces:**
- Consumes: Tasks 2 and 3.
- Produces: image size figures for the PR description.

- [ ] **Step 1: Build each service image from the repo root**

The build context is the repo root for every service (CLAUDE.md), which is why `-f` is required:

```bash
for s in auth fleet media notification; do
  docker build -f apps/$s-service/Dockerfile -t myfleet-$s:task029 . || echo "FAILED: $s"
done
```

Expected: all four succeed, no `FAILED:` lines. A failure mentioning `go.mod requires go >= 1.27.0` means the Dockerfile tag did not actually change — recheck Task 3.

- [ ] **Step 2: Glance at image sizes**

```bash
docker images --format '{{.Repository}}:{{.Tag}}\t{{.Size}}' | grep task029
```

Expected: sizes in line with the pre-change images. A Go minor bump moves binary size by low single-digit percentages; anything beyond a few percent gets investigated before merge rather than waved through.

- [ ] **Step 3: Confirm the web image was not dragged in**

```bash
git diff main --stat -- apps/web/Dockerfile
```

Expected: no output. The Node toolchain (Node 24 in `apps/web/Dockerfile`, Node 22 in CI) is explicitly out of scope.

---

### Task 9: Verify the manifests render unchanged

No manifest carries a Go version, so both renders must come out byte-identical to the Task 1 baselines. CLAUDE.md records that skipping the *local* overlay's server dry-run hid a real failure (a missing `namespace:` producing `ClusterRoleBinding "myfleet-traefik" is invalid: subjects[0].namespace: Required value`) through ten reviews — so the local overlay is not exempt here either.

**Files:** none modified.

**Interfaces:**
- Consumes: the baseline files captured in Task 1 Step 1.
- Produces: the final green light before code review.

- [ ] **Step 1: Re-render and diff against the pre-change baseline**

```bash
kustomize build deploy/k8s/overlays/local > /tmp/task029-kustomize-local-after.yaml
kustomize build deploy/k8s/overlays/main  > /tmp/task029-kustomize-main-after.yaml
diff /tmp/task029-kustomize-local-before.yaml /tmp/task029-kustomize-local-after.yaml && echo "OK: local identical"
diff /tmp/task029-kustomize-main-before.yaml  /tmp/task029-kustomize-main-after.yaml  && echo "OK: main identical"
```

Expected: both `OK:` lines, no diff output.

- [ ] **Step 2: Re-assert the `main` overlay's invariants**

```bash
grep -c 'kind: PersistentVolumeClaim\|kind: Secret\|kind: ClusterRole' /tmp/task029-kustomize-main-after.yaml
grep -n 'REPLACE\|CHANGEME\|TODO\|placeholder' /tmp/task029-kustomize-main-after.yaml
```

Expected: `0` from the first, no output from the second. The `main` overlay must render with no PersistentVolumeClaims, no Secrets, no ClusterRole, and no placeholder values.

- [ ] **Step 3: Run both server dry-runs**

Rendering alone does not catch namespace or cross-resource-reference errors. `--dry-run=server` validates against the API server without persisting anything, so it is safe to point at the shared `bee` context; it needs the `traefik.io` CRDs, which bee has.

```bash
kubectl config current-context
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Expected: both report `(server dry run)` for every resource with no errors.

**If the cluster is unreachable**, do not silently skip this and do not claim it passed. Record it explicitly — in the PR description and to the user — as "server dry-runs not executed: cluster unreachable", and state that the render-diff in Step 1 (byte-identical to pre-change) is the evidence standing in for it. Since no manifest changed, the residual risk is genuinely nil, but the gap must be stated rather than papered over.

---

### Task 10: Code review, then open the PR

CLAUDE.md is explicit: always run the code-review step before opening a PR, and do not skip it even when the plan looks complete.

**Files:**
- Create: `docs/tasks/task-029-go-127-migration/audit.md` (written by the reviewer agents)

**Interfaces:**
- Consumes: Tasks 1–9.
- Produces: the audit and the PR.

- [ ] **Step 1: Request code review**

Invoke `superpowers:requesting-code-review`. It dispatches the appropriate subset of reviewers; for this branch expect `plan-adherence-reviewer` and `backend-guidelines-reviewer` (Go files changed — three whitespace-only edits). `frontend-guidelines-reviewer` is not applicable: no TypeScript or React file is touched.

Give the reviewers this branch's specific framing so they audit the right thing: the Go source changes are whitespace-only by design, and a finding of the form "these builder methods should be restructured" is out of scope for a version-bump PR.

- [ ] **Step 2: Address the findings**

Fix what the audit raises, or record an explicit justification for anything deliberately not changed. Re-run `make ci` after any code change.

- [ ] **Step 3: Open the PR**

The description must cover: the ordering rationale (lint pin first, because v2.12.2 cannot type-check 1.27 stdlib); the three whitespace-only `.go` edits, each individually justified as gofumpt fallout; the `go.work.sum` churn characterised as additive `/go.mod` bookkeeping with no dependency version changes; the Renovate grouping rule with its post-merge Dependency Dashboard confirmation flagged as a follow-up; and the results of Tasks 7–9, including any check that could not be run (e.g. server dry-runs if the cluster was unreachable) stated plainly rather than omitted.

---

## Rollback

Trivial and total: revert the PR. There is no migration, no data change, no manifest change, and no persisted state. The only artifacts are container images, and `deploy/k8s` pins by SHA tag, so a revert plus a rebuild fully restores the prior state.
