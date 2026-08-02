# Member Names & Ownership Transfer — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02

---

## 1. Overview

The fleet settings Members card is the only place in MyFleet where one household
member sees another. Today it shows a raw UUID and a role
(`MemberList.tsx:41`), so a fleet with three people reads as three
indistinguishable hex strings. Nobody can tell who they are about to remove.

The same card's only action is an unguarded destructive button. One click fires
`DELETE /fleets/{id}/members/{userId}` with no confirmation
(`MemberList.tsx:44-53`). And the button only renders for owners, which means a
regular member or viewer has **no way to leave a fleet at all** — the endpoint
gates on `RequireOwner` at both the token and database layers
(`membership/resource.go:50-58`). The one guard that does exist,
`ValidateRemoval` (`membership/processor.go:66`), returns 409 when the sole owner
tries to remove themselves. That guard is correct, but it is a dead end: there is
no ownership-transfer endpoint anywhere in fleet-service, so the owner who hits
that 409 has nothing they can do about it.

This task closes all three gaps. Members get human names, sourced from
auth-service through a new same-fleet-scoped lookup endpoint. Every removal —
whether an owner removing someone else or a person leaving on their own — passes
through an explicit confirmation dialog. Non-owners gain the ability to leave.
And ownership becomes transferable, both as a standalone "Make owner" action and
as a required step inside the sole owner's leave flow.

## 2. Goals

Primary goals:

- Show each fleet member by display name (falling back to email, then a
  shortened id) instead of a bare UUID.
- Require an explicit confirmation before any membership is removed.
- Let members and viewers leave a fleet on their own.
- Let an owner grant ownership to another member, so a sole owner has a real
  path out of the 409 they hit today.
- Keep the sole-owner invariant intact: a fleet must never end up with zero
  owners.

Non-goals:

- Deleting a fleet outright.
- Editing the member↔viewer distinction from the UI. The PATCH endpoint accepts
  all three roles, but this task only surfaces "Make owner".
- An owner demoting themselves from the UI. The endpoint supports it (with the
  sole-owner guard); no button is added.
- Multi-fleet membership or a fleet switcher.
- Reassigning vehicles, maintenance records, or media created by a departing
  member. Those rows keep their original `created_by_user_id`.
- Rendering member avatars. `avatarUrl` is returned by the new endpoint but the
  list stays text-only.
- Re-inviting someone who has left. The existing invite flow already covers it
  unchanged.

## 3. User Stories

- As a fleet owner, I want to see my members' names in the settings list so that
  I can tell them apart before acting on one of them.
- As a fleet owner, I want to confirm before removing someone so that a stray
  click does not silently revoke a family member's access.
- As a fleet member, I want to leave a fleet I no longer belong to so that I am
  not stuck waiting for an owner to remove me.
- As a fleet owner who is leaving, I want to hand ownership to another member as
  part of leaving so that the fleet is not orphaned and I am not blocked by an
  unexplained conflict error.
- As a fleet owner, I want to promote another member to owner without leaving so
  that we can share administration of the household fleet.
- As any member, I want to be warned about what I lose before I leave so that I
  do not confuse "leave" with "log out".

## 4. Functional Requirements

### 4.1 Member identity resolution

- **FR-1.1** auth-service exposes a JWT-protected batch user lookup that returns
  `displayName`, `email`, and `avatarUrl` for a set of user ids.
- **FR-1.2** The lookup is **scoped to the caller's active fleet**. auth-service
  resolves the caller's `active_fleet_id` from the JWT, fetches that fleet's
  active member ids from fleet-service's existing internal endpoint, and
  intersects the requested ids against that set. Requested ids outside the set
  are **silently omitted** from the response — never rejected with an error, so
  the endpoint cannot be used to probe whether a user id exists.
- **FR-1.3** A caller with no active fleet receives an empty `data` array.
- **FR-1.4** A requested id that is a fleet member but has no corresponding user
  row is omitted from `data`. This is not an error.
