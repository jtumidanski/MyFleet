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
