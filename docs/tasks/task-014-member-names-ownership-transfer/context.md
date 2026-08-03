# Task 014 — Implementation Context

Companion to [`plan.md`](./plan.md). Read this first if you are picking the task
up cold; it is the map of what already exists, what the plan changes, and which
decisions are already settled so you do not re-litigate them.

Sources: [`prd.md`](./prd.md), [`design.md`](./design.md), [`ux-flow.md`](./ux-flow.md).

---

## 1. What this task is

Three gaps in the fleet settings Members card, closed together:

1. **Members are UUIDs.** `MemberList.tsx:41` renders `m.attributes.userId`. A
   three-person fleet reads as three indistinguishable hex strings.
2. **Removal is unguarded, and only owners can do it.** One click fires the
   DELETE with no confirmation, and the button only renders for owners — so a
   member or viewer has **no way to leave a fleet at all**.
3. **The sole-owner 409 is a dead end.** `ValidateRemoval`
   (`membership/processor.go:66`) correctly blocks a sole owner from removing
   themselves, but there is no ownership-transfer endpoint anywhere, so the
   owner who hits that 409 has nothing they can do about it.

---

## 2. Two changes, joined only in the browser

```
                    Traefik   /api/* → strip /api → svc
       │                        │
       │ GET /api/auth/users    │ PATCH  /api/fleet/fleets/{id}/members/{userId}   ← NEW
       │ POST /api/auth/refresh │ DELETE /api/fleet/fleets/{id}/members/{userId}   ← RELAXED
       ▼                        ▼
 ┌──────────────┐        ┌──────────────┐
 │ auth-service │──────► │ fleet-service│
 │ /auth/users  │  GET /internal/fleets/{id}/members  (EXISTING, unchanged)
 └──────────────┘        └──────────────┘
```

- **Naming** is an auth-service read path. fleet-service is untouched by it.
- **Ownership transfer + leaving** is a fleet-service write path. auth-service is
  untouched by it.
- The SPA zips the two client-side. Neither service learns the other's data model.

The auth→fleet call direction is the one that **already exists**
(`auth-service/internal/membership/client.go`). Reversing it would close a cycle.

---

## 3. Two PRD statements that are wrong — corrected in design

Carry these corrections; the PRD text is stale.

### 3.1 Memberships have **no** soft delete

PRD §6 claims leaving stamps `deleted_at` and cites a partial unique index at
`membership/entity.go:57`. That file is 44 lines and has **no `DeletedAt`
field**. `idx_fleet_user` is a plain unique index over `(fleet_id, user_id)`, and
`dbAdministrator.Delete` issues a **hard delete**.

Consequences:

- Re-invite still works, by a different mechanism: the row is gone, so
  `idx_fleet_user` is free and `invite.Accept`'s `tx.Create(&me)` succeeds.
- No `deleted_at IS NULL` predicates are needed anywhere.
- **Do not add soft delete.** No requirement asks for it, and it would cost a
  migration, a partial-index swap, and predicates on four existing queries.
- The `activity_events` row is therefore the **only** evidence a membership ever
  existed. That is what promotes FR-5.2 (record inside the transaction) from
  nice-to-have to load-bearing.

### 3.2 `status` is vestigial

Written as `"active"` by the builder and never changed. `ListByFleetID` returns
all rows; `ListActiveByFleetID` filters. Equivalent today given the hard delete.

Decision: **do not change `GET /fleets/{id}/members`** — a behaviour change there
would ripple into notification and dashboard consumers. Instead the *new* guard
(`ValidateRoleChange`) states its intent explicitly by requiring
`status == "active"`. One comparison, one fewer latent trap.

---

## 4. Key files

### fleet-service

| File | Today | After |
|---|---|---|
| `internal/membership/model.go` | 17 lines, getters only | + `WithRole` |
| `internal/membership/builder.go:12` | `IsValidRole` owns the role vocabulary | unchanged — **call it, never re-list the roles** |
| `internal/membership/processor.go` | `RequireOwnerInFleet`, `GetMember`, `ValidateRemoval` | + `ErrInvalidRole`, `ValidateRoleChange` |
| `internal/membership/administrator.go` | `Insert`, `Delete`, `CreateFleetWithOwner`; `NewAdministrator` returns the interface | + `ActivityRecorder`, `WithActivityRecorder`, `UpdateRole`, `Remove`; `NewAdministrator` returns `*dbAdministrator` |
| `internal/membership/resource.go` | GET + owner-only DELETE | + PATCH; DELETE branches on `isSelf`; `InitializeRoutes` takes a recorder |
| `internal/membership/provider.go` | `GetByFleetAndUser`, `CountOwners`, … | unchanged |
| `cmd/main.go:186` | `membership.InitializeRoutes(log, db)` | `(log, db, activity.Record)` |