- **FR-1.5** The web member list renders, per member, in priority order:
  `displayName` → `email` → the first 8 characters of the user id. The role is
  shown beneath the name as it is today.
- **FR-1.6** The member matching the authenticated user is marked "(you)".
- **FR-1.7** While names are loading, the list renders the existing skeleton. If
  the name lookup fails, the list still renders with the id fallback from
  FR-1.5 — a name-service failure must not blank the members card.

### 4.2 Ownership transfer

- **FR-2.1** fleet-service exposes `PATCH /fleets/{id}/members/{userId}` that
  changes a membership's role.
- **FR-2.2** The endpoint is owner-only, guarded at both layers: `RequireOwner`
  against the token as a fast path, then `RequireOwnerInFleet` against the
  database as the authoritative stale-claim check, matching the pattern the
  DELETE handler already uses.
- **FR-2.3** Accepted roles are `owner`, `member`, `viewer`. Anything else is a
  422 naming the field and the allow-list, matching the existing theme-validation
  convention in `auth-service/internal/user/resource.go:21`.
- **FR-2.4** The target must be an existing active membership in the same fleet.
  Otherwise 404.
- **FR-2.5** **Multiple owners are permitted.** Promoting a member to owner does
  not demote the promoter. A fleet may hold any number of owners.
- **FR-2.6** A role change that would leave the fleet with zero owners is
  rejected with 409. Concretely: the target is currently `owner`, the requested
  role is not `owner`, and `CountOwners(fleetID) <= 1`.
- **FR-2.7** A PATCH that sets the role a membership already has succeeds as a
  no-op and returns 200 with the unchanged resource.
- **FR-2.8** The web member list shows a "Make owner" action on every active
  non-owner member, visible only to owners, behind its own confirmation.

### 4.3 Leaving and removal

- **FR-3.1** `DELETE /fleets/{id}/members/{userId}` is relaxed so that a member
  removing **themselves** does not require the owner role. Removing **anyone
  else** still requires owner at both the token and database layers.
- **FR-3.2** The existing sole-owner guard is unchanged: self-removal where the
  target's role is `owner` and `CountOwners(fleetID) <= 1` returns 409.
- **FR-3.3** Because multiple owners are permitted, an owner may remove another
  owner. This cannot orphan the fleet — the actor is themselves an owner and
  remains.
- **FR-3.4** Every member sees a "Leave" action for their own row, regardless of
  role. Owners continue to see "Remove" on other members' rows.
- **FR-3.5** Removing another member opens a confirmation dialog naming the
  person: *"Remove {name} from this fleet?"* with an explanation that they lose
  access to all the fleet's vehicles and records. The confirm button is styled
  destructive.
- **FR-3.6** Leaving opens a confirmation dialog explaining that the user loses
  access to the fleet's vehicles and records, and that rejoining requires a new
  invite.
- **FR-3.7** When the leaver is the **sole owner**, the leave dialog additionally
  requires choosing a successor from a select populated with the fleet's other
  active members. The confirm button is disabled until a successor is chosen and
  is labelled "Transfer & leave".
- **FR-3.8** The "Transfer & leave" action performs the promote (§4.2) followed
  by the self-delete. If the promote fails, the delete is not attempted and the
  error surfaces as a toast. If the promote succeeds and the delete fails, the
  fleet is left with two owners and the user still a member — a benign,
  recoverable state that the reopened dialog reflects correctly.
- **FR-3.9** When the leaver is an owner but **not** the sole owner, no successor
  picker is shown; the plain leave confirmation applies.
- **FR-3.10** A fleet with exactly one member, who is the owner, shows the Leave
  action disabled with an explanation that a sole member cannot leave their own
  fleet. There is no one to transfer to. (Deleting the fleet is out of scope.)

### 4.4 Post-mutation session handling

- **FR-4.1** `role` and `active_fleet_id` are JWT claims minted at login
  (`session/processor.go:64`). After any mutation that changes the **actor's own**
  membership, the SPA must `POST /api/auth/refresh` before continuing, so the
  claims match the database.
