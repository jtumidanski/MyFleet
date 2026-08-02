# Plan Adherence Audit — task-011-platform-admin-console

**Plan Path:** docs/tasks/task-011-platform-admin-console/plan.md
**Audit Date:** 2026-08-02
**Branch:** task-011-platform-admin-console
**Base Branch:** main
**Review Range:** f959cdc2d012acdd8666af361ab7778e7fd1216a..7ba674a161d91ddc755b88c1fd85b756f05cc024 (42 commits)

## Executive Summary

27 of 29 tasks are fully implemented; 2 are PARTIAL (Task 20, Task 28). The
branch is green across every automated gate: `make build`, `make vet`,
`go test -race` over the whole workspace, `make fe-test` (236 tests), `make
fe-build`, `make lint-check`, and `make manifests` all pass, and both overlays
render.

Test fidelity is generally excellent — the plan's sketched assertions were
reproduced faithfully and in several places *strengthened* (the vehicle-cascade
scope test, the orphan fixed-point sweep, the fleetless-admin 200 assertion, the
resolver table-drive that defeats a hardcoded `true`). No `t.Skip`, `.skip`,
`.only`, TODO, or FIXME was introduced anywhere in the branch.

Five substantive gaps warrant fixes before merge, two of them undisclosed
functional regressions where the shipped code (and its own comments) claim
behavior that is absent. Separately, one security-relevant deployment gap: the
`local` overlay ships notification-service's new destructive unauthenticated
purge endpoint with no `internal-deny` rule, contradicting an invariant the
source code explicitly states.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | fleet-service soft-delete schema + partial unique indexes | DONE | All 12 entities carry both columns; `membership/entity.go:20`, `invite/entity.go:41`, `dashboard/entity.go:57`. Migration tests are verbatim the plan's sketch. Postgres/SQLite dialect branch is a correctness fix for SQL the plan got wrong. |
| 2 | media/notification schema | DONE | `notification/entity.go:62`, `preferences/entity.go:51`. The subtle `ON CONFLICT ... TargetWhere: deleted_at IS NULL` fix is present in both administrators (`notification/administrator.go:49`, `preferences/administrator.go:34`). |
| 3 | shared SQLite test harness | DONE | `admin/admintest/db.go` — all 16 tables, 3 partial indexes, `SeedFleet` covering 12 tables. `db_test.go:12-53` keeps the anti-vacuity guard. |
| 4 | visibility sweep — mileage, schedules, membership, invites | DONE | All 14 filter sites present incl. `maintenanceschedule/provider.go:107` and `membership/provider.go:81` (`CountOwners`). Four `visibility_*_test.go` files reproduce the plan's assertions exactly. |
| 5 | visibility sweep — activity, documents, dashboards | DONE | `activity/provider.go:29,33,58`; `dashboard/processor.go:127-190` revive rewritten as sketched; `dashboard/aggregate.go:148`. All four revive assertions retained. |
| 6 | visibility sweep — notification, preferences, media variants | DONE | `notification/provider.go:39`, `preferences/provider.go:27,38`, `mediavariant/provider.go:39,54`, `mediaobject/purge.go:28-33`. |
| 7 | purge manifest + four operations + arch test | DONE | `admin/manifest.go` 13 targets byte-for-byte; `admin/operations.go:15,49,67,87,101`. Arch test AST-parses and has the empty-walk guard (`arch_test.go:73-75`). See Finding 6 for its walk-root limit. |
| 8 | pre-existing orphan defect | DONE | `admin/orphans.go:20-40,67-94` — strengthened past the plan to a fixed-point loop that the plan's single walk would have missed. `vehicle/purge.go:36-53`. |
| 9 | `auth.platform_admins` + two seeding hooks | DONE | `platformadmin/{entity,provider,administrator,seed}.go`; hooks at `auth-service/cmd/main.go:56,96`. Hardened past plan with a `revoked_at` tombstone and an `email_verified` gate, each with new tests. |
| 10 | `platform_admin` claim end to end | DONE | Minted `session/processor.go:72`; parsed `packages/shared-go/auth/middleware.go:50`; `/auth/me` `user/resource.go:72`. Sentinel scheme correctly *extended* for the new bool (`session/processor_test.go:266-280`), not weakened. Negative directions pinned at `middleware_test.go:118` (absent / `"true"` string / `float64(1)` all → false). |
| 11 | `RequirePlatformAdmin` + R7 separation arch test | DONE | Guard `authz/scope.go:46`; test `admin/arch_test.go:131`. Three deviations, only two disclosed — see Finding 7. |
| 12 | auth-service internal admin routes + deny rule | DONE | `platformadmin/resource.go:51,55,77,129`; registered outside JWT at `auth-service/cmd/main.go:131`. Deny rule renders twice in `main`. |
| 13 | media-service admin manifest + internal routes | DONE | `media-service/internal/admin/{manifest,operations,resource}.go`; wired outside JWT at `media-service/cmd/main.go:139`. All 9 named tests present. |
| 14 | notification-service admin manifest + internal routes | DONE | `notification-service/internal/admin/manifest.go:58-61` correctly scopes preferences to `ScopeSystem` only. Wired at `notification-service/cmd/main.go:82`. |
| 15 | `adminclient` package | DONE | `adminclient/http.go:23` 5s timeout, `:27` `MaxLookupIDs = 50`; `auth.go:90` `IsPlatformAdmin` with the three-way 200/404/other split. |
| 16 | purge operations, audit, confirmation, recovery window | DONE | `admin/confirmation.go:12,16,30,55`; persistence `admin/{entity,model,builder,provider,administrator}.go`; migration registered `fleet-service/cmd/main.go:60`. `confirmation_test.go:13-67` byte-identical to plan. |
| 17 | create a purge operation | DONE | `admin/processor.go:101-217` — fail-closed re-verify, single transaction, post-commit fan-out. Partial-failure recording implemented (`:190-203`). |
| 18 | cancel and retry | DONE | `admin/processor.go:302-368` (cancel, no re-verify per design §5.4) and `:377-442` (retry, re-verifies, rejects `reaped`+`cancelled`). |
| 19 | hourly reaper | DONE | `admin/reaper.go:32-113`; registration deferred to Task 21 as the plan explicitly permits (plan.md:8451-8453) and landed at `fleet-service/cmd/main.go:241-249`. |
| 20 | the `/admin` read surface | **PARTIAL** | All ten routes registered (`admin/resource.go:48-223`), each behind `RequirePlatformAdmin`, verified per route. **Owner-email search never implemented** — see Finding 4. |
| 21 | wire `/admin` route group + config | DONE | `fleet-service/cmd/main.go:212-234,279-284`. Every enumerated ConfigMap key shipped in k8s and compose. `check-manifests.sh` change is a documented strengthening. |
| 22 | three UI primitives | DONE | `ui/badge.tsx`, `ui/table.tsx`, `ui/dialog.tsx`; `badge.test.tsx` asserts per-variant classes. |
| 23 | `platformAdmin` in auth context, admin shell, route tree | DONE | `RequirePlatformAdmin.tsx:23-42`; guard test proves the **deny** path (`RequirePlatformAdmin.test.tsx:55-58`), plus the fleetless-admin case. `/admin` is a true sibling of `RequireAuth` in `App.tsx:72-86`. |
| 24 | admin service, hooks, purge-status vocabulary | DONE | 10 service methods + 10 hooks. Types verified field-for-field against `rest.go` incl. nullability and nil-slice/nil-map init — **zero mismatches**. Disclosed deviation #2 confirmed accurate: the plan's sketch really had drifted on 6 fields (`affected`→`affected_counts`, `pending_invites`→`invites`, `current_mileage`→`mileage`, `fleets[].id`→`fleet_id`, two spurious `\| null`), and the code follows the server. The 7th sketch field, `platform_admin`, was dropped — see Finding 3. Status vocabulary matches `admin/entity.go:14-17` exactly. |
| 25 | overview screen | DONE | `AdminOverviewPage.tsx:31-43,74-77,150`. FR-ADMIN-UI-6 test retains both `'—'` and `not.toHaveTextContent('0')`. |
| 26 | fleet inspector + blast-radius panel | DONE | `AdminFleetsPage.tsx:68-79,151,193-197,251-260`; `BlastRadiusPanel.test.tsx:21-25`. Minor: per-vehicle record counts omitted from the vehicles table. |
| 27 | confirmation dialog + purge flow | DONE (test weakened) | Phrase gate real and exact (`PurgeConfirmDialog.tsx:89,169`); blocked path proved twice. Phrases agree with the backend. **But `postPurgeRouting.test.tsx` does not import `App`** — see Finding 1. |
| 28 | purges, audit, users screens | **PARTIAL** | Purges screen complete. **Audit `?actor=` filter missing; users screen missing 3 of the plan's columns** — see Findings 3 and 5. |
| 29 | the full gate | DONE | Both overlays render; all four deny rules present twice in `main`; `main` clean of PVC/Secret/ClusterRole/placeholders; arch tests confirmed executing, not vacuous. Step 8's by-hand checks remain unverified by construction. |

