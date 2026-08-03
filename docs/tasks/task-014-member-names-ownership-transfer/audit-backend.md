# Backend Audit — task-014-member-names-ownership-transfer

- **Scope:** Go changes on `main...HEAD` (BASE `5ff93cd`, HEAD `d37afa5`)
- **Services:** `apps/fleet-service`, `apps/auth-service`
- **Guidelines Source:** `backend-dev-guidelines` skill
- **Date:** 2026-08-02
- **Build:** PASS (both services)
- **Tests:** PASS — fleet-service 17/17 packages ok, auth-service 7/7 packages ok, 0 failed
- **Overall:** NEEDS-WORK

All file:line references are relative to
`/home/tumidanski/source/MyFleet/.worktrees/task-014-member-names-ownership-transfer/`.

## Build & Test Results

```
$ cd apps/fleet-service && go build ./...      # exit 0
$ cd apps/auth-service && go build ./...       # exit 0

$ cd apps/fleet-service && go test ./... -count=1
ok  .../fleet-service/cmd                            0.031s
ok  .../fleet-service/internal/activity              0.009s
ok  .../fleet-service/internal/authz                 0.004s
ok  .../fleet-service/internal/dashboard             0.008s
ok  .../fleet-service/internal/events                0.010s
ok  .../fleet-service/internal/fleet                 0.008s
ok  .../fleet-service/internal/fuel                  0.009s
ok  .../fleet-service/internal/invite                0.015s
ok  .../fleet-service/internal/maintenancecategory   0.019s
ok  .../fleet-service/internal/maintenancerecord     0.013s
ok  .../fleet-service/internal/maintenanceschedule   0.011s
ok  .../fleet-service/internal/mediaclient           0.020s
ok  .../fleet-service/internal/membership            0.029s
ok  .../fleet-service/internal/mileage               0.006s
ok  .../fleet-service/internal/status                0.003s
ok  .../fleet-service/internal/vehicle               0.011s
ok  .../fleet-service/internal/vehiclemedia          0.006s

$ cd apps/auth-service && go test ./... -count=1
ok  .../auth-service/cmd                   0.028s
ok  .../auth-service/internal/arch         0.003s
ok  .../auth-service/internal/jwks         0.093s
ok  .../auth-service/internal/membership   0.025s
ok  .../auth-service/internal/oidc         0.009s
ok  .../auth-service/internal/session      0.429s
ok  .../auth-service/internal/user         0.026s
```

Build passes, tests pass ⇒ the audit does not auto-FAIL. FAIL checks below drop it to NEEDS-WORK.

## Package Classification

| Package | Classification | Basis |
|---|---|---|
| `apps/fleet-service/internal/membership` | Domain | `model.go:4` defines `Model` |
| `apps/auth-service/internal/user` | Domain | `model.go` present; `builder.go`, `entity.go`, `provider.go`, `administrator.go` present |
| `apps/auth-service/internal/membership` | Support | `client.go` only — outbound HTTP client, no `model.go`, no `resource.go`, no handlers, no writes. SUB-01..04 do not apply. |

