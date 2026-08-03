# Dev Guidelines Skill Drift — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite `.claude/skills/{backend,frontend}-dev-guidelines` and the two reviewer agent definitions so every normative statement is true of this repository and traceable to a real `file:line`.

**Architecture:** Two tiers over one ground truth. Tier 1 (23 skill files) is prose + worked examples read by implementing agents; Tier 2 (2 reviewer agent files) is the ID-bearing binary checklist. The invariant: a Tier-2 check may only assert something a Tier-1 file states, and a Tier-1 statement may only assert something a `file:line` under `apps/`, `packages/` shows. Tier 1 is rewritten first so Tier 2 can cite it.

**Tech Stack:** Markdown and JSON only. No Go, TypeScript, or YAML source is modified. Verification is a committed bash grep gate (`drift-gate.sh`) plus per-identifier existence greps plus a live dispatch of both reviewer agents.

---

## Global Constraints

Every task's requirements implicitly include this section.

- **Scope.** Only these paths may be modified: `.claude/skills/backend-dev-guidelines/**`, `.claude/skills/frontend-dev-guidelines/**`, `.claude/skills/skill-rules.json`, `.claude/agents/backend-guidelines-reviewer.md`, `.claude/agents/frontend-guidelines-reviewer.md`, and `docs/tasks/task-020-dev-guidelines-skill-drift/**`. `git diff --name-only main...HEAD` must show **zero** files under `apps/`, `packages/`, `deploy/`, `.github/`, or `.claude/settings.json`.
- **Evidence standard (PRD FR-7.1–7.4).** Every code example is copied or minimally adapted from the canonical source set (§ Canonical Sources below), never composed from a pattern description. Every Go identifier in a backend example must exist in `packages/shared-go` or a real `apps/*/internal/*` package. Where a rule is subtle or came from a real defect, cite the `file:line`. No rule may be stated that cannot be checked against something in this tree.
- **No silent rule loss (PRD §8).** Every task below opens with a rule inventory. Each row is `keep` / `fix` / `delete` / `replace`, and every `delete` and `replace` carries a stated reason. The inventory table goes in the commit body for that file.
- **One commit per file.** Commit message form: `docs(task-020): <verb> <path>`. Never amend a prior task's commit; corrections land as follow-up commits so the audit trail survives.
- **Checklist IDs are frozen.** `DOM-*`, `SUB-*`, `SEC-*`, `FE-*` numbers must not be renumbered or gapped — `docs/tasks/*/audit.md` files already cite them. Only `DOM-09` and `DOM-17` change meaning (design D2), both with recorded reasons.
- **Token budget (PRD §8).** These files load into agent context on every Go/TS change. Track net line delta per commit; the target across the whole branch is net-neutral or smaller. Record the running total in each commit body.
- **Verification after every task.** Run `bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh` (unpiped, so the exit code is visible). It must never regress: a check that passed in an earlier task must still pass.
- **Vocabulary map.** One substitution map, applied identically everywhere (design §4.3):

  | Prior-project concept | MyFleet analogue | Real source |
  | --- | --- | --- |
  | bucket (primary aggregate) | vehicle | `apps/fleet-service/internal/vehicle` |
  | policy (child config of aggregate) | maintenance schedule | `apps/fleet-service/internal/maintenanceschedule` |
  | ban (list-with-filters resource) | invite | `apps/fleet-service/internal/invite` |
  | replication (long-running triggered op) | purge | `apps/fleet-service/internal/vehicle/purge.go` |
  | `bucketarchive` (sub-domain action-event) | `vehiclemedia` | `apps/fleet-service/internal/vehiclemedia` |
  | `policyrevoke` | `mileage` | `apps/fleet-service/internal/mileage` |
  | read-only aggregation handler | dashboard | `apps/fleet-service/internal/dashboard` |

---

## Corrections to the Design

Three findings from Phase-3 verification change what the design told the implementer. Read these before Task 2.

### C1 — `database.Query` / `database.SliceQuery` **do** exist

Design §3.1 (DOM-10 row) and §6 state that "neither symbol exists; `packages/shared-go/database/query.go` has no such API." That is wrong. The file is:

```go
// packages/shared-go/database/query.go
package database

// Provider is a lazy, re-runnable data fetch (design §6: "Lazy data access").
type Provider[T any] func() (T, error)

func Query[T any](fetch func() (T, error)) Provider[T]          { return fetch }
func SliceQuery[T any](fetch func() ([]T, error)) Provider[[]T] { return fetch }
```

Four of nineteen `provider.go` files use the wrapper (`auth-service/internal/{user,session}`, `fleet-service/internal/invite`, `media-service/internal/mediavariant`); the other fifteen do not. And where it *is* used it is invoked immediately — `database.Query(func() (Model, error) {...})()`, note the trailing `()` at `apps/auth-service/internal/user/provider.go:49` — so the behaviour is identical to the plain form. The wrapper is a stylistic choice, not a convention.

**Consequences:**
- `model.Provider` / `model.Map` / `model.SliceMap` / `model.ParallelMap` / `model.ErrorProvider` / `model.FixedProvider` / `database.EntityProvider` are still confirmed absent (0 hits each). Gate checks G-04 and G-05 stand.
- Do **not** instruct anyone to delete `database.Query` from the repo, and do not write a guideline that forbids it.
- `DOM-10` must be re-grounded to the invariant all nineteen providers satisfy — the `Provider` interface + `dbProvider` struct + `NewProvider(db)` constructor + `gorm.ErrRecordNotFound` → domain `ErrNotFound` translation — not to the wrapper. Task 26 encodes this.
- `patterns-provider.md` (Task 2) documents the plain form as the default and mentions `database.Query` as an accepted equivalent, citing both.

### C2 — Backend tests use no assertion library

Design §6 asks Task 9 to check the mock convention "against real `*_test.go` across 108 test files." The count is **126**, and the finding is stronger than expected:

- `mock/` directories: **0**. `type ...Mock struct`: **0**.
- `stretchr/testify` in any `.go` source: **0** (it appears only transitively in `go.sum`).
- The real convention is unexported `fake*` / `stub*` structs declared inside the `_test.go` file that uses them, in the same package — e.g. `apps/auth-service/internal/user/processor_test.go:14` (`fakeProvider`), `:37` (`fakeAdmin`), `apps/fleet-service/internal/invite/processor_test.go:16` (`stubProvider`), `:354` (`stubAdministrator`). Assertions are stdlib `t.Fatalf` / `t.Errorf`.

So `testing-guide.md`'s `require.NoError` examples are drift too, which the design did not record. Task 9 deletes both the mock-directory sections and the testify usage.

### C3 — Gate-4 backend target is task-014, not task-013

Design §9 leaves the choice to Phase 3. **Chosen: task-014 (`member-names-ownership-transfer`, merge `92b1290`).**

It touches three packages — `apps/fleet-service/internal/membership`, `apps/auth-service/internal/{membership,user}` — and both `fleet-service/internal/membership` and `auth-service/internal/user` are complete domain packages carrying `model.go`, `entity.go`, `builder.go`, `provider.go`, `administrator.go`, `processor.go`, `resource.go`, `rest.go`. That exercises DOM-01 through DOM-19 across two services.

task-013 was rejected: its only `model.go` is `media-service/internal/mediavariant`, which has no `resource.go` and no `rest.go`, so DOM-04, DOM-05, DOM-07, DOM-08, DOM-09, DOM-16, DOM-17 and DOM-18 would go unexercised — precisely the checks this task changes.

---

## Canonical Sources

Every rewritten example is copied or minimally adapted from this set. An identifier that does not appear in one of these files is a defect (Gate 2).

**Backend**

| Concern | Canonical file |
| --- | --- |
| Domain package, end to end | `apps/fleet-service/internal/vehicle/` — `model.go`, `entity.go`, `builder.go`, `provider.go`, `administrator.go`, `processor.go`, `resource.go`, `rest.go` |
| Server / handler API | `packages/shared-go/server/handler.go` |
| JSON:API envelope, 5xx redaction | `packages/shared-go/server/jsonapi.go` |
| Error sentinels + status mapping | `packages/shared-go/server/errors.go` |
| Pagination | `packages/shared-go/server/pagination.go` |
| Lazy-wrapper provider variant | `packages/shared-go/database/query.go` · `apps/auth-service/internal/user/provider.go` |
| Processor with collaborators | `apps/fleet-service/internal/maintenancerecord/processor.go:19` |
| Processor with optional deps | `apps/media-service/internal/mediaobject/processor.go:202` |
| Sub-domain / action-event package | `apps/fleet-service/internal/vehiclemedia/` |
| Read-only aggregation handler | `apps/fleet-service/internal/dashboard/` |
| `db.Save` full-column hazard | `apps/fleet-service/internal/vehicle/model.go:20-27` |
| Entity guard | `packages/shared-go/database/entityguard/entityguard.go` |
| Test conventions | `apps/auth-service/internal/user/processor_test.go` · `apps/fleet-service/internal/vehicle/{processor_test.go,rest_test.go}` · `apps/auth-service/internal/user/provider_test.go` |
| Architecture tests | `apps/fleet-service/internal/admin/arch_test.go` · `apps/auth-service/internal/arch/arch_test.go` |
| Build/CI | `Makefile` · `.github/workflows/pr.yml` · `go.work` |
| Deploy | `deploy/k8s/base/kustomization.yaml` · `deploy/k8s/overlays/{local,main}` |

**Frontend**

| Concern | Canonical file |
| --- | --- |
| Service layer | `apps/web/src/services/api/BaseService.ts` · `VehicleService.ts` |
| React Query hooks + key factory | `apps/web/src/lib/hooks/api/vehicles.ts` |
| Provider stack | `apps/web/src/components/providers/AppProviders.tsx` · `App.tsx` |
| API client / errors | `apps/web/src/lib/api/client.ts` · `refresh.ts` · `packages/shared-ts/src/errors.ts` |
| Shared JSON:API types | `packages/shared-ts/src/jsonapi.ts` |
| Forms + Zod | `apps/web/src/lib/schemas/vehicle.ts` |
| Test conventions | `apps/web/src/test/renderWithProviders.tsx` · `lib/hooks/api/mileage.test.ts` · `components/PageHeader.test.tsx` · `src/test/conventions.test.ts` |
| Build/test config | `apps/web/package.json` · `tsconfig.app.json` · `vite.config.ts` |

Two of these — `arch_test.go` and `src/test/conventions.test.ts` — are executable guideline enforcement that already exists. Where a rewritten rule is already guarded by one of them, say so and cite it.

---

## Ground Truth Reference

Values every task needs, verified on branch `task-020-dev-guidelines-skill-drift` @ `2c5e8d4`. Do not re-derive these; do re-read the file if you need surrounding context.

**Repository layout.** Go services: `apps/{auth,fleet,media,notification}-service`, each with its own `go.mod`, `cmd/`, `internal/`, `Dockerfile`. Web app: `apps/web`. Shared Go: `packages/shared-go`, `packages/dto-go`. Shared TS: `packages/shared-ts`, `packages/ui-components`. Modules are joined by `go.work`. **There is no `services/` directory and no `migrations/` directory** — migrations are `func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }` in each domain's `entity.go` (e.g. `apps/auth-service/internal/user/entity.go:42`).

**Build commands (`Makefile`).** `make ci` = `lint-check vet test build fe-test fe-build manifests carfax-template`. Individually: `make vet`, `make test`, `make build`, `make fe-test`, `make fe-build`, `make lint-check`, `make manifests`. Node may need `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.

**`packages/shared-go/server` public API.**

```go
// handler.go
func New(log logrus.FieldLogger) *Server           // also calls SetErrorLogger(log)
func (s *Server) Logger() logrus.FieldLogger
func (s *Server) Use(mw ...func(http.Handler) http.Handler) *Server
func (s *Server) AddRouteInitializer(fn func(chi.Router)) *Server
func (s *Server) Router() chi.Router               // no Run()
func RegisterInputHandler[T any](fn func(http.ResponseWriter, *http.Request, T)) http.HandlerFunc

// jsonapi.go
type Resource struct { Type string; ID string; Attributes any; Relationships map[string]any }
type Document struct { Data any; Meta any; Links map[string]any }
func WriteJSON(w http.ResponseWriter, status int, body any)
func WriteError(w http.ResponseWriter, err error)   // 5xx title redacted to InternalErrorTitle
func SetErrorLogger(log logrus.FieldLogger)
const InternalErrorTitle = "internal server error"

// errors.go
var ErrBadRequest, ErrUnauthorized, ErrForbidden, ErrNotFound, ErrConflict,
    ErrGone, ErrRequestEntityTooLarge, ErrUnsupportedMediaType, ErrValidation,
    ErrTooManyRequests error                        // 400 401 403 404 409 410 413 415 422 429
func StatusFor(err error) int                       // default 500
func Detailed(base error, detail string) error      // adds JSON:API `detail`, 4xx only
type APIError struct { Status, Code, Title, Detail string; Source *ErrorSource }