- **FR-4.2** After a successful self-leave, the refreshed token resolves to an
  empty `active_fleet_id` (fleet-service's `/internal/memberships/active` returns
  404, which `membership.Client.Active` maps to a zero value by design). The SPA
  routes the user to onboarding.
- **FR-4.3** Mutations invalidate `memberKeys.lists()` and `fleetKeys.all`, as
  the current `useRemoveMember` already does, plus the new users-by-id query.
- **FR-4.4** No refresh is needed when an owner promotes or removes *someone
  else* — the actor's own claims are untouched.

### 4.5 Activity log

- **FR-5.1** Role changes and removals record fleet-level activity entries
  (`vehicleID` nil, as the existing `member.invited` comment in
  `activity/model.go:10` anticipates) with types `member.role_changed`,
  `member.removed`, and `member.left`.
- **FR-5.2** Activity is recorded inside the same transaction as the mutation so
  a failed write cannot leave a phantom entry.

## 5. API Surface

### 5.1 New — `GET /auth/users?ids=<id>,<id>` (auth-service)

Public path `/api/auth/users`; Traefik strips only `/api`
(`deploy/k8s/base/routing/middlewares.yaml:7`). Registered in the JWT-protected
group alongside `/auth/me` (`auth-service/cmd/main.go:88-92`).

Request: `ids` is a comma-separated list of user ids.

| Condition | Status |
|---|---|
| Success | 200 |
| `ids` missing or empty | 422 |
| More than 100 ids | 422 |
| No JWT | 401 |

```json
{
  "data": [
    {
      "type": "users",
      "id": "8f2c…",
      "attributes": {
        "displayName": "Jane Doe",
        "email": "jane@example.com",
        "avatarUrl": "https://…"
      }
    }
  ]
}
```

Ids not in the caller's active fleet, and ids with no user row, are omitted from
`data` without comment.

Implementation notes:

- auth-service already holds a fleet-service internal client
  (`membership.NewClient`, wired at `auth-service/cmd/main.go:49`). Extend it
  with `FleetMembers(ctx, fleetID) ([]Member, error)` against the **existing**
  `GET /internal/fleets/{fleetID}/members` endpoint
  (`fleet-service/internal/membership/resource.go:114`). No new fleet-service
  internal route is required.
- `user.Provider` needs a `ListByIDs([]string) ([]Model, error)`.
- The direction of the call is auth → fleet, which is the direction that already
  exists. No dependency cycle is introduced.

### 5.2 New — `PATCH /fleets/{id}/members/{userId}` (fleet-service)

Public path `/api/fleet/fleets/{id}/members/{userId}`.

```json
{ "data": { "type": "memberships", "attributes": { "role": "owner" } } }
```

| Condition | Status |
|---|---|
| Success | 200, updated `memberships` resource |
| Role not in `owner`\|`member`\|`viewer` | 422 |
| Caller not an owner (token or DB) | 403 |
| Caller's active fleet ≠ `{id}` | 404 |
| Target membership not found in fleet | 404 |
| Would leave the fleet with zero owners | 409 |

### 5.3 Modified — `DELETE /fleets/{id}/members/{userId}` (fleet-service)

Guard order changes to branch on self vs. other:

1. `RequireSameFleet(identity, fleetID)` → 404 on mismatch.
2. `isSelf := identity.UserID == targetUserID`.
3. If **not** self: `RequireOwner(identity)` then
   `proc.RequireOwnerInFleet(fleetID, identity.UserID)` → 403.
   If self: **no role requirement** — the actor's own membership row is the
   authority.
4. `proc.GetMember(fleetID, targetUserID)` → 404 if absent.
5. `proc.ValidateRemoval(...)` → 409 (unchanged).
6. Delete → 204.

The response contract is otherwise unchanged. The 409 remains reachable and the
frontend keeps handling it as a backstop, even though the new UI prevents
reaching it in the normal flow.

### 5.4 Unchanged

`GET /fleets/{id}/members` keeps its current shape. Names are joined client-side
per the chosen approach; the membership resource gains no new attributes.