## Domain Checklist — `apps/fleet-service/internal/membership`

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists with validating `Build()` | **FAIL** | `internal/membership/builder.go:19` has `NewBuilder()` and fluent setters, but `Build()` at `builder.go:30` is `func (b *Builder) Build() Model { return b.m }` — no invariant validation, no error return. Documented as deliberate at `builder.go:14-16`; role validation lives at `processor.go:93`. Pre-existing, unchanged by this branch. |
| DOM-02 | `ToEntity()` on Model | PASS | `entity.go:36` |
| DOM-03 | `Make(Entity)` | WARN | `entity.go:25` `func Make(e Entity) Model` — exists, but returns `Model` not `(Model, error)`. Repo-wide convention. |
| DOM-04 | `Transform` in `rest.go` | PASS | `rest.go:14` |
| DOM-05 | `TransformSlice`, no inline loops in handlers | PASS | `rest.go:28`; used at `resource.go:48`. No transform loop in `resource.go`. |
| DOM-06 | Processor takes `logrus.FieldLogger` | PASS | `processor.go:28` `func NewProcessor(log logrus.FieldLogger, p Provider) *Processor` |
| DOM-07 | Handlers pass `d.Logger()` | N/A / PASS-equivalent | No `HandlerDependency` in this framework. Logger is injected once at `apps/fleet-service/cmd/main.go:186` into `InitializeRoutes(log, db, activity.Record)` and threaded to `NewProcessor` at `resource.go:33`. `grep logrus.StandardLogger` over both services: zero matches. |
| DOM-08 | POST/PATCH use `RegisterInputHandler[T]` | PASS | `resource.go:53` `server.RegisterInputHandler(func(w, req, attrs struct{ Role string })` for `r.Patch`; decoder at `packages/shared-go/server/handler.go:42-55` |
| DOM-09 | Transform errors handled | N/A | `Transform` returns a single value (`rest.go:14`); there is no error to discard. No `_, _ :=` or `_ =` on a Transform call in `resource.go`. |
| DOM-10 | Providers lazy (`database.Query`) | WARN | `provider.go:31-83` executes GORM eagerly; no `database.Query`/`SliceQuery`. Pre-existing across all five provider methods; contrast `apps/auth-service/internal/user/provider.go:71` which does use `database.Query`. |
| DOM-11 | No `os.Getenv()` in handlers | PASS | `grep os.Getenv` over `internal/membership/`: zero matches |
| DOM-12 | No cross-domain logic in handlers | PASS | The one cross-domain concern (activity) is injected as a function value — `administrator.go:31` `type ActivityRecorder func(tx *gorm.DB, …)`, bound at `cmd/main.go:186` — and executed inside `administrator.go:80` / `:111` / `:115`, not in the handler. The membership package never imports `activity`. |
| DOM-13 | Handlers don't call providers directly | **FAIL (pre-existing)** | `resource.go:168` `m, err := prov.GetActiveByUserID(userID)` — handler → provider, bypassing the processor, in `InitializeInternalRoutes`. Its sibling at `resource.go:189` correctly uses `proc.ListActiveMembers`. Unchanged by this branch (present on `main`). All task-014 handlers (`resource.go:43`, `:71`, `:76`, `:129`, `:140`) go through the processor. |
| DOM-14 | No `db.Create`/`db.Save`/`db.Delete` in `resource.go` | PASS | `grep` over `resource.go`: zero matches |
| DOM-15 | `administrator.go` exists for writes, **called by processor** | **FAIL** | File exists (`administrator.go:1`) and holds every write, but the handler calls it directly: `resource.go:87` `adm.UpdateRole(...)` and `resource.go:145` `adm.Remove(...)`. Guideline: `file-responsibilities.md:43-50` and `anti-patterns.md:204-212` require `resource.go → processor.go → administrator.go`. See Non-Blocking note NB-1 — this is the repo-wide house pattern. |
| DOM-16 | Domain error → HTTP status | PASS | 422 validation `errRoleValidation` `resource.go:24` → `server/errors.go:35`; 404 `processor.go:99`,`:107` and `resource.go:131`; 409 `processor.go:80`,`:116` → `errors.go:27`; 403 `processor.go:39`,`:44` → `errors.go:25`. Verified end-to-end by `resource_test.go:139` (422), `:158` (404), `:169` (409), `:112` (403). |
| DOM-17 | JSON:API interface (`GetName`/`GetID`/`SetID`) | N/A | House convention is the shared `server.Resource{Type, ID, Attributes}` envelope — `rest.go:15-24`, `packages/shared-go/server/jsonapi.go`. No api2go, so the interface methods do not exist anywhere in this repo. |
| DOM-18 | Request models flat (no nested Data/Type/Attributes) | PASS | `resource.go:53-55` declares an anonymous flat `struct{ Role string \`json:"role"\` }`; the envelope is peeled by `handler.go:44-48`, not by the domain. |
| DOM-19 | Table-driven tests | **FAIL** | Zero `t.Run(` and zero `:= []struct{` in `processor_test.go`, `resource_test.go`, `administrator_db_test.go`. `resource_test.go:106`, `:210`, `:229` loop over roles with a bare `for … range` and no subtest, so a failure in a later iteration is reported without a distinguishing name. (Repo-wide `grep -rn "t.Run(" apps/` = 20 hits, none in these packages.) Coverage breadth is otherwise strong — see PASS rows above. |