### auth-service

| File | Today | After |
|---|---|---|
| `internal/user/provider.go` | `GetByID`, `GetBySub` | + `ListByIDs` |
| `internal/user/resource.go` | `/auth/me` GET + PATCH | + `FleetMemberGatherer`, `GET /auth/users`, `parseUserIDs`, `intersect` |
| `internal/user/rest.go` | `Attributes`, `Transform`, `TransformSlice` | unchanged — reused as-is |
| `internal/membership/client.go` | `Active` | + `FleetMemberIDs` |
| `cmd/main.go:91` | `user.InitializeRoutes(log, db)` | + a closure over `fleetClient.FleetMemberIDs` |

### web

| File | Today | After |
|---|---|---|
| `components/features/settings/MemberList.tsx` | 59 lines: UUID + role + one unguarded button | names, "(you)", three actions, three dialog modes |
| `lib/hooks/api/members.ts` | `useMembers`, `useRemoveMember(userId)` | + `useUpdateMemberRole`; `useRemoveMember({userId, isSelf})` |
| `services/api/MemberService.ts` | `listByFleet`, `removeMember` | + `updateRole` |
| `components/ui/` | button, card, form, input, label, select, skeleton, switch, textarea | + `alert-dialog.tsx` |
| `services/api/UserService.ts` | — | **new** |
| `lib/hooks/api/users.ts` | — | **new** |
| `pages/SettingsPage.tsx:52` | `<MemberList fleetId isOwner />` | **unchanged** — props do not move |

---

## 5. Settled decisions (do not re-open)

| # | Decision | Why |
|---|---|---|
| D1 | Names come from a **JWT-protected batch endpoint on auth-service**, zipped client-side | A SQL join across services is banned (D2); a fleet→auth call would close a cycle and make the list fail *whole* rather than degrading to ids |
| D2 | The lookup takes `?ids=`, **not** `?scope=fleet` | The scope form is simpler and safer but cannot serve the activity feed, whose actor ids are a larger set than "current members" |
| D3 | The fleet lookup is injected into `user.InitializeRoutes` as a **function value** | The `user` package must not import the fleet client — the same constraint that produced `PrincipalResolver`. It also makes the scoping rules unit-testable without fleet-service |
| D4 | A failed auth→fleet hop returns **500**, not an empty 200 | An empty array makes a fleet-service outage indistinguishable from a fleet with no members. FR-1.7 holds either way — the SPA renders id fallbacks regardless |
| D5 | Activity is recorded **inside** the write transaction, via an injected recorder | Copies `invite.Administrator` verbatim. A crash between two separate writes would leave a removal with no record, and per §3.1 that record is the only evidence |
| D6 | `member.removed` vs `member.left` is decided by `actor == target` | The same predicate that relaxes the authorization guard. One branch, two consequences, no chance of disagreement |
| D7 | "Transfer & leave" is **two client calls**, not an atomic endpoint | The intermediate state (two owners, actor still a member) is valid, not corrupt, and the reopened dialog lands in state 2 and completes the retry. An atomic endpoint would need a second copy of every guard |
| D8 | The successor picker **includes viewers** | Excluding them creates a second dead end: an owner whose only companion is a viewer would get an enabled Leave, an empty picker and a permanently disabled confirm |
| D9 | A promoted user's stale `member` claim is **accepted** | It fails *closed* and is bounded by the token TTL. Dropping the token fast-path would make every owner-only request pay a query before rejecting an obvious no |
| D10 | This task adds `@radix-ui/react-alert-dialog` | Inspected every worktree: nobody else adds it. `alert-dialog` not `dialog` because every dialog here is a confirm/cancel on a destructive or privilege-granting action |
| D11 | `user.Provider.ListByIDs` is **shared** with task-011; the routes stay separate | Different trust boundaries (network-restricted + admin-scoped vs. JWT + fleet-scoped), same `WHERE id IN (?)` |

---

## 6. Invariants the tests exist to protect

- **SEC-1 — the endpoint must not be a membership oracle.** A user id in another
  fleet, an id with no user row, and a malformed id all produce the *same*
  response: 200 with the id absent. There is deliberately no shape meaning "that
  user exists but you may not see them". Two tests assert this, one of them by
  comparing the two bodies byte-for-byte.
- **SEC-4 — self-leave is not privilege escalation.** The relaxed DELETE branch
  is reachable only when `identity.UserID == targetUserID`, and
  `RequireSameFleet` stays *outside* that branch so self-ness cannot bypass it.
- **SEC-5 — the database is the authority on ownership.** `RequireOwner` against
  the token is a fast path; `RequireOwnerInFleet` against the database is the
  decision. A stale `owner` claim must still 403.
