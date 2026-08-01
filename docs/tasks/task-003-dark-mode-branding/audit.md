# Code review — task-003 dark mode & branding

Branch `task-003-dark-mode-branding`, 352e8c1..41c5d3b. Three reviewers dispatched per CLAUDE.md; findings consolidated from the per-reviewer files in this folder.

---

# Plan Audit — task-003-dark-mode-branding

**Plan Path:** `docs/tasks/task-003-dark-mode-branding/plan.md`
**Audit Date:** 2026-07-31
**Branch:** `task-003-dark-mode-branding`
**Base Branch:** `main` (diff range `352e8c1..41c5d3b`, 25 commits, 60 files)
**Scope:** plan adherence only — read-only, no files mutated other than this report.

## Executive Summary

All 17 tasks in `plan.md` were implemented. Every file the plan's File Structure
table names exists with the content the plan specified, every named symbol is
present, and every test the plan wrote out verbatim is in the tree and enumerated
below. Task 17 is `PARTIAL` by design: its steps 4–5 need a browser, were not
executed, and the checklist is recorded at
`docs/tasks/task-003-dark-mode-branding/manual-verification.md`. Nothing was
silently skipped, stubbed, or deferred without a record.

No Critical or Important findings. Two Low findings, both cosmetic/process. The
branch ships several things beyond the plan — all additive test coverage and one
accessibility fix — itemised in "Shipped beyond the plan" below.

## Task Completion