## Domain Checklist — `apps/auth-service/internal/user` (changed surface only)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-04/05 | `Transform` + `TransformSlice` | PASS | `rest.go:14`, `rest.go:18`; `TransformSlice` used by the new handler at `resource.go:240`, no inline loop |
| DOM-06 | Processor takes `logrus.FieldLogger` | PASS | `processor.go:16` `func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator)` |
| DOM-08 | POST/PATCH use `RegisterInputHandler[T]` | PASS | `resource.go:158` for `PATCH /auth/me`. The new `/auth/users` route is a GET (`resource.go:196`) and correctly uses a plain handler. |
| DOM-10 | Providers lazy | PASS | New `ListByIDs` uses `database.Query(...)` at `provider.go:71` (per branch diff), matching `GetByID`/`GetBySub` |
| DOM-11 | No `os.Getenv()` in handlers | PASS | zero matches in `internal/user/` |
| DOM-13 | Handlers call processors, not providers | PASS | `resource.go:223` `proc.ListByIDs(allowed)`; the provider is reached only via `NewProcessor(log, NewProvider(db), …)` at `resource.go:114` |
| DOM-14 | No direct DB writes in `resource.go` | PASS | zero matches for `db.Create`/`db.Save`/`db.Delete` |
| DOM-16 | Domain error → HTTP status | PASS | 422 `resource.go:40`,`:41`,`:30`; 404 `resource.go:132`,`:172`; 500 via `errInternal` `resource.go:21` used at `:139`,`:178`,`:218`,`:228` |
| DOM-19 | Table-driven tests | **FAIL** | Zero `t.Run(` / struct tables in `users_resource_test.go`, `provider_test.go`. `users_resource_test.go:182` loops over four query strings without subtests. |
| Mocks in sync | Interface change propagated | PASS | `Provider.ListByIDs` added at `provider.go` and mirrored in `processor_test.go:33` (`fakeProvider`) and `cmd/main_test.go:38` (`fakeUsers`) — testing-guide.md mock-sync checklist satisfied; both services compile and test clean. |

## Security Review

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validated, not `ParseUnverified` | PASS | `packages/shared-go/auth/middleware.go:40` `jwt.ParseWithClaims(raw, claims, keyfn, jwt.WithValidMethods([]string{"RS256"}))` with `!tok.Valid` rejection at `:41`. `grep -rn ParseUnverified apps/ packages/` → zero matches. |
| SEC-02 | Claims taken only from validated tokens | PASS | Identity is built at `middleware.go:45-50` after validation and reaches handlers only via context (`fleet resource.go:57`, `:99`; `auth resource.go:204`). No handler parses a token. |
| SEC-03 | No open redirect | N/A | No redirect handler in the changed surface. `oidc` untouched by this branch. |
| SEC-04 | No hardcoded secrets | PASS | `grep -rniE "(secret\|password\|api[_-]?key\|private[_-]?key)\s*[:=]\s*"…"` over `apps/auth-service/internal` and `apps/fleet-service/internal/membership` → zero matches. Secrets come from `config.MustGet` (`apps/auth-service/cmd/main.go:66-72`). |
| SEC-05 | PATCH owner-only at BOTH layers, `RequireSameFleet` FIRST | PASS | Order is `RequireSameFleet` `resource.go:61` → `RequireOwner` (token) `:66` → `RequireOwnerInFleet` (DB) `:71`. DB check implemented at `processor.go:35-47` and returns `server.ErrForbidden` on both "no membership" and "not owner". `authz.RequireSameFleet` returns **404** (`internal/authz/scope.go:12-17`), so cross-fleet ids never leak existence. Proven: `resource_test.go:194` (cross-fleet → 404), `:120` (stale owner claim → 403), `:105` (member/viewer → 403). |
| SEC-06 | Fleet never left with zero owners | **FAIL — race** | Guard exists and is sequentially correct (`processor.go:110-118` demotion, `processor.go:72-83` self-removal; tests `resource_test.go:164`, `processor_test.go:145`, `:34`), but it is TOCTOU-racy. See IMP-1. |
| SEC-07 | DELETE self-relaxation is not privilege escalation | PASS | `isSelf` at `resource.go:115` is an exact `identity.UserID == targetUserID` match; the owner gates remain for every non-self path (`resource.go:116-127`). `RequireSameFleet` is at `resource.go:107`, **outside** the `isSelf` branch. Sequential zero-owner safety for owner-removes-other holds because the actor is a DB-verified owner (`resource.go:123`) and `actor != target`, so an owner always remains. Proven: `resource_test.go:228` (non-owner removing another → 403 **and** row still present), `:276` (cross-fleet self-leave → 404 **and** row still present), `:245` (sole-owner self-leave → 409). |
| SEC-08 | `/auth/users` JWT-protected | PASS | Registered inside the JWT group: `apps/auth-service/cmd/main.go:87-96` wraps `user.InitializeRoutes` in `pr.Use(authmw.JWT(ks.Keyfunc(), …))`. Protection is by placement, per the comment at `resource.go:188-190`. |
| SEC-09 | `/auth/users` returns only active members of the caller's active fleet | PASS | `resource.go:212` gathers the roster, `:222` `intersect(requested, memberIDs)`, `:223` lists only the intersection. Roster is active-only — `fleet-service .../membership/provider.go:54-64` filters `status = 'active'`. Fleetless caller short-circuits to `[]` without the hop (`resource.go:205-210`). |
| SEC-10 | Foreign and nonexistent ids indistinguishable | PASS | Omission is the only failure mode (`resource.go:79-91`). `users_resource_test.go:126` compares the two responses **byte for byte** (`foreign.Body.String() != ghost.Body.String()`) and the status codes; `:102` asserts a foreign user's email never appears. |
| SEC-11 | `ids` cap of 100 applied AFTER de-duplication | PASS | `parseUserIDs` `resource.go:55-73`: trim+dedupe loop `:58-65`, empty check `:66`, cap `:69-71` — cap strictly after dedupe. `maxUserIDs = 100` at `resource.go:35`. Proven both directions: `users_resource_test.go:206` (300 copies of one id → 200 with one result), `:191` (101 distinct → 422). |
| SEC-12 | Response error messages are compile-time constants | PASS | fleet: `resource.go:24` `server.Detailed(server.ErrValidation, "role must be one of owner, member, viewer")`. auth: `resource.go:21` `errInternal`, `:30` `errThemeValidation`, `:40` `errIDsRequired`, `:41` `errTooManyIDs` — all fixed strings. No caller input, user id, or upstream body is interpolated into anything reaching `server.WriteError`. Downstream errors are logged and replaced (`resource.go:217-218`, `:225-228`). Proven: `resource_test.go:148` asserts the rejected role value is **absent** from `detail`; `users_resource_test.go:238` asserts the downstream error text never reaches the client. Client-side errors carry a status code only, never the body or the fleet id: `internal/membership/client.go:53`, `:101`. |
| SEC-13 | Activity recorded in the SAME gorm transaction as the domain write | PASS | `administrator.go:71-85` (`UpdateRole`: `tx.Model(...).Update` then `a.record(tx, …)` inside one `a.db.Transaction`) and `administrator.go:103-119` (`Remove`: `tx.Delete` then `a.record(tx, …)`). Proven by rollback, not by inspection: `administrator_db_test.go:143` (recorder errors ⇒ role reverts to `member`) and `:218` (recorder errors ⇒ deleted row is still present). Recorder wired at `apps/fleet-service/cmd/main.go:186`. |