**Completion Rate:** 27/29 fully DONE (93%), 2 PARTIAL
**Skipped without approval:** 0
**Partial implementations:** 2 (Tasks 20, 28)
**Undisclosed deviations found:** 4 (Findings 1, 3, 4/5, 7)

Note: all 178 `- [ ]` checkboxes in plan.md remain unchecked. The plan was never
annotated during execution, so the checkbox state carries no signal either way.

## Findings

### 1. [Important] The R5 residual-risk test does not test the thing it exists for

`apps/web/src/components/admin/postPurgeRouting.test.tsx:53-86` hand-rebuilds a
replica of App.tsx's route tree instead of importing `App`. The plan specified
`renderWithProviders(<App />, { route: '/admin/purges' })` explicitly, twice
(plan.md:10886, 10893).

The file's entire stated purpose is to catch "a future refactor renesting
`/admin` under `RequireAuth`" — a refactor that would happen *in App.tsx*, and
therefore cannot fail a test that declares its own tree. No test in the repo
imports `App` (`grep -rn "from '.*App'" apps/web/src --include='*.test.tsx'` →
empty), so `App.tsx:72-86` is entirely unguarded.

The property is correct **today** — `/admin` is genuinely a sibling of
`RequireAuth` — so this is not a live bug. It is a missing guard on the exact
regression the plan called catastrophic (an admin bounced to `/onboarding` with
a five-day recovery window ticking). The substitution is documented in the file
comment at `:21-25` but was not disclosed as a deviation.