| # | Task | Status | Evidence |
|---|---|---|---|
| 1 | Theme allow-list | DONE | `apps/auth-service/internal/user/theme.go:7-18` (constants + `IsValidTheme`); test `theme_test.go:5-26`, 7 subtests, matches plan verbatim |
| 2 | Persist preference on user record | DONE | `model.go:13` (`ErrInvalidTheme`), `model.go:22,31,43-46`; `entity.go:19` (column), `entity.go:29-38` (`Make` normalisation), `entity.go:40-42` (`ToEntity`); `builder.go:13,20`; test DDL `provider_test.go:44`; tests `entity_test.go:8-69` (5 tests as specified) |
| 3 | `Processor.UpdateTheme` | DONE | `processor.go:54-63` — validate-before-read exactly as planned; tests `processor_test.go:131-178` (3 tests, 6 subtests) |
| 4 | Expose `themePreference` on the resource | DONE | `rest.go:11,15`; harness generalised at `resource_test.go:31-42` (`newAuthRouter`) and `:46-62` (`serve`); FR-TEST-3 regression `resource_test.go:93-117` |
| 5 | `PATCH /auth/me` | DONE | `resource.go:28-29` (`errThemeValidation` wrapping `server.ErrValidation`), `resource.go:76-102` (route on the JWT group, no user identifier in path/body); tests `resource_test.go:119-197` (4 tests incl. the 422 matrix and the no-echo title check) |
| 6 | Semantic status tokens | DONE | `apps/web/src/index.css:26-49` (light) and `:72-94` (dark) — all 32 values byte-identical to the plan; `tailwind.config.ts:51-75`; `types/models/user.ts:5-6,15`; `contrast.md` created. Independently confirmed the classes actually compile: `.bg-danger-subtle`, `.text-danger-subtle-foreground`, `.border-danger-border`, `.text-success`, `.text-warning`, `.bg-{success,warning,info}-subtle`, `.text-info-subtle-foreground` all present in `apps/web/dist/assets/index-5mURdli4.css` |
| 7 | Pure theme helpers | DONE | `src/lib/theme.ts:13,18,22-24,34-41,44-52,58-61,68-70` — all 7 exports; tests `theme.test.ts`, 11 cases matching the plan's 11 |
| 8 | `ThemeContext` | DONE | `src/context/ThemeContext.tsx:43-101` (`hasLocalOverride` ref at `:55`, unconditional media subscription at `:66-71`); `matchMedia` stub `src/test/setup.ts:34-84` incl. `setPrefersDark`/`resetMatchMedia`/`mediaListenerCount`; tests `ThemeContext.test.tsx`, 9 cases matching the plan's 9 |
| 9 | `useUpdateTheme` | DONE | `src/lib/hooks/api/auth.ts:65-74` (token-absent resolves `null`, no request) and `:89-106` (`onMutate` cache write, **no** `onError` rollback, as required by FR-PERSIST-5); tests `auth.test.ts`, 3 cases |
| 10 | `ThemeToggle` | DONE | `src/components/ThemeToggle.tsx:9-13` (cycle), `:17-21` (icon keyed on preference), `:40-47` (state change + mutation + toast), `:50-59` (`variant="ghost"`, `aria-label`, `title`); tests `ThemeToggle.test.tsx`, 5 cases |
| 11 | `ThemeSync` and the provider tree | DONE | `src/components/ThemeSync.tsx:14-51` (validates the wire value at `:36`, clears the override on sign-out at `:40-48`, renders `null`); tree `AppProviders.tsx:44-54` matches the required nesting exactly, `ThemedToaster` at `:17-20`; tests `ThemeSync.test.tsx`, 5 cases (plan specified 4 — see additions below) |
| 12 | Pre-paint script + first guard test | DONE | `apps/web/index.html:33-42` — synchronous, inline, `try/catch`, narrows to `system`, class name is a literal, placed before `type="module"` at `:46`; guards `conventions.test.ts:16-69`, 6 cases (plan specified 5) |
| 13 | Icon generator | DONE | `tools/generate-icons.py` (matches plan text apart from the documented budget-scoping fix in `e1628c5`); `tools/generate-icons.sh` matches plan text **byte-for-byte**; all six assets committed under `apps/web/public/` totalling 37,245 B (36.4 KiB) against the 100 KB budget; `src/components/brandMarkPath.ts:6-7` generated and its `d` string appears verbatim in `public/favicon.svg:6` |
| 14 | `BrandMark`, manifest, `<head>` wiring | DONE | `index.html:7-10` (icon/alternate/apple-touch/manifest links) and `:18-19` (both `theme-color` metas); `public/site.webmanifest` incl. the maskable entry; `src/components/BrandMark.tsx:15-21` (`currentColor`, `aria-hidden`, no hardcoded px); guards `conventions.test.ts:74-108` |
| 15 | `AppLayout` — tokens, brand mark, toggle | DONE | `src/components/AppLayout.tsx:28` (`bg-card` sidebar with the plan's rationale comment retained), `:30` (`<BrandMark className="h-5 w-5" />`), `:43-44` (`bg-accent` active / hover), `:59` (`<ThemeToggle />` immediately left of Sign out), `:60` (`variant="outline"` Button). No palette classes in the file |
| 16 | Convert remaining hardcoded colours | DONE | All 8 files converted: `PlaceholderPage.tsx:7`, `ActivityEventIcon.tsx:15`, `SeverityChip.tsx:10-24`, `MaintenanceQueueView.tsx:35,38,47,55,60,75`, `FleetOverviewWidget.tsx:27-40`, `OverdueMaintenanceWidget.tsx:28`, `UpcomingMaintenanceWidget.tsx:28`, `packages/ui-components/src/StatusBadge.tsx:3-13`. Guard `conventions.test.ts:113-154`. The stale "no shadcn semantic equivalent" comment FR-CONVERT-3 required removing is gone (`SeverityChip.tsx:10-12`) |
| 17 | Full verification | PARTIAL *(by design)* | Steps 1–3 and 6 done: `make ci` green, container serves all seven asset paths with 200 + non-HTML content types, asset budget 37,245 < 102,400 B, `git status` clean. `CLAUDE.md` correctly left unchanged (plan says skip unless a build command changed — none did). Steps 4–5 need a browser, were **not** executed, and are recorded as a checklist at `manual-verification.md:1-116` |

**Completion Rate:** 17/17 tasks (100%); 16 `DONE`, 1 `PARTIAL` by design
**Skipped without approval:** 0
**Partial implementations:** 1 (Task 17, steps 4–5, deferred with a recorded checklist)

## Skipped / Deferred Tasks

**Task 17, steps 4 (visual pass) and 5 (cross-device persistence check)** — not
executed; they require a browser and a running backend. `manual-verification.md`
records both as an explicit, per-item checklist with pass/fail criteria. This is
the only outstanding gate on the branch. Impact: the token conversions, the
Radix `select` dark-mode surface, the toast theming (FR-3P-1/2/3), and the
cross-profile persistence proof rest on reasoning and unit tests rather than on
observation. Nothing else in the plan depends on them.

## Accepted Deviations — Verified as Claimed

Each of the eight deviations supplied with this audit was checked against source;
all eight are what they claim to be.

1. **422 not 400.** `resource.go:28-29` wraps `server.ErrValidation`;
   `resource_test.go:157-162` asserts 422 for all four bad-input shapes.
   `packages/shared-go` is absent from the branch diff — unmodified as stated.
2. **`administrator.go` unchanged.** Absent from the diff; `administrator.go:24-30`
   already does `db.Save(&e)` and `ToEntity` (`entity.go:41`) carries the column.
3. **`ThemeSync` bridges.** `ThemeSync.tsx:14-51`; neither `AuthContext` nor
   `ThemeContext` imports the other.
4. **`NewBuilder()` seeds the default.** `builder.go:13`; pinned by
   `entity_test.go:65-69`.
5. **`generate-icons.sh` prefers Python.** Script matches the plan's text exactly;
   the rationale comment (`generate-icons.sh:6-13`) explains the inversion of
   design §8.2.
6. **Sidebar uses `bg-card`.** `AppLayout.tsx:28`, with the plan's "do not
   simplify back to `bg-muted`" comment preserved at `:23-27`.
7. **`@types/node` devDependency.** `apps/web/package.json` devDependencies;
   type-only, required by `conventions.test.ts:1-3`.
8. **Task 17 manual pass deferred.** `manual-verification.md`.

## Shipped Beyond the Plan

All additive, none contradicting the plan. Listed for completeness, not as findings.

| Addition | Location | Note |
|---|---|---|
| `AppLayout.test.tsx` (4 tests) | `apps/web/src/components/AppLayout.test.tsx` | Plan wrote no test for Task 15 |
| `AppProviders.test.tsx` (1 test) | `apps/web/src/components/providers/AppProviders.test.tsx:26-56` | Renders the real provider tree end to end; catches a tree reordering that `ThemeSync.test.tsx` cannot, because that file mocks `useAuth` |
| Extra `ThemeSync` sign-out test | `ThemeSync.test.tsx:104-143` | Exercises the real signed-in → signed-out → signed-in transition |
| `MEDIA_QUERY` drift guard | `ThemeContext.tsx:22`, `conventions.test.ts:63-68` | Also replaces the test's hardcoded `'myfleet.theme'` with an import of `THEME_STORAGE_KEY` (`conventions.test.ts:5,20,25`), closing a gap where a storage-key rename would have silently reinstated the flash |
| Empty-root guard in the palette scan | `conventions.test.ts:132-139` | Prevents a moved directory from making the FR-CONVERT-10 assertion pass trivially |
| AA fix: overdue-row body text | `MaintenanceQueueView.tsx:55,60` | `text-muted-foreground` on `bg-danger-subtle` measured 3.89:1 (AA fail) → `text-danger-subtle-foreground` at 6.80:1/8.14:1; recorded in `contrast.md:33-50`. A genuine defect the plan's own conversion would have shipped |
| Toast copy change | `ThemeToggle.tsx:44`, test `:89` | "next time you sign in" → "next time you reload". Also more accurate: `writeCachedTheme` persists the failed choice locally, so it is the next `ThemeSync` adoption on reload — not sign-in — that reverts it. Code and test changed together |
| `ThemeSync.wasSignedIn` moved to its own effect | `ThemeSync.tsx:29-31` | Plan set the flag inside the adopt effect; a user with a corrupted stored preference would then sign out without ever setting it, leaving a stale override to suppress adoption on the next sign-in. Rationale is in the comment at `:18-24` |
| `generate-icons.py` budget scoping | `tools/generate-icons.py:139-150` | FR-PERF-4 total now sums the six icon files rather than everything in `public/` |

## Build & Test Results

Not re-run — the controller's verification is authoritative and no doubt arose
that it does not already answer.

| Component | Build | Tests | Vet/Lint | Notes |
|---|---|---|---|---|
| `apps/auth-service` | PASS | PASS | PASS | via `make ci` |
| `apps/web` | PASS | PASS (115/115) | PASS | via `make ci` |
| `packages/shared-ts` | PASS | PASS (7/7) | PASS | via `make ci` |
| `deploy/k8s` manifests | PASS | n/a | n/a | `make ci` manifests target; no manifest change expected or made |
| Container asset serving | PASS | n/a | n/a | all 7 paths 200 + non-HTML content type |

One independent check was run for this audit (read-only): the semantic token
classes are confirmed present in the built stylesheet
`apps/web/dist/assets/index-5mURdli4.css`, which proves the Tailwind
registration in Task 6 reaches both `apps/web/src` and the
`packages/ui-components/src` content glob — `bg-success-subtle` originates only
in `StatusBadge.tsx`, so its presence in the bundle demonstrates the cross-package
glob works. No test covers that.

## Findings

### Low — plan checkboxes never marked complete

All 96 `- [ ]` step checkboxes in `plan.md` remain unchecked despite the work
being finished (`plan.md:82` through `plan.md:3322`). This is a documentation
hygiene issue: a reader arriving at the plan cold cannot tell the branch is done,
and a future `/execute-task` resumption would have no progress signal. No code
impact.

### Low — `tools/generate-icons.py` is not executable

`tools/generate-icons.py` is mode `0644` but carries a `#!/usr/bin/env python3`
shebang (`generate-icons.py:1`), so invoking it directly fails. Harmless in
practice: the only supported entry point is `tools/generate-icons.sh` (mode
`0755`), which calls `exec python3 "$ROOT/tools/generate-icons.py"`
(`generate-icons.sh:24`) and never needs the bit. Plan Task 13 step 3 only
chmod'd the shell script, so this matches the plan as written.

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE *(conditional on the manual browser pass)*

The implementation is a faithful execution of the plan. Where it departs from the
plan's literal text it does so in one direction only — additional test coverage,
one real accessibility fix, and guards that close gaps the plan's own tests left
open — and every departure carries an in-code rationale and a matching commit
message. The only thing standing between this branch and "genuinely done" is the
browser-dependent Task 17 checklist, which is honestly recorded rather than
quietly dropped.

## Action Items

1. Execute `manual-verification.md` — the Task 17 step 4 visual pass and step 5
   cross-device persistence check — before merging. This is the sole outstanding
   plan item.
2. *(Optional, hygiene)* Tick the completed checkboxes in `plan.md` so the
   document reflects the branch state.
3. *(Optional, cosmetic)* `chmod +x tools/generate-icons.py`, or drop its
   shebang, so the file's mode and its first line agree.

---

# Backend Audit — auth-service (task-003-dark-mode-branding)

- **Service Path:** `apps/auth-service`
- **Scope:** `apps/auth-service/internal/user/` (diff `352e8c1..41c5d3b`, Go files only)
- **Guidelines Source:** `backend-dev-guidelines` skill
- **Date:** 2026-07-31
- **Build:** PASS (`make ci` exit 0 — verified by controller, not re-run)
- **Tests:** all Go packages pass (verified by controller, not re-run)
- **Overall:** NEEDS-WORK

## Build & Test Results

Per the controller: `make ci` exits 0 — `go vet`, `go build`, `lint-check`, and every
Go package's tests pass. Not re-run here. One additional **audit probe** was run
against `internal/user` via `go test -overlay=` (working tree untouched, `git status
--porcelain` clean afterwards) to settle a specific doubt about the write path. Its
result is finding B-1 below.

## Package Classification (Phase 2)

| Package | Files | Classification | In scope |
|---------|-------|----------------|----------|
| `internal/user` | `model.go` present | Domain package — full DOM checklist | YES |
| `internal/session` | `model.go` present | Domain package | no (unchanged) |
| `internal/jwks` | no `model.go`/`resource.go` model | Support (key material + JWKS route) | no (unchanged) |
| `internal/oidc` | no `model.go` | Support (OAuth orchestration) | no (unchanged) |
| `internal/membership` | `client.go` only | Support (HTTP client to fleet-service) | no (unchanged) |

No sub-domain (action-event) packages in scope, so the SUB-* checklist does not apply.

## Domain Checklist Results

### internal/user

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists with `NewBuilder()`, fluent setters, `Build()` **with validation** | FAIL | `apps/auth-service/internal/user/builder.go:12` `NewBuilder()` and `:16-20` setters present, but `:21` `func (b *Builder) Build() Model { return b.m }` performs **zero** validation. `SetThemePreference` (`:20`) will place any string into the Model unchecked. Pre-existing shape; the new setter widens it. |
| DOM-02 | `ToEntity()` method | PASS | `apps/auth-service/internal/user/entity.go:40` `func (m Model) ToEntity() Entity`; carries `ThemePreference: m.themePreference` at `:41`. |
| DOM-03 | `Make(Entity)` function | WARN | `apps/auth-service/internal/user/entity.go:29` `func Make(e Entity) Model` — exists, but returns `Model` not `(Model, error)` as `file-responsibilities.md:18` specifies. Repo-wide convention (all 8 domains across fleet/media/notification services do the same). Pre-existing; not introduced by this diff. |
| DOM-04 | `Transform` function | PASS | `apps/auth-service/internal/user/rest.go:14`. |
| DOM-05 | `TransformSlice` function; no inline loops in `resource.go` | PASS | `apps/auth-service/internal/user/rest.go:18`. No list endpoint in this domain; `resource.go` contains no transform loop (`resource.go:64`, `:101` call `Transform` directly on a single model). |
| DOM-06 | Processor accepts `logrus.FieldLogger` | PASS | `apps/auth-service/internal/user/processor.go:16` `func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator)`; field typed `logrus.FieldLogger` at `:11`. Not `*logrus.Logger`. |
| DOM-07 | Handlers do not use `logrus.StandardLogger()` | PASS | `grep -n "StandardLogger" apps/auth-service/internal/user/*.go` → no matches. The injected `log logrus.FieldLogger` from `resource.go:34` is threaded to the processor at `resource.go:35` and used at `:56`, `:92`. (This codebase's `shared-go/server` has no `HandlerDependency`/`d.Logger()`; injection at `InitializeRoutes` is the house equivalent.) |
| DOM-08 | PATCH uses `RegisterInputHandler` | PASS | `apps/auth-service/internal/user/resource.go:76` `r.Patch("/auth/me", server.RegisterInputHandler(func(...)))`. The only other route (`:37`) is a GET and correctly uses a plain handler. No POST in this domain. |
| DOM-09 | Transform errors handled | PASS | `Transform` returns a single `server.Resource` with no error (`rest.go:14`), so there is no error to discard. `grep` for `Transform(` in `resource.go` → `:64`, `:101`; neither uses `_`. |
| DOM-10 | Providers use lazy evaluation | WARN | `apps/auth-service/internal/user/provider.go:35` and `:48` wrap the query in `database.Query(...)` but **immediately invoke it** — note the trailing `()` at `provider.go:44` and `:58`. Evaluation is eager despite the wrapper. Pre-existing; `provider.go` is unchanged by this diff. |
| DOM-11 | No `os.Getenv()` in handlers | PASS | `grep -n "os.Getenv" apps/auth-service/internal/user/*.go` → no matches. |
| DOM-12 | No cross-domain logic in handlers | PASS | `apps/auth-service/internal/user/resource.go:44` and `:81` call only `proc.*` on the `user` domain's own processor. The only non-user data touched is `id.ActiveFleetID`/`id.Role` (`:65`), read from the already-validated token, not fetched. |
| DOM-13 | Handlers don't call providers directly | PASS | `resource.go:44` `proc.GetByID(...)`, `resource.go:81` `proc.UpdateTheme(...)`. `NewProvider`/`NewAdministrator` appear only as constructor wiring at `resource.go:35`, never as call sites in a handler body. |
| DOM-14 | No direct entity writes in handlers | PASS | `grep -n "db\.Create\|db\.Save\|db\.Delete\|db\.Where" apps/auth-service/internal/user/resource.go` → no matches. |
| DOM-15 | `administrator.go` exists and is the sole write path | FAIL | The file exists (`apps/auth-service/internal/user/administrator.go:14`) and `processor.go:62` correctly routes the write through it. But the write it performs **destroys `created_at` on every PATCH** — see finding B-1. Reusing `Update(Model)` unchanged is the design decision that fails here. |
| DOM-16 | Domain error → HTTP status mapping | WARN | `resource.go:84-86` validation→**422** (not the guideline's 400), `:88-90` not-found→404, `:92-96` else→500. Deviation is documented and coherent (`docs/tasks/task-003-dark-mode-branding/design.md:37-50`, `plan.md:16`) — `shared-go/server/errors.go:5-13` has no 400 sentinel and `handler.go:50` already emits 422 for a malformed body, so 400-for-bad-enum next to 422-for-bad-JSON would be incoherent. **But** `prd.md:344-345` acceptance criteria still say 400 and were never amended. Spec drift, not a code defect. |
| DOM-17 | JSON:API `GetName()`/`GetID()`/`SetID()` on REST models | WARN | Absent. `apps/auth-service/internal/user/rest.go:14` returns `server.Resource{Type, ID, Attributes}` (`shared-go/server/jsonapi.go:9-14`) instead. This is the convention in every domain in the repo (`fleet`, `vehicle`, `invite`, `mediaobject`, `notification`, `preferences`, …); the guideline resource describes an api2go stack `shared-go` does not use. Pre-existing, not a regression from this diff. |
| DOM-18 | Request models flat, defined in `rest.go` | FAIL | The PATCH request model is an **anonymous struct declared inline in the handler signature** at `apps/auth-service/internal/user/resource.go:76-78`. The shape is flat (correct — `RegisterInputHandler` strips the envelope), but `file-responsibilities.md:118` requires request models to be defined in `rest.go`. `rest.go:5-6` even claims to be the mirror of `apps/web/src/types/models/user.ts`, yet the request contract is not there. |
| DOM-19 | Table-driven tests | PASS | `theme_test.go:5-25`, `entity_test.go:9-30`, `resource_test.go` (`TestPatchMe_rejectsInvalidValues`, `tests := []struct{...}` + `t.Run`). `processor_test.go` uses `for … range []string{…}` + `t.Run` — same shape. |

## Security Review

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validation uses verified parsing | PASS | `packages/shared-go/auth/middleware.go:24` `jwt.ParseWithClaims(raw, claims, keyfn, jwt.WithValidMethods([]string{"RS256"}))`, with `!tok.Valid` rejected at `:25`. `grep -rn "ParseUnverified"` over the repo → no matches. Algorithm confusion is blocked by the explicit `WithValidMethods`. |
| SEC-02 | No claims extracted from unvalidated tokens | PASS | `middleware.go:29-34` builds `Identity` only after the `:25` validity gate. `resource.go:80` reads it via `auth.IdentityFromContext`, which returns a zero `Identity` when absent (`packages/shared-go/auth/*.go:20-24`) — and a zero `UserID` resolves to `ErrNotFound`→404, never to another user. |
| SEC-03 | No open redirect | PASS (n/a) | No redirect in the changed files. `resource.go` writes only `server.WriteJSON`/`server.WriteError`; no `Location` header, no `http.Redirect`. |
| SEC-04 | Secrets not hardcoded | PASS | `grep -niE "secret\|password\|apikey\|private_key" apps/auth-service/internal/user/*.go` → no matches. |
| FR-SEC-1 | Horizontal privilege escalation on `PATCH /auth/me` | PASS | The route is mounted **inside** the JWT-protected group: `apps/auth-service/cmd/main.go:91-96` (`pr.Use(authmw.JWT(ks.Keyfunc()))` then `user.InitializeRoutes(log, db)(pr)`). The target user is `id.UserID` (`resource.go:80-81`) and nothing else — the path `/auth/me` has no parameter, the typed input struct (`resource.go:77`) has exactly one field, and no query parameter is read. **Probed:** a body carrying an extra `"email":"attacker@evil.com"` attribute is silently ignored by the decoder — a follow-up GET still returned `"email":"a@b.com"`. No mass assignment. |
| FR-SEC-3 | Persistence failure renders the sentinel only | PASS | `resource.go:96` `server.WriteError(w, errInternal)`, where `errInternal` is `errors.New("internal server error")` at `resource.go:19`. The raw driver/GORM error reaches only the log line at `resource.go:92`. `server.WriteError` copies `err.Error()` into `Title` (`shared-go/server/jsonapi.go:36`), so this is the correct discipline. |
| FR-SEC-3 | Validation sentinel is a compile-time-constant message | PASS | `resource.go:28-29` — package-level `var errThemeValidation = fmt.Errorf("%w: themePreference must be one of light, dark, system", server.ErrValidation)`. The only verb is `%w` on a sentinel; no caller-controlled `%s`/`%v`. `errors.Is` still reaches `server.ErrValidation`, so `StatusFor` (`shared-go/server/errors.go:29-30`) yields 422. Regression-tested at `resource_test.go` `TestPatchMe_validationTitleNamesTheFieldNotTheInput`. |
| FR-DATA-3 | `themePreference` never enters the JWT | PASS | `apps/auth-service/internal/session/processor.go:61-70` — `MintAccess` claims are exactly `sub`, `email`, `active_fleet_id`, `role`, `iss`, `aud`, `iat`, `exp`. `grep -rn "ThemePreference\|themePreference\|theme_preference" --include="*.go"` outside `internal/user/` → no matches. |
| — | `sub` overloading / login loop | PASS | The update path is `resource.go:81` → `processor.go:58` `pr.p.GetByID(userID)` → `provider.go:37` `s.db.Where("id = ?", id)`. `GetBySub` (`provider.go:48`, filtering `google_sub`) is reached only from `ProvisionFromGoogle` (`processor.go:34`). Guarded by `resource_test.go` `TestPatchMe_unknownUserIsNotFound` and `TestAuthMe_doesNotResolveAGoogleSub`. |

## Findings

### B-1 (Blocking) — `PATCH /auth/me` zeroes `created_at` on the user row

**Files:** `apps/auth-service/internal/user/administrator.go:24-29`, `apps/auth-service/internal/user/entity.go:40-42`

`Administrator.Update` builds a full `Entity` from `m.ToEntity()` and calls `db.Save(&e)`.
`ToEntity()` (`entity.go:41`) does not carry `CreatedAt` (the field does not exist on
`Model` at all — `model.go:16-24`), so the entity handed to `Save` has
`CreatedAt` at its zero value. GORM's `Save` on a struct with a non-zero primary key
issues a **full-column UPDATE**, writing `created_at = 0001-01-01 00:00:00`.

Verified empirically with an overlay probe against the package's own SQLite harness:

```
BEFORE: created_at=2026-07-31 16:14:52.636659128 -0400
PATCH  /auth/me {"themePreference":"dark"} -> 200
AFTER : created_at=0001-01-01 00:00:00 +0000 UTC
```

The stated design rationale — *"no new `Administrator` method: `Update(Model)` already
does a full `db.Save(&e)` and `ToEntity` carries the new column"* — is right about
`theme_preference` and wrong about the blast radius. The same full save also overwrites
a column `ToEntity` does **not** carry.

The root cause is pre-existing (`ProvisionFromGoogle` → `Update` at `processor.go:43`
already did this on every login), but this change converts it from a
login-frequency side effect into an endpoint any authenticated user can hit at will,
and the new tests do not cover it: `resource_test.go`
`TestPatchMe_persistsAndEchoesTheNewPreference` asserts only on `theme_preference`.

**Failure scenario:** a user toggles dark mode; the account's `created_at` becomes the
zero date. Any report, sort, retention rule, or cohort query keyed on `created_at`
silently misclassifies the account.

**Remedies (either):**
1. Give `Administrator` a scoped write — `db.Model(&Entity{}).Where("id = ?", id).Update("theme_preference", pref)` — so the PATCH touches one column, or
2. Carry `CreatedAt` on `Model`/`ToEntity()` so the full save round-trips it.

Option 1 also fixes the pre-existing login-path corruption only if `Update` is
similarly narrowed; option 2 fixes both at once.

### B-2 (Moderate) — PATCH request model is an anonymous inline struct in `resource.go`

**File:** `apps/auth-service/internal/user/resource.go:76-78`

```go
r.Patch("/auth/me", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
    ThemePreference string `json:"themePreference"`
},
) {
```

`file-responsibilities.md:118` puts request models in `rest.go` ("Define request models
(`CreateRequest`, `UpdateRequest`)"). The struct is flat, which is the part that matters
for correctness, but as an anonymous type it cannot be referenced by a test, cannot be
named in the `rest.go:5-6` frontend-contract comment, and duplicates the `json` tag that
`Attributes` (`rest.go:11`) already declares. A named `UpdateRequest` in `rest.go`
resolves all three.

### B-3 (Minor) — `Builder.SetThemePreference` is dead code, and `Build()` validates nothing

**File:** `apps/auth-service/internal/user/builder.go:20-21`

`grep -rn "SetThemePreference" --include="*.go" .` returns exactly one line — the
declaration itself. Zero callers. `anti-patterns.md:162` flags leftover unreferenced
symbols. Compounding it, `Build()` (`builder.go:21`) performs no validation, so this
setter is a path that can place an out-of-allow-list value into a `Model` that
`Administrator.Insert` would persist; `IsValidTheme` is enforced only at
`processor.go:55`. Either delete the setter or have `Build()` reject an invalid theme
(which would also close DOM-01).

### B-4 (Minor, docs) — PRD acceptance criteria still specify 400

**File:** `docs/tasks/task-003-dark-mode-branding/prd.md:344-345`

The 422 decision is sound and well documented (`design.md:37-50`, `plan.md:16`,
`context.md:96-102`) and is applied consistently — `shared-go/server/handler.go:50`
emits 422 for a malformed body and `resource.go:85` emits 422 for a bad enum, so there
is no split-brain. But the PRD's checklist still reads 400 and was never amended, which
will read as an unmet acceptance criterion to anyone checking the PRD alone.

## Summary

### Blocking (must fix)
- **B-1 / DOM-15** — `apps/auth-service/internal/user/administrator.go:24-29`: `db.Save(m.ToEntity())` writes a zero `created_at` because `ToEntity()` (`entity.go:41`) does not carry it. Empirically confirmed: `PATCH /auth/me` turns `created_at` into `0001-01-01`.
- **B-2 / DOM-18** — `apps/auth-service/internal/user/resource.go:76-78`: request model is an anonymous inline struct, not a named type in `rest.go`.

### Non-Blocking (should fix)
- **B-3 / DOM-01** — `builder.go:20` `SetThemePreference` has zero callers; `builder.go:21` `Build()` validates nothing.
- **B-4 / DOM-16** — `prd.md:344-345` still says 400; the 422 decision is correct but the PRD was not amended.
- **DOM-03** — `entity.go:29` `Make` returns `Model`, not `(Model, error)` per `file-responsibilities.md:18`. Repo-wide convention; pre-existing.
- **DOM-10** — `provider.go:44`, `:58` invoke `database.Query(...)` immediately; evaluation is eager despite the lazy wrapper. Pre-existing, file unchanged.
- **DOM-17** — `rest.go` uses `server.Resource` rather than `GetName()`/`GetID()`/`SetID()`. Repo-wide convention; the guideline describes an api2go stack `shared-go` does not use.
- **Context propagation** — `provider.go:37`, `administrator.go:18`, `:26` use the bare `db` rather than `db.WithContext(ctx)` (`testing-guide.md:593`). The processor is also constructed once at route-init (`resource.go:35`) and shared across requests, so no per-request context or logger can reach the data layer. Pre-existing; the new PATCH path inherits it.

---

# Frontend Audit — task-003-dark-mode-branding

- **Audit Scope:** `git diff 352e8c1..41c5d3b -- 'apps/web/**' 'packages/ui-components/**'` (28 `.ts`/`.tsx` files)
- **Guidelines Source:** `.claude/skills/frontend-dev-guidelines` (SKILL.md + anti-patterns, architecture-overview, patterns-react-query, patterns-forms-validation, patterns-service-layer, patterns-types, patterns-styling, patterns-components, testing-guide)
- **Date:** 2026-07-31
- **Build:** PASS (`make ci` exit 0 — verified by controller, not re-run)
- **Tests:** 115/115 `apps/web`, 7/7 `packages/shared-ts` (verified by controller, not re-run)
- **Overall:** NEEDS-WORK

## Build & Test Results

Per the invoking controller, already verified and not re-run here:

```
make ci exits 0
  apps/web            115/115 passed
  packages/shared-ts    7/7   passed
  lint-check (--max-warnings 0)  clean
  build (tsc -b && vite build)   clean
  format:check                   clean
```

Only two read-only verifications were run locally, both to settle specific doubts:

1. Tailwind's default ring-offset colour — `node_modules/tailwindcss/lib/corePlugins.js:3837`:
   `"--tw-ring-offset-color": theme("ringOffsetColor.DEFAULT", "#fff")`.
2. A replay of `conventions.test.ts`'s directory walk, to prove it reaches
   `packages/ui-components/src/StatusBadge.tsx` (it does — exactly one file).

## File Inventory

**Page**
- `apps/web/src/pages/PlaceholderPage.tsx`

**Component**
- `apps/web/src/components/AppLayout.tsx`
- `apps/web/src/components/BrandMark.tsx`
- `apps/web/src/components/ThemeSync.tsx`
- `apps/web/src/components/ThemeToggle.tsx`
- `apps/web/src/components/providers/AppProviders.tsx`
- `apps/web/src/components/features/activity/ActivityEventIcon.tsx`
- `apps/web/src/components/features/dashboard/widgets/FleetOverviewWidget.tsx`
- `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx`
- `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx`
- `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx`
- `apps/web/src/components/features/vehicles/maintenance/SeverityChip.tsx`
- `packages/ui-components/src/StatusBadge.tsx`

**Hook**
- `apps/web/src/lib/hooks/api/auth.ts`

**Service** — none. This repo has no `services/api/` layer; hooks call `apiClient` directly (`apps/web/src/lib/api/client.ts`). FE-11 is therefore N/A.

**Schema** — none. No Zod, no forms in scope. FE-13/FE-14 N/A.

**Type**
- `apps/web/src/types/models/user.ts`

**Other**
- `apps/web/src/context/ThemeContext.tsx` (context)
- `apps/web/src/lib/theme.ts` (pure helpers)
- `apps/web/src/components/brandMarkPath.ts` (generated constant)
- `apps/web/tailwind.config.ts`, `apps/web/src/index.css`, `apps/web/index.html`, `apps/web/package.json`
- Tests: `AppLayout.test.tsx`, `ThemeSync.test.tsx`, `ThemeToggle.test.tsx`, `ThemeContext.test.tsx`, `theme.test.ts`, `auth.test.ts`, `AppProviders.test.tsx`, `conventions.test.ts`, `test/setup.ts`

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grepped `: any` / `as any` / `<any>` across all 28 in-scope files — zero matches. Casts used are narrow and typed: `ThemeSync.test.tsx:26` (`as User`, needed to feed the invalid `'purple'` case), `test/setup.ts:78` (`as unknown as MediaQueryList` for the matchMedia polyfill), `theme.ts:22` (`VALID as readonly string[]`). |
| FE-02 | No manual class concatenation | **FAIL** | `packages/ui-components/src/StatusBadge.tsx:16` — `` className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${VARIANT[status]}`} ``. **Pre-existing**: `git show 352e8c1:packages/ui-components/src/StatusBadge.tsx` has the byte-identical line; this diff only changed the `VARIANT` map (lines 8–11). `cn()`'s deps (`clsx`, `tailwind-merge`) are absent from `packages/ui-components/package.json`, so fixing it needs a dependency addition. Benign in practice — the `VARIANT` classes (`bg-*-subtle`, `text-*-subtle-foreground`) do not collide with the base classes, so there is nothing for `twMerge` to resolve. All in-scope `apps/web` files use `cn()` (`AppLayout.tsx:40`) or plain literals. |
| FE-03 | No direct API client calls in components | PASS | No component or page imports `lib/api/client`. The only `apiClient` import in scope is `apps/web/src/lib/hooks/api/auth.ts:3`, which is the hook layer. `ThemeToggle.tsx:5` reaches the network solely through `useUpdateTheme`. |
| FE-04 | No inline Zod schemas in components | PASS | Zero `z.object(` / `z.string(` / `zodResolver` matches across scope — no forms were touched. |
| FE-05 | No spinners for content loading | PASS | Zero `animate-spin` matches. `MaintenanceQueueView.tsx:23-31` and `FleetOverviewWidget.tsx:12-21` both use `<Skeleton>`. |
| FE-06 | No hardcoded colors | PASS (with note) | Zero palette-class matches across the 28 files. Conversions verified individually: `AppLayout.tsx:30,42-44,54,56`, `SeverityChip.tsx:16,20,24`, `MaintenanceQueueView.tsx:39,47,55,60,76`, `FleetOverviewWidget.tsx:32,36,40`, `OverdueMaintenanceWidget.tsx:28`, `UpcomingMaintenanceWidget.tsx:28`, `ActivityEventIcon.tsx:15`, `PlaceholderPage.tsx:7`, `StatusBadge.tsx:8-11`. Every token used is defined in both `:root` (`index.css:33-48`) and `.dark` (`index.css:78-93`) and mapped in `tailwind.config.ts:51-74`. **Note**: three hex literals remain outside the Tailwind cascade and are unavoidable there — `public/favicon.svg:3-4` (a standalone asset has no access to app CSS variables) and `index.html:16-17` (`theme-color` metas; the manifest format has no media-query support). Both are documented at `index.html:9-14` as manually coupled to `--background`. See **Important #1** for a fourth, avoidable one. |
| FE-07 | No state mutation | PASS | Zero `.push(` / `.splice(` / `.sort(` / `.reverse(` matches. `auth.ts:94-104` builds a fully spread replacement (`{...previous, user: {...previous.user, attributes: {...previous.user.attributes, themePreference}}}`). `ThemeContext.tsx:78-91` uses `setPreferenceState` with plain values; the two `useRef`s hold booleans, not state. |
| FE-08 | No default exports for components | PASS | The only `export default` in scope is `tailwind.config.ts:3`, which is the required shape for a Tailwind config, not a component. All components are named exports: `BrandMark.tsx:16`, `ThemeSync.tsx:14`, `ThemeToggle.tsx:34`, `ThemeContext.tsx:41`, `AppProviders.tsx:20`. |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | The mutation's transport path terminates in `packages/shared-ts/src/apiClient.ts:50` — `if (!res.ok) throw createErrorFromUnknown({status, body})`. The rejection surfaces to the user via `toast.error(...)` at `ThemeToggle.tsx:53-55`. The two bare `catch` blocks in `lib/theme.ts:38,47` are storage-availability guards, not error swallowing — they are the documented FR-FLASH-3 behaviour (blocked `localStorage` must not break boot), each carries an explanatory comment, and both are covered by tests (`theme.test.ts:69-77`, `theme.test.ts:91-99`). |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/user.ts:16` — `export type User = JsonApiResource<UserAttributes>`; `themePreference: ThemePreference` added at line 14 inside `attributes`. Verified against the backend rather than assumed: `apps/auth-service/internal/user/rest.go:11` declares `ThemePreference string \`json:"themePreference"\``, and `apps/auth-service/internal/user/resource_test.go:119` pins the accepted PATCH envelope as `{"data":{"type":"users","attributes":{"themePreference":%q}}}` — byte-for-byte what `auth.ts:71` sends. |
| FE-11 | Service extends `BaseService` | N/A | No `services/api/` layer exists in this repo; the documented direct-client pattern is used via `apps/web/src/lib/api/client.ts`. Consistent with the pre-existing `useMe`/`logoutRequest` in the same file. |
| FE-12 | Query key factory uses `as const` | PASS | `auth.ts:7-10` — `all: ['auth'] as const`, `me: () => ['auth','me'] as const`. `useUpdateTheme` writes through the factory (`auth.ts:94`), never a literal. Sign-out removal at `AuthContext.tsx:56` uses `authKeys.all`, which prefix-matches `authKeys.me()`. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form components in scope. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No Zod schemas in scope. Runtime validation is done with a hand-written type guard, `theme.ts:24-26`, which is appropriate for a three-value union and is exercised directly by `theme.test.ts:11-26`. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | The only interactive element added is `ThemeToggle.tsx:50-58`, which renders `<Button>`; `components/ui/button.tsx:7` carries `cursor-pointer` in the CVA base string. `AppLayout.tsx:56` converts the raw `<button>` Sign-out to the same `<Button>`, so it gains the affordance it previously lacked. `AppLayout.tsx:36` `NavLink` renders a native `<a href>`. `BrandMark.tsx:17` is a non-interactive `aria-hidden` `<svg>`. No clickable `<div>`s, rows, or `render`-prop triggers were introduced. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS (with warnings) | New non-trivial units all have dedicated suites: `theme.ts`→`theme.test.ts` (13 cases), `ThemeContext.tsx`→`ThemeContext.test.tsx` (9), `ThemeSync.tsx`→`ThemeSync.test.tsx` (5), `ThemeToggle.tsx`→`ThemeToggle.test.tsx` (5), `auth.ts`→`auth.test.ts` (3), `AppLayout.tsx`→`AppLayout.test.tsx` (4), `AppProviders.tsx`→`AppProviders.test.tsx` (1 end-to-end provider-tree test). `AppProviders.test.tsx:26-56` is the strongest test in the set — it renders the real `QueryClientProvider > ThemeProvider > AuthProvider > ThemeSync` nesting with only `fetch` stubbed, so a provider reordering fails it. `BrandMark.tsx` has no own suite but is a pure SVG wrapper covered indirectly by `AppLayout.test.tsx:60-65`. The class-only conversions (`SeverityChip`, the three widgets, `ActivityEventIcon`, `StatusBadge`) had no tests before this change and still have none; they are covered by the `conventions.test.ts:113-154` palette guard. See **Minor #4/#5/#6** for three tests that assert less than their comments claim. |
| FE-17 | Mocks updated when services changed | N/A | `find apps packages -name '__mocks__'` returns nothing — this repo has no shared mock directory. `UserAttributes.themePreference` is non-optional, so every `User` fixture had to be updated or the build would break; the three fixtures in scope all carry it (`auth.test.ts:36`, `ThemeSync.test.tsx:22-23`, `AppProviders.test.tsx:39`). |

---

## Points Specifically Requested

### `localStorage` validated everywhere it is read — CONFIRMED

There are exactly two readers, and both reject out-of-range input before it can influence anything:

- `apps/web/src/lib/theme.ts:34-40` — `readCachedTheme()` gates on `isThemePreference(raw)` (`theme.ts:24-26`, allow-list at `theme.ts:22`) and returns `null` otherwise. `ThemeContext.tsx:50` supplies `?? 'system'`. Covered by `theme.test.ts:57-60` and `ThemeContext.test.tsx:52-56`.
- `apps/web/index.html:36-37` — `if (p !== 'light' && p !== 'dark') p = 'system';` narrows to a known-good default rather than rejecting, so a corrupted value and an absent key take the identical path.

Neither path concatenates the stored value into a class name, URL, or markup: `theme.ts:69` toggles the string literal `'dark'`, and `index.html:41` adds the literal `'dark'`. The server value is validated too — `ThemeSync.tsx:41` runs `isThemePreference(serverPreference)` before adopting, tested at `ThemeSync.test.tsx:147-154`.

### Toggle accessible name and keyboard reachability — CONFIRMED

`ThemeToggle.tsx:50-58` renders `<Button type="button">`, which resolves to a native `<button>` (`ui/button.tsx:38`) and is therefore tab-reachable and Enter/Space-activatable with no extra wiring. `aria-label` (`ThemeToggle.tsx:57`) names both the current state and the next action — `"Theme: light. Switch to dark."` — which is what makes a three-way cycle operable without sighted feedback; the icon is `aria-hidden` (`ThemeToggle.tsx:59`) so it does not compete. `AppLayout.test.tsx:71` finds it by role and accessible name, and `ThemeToggle.test.tsx:43-53` pins all three cycle labels.

### Other sub-AA pairings — none found beyond the one already fixed

Grepped every use of the new families (12 sites). Every `-subtle` fill is paired with its matching `-subtle-foreground`:
`SeverityChip.tsx:16,20,24`; `StatusBadge.tsx:8-10`; `MaintenanceQueueView.tsx:47` + `:55,60`.
The remaining text inside the one `bg-danger-subtle` container is `MaintenanceQueueView.tsx:51`, which inherits `--card-foreground` (`222.2 84% 4.9%` light / `210 40% 98%` dark) against `--danger-subtle` — both far above AA. `StatusBadge.tsx:11` (`Inactive`) uses `bg-muted`/`text-muted-foreground`, the pairing `--muted-foreground` was calibrated for, not a `-subtle` fill. The bare tokens (`text-success|warning|danger`) all sit on `--background`/`--card` per `contrast.md`. No new instance of the `text-muted-foreground`-on-`-subtle` mistake exists.

### `conventions.test.ts` genuinely reaches `packages/ui-components` — CONFIRMED

Replayed the walk from `conventions.test.ts:117-126` with the same `resolve(WEB_ROOT, '../../packages/ui-components/src')`: it returns exactly `packages/ui-components/src/StatusBadge.tsx`. The per-root `expect(files.length).toBeGreaterThan(0)` at `conventions.test.ts:137-139` means a moved or renamed directory fails loudly rather than passing vacuously — the right guard given `make fe-test` never runs that package's own suite. See **Minor #6** for two blind spots in the regex/extension filter.

---

## Summary

### Blocking (must fix)

- **FE-02** — `packages/ui-components/src/StatusBadge.tsx:16` builds `className` by template-string interpolation instead of `cn()`. This is the single checklist FAIL and it is **pre-existing and untouched** by this diff; flagged because the file is in scope and the checklist criterion is "zero matches".

### Important (should fix — outside the FE-* IDs, inside the brief's dark-mode/a11y remit)

1. **Focused buttons draw a white halo in dark mode.** `apps/web/src/components/ui/button.tsx:7` applies `focus-visible:ring-offset-2` but never sets `focus-visible:ring-offset-background`. `apps/web/tailwind.config.ts` defines no `ringOffsetColor`, so Tailwind falls back to `#fff` (`node_modules/tailwindcss/lib/corePlugins.js:3837`). Against `--background: 222.2 84% 4.9%` that is a bright 2px white ring between the button edge and the `--ring` ring on every keyboard-focused button in the app — including the `ThemeToggle` this task adds (`ThemeToggle.tsx:50-58`). The other four focusable primitives in the repo all set it correctly (`ui/input.tsx:10`, `ui/textarea.tsx:11`, `ui/select.tsx:17`, `ui/switch.tsx:11`), so `button.tsx` is the lone outlier. `button.tsx` is outside the diff, but making dark mode reachable is what turns a dormant default into a visible defect, so it belongs to this change. One-word fix: add `focus-visible:ring-offset-background` at `button.tsx:7`.

### Non-Blocking (should fix)

2. **The sidebar fill is now a no-op.** `AppLayout.tsx:30` uses `bg-card`, but `--card` and `--background` hold identical values in *both* themes (`index.css:9` vs `index.css:7`; `index.css:55` vs `index.css:53`). The pre-change `bg-gray-50` gave the sidebar a visible tint against a white main area; `bg-card` gives none, so the panel is delineated only by `border-r border-border`. The recorded rationale (avoid `bg-muted` because `--muted` and `--accent` are equal, which would flatten the nav states) correctly rules out `bg-muted` but does not establish that `bg-card` produces a panel. The nav states themselves are fine — `--accent` differs from `--card` in both themes, so active (`AppLayout.tsx:43`) and hover (`AppLayout.tsx:44`) still read. Consider `bg-muted/40` or a dedicated `--sidebar` token if a distinct panel is wanted.

3. **`SeverityChip` "Urgent" now shares its fill with the overdue row it sits in.** `SeverityChip.tsx:16` is `bg-danger-subtle`; `MaintenanceQueueView.tsx:47` fills the enclosing row with the same `bg-danger-subtle`. Before the change these were distinct (`bg-red-100` chip on a `bg-red-50` row). The chip is now separated from its row only by `border-danger-border`, which is ~1.1:1 against the fill in light mode (`0 96.3% 89.4%` vs `0 93.3% 94.1%`). Text stays legible (6.80:1 light / 8.14:1 dark, `contrast.md`), so this is an affordance regression, not an AA failure. Only the urgent severity inside the overdue queue is affected.

4. **`ThemeToggle.test.tsx:96-101` would pass with the behaviour it guards removed.** The comment above it cites FR-TOGGLE-3 ("the icon tracks the PREFERENCE, not the resolved theme"), but the body sets the preference to `system` and asserts only `screen.getByRole('button').querySelector('svg')` is truthy. All three `META` entries (`ThemeToggle.tsx:18-20`) render an svg, so the test passes if `META` were re-keyed on `resolvedTheme` — the exact regression it claims to catch — or if all three icons were identical. A real assertion would render each preference and compare something distinguishing (the `<path d>`, or a `data-icon` attribute).

5. **`AppLayout.test.tsx:63-64` asserts less than it says.** `screen.getByText('MyFleet')` resolves to the brand `<div>` (`AppLayout.tsx:31`), so `.parentElement` is the `<aside>` and `querySelector('svg')` scans the entire sidebar. The test would still pass with the mark moved into the nav list. It does catch outright removal of `BrandMark`, which is most of the value, but "beside the wordmark" is not what it checks.

6. **The palette guard has two blind spots.** `conventions.test.ts:115`'s regex omits `indigo|violet|purple|fuchsia|pink|rose|sky|cyan|teal|lime|stone`, so `bg-indigo-500` would sail through. `conventions.test.ts:120` restricts the walk to `.tsx`, so a `cva` variant map or class-name constant living in a `.ts` file — the common shadcn shape, and the shape `SeverityChip`/`StatusBadge` would take if extracted — is never scanned. Neither is a live violation today (FE-06 is clean), but both weaken the guard that is the sole protection for `packages/ui-components`.

7. **`auth.ts` exports no invalidation helper.** `patterns-react-query.md` ("Invalidation Helper Pattern") says every hook file exports one; `auth.ts` has no `useInvalidateAuth`. Pre-existing shape of the file, not introduced here. Separately, `useUpdateTheme` (`auth.ts:89-107`) has no `onSettled` invalidation, which departs from the documented mutation pattern — but that omission is deliberate and correct: an invalidation would refetch `me` and let `ThemeSync` re-adopt a stale value, which is precisely what FR-PERSIST-5 forbids. The `onMutate`-only shape and the missing `onError` rollback are both sound as designed and documented at `auth.ts:76-88`.

### Design decisions evaluated and accepted

- **`ThemeContext` network-/auth-unaware, bridged by `ThemeSync`** — sound, and the tests prove the payoff: `ThemeContext.test.tsx:23-29` renders the provider with a bare `render()`, no `QueryClientProvider`, no token fixture. `AppProviders.test.tsx` covers the seam the isolation opens up (a provider reordering), which is the right complement.
- **No `onError` rollback in `useUpdateTheme`** — correct. A rollback would restore the pre-change value into `authKeys.me()`, `ThemeSync.tsx:39-43` would see it change and re-adopt it, and the theme would flip under the user. `ThemeToggle.tsx:52-56` owns the toast instead, verified at `ThemeToggle.test.tsx:76-92` (asserts the class *and* the cache write both survive the failure).
- **Duplicated pre-paint logic in `index.html`** — unavoidable; a module import in `<head>` is async by definition. The guard tests are real: `conventions.test.ts:20` interpolates the imported `THEME_STORAGE_KEY` and `:67` the imported `MEDIA_QUERY`, and `:63-68` correctly scopes the media-query check to the script body rather than the whole file (the `theme-color` metas contain the same substring and would have made a whole-file check pass vacuously). `:33-39` and `:43-49` pin synchronicity and the try/catch.
- **`--destructive` untouched, distinct from `danger`** — confirmed: `index.css:21,67` unchanged, and no in-scope file mixes the two.
- **`@types/node` in devDependencies** — type-only, forced by `conventions.test.ts:1-3`'s `node:fs`/`node:url`/`node:path` imports. No runtime dependency added; `apps/web/package.json` `dependencies` is unchanged.
- **`wasSignedIn` set on any authenticated render** (`ThemeSync.tsx:36-38`) rather than inside the adopt effect — correct, and the reasoning in the comment holds: a user whose stored `themePreference` failed validation would otherwise sign out without ever setting the flag, stranding a stale override that would suppress adoption on the next sign-in too. `ThemeSync.test.tsx:104-143` exercises the full sign-in → override → sign-out → sign-in cycle with three distinct values, so it genuinely proves the override was cleared rather than coincidentally matching.