## Sub-Domain Checklist

Not applicable. `apps/auth-service/internal/membership` is a support package
(`client.go` — outbound HTTP client only, no handlers, no writes, no `resource.go`).

## Findings

### Blocking

**IMP-1 (Important) — the zero-owner invariant is TOCTOU-racy.**
`Processor.ValidateRoleChange` counts owners at `processor.go:111-117` and
`Processor.ValidateRemoval` at `processor.go:74-80`. Both run **outside** the
write transactions opened at `administrator.go:71` and `administrator.go:103`,
with no row lock (`SELECT … FOR UPDATE`), no re-check inside the tx, and no DB
constraint. In a fleet with exactly two owners A and B, two concurrent requests
— A demotes B while B demotes A, or A demotes B while B self-leaves — each
observe `CountOwners() == 2`, each pass the guard, and both commit. The fleet is
left with zero owners and, since `PATCH` and `DELETE` both require an owner, no
in-band way to recover. The prompt states this invariant must never break.
`design.md` D5 (lines 206-243) discusses the transaction only for the activity
record; the design contains no discussion of concurrency for this guard (grep
for race/concurrent/lock/serializable over `design.md` returns nothing on this
path).
*Fix shape:* move the `CountOwners` check inside the `db.Transaction` in
`UpdateRole`/`Remove` using a locking read, or add a partial unique/`CHECK`
constraint that makes a zero-owner fleet unrepresentable.