## 6. Data Model

**No schema migration.** `fleet.fleet_memberships.role` is already a plain
`not null` string (`membership/entity.go:22`) and the PATCH writes it in place.

New code-level additions:

- `membership.Model.WithRole(role string) Model` — immutable transition,
  matching the `user.Model.WithLogin` pattern (`user/model.go:34`).
- `membership.Administrator.UpdateRole(id, role string) error`.
- `user.Provider.ListByIDs(ids []string) ([]Model, error)` in auth-service.

Soft-delete behaviour is untouched: leaving stamps `deleted_at`, and the partial
unique index predicated on `deleted_at IS NULL`
(`membership/entity.go:57`) means a departed member can be re-invited and rejoin
without colliding with their old row. This is exactly the lockout scenario that
index was built to prevent, and the leave flow depends on it.

## 7. Service Impact

**auth-service**

- New `GET /auth/users` route in the JWT-protected group.
- `internal/user`: `Provider.ListByIDs`, and the batch transform (`TransformSlice`
  already exists at `user/rest.go:18`).
- `internal/membership`: new `FleetMembers` client method + `Member` type.
- New resource-level tests for the same-fleet containment rule.

**fleet-service**

- `internal/membership/resource.go`: new PATCH handler; DELETE guard restructured
  around self vs. other.
- `internal/membership/processor.go`: `ValidateRoleChange` implementing FR-2.3,
  FR-2.4, FR-2.6.
- `internal/membership/administrator.go`: `UpdateRole`.
- `internal/membership/model.go`: `WithRole`.
- Activity recording for the three new event types.

**web**

- New dependency `@radix-ui/react-alert-dialog` and a shadcn
  `components/ui/alert-dialog.tsx`. **There is currently no dialog primitive of
  any kind in `apps/web/src/components/ui/`** — this is a genuinely new addition,
  not a reuse. Check whether `task-011-platform-admin-console` or
  `task-012-vehicle-detail-redesign` land one first and reconcile rather than
  duplicate.
- New `services/api/UserService.ts` with `listByIds`.
- New `lib/hooks/api/users.ts` exposing `useUsers(ids)` keyed on the sorted id
  list, returning a `Record<string, UserAttributes>`.
- `lib/hooks/api/members.ts`: new `useUpdateMemberRole`, and `useRemoveMember`
  extended to refresh the session when the removal is a self-leave.
- `components/features/settings/MemberList.tsx`: names, "(you)", Leave/Remove/
  Make owner actions, and the three dialogs.
- Tests for the sole-owner leave flow, the successor picker, and the name
  fallback chain.

**notification-service / media-service** — unaffected.

**deploy** — no manifest change. The new auth route rides the existing
`/api/auth` IngressRoute and the existing `FLEET_SERVICE_URL` config.

## 8. Non-Functional Requirements

**Security**

- **SEC-1** `GET /auth/users` must never return a user outside the caller's
  active fleet. This gets a dedicated test: caller in fleet A requests a user id
  belonging to fleet B and receives an empty `data`, not a 403 and not a 404 —
  the response must be indistinguishable from asking about an id that does not
  exist, so membership in another fleet cannot be probed.
- **SEC-2** The endpoint is unreachable without a JWT.
- **SEC-3** The `ids` cap (100) bounds both the response size and the work done
  per request.
- **SEC-4** Self-leave must not become a privilege escalation: the relaxed
  DELETE branch applies **only** when `identity.UserID == targetUserID`. A test
  must assert a viewer cannot delete another member.
- **SEC-5** PATCH keeps the database-authoritative owner check. A stale `owner`
  claim in a token that the database no longer backs must be rejected — the same
  reason `RequireOwnerInFleet` exists today.

**Performance**

- One additional internal hop (auth → fleet) per member-list load. The React
  Query `staleTime` of 60s already applied to `useMembers` should apply to
  `useUsers` too.
- The name lookup is a single batch request, not one per member.

**Observability**

- Role changes and removals are recorded in the activity log (§4.5), which is
  the auditable record of who removed or promoted whom.
