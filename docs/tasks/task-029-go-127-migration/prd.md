# Go 1.27 Toolchain Migration — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-25
---

## 1. Overview

Every Go surface in MyFleet is currently pinned to one of two different Go
versions, and neither is 1.27. The six modules in the workspace (`go.work` plus
`apps/{auth,fleet,media,notification}-service` and `packages/{dto-go,shared-go}`)
declare `go 1.25.0`. The four service Dockerfiles build on `golang:1.26-alpine`,
and all three `setup-go` steps in CI pin `go-version: '1.26'`. Documentation is
a third variant again: `README.md` claims "Go 1.25+", and the
`backend-dev-guidelines` skill asserts a Go version and a Dockerfile runtime
base that no longer match the tree. This split is not deliberate — it is drift
that accumulated because the language directive, the builder image, and the CI
input are four separate files that nothing keeps in sync.

This task collapses all of it onto Go 1.27: language directive, builder images,
CI inputs, and the documentation that describes them. It is a version-bump task,
not a feature-adoption task — no Go 1.27 language or standard-library additions
are swept into the codebase. The only code changes permitted are those required
to keep `make ci` green.

There is one hard prerequisite, empirically confirmed rather than assumed. The
pinned linter, `golangci-lint v2.12.2` (`tools/lint.versions`), cannot type-check
against the Go 1.27 standard library. Running `tools/lint.sh --check --go` under
a local `go1.27.0` toolchain fails all six modules; `packages/dto-go` reports
`internal/poll/splice_linux.go:237:21: unknown field rfd in struct literal of
type splicePipe (typecheck)` and the larger modules panic outright inside
`goanalysis/runner_loadingpackage.go`. CI does not see this today only because
CI is still on 1.26. The moment CI moves to 1.27, lint breaks. A probe of
`golangci-lint v2.13.1` against `packages/dto-go` with the repo's own
`.golangci.yml` reports `0 issues`, so the fix is a pin bump, and it must land
in the same change as the toolchain bump rather than after it.

## 2. Goals

Primary goals:

- Declare Go 1.27 in exactly one form across `go.work` and all six `go.mod`
  files (`go 1.27.0`), eliminating the 1.25-vs-1.26 language/toolchain split.
- Build all four service containers on `golang:1.27-alpine`.
- Run all three CI `setup-go` steps on `go-version: '1.27'`.
- Raise `GOLANGCI_LINT_VERSION` to a release that type-checks Go 1.27, so
  `make lint-check` passes under the new toolchain.
- Correct every stale Go-version claim in `README.md` and the
  `backend-dev-guidelines` skill so the documented version matches the tree.
- Keep `make ci` green and both kustomize overlays rendering unchanged.

Non-goals:

- Adopting new Go 1.27 language or standard-library features anywhere in the
  codebase. A follow-up task may do this; this one deliberately does not.
- Upgrading third-party Go dependencies beyond whatever `go mod tidy` rewrites
  as a mechanical consequence of the directive change.
- Adding a `toolchain` line to `go.work` or any `go.mod`. The `go` directive
  remains the floor; CI and the Dockerfiles pin the concrete version.
- Any change to the web toolchain (Node 24 in `apps/web/Dockerfile`, Node 22 in
  CI), the frontend dependency tree, or `apps/web` in general.
- Behavioural changes to any service. No API, schema, or manifest changes.

## 3. User Stories

- As a MyFleet developer, I want `make ci` to pass on my machine with the Go
  toolchain I actually have installed (`go1.27.0`), so that local verification
  and CI agree instead of silently diverging.
- As a MyFleet developer, I want one authoritative Go version declared in the
  repo, so that I do not have to reconcile `go.work`, a Dockerfile, and a
  workflow input to answer "what Go are we on?".
- As a developer scaffolding a new service, I want the `backend-dev-guidelines`
  skill's Dockerfile template to state the version and base image the existing
  services actually use, so that a new service does not start life on a stale
  builder.