Also untested: `confirmSystemPurge`'s cache-clear/refetch
(`AdminOverviewPage.tsx:96-107`).

### 2. [Important] The `local` overlay ships a destructive unauthenticated endpoint with no deny rule

`notification-service/cmd/main.go:78-82` registers `/internal/admin/*` outside
the JWT group, and its own comment states the invariant plainly:

> the priority-200 internal-deny rule in the main overlay's ingressroute is what
> keeps them off the public internet; **the two ship together and never separately**

They ship separately in `local`:

```
kustomize build deploy/k8s/overlays/local | grep -c internal-deny  →  0
kustomize build deploy/k8s/overlays/main  | grep -c internal-deny  →  9
```

`notifications-stripprefix` removes the **full** `/api/notifications` prefix
(`deploy/k8s/base/routing/middlewares.yaml:48-52`), so on a local cluster
`/api/notifications/internal/admin/purge` arrives at the service as
`/internal/admin/purge` — an unauthenticated fleet-scoped delete. This is the
precise scenario `tools/check-manifests.sh:102` was extended to protect against,
and the script only inspects `main`.

The plan only ever specified the `main` overlay, so this is not strictly a plan
deviation — but notification-service's internal routes are **new in this branch**
(the pre-branch `cmd/main.go` had none), so the exposure is new too. This needs a
conscious accept or a `local` deny rule, not silence.

