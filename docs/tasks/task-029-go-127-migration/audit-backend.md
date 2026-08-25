# Backend Audit — task-029-go-127-migration (Go 1.27 toolchain bump)

- **Scope:** Go-side changes only, range `285e175..9daa98d`
- **Guidelines Source:** backend-dev-guidelines skill
- **Date:** 2026-08-25
- **Build/Test:** Not re-run (per instructions) — verified green via `.superpowers/sdd/plan/task-7-9-report.md` / `progress.md` (`make ci` all six sub-targets green; four container builds succeed on `golang:1.27-alpine`; both kustomize server dry-runs byte-identical, zero errors).
- **Overall:** PASS

## Scope confirmation

- `git diff 285e175..9daa98d -- '*.go'` touches exactly 3 files, 10 insertions / 7 deletions:
  - `apps/fleet-service/internal/fuel/builder.go` (+7/-6)
  - `apps/fleet-service/internal/maintenancerecord/builder.go` (+1/-0)
  - `apps/media-service/internal/processing/worker_test.go` (+2/-1)
- `git diff -w --ignore-blank-lines 285e175..9daa98d -- '*.go'` is **empty**. PASS — confirms the entire Go source diff is blank-line-only (gofumpt re-alignment + one inserted blank line per file), no logic change.
- Full `git diff --stat 285e175..9daa98d`: 26 files, all accounted for as toolchain-version-string edits (go.mod x6, go.work, Dockerfile x4, CI workflow x2, README, renovate.json, tools/lint.versions, two skill-doc files, five new docs/tasks artifacts). No file outside this inventory changed. PASS.

## 1. SEC-* / "fix, never suppress" constraint (FR-12/FR-13)

| Check | Verification | Result |
|---|---|---|
| No `.golangci.yml` in diff | `git diff --name-status 285e175..9daa98d -- .golangci.yml` → empty | PASS — file absent from diff |
| `default: standard` unmodified | `.golangci.yml:10` reads `default: standard` | PASS — `.golangci.yml:10` |
| gofumpt + goimports still the formatters | `.golangci.yml:14-15` list `gofumpt`, `goimports` | PASS — `.golangci.yml:14-15` |
| No `//nolint` added | `git diff 285e175..9daa98d \| grep -ni nolint` → one hit, at `docs/tasks/task-029-go-127-migration/plan.md:825`, which is prose describing the constraint itself ("No `//nolint`, no `.golangci.yml` exclusion..."), not a directive added to source | PASS — no actual suppression directive introduced |
| No exclusion/narrowing added | Same grep, "exclude" — only the plan.md prose hit above | PASS |
| No rev-gating introduced | `go.mod`/`go.work` diffs (below) show a flat version bump on all 6 modules simultaneously, no conditional build tags or per-module skip | PASS |
| Linter pin moved cleanly | `tools/lint.versions:5` → `GOLANGCI_LINT_VERSION=v2.13.1` (was v2.12.2 per task framing) | PASS — single source of truth updated, no per-CI-job override found |

Verdict: zero suppressions of any kind. The three blank-line insertions are gofumpt's v2.13.1 formatting output for adjacent one-liner method groups — the mechanism guidelines require ("fix in source") rather than a workaround.

## 2. Do the three whitespace edits comply with formatting/style conventions?

Read each file around the change:

- `apps/fleet-service/internal/fuel/builder.go:16-24` — six aligned one-line `Set*` methods, then a blank line, then `SetCreatedByUserID` on its own. Fluent-builder setter style (`patterns-functional.md`) is unaffected — each setter is still `func (b *Builder) SetX(...) *Builder { ...; return b }`. The blank line separates a gofmt-alignment group (the six setters realigned together, since gofumpt/gofmt align consecutive single-line struct-tag-like statements as a block) from the trailing setter that isn't part of that alignment run. This is gofumpt's normal behavior when a run of similarly-shaped one-liners is followed by a differently-shaped one — not a stylistic regression. PASS, no anti-pattern present.
- `apps/fleet-service/internal/maintenancerecord/builder.go:19-27` — same shape: seven aligned setters, blank line, then `SetDocumentMediaIDs` (a different-width line) on its own before the doc-commented `SetDescription`. Consistent with the pattern above. PASS.
- `apps/media-service/internal/processing/worker_test.go:36-40` — test fakes; blank line separates `GetByID` (short) from `GetByIDIncludingDeleted` (longer), both one-line fake methods. No behavior or table-test structure touched; `testing-guide.md`'s table-driven preference is not implicated since this file is a fake/stub, not a test-case table. PASS.