- As a reviewer, I want the golangci-lint pin to move in lockstep with the Go
  toolchain, so that a green CI run genuinely means the linter type-checked the
  code rather than failing open.
- As the repo maintainer, I want Renovate to surface future Go toolchain bumps
  as one coherent change, so that the builder image and the CI input cannot
  drift apart again the way they did between 1.25 and 1.26.

## 4. Functional Requirements

### 4.1 Module version declarations

- FR-1: `go.work:1` declares `go 1.27.0`.
- FR-2: Each of the six module `go.mod` files declares `go 1.27.0`:
  `apps/auth-service`, `apps/fleet-service`, `apps/media-service`,
  `apps/notification-service`, `packages/dto-go`, `packages/shared-go`.
- FR-3: No `toolchain` directive is added to `go.work` or any `go.mod`. If
  `go mod tidy` or `go work sync` inserts one, it is removed before commit.
- FR-4: `make tidy` is run after the directive change, and any resulting
  `go.sum` / `go.work.sum` churn is committed as part of the same change.
  Dependency *versions* must not change; only checksum-file bookkeeping may.

### 4.2 Container builds

- FR-5: All four service Dockerfiles use `FROM golang:1.27-alpine AS build` on
  line 1. The `1.27-alpine` tag exists and resolves to `alpine3.24`, matching
  the runtime stage.
- FR-6: The runtime stage stays `FROM alpine:3.24`. Alpine 3.24 is the current
  series and the floating `3.24` tag already tracks the latest patch, so this
  scope item resolves to a verification with no edit. If verification shows a
  newer series has shipped, the bump is in scope; otherwise no change.
- FR-7: Each service image builds from the repo root:
  `docker build -f apps/<service>/Dockerfile .` succeeds for all four services.
- FR-8: `apps/web/Dockerfile` is not modified.

### 4.3 CI

- FR-9: All three `setup-go` steps use `go-version: '1.27'` —
  `.github/workflows/pr.yml:15`, `.github/workflows/pr.yml:50`, and
  `.github/workflows/main.yml:44`. Surrounding comments about cache-key
  namespacing are preserved verbatim.
- FR-10: The explicit Go build/module cache steps continue to work. Cache keys
  that embed the Go version will miss once on the first post-bump run; this is
  expected and requires no key change.

### 4.4 Lint toolchain

- FR-11: `tools/lint.versions` sets `GOLANGCI_LINT_VERSION=v2.13.1` (or a later
  release verified to type-check Go 1.27).
- FR-12: `tools/lint.sh --check` passes tree-wide with zero findings, for both
  the Go and web layers. The repo's zero-finding baseline is preserved — any
  new finding introduced by the newer linter is fixed, not suppressed, and not
  rev-gated, per the policy stated in `tools/lint.sh`.
- FR-13: `.golangci.yml` keeps `default: standard` and the existing `gofumpt` +
  `goimports` formatter configuration. If v2.13.1 changes the membership of the
  `standard` group, the resulting findings are fixed rather than the group being
  narrowed.

### 4.5 Documentation and skill accuracy

- FR-14: `README.md:87` reads `Go 1.27+` instead of `Go 1.25+`.
- FR-15: `.claude/skills/backend-dev-guidelines/resources/architecture-overview.md:18-19`
  states Go 1.27, with its file:line citations (`go.work:1`,
  `.github/workflows/pr.yml:15,50`) re-verified against the post-change tree.
- FR-16: `.claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md:70`
  states `FROM golang:1.27-alpine AS build`, and line 73's runtime base is
  corrected from the stale `alpine:3.20` to the actual `alpine:3.24`.
- FR-17: A tree-wide sweep confirms no other non-task-archive file asserts a Go
  version. Files under `docs/tasks/task-0NN-*/` are historical records of past
  work and are explicitly **not** updated.

### 4.6 Renovate

- FR-18: `renovate.json` gains a package rule that groups the Go toolchain
  surfaces — the `golang` Docker image and the `setup-go` `go-version` input —
  into a single named group, so a future 1.28 bump arrives as one PR touching
  both rather than as separate `dockerfile` and `github-actions` PRs that can
  land weeks apart. The rule must not disable `minimumReleaseAge` or enable
  automerge; major Go releases continue to require review.