// pagination.go
type Page struct { Number, Size int }
type PageMeta struct { Total, TotalPages, Number, Size int }
func ParsePage(r *http.Request) Page                // page[number] default 1, page[size] default 25 max 100
func (p Page) Offset() int
func (p Page) Meta(total int) PageMeta
```

**Identifiers that do NOT exist anywhere in `apps/` or `packages/`** (0 hits each — these are the drift): `api2go`, `jsonapi.ServerInformation`, `server.RegisterHandler`, `server.MarshalResponse`, `server.RouteInitializer`, `server.GetHandler`, `server.InputHandler` (as a type), `HandlerDependency`, `HandlerContext`, `d.Logger()`, `ParseId`, `RestModel`, `GetName`, `SetID`, `model.Provider`, `model.Map`, `model.SliceMap`, `model.ParallelMap`, `model.ErrorProvider`, `model.FixedProvider`, `database.EntityProvider`, `mux.Router`, `uuid.UUID`, `uint32` (as an entity ID).

**Frontend facts.** Root is `apps/web`. `src/` contains `App.tsx`, `main.tsx`, `index.css`, `components/`, `context/`, `lib/`, `pages/`, `services/`, `test/`, `types/`. `components/` splits into `admin/`, `features/`, `frame/`, `providers/`, `ui/` — **there is no `components/common/`**. `types/` contains only `models/` — **there is no `types/api/`**. There is **no `lib/breadcrumbs/`**, **no `lib/query-client.ts`**, **no `services/api/index.ts`**, **no `__mocks__/` directory**, **no `DataTable` component**, and TanStack React Table is not a dependency. **No `@/*` path alias is configured** in `tsconfig.app.json` or `vite.config.ts`; imports are relative (`'../../lib/api/client'`). `exactOptionalPropertyTypes` and `noImplicitOverride` are not set. Services are PascalCase (`VehicleService.ts`, `MediaService.ts`); schemas are `lib/schemas/vehicle.ts` (no `.schema.` infix). Test runner is Vitest (`"test": "vitest run"`); `@testing-library/jest-dom` is a real matcher library and its import is **not** drift.

---

## Task 1: Drift gate script and failing baseline

The gate is written first and must fail. Every later task drives checks from FAIL to PASS; Task 28 requires all-PASS.

**Files:**
- Create: `docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh`
- Create: `docs/tasks/task-020-dev-guidelines-skill-drift/drift-baseline.txt`

**Interfaces:**
- Produces: `drift-gate.sh`, run from anywhere in the worktree (it `cd`s to the repo root). Exits `0` when all checks pass, `1` otherwise. Every later task runs it.

- [ ] **Step 1: Write the gate script**

Create `docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh`:

```bash
#!/usr/bin/env bash
# Drift gate for task-020. Every check must report 0.
# Scope: the two dev-guidelines skills plus the two reviewer agents that read them.
set -uo pipefail
cd "$(git rev-parse --show-toplevel)" || exit 2

SCOPE=(
  .claude/skills/backend-dev-guidelines
  .claude/skills/frontend-dev-guidelines
  .claude/skills/skill-rules.json
  .claude/agents/backend-guidelines-reviewer.md
  .claude/agents/frontend-guidelines-reviewer.md
)
FE=.claude/skills/frontend-dev-guidelines
BE=.claude/skills/backend-dev-guidelines
FEAGENT=.claude/agents/frontend-guidelines-reviewer.md
BEAGENT=.claude/agents/backend-guidelines-reviewer.md

fail=0
check() { # check <id> <description> <grep-output>
  local id=$1 desc=$2 out=$3 n
  n=$(printf '%s' "$out" | grep -c .)
  if [ "$n" -eq 0 ]; then
    printf 'PASS  %-8s %s\n' "$id" "$desc"
  else
    fail=1
    printf 'FAIL  %-8s %s  (%d)\n' "$id" "$desc" "$n"
    printf '%s\n' "$out" | sed 's/^/        /'
  fi
}

# --- PRD §10 mechanical checks ---
check G-01 "api2go"                  "$(grep -rin 'api2go' "${SCOPE[@]}")"
check G-02 "ServerInformation"       "$(grep -rn 'ServerInformation' "${SCOPE[@]}")"
check G-03 "MarshalResponse"         "$(grep -rn 'MarshalResponse' "${SCOPE[@]}")"
check G-04 "model.* composition lib" "$(grep -rnE 'model\.(Provider|Map|SliceMap|ParallelMap|ErrorProvider|FixedProvider)' "${SCOPE[@]}")"
check G-05 "EntityProvider"          "$(grep -rn 'EntityProvider' "${SCOPE[@]}")"
check G-06 "RouteInitializer"        "$(grep -rn 'RouteInitializer' "${SCOPE[@]}")"
check G-07 "RegisterHandler("        "$(grep -rnE 'RegisterHandler\(' "${SCOPE[@]}")"
check G-08 "services/<svc>-service"  "$(grep -rnE '\bservices/[a-z-]+-service' "${SCOPE[@]}")"
check G-09 "Jest (not jest-dom)"     "$(grep -rinE '\bjest\b' "$FE" "$FEAGENT" | grep -v 'jest-dom')"
# Substring, not \b-anchored: the drift also lives inside identifiers
# (bucketKeys, bansService, CreateBucketDialog, BanType). English words that
# merely contain "ban" are filtered; a legitimate prose use of "policy" can be
# kept by putting ALLOW-VOCAB on the line.
check G-10 "prior-project vocabulary" "$(grep -rinE 'bucket|replication|\bban|policy|policies' "${SCOPE[@]}" | grep -viE 'banner|bandwidth|abandon|\bband\b|ALLOW-VOCAB')"

# --- design §9 Gate 1 additions ---
check G-11 "RestModel / GetName()"   "$(grep -rnE 'RestModel|GetName\(\)' "${SCOPE[@]}")"
check G-12 "@/ path alias"           "$(grep -rnE "from ['\"]@/" "$FE" "$FEAGENT")"
check G-13 "__mocks__/watchAll/mux"  "$(grep -rnE '__mocks__|watchAll|Methods\(http\.Method' "${SCOPE[@]}")"

# --- additions from Phase-3 verification ---
check G-14 "uuid.UUID entity ids"    "$(grep -rn 'uuid\.UUID' "${SCOPE[@]}")"
check G-15 "uint32 entity ids"       "$(grep -rn 'uint32' "${SCOPE[@]}")"
check G-16 "gorilla mux idiom"       "$(grep -rnE 'mux\.Router|router\.HandleFunc' "${SCOPE[@]}")"
check G-17 "fake handler deps"       "$(grep -rnE 'HandlerDependency|HandlerContext|server\.GetHandler|d\.Logger\(\)|ParseId\(' "${SCOPE[@]}")"
check G-18 "testify in Go examples"  "$(grep -rnE 'require\.(NoError|Error|Equal)|assert\.' "$BE" "$BEAGENT")"
check G-19 "mock-struct convention"  "$(grep -rnE 'Mock struct|\{package\}/mock/|mock/processor\.go|mock/provider\.go' "${SCOPE[@]}")"
check G-20 "frontend/ root path"     "$(grep -rnE '(^|[^a-z/])frontend/' "$FE" "$FEAGENT")"
check G-21 "dead FE paths"           "$(grep -rnE 'components/common/|types/api/|lib/breadcrumbs/|lib/query-client|React Table|DataTable|data-table|\.service\.ts|services/api/index\.ts' "$FE" "$FEAGENT")"
check G-22 "unset tsconfig flags"    "$(grep -rnE 'exactOptionalPropertyTypes|noImplicitOverride' "$FE")"
check G-23 "compose/nginx/bruno"     "$(grep -rniE 'docker-compose|deploy/compose|nginx\.conf|bruno' "$BE")"

echo
if [ "$fail" -eq 0 ]; then echo "drift-gate: ALL CHECKS PASS"; else echo "drift-gate: FAILURES PRESENT"; fi
exit "$fail"
```

- [ ] **Step 2: Make it executable and run it to verify it FAILS**

```bash
chmod +x docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh > docs/tasks/task-020-dev-guidelines-skill-drift/drift-baseline.txt
echo "exit=$?"
```

Expected: `exit=1`, and the summary lines are exactly these 23 FAILs with these counts. If a count differs, the tree has moved since planning — stop and reconcile before continuing.

```
FAIL  G-01     api2go  (7)
FAIL  G-02     ServerInformation  (7)
FAIL  G-03     MarshalResponse  (7)
FAIL  G-04     model.* composition lib  (13)
FAIL  G-05     EntityProvider  (5)
FAIL  G-06     RouteInitializer  (6)
FAIL  G-07     RegisterHandler(  (7)
FAIL  G-08     services/<svc>-service  (4)
FAIL  G-09     Jest (not jest-dom)  (25)
FAIL  G-10     prior-project vocabulary  (269)
FAIL  G-11     RestModel / GetName()  (30)
FAIL  G-12     @/ path alias  (12)
FAIL  G-13     __mocks__/watchAll/mux  (12)
FAIL  G-14     uuid.UUID entity ids  (13)
FAIL  G-15     uint32 entity ids  (3)
FAIL  G-16     gorilla mux idiom  (11)
FAIL  G-17     fake handler deps  (52)
FAIL  G-18     testify in Go examples  (3)
FAIL  G-19     mock-struct convention  (5)
FAIL  G-20     frontend/ root path  (4)
FAIL  G-21     dead FE paths  (27)
FAIL  G-22     unset tsconfig flags  (4)
FAIL  G-23     compose/nginx/bruno  (6)
drift-gate: FAILURES PRESENT
```

- [ ] **Step 3: Record the line-count baseline**

Append to `drift-baseline.txt`:

```bash
{
  echo
  echo "=== line counts at baseline ==="
  find .claude/skills -name '*.md' -exec wc -l {} + | sort -n
  wc -l .claude/agents/backend-guidelines-reviewer.md .claude/agents/frontend-guidelines-reviewer.md
} >> docs/tasks/task-020-dev-guidelines-skill-drift/drift-baseline.txt
```

Baseline totals to beat: skills `4389` lines across 23 `.md` files; agents `198` + `167`.

- [ ] **Step 4: Commit**

```bash
git add docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh \
        docs/tasks/task-020-dev-guidelines-skill-drift/drift-baseline.txt
git commit -m "test(task-020): drift gate script and failing baseline"
```

---

## Task 2: Rebuild `backend-dev-guidelines/resources/patterns-provider.md` (class R)

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/patterns-provider.md` (39 lines, full rebuild)

**Canonical sources:** `apps/fleet-service/internal/vehicle/provider.go` (whole file, 50 lines) · `apps/fleet-service/internal/vehicle/administrator.go:1-60` · `packages/shared-go/server/pagination.go` · `packages/shared-go/database/query.go` · `apps/auth-service/internal/user/provider.go:38-50`

**Interfaces:**
- Produces: the canonical vocabulary for data access — `Provider` interface, `dbProvider`, `NewProvider(db)`, `Administrator` interface, `dbAdministrator`, `NewAdministrator(db)`, `TxHook`, `ErrNotFound`, `server.Page`. Tasks 4, 5, 7, 8 and 26 cite this file rather than re-deriving these names.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| P1 | 4 | Frontmatter: "Functional data access pattern used for lazy evaluation and error propagation" | **replace** | Describes a library that was never written. New description: the per-domain read/write data-access contract. |
| P2 | 13 | "Return `model.Provider[T]` for lazy evaluation" | **delete** | `model.Provider` has 0 hits in the tree. No correct identifier to substitute. |
| P3 | 14 | "Compose via `model.Map`, `model.SliceMap`, `model.ParallelMap`" | **delete** | All three have 0 hits. The repo composes with plain method calls. |
| P4 | 15 | "Use `model.ErrorProvider[T]` for error propagation" | **delete** | 0 hits. Real error propagation is a plain `(Model, error)` return with sentinel translation. |
| P5 | 19-33 | `getById` example returning `database.EntityProvider[Entity]` | **replace** | `EntityProvider` has 0 hits; the example also uses `uint32` IDs (real IDs are string UUIDs) and `model.FixedProvider`. Replaced by `vehicle/provider.go` verbatim. |
| P6 | 31-33 | `ByIdProvider` on `*ProcessorImpl` with `p.db.WithContext(p.ctx)` | **delete** | No processor holds a `*gorm.DB` or a `ctx` field; processors hold a `Provider` and an `Administrator` (`maintenancerecord/processor.go:19`). Covered instead by Task 5's processor-constructor section. |
| P7 | 36-38 | Benefits: declarative pipelines / clear error handling / testable | **replace** | "Declarative data pipelines" is false. Keep "testable and composable" — the interface + `NewProvider` seam is exactly what makes the `fake*` doubles in Task 9 possible — and restate error handling as the sentinel-translation rule. |
| — | — | *(absent)* — `gorm.ErrRecordNotFound` → `ErrNotFound` translation | **add** | Real, load-bearing, and previously undocumented (`vehicle/provider.go:26-30`). PRD FR-3.1. |
| — | — | *(absent)* — `server.Page` threading through list methods | **add** | Real and previously undocumented (`vehicle/provider.go:15,45-46`). PRD FR-3.1. |
| — | — | *(absent)* — the whole `Administrator` write side | **add** | Design D3: the write contract lived only in `file-responsibilities.md` and was fully drifted. Owning both halves here is what stops them diverging again. |
| — | — | *(absent)* — `TxHook` | **add** | Design §3.2. Present in code (`vehicle/administrator.go:9-12,42-60`), absent from guidelines; the write-side analogue of the read-side sentinel translation. |
| — | — | *(absent)* — `database.Query` / `SliceQuery` as an accepted variant | **add** | Correction C1. The symbol is real and used by 4 of 19 providers; a guideline that ignores it would make those four look non-compliant. |

- [ ] **Step 1: Confirm the gate is red for this file's checks**

```bash
grep -nE 'model\.(Provider|Map|SliceMap|ParallelMap|ErrorProvider|FixedProvider)|EntityProvider|uint32' \
  .claude/skills/backend-dev-guidelines/resources/patterns-provider.md
```

Expected: 8 matching lines (13, 14, 15, 19, 20, 24, 26, 31 and the `uint32` on 19/31).

- [ ] **Step 2: Replace the file body**

Write `.claude/skills/backend-dev-guidelines/resources/patterns-provider.md`:

````markdown
---
title: Data Access — Provider and Administrator
description: The per-domain read (Provider) and write (Administrator) contracts, their constructors, error translation, pagination, and transaction hooks.
---

# Data Access — Provider and Administrator

Every domain package splits database access in two: `provider.go` owns reads,
`administrator.go` owns writes. Both are **interfaces** with a `db`-backed
implementation and a `New*` constructor. Nothing else in the package touches
GORM.

The interface is not ceremony — it is the seam the processor tests substitute.
See [testing-guide.md](testing-guide.md) for the `fake*` / `stub*` doubles that
plug into it.

## Provider (reads)

```go
// apps/fleet-service/internal/vehicle/provider.go
var ErrNotFound = errors.New("vehicle not found")

// Provider is the read-only interface for vehicle data access.
type Provider interface {
	GetByID(id string) (Model, error)
	ListByFleet(fleetID string, page server.Page) ([]Model, int, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }
```

**Rules:**

- The interface returns **domain `Model`s**, never `Entity`s. `Make(e)` runs
  inside the provider.
- Methods return `(Model, error)` — eager, plain values. There is no lazy
  provider type.
- IDs are `string` UUIDs (`uuid.NewString()`, `builder.go:13`), never `uint32`
  and never `uuid.UUID`.
- Providers never modify state.

### Translate `gorm.ErrRecordNotFound` at the boundary

A raw `gorm.ErrRecordNotFound` reaching a handler maps to **500**, not 404 —
`server.StatusFor` does not recognise it. Translate it in the provider, where
the query is:

```go
func (p *dbProvider) GetByID(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}
```

Each domain declares its own `ErrNotFound` sentinel. Map it to
`server.ErrNotFound` at the handler, or wrap it with `server.Detailed` — see
[patterns-rest-jsonapi.md](patterns-rest-jsonapi.md#error-handling).

### Pagination is a provider parameter

List methods take `server.Page` and return `(models, total, error)`. The total
is a separate `COUNT`, because the page query is offset-limited:

```go
func (p *dbProvider) ListByFleet(fleetID string, page server.Page) ([]Model, int, error) {
	var total int64
	if err := p.db.Model(&Entity{}).Where("fleet_id = ? AND deleted_at IS NULL", fleetID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := p.db.Where("fleet_id = ? AND deleted_at IS NULL", fleetID).
		Offset(page.Offset()).Limit(page.Size).Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}
```

The handler builds the page with `server.ParsePage(req)` and the response meta
with `page.Meta(total)` (`packages/shared-go/server/pagination.go`).

### Accepted variant: `database.Query`

`packages/shared-go/database/query.go` offers a thin wrapper:

```go
type Provider[T any] func() (T, error)
func Query[T any](fetch func() (T, error)) Provider[T]
func SliceQuery[T any](fetch func() ([]T, error)) Provider[[]T]
```

Four domains wrap their query bodies in it and invoke immediately — note the
trailing `()`:

```go
// apps/auth-service/internal/user/provider.go:39-50
func (s *dbProvider) GetByID(id string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := s.db.Where("id = ?", id).First(&e).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}
```

Behaviourally identical to the plain form. **Both are acceptable**; the plain
form is the majority (15 of 19 `provider.go` files) and is what a new domain
should use. Do not convert existing code between the two.

## Administrator (writes)

```go
// apps/fleet-service/internal/vehicle/administrator.go:14-32
// Administrator is the write interface for vehicle data access.
// All mutations (inserts, updates, soft-delete, restore, primary-image) go here.
type Administrator interface {
	Insert(Model) (Model, error)
	InsertWithHooks(m Model, hooks ...TxHook) (Model, error)
	Update(Model) (Model, error)
	SoftDelete(id string) (Model, error)
	RestoreRow(id string) (Model, error)
	SetPrimaryImage(id, mediaID string) (Model, error)
	GetByIDIncludingDeleted(id string) (Model, error)
	UpdateCurrentMileage(vehicleID string, mileage int) error
}

type dbAdministrator struct{ db *gorm.DB }

// NewAdministrator returns an Administrator backed by the given database.
func NewAdministrator(db *gorm.DB) Administrator { return &dbAdministrator{db: db} }
```

**Rules:**

- Takes a `Model` and returns a `Model`. The administrator calls `m.ToEntity()`
  on the way in and `Make(e)` on the way out — the processor never sees an
  `Entity`.
- Every state change lives here. `db.Create` / `db.Save` / `db.Delete` must not
  appear in `processor.go` or `resource.go`.

```go
func (a *dbAdministrator) Insert(m Model) (Model, error) {
	e := m.ToEntity()
	if err := a.db.Create(&e).Error; err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
```

### `db.Save` UPDATEs every column

`db.Save` writes **all** columns, so any column the `Model` does not carry is
silently zeroed on an ordinary write. The fix is to round-trip the field through
the `Model` even when nothing reads it, as `vehicle` does
(`apps/fleet-service/internal/vehicle/model.go:20-27`):

```go
// purgeOperationID must round-trip through the Model: Administrator writes
// with db.Save, which UPDATEs every column, so a field the Model does not
// carry is silently zeroed on any ordinary write. Zeroing THIS one detaches
// the row from the purge that owns it — it stays soft-deleted but becomes
// unreachable by both restore and reap. entityguard caught it.
purgeOperationID *string
purgeAfter       *time.Time
```

`packages/shared-go/database/entityguard` is the executable guard for this
class of bug. When you add a column to an `Entity`, add the matching field to
the `Model` in the same commit.

### `TxHook` — side effects that must commit with the write

When work must commit or roll back atomically with the insert, pass it as a
hook rather than running it after the write returns
(`apps/fleet-service/internal/vehicle/administrator.go:9-12,42-60`):

```go
// TxHook runs side-effecting work (activity append + event emission) on the same
// transaction as the vehicle insert, so the writes commit/rollback atomically.
// Errors are FATAL: they roll back the whole transaction.
type TxHook func(tx *gorm.DB, created Model) error

func (a *dbAdministrator) InsertWithHooks(m Model, hooks ...TxHook) (Model, error) {
	var created Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		e := m.ToEntity()
		if err := tx.Create(&e).Error; err != nil {
			return err
		}
		created = Make(e)
		for _, h := range hooks {
			if err := h(tx, created); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return created, nil
}
```

A hook error is fatal to the whole transaction. Do not use a hook for work that
is allowed to fail independently of the write — do that in the processor after
the administrator returns.

## Why this separation

- **Testability** — the processor takes `Provider` and `Administrator` as
  constructor arguments, so a test substitutes a `fakeProvider` / `fakeAdmin`
  with no database.
- **Clear intent** — a reviewer finds every state change by opening one file.
- **Single responsibility** — `provider.go` has no way to write.
````

- [ ] **Step 3: Verify the file's identifiers all exist**

```bash
for id in "type Provider interface" "dbProvider" "NewProvider" "type Administrator interface" \
          "dbAdministrator" "NewAdministrator" "TxHook" "InsertWithHooks" "ErrNotFound" \
          "server.Page" "ParsePage" "entityguard" "database.Query" "SliceQuery"; do
  printf '%-28s %s\n' "$id" "$(grep -rn -- "$id" apps packages --include='*.go' | wc -l)"
done
```

Expected: every count ≥ 1. A `0` means an invented identifier — fix it.

- [ ] **Step 4: Run the gate**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: still `exit=1` overall, but G-04 drops from 13 to 8, G-05 from 5 to 4, G-15 from 3 to 1. No check may go up.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/backend-dev-guidelines/resources/patterns-provider.md
git commit
```

Commit message (paste the rule inventory table from above into the body):

```
docs(task-020): rebuild patterns-provider.md against the real data-access contract

Documented model.Provider/Map/SliceMap/ParallelMap/ErrorProvider/FixedProvider
and database.EntityProvider — all 0 hits in this tree. Rebuilt from
apps/fleet-service/internal/vehicle/{provider,administrator}.go: eager Provider
and Administrator interfaces, ErrNotFound translation, server.Page threading,
db.Save full-column hazard, TxHook. Absorbs the write side per design D3.

Lines: 39 -> <N> (delta <+/-N>; running total <T>)

<rule inventory table>
```

---

## Task 3: Rebuild `backend-dev-guidelines/resources/patterns-rest-jsonapi.md` (class R)

The largest rebuild: 444 lines, ~40 rules, built end-to-end on api2go. Design §12 flags this as the file most likely to lose a real rule — work the inventory row by row.

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/patterns-rest-jsonapi.md` (444 lines, full rebuild)

**Canonical sources:** `apps/fleet-service/internal/vehicle/resource.go` (244 lines) · `apps/fleet-service/internal/vehicle/rest.go` (103 lines) · `packages/shared-go/server/handler.go` · `jsonapi.go` · `errors.go` · `pagination.go`

**Interfaces:**
- Consumes: `Provider`, `Administrator`, `NewProvider`, `NewAdministrator`, `server.Page` (Task 2).
- Produces: the transport vocabulary — `InitializeRoutes(...) func(chi.Router)`, `server.RegisterInputHandler[T]`, `server.WriteJSON`, `server.WriteError`, `server.Resource`, `server.Document`, `Transform`, `TransformSlice`, `createAttributes`, `patchAttributes`. Tasks 4, 6, 7, 8 and 26 cite these names.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| R1 | 4 | Frontmatter names `server.RegisterHandler` and api2go | **replace** | Neither exists. New description: chi routing, `RegisterInputHandler`, hand-rolled JSON:API envelope. |
| R2 | 10 | "Use `server.RegisterHandler` and `server.RegisterInputHandler` for automatic tracing and JSON:API deserialization" | **fix** | `RegisterHandler` does not exist; `RegisterInputHandler` does but is not curried and does no tracing. Restate: body-less routes are plain `http.HandlerFunc` on chi; bodied routes wrap in `RegisterInputHandler[T]`. |
| R3 | 11 | "REST models implement JSON:API interface methods (`GetName()`, `GetID()`, `SetID()`)" | **delete** | 0 hits for all three in the tree. Meaningless without api2go; the `type` is a literal on `server.Resource` (`rest.go:75`). |
| R4 | 12 | "Handlers are thin — delegate ALL business logic to processors" | **keep** | Accurate and enforced by `arch_test.go`. |
| R5 | 13 | "NEVER call provider functions directly from handlers" | **keep** | Accurate. `vehicle/resource.go` calls `proc.*` exclusively. |
| R6 | 14 | "Use `server.MarshalResponse` for success responses" | **replace** | 0 hits. Replaced by `server.WriteJSON(w, status, server.Document{...})`. |
| R7 | 15 | "Map domain errors to HTTP status codes explicitly" | **fix** | The intent survives, the mechanism inverts: handlers call `server.WriteError(w, err)` and `server.StatusFor` does the mapping. Explicit per-error `w.WriteHeader` is now the anti-pattern. |
| R8 | 22-51 | §Route Registration — `InitializeRoutes(si jsonapi.ServerInformation) func(db *gorm.DB) server.RouteInitializer`, `mux.Router`, `router.HandleFunc(...).Methods(...)`, curried `RegisterHandler(l)(si)(name, h)` | **replace** | Every identifier is fictional. Rebuilt from `vehicle/resource.go:29-33`. |
| R9 | 45 | "Return a curried function `func(db *gorm.DB) server.RouteInitializer` for DI" | **replace** | Real DI is explicit parameters on `InitializeRoutes` plus `With*` chaining on the processor (`resource.go:29-31`). |
| R10 | 50 | "Handler names (e.g. 'get-users') are used for tracing and logging" | **delete** | No handler-name registry exists. Tracing is `telemetry.CorrelationIDFromContext` (`resource.go:82`), which is context-based and needs no name. |
| R11 | 54-73 | §Handler Choice Is a Frontend API Contract | **fix** | **Highest-value section in the file — preserve the hazard.** The mechanism changes (`RegisterInputHandler[T]` decodes `{data:{attributes:T}}` and writes `ErrValidation` on failure — `handler.go:42-58` — rather than matching `T.GetName()`), and the failure is now `422` with title `validation`, not `400 "Could not parse request body"`. The rule that the frontend must change in the same commit is unchanged and stays. |
| R12 | 62 | "Define the request type in `rest.go` with `GetName()`/`GetID()`/`SetID()`" | **replace** | Replaced by: define a narrow unexported `createAttributes` / `patchAttributes` struct. |
| R13 | 65 | Envelope example `{"data":{"type":"<T.GetName()>","id":"...","attributes":{}}}` | **fix** | Shape is right; `type` comes from the service's `resourceType` literal, not `GetName()`. Real caller: `VehicleService.restore` (`apps/web/src/services/api/VehicleService.ts:41-47`). |
| R14 | 69-72 | "Conversely, keep body-less handlers on `server.RegisterHandler`" | **fix** | Same rule, real mechanism: register a plain `func(w, r)` directly on chi. |
| R15 | 73 | "Backend tests will not catch it because they invoke the handler directly" | **keep** | Still true, and still the reason the frontend change must be in the same commit. |
| R16 | 80-113 | §GET Handler example — `server.GetHandler`, `HandlerDependency`, `HandlerContext`, `ParseId`, `d.Logger()`, `ops.Map`, `MarshalResponse` | **replace** | Seven fictional identifiers in one example. Rebuilt from `vehicle/resource.go:33-57`. |
| R17 | 116-157 | §POST/PATCH Handler example — `server.InputHandler[CreateRequest]` as a type | **replace** | `InputHandler` is not a type; `RegisterInputHandler` takes a plain func. Rebuilt from `vehicle/resource.go:59-90`. |
| R18 | 161-164 | "Handler Dependency Benefits: `d.Logger()`, `d.Context()`, `c.ServerInformation()`" | **delete** | None exist. The logger is captured by `InitializeRoutes`' `log` parameter and closed over; the context is `req.Context()`. |
| R19 | 168-209 | §REST Model Structure + receiver-type requirements | **replace** | No `RestModel`, no `GetName`/`GetID`/`SetID`. Replaced by the `Attributes` struct + `server.Resource` literal (`rest.go:15-29,74-93`). PRD FR-2.6 explicitly drops the value-receiver rule. |
| R20 | 211-251 | §Request Models implementing the JSON:API interface | **replace** | Replaced by narrow `createAttributes` / `patchAttributes` (FR-2.8). |
| R21 | 248 | "ID field tagged `json:\"-\"` (set via SetID)" | **delete** | No `SetID`. The ID is a separate field on `server.Resource`, not an attribute. |
| R22 | 249 | "Pointer fields for optional attributes (omitempty)" | **keep** | Accurate, and load-bearing on patch structs — pointers distinguish absent from zero (`rest.go:46-52`). |
| R23 | 250 | "Flat structure (no nested Data/Type/Attributes)" | **keep** | Accurate. `RegisterInputHandler` strips the envelope, so the typed struct is the attributes object. |
| R24 | 251 | "`jsonapi.Unmarshal` handles the envelope automatically" | **fix** | Real mechanism is `encoding/json` inside `RegisterInputHandler` (`handler.go:47-57`). |
| R25 | 255-296 | §Transform Functions — `Transform(Model) (RestModel, error)`, `TransformSlice([]Model) ([]RestModel, error)` | **replace** | Real signatures return `server.Resource` / `[]server.Resource` with **no error** (`rest.go:58,97`). |
| R26 | 283-295 | `TransformSlice` implementation with error propagation | **fix** | Loop stays; the error handling goes because `Transform` cannot fail. |
| R27 | — | "**Both `Transform` and `TransformSlice` are mandatory**; list handlers must not inline transform loops" (stated in `file-responsibilities.md:123`, enforced as DOM-05) | **keep** | Accurate. **Add a caveat:** `vehicle` list handlers legitimately inline a loop because they attach per-row derived values via `TransformDerived` (`resource.go:49-52`); the ban is on re-implementing `Transform`, not on decorating it. Without this caveat DOM-05 produces a false positive on `vehicle`. |
| R28 | 300-329 | §Error Handling — per-error `errors.Is` + `w.WriteHeader` ladder | **replace** | Real handlers call `server.WriteError(w, err)` once; `StatusFor` maps the sentinel (`errors.go:18-43`). Show `server.Detailed` for a client-facing message. |
| R29 | 331-335 | Status code guidelines 400/404/409/500 | **fix** | Correct but incomplete. Replace with the real ten-sentinel table from `errors.go:5-16`, and document that anything unrecognised maps to 500. |
| R30 | — | *(absent)* — 5xx bodies are redacted | **add** | `WriteError` replaces a 5xx title with `InternalErrorTitle` and logs the real error server-side (`jsonapi.go:95-116`). This is the behaviour PRD §8 attributes to SEC-09; it is real and belongs here. |
| R31 | 339-377 | §Relationship Endpoints — `AssociatePolicyRequest` with `GetName()` returning the related type | **replace** | No relationship endpoints of this shape exist. Replace with the real nested-resource routing this repo uses: `/fleets/{id}/vehicles` (`resource.go:33,59`) and `/vehicles/{id}/primary-image` delegating across domains via an injected interface (`resource.go:20-23`). |
| R32 | 381-395 | ❌ Calling providers directly from handlers | **keep** | Accurate; only the example's identifiers change. |
| R33 | 397-409 | ✅ Use the processor | **fix** | Same rule; rewrite the example against the real `proc` closure. |
| R34 | 411-424 | ❌ Manual JSON decoding of a nested envelope | **keep** | Accurate — this is exactly what `RegisterInputHandler` exists to prevent, and SUB-04 checks it. |
| R35 | 426-434 | ✅ Use `RegisterInputHandler` with flat request models | **fix** | Rule stands; example becomes `createAttributes`. |
| R36 | 438-444 | §Validation Guidelines (validate in processor, typed domain errors, map in handler, log with context) | **fix** | Keep all four; the fourth's mechanism changes — `WriteError` does the 5xx logging (`jsonapi.go:102-105`), so handlers should not log 4xx separately. **Add:** builder-level invariant validation returning `server.ErrValidation` (`builder.go:26-30`) is the first line, ahead of the processor. |
| — | — | *(absent)* — `createAttributes` / `patchAttributes` narrow-struct convention | **add** | PRD FR-2.8. The named narrow struct *is* the read-only enforcement (`rest.go:31-52`). |
| — | — | *(absent)* — pagination round trip | **add** | `server.ParsePage(req)` → provider → `page.Meta(total)` → `server.Document{Data:..., Meta:...}` (`resource.go:44-56`). |

- [ ] **Step 1: Confirm the gate is red for this file**

```bash
grep -cE 'api2go|ServerInformation|MarshalResponse|RouteInitializer|RegisterHandler\(|RestModel|GetName\(\)|uuid\.UUID|mux\.Router|HandlerDependency' \
  .claude/skills/backend-dev-guidelines/resources/patterns-rest-jsonapi.md
```

Expected: a non-zero count (≈60 lines).

- [ ] **Step 2: Rebuild the file**

Replace the whole body. Section order stays as-is so the diff is readable; the required content per section is the inventory above. These four blocks must be copied **verbatim** from source — they are the ones most likely to be got wrong:

*Route registration and a body-less handler* (from `apps/fleet-service/internal/vehicle/resource.go:29-57`):

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, primaryImage PrimaryImageSetter, statusDeps StatusDeps, record ActivityRecorder, emit EventEmitter) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db)).
		WithActivityRecorder(record).
		WithEventEmitter(emit)
	return func(r chi.Router) {
		// GET /fleets/{id}/vehicles — list vehicles (fleet-paged)
		r.Get("/fleets/{id}/vehicles", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			page := server.ParsePage(req)
			ms, total, err := proc.ListByFleet(fleetID, page)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			now := time.Now().UTC()
			resources := make([]server.Resource, 0, len(ms))
			for _, m := range ms {
				resources = append(resources, TransformDerived(m, statusDeps.Derive(m, now)))
			}
			server.WriteJSON(w, http.StatusOK, server.Document{
				Data: resources,
				Meta: page.Meta(total),
			})
		})
```

*A bodied handler* (`resource.go:59-90`) — show through `proc.Create` and the `server.WriteError` returns; do not paraphrase the `NewBuilder()` chain.

*The typed-input decoder* (`packages/shared-go/server/handler.go:42-58`) — quote it in full under §Handler Choice, because the `ErrValidation`-on-decode-failure line is what makes the breaking-change hazard real.

*Rest-model shape* (`rest.go:15-52` and `:58,69-103`) — the `Attributes` struct, both narrow request structs with their existing comments (they explain *why* the shape is narrow), `Transform`, `TransformDerived`, `TransformSlice`.

- [ ] **Step 3: Verify every identifier exists**

```bash
for id in "func InitializeRoutes" "chi.Router" "chi.URLParam" "RegisterInputHandler" \
          "server.WriteJSON" "server.WriteError" "server.Resource" "server.Document" \
          "server.ParsePage" "server.Detailed" "server.ErrValidation" "server.StatusFor" \
          "InternalErrorTitle" "createAttributes" "patchAttributes" "TransformSlice" \
          "TransformDerived" "CorrelationIDFromContext" "IdentityFromContext"; do
  printf '%-30s %s\n' "$id" "$(grep -rn -- "$id" apps packages --include='*.go' | wc -l)"
done
```

Expected: every count ≥ 1.

- [ ] **Step 4: Run the gate**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected drops: G-01 7→2, G-02 7→3, G-03 7→4, G-06 6→3, G-07 7→4, G-11 30→~22, G-14 13→~9, G-16 11→~5, G-17 52→~35. No check may go up.

- [ ] **Step 5: Commit**

```bash
git add .claude/skills/backend-dev-guidelines/resources/patterns-rest-jsonapi.md
git commit
```

```
docs(task-020): rebuild patterns-rest-jsonapi.md against chi + shared-go/server

444 lines built on api2go, jsonapi.ServerInformation, server.RegisterHandler,
server.MarshalResponse, server.RouteInitializer, RestModel and GetName/GetID/SetID
— all 0 hits in this tree. Rebuilt from apps/fleet-service/internal/vehicle/
{resource,rest}.go and packages/shared-go/server.

Preserved: the handler-choice-is-a-frontend-API-contract hazard (restated against
RegisterInputHandler's decode failure -> ErrValidation), the thin-handler and
no-providers-from-handlers rules, TransformSlice-is-mandatory, flat request
structs, pointer-for-optional, and the validation guidelines.

Added: createAttributes/patchAttributes narrow structs, pagination round trip,
5xx redaction in WriteError, and the DOM-05 caveat for TransformDerived
decoration (vehicle/resource.go:49-52) which would otherwise false-positive.

Lines: 444 -> <N> (delta <+/-N>; running total <T>)

<rule inventory table>
```

---

## Task 4: Repair `backend-dev-guidelines/resources/file-responsibilities.md` (class S)

Section-scoped. Six of thirteen sections are drifted; the layer-violation blocks and rationales are accurate and must survive byte-for-byte.

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/file-responsibilities.md` (128 lines)

**Canonical sources:** `apps/fleet-service/internal/vehicle/` (all eight files) · `apps/fleet-service/internal/maintenancerecord/processor.go:19` · Task 2 and Task 3 output

**Interfaces:**
- Consumes: `Provider` / `Administrator` / `TxHook` (Task 2); `InitializeRoutes` / `server.WriteError` / `Transform` (Task 3).
- Produces: nothing new. This file is a one-paragraph-per-file role index that points at Tasks 2 and 3.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| F1 | 9-10 | `model.go` — immutable objects, private fields, accessors | **keep** | Accurate (`vehicle/model.go:6-27`). |
| F2 | 12-20 | `entity.go` — GORM entities and migrations | **keep** | Accurate. |
| F3 | 17 | `Make(Entity) (Model, error)` | **fix** | Real signature is `func Make(e Entity) Model` — no error — in 17 of 17 domains. This is the claim that makes `DOM-03` fail every domain. |
| F4 | 18 | `ToEntity() Entity` on Model | **keep** | Accurate (`entity.go`), and `DOM-02` checks it. |
| F5 | 20 | "Both directions are mandatory" | **keep** | Accurate. |
| F6 | 22-24 | `builder.go` — fluent, `Build()` enforces invariants | **keep** | Accurate (`builder.go:26-30`). Add that `Build()` returns `server.ErrValidation`, which is what makes the 422 mapping work end to end. |
| F7 | 30 | `NewProcessor(l logrus.FieldLogger, ctx context.Context, db *gorm.DB)` | **replace** | No processor takes a `ctx` or a `*gorm.DB`. Real: `NewProcessor(log logrus.FieldLogger, p Provider, a Administrator)` (`maintenancerecord/processor.go:19`). |
| F8 | 31 | Logger must be `logrus.FieldLogger`, not `*logrus.Logger` | **keep** | Accurate across every real `NewProcessor`; `DOM-06` checks it. |
| F9 | 34-36 | Orchestrate providers/administrators; enforce rules | **keep** | Accurate. |
| F10 | 36 | "Always use `p.db.WithContext(p.ctx)`" | **delete** | Processors hold no `db` and no `ctx`. Nothing in the tree does this. Under FR-7.4 an uncheckable rule is dropped rather than reworded. |
| F11 | 38-41 | ✅/❌ processor → provider / administrator / not direct db | **keep** | Accurate and enforced by `arch_test.go`. |
| F12 | 43-46 | `administrator.go` owns writes | **keep** | Accurate. |
| F13 | 48 | "write functions accept `*gorm.DB` as the first parameter" | **replace** | Real pattern is an exported interface with a `db`-backed impl holding the handle (`administrator.go:15-32`). |
| F14 | 49 | "Return the modified Entity (not Model) — the processor converts via `Make`" | **replace** | Backwards. The administrator calls `Make` itself and returns a `Model`. Design §3.2. |
| F15 | 51-56 | Signatures `create(db, name) (Entity, error)` etc. with `uuid.UUID` | **replace** | Free functions, `Entity` returns and `uuid.UUID` IDs are all fictional. Replace with a pointer to Task 2's file rather than restating it. |
| F16 | 58-61 | "Why This Separation" (testability / intent / SRP) | **keep** | Accurate and worth keeping. |
| F17 | 63-65 | `provider.go` owns reads | **keep** | Accurate. |
| F18 | 68 | "return `database.EntityProvider[T]`" | **replace** | `EntityProvider` has 0 hits. |
| F19 | 69 | "provide `modelFromEntity(e entity) (Model, error)`" | **delete** | 0 hits. The conversion is `Make`, documented at F3. |
| F20 | 70-71 | "Use `database.Query[T]` / `database.SliceQuery[T]`" | **fix** | Both are real (correction C1) but used by only 4 of 19 providers. Restate as an accepted variant, not a requirement. |
| F21 | 72 | "Never modify database state" | **keep** | Accurate. |
| F22 | 74-80 | Typical signatures with `uint32` IDs | **replace** | IDs are string UUIDs. |
| F23 | 82 | "Provider functions are curried … enables lazy evaluation and composition with `model.Map`/`SliceMap`/`ParallelMap`" | **delete** | Nothing is curried; the three `model.*` symbols have 0 hits. |
| F24 | 84-87 | "Why This Separation" (provider) | **keep** | Accurate, minus the "pure composition" clause which references the deleted library — reword to "read path stays side-effect free". |
| F25 | 90-91 | `resource.go` — routes and handlers | **keep** | Accurate. |
| F26 | 94 | `InitializeRoutes(si jsonapi.ServerInformation) func(db *gorm.DB) server.RouteInitializer` | **replace** | Fictional. Point at Task 3. |
| F27 | 95 | "Use `server.RegisterHandler(l)(si)` for GET/DELETE" | **replace** | 0 hits. Real: register a plain `func(w, r)` on chi. |
| F28 | 96 | "Use `server.RegisterInputHandler[T](l)(si)` for POST/PATCH" | **fix** | Right function, wrong shape — it takes one argument (`handler.go:42`). |
| F29 | 97 | "match `server.GetHandler` / `server.InputHandler[T]` signatures" | **delete** | Neither type exists. |
| F30 | 98 | "Map domain errors to HTTP status codes (400/404/409/500)" | **fix** | Mechanism is `server.WriteError`; see Task 3 R28/R29. |
| F31 | 99 | "**Delegate ALL business logic to processors — NEVER call provider functions directly**" | **keep** | Accurate; `DOM-13` checks it. |
| F32 | 100 | "Use `server.MarshalResponse[T]`" | **replace** | 0 hits → `server.WriteJSON`. |
| F33 | 101 | "Log errors with `d.Logger().WithError(err)`" | **replace** | `d.Logger()` has 0 hits. `WriteError` logs 5xx itself (`jsonapi.go:102-105`); handlers should not double-log. |
| F34 | 105-108 | ✅/❌ resource → processor / not provider / not db | **keep** | Accurate; `arch_test.go` enforces it. |
| F35 | 111-112 | `rest.go` — serialization | **keep** | Accurate. |
| F36 | 115 | "Define `RestModel` implementing `GetName()`/`GetID()`/`SetID()`" | **replace** | 0 hits → `Attributes` struct + `server.Resource` literal. |
| F37 | 116 | "Define `CreateRequest`/`UpdateRequest` implementing the JSON:API interface" | **replace** | → `createAttributes` / `patchAttributes`. |
| F38 | 117-118 | `Transform` / `TransformSlice` with error returns | **fix** | No error return. |
| F39 | 119 | "Use flat structure for request models" | **keep** | Accurate. |
| F40 | 120 | "Mark ID field with `json:\"-\"` (set via SetID)" | **delete** | No `SetID`; the ID is a `server.Resource` field, not an attribute. |
| F41 | 121 | "Pointer fields for optional attributes with `omitempty`" | **keep** | Accurate and load-bearing on patch structs. |
| F42 | 123 | "**Both `Transform` and `TransformSlice` are mandatory.** List handlers must use `TransformSlice`" | **keep** | Accurate; `DOM-05`. Add Task 3's `TransformDerived` caveat. |
| F43 | 125 | "JSON:API-compliant DTOs with automatic marshaling via api2go" | **replace** | api2go has 0 hits. |
| — | — | *(absent)* — `administrator.go` may expose `TxHook` | **add** | Design §3.2; one line pointing at Task 2. |

- [ ] **Step 1: Apply the section edits**

Edit only the sections named above. **Do not touch** lines 9-10, 22-24, 38-41, 58-61, 84-87 (minus the one clause in F24), 105-108 — those are the `keep` rows and the four ✅/❌ blocks.

Where a `replace` row would restate a contract Task 2 or Task 3 already owns, write the one-line role statement and link, e.g.:

```markdown
## `administrator.go`

**Write operations** — every insert, update, soft-delete and restore. Exposes an
`Administrator` interface with a `db`-backed implementation and a
`NewAdministrator(db)` constructor; takes and returns domain `Model`s. Side
effects that must commit atomically with the write go through `TxHook`.

Full contract and worked example: [patterns-provider.md](patterns-provider.md).
```

- [ ] **Step 2: Verify no keep-row text was lost**

```bash
git diff -U0 .claude/skills/backend-dev-guidelines/resources/file-responsibilities.md \
  | grep '^-' | grep -iE 'Why This Separation|Testability|Clear intent|Single responsibility|✅|❌'
```

Expected: **no output**. Any hit means a `keep` row was edited — restore it.

- [ ] **Step 3: Run the gate**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: G-02, G-03, G-05, G-06, G-07, G-11, G-14, G-15, G-17 all drop. No check goes up.

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/backend-dev-guidelines/resources/file-responsibilities.md
git commit -m "docs(task-020): re-ground file-responsibilities.md role contracts

Fixed six drifted sections: entity.go (Make returns no error — the claim that
made DOM-03 fail every domain), processor.go (constructor takes Provider and
Administrator, not ctx and db), administrator.go (interface + Model in/out, not
free functions returning Entity — design §3.2), provider.go (no EntityProvider,
no currying, string UUIDs not uint32), resource.go and rest.go (point at
patterns-rest-jsonapi.md).

Kept verbatim: model.go and builder.go sections, all four layer-violation
blocks, both Why-This-Separation rationales.

Lines: 128 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 5: Repair `backend-dev-guidelines/resources/patterns-functional.md` (class S)

Two of five sections are deleted outright. After the deletions nothing "functional" remains, so the file is **retitled** — heading and frontmatter `title` only. **The filename does not change**: `SKILL.md:113` and `.claude/agents/backend-guidelines-reviewer.md:47` both reference the path, and renaming it would churn two files for no benefit.

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/patterns-functional.md` (59 lines)

**Canonical sources:** `apps/fleet-service/internal/vehicle/model.go:5-27` · `builder.go` (whole file) · `apps/fleet-service/internal/maintenancerecord/processor.go:19` · `apps/media-service/internal/mediaobject/processor.go:202` · `apps/fleet-service/internal/vehicle/resource.go:29-31`

**Interfaces:**
- Consumes: `Provider`, `Administrator` (Task 2).
- Produces: `NewProcessor(log, Provider, Administrator)`, the `With*` chaining form, and `ProcessorOption`. Tasks 4, 7, 8 and 26 (`DOM-06`, `DOM-07`) cite these.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| N1 | 3-4 | Frontmatter: "Immutability, builders, and curried function patterns" | **replace** | Nothing is curried. New title: *Model, Builder and Processor Patterns*. |
| N2 | 10-13 | §Immutability — private fields, public getters, mutations via builders | **keep** | Accurate (`vehicle/model.go:6-27`). Add the `With*` copy-returning form, which the file omits and `model.go` uses. |
| N3 | 16-25 | §Builder Pattern — fluent, validation in `Build()`, `Model.Builder()` | **keep** | Accurate. **Fix the example only:** `SetId(1)` uses an int ID; real builders take no ID (it is `uuid.NewString()` in `NewBuilder()`, `builder.go:13`). Replace with the real `vehicle` chain. |
| N4 | 25 | "`Model.Builder()` supports modification flows" | **fix** | Verify against the tree before keeping. If no domain exposes `Model.Builder()`, delete it under FR-7.4 and document the `With*` mutator as the modification path instead. Record which way it went. |
| N5 | 28-44 | §Processor Constructor — `NewProcessor(l, ctx, db)` holding `ctx` and `db` | **replace** | No processor holds either. Real form takes collaborators. |
| N6 | 30 | "must accept `logrus.FieldLogger`, not `*logrus.Logger`" | **keep** | Accurate everywhere; `DOM-06`. |
| N7 | 44 | "This ensures handlers can pass `d.Logger()`" | **replace** | `d.Logger()` has 0 hits. The real reason: `InitializeRoutes` receives `log logrus.FieldLogger` and passes it straight through (`vehicle/resource.go:29-30`). This is also what `DOM-07` must be re-grounded to in Task 26. |
| N8 | 46-51 | §Curried Function Pattern — `func Create(db, log) func(CreateParams) model.Provider[Entity]`; "consistent function-first design over interface abstractions" | **delete** | Not valid against this tree and actively wrong: the repo is built on interfaces (`Provider`, `Administrator`, `PrimaryImageSetter`, `OwnerChecker`). This section argues against the codebase's actual practice. |
| N9 | 53-59 | §Functional Composition — `model.Map(Transform)(entityProvider)(model.ParallelMap())()` | **delete** | Not valid Go (it does not parse), and all three identifiers have 0 hits. PRD FR-3.2. |
| — | — | *(absent)* — optional dependencies via `With*` chaining | **add** | Real (`resource.go:30-31`: `.WithActivityRecorder(record).WithEventEmitter(emit)`). |
| — | — | *(absent)* — optional dependencies via `ProcessorOption` | **add** | Real variant (`apps/media-service/internal/mediaobject/processor.go:202`). Document both and say which to use when. |

- [ ] **Step 1: Resolve N4 before editing**

```bash
grep -rn "func (m Model) Builder()" apps --include='*.go'
```

If zero hits, delete rule N4 and record the reason in the commit body. If it exists, keep it and cite the `file:line`.

- [ ] **Step 2: Apply the edits**

Retitle, keep §Immutability and §Builder Pattern (fixing only the `SetId(1)` example), replace §Processor Constructor with:

```go
// apps/fleet-service/internal/maintenancerecord/processor.go:19
func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor
```

plus the two optional-dependency forms, then delete §Curried Function Pattern and §Functional Composition entirely.

- [ ] **Step 3: Run the gate**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: G-04 drops to 0 or near it, G-17 drops. No check goes up.

- [ ] **Step 4: Commit**

```bash
git add .claude/skills/backend-dev-guidelines/resources/patterns-functional.md
git commit -m "docs(task-020): retitle patterns-functional.md; drop the fictional composition sections

Deleted §Curried Function Pattern (model.Provider[Entity], 0 hits; the section
argues against this repo's actual interface-based DI) and §Functional
Composition (model.Map/ParallelMap, 0 hits, and the snippet does not parse).
Rewrote §Processor Constructor against NewProcessor(log, Provider, Administrator)
with the With*-chaining and ProcessorOption variants.

Kept §Immutability and §Builder Pattern. Retitled to 'Model, Builder and
Processor Patterns'; filename unchanged because SKILL.md:113 and
backend-guidelines-reviewer.md:47 both reference the path.

Lines: 59 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 6: Repair `backend-dev-guidelines/resources/architecture-overview.md` (class S + V)

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/architecture-overview.md` (57 lines)

**Canonical sources:** `packages/shared-go/server/handler.go:11-45` · `apps/fleet-service/cmd/main.go` · `go.work` · `apps/fleet-service/internal/vehiclemedia/` · `apps/fleet-service/internal/dashboard/` · `apps/fleet-service/internal/vehicle/resource.go:29-31`

**Interfaces:**
- Consumes: `InitializeRoutes` shape (Task 3), processor collaborators (Task 5).
- Produces: the startup snippet other files reference.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| A1 | 9 | "strict layered design with functional composition and immutability" | **fix** | Drop "functional composition"; the rest holds. |
| A2 | 11-15 | Four layers: domain / infrastructure / transport / application | **keep** | Accurate. |
| A3 | 18 | "Go 1.24+" | **fix** | `go.work` declares `go 1.25.0`; CI pins `go-version: '1.25'`. |
| A4 | 19-22 | GORM/PostgreSQL, JSON:API, logrus, OpenTelemetry | **keep** | Verify OpenTelemetry against `packages/shared-go/telemetry` before keeping; it is real (`CorrelationIDFromContext` is used at `vehicle/resource.go:82`). Add chi, which is the router and goes unmentioned. |
| A5 | 23 | "Functional programming with curried functions" | **delete** | PRD FR-3.2. Nothing in the tree is curried. |
| A6 | 27 | "Immutability: models never mutate" | **keep** | Accurate. |
| A7 | 28 | "Separation of Concerns" | **keep** | Accurate. |
| A8 | 29 | "Functional Composition: use `Provider`, `Map`, `ParallelMap` for chaining" | **delete** | `Map` and `ParallelMap` have 0 hits. PRD FR-3.2. |
| A9 | 30 | "Stateless Services: all state in database" | **keep** | Accurate. |
| A10 | 32-40 | §Sub-Domain / Action-Event Packages — layer separation, fold-if-simple, prefer fewer packages | **keep** (structure) / **V** (vocabulary) | The structural rules are accurate; only the examples change. `bucketarchive`/`policyrevoke`/`replicationtrigger` → `vehiclemedia`/`mileage`/`purge`. PRD FR-4.2. |
| A11 | 38 | "Sub-domain POST endpoints must use `server.RegisterInputHandler[T]`, not `server.RegisterHandler`" | **fix** | First half is right (`SUB-03`); `RegisterHandler` has 0 hits, so the contrast becomes "not a hand-rolled `json.NewDecoder`". |
| A12 | 42-47 | §Cross-Domain Orchestration — move to processor; read-only aggregation exception | **keep** (structure) / **V** (vocabulary) | Accurate. "creating a bucket also creates a default policy" → creating a vehicle also records activity and emits an event (`resource.go:29-31`); "dashboard summaries" is already the real analogue (`internal/dashboard`) — cite it. |
| A13 | 49-57 | §Startup Example — `database.Connect(...)`, `server.New(logger).AddRouteInitializer(domain.InitializeRoutes(db)(GetServer())).Run()` | **replace** | `Run()` and `GetServer()` do not exist and the `InitializeRoutes` arity is wrong. Rebuild from `packages/shared-go/server/handler.go` and a real `cmd/main.go`. PRD FR-1.4. |

- [ ] **Step 1: Read a real service entry point**

```bash
cat apps/fleet-service/cmd/main.go
```

Copy the actual `server.New(...).Use(...).AddRouteInitializer(...)` / `Router()` sequence and the real HTTP-serve call. Do not compose one.

- [ ] **Step 2: Apply the edits, then run the gate**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: G-04 → 0, G-07 drops, G-10 drops. No check goes up.

- [ ] **Step 3: Commit**

```bash
git add .claude/skills/backend-dev-guidelines/resources/architecture-overview.md
git commit -m "docs(task-020): correct architecture-overview.md startup path and principles

Dropped ParallelMap from Principles and curried functions from Core Technologies
(0 hits each). Rebuilt §Startup Example against packages/shared-go/server/
handler.go — New/Use/AddRouteInitializer/Router, no Run(). Go 1.24+ -> 1.25;
added chi. Re-grounded the sub-domain and cross-domain examples on vehiclemedia,
mileage, purge and dashboard; the structural rules are unchanged.

Lines: 57 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 7: Repair `backend-dev-guidelines/resources/anti-patterns.md` (class S + V)

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/anti-patterns.md` (249 lines)

**Canonical sources:** Tasks 2, 3, 5 output · `apps/fleet-service/internal/vehicle/model.go:20-27` · `packages/shared-go/database/entityguard/entityguard.go` · `apps/fleet-service/internal/dashboard/` · `packages/shared-go/server/jsonapi.go:95-116`

**Interfaces:**
- Consumes: everything Tasks 2, 3 and 5 define.
- Produces: the `db.Save` anti-pattern and the hand-rolled-error-envelope anti-pattern, both cited by Task 26's `DOM-09`.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| X1 | 12 | Business logic in handlers | **keep** | Accurate. |
| X2 | 13 | Handlers calling providers directly | **keep** | Accurate; `DOM-13`. |
| X3 | 14 | Direct entity creation in handlers (`db.Create` in `resource.go`) | **keep** | Accurate; `DOM-14`. |
| X4 | 15 | Cross-domain business logic in handlers | **keep** | Accurate; `DOM-12`. |
| X5 | 16 | Mutable public fields | **keep** | Accurate. |
| X6 | 17 | "Database logic in processors / violates functional purity" | **fix** | Rule is right, justification is wrong — processors are not pure (they orchestrate I/O through collaborators). Restate as a layering violation. |
| X7 | 18 | Missing validation | **keep** | Accurate. |
| X8 | 19 | `logrus.StandardLogger()` in handlers → use `d.Logger()` | **fix** | The ban is real; `d.Logger()` has 0 hits. Real alternative: the `log logrus.FieldLogger` parameter `InitializeRoutes` receives and closes over (`vehicle/resource.go:29`). |
| X9 | 20 | `*logrus.Logger` in processor constructors | **keep** | Accurate; `DOM-06`. |
| X10 | 21 | "`server.RegisterHandler` (GET signature) for POST/PATCH" | **replace** | `RegisterHandler` has 0 hits. Real anti-pattern: a plain `http.HandlerFunc` on a POST/PATCH route that then hand-decodes the body, instead of `server.RegisterInputHandler[T]`. |
| X11 | 22 | "Discarding Transform errors with `_`" | **replace** | `Transform` returns no error, so the row is unfalsifiable. Replaced by: hand-built error envelopes / bare `http.Error` / per-error `w.WriteHeader` ladders instead of `server.WriteError(w, err)`. This is the Tier-1 statement `DOM-09` will cite (design D2). |
| X12 | 23 | `os.Getenv()` in handlers | **keep** | Accurate; `DOM-11`. |
| X13 | 24 | "Eager provider execution … use `database.Query`/`SliceQuery` for lazy evaluation, enables composition with `model.Map`/`ParallelMap`" | **delete** | Correction C1: `database.Query` is real but immediately invoked, so it is not lazy; 15 of 19 providers do not use it; and the `model.*` composition it claims to enable has 0 hits. The row cannot be made true without banning the majority idiom. Its ID (`DOM-10`) is re-grounded in Task 26 to the Provider-interface invariant instead. |
| X14 | 25 | Global context usage | **keep** | Accurate. |
| X15 | 26 | Manual JSON:API envelope handling | **keep** | Accurate; `SUB-04`. |
| X16 | 27 | "Nested Data/Type/Attributes in requests — let api2go handle envelope" | **fix** | Rule stands; the mechanism is `RegisterInputHandler`, not api2go. |
| X17 | 28 | "Custom error response helpers — just write status codes directly" | **replace** | Now inverted: writing status codes directly *is* the anti-pattern; `server.WriteError` is the required helper. Merge into X11. |
| X18 | 29 | "jsonapi struct tags on REST models — use `GetName`/`GetID`/`SetID`" | **delete** | Both halves are fictional. Neither jsonapi struct tags nor the interface methods exist anywhere. |
| X19 | 30 | "Plain `http.HandlerFunc` for routes — use `server.RegisterHandler` for automatic tracing" | **delete** | Exactly backwards: a plain `http.HandlerFunc` on chi is the correct form for body-less routes (`vehicle/resource.go:33`). Keeping any version of this row would fail every domain. |
| X20 | 31 | Type aliases for library migrations | **keep** | Accurate; matches CLAUDE.md's "prefer straightforward moves over re-exporting type aliases". |
| X21 | 32 | Leaving dead code after refactoring | **keep** | Accurate. |
| X22 | 34 | "Always prefer pure, context-aware, curried, and testable functions" | **fix** | Drop "curried". |
| X23 | 36 | "For REST: use `server.RegisterHandler` and `RegisterInputHandler` with flat JSON:API models" | **fix** | Drop the first; keep flat models. |
| X24 | 40-82 | §Handler Logger Anti-Pattern | **fix** | Keep the section — the ban on `logrus.StandardLogger()` is real. Rewrite both examples against the closed-over `log` parameter; `HandlerDependency`/`HandlerContext`/`InputHandler[T]` have 0 hits. |
| X25 | 86-118 | §Wrong Handler Type for POST/PATCH | **replace** | Built entirely on `RegisterHandler(l)(si)` and `router.HandleFunc(...).Methods(...)`. Rebuild against chi `r.Post(...)` + `server.RegisterInputHandler`. The underlying point — a bodied route that hand-parses is wrong — is preserved. |
| X26 | 122-143 | §Transform Error Handling | **replace** | Per X11. New section: §Hand-Rolled Error Responses, showing `w.WriteHeader(500)` + `json.NewEncoder` versus `server.WriteError(w, err)`, and noting that only `WriteError` gets the 5xx redaction and the server-side log (`jsonapi.go:95-116`). |
| X27 | 147-156 | §Sub-Domain / Action-Event Packages | **keep** (structure) / **V** (vocabulary) | Rules accurate; `SUB-01`–`SUB-04` depend on them. Examples → `vehiclemedia`, `mileage`, `purge`. |
| X28 | 160-212 | §Critical Layer Violations, incl. the ✅/❌ dependency lists | **keep** (structure) / **V** (vocabulary) | Accurate and enforced by `arch_test.go` — cite it. `handleGetBucketRequest` → `handleGetVehicleRequest`; `rest.HandlerDependency` → the real closure form. |
| X29 | 214-247 | §Exception: Cross-Domain Read-Only Views with Circular Dependencies | **keep** (structure) / **V** (vocabulary) | The exception is real and `internal/dashboard` is the live analogue. `bucket`/`policy` → `vehicle`/`maintenanceschedule`. All four "requirements for using this exception" stay. |
| — | — | *(absent)* — `db.Save` zeroes every column | **add** | PRD FR-3.5, design §12 calls it "the highest-value addition in this task". Real, caught-in-production comment at `vehicle/model.go:20-27`; `entityguard` is the executable guard. |

- [ ] **Step 1: Apply the edits**

Work the table top-down. The quick-reference table at lines 10-32 loses three rows (X13, X18, X19) and gains one (`db.Save`); every deletion needs its reason in the commit body.

- [ ] **Step 2: Verify the `keep` sections survived**

```bash
git diff -U0 .claude/skills/backend-dev-guidelines/resources/anti-patterns.md \
  | grep '^-' | grep -iE 'Requirements for using this exception|Never use this exception for write|Valid dependencies|Invalid dependencies'
```

Expected: **no output**.

- [ ] **Step 3: Run the gate, then commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/backend-dev-guidelines/resources/anti-patterns.md
git commit -m "docs(task-020): re-ground anti-patterns.md; add the db.Save hazard

Replaced §Wrong Handler Type (built on server.RegisterHandler, 0 hits) and
§Transform Error Handling (Transform returns no error, so the row was
unfalsifiable) with the chi/RegisterInputHandler form and a new §Hand-Rolled
Error Responses — the Tier-1 statement DOM-09 now cites (design D2).

Deleted three quick-reference rows: lazy-providers (correction C1 — database.Query
is real but not lazy and 15 of 19 providers skip it), jsonapi-struct-tags (both
halves fictional), and plain-http.HandlerFunc-is-wrong (exactly backwards; a
plain handler on chi is correct for body-less routes and the row would fail
every domain).

Added the db.Save full-column-zeroing anti-pattern (vehicle/model.go:20-27,
guarded by packages/shared-go/database/entityguard).

Kept: §Handler Logger, §Critical Layer Violations, the cross-domain circular-
dependency exception and all four of its requirements.

Lines: 249 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 8: Repair `backend-dev-guidelines/resources/ai-guidance.md` (class S + V)

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/ai-guidance.md` (277 lines)

**Canonical sources:** Tasks 2, 3, 5, 7 output · `Makefile` · `.github/workflows/pr.yml` · `go.work`

**Interfaces:**
- Consumes: everything Tasks 2–7 define.
- Produces: the *Commonly Missed Items Checklist* — the list `backend-guidelines-reviewer.md:41` reads in Phase 0 and the closest Tier-1 mirror of the `DOM-*` table. Task 26 must keep the two in agreement.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| I1 | 9-19 | §Mandatory Implementation Workflow | **keep**, one **fix** | Accurate except "update mocks immediately when interfaces change" — there are no mock files (correction C2). Restate as "update the in-package `fake*`/`stub*` doubles in the affected `_test.go`". |
| I2 | 22 | "Respect file responsibilities" | **keep** | Accurate. |
| I3 | 23 | "Maintain immutability and functional composition" | **fix** | Drop "functional composition". |
| I4 | 24 | "Use curried functions for dependency injection" | **delete** | Nothing is curried; DI is constructor parameters and interfaces. |
| I5 | 25 | "Use `server.RegisterHandler` and `server.RegisterInputHandler`" | **fix** | Drop the first. |
| I6 | 26 | "Implement JSON:API interface methods on all REST models" | **delete** | 0 hits for `GetName`/`GetID`/`SetID`. |
| I7 | 27-29 | Verify types exist / run builds and tests / ask before new features | **keep** | Accurate and valuable — this is the discipline that would have caught the drift. |
| I8 | 31-74 | §Validation Rules + examples | **keep** (structure) / **V** (vocabulary) | The "verify before you use" discipline is exactly right. Examples reference `validation/model.go` and `operation_executor.go`, neither of which exists — replace with a real analogue: verifying a sentinel exists in `packages/shared-go/server/errors.go` before using it. |
| I9 | 76-107 | §Testing Rules + build/test workflow | **fix** | Rules keep; commands change. `cd /path/to/workspace/root && go build ./...` becomes `make build` / `make vet` / `make test` from the repo root (they cover every module in `go.work`). Drop "update all mocks". |
| I10 | 109-114 | §Common Build Failures table | **fix** | `missing method ChangeFace` is a foreign symbol. Replace with a real failure mode: adding a method to `Provider` and not updating the `fake*` double in `processor_test.go`. |
| I11 | 116-122 | §When to Run Tests | **keep** | Accurate. |
| I12 | 124-156 | §Implementation Rules / ask-before-implementing | **keep** | Accurate and matches CLAUDE.md's workflow rules. |
| I13 | 158-164 | §Migration & Refactoring Rules (no type aliases, clean up dead code) | **keep** | Accurate; matches CLAUDE.md's Code Patterns section. |
| I14 | 170 | Checklist: `builder.go` exists | **keep** | `DOM-01`. |
| I15 | 171 | Checklist: `ToEntity()` exists | **keep** | `DOM-02`. |
| I16 | 172 | Checklist: `TransformSlice` alongside `Transform` | **keep** | `DOM-05`. Add Task 3's `TransformDerived` caveat. |
| I17 | 173 | Checklist: processor accepts `logrus.FieldLogger` | **keep** | `DOM-06`. |
| I18 | 174 | Checklist: handlers pass `d.Logger()` | **fix** | `DOM-07`. → handlers use the `log` parameter `InitializeRoutes` receives, never `logrus.StandardLogger()`. |
| I19 | 175 | Checklist: POST/PATCH use `RegisterInputHandler[T]`, not `RegisterHandler` | **fix** | `DOM-08`. Drop the `RegisterHandler` contrast. |
| I20 | 176 | Checklist: Transform errors checked | **replace** | `DOM-09` per design D2 → every domain error goes through `server.WriteError`. |
| I21 | 177 | Checklist: providers use lazy evaluation | **replace** | `DOM-10` per correction C1 → provider is an interface with `NewProvider(db)` and translates `gorm.ErrRecordNotFound` to a domain sentinel. |
| I22 | 178 | Checklist: no `os.Getenv()` in handlers | **keep** | `DOM-11`. |
| I23 | 179 | Checklist: no cross-domain logic in handlers | **keep** | `DOM-12`. |
| I24 | 180 | Checklist: sub-domain packages have proper layers | **keep** | `SUB-01`/`SUB-02`. |
| I25 | 184-197 | §Generation Workflow (13 steps) | **fix** | Steps keep; step 10-11's commands become `make build` / `make test`, and step 2-7's file order gains `administrator.go`, which the list omits. |
| I26 | 199-212 | §REST Generation Specifics — `rest.go` | **replace** | Six of eight bullets reference `GetName`/`SetID`/`RestModel`/error-returning `Transform`. Rebuild against `rest.go`. |
| I27 | 214-222 | §REST Generation Specifics — `resource.go` | **replace** | `RouteInitializer`, `RegisterHandler(l)(si)`, `MarshalResponse`, `d.Logger()` — all 0 hits. |
| I28 | 224-235 | §Useful Composition — `model.Map`, `ops.SliceMap`, `ops.ParallelMap` | **delete** | 0 hits for every identifier; the snippets do not parse. |
| I29 | 240-260 | §Common Anti-Patterns — manual envelope vs flat request models | **fix** | Rule stands; `CreateRequest` with `uuid.UUID` → `createAttributes`. |
| I30 | 262-277 | §Manual HTTP Handler Registration vs `server.RegisterHandler` | **delete** | Same defect as X19: it forbids the correct pattern. `router.HandleFunc` is gorilla; the repo uses `r.Get`/`r.Post` on chi. |

- [ ] **Step 1: Verify the build commands before writing them**

```bash
grep -nE '^(build|vet|test|ci|lint-check):' Makefile
```

Cite `make` targets, not raw `go` invocations — `make build` iterates `go.work`'s modules, which a bare `go build ./...` at the root does not.

- [ ] **Step 2: Apply the edits, run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/backend-dev-guidelines/resources/ai-guidance.md
git commit -m "docs(task-020): re-ground ai-guidance.md checklist and REST specifics

Rebuilt §REST Generation Specifics for rest.go and resource.go (RouteInitializer,
RegisterHandler, MarshalResponse, d.Logger(), GetName/SetID — 0 hits each) and
deleted §Useful Composition and §Manual HTTP Handler Registration (the latter
forbids r.Get/r.Post on chi, which is the correct pattern).

Commonly Missed Items: DOM-09 and DOM-10 rows replaced per design D2 and
correction C1; DOM-07/DOM-08 rows re-grounded. Build commands now cite make
targets, which iterate go.work. Mock-update instructions replaced with the real
in-package fake*/stub* convention (correction C2).