**IMP-2 (Important) — `CountOwners` ignores `status`, and it guards the invariant.**
`provider.go:79` counts `fleet_id = ? AND role = 'owner'` with **no**
`status = 'active'` predicate, while `ValidateRoleChange` explicitly requires the
target to be active (`processor.go:106`) and `ListActiveByFleetID`
(`provider.go:56`) does filter status. The code anticipates a second status value
arriving (`processor.go:103-105` calls the check "a statement of intent"). The
moment one does, a fleet with one active owner and one revoked owner counts as
`2`, the demotion guard passes, and the last real owner is demoted — the exact
invariant IMP-1 also threatens. Add `AND status = 'active'` to `provider.go:79`.

**IMP-3 (Important) — writes ignore `RowsAffected`, so the audit log can record
events that never happened.**
`administrator.go:72` (`Update("role", role)`) and `administrator.go:104`
(`Delete(&Entity{}, "id = ?", …)`) never check `RowsAffected` (`grep RowsAffected
administrator.go` → zero matches). The `Model` they act on is read outside the
transaction (`resource.go:76` via `ValidateRoleChange`, `resource.go:129` via
`GetMember`). If the row is removed concurrently: the update matches zero rows,
the transaction still commits, `a.record(...)` writes a `member.role_changed` for
a membership that no longer exists, and the handler returns **200** carrying
`updated` — which is `m.WithRole(role)` computed in memory at
`administrator.go:70`, never read back from the database. The DELETE path is the
same: a repeated request returns 204 and writes a second `member.left`. This
matters more than usual here because `administrator.go:94-96` states the activity
row is the *only* record the membership ever existed — an unconditional record
is an audit-integrity gap, not a cosmetic one.

**DOM-15 — handlers call the administrator directly.**
`resource.go:87` (`adm.UpdateRole`) and `resource.go:145` (`adm.Remove`) bypass
the processor, contradicting `file-responsibilities.md:43-50` and
`anti-patterns.md:204-212` (`resource.go → processor.go → administrator.go`).
Blocking against the letter of the checklist; see NB-1 for why it may be
deliberately deferred.

**DOM-19 — no table-driven tests in any changed test file.**
Zero `t.Run(` and zero struct tables across `processor_test.go`,
`resource_test.go`, `administrator_db_test.go`, `users_resource_test.go`,
`provider_test.go`, `client_test.go`. Bare `for … range` loops at
`resource_test.go:106`, `:210`, `:229` and `users_resource_test.go:182` report
failures without naming the failing case.

**DOM-01 — `Build()` performs no validation.**
`builder.go:30` returns the model unconditionally. `patterns-functional.md`
("Validation occurs in `Build()`") and the Commonly Missed Items Checklist both
require invariant enforcement here. Pre-existing; role validity is instead
enforced at `processor.go:93`, which does cover every user-facing path on this
branch.

### Non-Blocking

- **NB-1 — DOM-15 is the repo-wide house pattern, not a task-014 regression.**
  Handlers call administrators directly in `invite/resource.go:90`, `:191`,
  `:237`; `mileage/resource.go:123`; `fuel/resource.go:211`, `:237`;
  `fleet/resource.go:44`. This branch follows the existing convention. Fixing it
  here alone would make `membership` the odd one out; it is a service-wide
  refactor, not a task-014 change.
- **NB-2 — DOM-13 violation at `resource.go:168`** (`prov.GetActiveByUserID` from
  the internal handler) is present on `main` and untouched by this branch.
- **NB-3 — unbounded upstream decode.** `internal/membership/client.go:107`
  `json.NewDecoder(res.Body).Decode(&rows)` has no `http.MaxBytesReader` /
  `io.LimitReader`, so a misbehaving fleet-service response is unbounded in
  memory. Same at `client.go:56`. Internal, network-restricted callee ⇒ low.
- **NB-4 — body decoding precedes authorization on PATCH.**
  `server.RegisterInputHandler` (`handler.go:42-53`) decodes and can emit 422
  before the wrapped function — and therefore before `RequireSameFleet`
  (`resource.go:61`) — runs. A cross-fleet caller sending a malformed body
  receives 422 rather than the 404 the same-fleet guard would give. Not an
  oracle: the 422 is constant regardless of whether the fleet or target exists.
  Informational.
- **NB-5 — DOM-03 / DOM-10 deviations** (`Make` returns `Model` not
  `(Model, error)`; `membership` providers are eager rather than
  `database.Query`-lazy) are repo-wide conventions, unchanged by this branch.
- **NB-6 — `/auth/users` returns `themePreference` and `email` for other fleet
  members** (`resource.go:240` reuses `Transform`). Deliberate and documented at
  `resource.go:234-239`; scoped to co-members. Noted, not objected to.
