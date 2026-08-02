# Plan Audit — task-014-member-names-ownership-transfer

**Plan Path:** `docs/tasks/task-014-member-names-ownership-transfer/plan.md`
**Audit Date:** 2026-08-02
**Branch:** `task-014-member-names-ownership-transfer` (HEAD `d37afa5`)
**Base Branch:** `main` (`5ff93cd`)
**Method:** every task verified against the working tree with `file:line` evidence.
Ticked checkboxes were treated as *no* evidence — they were all applied in a single
post-hoc commit (`d37afa5`).

## Executive Summary

All 11 tasks are genuinely implemented. Every interface the plan promised exists,
with the promised signature, at the promised location; every test the plan
specified exists under the same name, plus three the plan did not ask for. The
full `make ci` surface (lint-check, vet, test, build, fe-test, fe-build,
manifests, carfax-template) was re-run from this worktree and is green: 40 Go
packages `ok`, 321/321 web tests passing, both kustomize overlays rendering.

Four deviations from the plan text exist. All four are sound; two of them
(`server.Detailed`, the `@radix-ui/react-select` bump) were *necessary* — the
plan's own literal code would have failed the plan's own tests. Three PRD §10
acceptance criteria are ticked on inspection rather than on automated test
evidence; one of those (`401 without a JWT`) was explicitly sanctioned by the
plan, two were not called out.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | `WithRole` + `ValidateRoleChange` + `ErrInvalidRole` | DONE | `apps/fleet-service/internal/membership/model.go:21-24` (value receiver, value return); `processor.go:15` (`ErrInvalidRole`); `processor.go:92-119` (`ValidateRoleChange`). Vocabulary check precedes the lookup (`processor.go:93`) as specified; delegates to `IsValidRole`, does not re-list roles. All 9 plan tests present verbatim in `processor_test.go`. |
| 2 | Transactional `UpdateRole` / `Remove` + `ActivityRecorder` | DONE | `administrator.go:30` (`ActivityRecorder` type), `:44` (`NewAdministrator` now returns `*dbAdministrator`), `:49-52` (`WithActivityRecorder`), `:63-89` (`UpdateRole`, narrow `Update("role",…)` inside `db.Transaction`), `:101-118` (`Remove`, `actorUserID == m.UserID()` picks `member.left`). Interface widened at `:18-24`. 8 tests in `administrator_db_test.go:81-243` incl. both rollback cases and both nil-recorder cases. |
| 3 | PATCH endpoint + self-service DELETE | DONE | `resource.go:30` (`InitializeRoutes(log, db, rec ActivityRecorder)`), `:50-90` (PATCH handler), `:93-152` (DELETE). Guard order matches the plan's Global Constraint: `RequireSameFleet` (`:60`, `:104`) → `RequireOwner` → `RequireOwnerInFleet` → domain validation. `isSelf` is computed *after* `RequireSameFleet` (`resource.go:112`), so self-ness cannot bypass fleet scoping. Wiring at `apps/fleet-service/cmd/main.go:186`. All 14 plan tests present in `resource_test.go:85-296`. |
| 4 | `user.Provider.ListByIDs` | DONE | `apps/auth-service/internal/user/provider.go:26-30` (interface), `:66-84` (impl, empty-list short-circuit at `:71`); processor pass-through `processor.go:65-70`. Stub `ListByIDs` added to both existing fakes (`processor_test.go:33-35`, `cmd/main_test.go:38-40`) exactly as the plan's fallback note anticipated. |
| 5 | `membership.Client.FleetMemberIDs` | DONE | `apps/auth-service/internal/membership/client.go:64` (5s timeout const), `:75-115`. Non-2xx fails closed at `:99-101`; error text carries only the status code — no fleet id, no upstream body. `url.PathEscape` at `:87`. 5 tests in `client_test.go`. No new fleet-service route was added. |
| 6 | `GET /auth/users` | DONE | `apps/auth-service/internal/user/resource.go:47` (`FleetMemberGatherer`), `:53-72` (`parseUserIDs`, dedupe before cap), `:78-90` (`intersect`), `:113` (3-arg `InitializeRoutes`), `:196-239` (handler). `maxUserIDs = 100` at `:35`. Cross-fleet ids are dropped silently — no 403/404 branch exists, so the membership-oracle property holds by construction. 9 tests in `users_resource_test.go`. SEC-2 placement verified: `apps/auth-service/cmd/main.go:88-96` — the call sits inside the `pr.Use(authmw.JWT(...))` group, not a sibling initializer. |
| 7 | `alert-dialog` primitive | DONE | `apps/web/src/components/ui/alert-dialog.tsx` — all 11 exports present (`:101-113`). `@radix-ui/react-alert-dialog@^1.1.23` in `apps/web/package.json:15`. See Deviation 3 re: the overlay class. |
| 8 | `UserService.listByIds` + `useUsers` | DONE | `apps/web/src/services/api/UserService.ts:27-30`; `apps/web/src/lib/hooks/api/users.ts:25-28` (`userKeys`, sorted-join key), `:39-52` (`useUsers`, `enabled: sorted.length > 0`, `select` indexes by id). 7 tests in `users.test.ts`. The plan's optional "drop the unused `apiClient` import" was taken. |
| 9 | `useUpdateMemberRole` + self-aware `useRemoveMember` | DONE | `apps/web/src/services/api/MemberService.ts:48-62` (`updateRole`); `members.ts:68-116` (`useRemoveMember` with `{userId, isSelf}`), `:127-146` (`useUpdateMemberRole`). `userKeys.all` invalidated on settle (`members.ts:99`). 7 new tests in `members.test.ts` (plan asked for 5). See Deviation 4. |
| 10 | MemberList rewrite | DONE | `MemberList.tsx` fully rewritten. `PendingAction` single-slot state at `:41-45`; `myRole` derived from the members list not `useAuth().role` (`:68`); all four ux-flow states at `:70-72`; sequential-await transfer at `:112-125`; viewers offered as successors (`:74`, `:282-286`). 15 tests in `MemberList.test.tsx` (plan asked for 14) — all pass, incl. the Radix `Select` picker path with the plan's suggested jsdom polyfills (`MemberList.test.tsx:12-16`). See Deviation 2. |
| 11 | Full verification | DONE | Prettier commit `48d6188`; `git diff --name-only main...HEAD -- deploy/` is empty (Step 3 satisfied); full CI surface re-run below. |