- **The zero-owner invariant.** Two guards, both reachable:
  `ValidateRoleChange` (demoting the last owner → 409) and the unchanged
  `ValidateRemoval` (sole owner removing themselves → 409).
- **FR-3.8 — a failed promote is never followed by a delete.** Enforced by
  sequential `await`s inside one function, not by callback wiring.
- **FR-5.2 — a failed activity write rolls the domain write back.** The proving
  test is a stub recorder that errors, followed by asserting the membership row
  is still there.
- **FR-1.7 — a name-service failure must not blank the members card.**
  `useUsers` is a second independent query, so `useMembers` succeeding while it
  fails is a normal renderable state.

---

## 7. Traps found while reading the code

- **`||` not `??` for the name fallback.** Go marshals an unset `displayName` as
  `""`, not `null`. `??` lets the empty string through and renders a blank row.
- **`Update("role", …)` not `Save(&entity)`.** A full-row save rewrites
  `created_at` and `status` from a model read *outside* the transaction. This
  exact defect already bit `fleet.Administrator.Update` — see
  `fleet/administrator_db_test.go` and the `<-:create` tag on
  `user.Entity.CreatedAt`.
- **`mintAccessToken`, not `refreshAccessToken`, after a self-leave.** The
  removal already committed server-side; `refreshAccessToken` clears the token on
  failure and would log the user out of the session they are mid-way through
  leaving. `useAcceptInvite` records the same reasoning.
- **No manual `navigate('/onboarding')` after leaving.** Invalidating
  `authKeys.all` refetches `/auth/me`, which now reports no active fleet, and
  `RequireAuth` redirects on its own. A manual navigate is a second, racing
  source of truth.
- **`myRole` comes from the members list, not `useAuth().role`.** The list is the
  database; the auth role is a token claim that can be stale. The leave flow is
  only correct if `myRole` and `ownerCount` agree, and they only agree if both
  come from the same response.
- **404 means different things in the two client methods.** `Client.Active` maps
  404 to a zero value because "this user has no fleet" is real.
  `FleetMemberIDs` must treat it as an **error** — the fleet id came off a
  validated token, so its absence is a fault, and turning it into "no members"
  would silently blank every name.
- **SQLite test harness.** Every entity here has a schema-qualified `TableName`
  (`fleet.fleet_memberships`, `auth.users`). Tests must
  `ATTACH DATABASE ':memory:' AS fleet` (or `auth`) and create the table with
  **explicit DDL** — `AutoMigrate` emits `CREATE INDEX` with the schema prefix
  stripped and fails. Precedents: `invite/resource_test.go:36-53`,
  `user/provider_test.go:18-45`.
- **`server.Document.Data` is `any` with `omitempty`.** A non-nil
  `[]server.Resource{}` still marshals as `{"data":[]}` because `omitempty` on an
  interface field checks only for a nil interface. `TransformSlice` already
  returns a non-nil empty slice. The handler test asserts the `data` key is
  present, because `{}` and `{"data":[]}` are different answers.

---

## 8. Verification

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci     # lint-check, vet, test, build, fe-test, fe-build, manifests, carfax-template
```

No deployment manifest changes: the new auth route rides the existing
`/api/auth` IngressRoute and `FLEET_SERVICE_URL` is already configured. So no
`kustomize` re-render or cluster dry-run is required for this task — but `make
ci` still runs `manifests`, so do not break the overlays.

`node_modules` is absent in this worktree; `npm install` runs as part of Task 7.

---

## 9. Cross-branch reconciliation

Both are expected and neither should be resolved unilaterally:

- **`task-011-platform-admin-console`** also defines
  `user.Provider.ListByIDs([]string) ([]Model, error)` for its network-restricted
  admin route. Signatures are deliberately identical and neither carries scoping.
  Whoever merges second deletes their duplicate.
- **`task-012-vehicle-detail-redesign`** adds `@radix-ui/react-popover` and
  `cmdk` to the same `apps/web/package.json`. Different packages; a plain merge.

---

## 10. Explicitly out of scope

- Deleting a fleet outright (which is why ux-flow state 4 has no path forward).
- Editing the member↔viewer distinction from the UI — the PATCH endpoint accepts
  all three roles, but only "Make owner" is surfaced.
- An owner demoting themselves from the UI. The endpoint supports it; no button.
- Reassigning vehicles, maintenance records or media created by a departing
  member. Those rows keep their `created_by_user_id`.
- Member avatars. `avatarUrl` is returned but the list stays text-only.
- Activity-feed name resolution (PRD open question 3). `useUsers` is built so the
  feed can adopt it later; departed actors would fall back to the 8-character id
  exactly as FR-1.5 specifies.
- Notification/outbox events for removals or promotions.
- Soft delete for memberships (§3.1).