Kept: §Mandatory Workflow, §Validation Rules discipline, §Testing Rules,
§Implementation Rules, §Migration & Refactoring Rules.

Lines: 277 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 9: Repair `backend-dev-guidelines/resources/testing-guide.md` (class S)

Resolves design §10.3. **Disposition decided by evidence, recorded here:**

The ~90 lines describing mock directories (§Mock Implementation Pattern, §Finding Mock Files, §Mock Maintenance, §Common Mock Update Errors) are **deleted**, not rewritten. Evidence across 126 `*_test.go` files:

```
mock/ directories:        0
type ...Mock struct:      0
stretchr/testify in .go:  0   (go.sum only, transitive)
```

The real convention is unexported `fake*` / `stub*` structs declared in the `_test.go` that uses them, in the same package, asserted with stdlib `t.Fatalf` — `apps/auth-service/internal/user/processor_test.go:14,37`, `apps/fleet-service/internal/invite/processor_test.go:16,354`, `apps/notification-service/internal/consumer/consume_test.go:15,29,36`.

§Recommended Git Hooks is **deleted**: `.git/hooks/pre-commit` does not exist and this is aspirational, which FR-7.4 forbids leaving as a checklist item. The real gate is `make ci` and `.github/workflows/pr.yml`.

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/testing-guide.md` (267 lines; expect a large net reduction)

**Canonical sources:** `apps/auth-service/internal/user/processor_test.go` · `provider_test.go` · `apps/fleet-service/internal/vehicle/{processor_test.go,rest_test.go,administrator_db_test.go}` · `apps/fleet-service/internal/admin/arch_test.go` · `apps/auth-service/internal/arch/arch_test.go` · `Makefile` · `.github/workflows/pr.yml`

**Interfaces:**
- Consumes: `Provider` / `Administrator` interfaces (Task 2) — these are the seams the doubles replace.
- Produces: the test conventions `DOM-19` cites.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| T1 | 9-27 | §Test DB Setup — in-memory SQLite + `AutoMigrate` | **keep**, one **fix** | The pattern is real (`apps/notification-service/internal/preferences/migration_test.go:17` and 4 more). The `require.NoError` calls are not — replace with `if err != nil { t.Fatalf(...) }`. |
| T2 | 31-36 | §Focus Areas: builders, processors, providers, REST | **keep** | Accurate; matches the real test-file distribution. |
| T3 | 39 | "Prefer table-driven tests" | **keep** | `DOM-19`. |
| T4 | 40 | "Mock DB providers" | **fix** | → substitute the `Provider`/`Administrator` interfaces with in-package `fake*`/`stub*` structs. |
| T5 | 41 | "Verify span propagation" | **delete** | No test in the tree asserts on spans. FR-7.4: an uncheckable rule is dropped. |
| T6 | 44-49 | Example: `NewBuilder().SetId(0).Build()` | **replace** | No builder takes an ID. Replace with the real invariant test — `vehicle`'s `Build()` rejects an empty make/model/year and returns `server.ErrValidation` (`builder.go:26-30`). |
| T7 | 53-69 | §Interface Change Workflow checklist | **fix** | Keep the discipline; retarget "update ALL mock implementations in `mock/` directories" to the in-package doubles. `go test ./... -count=1` becomes `make test`. |
| T8 | 71-98 | §Mock Implementation Pattern — `ProcessorMock` with `*Func` fields and nil-checks | **delete** | 0 occurrences of this pattern in 126 test files. Replaced by the real convention (see the add row). |
| T9 | 100-107 | §Finding Mock Files — `{package}/mock/processor.go`, `grep -r "type.*Mock struct"` | **delete** | Both paths and the grep target do not exist. The grep returns nothing, so the instruction is actively misleading. |
| T10 | 111-143 | §Test Execution Standards / §Running Tests | **keep**, **fix** commands | The when-to-run list is good. `go test ./... -count=1` at the root does not cover `go.work`'s modules — use `make test`. Keep `-count=1` and `-race` as per-package variants. |
| T11 | 142 | `go test ./bucket/... -v -count=1` | **fix** | → `go test ./internal/vehicle/... -v -count=1` run from `apps/fleet-service`. |
| T12 | 145-154 | §Test Failure Protocol | **keep**, one **fix** | Sound. Drop step 4 "update mocks if needed". |
| T13 | 158-184 | §Mock Maintenance (why mocks stay in sync, 4 sync rules, verification commands) | **delete** | Predicated on `mock/` packages. `go test ./*/mock/... -count=1` matches nothing. The one durable idea — a double must implement the whole interface or the compiler rejects it — moves into the new section. |
| T14 | 186-199 | §Common Mock Update Errors | **delete** | Both quoted compiler messages name `CreateBucket`, a foreign symbol; the section teaches a workflow for files that do not exist. |
| T15 | 203-213 | §Pre-Commit Checklist | **keep**, two **fix** | Good list. Drop "verify all mocks are updated"; `db.WithContext(ctx)` is not used anywhere in providers (0 hits) — drop that row too. Add `make ci`. |
| T16 | 215-227 | §Recommended Git Hooks | **delete** | Aspirational; no such hook exists. FR-7.4. The real gate is `make ci` + `.github/workflows/pr.yml`. |
| T17 | 231-255 | §Common Testing Pitfalls (6 items) | **keep** 4 / **delete** 2 | Keep 1 (skipping the full suite), 2 (cached results), 5 (untested error paths), 6 (race conditions). Delete 3 (forgetting mock updates) and 4 (incomplete mocks, "especially for curried functions") — both reference the deleted convention. |
| T18 | 259-268 | §AI Coding Assistant Guidance | **keep**, one **fix** | Sound. Item 2 and 4 reference mock locations; retarget to the in-package doubles. |
| — | — | *(absent)* — the real test-double convention | **add** | Replaces T8/T9/T13/T14. Copy `fakeProvider`/`fakeAdmin` from `apps/auth-service/internal/user/processor_test.go:14-40` verbatim, including its comment about what a map-keyed fake cannot express — that comment is a real lesson from the `/auth/me` login-loop bug. |
| — | — | *(absent)* — DB-backed tests alongside fakes | **add** | `provider_test.go` / `administrator_db_test.go` exist precisely because fakes cannot express column-level behaviour. The `user/processor_test.go:9-12` comment says so. This is the counterweight to over-faking. |
| — | — | *(absent)* — `arch_test.go` as executable enforcement | **add** | Design §5: the cheapest available anti-drift measure. Cite `apps/fleet-service/internal/admin/arch_test.go` and `apps/auth-service/internal/arch/arch_test.go`. |
| — | — | *(absent)* — no assertion library | **add** | Correction C2. State it explicitly so nobody adds testify to match the guideline. |

- [ ] **Step 1: Re-confirm the evidence before deleting 90 lines**

```bash
echo "mock dirs:   $(find apps packages -type d -name mock | wc -l)"
echo "Mock structs:$(grep -rn 'type .*[Mm]ock.* struct' apps packages --include='*.go' | wc -l)"
echo "testify:     $(grep -rn 'stretchr/testify' apps packages --include='*.go' | wc -l)"
echo "fake/stub:   $(grep -rnE 'type (fake|stub)' apps packages --include='*_test.go' | wc -l)"
echo "test files:  $(find apps packages -name '*_test.go' | wc -l)"
```

Expected: `0`, `0`, `0`, ≥20, 126. If any of the first three is non-zero the disposition changes — stop and re-decide.

- [ ] **Step 2: Apply the edits, run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/backend-dev-guidelines/resources/testing-guide.md
git commit -m "docs(task-020): replace the mock-directory convention with the real test doubles

Deleted ~90 lines across §Mock Implementation Pattern, §Finding Mock Files,
§Mock Maintenance and §Common Mock Update Errors. Evidence over 126 *_test.go
files: 0 mock/ directories, 0 'type ...Mock struct', 0 stretchr/testify in any
.go source. The documented grep and the documented paths both match nothing.

Replaced with the real convention — unexported fake*/stub* structs declared in
the _test.go that uses them, same package, stdlib t.Fatalf assertions
(auth-service/internal/user/processor_test.go:14,37). Added the DB-backed
provider_test.go counterweight and arch_test.go as executable enforcement.