**Completion Rate:** 11/11 tasks (100%) — 68/68 plan steps
**Skipped without approval:** 0
**Partial implementations:** 0

## Deviations From the Plan Text

### 1. `errRoleValidation` uses `server.Detailed`, not `fmt.Errorf("%w: …")` — SOUND, and required

`apps/fleet-service/internal/membership/resource.go:23`

```go
var errRoleValidation = server.Detailed(server.ErrValidation, "role must be one of owner, member, viewer")
```

The plan (line 1078) specified `fmt.Errorf("%w: role must be one of owner, member, viewer", server.ErrValidation)`.

This is not merely different — the plan's literal form would have failed the plan's
own test. `WriteError` (`packages/shared-go/server/jsonapi.go:33-47`) puts
`err.Error()` in `title` and only populates `detail` when the chain satisfies
`interface{ Detail() string }`. A `%w` wrap does not. `TestPatchRole_rejectsAnUnknownRoleWithTheAllowList`
(`resource_test.go:133`) decodes `errors[0].detail` and asserts it names `role`,
`owner`, `member`, `viewer`. With the plan's form that detail would have been the
empty string. The security property the plan actually cared about is preserved:
the message is a package-level constant, no caller input is interpolated, and the
test asserts `"admin"` does *not* appear in the response.

Note this makes `errRoleValidation` inconsistent with the sibling
`errThemeValidation` (`apps/auth-service/internal/user/resource.go:30`), which
still uses the `%w` form and therefore still renders its message only into
`title`. Not this task's bug, but the two now differ.

### 2. `displayFor` lives in `lib/utils/displayName.ts`, not exported from `MemberList.tsx` — SOUND

`apps/web/src/lib/utils/displayName.ts:14-17`

