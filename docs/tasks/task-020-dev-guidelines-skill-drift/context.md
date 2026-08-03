# Dev Guidelines Skill Drift — Implementation Context

Companion to [plan.md](plan.md). Read this first if you are picking the task up cold.

PRD: [prd.md](prd.md) · Design: [design.md](design.md) · Specification inventory: [drift-inventory.md](drift-inventory.md)
Verified against branch `task-020-dev-guidelines-skill-drift` @ `2c5e8d4` (tree identical to `main` @ `d3c9eaa` for every path in scope).

---

## 1. What this task is

`.claude/skills/backend-dev-guidelines` and `.claude/skills/frontend-dev-guidelines` describe an architecture MyFleet does not have. They were carried in from a prior object-storage project and never reconciled. The backend skill documents a `services/<name>/` layout, an api2go JSON:API transport, a curried `server.RegisterHandler(l)(si)(...)` registration API, and a lazy `model.Provider[T]` / `Map` / `ParallelMap` composition library. None of that exists here. The frontend skill documents Jest and illustrates every pattern with buckets, bans and policies.

This is not cosmetic. `.claude/settings.json` wires a `skill-activation-prompt` hook that suggests these skills on TypeScript changes, and CLAUDE.md makes `backend-guidelines-reviewer` and `frontend-guidelines-reviewer` a required pre-PR gate. A reviewer applying these checklists literally produces mostly false positives — which is what happened in the task-010 review (#20), where the backend reviewer had to explicitly set the checklist aside.

The work is a rewrite from repository source, not a rename-in-place. `patterns-provider.md` documents a data-access library that was never written; there is no correct identifier to substitute.

---

## 2. The two tiers

```
              ground truth: apps/**/internal/**  ·  packages/shared-go/**  ·  apps/web/src/**
                                        │
Tier 1   .claude/skills/{backend,frontend}-dev-guidelines/     23 files   prose + worked examples
                                        │  Phase 0 read list
Tier 2   .claude/agents/{backend,frontend}-guidelines-reviewer.md  2 files   ID'd binary checks
                                        │
                              docs/tasks/<task>/audit.md
```

**The invariant the rewrite establishes:** a Tier-2 check may only assert something a Tier-1 file states, and a Tier-1 statement may only assert something a ground-truth `file:line` shows. Today the chain is broken at both joints — Tier 2 asserts `Make` returns an error, Tier 1 also (wrongly) claims it, and neither matches the code.

**Tier 2 is in scope**, which the PRD did not anticipate. `grep -rln "DOM-0\|SUB-0\|SEC-0\|FE-0" .claude/` returns exactly two files, and neither is a skill: the ID-bearing checks live in the agent definitions. PRD §2's goal ("reviewers can run their checklists literally without setting any item aside") is unreachable by editing skills alone. Design D1 folds both agent files in; PRD §9's open question is closed as "fold in".

---

## 3. Sequencing and why it matters

Tasks run in plan order. The order is not arbitrary:

| Tasks | Block | Why here |
| --- | --- | --- |
| 1 | Drift gate | The gate is written first and must fail. It is the test. |
| 2-3 | Backend **R** rebuilds | `patterns-provider.md` then `patterns-rest-jsonapi.md` define the API vocabulary everything downstream reuses — later files quote rather than re-derive. |
| 4-10 | Backend **S** files | Internally independent; nothing in this block blocks anything else in it. |
| 11 | Backend `SKILL.md` | Isolated so an activation failure reverts surgically (PRD FR-5.2). |
| 12 | Frontend `architecture-overview.md` | Settles the tree, the config, and the `@/`-alias question the other eleven files depend on. |
| 13-24 | Frontend **R/S/V** files | Service layer → types → client → hooks → tests → forms → components → routing → anti-patterns → ai-guidance → styling → SKILL.md. |
| 25 | `skill-rules.json` | Trigger data; Task 28's activation check needs it. |
| 26-27 | Tier 2 | Last, so every check can cite a Tier-1 file that already says the right thing. |
| 28-29 | Gates 1-3, then Gate 4 | Gate 4 is the only one that tests the PRD's goal; 1-3 exist to make it cheap. |

One commit per file (PRD §8). Fixes land as follow-up commits, never amendments, so the audit trail survives.

---

## 4. Corrections this plan makes to the design

Three Phase-3 findings change what the design told the implementer. They are stated in full in plan.md's *Corrections to the Design*; in brief:

**C1 — `database.Query` / `database.SliceQuery` do exist.** Design §3.1 and §6 say they do not. They are in `packages/shared-go/database/query.go:4-7` and used by 4 of 19 `provider.go` files — but invoked immediately (`database.Query(func()...)()`), so behaviourally identical to the plain form. `DOM-10` is still broken, but for a different reason: it fails the 15-provider majority. Do not delete a real symbol, and re-ground `DOM-10` on the `Provider`-interface invariant all 19 satisfy. (`model.Provider`, `model.Map`, `FixedProvider`, `EntityProvider` remain confirmed absent.)

**C2 — backend tests use no assertion library.** Zero `mock/` directories, zero `type ...Mock struct`, zero `stretchr/testify` in any `.go` source, across 126 test files. The real convention is unexported `fake*`/`stub*` structs in the `_test.go` that uses them, with stdlib `t.Fatalf`. So `testing-guide.md`'s `require.NoError` examples are drift too — the design did not record that.

**C3 — Gate-4 backend target is task-014, not task-013.** task-014 (`92b1290`) touches three packages across two services, two of them complete domain packages with `model.go` through `rest.go`. task-013's only `model.go` is `mediavariant`, which has no `resource.go`/`rest.go`, leaving eight DOM checks unexercised.

---

## 5. Key decisions carried from design

| Decision | Where |
| --- | --- |
| Both reviewer agent files are in scope | design D1 · plan Tasks 26-27 |
| `DOM-09` and `DOM-17` are **replaced**, IDs retained — `docs/tasks/*/audit.md` already cites these numbers, so renumbering silently changes what historical records mean | design D2 · plan Task 26 P-14, P-22 |
| `patterns-provider.md` is rewritten in place and absorbs the write side | design D3 · plan Task 2 |
| Rule inventories are rule-level and live in each task, not in design.md | design D4 · plan, every task |
| `skill-rules.json` gets backend file triggers (the backend skill has **never** auto-suggested on a Go change) | design D5 · plan Task 25 |
| Docs describe relative imports; the `@/` alias is **not** added | design §7 · plan Task 12 Q20 |
| `patterns-functional.md` is **retitled, not renamed** — `SKILL.md:113` and the backend agent's Phase 0 list both reference the path | plan Task 5 |
| `scaffolding-checklist.md` §4/§5/§6 deleted (no compose file, no Bruno collection; the only nginx.conf serves the SPA) | plan Task 10 |
| `testing-guide.md`'s ~90 mock-directory lines deleted | plan Task 9 · correction C2 |
| No new checks. `SEC-05` for the `WriteError` 5xx redaction is real and unchecked, but adding it is out of scope | PRD §2 · plan Task 26 P-26 |

---

## 6. Ground truth cheat sheet

Everything below was verified during planning. Do not re-derive; do re-read the source file when you need surrounding context.

**Layout.** Go services `apps/{auth,fleet,media,notification}-service` (each with its own `go.mod`, `cmd/`, `internal/`, `Dockerfile`); web app `apps/web`; shared `packages/{shared-go,dto-go,shared-ts,ui-components}`; joined by `go.work`. **No `services/` directory. No `migrations/` directory** — migrations are `func Migration(db *gorm.DB) error { return db.AutoMigrate(&Entity{}) }` in each domain's `entity.go`.

**Build.** `make ci` = `lint-check vet test build fe-test fe-build manifests carfax-template`. Node may need `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

**`packages/shared-go/server`.** `New`/`Use`/`AddRouteInitializer(func(chi.Router))`/`Router()` — **no `Run()`**. `RegisterInputHandler[T](fn func(http.ResponseWriter, *http.Request, T)) http.HandlerFunc` — one argument, decodes `{data:{attributes:T}}`, writes `ErrValidation` on decode failure. `WriteJSON`, `WriteError` (5xx title redacted to `InternalErrorTitle`, real error logged server-side), `Resource`, `Document`, ten error sentinels + `StatusFor`, `Detailed`, `Page`/`PageMeta`/`ParsePage`.

**Absent from the whole tree (0 hits each — this is the drift):** `api2go`, `jsonapi.ServerInformation`, `server.RegisterHandler`, `server.MarshalResponse`, `server.RouteInitializer`, `server.GetHandler`, `HandlerDependency`, `HandlerContext`, `d.Logger()`, `ParseId`, `RestModel`, `GetName`, `SetID`, `model.Provider`/`Map`/`SliceMap`/`ParallelMap`/`ErrorProvider`/`FixedProvider`, `database.EntityProvider`, `mux.Router`, `uuid.UUID`, `uint32` as an entity ID.

**Frontend.** Root `apps/web`. `components/` = `admin/`, `features/`, `frame/`, `providers/`, `ui/` — **no `common/`**. `types/` holds only `models/` — **no `types/api/`**. **No** `lib/breadcrumbs/`, `lib/query-client.ts`, `services/api/index.ts`, `__mocks__/`, `DataTable`, TanStack React Table. **No `@/*` path alias** in `tsconfig.app.json` or `vite.config.ts` — imports are relative. `exactOptionalPropertyTypes` and `noImplicitOverride` are not set. Services are PascalCase (`VehicleService.ts`); schemas are `lib/schemas/vehicle.ts` (no `.schema.` infix). Runner is Vitest; `@testing-library/jest-dom` is a real matcher library and **not** drift. React Query defaults live in `AppProviders.tsx` (`retry: 1`, `refetchOnWindowFocus: false`, no staleTime); staleness is per hook (`staleTime: 60_000`, `gcTime: 300_000`).

---

## 7. Canonical sources

Every rewritten example is copied or minimally adapted from this set — that is what makes the identifier gate tractable. Full table in plan.md; the load-bearing ones:

- **Backend:** `apps/fleet-service/internal/vehicle/` (all eight files) · `packages/shared-go/server/{handler,jsonapi,errors,pagination}.go` · `packages/shared-go/database/query.go` · `apps/fleet-service/internal/maintenancerecord/processor.go:19` · `apps/media-service/internal/mediaobject/processor.go:202` · `apps/auth-service/internal/user/{provider,processor_test}.go`
- **Frontend:** `apps/web/src/services/api/{BaseService,VehicleService}.ts` · `lib/hooks/api/vehicles.ts` · `components/providers/AppProviders.tsx` · `lib/api/{client,refresh}.ts` · `packages/shared-ts/src/{jsonapi,errors}.ts` · `src/test/renderWithProviders.tsx`

**Two of these are executable guideline enforcement that already exists** — `apps/fleet-service/internal/admin/arch_test.go` (and `apps/auth-service/internal/arch/arch_test.go`) and `apps/web/src/test/conventions.test.ts`. Where a rewritten rule is already guarded by one of them, the guideline should say so and cite it. That is the cheapest available form of the anti-drift enforcement PRD §2 defers.

---

## 8. Verification

Four gates. Gate 4 is the only one that tests the PRD's goal.

**Gate 1 — `drift-gate.sh`** (Task 1, committed to the task folder so the audit can re-run it verbatim). 23 checks, all failing at baseline with 501 total hits. Two need care and are already handled: the `services/` guard must not flag the legitimate frontend `services/api/`, and the Jest guard must not flag `@testing-library/jest-dom`. The vocabulary check (G-10) matches substrings rather than word boundaries, because the drift also lives inside identifiers (`bucketKeys`, `bansService`, `CreateBucketDialog`, `BanType`); a legitimate prose use of "policy" can be kept by putting `ALLOW-VOCAB` on the line.

**Gate 2 — identifier existence.** Every Go identifier in a rewritten backend code block must resolve in `packages/shared-go` or a real `apps/*/internal/*` package. This is PRD FR-7.2 made executable without a build harness.

**Gate 3 — non-regression.** No file under `apps/`, `packages/`, `deploy/`, `.github/` or `.claude/settings.json` in the branch diff; `make ci` passes; every documented path resolves; the backend skill loads under its new name and the activation hook fires on a Go change (which requires Task 25 — it does not fire today).

**Gate 4 — the real test.** Both reviewers dispatched against real merged diffs (backend: task-014 `92b1290`; frontend: task-017 `ea9e37a`). Success is not "the audit passes" but all four of: no checklist item set aside as inapplicable; no finding traceable to guideline drift; **every row cites `file:line` on PASS as well as FAIL**; and Phase 1 completed for both. That third clause is the one `DOM-08` and `FE-03` would fail today *by passing* — a grep that can never match reports PASS with no evidence.

---

## 9. Risks

| Risk | Mitigation |
| --- | --- |
| A real rule is silently lost in an **R** rebuild | Every task opens with a rule-level inventory carrying a stated reason for each delete/replace; `patterns-rest-jsonapi.md` (444 lines, ~40 rules) is where this is most likely |
| Rewritten examples introduce *new* drift — plausible Go that does not compile | Gate 2's per-identifier grep plus the fixed canonical source set: examples are copied, not composed |
| The `SKILL.md` rename breaks activation | Isolated commit (Task 11); FR-5.2 requires revert-and-record on failure. `skill-rules.json` already keys by directory name, so the rename brings the two into agreement |
| Tier-2 edits change what past audits meant | Design D2 forbids renumbering; only `DOM-09`, `DOM-17` and `FE-17` change meaning, each recorded with a reason |
| Verify-decisions get skipped and an unused pattern is documented as current | Each is a numbered step naming the command that decides it, and the decision must be recorded in the commit body (FR-7.4) |
| Token budget grows (PRD §8) | The deletions are large — `patterns-provider.md` loses its whole body, `patterns-functional.md` two sections, `scaffolding-checklist.md` three, backend `testing-guide.md` ~90 lines, frontend `patterns-components.md` ~59, frontend `ai-guidance.md` ~30. Each commit records its line delta and the running total; baseline is 4389 skill lines |
| Re-drift the next time `shared-go/server` changes | Not solved here (PRD §2). Partially mitigated by citing `arch_test.go` and `conventions.test.ts` where a rule is already executably enforced, and by `drift-gate.sh` being committed — promoting it to CI later is a one-line workflow change |

---

## 10. Deferred

| Item | Why |
| --- | --- |
| `@/` path alias in `apps/web` | Application config plus hundreds of import rewrites; PRD §2 excludes app changes (design §7) |
| `SEC-05` for `WriteError`'s 5xx redaction | Real and currently unchecked (`packages/shared-go/server/jsonapi.go:95-116`), but adding a check is a new check — PRD §2 excludes that |
| CI grep guard against re-drift | PRD §2 defers it; `drift-gate.sh` is committed, so promoting it is a one-line change |
| Compile-checked example snippets | PRD §2 defers it; Gate 2 is the manual stand-in |
| Broader `docs/` audit | PRD §2 excludes it |