Deleted §Recommended Git Hooks (no .git/hooks/pre-commit exists; FR-7.4 forbids
aspirational checklist items) and the span-propagation rule (nothing asserts on
spans). Commands retargeted to make test / make ci, which cover go.work.

Lines: 267 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 10: Repair `backend-dev-guidelines/resources/scaffolding-checklist.md` (class S)

Resolves design §10.2. **Disposition decided by evidence, recorded here:**

- **§4 Docker Compose — delete.** No `docker-compose*` file exists anywhere in the tree. There is no `deploy/compose/` directory.
- **§5 Nginx Routing — delete and replace.** The only `nginx.conf` is `apps/web/nginx.conf`, which serves the SPA inside the web container; it does no API routing. API routing is Traefik `IngressRoute` objects under `deploy/k8s/overlays/main/ingressroute.yaml` and `deploy/k8s/infra-local/ingressroute.yaml`. Replace §5 with the real kustomize requirement.
- **§6 Bruno Collection — delete.** `find . -iname '*bruno*'` returns nothing.

Deleting three sections of a checklist is the biggest single call in this task; the evidence commands are in Step 1 and must be re-run before the deletion, not after.

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md` (152 lines; expect a large net reduction)

**Canonical sources:** `go.work` · `apps/fleet-service/` (layout) · `apps/fleet-service/Dockerfile` · `deploy/k8s/base/kustomization.yaml` · `deploy/k8s/base/fleet-service/` · `deploy/k8s/overlays/{local,main}` · `.github/workflows/pr.yml` · `Makefile` · `apps/auth-service/internal/user/entity.go:42`

**Interfaces:**
- Consumes: the file-role model (Task 4), the checklist rows mirrored from Task 8.
- Produces: nothing other tasks consume.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| S1 | 6, 10 | Directory `services/<service-name>/` | **fix** | PRD FR-1.1 → `apps/<service-name>/`. |
| S2 | 11-25 | Required structure incl. `migrations/` | **fix** | `go.mod`, `cmd/main.go`, `internal/<domain>/`, `Dockerfile` are all real. **`migrations/` does not exist in any service** — migrations are `func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }` in each domain's `entity.go` (`apps/auth-service/internal/user/entity.go:42`). Remove the directory, add the `Migration` function to the per-domain file list. |
| S3 | 21 | `administrator.go` in the domain file list | **keep** | Accurate and matches Task 4. |
| S4 | 28 | "Add the module to `go.work` at the repo root" | **keep** | Accurate — `go.work` lists all six modules. |
| S5 | 30-31 | K8s manifest at `deploy/k8s/<service-name>.yaml`, "two resources: Deployment + Service" | **replace** | Real layout is `deploy/k8s/base/<service-name>/{configmap,deployment,service}.yaml` — three files, and the service must also be added to `deploy/k8s/base/kustomization.yaml`'s `resources:` list or it is silently not deployed. |
| S6 | 34-82 | Inline Deployment + Service YAML with `db-credentials` secretKeyRef | **replace** | Copy the shape from `deploy/k8s/base/fleet-service/` instead of restating a foreign one. Note the `main` overlay's constraints from CLAUDE.md: no PVCs, no Secrets, no ClusterRole, no placeholders. |
| S7 | 84-85 | Dockerfile at `services/<service-name>/Dockerfile` | **fix** | → `apps/<service-name>/Dockerfile`. |
| S8 | 87-91 | Multi-stage build, `golang:1.24-alpine`, alpine runtime, binary `/server`, port 8080 | **fix** | Verify each against a real `apps/*/Dockerfile` and correct what differs — particularly the Go version (the tree is on 1.25). **Add** the build-context rule from CLAUDE.md: `docker build -f apps/<service>/Dockerfile .` from the repo root, for every service including `apps/web`. |
| S9 | 93-100 | §4 Docker Compose Entry | **delete** | No compose file and no `deploy/compose/` exists. |
| S10 | 102-110 | §5 Nginx Routing | **replace** | Replaced by: add the service's routes to the Traefik `IngressRoute` in `deploy/k8s/overlays/main/ingressroute.yaml` **and** `deploy/k8s/infra-local/ingressroute.yaml`. Cite `.github/workflows/pr.yml`'s `manifests` job comment: the `:80` and `:443` IngressRoutes must carry identical route sets, because the set includes the internal-deny rule guarding fleet-service's unauthenticated `/internal/*` endpoints — drift between them is a security hole. |
| S11 | 112-121 | §6 Bruno Collection | **delete** | No Bruno collection exists. |
| S12 | 123-126 | §7 CI Configuration — `.github/workflows/` build/test steps and `scripts/build-<service-name>.sh` | **fix** | `.github/workflows/` is right but needs specifics: add the service to the `containers` job's `strategy.matrix.service` list in **both** `pr.yml` and `main.yml`, or its image is never built. **`scripts/build-<service>.sh` does not exist** — `scripts/` holds only `dev-up.sh` and `gen-jwt-key.sh`. Delete that bullet. |
| S13 | 128-131 | §8 Post-Scaffold Verification — dispatch `backend-guidelines-reviewer` or `/audit-plan` | **keep** | Accurate and matches CLAUDE.md's "Code Review Before PR". |
| S14 | 139 | Compliance row: `builder.go` exists | **keep** | `DOM-01`. |
| S15 | 140 | Compliance row: `ToEntity()` | **keep** | `DOM-02`. |
| S16 | 141 | Compliance row: `TransformSlice` | **keep** | `DOM-05`; add the `TransformDerived` caveat. |
| S17 | 142 | Compliance row: `logrus.FieldLogger` | **keep** | `DOM-06`. |
| S18 | 143 | Compliance row: `d.Logger()` in handlers | **fix** | `DOM-07` → the `log` parameter `InitializeRoutes` receives. |
| S19 | 144 | Compliance row: `RegisterInputHandler[T]`, not `RegisterHandler` | **fix** | `DOM-08`; drop the `RegisterHandler` contrast. |
| S20 | 145 | Compliance row: Transform error handling | **replace** | `DOM-09` per design D2 → route errors through `server.WriteError`. |
| S21 | 146 | Compliance row: lazy providers via `database.Query`/`SliceQuery`, not `FixedProvider` | **replace** | `DOM-10` per correction C1 → `Provider` interface + `NewProvider(db)` + `ErrNotFound` translation. |
| S22 | 147 | Compliance row: no `os.Getenv()` in handlers | **keep** | `DOM-11`. |
| S23 | 149-152 | §Database Notes — own schema, UUID PKs generated in-app, migrations on startup | **keep** | All three accurate (`builder.go:13` uses `uuid.NewString()`; `Migration` runs at startup). |
| — | — | *(absent)* — render the overlays after adding manifests | **add** | CLAUDE.md requires `kustomize build deploy/k8s/overlays/{local,main}` plus both server dry-runs, and records that skipping the *local* dry-run let a missing `namespace:` reach ten reviews. `make manifests` is the local entry point. |

- [ ] **Step 1: Re-confirm the three deletions**

```bash
ls docker-compose* 2>/dev/null || echo "no compose file"
ls deploy/compose 2>/dev/null || echo "no deploy/compose"
find . -iname '*bruno*' -not -path './node_modules/*' | head || true
ls scripts/
grep -rn "nginx" deploy/k8s/ | head
```

Expected: no compose file, no `deploy/compose`, no bruno hits, `scripts/` contains only `dev-up.sh` and `gen-jwt-key.sh`, and the only nginx reference is `deploy/k8s/base/web/` (the SPA container). If any expectation fails, stop and re-decide the disposition.

- [ ] **Step 2: Read the real deploy shape before rewriting §2 and §5**

```bash
ls deploy/k8s/base/fleet-service/
sed -n '1,40p' deploy/k8s/base/kustomization.yaml
grep -n "service:" -A6 .github/workflows/pr.yml | head -20
```

- [ ] **Step 3: Apply the edits, run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: G-08 → 0, G-23 → 0.

```bash
git add .claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md
git commit -m "docs(task-020): rebuild scaffolding-checklist.md for the real deploy layout

services/<svc>/ -> apps/<svc>/ (3 sites). Removed migrations/ from the required
structure — no service has one; migrations are Migration(db) + AutoMigrate in
each domain's entity.go.

Deleted §4 Docker Compose (no compose file, no deploy/compose/ in the tree) and
§6 Bruno Collection (no bruno collection). Replaced §5 Nginx Routing with the
real Traefik IngressRoute requirement, including pr.yml's rule that the :80 and
:443 route sets must stay identical because the set carries the internal-deny
rule for fleet-service's /internal/* endpoints.

§2 now points at deploy/k8s/base/<svc>/{configmap,deployment,service}.yaml and
the base kustomization resources list. §7 CI drops the nonexistent
scripts/build-<svc>.sh and names the containers-job matrix in pr.yml and main.yml.
Added the overlay render + both server dry-runs per CLAUDE.md.

Compliance rows DOM-07/08/09/10 re-grounded to match ai-guidance.md.

Lines: 152 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 11: `backend-dev-guidelines/SKILL.md` — frontmatter rename and content (class M + S)

Isolated so that if activation breaks, the revert is surgical (design §8 step 4, PRD FR-5.2).

**Files:**
- Modify: `.claude/skills/backend-dev-guidelines/SKILL.md` (121 lines)

**Canonical sources:** `.claude/skills/skill-rules.json` · `Makefile` · Tasks 2-10 output

**Interfaces:**
- Consumes: every backend resource file, via the Navigation Guide table.
- Produces: `name: backend-dev-guidelines`, which `skill-rules.json` already keys by and which the `skill-activation-prompt` hook resolves.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| K1 | 2 | `name: golang-microservice` | **fix** | PRD FR-5.1. The directory is `backend-dev-guidelines` and `skill-rules.json` keys it by directory name; the two disagree today. |
| K2 | 3 | `description: … DDD, immutable models, functional composition, GORM entities, and JSON:API transport` | **fix** | PRD FR-5.3. "Functional composition" describes a library that does not exist. Rewrite around: immutable models, builders, Provider/Administrator data access, chi routing and a hand-rolled JSON:API envelope. |
| K3 | 7 | H1 "Golang Microservice Skill" | **fix** | Align with the new name. |
| K4 | 9-17 | §Purpose and §When to Use | **keep**, one **fix** | Accurate. The file list omits `administrator.go`; add it. |
| K5 | 22 | Checklist: immutable Model with accessors | **keep** | Accurate. |
| K6 | 23 | Checklist: Entity with GORM tags and migrations | **keep** | Accurate. |
| K7 | 24 | Checklist: fluent Builder enforcing invariants | **keep** | Accurate. |
| K8 | 25 | Checklist: "Pure **Processor** functions" | **fix** | Processors are not pure — they orchestrate a Provider and an Administrator. Restate. |
| K9 | 26 | Checklist: "Lazy **Provider** for data access" | **replace** | Correction C1 → `Provider` interface for reads and `Administrator` for writes, both with `New*(db)` constructors. |
| K10 | 27 | Checklist: Resource file for routes and handlers | **keep** | Accurate. |
| K11 | 28 | Checklist: "**Service README** updated if API contracts changed" | **delete** | No service has a README (`ls apps/*/README.md` returns nothing). FR-7.4: verify before keeping; if one exists, keep the row and cite it. |
| K12 | 29 | Checklist: table-driven tests | **keep** | `DOM-19`. |
| K13 | 34-57 | §Standard Implementation Workflow | **keep**, two **fix** | Step 2 "update mocks immediately" → the in-package doubles (correction C2). Steps 4 and 6's `go test ./...` / `go build ./...` → `make test` / `make build`, which cover `go.work`. |
| K14 | 59-66 | §Critical Rules | **keep**, one **fix** | Drop "never update interface without updating mocks"; keep the rest. |
| K15 | 68-78 | §When Tests Fail | **keep**, one **fix** | Step 2 "check for missing mock methods" → missing methods on the in-package double. |
| K16 | 83 | Principle: Immutability | **keep** | Accurate. |
| K17 | 84 | Principle: "Functional Composition — use curried functions and providers" | **delete** | Nothing is curried. PRD FR-3.2. |
| K18 | 85 | Principle: Context Propagation "passed explicitly, not stashed in globals" | **fix** | True in spirit but the mechanism differs: `req.Context()` carries identity and correlation ID (`auth.IdentityFromContext`, `telemetry.CorrelationIDFromContext`), and processors do not hold a `ctx`. Restate against those. |
| K19 | 86 | Principle: Layer Separation | **keep** | Accurate; `arch_test.go` enforces it. |
| K20 | 87 | Principle: "Pure Logic First — business logic runs without side effects" | **fix** | Processors call administrators, which write. Restate as: business rules live in the processor, I/O is delegated to injected collaborators. |
| K21 | 94-102 | §File Responsibilities table | **keep**, two **fix** | `provider.go` "Lazy database access" → "Read-only data access". Table omits `administrator.go` — add it. |
| K22 | 109-119 | §Navigation Guide | **fix** | Update the *Functional & Builder Patterns* label to match Task 5's retitle. Verify every link target resolves. |

- [ ] **Step 1: Resolve K11 before editing**

```bash
ls apps/*/README.md 2>/dev/null || echo "no service READMEs"
```

- [ ] **Step 2: Apply the edits**

Frontmatter becomes:

```yaml
---
name: backend-dev-guidelines
description: Skill for creating and modifying MyFleet Go services — immutable domain models with fluent builders, Provider/Administrator data access over GORM, chi routing, and the hand-rolled JSON:API transport in packages/shared-go/server.
---
```

- [ ] **Step 3: Verify the rename did not break activation (PRD FR-5.2)**

Two checks, both required:

```bash
# 1. The key in skill-rules.json matches the frontmatter name and the directory.
grep -n '"backend-dev-guidelines"' .claude/skills/skill-rules.json
grep -n '^name:' .claude/skills/backend-dev-guidelines/SKILL.md
basename $(dirname .claude/skills/backend-dev-guidelines/SKILL.md)
```

All three must read `backend-dev-guidelines`.

```bash
# 2. Every Navigation Guide link resolves.
grep -oE '\(resources/[a-z-]+\.md\)' .claude/skills/backend-dev-guidelines/SKILL.md \
  | tr -d '()' | while read -r p; do
      [ -f ".claude/skills/backend-dev-guidelines/$p" ] || echo "BROKEN: $p"
    done
```

Expected: no output.

Then, in a fresh session, invoke `Skill("backend-dev-guidelines")` and confirm it loads. **If it does not load, revert this commit and record why in the audit** (PRD FR-5.2) — do not work around it. The remaining live check (that the hook fires on a Go file change) depends on Task 25 and is verified in Task 28.

- [ ] **Step 4: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/backend-dev-guidelines/SKILL.md
git commit -m "docs(task-020): rename backend skill to backend-dev-guidelines; fix quick-start

name: golang-microservice -> backend-dev-guidelines, matching the directory and
the key skill-rules.json already uses. Rewrote the description (it advertised
functional composition, which no longer describes anything in this tree).

Quick-start: 'Lazy Provider' -> Provider + Administrator interfaces; 'Pure
Processor functions' restated; dropped the Service README row (no apps/*/README.md
exists). Principles lose Functional Composition and restate Context Propagation
and Pure-Logic-First against req.Context() and injected collaborators. File
Responsibilities table gains administrator.go. Build commands -> make targets.