- FR-19: The existing rules (TypeScript `<7` ceiling, in-repo module exclusion,
  the four grouping rules) are left intact.

## 5. API Surface

None. This task changes no HTTP endpoint, request shape, response shape, error
code, or event schema. Any diff touching a handler, route, DTO, or event type is
out of scope and indicates the task has drifted.

## 6. Data Model

None. No entities, fields, relationships, constraints, or migrations change. No
GORM model or `AutoMigrate` call is touched.

## 7. Service Impact

| Surface | Change |
|---|---|
| `go.work` | `go 1.25.0` → `go 1.27.0`; `go.work.sum` churn from `make tidy` |
| `apps/auth-service` | `go.mod` directive; `Dockerfile:1` builder image |
| `apps/fleet-service` | `go.mod` directive; `Dockerfile:1` builder image |
| `apps/media-service` | `go.mod` directive; `Dockerfile:1` builder image |
| `apps/notification-service` | `go.mod` directive; `Dockerfile:1` builder image |
| `packages/dto-go` | `go.mod` directive |
| `packages/shared-go` | `go.mod` directive |
| `apps/web` | **No change** |
| `.github/workflows/pr.yml` | two `go-version` inputs |
| `.github/workflows/main.yml` | one `go-version` input |
| `tools/lint.versions` | `GOLANGCI_LINT_VERSION` bump |
| `README.md` | prerequisite version line |
| `.claude/skills/backend-dev-guidelines/` | two resource files |
| `renovate.json` | one new package rule |
| `deploy/k8s` | **No change** — no manifest references a Go version |

All four services are affected identically and none is a special case. No
service-to-service contract, no shared-package API, and no
`packages/shared-go/server` transport behaviour changes.

## 8. Non-Functional Requirements

**Correctness.** `go test -race` across the whole workspace must pass on 1.27.
The baseline was captured green on this branch before any change, so any failure
after the bump is attributable to the bump and must be root-caused, not retried.

**Build reproducibility.** The `go 1.27.0` directive raises the minimum toolchain
for anyone building the repo. Because no `toolchain` line is added, a contributor
on an older Go gets a clear "requires go >= 1.27.0" error rather than a silent
auto-download. This is the intended trade-off and must be reflected in the
README prerequisite.

**Container size and startup.** Image size and service startup time should not
regress meaningfully. A Go minor bump can shift binary size by a small
percentage; anything beyond that warrants investigation before merge.

**Observability.** No change to logging, metrics, or tracing. The `telemetry`,
`health`, and `jobs` packages in `shared-go` are compiled but not modified.

**Security.** Moving to the current Go release and a current golangci-lint is a
net security improvement: the 1.26 builder stops receiving fixes once 1.28
ships. `osvVulnerabilityAlerts` in `renovate.json` stays enabled. No new
dependency is introduced.

**CI duration.** The first post-bump run rebuilds every Go cache from cold. A
one-run slowdown is acceptable; a persistent one indicates a broken cache key.

## 9. Open Questions

1. **Does golangci-lint v2.13.1 stay clean across all six modules, not just
   `packages/dto-go`?** The pre-spec probe ran only `packages/dto-go` (0 issues).
   The larger modules — `packages/shared-go` and the four services — carry far
   more code and are where a newer `staticcheck` (v2.13.1 vendors
   `honnef.co/go/tools v0.8.0`) is most likely to surface new findings. The
   design phase must run the full `tools/lint.sh --check --go` under v2.13.1 and
   quantify the fix list before planning. If it is large, FR-12's "fix, don't
   suppress" policy may need an explicit carve-out decision from the user.

2. **Should a `.go-version` / `mise` / `asdf` file be introduced?** The repo has
   none today, which is why local toolchains (`go1.27.0`) and CI (`1.26`) could
   diverge unnoticed. Adding one would give editors and version managers a
   single source of truth, but it is a new mechanism and arguably belongs in its
   own task. Currently assumed **out of scope**.