- Error paths follow the existing `server.WriteError` envelope. Per the existing
  convention in `auth-service/internal/membership/client.go:46-50`, user ids must
  not be interpolated into error messages that reach logs.

## 9. Open Questions

1. **Overlap with task-011.** `task-011-platform-admin-console` (in flight,
   unmerged) specifies `GET /internal/admin/users?ids=a,b,c → {email,
   displayName}` on auth-service. That one is network-restricted and
   admin-scoped; this one is JWT-protected and fleet-scoped. They are genuinely
   different trust boundaries and should stay separate routes, but the
   `ListByIDs` provider method and the attribute shape should be shared. Whoever
   merges second reconciles.
2. **Alert-dialog ownership.** Same question for the shadcn `alert-dialog`
   component — three in-flight tasks may each want one. Design phase should check
   the other worktrees before adding the dependency.
3. **Should a departing member's created records be attributed differently?**
   Currently `created_by_user_id` points at a user who is no longer in the fleet,
   and the activity feed would resolve that id to nothing once the name lookup
   is fleet-scoped. Proposal: the activity feed falls back to the shortened id,
   same as FR-1.5. Confirm during design.
4. **Should the successor picker exclude viewers?** Promoting a viewer straight
   to owner is a two-step jump. FR-2.3 permits it; whether the picker offers it
   is a UX call for the design phase.

## 10. Acceptance Criteria

**Names**

- [ ] The settings Members card shows display names, not UUIDs.
- [ ] A member with no `displayName` shows their email; with neither, the first
      8 characters of their id.
- [ ] The authenticated user's own row is marked "(you)".
- [ ] `GET /api/auth/users?ids=…` returns 200 with only the ids that are active
      members of the caller's active fleet.
- [ ] A request for a user id in a *different* fleet returns 200 with that id
      absent from `data` — not 403, not 404.
- [ ] A request with no `ids`, or more than 100, returns 422.
- [ ] A request with no JWT returns 401.
- [ ] Simulating a failure of the name lookup still renders the member list with
      id fallbacks.

**Confirmation**

- [ ] Clicking Remove on another member opens a dialog naming that member; the
      DELETE fires only after confirming.
- [ ] Cancelling the dialog fires no request.
- [ ] Clicking Leave opens a dialog explaining the consequences; the DELETE fires
      only after confirming.

**Leaving**

- [ ] A `member` can leave their own fleet and receives 204.
- [ ] A `viewer` can leave their own fleet and receives 204.
- [ ] A `viewer` attempting to delete *another* member receives 403.
- [ ] A `member` attempting to delete *another* member receives 403.
- [ ] After leaving, the SPA refreshes the session and lands on onboarding.
- [ ] A sole member who is the owner sees Leave disabled with an explanation.

**Ownership transfer**

- [ ] An owner can PATCH another member's role to `owner`; both are owners
      afterwards.
- [ ] A non-owner PATCHing any role receives 403.
- [ ] An owner with a stale `owner` token claim not backed by the database
      receives 403.
- [ ] PATCH with `role: "admin"` returns 422 naming the field and the allowed
      values.
- [ ] PATCH against a user who is not a member of the fleet returns 404.
- [ ] Demoting the only owner returns 409.
- [ ] PATCH setting a role the member already has returns 200 unchanged.
- [ ] The sole owner's Leave dialog requires a successor; the confirm button is
      disabled until one is selected.
- [ ] "Transfer & leave" promotes the successor and then removes the actor; the
      fleet retains exactly one owner.
- [ ] An owner who is *not* the sole owner sees the plain leave dialog with no
      successor picker.
- [ ] The standalone "Make owner" action is visible to owners on non-owner rows
      and hidden from non-owners.

**Activity**

- [ ] A role change writes a `member.role_changed` activity entry.
- [ ] An owner-initiated removal writes `member.removed`.
- [ ] A self-leave writes `member.left`.

**Build**

- [ ] `make ci` passes (lint-check, vet, test, build, fe-test, fe-build).