The plan's Task 10 "Produces" line said `MemberList.tsx` also exports `displayFor`.
Exporting a non-component from a `.tsx` component module trips the
`react-refresh/only-export-components` rule that `make lint-check` enforces. The
extraction is the right resolution rather than a lint suppression, and it better
serves the plan's own stated motive ("available for reuse when the activity feed
adopts name resolution later") — the feed can now import it without pulling in a
settings component. The `||`-not-`??` constraint is preserved
(`displayName.ts:16`) and covered by `MemberList.test.tsx:115` ("falls through an
empty-string displayName to the email").

### 3. `--overlay` design token instead of `bg-black/80`; `@radix-ui/react-select` bumped — SOUND, but scope additions

Neither change appears in the plan.

- `alert-dialog.tsx:15` uses `bg-overlay/80` against a new `--overlay` token
  (`apps/web/src/index.css:13-15`, `:62-64`; `tailwind.config.ts:23`). The plan
  specified `bg-black/80`. The token is deliberately the same dark value in both
  themes, so the visual result is unchanged; this satisfies the frontend
  guideline against hardcoded colors. Additive — no existing component changes.
- `apps/web/package.json:17` bumps `@radix-ui/react-select` `^2.2.6` → `^2.3.7`.
  Verified this achieves what the commit message claims: `package-lock.json` now
  resolves a **single** `@radix-ui/react-focus-scope@1.1.16` (lines 1665, 1973,
  2416) and `find node_modules -name react-focus-scope` returns exactly one
  directory. Two copies meant two focus-scope stacks, which is a real
  focus-trap-inside-focus-trap bug for a `Select` nested in an `AlertDialog` —
  exactly the FR-3.7 successor picker. Without this, `MemberList.test.tsx`'s
  picker tests could not pass.

  This is the riskiest deviation because `Select` is used outside this feature.
  Mitigation confirmed: the full 321-test web suite passes, including
  `MaintenanceRecordForm.test.tsx` which drives a `Select`.

### 4. `useRemoveMember` mints in `onSuccess`, not `mutationFn`, with a null-token guard — SOUND

`apps/web/src/lib/hooks/api/members.ts:70-99`

The plan (line 2680-2693) put `await mintAccessToken()` inside `mutationFn`. The
implementation moved it to `onSuccess` and added:

```ts
const token = await mintAccessToken();
if (!token) { toast.error('You left the fleet, but your session could not be updated. …'); return; }
```

`mintAccessToken` returns `Promise<string | null>` (`apps/web/src/lib/api/refresh.ts:45`)
— it resolves `null` on failure rather than rejecting, so the plan's version
would have silently proceeded to invalidate `authKeys` with a stale token and
left the user staring at a fleet they had already left. The guard mirrors the
existing `useAcceptInvite` precedent in the same file, and the new test
(`members.test.ts:216-232`) is a direct analogue of the pre-existing
`useAcceptInvite` mint-failure test (`members.test.ts:425-441`). The plan's own
assertions (`mintAccessToken` called once; `authKeys.all` invalidated) still hold
on the happy path.

## PRD §10 Acceptance Criteria — Test Coverage

Every criterion has real coverage except the three noted.

| Criterion group | Coverage |
|---|---|
| Names (display/email/id fallback, "(you)", 200-with-omission, 422 caps, degraded render) | `MemberList.test.tsx:92,115,124,137`; `users_resource_test.go:75,102,126,144,159,179,191,206,228`; `provider_test.go` ×3 |
| Confirmation (dialog gates DELETE; cancel fires nothing; Leave dialog) | `MemberList.test.tsx:155,173,229` |
| Leaving (member/viewer 204; member/viewer 403 on others; sole-member disabled) | `resource_test.go:209,228,245,260,276,289`; `MemberList.test.tsx:345` |
| Ownership transfer (promote, 403 non-owner, 403 stale claim, 422 `admin`, 404 non-member, 409 sole-owner demote, no-op 200, successor required, ordered transfer, plain dialog for co-owner, Make owner visibility) | `resource_test.go:85,105,120,133,153,164,178`; `MemberList.test.tsx:193,213,249,263,278,308,329` |
| Activity (`member.role_changed` / `member.removed` / `member.left`) | `administrator_db_test.go:81,122,176,200` |
| `make ci` passes | Re-verified below |

**Gaps (all Minor):**

1. **"A request with no JWT returns 401" — no automated test.** Sanctioned by the
   plan (Task 6 Step 7): the handler tests inject an `Identity` directly, and the
   middleware lives in `cmd/main.go`. I verified the placement myself —
   `apps/auth-service/cmd/main.go:88-96` registers `user.InitializeRoutes` on the
   `pr` router inside `pr.Use(authmw.JWT(...))`. The property holds today but is
   protected only by code review; moving the initializer to a sibling
   `AddRouteInitializer` would make `/auth/users` world-readable with no test
   failing.
2. **"After leaving, the SPA refreshes the session and lands on onboarding."**
   The refresh half is tested (`members.test.ts:180`); the "lands on onboarding"
   half is not — there is no routing test asserting `RequireAuth` redirects.
3. **"the fleet retains exactly one owner"** after Transfer & leave. The test
   (`MemberList.test.tsx:278`) asserts call *ordering* (`['patch','delete']`) and
   arguments, which implies the invariant, but the resulting owner count is never
   asserted. The server-side guard that actually protects this
   (`ValidateRoleChange` / `ValidateRemoval`) *is* directly tested.

## Build & Test Results

All commands run from
`/home/tumidanski/source/MyFleet/.worktrees/task-014-member-names-ownership-transfer`.

| Target | Result | Notes |
|---|---|---|
| `go build github.com/jtumidanski/myfleet/...` | **PASS** | no output |
| `go vet github.com/jtumidanski/myfleet/...` | **PASS** | no output |
| `go test -race -count=1 github.com/jtumidanski/myfleet/...` | **PASS** | 40 packages `ok`, 0 failures. Incl. `apps/fleet-service/internal/membership` and `apps/auth-service/internal/{user,membership}` |
| `npm run -w apps/web test` | **PASS** | 41 files, **321/321** tests. `MemberList.test.tsx` 15/15 |
| `npm run -w apps/web build` | **PASS** | 1828 modules, built in 3.73s (pre-existing >500 kB chunk warning only) |
| `./tools/lint.sh --check` | **PASS** | `0 issues` ×6, prettier clean, eslint `--max-warnings 0` clean |
| `make manifests` | **PASS** | both overlays render; no PVC/Secret/ClusterRole/placeholders in `main`; IngressRoute parity 7/7 |
| `make carfax-template` | **PASS** | all three homes agree |

> Note for anyone re-running: the plan's `go build ./...` / `go test ./...`
> instructions (Tasks 1-6) do **not** work from the workspace root — there is no
> root `go.mod`, so `./...` errors with *"directory prefix . does not contain
> modules listed in go.work"*. Use the Makefile's
> `github.com/jtumidanski/myfleet/...` form. This is a defect in the plan's
> instructions, not in the implementation.

`git diff --name-only main...HEAD -- deploy/` → empty. No manifest drift.

## Other Observations (non-blocking)

- **`membership.Administrator.Delete` is now dead in production code.**
  `administrator.go:16` / `:59`. The DELETE handler uses `Remove`; the only
  remaining callers are tests. The plan explicitly chose to retain it ("part of
  the existing contract"), so this is intentional — but it is now an unaudited
  hard-delete path sitting next to an audited one, and nothing prevents a future
  caller picking the wrong one.
- **`useRemoveMember.onSettled` invalidates `memberKeys.lists()` and `fleetKeys.all`
  after a self-leave** (`members.ts:96-100`), which will refetch fleet-scoped
  queries the user is no longer authorized for. Harmless (the redirect wins the
  race in practice) and it matches the pre-existing pattern, but it will emit
  403/404s in the network log.
- **`MemberList` when the caller is absent from the members list**: `myRole` is
  `undefined` (`MemberList.tsx:68`), so `soleOwner`/`leaveBlocked` are both false
  and no self row renders — no Leave button appears. Correct by accident rather
  than by intent; worth a comment if the list ever paginates.

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE

Every task is implemented as specified or better. The four deviations are all
improvements on the plan text — two of them fix plan bugs that would have
prevented the plan's own tests from passing. The full CI surface is green from a
clean re-run.

## Action Items

None blocking. Optional follow-ups, in priority order:

1. Consider a test for the "lands on onboarding" half of the self-leave flow, or
   downgrade that PRD §10 line to "verified manually" — it is currently ticked
   with no automated evidence.
2. Consider aligning `errThemeValidation`
   (`apps/auth-service/internal/user/resource.go:30`) with the `server.Detailed`
   form now used by `errRoleValidation`, so validation messages render into
   JSON:API `detail` consistently across services. Out of scope for this task.
3. Fix the plan's `go build ./...` / `go test ./...` instructions in the
   plan-writing skill or template — they cannot work from a Go workspace root and
   will mislead the next executor.
4. Track the reconciliations the plan flagged at lines 3592-3595 (duplicate
   `user.Provider.ListByIDs` with `task-011`; `package.json` merge with
   `task-012`) at merge time.