Isolated commit per design §8 step 4 so an activation failure reverts cleanly.

Lines: 121 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 12: Rebuild `frontend-dev-guidelines/resources/architecture-overview.md` (class R)

Structurally wrong, not merely lexically (design §3.3). It goes first among the frontend files because every other file's import examples and directory references depend on the tree and the alias question this file settles.

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/architecture-overview.md` (104 lines, full rebuild)

**Canonical sources:** `apps/web/src/` (listing) · `apps/web/package.json` · `apps/web/tsconfig.app.json` · `apps/web/vite.config.ts` · `apps/web/src/components/providers/AppProviders.tsx` · `apps/web/src/App.tsx`

**Interfaces:**
- Produces: the project tree, the config facts, and the **relative-import convention** every other frontend task follows. Tasks 13-24 and 27 cite this file rather than restating the tree.

**Rule inventory:**

| # | Line | Current claim | Reality | Disposition |
| --- | --- | --- | --- | --- |
| Q1 | 5-18 | Tech stack table (11 rows) | React, Vite, TS, React Router, Tailwind, shadcn/ui, TanStack React Query, react-hook-form, Zod, sonner, Lucide are all real | **keep** those 11 |
| Q2 | 14 | TanStack React Table | **Not a dependency** (`apps/web/package.json`) | **delete** — nothing in `src/` imports it |
| Q3 | 23 | Root is `frontend/` | `apps/web/` | **fix** |
| Q4 | 24 | `pages/` | Real: `src/pages/` with `admin/` subdir | **fix** — the tree omits the `src/` level entirely |
| Q5 | 27 | `components/common/` | **Does not exist** | **delete** — real split is `admin/`, `features/`, `frame/`, `providers/`, `ui/` |
| Q6 | 34 | `lib/breadcrumbs/` | **Does not exist** | **delete** |
| Q7 | 39 | `types/api/` | **Does not exist**; API types come from `@myfleet/shared-ts` | **delete** |
| Q8 | 41 | `tests/` | `src/test/` | **fix** |
| Q9 | 30-35 | `lib/` with `api/`, `hooks/api/`, `schemas/`, `utils.ts` | All real, but the tree omits `lib/auth/`, `lib/config/`, `lib/invites/`, `lib/admin/`, `lib/utils/` | **fix** — rebuild from the actual listing |
| Q10 | 36 | `services/api/` | Real | **keep** |
| Q11 | 40 | `context/` | Real (`AuthContext.tsx`, `ThemeContext.tsx`) | **keep** |
| Q12 | 46-60 | Layer diagram (pages → features → hooks → services → client → backend) | Accurate | **keep** |
| Q13 | 62 | "Data flows top-down. Never skip layers" | Accurate; `FE-03` depends on it | **keep** |
| Q14 | 66-77 | Provider hierarchy `BrowserRouter > QueryProvider > ThemeProvider > AuthProvider` | `BrowserRouter` (`App.tsx:100`) wraps `AppProviders`, which is `QueryClientProvider > ThemeProvider > AuthProvider`. **No `QueryProvider` component exists.** `ThemeSync` and `ThemedToaster` are undocumented | **replace** — copy the real stack from `AppProviders.tsx` |
| Q15 | 79-83 | "Order matters" rationale, incl. "AuthProvider depends on QueryProvider" | Ordering claim is right; the named component is not. `AppProviders.tsx:26-30` documents the *real* reason ThemeProvider sits above the toaster and why `ThemeSync` bridges | **fix** — keep the ordering discipline, cite the real comment |
| Q16 | 88 | `strict: true` | Real | **keep** |
| Q17 | 89 | `noUncheckedIndexedAccess: true` | Verify against `tsconfig.app.json`; keep only if set | **fix** — verify in Step 1 |
| Q18 | 90 | `exactOptionalPropertyTypes: true` | **Not set** | **delete** — FR-7.4; documenting an unset compiler flag makes agents write code for a stricter mode than the build enforces |
| Q19 | 91 | `noImplicitOverride: true` | **Not set** | **delete** — same |
| Q20 | 92, 103 | Path alias `@/*` maps to project root | **No alias is configured**, in `tsconfig.app.json` or `vite.config.ts` | **replace** — document relative imports. Design §7: this is the consequential claim, appearing in 8 of 12 resource files and in `FE-03`; every example a developer copies fails to resolve. Adding the alias is the nicer end state but is application config plus hundreds of import rewrites, which PRD §2 excludes — filed as deferred |
| Q21 | 94-99 | React Query defaults in `lib/query-client.ts`: stale 5min, gc 10min, retry 3 + backoff, `refetchOnWindowFocus: false`, mutations 1 retry | **No such file.** The client is created in `AppProviders.tsx:33-41` with `retry: 1`, `refetchOnWindowFocus: false`, and **no** staleTime/gcTime defaults | **replace** — and document the real convention: staleness is set per hook (`staleTime: 60 * 1000`, `gcTime: 5 * 60 * 1000` at `lib/hooks/api/vehicles.ts:30-31`), not globally |
| Q22 | 102-104 | Vite: React plugin, `@/*` alias, dev proxy | Plugin and proxy are real (`vite.config.ts:5-8` documents the proxy forwarding full `/api/<service>/...` paths); the alias is not | **fix** |

- [ ] **Step 1: Read the real config and tree**

```bash
ls apps/web/src apps/web/src/components apps/web/src/lib apps/web/src/types
grep -nE 'strict|noUnchecked|exactOptional|noImplicitOverride|paths' apps/web/tsconfig.app.json
grep -nE 'alias|resolve|proxy' apps/web/vite.config.ts
grep -nE '"(@tanstack|react-hook-form|zod|sonner|lucide)' apps/web/package.json
sed -n '90,110p' apps/web/src/App.tsx
```

Record which of `noUncheckedIndexedAccess` is actually set (Q17) before writing the config section.

- [ ] **Step 2: Rebuild the file**

The provider section must be the real stack, copied from `AppProviders.tsx`:

```tsx
// App.tsx wraps AppProviders in BrowserRouter (App.tsx:100)
<BrowserRouter>
  <AppProviders>          {/* components/providers/AppProviders.tsx */}
    {/* QueryClientProvider > ThemeProvider > AuthProvider > ThemeSync + children,
        with ThemedToaster as a sibling of AuthProvider inside ThemeProvider */}
  </AppProviders>
</BrowserRouter>
```

Add an explicit **Imports** subsection stating the relative-import rule, since it replaces a claim that appears in eight other files:

```markdown
### Imports

There is **no `@/*` path alias**. Neither `tsconfig.app.json` nor
`vite.config.ts` configures one. All intra-`src` imports are relative:

```ts
// apps/web/src/lib/hooks/api/vehicles.ts:2
import { vehicleService } from '../../../services/api/VehicleService';
// apps/web/src/services/api/BaseService.ts:2
import { apiClient } from '../../lib/api/client';
```

Shared code is imported by package name: `@myfleet/shared-ts`,
`@myfleet/ui-components`.
```

- [ ] **Step 3: Verify every path in the file resolves**

```bash
grep -oE '(src/)?(components|lib|pages|services|types|context|test)/[a-zA-Z/.-]*' \
  .claude/skills/frontend-dev-guidelines/resources/architecture-overview.md \
  | sort -u | while read -r p; do
      [ -e "apps/web/src/${p#src/}" ] || echo "MISSING: $p"
    done
```

Expected: no output.

- [ ] **Step 4: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/architecture-overview.md
git commit -m "docs(task-020): rebuild frontend architecture-overview.md from apps/web/src

The file was structurally wrong, not just lexically (design §3.3). Root was
frontend/ (real: apps/web); components/common/, lib/breadcrumbs/ and types/api/
do not exist; tests/ is src/test/; TanStack React Table is not a dependency;
lib/query-client.ts does not exist and its documented defaults (stale 5min, gc
10min, retry 3) are not what AppProviders.tsx:33-41 sets (retry 1, no staleTime).

Deleted the @/* path-alias claim: no alias is configured in tsconfig.app.json or
vite.config.ts. This claim propagated into 8 of 12 resource files and FE-03, so
every copied import example failed to resolve. Replaced with an explicit Imports
section documenting relative imports. Adding the alias is deferred — it is
application config plus hundreds of rewrites, which PRD §2 excludes.

Deleted exactOptionalPropertyTypes and noImplicitOverride: neither is set, and
documenting unset compiler flags makes agents write for a stricter mode than the
build enforces. Provider stack rebuilt from AppProviders.tsx including ThemeSync
and ThemedToaster.

Kept: the layer diagram and the never-skip-layers rule.

Lines: 104 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 13: Repair `frontend-dev-guidelines/resources/patterns-service-layer.md` (class S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-service-layer.md` (169 lines)

**Canonical sources:** `apps/web/src/services/api/BaseService.ts` (69 lines, whole file) · `VehicleService.ts` · `MediaService.ts` · `packages/shared-ts/src/{jsonapi.ts,apiClient.ts}` · `apps/web/src/lib/api/client.ts`

**Interfaces:**
- Consumes: relative-import rule (Task 12).
- Produces: the `BaseService` contract `FE-11` checks — abstract `resourceType` / `basePath`, `ListResult<A>`, `listAt` / `createAt`.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| V1 | 5 | "Services are **singletons** — instantiated once and exported as module-level constants" | **keep** | Accurate (`export const vehicleService = new VehicleService()`). |
| V2 | 5 | Service layer lives in `services/api/` | **keep** | Accurate — PRD FR-6.3 is explicit that the directory path is right and must not change. |
| V3 | 7-58 | §"Two Service Patterns" → 1. BaseService "for complex resources" | **replace** | There is one pattern, not two. Every concrete service extends `BaseService` (`FE-11`). The class shown (`BansService` with `validate`/`transformResponse` overrides, `api.getList`) has no counterpart: `BaseService` defines no `validate` and no `transformResponse` hook. |
| V4 | 14 | Filename `services/api/bans.service.ts` | **fix** | PRD FR-6.3 → PascalCase `VehicleService.ts`. |
| V5 | 16 | `protected basePath = '/api/bans'` | **fix** | Real: `protected readonly basePath` **and** `protected readonly resourceType`, both `abstract` on the base (`BaseService.ts:22-23`). The `resourceType` half is missing from the doc entirely and is what every write envelope's `type` comes from. |
| V6 | 19-27 | `protected override validate<T>()` | **delete** | No `validate` hook exists on `BaseService`. Validation is Zod at the form layer (`FE-13`/`FE-14`). |
| V7 | 30-42 | `protected override transformResponse<T>()` | **delete** | No such hook. Attribute coercion happens in the type layer. |
| V8 | 45-48 | `async getAllBans(options?: QueryOptions)` calling `api.getList` | **replace** | No `api.getList`; the real primitive is `apiClient.request<JsonApiDocument<...>>(path)` and the base method is `list(): Promise<ListResult<A>>`. |
| V9 | 51-54 | Private type guard `isBan` | **delete** | No service in `apps/web/src/services/api/` defines a runtime type guard; typing comes from `JsonApiResource<A>`. |
| V10 | 60-89 | §2 "Direct API Client Pattern (Simple Resources)" — a class that does not extend `BaseService` | **delete** | Contradicts `FE-11`. Every real service extends `BaseService`; nested routes are handled by the protected `listAt`/`createAt` escape hatches, not by opting out of the base. |
| V11 | 91-104 | §BaseService Template Methods table (10 rows: `getAll`, `getById`, `exists`, `create`, `update`, `patch`, `delete`, `createBatch`, `updateBatch`, `deleteBatch`) | **replace** | Seven of ten do not exist. Real surface (`BaseService.ts`): `listAt(path)` (protected), `list()`, `get(id)`, `createAt(path, attributes)` (protected), `create(attributes)`, `patch(id, attributes)`, `remove(id)`. No `exists`, no `update`, no batch methods. |
| V12 | 106-118 | §JSON:API Request Format — `{data:{type, id, attributes}}` | **keep** | Accurate; `BaseService.ts:46,60` builds exactly this. Note `create` omits `id` and `patch` includes it. |
| V13 | 120-134 | §Action endpoints (no real attributes) | **keep**, **fix** the mechanism | **High-value section — the frontend half of Task 3's R11 hazard.** The rule is right and there is a live example (`VehicleService.restore`, `VehicleService.ts:38-47`, whose comment says the route runs through `RegisterInputHandler` and parses the body first). Fix: the failure is now `422` (`server.ErrValidation`), not `400 "Could not parse request body"`; `type` comes from the service's `resourceType`, not a backend `GetName()`; and the "some endpoints use `server.RegisterHandler` and ignore the body" paragraph goes — `RegisterHandler` has 0 hits, and body-less routes are plain chi handlers that ignore the body anyway, so the practical advice ("default to sending the envelope") survives with a corrected reason. |
| V14 | 136-151 | §Update Pattern (Immutable) — merge attributes, return new object | **fix** | The immutability rule is right (`FE-07`). The example calls `this.patch<void, typeof input>(id, input)` with a two-type-parameter signature and a full envelope argument; the real `patch(id, attributes)` takes bare attributes and returns `JsonApiResource<A>`. Rewrite against `BaseService.ts:55-64`. |
| V15 | 154-169 | §Exports (index.ts) | **delete** | PRD FR-6.4: **no `services/api/index.ts` exists.** Replace with the real convention — import the concrete service module directly by relative path (`import { vehicleService } from '../../../services/api/VehicleService'`, `lib/hooks/api/vehicles.ts:2`). |
| — | — | *(absent)* — the gateway-path constraint | **add** | PRD FR-6.5. `apiClient.baseUrl` is `''` (`lib/api/client.ts:18`), so every path must be an absolute gateway path. Copy the Traefik prefix-stripping asymmetry comment verbatim from `BaseService.ts:14-20` — it is the kind of thing that is wrong silently. |
| — | — | *(absent)* — types come from `@myfleet/shared-ts` | **add** | `JsonApiDocument`, `JsonApiResource`, `PageMeta` (`BaseService.ts:1`). |
| — | — | *(absent)* — `ListResult<A>` | **add** | `{ data: Array<JsonApiResource<A>>; meta?: PageMeta }` (`BaseService.ts:4-7`) — the return type of every list call, and how pagination meta reaches the hook. |
| — | — | *(absent)* — nested routes via `listAt` / `createAt` | **add** | The reason `BaseService` has protected variants at all (`VehicleService.ts:30-36`: `listByFleet`, `createInFleet`). |

- [ ] **Step 1: Copy the real base class**

Read `apps/web/src/services/api/BaseService.ts` in full and reproduce the class declaration, the two abstract members, `ListResult<A>`, and at least `list`/`get`/`create`/`patch`/`remove` verbatim. Then show `VehicleService` as the worked subclass.

- [ ] **Step 2: Verify every documented member exists**

```bash
for m in "abstract class BaseService" "resourceType" "basePath" "ListResult" \
         "listAt" "createAt" "list()" "get(" "create(" "patch(" "remove("; do
  printf '%-28s %s\n' "$m" "$(grep -c -- "$m" apps/web/src/services/api/BaseService.ts)"
done
```

Expected: every count ≥ 1. Then confirm the deletions:

```bash
grep -nE 'validate|transformResponse|getAll|exists\(|createBatch|updateBatch|deleteBatch' \
  apps/web/src/services/api/BaseService.ts || echo "confirmed absent"
ls apps/web/src/services/api/index.ts 2>/dev/null || echo "confirmed: no index.ts"
```

- [ ] **Step 3: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-service-layer.md
git commit -m "docs(task-020): rebuild patterns-service-layer.md from BaseService.ts

Documented two service patterns; there is one — every concrete service extends
BaseService (FE-11), and the 'direct API client' alternative contradicted that
check. Seven of the ten documented template methods (getAll, getById, exists,
update, createBatch, updateBatch, deleteBatch) do not exist; the real surface is
listAt/list/get/createAt/create/patch/remove. The validate() and
transformResponse() override hooks do not exist at all.

Filenames .service.ts -> PascalCase VehicleService.ts. Deleted the
services/api/index.ts barrel section (PRD FR-6.4: no such file) in favour of
direct relative imports.

Added: abstract resourceType + basePath, ListResult<A>, @myfleet/shared-ts
types, nested-route escape hatches (listAt/createAt), and the
apiClient.baseUrl === '' absolute-gateway-path constraint with the Traefik
prefix-stripping asymmetry from BaseService.ts:14-20.

