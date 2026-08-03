# Dev Guidelines Skill Drift — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-03
Issue: [#23](https://github.com/jtumidanski/MyFleet/issues/23)
---

## 1. Overview

`.claude/skills/backend-dev-guidelines` and `.claude/skills/frontend-dev-guidelines` describe an architecture MyFleet does not have. They were carried in from a prior object-storage project and never reconciled with this repo. The backend skill documents a `services/<name>/` layout, an api2go-based JSON:API transport, a curried `server.RegisterHandler(l)(si)(...)` registration API, and a lazy `model.Provider[T]` / `Map` / `ParallelMap` composition library. None of that exists here. The frontend skill documents Jest and illustrates every pattern with buckets, bans, and policies — resources this product does not have.

This is not cosmetic. `.claude/settings.json` wires a `skill-activation-prompt` hook that auto-suggests these skills whenever Go or TypeScript files change, and CLAUDE.md makes `backend-guidelines-reviewer` and `frontend-guidelines-reviewer` a required pre-PR gate. A reviewer applying these checklists literally produces mostly false positives. That is exactly what happened during the task-010 review (#20), where the backend reviewer had to explicitly set the checklist aside to avoid emitting a page of spurious failures. A gate that reliably cries wolf trains its readers to skim, and the DOM-\*/SUB-\*/SEC-\* checks that *are* accurate get buried alongside the noise.

The fix is to rewrite the resource files from the repository's actual source rather than to rename identifiers in place. The drift is structural, not lexical: `patterns-provider.md` documents a data-access library that was never written, so there is no correct identifier to substitute. Every normative claim in the rewritten guidelines must be traceable to a real file in this tree.

## 2. Goals

Primary goals:

- Every normative statement in both skills' resource files is true of this repository, and traceable to a real `file:line`.
- The `backend-guidelines-reviewer` and `frontend-guidelines-reviewer` agents can run their checklists literally, without setting any item aside, and produce zero false positives attributable to guideline drift.
- Accurate existing content — the layer model, immutability rules, the DOM-\*/SUB-\*/SEC-\* checks, the frontend FE-\* checks — survives the rewrite intact.
- `backend-dev-guidelines/SKILL.md` declares a `name` matching its directory.

Non-goals:

- No application code changes. This task touches `.claude/**` only.
- No automated anti-drift enforcement (CI grep guard, compile-checked example snippets). Considered and explicitly deferred; see §9.
- No change to the reviewer agents' checklist *semantics*. Checks are re-grounded in real APIs, not redesigned. Adding, removing, or reweighting checks is separate work.
- No change to `.claude/settings.json` hook wiring.
- Not a general documentation audit of `docs/`. Only the two skills are in scope.

## 3. User Stories

- As a reviewer agent, I want the checklist I am handed to describe the code I am reading, so that I can apply every item literally instead of judging which items to discard.
- As a developer reading a review, I want each reported finding to be plausibly real, so that I read the findings instead of skimming past them.
- As an implementing agent that had `backend-dev-guidelines` auto-suggested on a Go change, I want its code examples to compile against `packages/shared-go`, so that following the skill produces working code rather than code shaped like another project's.
- As the repo owner, I want the guidelines to name real files, so that when the shared server API changes I can find every doc that describes it.

## 4. Functional Requirements

### 4.1 Backend — layout and architecture

- **FR-1.1** Replace every `services/<service-name>/` path with `apps/<service-name>/`. The services are `auth-service`, `fleet-service`, `media-service`, `notification-service` (plus `apps/web`, which is not a Go service). Current drift: 3 occurrences in `scaffolding-checklist.md`.
- **FR-1.2** Document the real domain package location: `apps/<service>/internal/<domain>/`. Shared Go code lives in `packages/shared-go/<area>/`; shared DTOs in `packages/dto-go`.
- **FR-1.3** Preserve the file-role model — `model.go`, `entity.go`, `builder.go`, `processor.go`, `provider.go`, `resource.go`, `rest.go` — which is accurate. Verified against `apps/fleet-service/internal/vehicle/`.
- **FR-1.4** Correct the startup example in `architecture-overview.md`. The documented `server.New(logger).AddRouteInitializer(domain.InitializeRoutes(db)(GetServer())).Run()` does not match the real `Server` API in `packages/shared-go/server/handler.go`, which exposes `New`, `Use`, `AddRouteInitializer(func(chi.Router))`, `Router()`, and no `Run()`.

### 4.2 Backend — transport layer

This is the largest rewrite. The documented transport stack does not exist.

- **FR-2.1** Remove all api2go references (7 occurrences across 3 files). The repo has zero api2go dependencies; JSON:API types are hand-rolled in `packages/shared-go/server/jsonapi.go`.
- **FR-2.2** Remove all `jsonapi.ServerInformation` references (7 occurrences) and the `InitializeRoutes(si jsonapi.ServerInformation) func(db *gorm.DB) server.RouteInitializer` signature. The real signature takes explicit dependencies and returns a chi router initializer — e.g. `func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, primaryImage PrimaryImageSetter, statusDeps StatusDeps, record ActivityRecorder, emit EventEmitter) func(chi.Router)` (`apps/fleet-service/internal/vehicle/resource.go:29`).
- **FR-2.3** Remove the curried `server.RegisterHandler(l)(si)("name", handler)` form. `RegisterHandler` does not exist in any form. Routes are registered directly on chi: `r.Get("/fleets/{id}/vehicles", handler)`.
- **FR-2.4** Correct `RegisterInputHandler`. Real signature: `func RegisterInputHandler[T any](fn func(http.ResponseWriter, *http.Request, T)) http.HandlerFunc` (`packages/shared-go/server/handler.go:42`) — one argument, not curried. It decodes `{data:{attributes:T}}` and writes `ErrValidation` on a decode failure.
- **FR-2.5** Replace `server.MarshalResponse[T](l)(w)(c.ServerInformation())(map[string][]string{})(res)` (7 occurrences across 4 files) with the real response helpers: `server.WriteJSON(w, status, body)` and `server.WriteError(w, err)`.
- **FR-2.6** Correct the DTO contract. Documented: `Transform(Model) (RestModel, error)` and `TransformSlice([]Model) ([]RestModel, error)`, with a `RestModel` carrying a `GetName()` value receiver for api2go. Real: `Transform(m Model) server.Resource` and `TransformSlice(ms []Model) []server.Resource` — **no error return**, no `RestModel` type, no `GetName()`. The JSON:API `type` is a literal in the returned `server.Resource` (`apps/fleet-service/internal/vehicle/rest.go:58,97`). Drop the "`GetName()` MUST use a value receiver" rule entirely; it is meaningless without api2go.
- **FR-2.7** Preserve the accurate normative rules in this area: `TransformSlice` must exist alongside `Transform` and list handlers must not inline transform loops; POST/PATCH routes take a body via `RegisterInputHandler[T]`. Preserve the breaking-change warning in `patterns-rest-jsonapi.md:56` about switching a handler between body and body-less registration — the underlying hazard is real, but restate it against the real API.
- **FR-2.8** Document the request-shape conventions actually in use, drawn from `apps/fleet-service/internal/vehicle/rest.go`: narrow named `createAttributes` / `patchAttributes` structs that make derived read-only fields unbindable, and pointer fields on patch structs to distinguish absent from zero.

### 4.3 Backend — data access and composition

- **FR-3.1** Rewrite `patterns-provider.md` in full. It documents `model.Provider[T]`, `model.Map`, `model.SliceMap`, `model.ParallelMap`, `model.ErrorProvider[T]`, `model.FixedProvider`, and `database.EntityProvider[Entity]`. **None of these identifiers exist anywhere in the repository.** The real pattern is an eager `Provider` interface per domain with a `dbProvider` implementation returning `(Model, error)` — see `apps/fleet-service/internal/vehicle/provider.go`, including the `gorm.ErrRecordNotFound` → domain `ErrNotFound` translation and the `server.Page` pagination parameter.
- **FR-3.2** Remove the "Functional Composition" section of `patterns-functional.md`, whose example is not valid Go and references `model.Map` / `model.ParallelMap`. Remove `ParallelMap` from the principles list in `architecture-overview.md`.
- **FR-3.3** Correct the processor constructor pattern. Documented: `NewProcessor(l logrus.FieldLogger, ctx context.Context, db *gorm.DB)`. Real processors take a logger plus their domain collaborators — e.g. `NewProcessor(log logrus.FieldLogger, p Provider, a Administrator)` (`apps/fleet-service/internal/maintenancerecord/processor.go:19`), with optional-dependency variants via `With*` chaining or `ProcessorOption` (`apps/media-service/internal/mediaobject/processor.go:202`). The `logrus.FieldLogger`-not-`*logrus.Logger` rule is accurate and must be preserved.
- **FR-3.4** Preserve immutability and builder guidance. Both are accurate: private fields with value-receiver accessors and `With*` copy-returning mutators (`apps/fleet-service/internal/vehicle/model.go`), and a fluent `*Builder` validating in `Build() (Model, error)` (`builder.go:26`).
- **FR-3.5** Document the `db.Save` full-column-update hazard recorded at `apps/fleet-service/internal/vehicle/model.go:20-27`: a column the Model does not carry is silently zeroed on any ordinary write. This is a real, caught-in-production-code trap and belongs in the guidelines.

### 4.4 Backend — examples and domain vocabulary

- **FR-4.1** Re-ground every example in MyFleet domains. Current drift: 24 `bucket` hits across 4 files, 22 `policy` hits across 3, 2 `replication`. Sub-domain examples cite `bucketarchive`, `policyrevoke`, `replicationtrigger`; real analogues include `vehiclemedia`, `mileage`, `invite`, `purge`.
- **FR-4.2** Preserve the structural guidance those examples illustrate — layer separation for sub-domain packages, "prefer fewer well-structured packages", processor-level cross-domain orchestration, and the read-only aggregation handler exception (real analogue: `apps/fleet-service/internal/dashboard`). Only the vocabulary changes.
- **FR-4.3** Correct entity ID types where examples use `uint32`. Real entities use string UUIDs (`uuid.NewString()`, `builder.go:13`).

### 4.5 Backend — skill metadata

- **FR-5.1** Change `.claude/skills/backend-dev-guidelines/SKILL.md` frontmatter `name:` from `golang-microservice` to `backend-dev-guidelines`, matching its directory and the key already used in `.claude/skills/skill-rules.json`. `frontend-dev-guidelines/SKILL.md` is already correct.
- **FR-5.2** Verify after the rename that the skill still loads via the `Skill` tool and that the `skill-activation-prompt` hook still resolves it. If the rename breaks either, revert and record why in the audit.
- **FR-5.3** Update the `description:` in the same frontmatter, which describes "GORM entities, and JSON:API transport" via a stack that no longer matches.

### 4.6 Frontend

- **FR-6.1** Replace all Jest references with Vitest (25 occurrences across 2 files: `testing-guide.md`, `anti-patterns.md`). `apps/web` runs `vitest run` (`apps/web/package.json:8`); `@testing-library/jest-dom` is a matcher library and its import is *not* drift — do not blanket-replace the string.
- **FR-6.2** Re-ground examples. Current drift: 129 `bucket` hits across 9 files, 81 `ban` hits across 5. Real domains: vehicles, fleets, maintenance records/schedules/categories, fuel, mileage, media, invites, members, notifications.
- **FR-6.3** Correct service-layer file naming. Documented: `services/api/bans.service.ts`, `services/api/buckets.service.ts`. Real: PascalCase `VehicleService.ts`, `MediaService.ts` etc. in `apps/web/src/services/api/`. The directory path itself is accurate and must not be changed.
- **FR-6.4** Correct the barrel-export claims. `patterns-types.md:207` and `ai-guidance.md:72` instruct adding to `services/api/index.ts`; **no `index.ts` exists** in that directory. Document the real import convention instead.
- **FR-6.5** Correct `BaseService` documentation against the real abstract class (`apps/web/src/services/api/BaseService.ts`): abstract `resourceType` / `basePath`, `ListResult<A>`, `listAt` / `createAt` protected helpers, types from `@myfleet/shared-ts`, and the gateway-path constraint (`apiClient.baseUrl` is `''`, so paths must be absolute).
- **FR-6.6** Audit the remaining frontend resource files — `patterns-react-query.md`, `patterns-forms-validation.md`, `patterns-styling.md`, `patterns-routing.md`, `patterns-components.md`, `patterns-api-client.md`, `architecture-overview.md` — against `apps/web/src`, correcting anything that fails the §4.7 evidence standard. These were not individually verified during specification; the design phase must inventory them.
- **FR-6.7** Preserve the FE-\* checklist semantics referenced by `.claude/agents/frontend-guidelines-reviewer.md`. FE-11's "Service extends `BaseService`" is accurate and stays.

### 4.7 Evidence standard (applies to all of the above)

- **FR-7.1** Every code example must be copied or minimally adapted from real repository source, not written from the pattern description.
- **FR-7.2** Every Go example must compile against the real `packages/shared-go` API. Verification is by inspection against source; no build harness is added (§2 non-goals).
- **FR-7.3** Where a rule is subtle or was learned from a real defect, cite the `file:line` it came from. Where a rule is general, no citation is needed.
- **FR-7.4** No rule may be stated that cannot be checked against something in this tree. If a documented practice is aspirational rather than current, either implement it or drop it — do not leave it as a checklist item.

## 5. API Surface

None. This task changes no runtime code and no HTTP contract.

## 6. Data Model

None.

## 7. Service Impact

| Path | Change |
| --- | --- |
| `.claude/skills/backend-dev-guidelines/SKILL.md` | `name` + `description` frontmatter fix; checklist re-grounding |
| `.claude/skills/backend-dev-guidelines/resources/architecture-overview.md` | Layout, startup example, remove `ParallelMap` |
| `.claude/skills/backend-dev-guidelines/resources/file-responsibilities.md` | Route-registration and DTO contracts |
| `.claude/skills/backend-dev-guidelines/resources/scaffolding-checklist.md` | `services/` → `apps/`, Dockerfile path, checklist rows |
| `.claude/skills/backend-dev-guidelines/resources/patterns-rest-jsonapi.md` | Largest rewrite — 444 lines built on api2go |
| `.claude/skills/backend-dev-guidelines/resources/patterns-provider.md` | Full rewrite — documents a nonexistent library |
| `.claude/skills/backend-dev-guidelines/resources/patterns-functional.md` | Drop functional-composition section, fix processor constructor |
| `.claude/skills/backend-dev-guidelines/resources/anti-patterns.md` | Re-ground rows citing `RegisterHandler` / api2go |
| `.claude/skills/backend-dev-guidelines/resources/ai-guidance.md` | Re-ground the commonly-missed checklist |
| `.claude/skills/backend-dev-guidelines/resources/testing-guide.md` | Audit against real `*_test.go` conventions |
| `.claude/skills/frontend-dev-guidelines/**` | Vitest, domain vocabulary, service-layer naming, `index.ts` claims |

No Go or TypeScript source is modified. No deployment manifests are affected.

## 8. Non-Functional Requirements

- **Reviewability.** The diff will be large and mostly prose. Commits should be split per resource file so each is reviewable independently.
- **Token budget.** These resources are loaded into agent context on every Go/TS change. The rewrite should not materially grow total size; deleting the nonexistent-library content should offset additions.
- **No regression in coverage.** The rewrite must not silently drop a check. The design phase produces an inventory of every current normative rule with a keep/fix/delete disposition, and each deletion needs a stated reason.
- **Security.** The SEC-\* checks are the most consequential items in the backend checklist and the ones most damaged by being buried. They must be verified against the real `server.WriteError` redaction behavior (`packages/shared-go/server/jsonapi.go`), which already implements the 5xx-redaction rule SEC-09 describes.

## 9. Open Questions

- **The reviewer agent definitions carry the same drift.** `.claude/agents/backend-guidelines-reviewer.md` refers to `services/auth-service` at lines 7, 9, 24, and 39. Scoping deliberately limited this task to the two skills, but leaving the consuming agent pointing at a directory that does not exist reproduces the exact defect being fixed, in the file that reads the checklist. Recommend folding the path corrections in; the alternative is a follow-up issue filed at merge time. **Decide during design.**
- Should `patterns-provider.md` be deleted outright rather than rewritten? Its subject — a lazy composition library — has no analogue here, and the real provider pattern is small enough to fold into `patterns-functional.md` or `file-responsibilities.md`. Fewer files means less to drift.
- The `notes.customization` block in `skill-rules.json` still carries the prior project's example values (`blog-api`). Cosmetic; include only if free.
- `testing-guide.md` (both skills) was not verified during specification. Its drift magnitude is unknown until the design phase inventories it, which may change the size estimate for this task.

## 10. Acceptance Criteria

Mechanical checks — each must return zero matches under `.claude/skills/`:

- [ ] `grep -ri "api2go" .claude/skills/`
- [ ] `grep -r "ServerInformation" .claude/skills/`
- [ ] `grep -r "MarshalResponse" .claude/skills/`
- [ ] `grep -rE "model\.(Provider|Map|SliceMap|ParallelMap|ErrorProvider|FixedProvider)" .claude/skills/`
- [ ] `grep -r "EntityProvider" .claude/skills/`
- [ ] `grep -r "RouteInitializer" .claude/skills/`
- [ ] `grep -rE "RegisterHandler\(" .claude/skills/`
- [ ] `grep -rE "\bservices/[a-z-]+-service" .claude/skills/` (guards `services/<svc>/`; must not flag the legitimate frontend `services/api/`)
- [ ] `grep -riE "\bjest\b" .claude/skills/frontend-dev-guidelines/` — except `@testing-library/jest-dom` imports
- [ ] `grep -riE "\b(bucket|ban|policy|replication)s?\b" .claude/skills/` returns no *example* usage (incidental prose use of "policy" is acceptable)

Substantive checks:

- [ ] Every Go code block in `backend-dev-guidelines/resources/` uses only identifiers that exist in `packages/shared-go` or a real `apps/*/internal/*` package, verified by grep per identifier.
- [ ] `backend-dev-guidelines/SKILL.md` frontmatter reads `name: backend-dev-guidelines`; the skill loads via the `Skill` tool and the `skill-activation-prompt` hook still fires on a Go file change.
- [ ] Every example service path resolves to a real directory under `apps/`.
- [ ] The design-phase rule inventory shows a keep/fix/delete disposition for every current normative rule, with a reason recorded for each deletion.
- [ ] `backend-guidelines-reviewer` and `frontend-guidelines-reviewer` are run against a recent merged task's diff (task-017 is a good frontend candidate) and neither sets any checklist item aside as inapplicable, nor emits a finding traceable to guideline drift.
- [ ] `make ci` passes — expected trivially, since no source changes, but it confirms nothing was touched by accident.
- [ ] No file under `apps/`, `packages/`, or `deploy/` appears in the branch diff.