3. **Is `1.27-alpine` or `1.27.0-alpine` the right builder tag?** The repo's
   existing convention is the floating minor tag (`1.26-alpine`), which picks up
   patch releases automatically. Continuing that convention is assumed, but it
   means Renovate only sees minor bumps, not patch ones. Keeping the existing
   convention is the default unless the design phase argues otherwise.

4. **Does the CI Go cache key need a manual bust?** `main.yml` documents a
   hand-rolled cache key; whether it embeds the Go version needs to be confirmed
   during design. If it does not, stale 1.26 build objects could be restored
   into a 1.27 job.

5. **Exact shape of the Renovate grouping rule (FR-18).** Grouping a
   `dockerfile` `golang` image with a `github-actions` `go-version` input spans
   two managers, and `go-version` in `setup-go` is not reliably detected by the
   `github-actions` manager without a `customManager` regex. The design phase
   must confirm which mechanism Renovate actually needs here rather than
   assuming a `packageRules` entry suffices.

## 10. Acceptance Criteria

Version declarations:

- [ ] `grep '^go ' go.work apps/*/go.mod packages/*/go.mod` returns `go 1.27.0`
      for all seven files.
- [ ] `grep -r '^toolchain' go.work apps/*/go.mod packages/*/go.mod` returns
      nothing.
- [ ] `git diff` on `go.sum`/`go.work.sum` shows no dependency *version*
      changes, only checksum bookkeeping.

Builds and tests:

- [ ] `make vet` passes.
- [ ] `make test` passes (`go test -race` across the workspace), with no test
      skipped or removed relative to the pre-change baseline.
- [ ] `make build` passes.
- [ ] `make lint-check` passes with zero findings, tree-wide.
- [ ] `make fe-test` and `make fe-build` pass, unchanged.
- [ ] `make ci` passes end to end.

Containers:

- [ ] `grep -n 'FROM golang' apps/*/Dockerfile` shows `golang:1.27-alpine` for
      all four services.
- [ ] `docker build -f apps/<service>/Dockerfile .` succeeds from the repo root
      for auth, fleet, media, and notification.
- [ ] `apps/web/Dockerfile` is unmodified in the diff.

CI:

- [ ] `grep -rn "go-version" .github/workflows/` shows `'1.27'` at all three
      sites and nothing else.
- [ ] Comments adjacent to the changed lines are byte-identical to before.

Lint pin:

- [ ] `tools/lint.versions` pins `v2.13.1` or later.
- [ ] The new linter version was actually exercised locally against all six
      modules, not just assumed compatible.

Manifests:

- [ ] `kustomize build deploy/k8s/overlays/local` renders identically to the
      pre-change output.
- [ ] `kustomize build deploy/k8s/overlays/main` renders identically, still with
      no PersistentVolumeClaims, no Secrets, no ClusterRole, and no placeholder
      values.
- [ ] Both `kubectl apply --dry-run=server` invocations pass against a reachable
      cluster.

Documentation:

- [ ] `README.md` states `Go 1.27+`.
- [ ] `architecture-overview.md` states Go 1.27 with re-verified file:line
      citations.
- [ ] `scaffolding-checklist.md` states `golang:1.27-alpine` and `alpine:3.24`.
- [ ] `grep -rn '1\.25\|1\.26' --include='*.md' .` returns hits only under
      `docs/tasks/task-0NN-*/` (historical archives, intentionally untouched).

Renovate:

- [ ] `renovate.json` is valid against its `$schema` and contains the new Go
      toolchain grouping rule.
- [ ] The TypeScript `<7` ceiling and the in-repo module exclusion rules are
      present and unmodified.

Scope discipline:

- [ ] No `.go` file under `apps/` or `packages/` is modified except where
      required to satisfy `make lint-check` or `make test`, and every such edit
      is individually justified in the PR description.
- [ ] Code review (`superpowers:requesting-code-review`) is run and its findings
      addressed before the PR is opened.