No file reads oddly against the documented fluent-builder convention (`patterns-functional.md`, `scaffolding-checklist.md`). This is cosmetic regrouping by the formatter, not a builder-invariant or `Build()` signature change — `Build() (Model, error)` semantics are untouched in both files (confirmed by reading `fuel/builder.go:28-34`, which still validates `vehicleID` and `gallons` before returning `server.ErrValidation`).

## 3. Skill resource accuracy (`.claude/skills/backend-dev-guidelines/resources/`)

| Claim | File:line | Verified against | Result |
|---|---|---|---|
| Go 1.27, CI pins `'1.27'` at `pr.yml:15,50` | `architecture-overview.md:18-19` | `.github/workflows/pr.yml:15` → `go-version: '1.27'`; `pr.yml:50` → `go-version: '1.27'` | PASS — citation exact |
| `go.work:1` declares `go 1.27.0` | `architecture-overview.md:18` | `go.work:1` → `go 1.27.0` | PASS |
| Builder `golang:1.27-alpine` | `scaffolding-checklist.md:70` | All 4 service Dockerfiles line 1 → `FROM golang:1.27-alpine AS build` | PASS |
| Runtime `alpine:3.24` | `scaffolding-checklist.md:73` | All 4 service Dockerfiles line 31 → `FROM alpine:3.24` | PASS |
| No other stale toolchain/version claim left in the skill directory | `grep -rn "1\.2[4-6][^0-9]\|golang:1\.2[4-6]\|alpine:3\.2[0-3]" .claude/skills/backend-dev-guidelines/` | Zero matches | PASS — full sweep of the skill directory found no leftover stale reference |

All four skill-doc claims in scope check out exactly against the post-change tree, and a directory-wide sweep found no other file under the skill carrying a stale Go/alpine version.

## 4. Go toolchain surface

| Check | Verification | Result |
|---|---|---|
| No `toolchain` directive added | `grep -rn "^toolchain" apps/*/go.mod packages/*/go.mod go.work` → zero matches | PASS — deliberate per design; a `toolchain` line would only name a second version string that could drift from the `go` directive, and omitting it changes nothing about download behavior — under the default `GOTOOLCHAIN=auto` an older local `go` silently downloads 1.27 regardless; only `GOTOOLCHAIN=local` (not set here) would make an older `go` fail outright |
| `go 1.27.0` directive form (not bare `1.27`) | All 6 `go.mod` + `go.work` diffs show `go 1.25.0` → `go 1.27.0` | PASS — full patch-version form used consistently everywhere it was already used pre-change |
| Builder tag `golang:1.27-alpine` is a floating minor tag, not a digest pin | `apps/*/Dockerfile:1` | Confirmed as designed/expected — flagged here per audit instructions as a floating tag but this is the pre-existing repo convention (prior tag `golang:1.26-alpine` was equally floating), not a regression introduced by this branch. No action item. |
| `go.work.sum` working-tree state | `git status --short go.work.sum` → no output | PASS — clean; the known any-Go-workspace-command side effect (build/test/vet/lint, reproduces on unmodified `main`; documented in `progress.md`, ruling P-7 amended) is not present in this worktree right now and was not committed |

## Domain / Sub-domain checklist (DOM-*/SUB-*)

N/A — no domain-model file (`model.go`, `builder.go` logic, `resource.go`, `processor.go`, `provider.go`, `entity.go`, `rest.go`) had any semantic change. The only domain-package files touched (`fuel/builder.go`, `maintenancerecord/builder.go`) are whitespace-only, confirmed by the `-w --ignore-blank-lines` empty diff above. Running the full DOM-01..DOM-19 checklist against unchanged domain logic is out of scope per the audit brief and would manufacture findings against code this branch did not touch.

## Security Review (SEC-*)

N/A — this is not an auth-related service change; no authentication/authorization/token code is in the diff. The only "SEC" item in scope is the fix-never-suppress linter constraint, covered in section 1 above.

## Summary

### Blocking (must fix)
None.

### Non-Blocking (should fix)
None. All four "what I actually want audited" items pass with direct evidence:
1. No suppression of any kind (`//nolint`, `.golangci.yml` exclusion, narrowed `standard` group, rev-gating) — confirmed absent.
2. The three gofumpt-inserted blank lines are ordinary formatter regrouping of one-line setter/method blocks and don't violate the fluent-builder convention.
3. Both edited skill-doc files (`architecture-overview.md`, `scaffolding-checklist.md`) state facts that are true of the post-change tree, and no other file in the skill directory carries a stale version.
4. Toolchain surface is clean: no `toolchain` directive (deliberate), consistent `go 1.27.0` form, and the builder image floats on `golang:1.27-alpine` per pre-existing repo convention (not a new pattern introduced by this branch).