Kept: the singleton rule, services/api/ path, JSON:API envelope shape, the
immutable-update rule, and the action-endpoint envelope hazard (restated: the
failure is 422 ErrValidation, and `type` comes from the service's resourceType).

Lines: 169 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 14: Repair `frontend-dev-guidelines/resources/patterns-types.md` (class S + V)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-types.md` (233 lines)

**Canonical sources:** `packages/shared-ts/src/jsonapi.ts` (27 lines, whole file) · `packages/shared-ts/src/errors.ts` · `apps/web/src/types/models/vehicle.ts` · `apps/web/tsconfig.app.json`

**Interfaces:**
- Consumes: relative imports (Task 12), `ListResult<A>` (Task 13).
- Produces: `JsonApiResource<A, R>`, `JsonApiDocument<T>`, `PageMeta`, `JsonApiError`, `ApiError`, `createErrorFromUnknown` — the names `FE-09` and `FE-10` check.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| Y1 | 5 | "All types live in `types/` — domain models in `types/models/` and API types in `types/api/`" | **fix** | `types/models/` is real; **`types/api/` does not exist**. API types come from `@myfleet/shared-ts`. |
| Y2 | 9-29 | JSON:API model structure `{ id, attributes }` | **keep**, **fix** shape | The rule is right and `FE-10` checks it, but the real declaration is `export type Vehicle = JsonApiResource<VehicleAttributes>` — the alias comes from the shared package, and the resource carries a `type` field too (`jsonapi.ts:1-6`). The hand-written `interface Bucket { id; attributes }` form appears nowhere. |
| Y3 | 12 | `types/models/bucket.ts` | **V** | → `types/models/vehicle.ts`. |
| Y4 | 32-35 | Pattern notes: `id` is always string; attributes hold data; nested types; optional relationships | **keep** | All four accurate (`jsonapi.ts:1-6` has `relationships?: R`). |
| Y5 | 37-68 | §Enum + Label Map Pattern (`BanType`, `BanReasonCode` + label records) | **fix** or **delete** — decide in Step 1 | Numeric enums with label maps may have no analogue here; MyFleet attributes are string unions (`state: 'upcoming' \| 'overdue'`, `vehicle.ts:8`). If no numeric enum + label map exists in `apps/web/src`, delete the section under FR-7.4 and document the string-union + `as const` record convention instead. Record which way it went. |
| Y6 | 71-90 | §Helper Functions on Models (standalone functions beside the model) | **keep**, **V** | The convention is real — `apps/web/src/lib/vehicleStats.ts`, `vehicleRecords.ts`, `carfax.ts` are exactly this. Verify the location claim: the helpers live in `lib/`, not in `types/models/`. Fix the path, keep the rule. |
| Y7 | 92-120 | §API Response Types — `ApiResponse<T>`, `ApiListResponse`, `ApiSingleResponse`, `ApiErrorResponse`, `isApiErrorResponse` | **replace** | None exist. Real: `JsonApiDocument<T>` (`{ data, meta?, links? }`) and `JsonApiError` (`{ status, code, title, detail?, source? }`) — `jsonapi.ts:15-27`. The documented `{ error: { detail, status, code } }` envelope is not what the backend emits; `server.WriteError` emits `{ errors: [APIError] }` (`packages/shared-go/server/jsonapi.go:113-115`). |
| Y8 | 122-150 | §Error Type Hierarchy — `ApiError` interface plus five subtypes and type guards | **replace** | Real `ApiError` is a **class** extending `Error` (`packages/shared-ts/src/errors.ts:3`), not an interface, and there are no `NetworkError`/`ValidationError`/`AuthenticationError`/`NotFoundError`/`ServerError` subtypes and no `isNetworkError`/`isNotFoundError` guards. Replace with the real class and `createErrorFromUnknown` (`errors.ts:23`) — the function `FE-09` checks. |
| Y9 | 152-166 | §Result Pattern — `Result<T,E>`, `createSuccessResult`, `createErrorResult` | **delete** | 0 occurrences in `apps/web/src` or `packages/shared-ts`. FR-7.4. |
| Y10 | 168-183 | §Type Guard Pattern (Service Layer) — private `isBan` guard | **delete** | No service defines a runtime type guard; see Task 13 V9. |
| Y11 | 185-203 | §Update Data Types — separate partial-attribute types for update payloads | **keep**, **V** | Real and important: `CreateVehicleAttributes` / `UpdateVehicleAttributes` (`vehicle.ts:36+`) exist precisely so a create payload cannot carry a derived read-only field — the mirror of the backend's `createAttributes` narrow struct (Task 3). Cite both sides. |
| Y12 | 205-215 | §Type Re-exports — "types are re-exported from `services/api/index.ts`" with `import ... from "@/services/api"` | **delete** | PRD FR-6.4: no `index.ts`, and no `@/` alias. Both halves are wrong. Replace with the real import: `import type { Vehicle } from '../../types/models/vehicle'`. |
| Y13 | 217-228 | §TypeScript Strict Mode Features JSON block | **fix** | Keep `strict`; verify `noUncheckedIndexedAccess`; **delete `exactOptionalPropertyTypes` and `noImplicitOverride`** — neither is set (Task 12 Q18/Q19). |
| Y14 | 230-233 | Implications: check index access, `!` only after validation, `override` keyword | **fix** | Keep the first two; the `override` implication goes with `noImplicitOverride`. |
| — | — | *(absent)* — `PageMeta` and how pagination meta is typed | **add** | `jsonapi.ts:8-13`, surfaced through `ListResult<A>` (Task 13). |
| — | — | *(absent)* — the doc-comment convention on model files | **add** | `vehicle.ts:14-15` names the backend file the type mirrors and marks which attributes are derived read-only. That comment is the anti-drift mechanism for the type layer; recommend it. |

- [ ] **Step 1: Resolve Y5 and Y6 before editing**

```bash
grep -rn "^export enum" apps/web/src/types/models/ || echo "no numeric enums"
grep -rn "Labels: Record<" apps/web/src/ || echo "no label maps"
ls apps/web/src/lib/*.ts | head
```

- [ ] **Step 2: Apply the edits, verify, commit**

```bash
grep -rn "ApiResponse\|ApiListResponse\|isApiErrorResponse\|createSuccessResult\|Result<" apps/web/src packages/shared-ts/src \
  || echo "confirmed absent"
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-types.md
git commit -m "docs(task-020): re-ground patterns-types.md on @myfleet/shared-ts

Replaced §API Response Types and §Error Type Hierarchy: ApiResponse,
ApiListResponse, ApiErrorResponse and the five ApiError subtypes with their type
guards do not exist. Real types are JsonApiResource<A,R>, JsonApiDocument<T>,
PageMeta and JsonApiError from packages/shared-ts/src/jsonapi.ts, and ApiError
is a class extending Error with createErrorFromUnknown (errors.ts:3,23) — the
function FE-09 checks.

Deleted §Result Pattern and §Type Guard Pattern (0 occurrences), the
services/api/index.ts re-export section (PRD FR-6.4: no such file), and
exactOptionalPropertyTypes / noImplicitOverride (not set).

types/api/ removed — it does not exist. Kept the JSON:API model-shape rule
(FE-10), helper-functions convention (path corrected to lib/), and the
separate create/update payload types, which mirror the backend's narrow
createAttributes struct.

Lines: 233 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 15: Repair `frontend-dev-guidelines/resources/patterns-api-client.md` (class S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-api-client.md` (144 lines)

**Canonical sources:** `apps/web/src/lib/api/client.ts` (21 lines, whole file) · `packages/shared-ts/src/apiClient.ts` · `apps/web/src/lib/api/{refresh.ts,token.ts,authRoutes.ts}` · `packages/shared-ts/src/errors.ts`

**Interfaces:**
- Consumes: relative imports (Task 12).
- Produces: `apiClient`, `apiClient.request`, `getAccessToken`, `refreshAccessToken` — the names `FE-03` is re-grounded against in Task 27.

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| W1 | 5 | "`lib/api/client.ts` is a singleton `ApiClient` class providing all HTTP communication" | **keep**, **fix** | Singleton is right; the class is not defined there. `lib/api/client.ts` is 21 lines that *construct* `new ApiClient({...})` from `@myfleet/shared-ts` (`client.ts:1,17-21`). Say where the implementation lives. |
| W2 | 5 | "includes request deduplication, response caching, retry with exponential backoff, progress tracking" | **replace** | Verify each against `packages/shared-ts/src/apiClient.ts` in Step 1 and keep only what is implemented. The real distinguishing behaviour is the **401 refresh hook** (`onRefresh: refreshAccessToken`), which the file does not mention at all. |
| W3 | 9-25 | The `api` object with `getList`/`getOne`/`get`/`post`/`put`/`patch`/`delete`/`upload`/`download`/`clearCache`/`clearCacheByPattern`/`getCacheStats`/`clearPendingRequests` | **replace** | There is no `api` object with these methods. Real surface: `apiClient.request<T>(path, init?)` — that is what `BaseService` calls (`BaseService.ts:27,36,44,56,67`). Document the real method list from `apiClient.ts`. |
| W4 | 30-42 | `ApiRequestOptions` (10 fields) | **replace** | Verify field-by-field against `apiClient.ts`; document only what exists. |
| W5 | 46-71 | §Caching — `CacheOptions`, the `cache` helper object, "most hooks pass `useCache: false`" | **delete** unless verified | No caller passes `useCache`. React Query owns caching; the client layer having a parallel TTL cache would be a real design claim needing real evidence. Verify in Step 1; delete if absent. |
| W6 | 73-84 | §Request Deduplication with a three-identical-GETs example | **delete** unless verified | Same. React Query already dedupes by query key. |
| W7 | 86-93 | §Retry Logic — 3 retries, 1s→2s→4s, retryable only, no 4xx retry | **replace** | The app-level retry is React Query's `retry: 1` (`AppProviders.tsx:37`). Whatever the client does must be read from `apiClient.ts`, not assumed. The 401→refresh→replay path in `lib/api/refresh.ts` is the retry behaviour that actually matters and is undocumented. |
| W8 | 95-104 | §Cancellation — `cancellation` helper object | **delete** unless verified | 0 references in `apps/web/src`. |
| W9 | 106-125 | §Error Handling — `isRetryableError`, `requiresAuthentication`, `getErrorActions`, `transformError`, imported from `@/lib/api/errors` | **replace** | **`lib/api/errors.ts` does not exist** (`lib/api/` holds `authRoutes`, `client`, `refresh`, `token`). None of the four functions exists. Real: `createErrorFromUnknown` from `@myfleet/shared-ts` (`errors.ts:23`), which `FE-09` checks. Also fixes an `@/` import. |
| W10 | 127-143 | §Progress Tracking — `progress` helper object, `api.upload(...)` with `onProgress` | **replace** unless verified | Media upload is real (`MediaService.ts`); the documented helper object is not. Read `MediaService.ts` and document what upload actually does. |
| — | — | *(absent)* — `baseUrl: ''` and absolute gateway paths | **add** | `client.ts:8-12`. Copy the Traefik prefix-stripping comment verbatim; it is the same hazard Task 13 documents from the other side. |
| — | — | *(absent)* — the 401 refresh path | **add** | `client.ts:14-15` + `lib/api/refresh.ts`: `onRefresh` returns `null` on failure so the original request surfaces as a 401. This is the single most consequential behaviour in the file and is entirely undocumented. |
| — | — | *(absent)* — `getAccessToken` reads from localStorage | **add** | `client.ts:13`, `lib/api/token.ts`. |

- [ ] **Step 1: Read the real client before writing anything**

```bash
cat apps/web/src/lib/api/client.ts
cat packages/shared-ts/src/apiClient.ts
cat apps/web/src/lib/api/refresh.ts
ls apps/web/src/lib/api/
grep -rn "useCache\|clearCache\|onProgress\|cancellation\|isRetryableError\|transformError" apps/web/src \
  || echo "confirmed: none of these are used"
```

Every W-row marked "verify" is decided by this output. Document only what you can point at.

- [ ] **Step 2: Apply the edits, run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-api-client.md
git commit -m "docs(task-020): rebuild patterns-api-client.md from the real client

The documented api object (getList/getOne/upload/download/clearCache/...),
ApiRequestOptions, the cache/cancellation/progress helper objects and
lib/api/errors.ts with isRetryableError/requiresAuthentication/getErrorActions/
transformError do not exist. lib/api/client.ts is 21 lines constructing
new ApiClient({ baseUrl: '', getAccessToken, onRefresh }) from @myfleet/shared-ts,
and BaseService calls apiClient.request<T>(path, init?).

Added the three behaviours that actually matter and were undocumented: baseUrl
is '' so paths are absolute gateway paths (with the Traefik prefix-stripping
asymmetry), the 401 -> refreshAccessToken -> replay hook that returns null on
failure so the request surfaces as a 401, and getAccessToken reading from
localStorage. Error handling now cites createErrorFromUnknown from
@myfleet/shared-ts, which FE-09 checks.

Lines: 144 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 16: Repair `frontend-dev-guidelines/resources/patterns-react-query.md` (class V + S)

The code already cites this file — `apps/web/src/lib/hooks/api/vehicles.ts:7-14` says its key factory follows "frontend-dev-guidelines". That makes the key-factory section the one place where Tier 1 and the code already agree; keep it and cite back.

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-react-query.md` (193 lines)

**Canonical sources:** `apps/web/src/lib/hooks/api/vehicles.ts` (key factory at `:14-21`, staleness at `:30-31`, invalidation at `:52-66`) · `mileage.ts` · `media.ts` · `apps/web/src/components/providers/AppProviders.tsx:33-41`

**Interfaces:**
- Consumes: service singletons (Task 13).
- Produces: the query-key-factory contract `FE-12` checks (`as const`) and the per-hook staleness convention.

**Rule inventory:**

| # | Section | Lines | Disposition | Notes |
| --- | --- | --- | --- | --- |
| Z1 | §Overview | 3-5 | **fix** | State the real hook location: `apps/web/src/lib/hooks/api/*.ts`, one module per resource. |
| Z2 | §Query Key Factory Pattern | 7-42 | **keep** (rule) / **V** (example) | The hierarchical `all` / `lists()` / `list()` / `details()` / `detail()` shape with `as const` is exactly what the code does (`vehicles.ts:14-21`), and `FE-12` checks the `as const`. Replace `bucketKeys`/`policyKeys` with `vehicleKeys` copied verbatim, **including its comment** — the comment names the canonical test that pins the key shapes, which is executable enforcement worth citing. Filename `useBuckets.ts` → `vehicles.ts` (the real modules are resource-named, not `use`-prefixed). |
| Z3 | §Query Hook Pattern | 44-63 | **keep** (rule) / **V** + **S** (example) | Replace with `useVehicles` / `useVehicle` verbatim (`vehicles.ts:25-42`). **S:** the real hooks carry `enabled: !!fleetId` for the nullable-parameter case, which the doc omits and which is load-bearing — without it the query fires with an empty key. |
| Z4 | §Mutation Hook Pattern | 65-80 | **keep** (rule) / **V** + **S** (example) | Replace with `useCreateVehicle` / `useUpdateVehicle` (`vehicles.ts:46-67`). **S:** the real convention is `onSettled`, not `onSuccess`, for invalidation — invalidating on settle refetches after a failed mutation too, so a partially-applied write cannot leave stale cache. Document the choice. Note the `void` before `invalidateQueries` (the promise is deliberately not awaited). |
| Z5 | §Optimistic Update Pattern | 82-131 | **verify, then keep or delete** | Check whether any hook in `lib/hooks/api/` uses `onMutate` + `cancelQueries` + rollback. If none does, delete under FR-7.4 rather than documenting an unused pattern; record the decision. |
| Z6 | §Invalidation Helper Pattern | 133-154 | **verify, then fix or delete** | Same test: does a shared invalidation helper exist, or does each mutation invalidate inline? The real hooks invalidate inline (`vehicles.ts:53,64-65`). Align the doc with whichever the code does. |
| Z7 | §Prefetch Pattern | 156-171 | **verify, then keep or delete** | `grep -rn "prefetchQuery" apps/web/src` — delete if absent. |
| Z8 | §Stale Time Guidelines | 173-181 | **replace** | Task 12 Q21: there are no global defaults. Document the real per-hook convention — `staleTime: 60 * 1000`, `gcTime: 5 * 60 * 1000` (`vehicles.ts:30-31`) — and the two global defaults that *are* set (`retry: 1`, `refetchOnWindowFocus: false`, `AppProviders.tsx:36-37`). |
| Z9 | §Hook File Structure | 183-193 | **fix** | Verify against a real module's section order (`// --- Queries ---` / `// --- Mutations ---`, `vehicles.ts:23,44`). |
| — | vocabulary | throughout | **V** | 28 `bucket` + 11 `policy` → vehicle / maintenance schedule. |

- [ ] **Step 1: Resolve Z5, Z6, Z7**

```bash
grep -rln "onMutate" apps/web/src/lib/hooks/api/ || echo "no optimistic updates"
grep -rln "prefetchQuery" apps/web/src/ || echo "no prefetch"
grep -rn "invalidate" apps/web/src/lib/hooks/api/*.ts | head
```

Record each decision in the commit body.

- [ ] **Step 2: Apply the edits, run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-react-query.md
git commit -m "docs(task-020): re-ground patterns-react-query.md on lib/hooks/api/vehicles.ts

Key factory, query hooks and mutation hooks replaced with the real code, which
already cites these guidelines (vehicles.ts:7-14) — including the comment naming
the test that pins the key shapes.

Corrected: hooks live at lib/hooks/api/<resource>.ts (not useBuckets.ts);
staleness is per hook (staleTime 60s, gcTime 5min) not global — there is no
lib/query-client.ts and AppProviders.tsx sets only retry:1 and
refetchOnWindowFocus:false; invalidation is onSettled, not onSuccess, so a
failed mutation still refetches; nullable query parameters need enabled:!!x.

Lines: 193 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 17: Repair `frontend-dev-guidelines/resources/testing-guide.md` (class S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/testing-guide.md` (243 lines)

**Canonical sources:** `apps/web/src/test/renderWithProviders.tsx` (45 lines, whole file) · `apps/web/src/lib/hooks/api/mileage.test.ts` · `apps/web/src/components/PageHeader.test.tsx` · `apps/web/src/test/{setup.ts,conventions.test.ts}` · `apps/web/package.json` · `apps/web/tsconfig.app.json` · `Makefile`

**Interfaces:**
- Consumes: hooks and services (Tasks 13, 16).
- Produces: the test conventions `FE-16` checks, and the `vi.mock` convention that replaces `FE-17`'s `__mocks__` check in Task 27.

**Rule inventory:**

| # | Section | Lines | Disposition | Notes |
| --- | --- | --- | --- | --- |
| J1 | §Overview: "Jest 30 with React Testing Library and jsdom" | 5 | **fix** | PRD FR-6.1 → **Vitest** (`apps/web/package.json:8`, `"test": "vitest run"`). RTL and jsdom stay. |
| J2 | §Overview: "tests live alongside source in `__tests__/` directories or as `*.test.{ts,tsx}`" | 5 | **fix** | No `__tests__/` directory exists; every test is a sibling `*.test.ts(x)`. |
| J3 | §Test Configuration `jest.config.js` | 7-17 | **replace** | Replace with the real Vitest config in `apps/web/vite.config.ts` and the `setupFiles` entry pointing at `src/test/setup.ts`. |
| J4 | §Test Structure tree | 19-41 | **V** + **fix** | Rebuild from the real layout — tests sit beside their subject (`components/PageHeader.test.tsx`, `lib/hooks/api/mileage.test.ts`, `pages/VehiclesPage.test.tsx`). |
| J5 | §Unit Test Pattern | 43-75 | **keep** (rule) / **V** (example) | Pure-function tests are real (`lib/vehicleStats.test.ts`, `lib/carfax.test.ts`). |
| J6 | §Unit Test Pattern: `jest.fn()` | 92 | **fix** | → `vi.fn()`. |
| J7 | §Component Test Pattern | 77-105 | **fix** | Rebuild against `components/PageHeader.test.tsx` and **`renderWithProviders`** (`src/test/renderWithProviders.tsx`), which the guide does not mention at all. That helper is the project's actual entry point for rendering anything inside React Query + a router; a test that calls bare `render()` on a component using a hook will fail. |
| J8 | §Integration Test Pattern | 106-167 | **fix** | `jest.mock('@/services/api', ...)` → `vi.mock('../../../services/api/VehicleService', ...)`: both the function and the specifier change, since there is no `@/` alias and no `services/api/index.ts` barrel to mock. |
| J9 | §Common Mocks — React Router | 171-179 | **fix** | `jest.mock('react-router-dom')` → `vi.mock`. Note `renderWithProviders`' `route` option as the preferred alternative to mocking the router at all (`renderWithProviders.tsx:22,34`). |
| J10 | §Common Mocks — Toast (sonner) | 181-186 | **fix** | → `vi.mock`. |
| J11 | §Common Mocks — Service Modules | 188-197 | **fix** | → `vi.mock` with a relative specifier. |
| J12 | §Common Mocks — React Query Wrapper | 199-212 | **replace** | Replace the hand-rolled wrapper with `createTestQueryClient()` / `renderWithProviders()`, and keep the real reason from `renderWithProviders.tsx:7-9`: retries are off so a test asserting an error state reaches it on the first rejection instead of waiting out the retry policy. |
| J13 | §Testing Rules | 214-223 | **keep**, verify | Sound in principle; check each rule against the real suite before keeping. |
| J14 | §Running Tests | 225-236 | **replace** | `npm test` / watch mode / coverage → `make fe-test` (which covers `apps/web` **plus** `packages/shared-ts` and `packages/ui-components` — `.github/workflows/pr.yml` documents why running only the web workspace left `fetchAuthenticated` ungated) and `make fe-build`. Note the nvm line from CLAUDE.md. |
| J15 | §Pre-Commit Checklist | 238-243 | **fix** | Retarget to `make fe-test` / `make fe-build` / `make ci`. |
| — | `@testing-library/jest-dom` | — | **keep** | PRD FR-6.1 is explicit: this is a real matcher library (listed in `tsconfig.app.json` types) and its import is **not** drift. Do not blanket-replace the string `jest`. |
| — | *(absent)* — `src/test/conventions.test.ts` | **add** | Executable guideline enforcement (design §5) — a test that fails when a documented convention is silently removed. Cite it as the model for pinning a convention that types cannot express. |

- [ ] **Step 1: Apply the edits**

Replace `jest.` → `vi.` **only** where it is the test API. Then confirm the one legitimate survivor:

```bash
grep -rn "jest" .claude/skills/frontend-dev-guidelines/resources/testing-guide.md
```

Expected: only `@testing-library/jest-dom` lines remain.

- [ ] **Step 2: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: G-09 drops sharply (the frontend `anti-patterns.md` share remains for Task 21).

```bash
git add .claude/skills/frontend-dev-guidelines/resources/testing-guide.md
git commit -m "docs(task-020): convert frontend testing-guide.md to Vitest

Jest -> Vitest (apps/web runs 'vitest run'); jest.mock/jest.fn -> vi.mock/vi.fn.
@testing-library/jest-dom left alone — it is a real matcher library, not drift
(PRD FR-6.1). jest.config.js replaced with the real Vitest config in
vite.config.ts + src/test/setup.ts. __tests__/ directories removed; every test
is a sibling *.test.ts(x).

Rebuilt the component and integration patterns around
src/test/renderWithProviders.tsx, which the guide never mentioned and which is
the project's actual entry point for rendering anything using a hook — including
its documented reason for retry:false. Mock specifiers are now relative: there
is no @/ alias and no services/api/index.ts barrel to mock.

Commands -> make fe-test / make fe-build, which also cover packages/shared-ts
and ui-components. Added src/test/conventions.test.ts as executable enforcement.

Lines: 243 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 18: Repair `frontend-dev-guidelines/resources/patterns-forms-validation.md` (class V + S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md` (308 lines)

**Canonical sources:** `apps/web/src/lib/schemas/vehicle.ts` · `maintenanceSchedule.ts` · `fuel.ts` · the dialog that consumes `vehicle.ts` (find it in Step 1) · `apps/web/src/components/ui/form.tsx`

**Interfaces:**
- Consumes: relative imports (Task 12), service singletons (Task 13), mutation hooks (Task 16), `createErrorFromUnknown` (Task 14).
- Produces: the schema-location and `zodResolver` conventions `FE-13` and `FE-14` check. Those two checks depend on this file, so it must be right before Task 27.

**Rule inventory:**

| # | Section | Lines | Disposition | Notes |
| --- | --- | --- | --- | --- |
| B1 | §Overview | 3-5 | **fix** | State the real location: `apps/web/src/lib/schemas/<resource>.ts` — **no `.schema.` infix**. |
| B2 | §Basic Schema | 9-38 | **keep** (rule) / **V** + **fix** (example) | The rule — schema in `lib/schemas/`, paired `z.infer` export — is exactly `FE-14`. Filename `lib/schemas/bucket.schema.ts` → `lib/schemas/vehicle.ts`. Replace the body with the real schema. |
| B3 | defaults export (`createBucketDefaults`) | 33 | **verify** | Check whether real schema modules export a defaults object. Keep only if they do; otherwise delete under FR-7.4. |
| B4 | §Schema with Cross-Field Validation | 40-66 | **keep** (rule) / **V** (example) | `.refine()` / `.superRefine()` for cross-field rules is a real and important pattern — `FE-04` explicitly exempts `.refine()` from the no-inline-schema ban. Re-ground on a real schema (e.g. mileage or maintenance-schedule interval rules). |
| B5 | §Discriminated Union Schema | 68-88 | **verify, then keep or delete** | `grep -rn "discriminatedUnion" apps/web/src` — delete if absent rather than documenting an unused pattern. |
| B6 | §Reusable Validation Primitives | 90-104 | **verify** | Keep only if `lib/schemas/` actually shares primitives across modules. |
| B7 | §Form Component Pattern (shadcn/ui `Form`) | 106-238 | **keep** (rule) / **V** + **S** (example) | `useForm({ resolver: zodResolver(schema) })` is `FE-13` and is real. **S:** line 113 imports from `@/components/ui/form` — no alias; make it relative. Rebuild the example around a real dialog and a real mutation hook (`useCreateVehicle`), not `bansService.createBan`. |
| B8 | §Using `register()` (Simpler forms) | 239-259 | **verify, then keep or delete** | Check whether any real form uses bare `register()` rather than shadcn `FormField`. |
| B9 | §Cascading Dropdown Pattern | 260-284 | **verify, then keep or delete** | Real analogue would be maintenance category → schedule. Delete if nothing matches. |
| B10 | §Error Display Pattern | 285-303 | **fix** | `PolicyCreationError` (line 297) is a foreign symbol. Real error surface is `createErrorFromUnknown` + `toast.error` (`FE-09`). |
| B11 | §Dialog Close Behavior | 304-308 | **verify, then keep** | Check against a real dialog's `onOpenChange` handling. |
| — | vocabulary | throughout | **V** | 11 `ban` + 6 `bucket` + 5 `policy` → vehicle / maintenance schedule / invite. |

- [ ] **Step 1: Find the real forms and resolve B3, B5, B6, B8, B9, B11**

```bash
cat apps/web/src/lib/schemas/vehicle.ts
grep -rln "zodResolver" apps/web/src/components apps/web/src/pages
grep -rn "discriminatedUnion\|superRefine\|\.refine(" apps/web/src/lib/schemas/
grep -rn "register(" apps/web/src/components | head
```

Record each verify-decision in the commit body.

- [ ] **Step 2: Apply the edits, verify no alias survives, commit**

```bash
grep -n "from '@/\|from \"@/" .claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md \
  || echo "no alias imports"
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-forms-validation.md
git commit -m "docs(task-020): re-ground patterns-forms-validation.md on lib/schemas

Schema paths lose the .schema. infix (real: lib/schemas/vehicle.ts). Examples
re-grounded on real schemas and a real dialog + mutation hook. Fixed the @/
import (no alias exists). Error display now uses createErrorFromUnknown + toast
instead of the foreign PolicyCreationError.

Kept the two rules FE-13 and FE-14 depend on: useForm + zodResolver, and every
z.object paired with an exported z.infer type. Kept .refine()/.superRefine() for
cross-field validation, which FE-04 explicitly exempts from the inline-schema ban.

Lines: 308 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 19: Repair `frontend-dev-guidelines/resources/patterns-components.md` (class V + S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-components.md` (334 lines)

**Canonical sources:** `apps/web/src/components/` (listing: `admin/`, `features/`, `frame/`, `providers/`, `ui/`) · `apps/web/src/components/features/vehicles/` · `PageHeader.tsx` · `apps/web/src/lib/utils.ts`

**Interfaces:**
- Consumes: relative imports (Task 12), `cn()` (Task 23).
- Produces: the component taxonomy `FE-08` and `FE-15` reference.

**Rule inventory:**

| # | Section | Lines | Disposition | Notes |
| --- | --- | --- | --- | --- |
| C1 | §Component Organization tree | 3-20 | **replace** | Documents `components/common/` and feature dirs `buckets/`, `policies/`. Real split is `admin/`, `features/`, `frame/`, `providers/`, `ui/`; **`common/` does not exist**, and `frame/` (the app shell) is undocumented. Rebuild from the listing. |
| C2 | §Presentational Components (`ui/`, `common/`) | 22-55 | **fix** | Drop `common/`. `ui/` (shadcn primitives) is real; the second tier is top-level `components/*.tsx` (`PageHeader.tsx`, `BrandMark.tsx`, `ThemeToggle.tsx`). |
| C3 | §Feature Components (`features/`) | 56-92 | **keep** (rule) / **V** (example) | Real: `features/{activity,dashboard,notifications,settings,vehicles}/`. `CreateBanDialog` → a real dialog under `features/vehicles/`. |
| C4 | §Component Structure Convention | 93-122 | **keep**, verify | Check the documented ordering (props interface, then component, then subcomponents) against 2-3 real files. |
| C5 | §Data Table Pattern | 123-181 | **delete** | **No `DataTable` component exists** and TanStack React Table is not a dependency. 59 lines documenting a component nobody can import. FR-7.4. If a real list rendering pattern should be documented, point at a real list page instead. |
| C6 | §Dialog Pattern | 182-205 | **keep** (rule) / **V** (example) | shadcn `Dialog` is real. Re-ground on a real dialog. |
| C7 | §Loading State Pattern | 206-243 | **keep** (rule) / **V** (example) | Skeleton-not-spinner is `FE-05` and is real. `BucketPageSkeleton` → a real skeleton. |
| C8 | §Badge Pattern | 244-258 | **keep**, verify | Verify against a real badge usage (vehicle status). |
| C9 | §Collapsible Section Pattern | 259-274 | **verify, then keep or delete** | `grep -rn "Collapsible" apps/web/src` — delete if absent. |
| C10 | §Text Casing Rules | 275-309 | **keep** (rule) / **V** (examples) | Title Case for buttons and dialog titles is a real house rule and `ai-guidance.md` §12 restates it. "Create Bucket" → "Add Vehicle" etc. |
| C11 | §Cursor Behavior | 310-321 | **keep** | `FE-15` cites the styling file for this; keep them consistent (Task 23). |
| C12 | §Empty State Pattern | 322-334 | **keep**, verify | Verify against a real empty state. |
| — | `@/` imports | 98, 129 | **S** | Two alias imports → relative. |
| — | vocabulary | throughout | **V** | 10 `ban` + 8 `bucket` + 2 `policy`. |

- [ ] **Step 1: Read the real taxonomy and resolve C4, C8, C9, C12**

```bash
ls apps/web/src/components apps/web/src/components/features apps/web/src/components/frame apps/web/src/components/ui | head -40
grep -rn "Collapsible" apps/web/src | head
grep -rln "Skeleton" apps/web/src/components | head
```

- [ ] **Step 2: Apply the edits, run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-components.md
git commit -m "docs(task-020): rebuild the component taxonomy in patterns-components.md

components/common/ does not exist; the real split is admin/, features/, frame/,
providers/, ui/ — frame/ (the app shell) was undocumented entirely. Deleted
§Data Table Pattern (59 lines): there is no DataTable component and TanStack
React Table is not a dependency, so nothing in it could be imported.

Fixed two @/ imports. Re-grounded the dialog, loading-state, badge and
text-casing examples on real components.

Kept: the presentational/feature split, skeleton-not-spinner (FE-05), Title
Case rules, cursor affordance (FE-15) and named-exports-only (FE-08).

Lines: 334 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 20: Repair `frontend-dev-guidelines/resources/patterns-routing.md` (class V + S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-routing.md` (121 lines)

**Canonical sources:** `apps/web/src/App.tsx` (routes) · `apps/web/src/components/RequireAuth.tsx` · `apps/web/src/pages/` (listing) · `apps/web/src/components/frame/`

**Rule inventory:**

| # | Section | Lines | Disposition | Notes |
| --- | --- | --- | --- | --- |
| U1 | §Route Structure page list | 7-19 | **replace** | `BucketsPage`/`PoliciesPage` → the real set: `ActivityPage`, `DashboardPage`, `InviteAcceptPage`, `LoginPage`, `NotificationsPage`, `OnboardingPage`, `SettingsPage`, `VehicleDetailPage`, `VehiclesPage`, plus `pages/admin/`. |
| U2 | §Route Configuration | 20-45 | **replace** | Copy the real `<Routes>` block from `App.tsx`, including how `RequireAuth` wraps protected routes — the doc's `/app/buckets` paths are fictional. |
| U3 | §List Page Pattern | 46-80 | **replace** | The example uses `useState` + `useEffect` + a direct `bucketsService.getAll()` call. That **contradicts the layer model** (Task 12 Q13) and `FE-03`: real list pages use a React Query hook (`useVehicles`). Rebuild from `VehiclesPage.tsx`. This is a rule conflict, not just vocabulary — flag it in the commit. |
| U4 | §Detail Page Pattern | 81-93 | **replace** | Same defect; rebuild from `VehicleDetailPage.tsx` using `useVehicle` and `useParams`. |
| U5 | §Root Layout | 94-115 | **fix** | Align with `App.tsx` + `components/frame/` + `AppProviders` (Task 12 Q14). Remove any breadcrumb system claim unless `lib/breadcrumbs/` reappears — it does not exist. |
| U6 | §Navigation Patterns | 116-121 | **keep**, verify | Verify `useNavigate` / `<Link>` usage against real pages. |
| — | `@/` import | 50 | **S** | One alias import → relative. |
| — | vocabulary | throughout | **V** | 9 `bucket`. |

- [ ] **Step 1: Read the real routes**

```bash
sed -n '1,120p' apps/web/src/App.tsx
cat apps/web/src/components/RequireAuth.tsx
sed -n '1,50p' apps/web/src/pages/VehiclesPage.tsx
```

- [ ] **Step 2: Apply the edits, run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-routing.md
git commit -m "docs(task-020): rebuild patterns-routing.md from App.tsx

Route table and page list replaced with the real routes and RequireAuth wrapping.
The list- and detail-page patterns were useState + useEffect + a direct service
call, which contradicts both the layer model and FE-03 (no direct service calls
from pages — go through a React Query hook). Rebuilt from VehiclesPage.tsx and
VehicleDetailPage.tsx using useVehicles/useVehicle. Removed the breadcrumb-system
claim (lib/breadcrumbs/ does not exist) and fixed one @/ import.

Lines: 121 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 21: Repair `frontend-dev-guidelines/resources/anti-patterns.md` (class V + S)

Every row here must correspond to an `FE-*` check after Task 27, and vice versa. Task 27 depends on this file being right.

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/anti-patterns.md` (148 lines)

**Canonical sources:** Tasks 12-20 output · `apps/web/src/lib/utils.ts` · `packages/shared-ts/src/errors.ts`

**Rule inventory:**

| # | Section | Lines | Disposition | Maps to |
| --- | --- | --- | --- | --- |
| D1 | §Quick Reference table | 3-17 | **fix** | Must list exactly the anti-patterns that have an `FE-*` row. Reconcile against Task 27's final list. |
| D2 | §1 Calling API Client Directly from Components | 21-32 | **keep** (rule) / **S** (import) | `FE-03`. The `import { api } from "@/lib/api/client"` example is doubly wrong: no `@/` alias, and the export is `apiClient`, not `api`. Fix to the real relative import so `FE-03`'s grep can be re-grounded on it. |
| D3 | §2 Manual Class String Concatenation | 33-42 | **keep** | `FE-02`; `cn()` is real (`lib/utils.ts`). |
| D4 | §3 Using `any` Type | 43-57 | **keep** (rule) / **V** (example) | `FE-01`. |
| D5 | §4 Inline Schema Definition | 58-77 | **keep** (rule) / **S** (import) | `FE-04`. Line 73 imports from `@/lib/schemas/resource.schema` — fix both the alias and the `.schema.` infix. |
| D6 | §5 Spinner for Content Loading | 78-93 | **keep** | `FE-05`. |
| D7 | §6 Hardcoded Colors | 94-104 | **keep** | `FE-06`; consistent with Task 23. |
| D8 | §7 Mutating State | 105-119 | **keep** (rule) / **V** (example) | `FE-07`. |
| D9 | §8 Missing Error Handling in Async Operations | 120-139 | **keep** (rule) / **fix** | `FE-09`. `createErrorFromUnknown` is real but imported from `@myfleet/shared-ts`, not a local util — say so, because `FE-09`'s verification depends on it. |
| D10 | §9 Default Exports for Components | 140-148 | **keep** (rule) / **V** (example) | `FE-08`. |
| — | `@/` imports | 25, 29, 73 | **S** | Three alias imports → relative. |
| — | vocabulary | throughout | **V** | 12 `bucket`. |

- [ ] **Step 1: Apply the edits, then reconcile**

After editing, list this file's sections and hold the list against `frontend-guidelines-reviewer.md`'s `FE-*` rows. Every anti-pattern here needs a check there; every check there needs a rule somewhere in Tier 1. Note any mismatch — Task 27 resolves it.

- [ ] **Step 2: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: G-09 → 0 and G-12 → 0 (this is the last frontend file holding either).

```bash
git add .claude/skills/frontend-dev-guidelines/resources/anti-patterns.md
git commit -m "docs(task-020): fix imports and vocabulary in frontend anti-patterns.md

Fixed three @/ imports and the api -> apiClient export name in the FE-03 example
(the client is exported as apiClient from lib/api/client.ts). Corrected the
schema import path (no @/ alias, no .schema. infix). Noted that
createErrorFromUnknown comes from @myfleet/shared-ts, not a local util —
FE-09's verification depends on knowing that.

All nine anti-patterns kept; each maps to an FE-* row reconciled in task 27.

Lines: 148 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 22: Repair `frontend-dev-guidelines/resources/ai-guidance.md` (class S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/ai-guidance.md` (222 lines)

**Canonical sources:** Tasks 12-21 output · `Makefile` · `apps/web/package.json`

**Rule inventory:**

| # | Section | Lines | Disposition | Notes |
| --- | --- | --- | --- | --- |
| E1 | §Mandatory Implementation Workflow | 3-13 | **keep**, **fix** commands | `npm test` / `npm run build` → `make fe-test` / `make fe-build` (Task 17 J14). |
| E2 | §Core Rules 1-11 | 15-49 | **keep**, **fix** | Rules 1-11 (follow patterns, read before write, type everything, JSON:API shape, component library, `cn()`, sonner, skeleton, RHF+Zod, immutable updates, named exports) all map to live `FE-*` checks. Verify each names a real symbol. |
| E3 | §Generation Workflow Step 1: Types | 54-63 | **fix** | Point at `types/models/` and `@myfleet/shared-ts`; **`types/api/` does not exist**. |
| E4 | §Generation Workflow Step 2: Service | 64-73 | **fix** | PRD FR-6.4: **delete the `services/api/index.ts` instruction at line 72** — no such file. Replace with: create `<Resource>Service.ts` extending `BaseService`, export a singleton, import it directly. |
| E5 | §Step 3: React Query Hooks | 74-83 | **fix** | Align with Task 16 — `lib/hooks/api/<resource>.ts`, key factory, per-hook staleness, `onSettled` invalidation. |
| E6 | §Step 4: Zod Schema | 84-92 | **fix** | `lib/schemas/<resource>.ts`, no `.schema.` infix (Task 18). |
| E7 | §Step 5: Feature Components | 93-101 | **fix** | `components/features/<resource>/` (Task 19). |
| E8 | §Step 6: Pages | 102-108 | **fix** | Use hooks, not direct service calls (Task 20 U3). |
| E9 | §Step 7: Navigation | 109-113 | **fix** | Align with the real `App.tsx` routes and `components/frame/`. |
| E10 | §Step 8: Tests | 114-118 | **fix** | `renderWithProviders`, `vi.mock`, sibling `*.test.tsx` (Task 17). |
| E11 | §12 Title Case for Interactive Text | 119-121 | **keep** | Matches Task 19 C10. |
| E12 | §13 Cursor Pointer on Clickable Elements | 122-124 | **keep** | `FE-15`. |
| E13 | §14 Capitalize Enum Display Values | 125-127 | **verify** | Keep only if a real enum display map exists (Task 14 Y5 decides this). |
| E14 | §Validation Rules | 128-144 | **keep**, verify | The verify-before-you-use discipline is right; check the examples name real symbols. |
| E15 | §Adding a Column to DataTable | 147-177 | **delete** | No `DataTable` exists (Task 19 C5). 30 lines of instructions for a component that cannot be imported. |
| E16 | §Adding a Dialog with Form | 178-201 | **keep**, **fix** | Real workflow; align with Tasks 18 and 19. |
| E17 | §Adding a New Hook | 202-222 | **keep**, **fix** | Align with Task 16. |

- [ ] **Step 1: Apply the edits, verify the deletion, commit**

```bash
grep -n "index.ts\|DataTable\|types/api" .claude/skills/frontend-dev-guidelines/resources/ai-guidance.md \
  || echo "clean"
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/ai-guidance.md
git commit -m "docs(task-020): align frontend ai-guidance.md with the rewritten patterns

Deleted the services/api/index.ts barrel instruction (PRD FR-6.4: no such file)
and §Adding a Column to DataTable (no DataTable component, and TanStack React
Table is not a dependency). Removed types/api/ from the generation workflow.
Commands -> make fe-test / make fe-build. Every generation step realigned with
the rewritten pattern files.

Kept: all 11 Core Rules, the Validation Rules discipline, Title Case, cursor
affordance, and the dialog and hook workflows.

Lines: 222 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 23: Verify `frontend-dev-guidelines/resources/patterns-styling.md` (class V)

The cleanest file in either skill — zero drift markers except one import. `FE-15` cites its "Cursor affordance" section directly, so it must be verified rather than assumed.

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/resources/patterns-styling.md` (236 lines) — expect a small diff

**Canonical sources:** `apps/web/src/index.css` · `apps/web/tailwind.config.ts` · `apps/web/src/lib/utils.ts` · `apps/web/src/context/ThemeContext.tsx` · `apps/web/src/test/sidebarTokens.test.ts`

**Rule inventory (section level — class V):**

| # | Section | Lines | Disposition | Notes |
| --- | --- | --- | --- | --- |
| G1 | §cn() Utility | 7-32 | **verify, keep** | Check the signature against `lib/utils.ts`. `FE-02` depends on it. |
| G2 | §Tailwind Class Order | 33-46 | **verify, keep** | House convention; keep unless it contradicts the code. |
| G3 | §CSS Variable Theming | 47-86 | **verify, keep** | Check the token names against `index.css`. `FE-06` (no hardcoded colors) depends on the semantic tokens existing. |
| G4 | §shadcn/ui Configuration | 87-112 | **verify** | Check against `tailwind.config.ts` and any `components.json`. Correct or delete what does not match. |
| G5 | §Component Variant Pattern (CVA) | 113-144 | **verify, keep** | Confirm `class-variance-authority` is a dependency and is used. |
| G6 | §Common Layout Patterns | 145-189 | **verify, keep** | Four sub-patterns; spot-check each against a real component. |
| G7 | §Icon Usage | 190-208 | **verify, keep** | Lucide is real. |
| G8 | §Dark Mode | 209-229 | **fix** | Line 215 imports `useTheme` from `@/context/theme-context`. Two errors: no `@/` alias, and the real module is `context/ThemeContext.tsx` (PascalCase). Also mention `ThemeSync` and the pre-paint script in `index.html`, which `src/test/conventions.test.ts` pins. |
| G9 | §Cursor affordance for interactive elements | 230-236 | **verify, keep verbatim** | `FE-15` cites this section by name. Do not reword it — Task 27 must be able to keep pointing here. |

- [ ] **Step 1: Verify each section against source**

```bash
cat apps/web/src/lib/utils.ts
grep -nE '^\s*--[a-z-]+:' apps/web/src/index.css | head -30
grep -n "class-variance-authority" apps/web/package.json
ls apps/web/components.json 2>/dev/null || echo "no components.json"
grep -n "useTheme" apps/web/src/context/ThemeContext.tsx
```

Correct or delete anything that does not match. Record any section you deleted and why.

- [ ] **Step 2: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/frontend-dev-guidelines/resources/patterns-styling.md
git commit -m "docs(task-020): verify patterns-styling.md against index.css and tailwind.config

Verified the cn() signature, semantic token names, CVA usage, layout patterns and
icon conventions against source. Fixed the one drifted import in §Dark Mode:
useTheme comes from context/ThemeContext.tsx, not @/context/theme-context (no
alias, and the module is PascalCase). Noted ThemeSync and the pre-paint script
that src/test/conventions.test.ts pins.

§Cursor affordance kept verbatim — FE-15 cites it by name.

Lines: 236 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 24: Repair `frontend-dev-guidelines/SKILL.md` (class S)

**Files:**
- Modify: `.claude/skills/frontend-dev-guidelines/SKILL.md` (141 lines)

**Rule inventory:**

| # | Line | Current rule | Disposition | Reason |
| --- | --- | --- | --- | --- |
| H1 | 2 | `name: frontend-dev-guidelines` | **keep** | Already correct (PRD FR-5.1). |
| H2 | 3 | Description | **keep**, verify | Every technology named is real. Confirm React Query, RHF, Zod, Tailwind, shadcn/ui, Vite are all in `package.json`. |
| H3 | 14 | "Any file inside `frontend/`" | **fix** | → `apps/web/`. |
| H4 | 15-23 | When-to-use list | **keep**, one **fix** | Drop "Data table configurations" — no `DataTable` exists (Task 19 C5). |
| H5 | 24 | "Testing with Jest and React Testing Library" | **fix** | → Vitest (PRD FR-6.1). |
| H6 | 29 | Checklist: presentational/container split (ui/ vs features/) | **keep** | Accurate. |
| H7 | 30 | Checklist: JSON:API types (`id` + `attributes`) | **keep** | `FE-10`. |
| H8 | 31 | Checklist: "Service extends `BaseService` **or uses direct API client pattern**" | **fix** | Task 13 V10 deleted the alternative; every service extends `BaseService`. `FE-11` should not offer an escape hatch nobody uses. |
| H9 | 32 | Checklist: query key factory with `as const` | **keep** | `FE-12`. |
| H10 | 33 | Checklist: RHF + `zodResolver` | **keep** | `FE-13`. |
| H11 | 34 | Checklist: schema in `lib/schemas/` with inferred types | **keep** | `FE-14`. |
| H12 | 35 | Checklist: skeletons not spinners | **keep** | `FE-05`. |
| H13 | 36 | Checklist: `createErrorFromUnknown()` + toast | **keep**, **fix** | `FE-09`; note the `@myfleet/shared-ts` origin. |
| H14 | 37 | Checklist: Tailwind + `cn()` | **keep** | `FE-02`. |
| H15 | 38 | Checklist: "Tests written with Jest + RTL" | **fix** | → Vitest. |
| H16 | 39 | Checklist: test execution verified | **keep** | Accurate. |
| H17 | 43-65 | §Standard Implementation Workflow | **keep**, **fix** | Step 3 references `types/api/` (does not exist). Steps 6 and 8's `npm test` / `npm run build` → `make fe-test` / `make fe-build`. |
| H18 | 67-77 | §Critical Rules | **keep** | All nine map to live `FE-*` checks. |
| H19 | 79-89 | §When Tests Fail | **keep**, **fix** | "Check for missing mocks" → `vi.mock` specifiers, and note that a component using a hook needs `renderWithProviders`. |
| H20 | 93-99 | §Key Principles (5) | **keep** | All accurate. |
| H21 | 104-121 | §File Responsibilities table | **fix** | Three dead rows: `components/common/`, `lib/api/errors.ts`, `types/api/`. Add `components/frame/` and `components/admin/`. `lib/api/client.ts`'s "caching, retry, dedup" description must match whatever Task 15 established. |
| H22 | 126-139 | §Navigation Guide | **keep**, verify | Confirm all twelve link targets resolve. |

- [ ] **Step 1: Apply the edits and verify links**

```bash
grep -oE '\(resources/[a-z-]+\.md\)' .claude/skills/frontend-dev-guidelines/SKILL.md \
  | tr -d '()' | while read -r p; do
      [ -f ".claude/skills/frontend-dev-guidelines/$p" ] || echo "BROKEN: $p"
    done
```

Expected: no output.

- [ ] **Step 2: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: **G-20, G-21 and G-22 all reach 0** — this is the last frontend skill file.

```bash
git add .claude/skills/frontend-dev-guidelines/SKILL.md
git commit -m "docs(task-020): update frontend SKILL.md paths, runner and file table

frontend/ -> apps/web; Jest -> Vitest (2 sites); commands -> make fe-test /
make fe-build. Quick-start no longer offers the 'direct API client pattern'
alternative to BaseService — Task 13 established there is only one pattern and
FE-11 should not offer an escape hatch nobody uses. Dropped the data-table
bullet (no DataTable component).

File Responsibilities table: removed components/common/, lib/api/errors.ts and
types/api/ (none exist); added components/frame/ and components/admin/.

name: already correct; unchanged.

Lines: 141 -> <N> (delta <+/-N>; running total <T>)"
```

---

## Task 25: Populate `skill-rules.json` file triggers (class M)

Design D5. Not in the PRD: `skills.backend-dev-guidelines.fileTriggers.pathPatterns` is `[]`, so the backend skill has **never** auto-suggested on a Go file change — it fires on prompt keywords only. PRD §1 and CLAUDE.md both assert otherwise, and PRD §10 asks the audit to confirm the hook fires on a Go change, which cannot pass today and would not pass after a correct rename either.

This is the trigger *data*, not the `.claude/settings.json` hook wiring PRD §2 excludes. That file stays untouched.

**Files:**
- Modify: `.claude/skills/skill-rules.json`

**Interfaces:**
- Consumes: `name: backend-dev-guidelines` (Task 11). The JSON key already matches the directory; after Task 11 the frontmatter agrees too.
- Produces: the trigger data Task 28's activation check exercises.

**Rule inventory:**

| # | Path | Current | Disposition | Reason |
| --- | --- | --- | --- | --- |
| M1 | `skills.backend-dev-guidelines.fileTriggers.pathPatterns` | `[]` | **fix** | Populate: `apps/*/internal/**/*.go`, `packages/shared-go/**/*.go`, `packages/dto-go/**/*.go`. These are the paths a Go change actually lands in. |
| M2 | `skills.backend-dev-guidelines.fileTriggers.pathExclusions` | `[]` | **fix** | `**/*_test.go`, matching the frontend entry's convention of excluding tests. |
| M3 | `skills.frontend-dev-guidelines.fileTriggers.pathPatterns[0..1]` | `frontend/**/*.ts`, `frontend/**/*.tsx` | **delete** | No `frontend/` directory exists; these two patterns can never match. |
| M4 | `skills.frontend-dev-guidelines.fileTriggers.pathPatterns[2..6]` | `**/components/**/*.tsx`, `**/pages/**/*.tsx`, `**/lib/hooks/**/*.ts`, `**/lib/schemas/**/*.ts`, `**/services/api/**/*.ts` | **keep** | All five match real `apps/web/src` paths. Drift inventory §C explicitly says leave them alone. |
| M5 | `skills.frontend-dev-guidelines.fileTriggers.pathExclusions` | `**/*.test.ts`, `**/*.test.tsx` | **keep** | Accurate. |
| M6 | `skills.backend-dev-guidelines.promptTriggers` | keywords + intent patterns | **keep** | Generic but harmless and currently the only thing that fires the skill. |
| M7 | `skills.frontend-dev-guidelines.promptTriggers` | keywords + intent patterns | **keep** | Accurate. |
| M8 | `notes.customization.pathPatterns` | "Adjust to match your project structure (blog-api, auth-service, etc.)" | **fix** | PRD §9: prior project's example values. → "(apps/fleet-service, apps/web, etc.)". Cosmetic, but the file is already open. |
| M9 | `notes.enforcement_types`, `notes.priority_levels` | — | **keep** | Generic documentation of the schema, accurate. |

- [ ] **Step 1: Apply the edits**

```json
"fileTriggers": {
  "pathPatterns": [
    "apps/*/internal/**/*.go",
    "packages/shared-go/**/*.go",
    "packages/dto-go/**/*.go"
  ],
  "pathExclusions": [
    "**/*_test.go"
  ],
  "contentPatterns": []
}
```

- [ ] **Step 2: Validate the JSON**

```bash
python3 -c "import json;d=json.load(open('.claude/skills/skill-rules.json'));print(json.dumps(d['skills']['backend-dev-guidelines']['fileTriggers'],indent=2));print('frontend patterns:',d['skills']['frontend-dev-guidelines']['fileTriggers']['pathPatterns'])"
```

Expected: valid JSON, the three backend patterns, and exactly five frontend patterns with no `frontend/**` entries.

- [ ] **Step 3: Confirm the patterns match real files**

```bash
ls apps/fleet-service/internal/vehicle/*.go | head -3
ls packages/shared-go/server/*.go | head -3
ls packages/dto-go/ | head -3
```

- [ ] **Step 4: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/skills/skill-rules.json
git commit -m "docs(task-020): populate backend file triggers in skill-rules.json

backend-dev-guidelines.fileTriggers.pathPatterns was [], so the skill has never
auto-suggested on a Go file change despite PRD §1 and CLAUDE.md both asserting it
does — and PRD §10 asks the audit to confirm exactly that. Populated with
apps/*/internal/**/*.go, packages/shared-go/**/*.go and packages/dto-go/**/*.go,
excluding **/*_test.go to match the frontend entry's convention.

Dropped the two dead frontend/**/*.ts(x) patterns (no frontend/ directory); the
other five match real apps/web/src paths and are untouched. Corrected the
notes.customization blog-api example values.

.claude/settings.json is not modified — this is the trigger data the wiring
reads, not the wiring (PRD §2)."
```

---

## Task 26: Re-ground `.claude/agents/backend-guidelines-reviewer.md` (class C)

Tier 2. Six `DOM-*` checks are provably broken against this tree; every one either fails all seventeen domains or silently always passes. This is where the false positives PRD §1 describes are actually generated.

**Files:**
- Modify: `.claude/agents/backend-guidelines-reviewer.md` (198 lines)

**Canonical sources:** Tasks 2-11 output (each check must cite a Tier-1 file that now says the right thing) · `apps/fleet-service/internal/vehicle/` · `packages/shared-go/server/` · `Makefile`

**Interfaces:**
- Consumes: every backend Tier-1 file. A check may only assert something a Tier-1 file states.
- Produces: the `DOM-*` / `SUB-*` / `SEC-*` checklist and both artifact schemas.

**Rule inventory:**

| # | Line | Current | Disposition | Reason |
| --- | --- | --- | --- | --- |
| P-1 | 7, 9, 24, 39 | `services/auth-service` (4 sites) | **fix** | Directory does not exist → `apps/auth-service`. PRD §9, closed as "fold in" by design D1. |
| P-2 | 18-35 | Mindset (default FAIL, cite file:line, no invented rules) | **keep** | This is the agent's value; unchanged. |
| P-3 | 40-48 | Phase 0 read list (8 resource files) | **fix** | Verify all eight paths still resolve after Tasks 2-10. Note that `patterns-functional.md` keeps its filename despite the retitle (Task 5). |
| P-4 | 52-55 | Phase 1: `cd <service-path> && go build ./...` / `go test ./... -count=1` | **fix** | Correct per-service, but the repo entry point is `make build` / `make test` from the root, which covers every `go.work` module. Use the make targets for the objective gate and keep the per-service commands as the narrowing step. |
| P-5 | 59-66 | Phase 2 domain discovery (domain / sub-domain / support classification by `model.go` / `resource.go`) | **keep** | Accurate against the real tree. |
| P-6 | `DOM-01` | `builder.go` exists with `NewBuilder()`, fluent setters, validating `Build()` | **keep** | Accurate (`vehicle/builder.go`). |
| P-7 | `DOM-02` | `ToEntity()` on Model in `entity.go` | **keep** | Accurate. |
| P-8 | `DOM-03` | `Make(` exists, **returns `(Model, error)`** | **fix** | Real signature is `func Make(e Entity) Model` — no error — in 17 of 17 domains. **Fails every domain today.** Pass criterion becomes: function exists and returns `Model`. |
| P-9 | `DOM-04` | `Transform(` in `rest.go` | **keep** | Accurate. |
| P-10 | `DOM-05` | `TransformSlice(` exists; list handlers use it, no inline loops | **fix** | Keep the rule; **add the `TransformDerived` caveat** (Task 3 R27) or it false-positives on `vehicle/resource.go:49-52`, which legitimately loops to attach per-row derived values. |
| P-11 | `DOM-06` | Processor constructor takes `logrus.FieldLogger` | **keep** | Accurate everywhere. |
| P-12 | `DOM-07` | Handlers pass `d.Logger()` to `NewProcessor`; none pass `logrus.StandardLogger()` | **fix** | `d.Logger()` has 0 hits — **fails every domain**. Real: `InitializeRoutes` takes `log logrus.FieldLogger` and passes it to `NewProcessor` (`vehicle/resource.go:29-30`). The `logrus.StandardLogger()` ban survives. |
| P-13 | `DOM-08` | Grep `Methods(http.MethodPost)` / `Methods(http.MethodPatch)` | **fix** | gorilla/mux idiom; chi uses `r.Post(...)` / `r.Patch(...)`. **Vacuous today** — the grep never matches, so the check silently always passes with no evidence. Re-ground on `r.Post(` / `r.Patch(` / `r.Put(` and require `server.RegisterInputHandler`. |
| P-14 | `DOM-09` | "Transform errors handled" — no `_, _ :=` on `Transform(` | **replace** (design D2) | `Transform` returns no error, so the check is unfalsifiable. **New:** every domain error is routed through `server.WriteError(w, err)`; no hand-built error envelope, no bare `http.Error`, no per-error `w.WriteHeader` ladder. Grounded in `packages/shared-go/server/jsonapi.go` and `vehicle/resource.go:38`, and stated in Tier 1 by Task 7 X26. ID retained — `docs/tasks/*/audit.md` files cite these numbers. |
| P-15 | `DOM-10` | Providers use `database.Query`/`SliceQuery`, not eager execution wrapped in `FixedProvider` | **replace** (correction C1) | `database.Query` and `SliceQuery` **do exist**, contrary to design §3.1 — but they are invoked immediately and only 4 of 19 providers use them, so the check fails the 15-provider majority. `FixedProvider` has 0 hits. **New:** `provider.go` declares a `Provider` interface with a `db`-backed implementation and a `NewProvider(db)` constructor, and translates `gorm.ErrRecordNotFound` to a domain `ErrNotFound`. That invariant holds for all 19. Stated in Tier 1 by Task 2. |
| P-16 | `DOM-11` | No `os.Getenv` in `resource.go` | **keep** | Accurate. |
| P-17 | `DOM-12` | No cross-domain logic in handlers | **keep** | Accurate. |
| P-18 | `DOM-13` | Handlers don't call providers directly | **keep** | Accurate; also enforced by `arch_test.go`. |
| P-19 | `DOM-14` | No `db.Create`/`db.Save`/`db.Delete` in `resource.go` | **keep** | Accurate. |
| P-20 | `DOM-15` | `administrator.go` exists for write operations | **keep** | Accurate. |
| P-21 | `DOM-16` | Domain error → HTTP status mapping | **fix** | Rule stands, mechanism inverts: `server.WriteError` + `server.StatusFor` do the mapping from sentinels (`errors.go:18-43`). Verify the domain returns the right sentinel, not that the handler writes the right number. |
| P-22 | `DOM-17` | `RestModel` implements `GetName()`/`GetID()`/`SetID()` | **replace** (design D2) | No `RestModel` and no `GetName` anywhere — **fails every domain**. **New:** `rest.go` returns `server.Resource` with a literal `Type` string and the model's ID; the JSON:API `type` is not computed or reflected. Grounded in `vehicle/rest.go:74-76`. ID retained. |
| P-23 | `DOM-18` | `CreateRequest`/`UpdateRequest` have no nested Data/Type/Attributes | **fix** | The underlying rule is real; the names are not — real names are `createAttributes` / `patchAttributes` (`rest.go:35,48`) and the nesting is stripped by `RegisterInputHandler`. Re-ground on the narrow-named-struct convention (Task 3's added rule). |
| P-24 | `DOM-19` | Table-driven tests (`tests := []struct{...}` + `t.Run`) | **keep** | Accurate. |
| P-25 | `SUB-01`-`SUB-04` | Sub-domain checklist | **keep**, one **fix** | All four accurate. `SUB-03`'s "POST endpoints use typed input handler" stays; verify against `vehiclemedia/`. |
| P-26 | `SEC-01`-`SEC-04` | Security review | **keep** | All four accurate and unaffected by the drift. **Do not add `SEC-05`** for the `WriteError` 5xx redaction — it is real and currently unchecked (`jsonapi.go:95-116`) but adding a check is a new check, which PRD §2 excludes. Deferred. |
| P-27 | 115-190 | Phase 5 artifact formats (`audit.md` and `audit.json` schemas) | **keep** | Both fine. |
| P-28 | 192-198 | Status assignment rules | **keep** | Accurate. |
| — | — | *(absent)* — evidence required on PASS as well as FAIL | **add** | Design §9 Gate 4: a grep that can never match reports PASS with no evidence, which is how `DOM-08` hid. Require a `file:line` citation on **every** row, PASS or FAIL. This makes vacuous checks visible instead of silently green. |

- [ ] **Step 1: Apply the edits**

Work the table row by row. For each `fix` and `replace` row, the new "How to Verify" must name a command that can actually match something in this tree, and the row must be traceable to a Tier-1 file rewritten in Tasks 2-11.

- [ ] **Step 2: Prove each re-grounded check can now pass**

Run each check's new verification against `apps/fleet-service/internal/vehicle` and confirm the expected result:

```bash
cd apps/fleet-service/internal/vehicle
grep -n "func Make(" entity.go                              # DOM-03: returns Model, no error
grep -n "func TransformSlice(" rest.go                      # DOM-05
grep -n "func InitializeRoutes(log logrus.FieldLogger" resource.go  # DOM-07
grep -nE 'r\.(Post|Patch|Put)\(' resource.go                # DOM-08: must match
grep -c "server.WriteError" resource.go                     # DOM-09: > 0
grep -nE "type Provider interface|func NewProvider|ErrRecordNotFound" provider.go  # DOM-10
grep -n "Type:" rest.go                                     # DOM-17: literal type string
grep -nE "createAttributes|patchAttributes" rest.go         # DOM-18
```

Every one must produce output. A check whose grep returns nothing is still vacuous — fix it before committing.

- [ ] **Step 3: Run the gate, commit**

```bash
cd - && bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
git add .claude/agents/backend-guidelines-reviewer.md
git commit -m "docs(task-020): re-ground the backend reviewer checklist

Six DOM checks were provably broken. DOM-03 (Make returns an error), DOM-07
(handlers pass d.Logger()) and DOM-17 (RestModel implements GetName/GetID/SetID)
failed all 17 domains. DOM-08 (grep Methods(http.MethodPost)) is a gorilla/mux
idiom that never matches chi, so it silently always passed with no evidence.
DOM-09 checked an error Transform does not return. DOM-10 required
database.Query — which does exist, contrary to design §3.1, but is invoked
immediately and used by only 4 of 19 providers.

DOM-09 and DOM-17 are replaced, IDs retained (design D2): DOM-09 now checks that
errors route through server.WriteError, DOM-17 that rest.go returns
server.Resource with a literal Type. Renumbering was rejected because
docs/tasks/*/audit.md already cites these numbers. DOM-10 is re-grounded on the
Provider-interface + NewProvider + ErrNotFound-translation invariant, which all
19 providers satisfy.

services/auth-service -> apps/auth-service (4 sites). Phase 1 uses make build /
make test. Added the requirement that every row cite file:line on PASS as well as
FAIL — a grep that cannot match must not report a silent PASS.

DOM-01/02/04/06/11-16/19, all SUB-*, all SEC-*, Phases 2 and 5, and both artifact
schemas are unchanged. SEC-05 for the WriteError 5xx redaction is deferred: it is
real and unchecked, but adding a check is out of scope per PRD §2."
```

---

## Task 27: Re-ground `.claude/agents/frontend-guidelines-reviewer.md` (class C)

Phase 1 cannot complete today: it runs `cd frontend && npm test -- --watchAll=false` against a repo whose web app is at `apps/web` and whose runner is Vitest, which rejects `--watchAll`. A reviewer that cannot finish its objective gate is worse than one emitting false positives.

**Files:**
- Modify: `.claude/agents/frontend-guidelines-reviewer.md` (167 lines)

**Canonical sources:** Tasks 12-24 output · `Makefile` · `apps/web/package.json`

**Interfaces:**
- Consumes: every frontend Tier-1 file, especially Task 21's anti-pattern list (each `FE-*` row must map to a rule there) and Task 23's cursor-affordance section (`FE-15` cites it by name).

**Rule inventory:**

| # | Line | Current | Disposition | Reason |
| --- | --- | --- | --- | --- |
| F-1 | 7, 24 | `frontend/src/pages`, `frontend/src` | **fix** | → `apps/web/src`. |
| F-2 | 18-33 | Mindset | **keep** | Unchanged. |
| F-3 | 37-48 | Phase 0 read list (10 files) | **fix** | Verify all ten resolve. `patterns-api-client.md` and `ai-guidance.md` are missing from the list although the skill has them — add or leave deliberately, and say which. |
| F-4 | 52-54 | Phase 1: `cd frontend && npm run build`; `npm test -- --watchAll=false` | **replace** | Path wrong, and `--watchAll` is a Jest flag Vitest rejects. → `make fe-build` and `make fe-test`, which also cover `packages/shared-ts` and `ui-components` (`.github/workflows/pr.yml` explains why that matters). Note the nvm line from CLAUDE.md. |
| F-5 | 59-69 | Phase 2 file classification | **keep**, one **fix** | `Hook` = `lib/hooks/api/*.ts` is correct. `Type` = `types/**/*.ts` should be `types/models/*.ts` — `types/api/` does not exist. |
| F-6 | `FE-01` | No `any` type | **keep** | Accurate. |
| F-7 | `FE-02` | No manual class concatenation; `cn()` used | **keep** | Accurate. |
| F-8 | `FE-03` | Grep components/pages for `import .* from "@/lib/api/client"` | **fix** | **Vacuous today** — no `@/` alias exists, so the grep never matches and the check silently always passes. Re-ground on the real import: `from '../../lib/api/client'` (any relative depth) and the real export name `apiClient`, not `api`. The rule — components go through the service layer — is unchanged. |
| F-9 | `FE-04` | No inline Zod in components, except `.refine()` | **keep** | Accurate; consistent with Task 18 B4. |
| F-10 | `FE-05` | No spinners for content loading (`animate-spin` allowed only on submit buttons) | **keep** | Accurate. |
| F-11 | `FE-06` | No hardcoded colors | **keep** | Accurate; verify the class-name pattern against Task 23's token list. |
| F-12 | `FE-07` | No state mutation | **keep** | Accurate. |
| F-13 | `FE-08` | No default exports for components | **keep** | Accurate. |
| F-14 | `FE-09` | Error handling with `createErrorFromUnknown` | **fix** | The function is real but imported from `@myfleet/shared-ts`, not a local util (`packages/shared-ts/src/errors.ts:23`). The check text must say so or the verifier looks in the wrong place. |
| F-15 | `FE-10` | JSON:API model shape in `types/models/` | **keep**, **fix** | Accurate; note the real form is `export type X = JsonApiResource<XAttributes>` (Task 14 Y2). |
| F-16 | `FE-11` | Service extends `BaseService` "or uses the documented direct-client pattern" | **fix** | Task 13 V10 removed the alternative — every service extends `BaseService`. Drop the escape clause. |
| F-17 | `FE-12` | Query key factory uses `as const` | **keep** | Accurate (`vehicles.ts:14-21`). |
| F-18 | `FE-13` | Forms use RHF + `zodResolver` | **keep** | Accurate. |
| F-19 | `FE-14` | Schema in `lib/schemas/` with inferred type | **keep**, **fix** | Accurate; drop any `.schema.` infix from the path pattern. |
| F-20 | `FE-15` | Interactive elements show `cursor-pointer`; cites `patterns-styling.md` → "Cursor affordance for interactive elements" | **keep** | The cited section survives verbatim (Task 23 G9). Confirm the link still resolves. |
| F-21 | `FE-16` | Tests exist for changed components | **keep** | Accurate. |
| F-22 | `FE-17` | Diff `__mocks__/` against changed services | **replace** | **No `__mocks__` directory exists anywhere under `apps/web`** — the check fails or is skipped every run. **New:** when a service's method signature changes, every `vi.mock` of that module in `*.test.ts(x)` is updated in the same change. Grounded in Task 17's `vi.mock` convention. ID retained. |
| F-23 | 112-159 | Phase 4 artifact format | **keep** | Fine. |
| F-24 | 161-167 | Status assignment rules | **keep** | Accurate. |
| — | — | *(absent)* — evidence required on PASS as well as FAIL | **add** | Same as Task 26. `FE-03` is exactly the failure mode: a grep that cannot match reported PASS with no evidence. |

- [ ] **Step 1: Apply the edits**

- [ ] **Step 2: Prove Phase 1 completes and each re-grounded check can match**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make fe-build && make fe-test
```

Both must succeed. Then:

```bash
grep -rn "from '\.\./\?.*lib/api/client'" apps/web/src | head    # FE-03: must match
grep -rn "createErrorFromUnknown" apps/web/src packages/shared-ts/src | head  # FE-09
grep -rn "extends BaseService" apps/web/src/services/api/ | head  # FE-11
grep -rn "as const" apps/web/src/lib/hooks/api/vehicles.ts        # FE-12
grep -rn "vi.mock" apps/web/src | head                            # FE-17
```

Every one must produce output.

- [ ] **Step 3: Reconcile against Task 21**

Hold the final `FE-*` list against `frontend-dev-guidelines/resources/anti-patterns.md`. Every check must map to a Tier-1 rule; every Tier-1 anti-pattern should have a check. Record any deliberate asymmetry.

- [ ] **Step 4: Run the gate, commit**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: **all 23 checks PASS, exit=0.** This is the last file in scope.

```bash
git add .claude/agents/frontend-guidelines-reviewer.md
git commit -m "docs(task-020): re-ground the frontend reviewer checklist

Phase 1 could not complete: it ran 'cd frontend && npm test -- --watchAll=false'
against a repo whose web app is at apps/web and whose runner is Vitest, which
rejects --watchAll. Replaced with make fe-build / make fe-test, which also cover
packages/shared-ts and ui-components.

FE-03 was vacuous — it grepped for an @/ import when no alias exists, so it
never matched and silently always passed. Re-grounded on the real relative
import and the real export name (apiClient, not api). FE-17 is replaced, ID
retained: there is no __mocks__ directory anywhere under apps/web, so it failed
or was skipped every run; it now checks that vi.mock call sites are updated when
a mocked service's signature changes.

FE-09 now states createErrorFromUnknown comes from @myfleet/shared-ts. FE-11
drops the direct-client escape clause (every service extends BaseService).
frontend/src -> apps/web/src; Type classification -> types/models/ (types/api/
does not exist). Added the requirement that every row cite file:line on PASS as
well as FAIL.

FE-01/02/04-08/10/12-16 unchanged."
```

---

## Task 28: Gates 1-3 — mechanical, identifier, and non-regression

**Files:**
- Create: `docs/tasks/task-020-dev-guidelines-skill-drift/verification.md`

**Interfaces:**
- Consumes: all of Tasks 1-27.
- Produces: the evidence record Task 29 and the eventual PR cite.

- [ ] **Step 1: Gate 1 — mechanical greps**

```bash
bash docs/tasks/task-020-dev-guidelines-skill-drift/drift-gate.sh; echo "exit=$?"
```

Expected: `drift-gate: ALL CHECKS PASS`, `exit=0`, all 23 rows PASS. Capture the full output into `verification.md`.

If a check still fails, fix it in a **follow-up commit**, not by amending the task that introduced it (design §8 step 8 — the audit trail survives).

- [ ] **Step 2: Gate 2 — per-identifier existence**

Extract every Go identifier from the backend skill's code blocks and confirm each is defined in the tree:

```bash
awk '/^```go/{f=1;next}/^```/{f=0}f' \
  .claude/skills/backend-dev-guidelines/SKILL.md \
  .claude/skills/backend-dev-guidelines/resources/*.md \
  | grep -oE '\b(server|database|entityguard|auth|telemetry|chi|gorm|logrus|uuid)\.[A-Za-z][A-Za-z0-9]*' \
  | sort -u > /tmp/claude-1000/ids.txt
wc -l /tmp/claude-1000/ids.txt

while read -r id; do
  pkg=${id%%.*}; sym=${id#*.}
  if ! grep -rqE "(func|type|var|const)\s+\(?[A-Za-z ]*\)?\s*${sym}\b|${sym}\s*=" \
       packages apps --include='*.go' 2>/dev/null; then
    echo "UNVERIFIED: $id"
  fi
done < /tmp/claude-1000/ids.txt
```

Every `UNVERIFIED` line needs a manual check — some are third-party (`chi.Router`, `gorm.DB`, `logrus.FieldLogger`, `uuid.NewString`) and are fine if they appear in real imports. Record the disposition of each in `verification.md`. **Zero misses among first-party `server.*`, `database.*`, `entityguard.*`, `auth.*`, `telemetry.*` identifiers.**

Do the same for the frontend, checking every documented symbol resolves in `apps/web/src` or `packages/shared-ts/src`.

- [ ] **Step 3: Gate 3a — no source was touched**

```bash
git diff --name-only main...HEAD | sort
git diff --name-only main...HEAD | grep -E '^(apps|packages|deploy|\.github)/' && echo "SCOPE VIOLATION" || echo "scope clean"
git diff --name-only main...HEAD | grep -E '^\.claude/settings' && echo "SCOPE VIOLATION" || echo "settings untouched"
```

Expected: only `.claude/skills/**`, `.claude/agents/{backend,frontend}-guidelines-reviewer.md`, and `docs/tasks/task-020-dev-guidelines-skill-drift/**`.

- [ ] **Step 4: Gate 3b — `make ci`**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: pass. This is trivially expected (no source changed) and exists to confirm nothing was touched by accident. Paste the tail into `verification.md`.

- [ ] **Step 5: Gate 3c — every documented path resolves**

```bash
grep -rhoE '(apps|packages|deploy|docs|tools)/[A-Za-z0-9_./-]+' .claude/skills .claude/agents/backend-guidelines-reviewer.md .claude/agents/frontend-guidelines-reviewer.md \
  | sed 's/[.,;:)]*$//' | sort -u \
  | while read -r p; do [ -e "$p" ] || echo "MISSING: $p"; done
```

Expected: no output, except for paths that are deliberately illustrative (e.g. `apps/<service>/Dockerfile`). List any survivors in `verification.md` with a reason.

- [ ] **Step 6: Gate 3d — skill activation**

Two checks:

```bash
# name / directory / skill-rules key all agree
grep '^name:' .claude/skills/backend-dev-guidelines/SKILL.md
grep '^name:' .claude/skills/frontend-dev-guidelines/SKILL.md
python3 -c "import json;print(list(json.load(open('.claude/skills/skill-rules.json'))['skills']))"
```

Then, **in a fresh session**: invoke `Skill("backend-dev-guidelines")` and confirm it loads, and touch a Go file under `apps/*/internal/` and confirm the `skill-activation-prompt` hook suggests the backend skill (this is what Task 25 made possible; it could not have passed before). Record both outcomes. **If the skill does not load, revert Task 11 and record why** (PRD FR-5.2).

- [ ] **Step 7: Record the token budget outcome**

```bash
find .claude/skills -name '*.md' -exec wc -l {} + | tail -1
wc -l .claude/agents/backend-guidelines-reviewer.md .claude/agents/frontend-guidelines-reviewer.md
```

Compare against the Task 1 baseline (skills: 4389 lines; agents: 198 + 167). PRD §8 targets net-neutral or smaller for the skills. If the total grew, say so plainly with the reason rather than trimming content that Gate 2 needs.

- [ ] **Step 8: Commit**

```bash
git add docs/tasks/task-020-dev-guidelines-skill-drift/verification.md
git commit -m "docs(task-020): record gates 1-3 verification results"
```

---

## Task 29: Gate 4 — dispatch both reviewers against real diffs

The only gate that tests the PRD's actual goal. Gates 1-3 exist to make this one cheap.

**Files:**
- Modify: `docs/tasks/task-020-dev-guidelines-skill-drift/verification.md`

**Interfaces:**
- Consumes: Tasks 26 and 27's checklists, and every Tier-1 file they read in Phase 0.

**Targets** (design §9, resolved by correction C3):
- **Frontend:** task-017 `app-frame-navigation`, merge `ea9e37a` (PRD §10 names it).
- **Backend:** task-014 `member-names-ownership-transfer`, merge `92b1290`. It touches `apps/fleet-service/internal/membership` and `apps/auth-service/internal/{membership,user}`; two of those are complete domain packages with `model.go` through `rest.go`, so DOM-01–DOM-19 are all exercised across two services. task-013 was rejected — its only `model.go` is `media-service/internal/mediavariant`, which has no `resource.go` or `rest.go`, leaving eight DOM checks unexercised.

- [ ] **Step 1: Produce the target diffs**

```bash
git diff --name-only 92b1290^1 92b1290 -- 'apps/*/internal/*' | sed 's|\(apps/[^/]*/internal/[^/]*\)/.*|\1|' | sort -u
git diff --name-only ea9e37a^1 ea9e37a -- 'apps/web/**' | sort
```

- [ ] **Step 2: Dispatch both reviewers**

Dispatch `backend-guidelines-reviewer` over the three task-014 packages and `frontend-guidelines-reviewer` over the task-017 web files, in parallel. Have each write to a scratch path, not to the historical task folders — those audits are a record of what was found at the time and must not be overwritten.

- [ ] **Step 3: Judge the result against the four success criteria**

Success is **not** "the audit passes". It is all four of:

1. **Neither reviewer sets any checklist item aside as inapplicable.** Search both reports for hedging — "N/A", "not applicable", "skipping", "does not apply to this codebase", "setting aside". Any hit is a failure; the check it names still describes something the repo does not have.
2. **No finding is traceable to guideline drift.** For every FAIL, confirm it names a real defect in the reviewed code, not a mismatch between the checklist and the architecture. Classify each FAIL as `real` or `drift`; any `drift` is a failure of this task.
3. **Every row carries a `file:line` citation — PASS or FAIL.** A row with a status and no evidence is a vacuous check. This is the clause `DOM-08` and `FE-03` would have failed today *by passing*.
4. **Phase 1 completed for both.** The frontend gate in particular could not run before Task 27.

- [ ] **Step 4: Handle failures**

If criterion 1 or 2 fails, the named check is still drifted: fix it in a follow-up commit on the relevant Tier-1 and Tier-2 files, then re-run Step 2 for that reviewer. Do not weaken the criteria. If criterion 3 fails for a row, that row's verification recipe cannot match anything — re-ground it.

- [ ] **Step 5: Record the outcome**

Append to `verification.md`: both reviewers' full check tables, the `real`/`drift` classification of every FAIL, an explicit statement for each of the four criteria, and the target diffs used with the reason task-014 was chosen over task-013.

- [ ] **Step 6: Commit**

```bash
git add docs/tasks/task-020-dev-guidelines-skill-drift/verification.md
git commit -m "docs(task-020): record gate 4 — both reviewers run clean on real diffs

Backend: task-014 (92b1290), three domain packages across two services,
exercising DOM-01 through DOM-19. Frontend: task-017 (ea9e37a) per PRD §10.

Criteria: no checklist item set aside, no finding traceable to guideline drift,
every row cites file:line on PASS as well as FAIL, and Phase 1 completed for
both reviewers."
```

---

## Self-Review Record

Run before handing off. Results of the checks the writing-plans skill requires:

**Spec coverage.** Every PRD functional requirement maps to a task: FR-1.1/1.2 → Tasks 4, 10; FR-1.3 → Task 4; FR-1.4 → Task 6; FR-2.1–2.7 → Task 3; FR-2.8 → Task 3; FR-3.1 → Task 2; FR-3.2 → Tasks 5, 6; FR-3.3 → Task 5; FR-3.4 → Task 5; FR-3.5 → Tasks 2, 7; FR-4.1/4.2 → Tasks 6, 7, 8; FR-4.3 → Tasks 2, 4; FR-5.1/5.3 → Task 11; FR-5.2 → Tasks 11, 28; FR-6.1 → Tasks 17, 24; FR-6.2 → Tasks 16–22; FR-6.3 → Task 13; FR-6.4 → Tasks 13, 14, 22; FR-6.5 → Task 13; FR-6.6 → Tasks 15–20, 23; FR-6.7 → Tasks 21, 27; FR-7.1–7.4 → Global Constraints + Task 28 Gate 2. Every design-§6 file has a task. Design §10's three Phase-3 deliverables are the per-task rule inventories, Task 10's §4/§5/§6 disposition, and Task 9's mock disposition.

**Deliberate scope additions beyond the PRD**, each carried from a design decision: both reviewer agent files (design D1, closing PRD §9's open question) and `skill-rules.json`'s backend triggers (design D5). Both are recorded in the commit bodies.

**Type and name consistency.** The identifiers introduced in Task 2 (`Provider`, `Administrator`, `NewProvider`, `NewAdministrator`, `TxHook`, `ErrNotFound`, `server.Page`) and Task 3 (`InitializeRoutes`, `RegisterInputHandler`, `WriteJSON`, `WriteError`, `Resource`, `Document`, `Transform`, `TransformSlice`, `createAttributes`, `patchAttributes`) are used with those exact spellings in Tasks 4-11 and 26. Frontend: `BaseService`, `ListResult<A>`, `resourceType`, `basePath`, `listAt`, `createAt`, `apiClient`, `createErrorFromUnknown`, `renderWithProviders`, `createTestQueryClient`, `vehicleKeys` are consistent across Tasks 12-24 and 27.

**Known open decisions**, each with a resolution step inside its task rather than left to the implementer: Task 5 N4 (`Model.Builder()`), Task 11 K11 (service READMEs), Task 14 Y5/Y6 (numeric enums, helper location), Task 15 W2/W5/W6/W8/W10 (client capabilities), Task 16 Z5/Z6/Z7 (optimistic updates, invalidation helper, prefetch), Task 18 B3/B5/B6/B8/B9/B11 (schema patterns), Task 19 C4/C8/C9/C12 (component patterns), Task 22 E13 (enum display), Task 23 G1-G7 (styling verification). Each step names the command that decides it and requires the decision be recorded in the commit body.