### 3. [Important] Users screen drops three columns; an explicit PRD requirement is unmet

The plan (plan.md:10916) asked for "id, email, display name, created, last login,
a platform-admin `Badge`, and the fleets they belong to". `AdminUsersPage.tsx:54-61`
renders four columns: User, Email, Fleets, Last sign-in.

**This is not merely a plan deviation — it is an unmet PRD requirement.**
`prd.md:192-194` FR-ADMIN-FLEET-6 states that `GET /admin/users` returns "(id,
email, display name, created date, last login, **platform-admin flag**, and the
fleets they belong to)". The flag is absent end-to-end: it is not in
`platformadmin/resource.go:21-27` (`InternalUser`), not in `adminclient/auth.go:16-22`,
not in `browse.go:124-131` (`UserRow`), not in `rest.go:129-135` (`userAttributes`),
and not in the TS type. The comment at `AdminUsersPage.tsx:20` cites a "PRD
non-goal" — but the PRD non-goal covers *granting* admin from the console, not
*displaying* the flag, which FR-ADMIN-FLEET-6 explicitly requires.

**Platform-admin badge (disclosed, wrong premise).** The flag genuinely is absent
from the payload — confirmed at `admin/browse.go:124-131`, `adminclient/auth.go:16-22`,
and `platformadmin/resource.go:110`. But the code comment at `AdminUsersPage.tsx:22-25`
claims "fleet-service would have to ask auth-service per user to know it." It
would not. `auth.platform_admins` and `auth.users` are the same database, same
schema, served by the same handler on the same `*gorm.DB`. One
`LEFT JOIN auth.platform_admins pa ON pa.user_id = u.id AND pa.revoked_at IS NULL`
in the existing query at `platformadmin/resource.go:110` supplies the flag with
zero extra round trips, and design D2 (no cross-service joins) is not implicated
because both tables belong to auth-service. The correct fix was to add the field
server-side, not to delete the column.

**`created` column (undisclosed).** Dropped despite the data already being in the
payload — `browse.go:128` and `types/models/admin.ts:136` both carry `created_at`.
There is no justification for this one, and no test flags it.

**`id` column (undisclosed).** Appears only as a fallback when `display_name` is
empty (`:68`).

### 4. [Important] Owner-email search specified, commented as implemented, never implemented

Plan lines 9143-9148 specify that `?q=` matches owner email in Go, after the
auth-service lookup, over the page's resolved owners.

`admin/browse.go:163-247` does only the SQL name filter at `:170`. After
`resolveUsers` at `:227` it appends every row unconditionally — there is no email
match anywhere in the function. Yet the comment at `:166-168` says "owner-email
search is applied after the auth-service lookup below," and the shipped UI
advertises it: `AdminFleetsPage.tsx:107` ships the placeholder
`"Fleet name, or owner email on this page"` with a comment at `:100-104` asserting
the post-lookup match exists.

Typing an owner email returns nothing. Either implement the post-filter after
`browse.go:227` or stop promising it in the placeholder and both comments.

### 5. [Important] Audit `?actor=` filter is dead plumbing

The plan (plan.md:10913) requires `?action=` and `?actor=` filters. Every layer
exists — `AdminService.ts:171,177`, `admin.ts:23,84,90`, and the server side at
`admin/resource.go:230` / `admin/provider.go:85` — but the UI hardcodes it:

```ts
// AdminAuditPage.tsx:43
const { data, isLoading, isError } = useAuditEvents({ action, actor: '', page: 1 });
```

Only the action filter has a control (`:54-66`). The feature is one input away
from complete and is currently unreachable.

### 6. [Moderate] Manifest-completeness arch tests only see files named `entity.go`

All three arch tests walk with `if d.IsDir() || filepath.Base(path) != "entity.go" { return nil }`
(`fleet-service/internal/admin/arch_test.go:39` and its two copies). A `TableName()`
in any other filename is invisible. This was proven empirically: a probe file
`model.go` declaring a new table passes; renaming the identical file to
`entity.go` fails as intended.

Three real tables already escape today:

- `media-service/internal/processedevents/processedevents.go:22` → `media.processed_events`
- `notification-service/internal/inbox/processed.go:25` → `notification.processed_events`
- `packages/shared-go/events/outbox.go:21` → `outbox` (outside the walk root entirely)

Consequently the `excludedTables` entries for `processed_events` and `outbox` are
never exercised by any test, and the `len(found) == 0` vacuity guard does not fire
because two tables are still discovered in each service.

This is faithful to the plan — the plan specified this walk — so it is not an
execution failure. But the guarantee the tests advertise ("a new table added
anywhere fails here") is narrower than stated. Suggested fix: match any `*.go`
under the walk root, or assert the discovered set equals `Manifest ∪ excludedTables`
so an unexercised exclusion is itself a failure.

### 7. [Moderate] `TestAdminTreeIsSeparate` — a third, undisclosed narrowing

Two changes were disclosed. There are three.

1. **Self-exclusion** (`arch_test.go:133,148`) — disclosed. Genuinely required:
   the file's own search literal at `:176` is `"RequireSameFleet("`, which matches
   itself. Risk negligible.
2. **Tier widened to `../adminclient/`** (`:134`) — disclosed. Required because
   `adminclient/auth.go:90` defines `IsPlatformAdmin`. A narrower fix
   (allowlisting the two adminclient files) was available and not taken.
3. **`RequireSameFleet` → `RequireSameFleet(`** (`:176`) — **not disclosed.**
   Required, because `admin/manifest.go:6`, `admin/resource.go:43`, and
   `admin/browse_test.go:124` all name the guard in prose. But it is the most
   substantive change to the assertion, and it means a wrapper such as
   `RequireSameFleetOrAdmin(` no longer matches inside the admin tier.

Adversarial probing confirms the test still catches the primary realistic
violations (a `PlatformAdmin` reference in an ordinary handler package; a
`RequireSameFleet(` call inside either admin-tier package). Two holes remain:

- Introduced by the widening: a file under `internal/adminclient` reading
  `Identity.PlatformAdmin` in a cross-fleet handler passes silently. Benign today
  — those four files hold only HTTP clients, no router — but nothing structurally
  keeps it that way.
- Pre-existing and also present in the plan: a bypass helper defined in the
  allowlisted `authz/scope.go` under a neutral name, called from neutral call
  sites, passes completely. That is rot-scenario #1 from the test's own doc comment.

The test also has no empty-walk vacuity guard, unlike its sibling at `:73-75`.

### 8. [Minor] Other observations

- `AdminAuditPage.test.tsx:50-65` "renders newest first" does not test ordering —
  the mock is already newest-first and the page renders in array order. Inherited
  verbatim from the plan's sketch, so not an implementer regression.
- `maintenanceschedule/administrator.go` `AdvanceTx` (`:78,:101`) still reads and
  writes with a bare `Where("id = ?", id)`. The plan's prose intent ("a purged
  schedule must not be recomputed") arguably covers it; the line the plan cited
  (`:132`) was changed correctly. Residual path reachable via `completion_db.go:148`.
- `admin/model.go:16` `ErrAlreadyReaped` is declared but never used — dead code
  inherited from the plan's own sketch.
- `make manifests` was red on this branch between commits `f00162c` and `ba54c83`.
- Task 26: the plan's per-vehicle "record counts" are absent from the fleet
  inspector's vehicles table (`AdminFleetsPage.tsx:271-299`).
- `lib/hooks/api/auth.ts:28`'s `?? false` default — the guard that stops an older
  server from revealing the console — has no regression test. The plan supplied it
  as code without a test, so this is not a deviation, but it is the one
  security-flavoured default in Tasks 23-24 with nothing behind it.
- Merge note (not a task deviation): `main` has moved ahead on
  `apps/web/src/components/RequireAuth.tsx`. That file is untouched by this branch
  and the new `/admin` sibling route does not interact with it, but the merge is
  still pending.

## Build & Test Results

| Gate | Result | Notes |
|------|--------|-------|
| `make build` | PASS | whole workspace |
| `make vet` | PASS | whole workspace |
| `go test -race ./...` | PASS | zero failures across all modules |
| `make fe-test` | PASS | 42 files, 236 tests; shared-ts 2 files, 7 tests |
| `make fe-build` | PASS | tsc -b + vite build clean |
| `make lint-check` | PASS | 0 issues, prettier + eslint clean |
| `tools/check-manifests.sh` | PASS | 9 routes both entrypoints; deny rules for all 4 services |
| `kustomize build overlays/main` | PASS | 0 PVC / 0 Secret / 0 ClusterRole / no placeholders |
| `kustomize build overlays/local` | PASS | renders; see Finding 2 |
| Arch tests | PASS | confirmed executing, not vacuous |

Not run (require a live cluster or manual observation): `kubectl apply
--dry-run=server` on both overlays, and Task 29 Step 8's by-hand acceptance walk
(stats degradation with notification-service down, post-system-purge login
survival, both-theme legibility).

## Overall Assessment

- **Plan Adherence:** MOSTLY_COMPLETE
- **Recommendation:** NEEDS_FIXES

The backend is in very good shape — the purge protocol, authorization tier, and
persistence layer are implemented faithfully and tested at or above the plan's
specified rigor, with several genuine strengthenings. The gaps cluster in the
frontend and in two places where shipped comments assert behavior the code does
not have, which is the failure mode most likely to mislead the next reader.

## Action Items

1. Fix `postPurgeRouting.test.tsx` to import and render `App` as the plan
   specified, so the R5 renesting regression is actually guarded (Finding 1).
2. Decide and document the `local` overlay's exposure of notification-service's
   unauthenticated `/internal/admin/purge` — add the deny rule or record a
   conscious accept (Finding 2).
3. Satisfy FR-ADMIN-FLEET-6: add the platform-admin flag server-side via a
   `LEFT JOIN auth.platform_admins` in `platformadmin/resource.go:110`, thread it
   through `InternalUser` → `adminclient/auth.go:16` → `browse.go:124` →
   `rest.go:129` → `types/models/admin.ts`, and restore the column; correct the
   inaccurate comments at `AdminUsersPage.tsx:20-25`. Alternatively, amend the
   PRD — but the requirement cannot simply be left silently unmet (Finding 3).
4. Restore the `created` column on the users screen — the data is already in the
   payload (Finding 3).
5. Either implement the owner-email post-filter after `browse.go:227` or remove
   the claim from `browse.go:166-168`, `AdminFleetsPage.tsx:100-104`, and the
   search placeholder (Finding 4).
6. Add the `?actor=` filter control to `AdminAuditPage.tsx` — every other layer
   already supports it (Finding 5).
7. Broaden the three manifest arch-test walks beyond `entity.go`, or assert
   `discovered == Manifest ∪ excludedTables`, so the three currently-invisible
   tables and the unexercised exclusion entries are covered (Finding 6).
8. Optional: add an empty-walk vacuity guard to `TestAdminTreeIsSeparate`, and
   consider narrowing the adminclient exemption to the two files that need it
   (Finding 7).
9. Optional: remove the unused `ErrAlreadyReaped` sentinel (`admin/model.go:16`).
