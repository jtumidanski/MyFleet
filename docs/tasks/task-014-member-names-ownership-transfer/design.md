# Member Names & Ownership Transfer — Design

Task: `task-014-member-names-ownership-transfer`
Input: [`prd.md`](./prd.md), [`ux-flow.md`](./ux-flow.md)
Status: Approved for planning

---

## 0. PRD corrections

Two statements in the PRD do not match the code on this branch. They are
corrected here and the corrections propagate through the rest of this document.

### 0.1 Membership has no soft delete

PRD §6 says:

> Soft-delete behaviour is untouched: leaving stamps `deleted_at`, and the
> partial unique index predicated on `deleted_at IS NULL`
> (`membership/entity.go:57`) means a departed member can be re-invited…

`apps/fleet-service/internal/membership/entity.go` is 44 lines long and has **no
`DeletedAt` field**:

```go
type Entity struct {
	ID        string `gorm:"type:uuid;primaryKey"`
	FleetID   string `gorm:"type:uuid;not null;uniqueIndex:idx_fleet_user"`
	UserID    string `gorm:"not null;uniqueIndex:idx_fleet_user"`
	Role      string `gorm:"not null"`
	Status    string `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
```

`idx_fleet_user` is a plain (non-partial) unique index over `(fleet_id,
user_id)`, and `dbAdministrator.Delete` (`membership/administrator.go:32`) issues
`db.Delete(&Entity{}, "id = ?", id)` against an entity with no `gorm.DeletedAt`
— a **hard delete**. Other fleet-service domains (`vehicle`,
`maintenancerecord`, `fuel`, `vehiclemedia`) do carry `DeletedAt *time.Time`;
membership deliberately does not.

Consequences, all of them favourable:

- The re-invite path the PRD wanted still works, by a different mechanism. The
  row is gone, so `idx_fleet_user` is free and `invite.Accept`'s
  `tx.Create(&me)` (`invite/administrator.go:86`) succeeds without needing a
  partial index.
- There is no tombstone to hide, so `CountOwners` and `ListByFleetID` do not
  need a `deleted_at IS NULL` predicate.
- **We are not adding soft delete.** Introducing `deleted_at` here would require
  a migration, a partial-index swap, and predicates on four existing queries —
  none of which any requirement in this PRD asks for. The design keeps the hard
  delete.
- The audit trail for a departure lives in `activity_events` (§4.5), not in a
  tombstone row. That is now the *only* record that someone left, which raises
  the importance of FR-5.1 from "nice observability" to "the sole evidence".

### 0.2 `status` is currently vestigial

`Status` is written as `"active"` by the builder and never changed. The list
endpoint (`ListByFleetID`) returns *all* rows regardless of status; the internal
recipient endpoint (`ListActiveByFleetID`) filters. Given the hard delete these
are equivalent today.

Decision: **do not change `GET /fleets/{id}/members`** (PRD §5.4 says
unchanged, and a behaviour change there would ripple into the notification and
dashboard consumers). Instead, the *new* guards state their intent explicitly —
`ValidateRoleChange` requires `status == "active"` — so that the semantics stay
correct if `status` ever gains a second value. This costs one comparison and
removes a latent trap.

---

## 1. Architecture at a glance

```
                    ┌────────────────────────────────────────┐
  browser  ──────►  │  Traefik   /api/* → strip /api → svc   │
                    └────────────────────────────────────────┘
       │                        │                        │
       │ GET /api/auth/users    │ GET  /api/fleet/fleets/{id}/members
       │ POST /api/auth/refresh │ PATCH /api/fleet/fleets/{id}/members/{userId}   ← NEW
       │                        │ DELETE /api/fleet/fleets/{id}/members/{userId}  ← RELAXED
       ▼                        ▼
 ┌──────────────┐        ┌──────────────┐
 │ auth-service │        │ fleet-service│
 │              │        │              │
 │ /auth/users  │        │ membership   │
 │   ▲          │        │   resource   │
 │   │ scope    │        │   processor  │
 │   │          │        │   admin ──► activity.Record (same tx)
 │   └──────────┼───────►│ /internal/fleets/{id}/members   (existing, unchanged)
 │  membership. │        │              │
 │  Client      │        └──────────────┘
 └──────────────┘
```

Two independent changes joined only at the SPA:

1. **Naming** is an auth-service read path. fleet-service is untouched by it;
   the existing internal members endpoint is consumed, not modified.
2. **Ownership transfer + leaving** is a fleet-service write path. auth-service
   is untouched by it; the SPA re-mints its token afterwards through the
   existing `/auth/refresh`.

The SPA joins the two client-side. No service learns about the other's data
model.

---

## 2. Decision log

### D1 — Where member names come from

**Chosen: a JWT-protected batch endpoint on auth-service, joined client-side.**

| Option | Verdict |
|---|---|
| (a) fleet-service joins `auth.users` in SQL | **Rejected.** Violates the standing no-cross-service-DB-join rule (D2, cited throughout `membership/resource.go:86-113`). fleet-service has no connection to auth's schema. |
| (b) fleet-service calls auth-service and denormalises `displayName` onto the membership resource | **Rejected.** Introduces a fleet→auth dependency where only auth→fleet exists today; auth-service already calls fleet-service (`auth-service/internal/membership/client.go`), so this closes a cycle. It also makes the member list fail *whole* when auth-service is slow, instead of degrading to ids. |
| (c) auth-service exposes a batch lookup; the SPA fetches memberships and names in parallel and zips them | **Chosen.** Preserves the existing call direction, keeps the two failures independent (FR-1.7), and is reusable by the activity feed later. |

Cost of (c): two round-trips from the browser instead of one, and one extra
internal hop (auth→fleet) per member-list load. At household scale (single-digit
members, 60 s `staleTime`) this is not worth optimising.

### D2 — How auth-service scopes the lookup

**Chosen: intersect the requested ids against the caller's active fleet's member
list, and silently drop the rest.**

The alternative considered was dropping the `ids` parameter entirely —
`GET /auth/users?scope=fleet` returning *every* member of the caller's active
fleet. It is strictly simpler and strictly safer: no id list to validate, no
100-cap, no probing surface at all, because the caller never names an id.

Rejected because it cannot serve the second consumer. The activity feed resolves
*actor* ids drawn from `activity_events`, which is a different (and larger) set
than "current members" — the ids form degrades gracefully there (unknown ids are
omitted), the scope form cannot express the question. Building the narrow one
now and a second one later is worse than building the general one once.

The safety property is preserved by making omission the *only* failure mode:

- id belongs to another fleet → omitted
- id has no `users` row → omitted
- id is malformed → omitted
- caller has no active fleet → `data: []`

All four are indistinguishable on the wire. This is what makes SEC-1 hold: there
is no response shape that means "that user exists but you may not see them".

### D3 — Wiring the fleet lookup into `user.InitializeRoutes`

`user.InitializeRoutes(log, db)` currently takes no collaborators, and the
`user` package must not import `membership` (auth-service's `main.go:47-49`
already documents this constraint — the `PrincipalResolver` exists precisely to
avoid that import).

**Chosen: inject a function value**, matching both the existing
`newPrincipalResolver` composition in auth-service and the `ActivityRecorder`
pattern in fleet-service:

```go
// user/resource.go
// FleetMemberGatherer returns the user ids of the active members of a fleet.
// Injected as a function value so the user package never imports the
// fleet-service client (auth-service/cmd/main.go Decision 1).
type FleetMemberGatherer func(ctx context.Context, fleetID string) ([]string, error)

func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, members FleetMemberGatherer) func(chi.Router)
```

`main.go` composes it from the existing `fleetClient`:

```go
user.InitializeRoutes(log, db, func(ctx context.Context, fleetID string) ([]string, error) {
	return fleetClient.FleetMemberIDs(ctx, fleetID)
})(pr)
```

Rejected alternatives: a new `internal/directory` package (more surface for one
handler, and it would still need the same injection), and passing the concrete
`*membership.Client` (creates the import the resolver was built to avoid).

### D4 — What happens when the auth→fleet hop fails

**Chosen: 500 with the opaque `errInternal` envelope.**

The tempting alternative is "return `data: []` on any downstream failure" —
the SPA already handles missing names (FR-1.7), so the user experience is
identical. It is rejected because it makes a fleet-service outage
*indistinguishable from a fleet with no members*, which is exactly the class of
bug the `membership.Client.Active` comment
(`auth-service/internal/membership/client.go:40-51`) was written to prevent
after it bit this codebase once already. A 500 is visible in metrics and logs; a
silent empty array is not.

FR-1.7 still holds: the SPA renders id fallbacks whether the query 500s or
resolves empty. Nothing about the UI depends on which.

### D5 — Where role changes and removals get written

**Chosen: extend `membership.Administrator` with transaction-wrapping methods
that record activity in the same tx, injected via an `ActivityRecorder` function
value.**

This copies `invite.Administrator` verbatim (`invite/administrator.go:20-46`,
`invite/resource.go:33-35`): a `WithActivityRecorder` builder method on the
concrete administrator, a `record` field, and a nil-check at the call site so
existing tests that construct the administrator bare keep working.

```go
type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string,
	vehicleID *string, payload map[string]any) error

type Administrator interface {
	Insert(Model) (Model, error)
	Delete(id string) error                                  // kept: used by nothing new
	UpdateRole(m Model, role, actorUserID string) (Model, error)
	Remove(m Model, actorUserID string) error
	CreateFleetWithOwner(db *gorm.DB, fleetName, userID string) (fleet.Model, error)
}
```

`UpdateRole` and `Remove` each open a `db.Transaction`, do the write, then call
`a.record(tx, …)`. `activity.Record` already takes a `tx` and is designed for
exactly this (`activity/administrator.go:36-40`), so FR-5.2 costs nothing extra.

`Remove` takes the whole `Model` rather than an id because the activity payload
needs the target's `user_id` and `role`, and the handler has already fetched it
via `proc.GetMember`. Re-reading inside the transaction would be a second query
for data we hold.

`Delete(id)` stays on the interface: it is part of the existing contract and
removing it is churn unrelated to this task. The DELETE handler switches to
`Remove`.

**Alternative rejected:** record activity from the resource layer after the
write returns. Simpler to write, but a crash between the two leaves a removal
with no record — and per §0.1 the activity row is now the *only* evidence the
membership ever existed. FR-5.2 is not decoration here.

### D6 — Event types and payloads

Three fleet-level types (`vehicleID` nil), matching the `member.invited`
precedent (`activity/model.go:10`):

| Type | Actor | Payload |
|---|---|---|
| `member.role_changed` | the promoting owner | `{target_user_id, from_role, to_role}` |
| `member.removed` | the removing owner | `{target_user_id, role}` |
| `member.left` | the departing member | `{role}` |

`member.removed` vs `member.left` is decided by `actorUserID == targetUserID`,
the same predicate that relaxes the authorization guard. One branch, two
consequences, no chance of them disagreeing.

`from_role`/`to_role` are recorded rather than just the new role so a
`member.role_changed` entry is self-contained; the feed does not have to replay
history to say what changed.

The no-op PATCH (FR-2.7) **does** write an activity entry with
`from_role == to_role`. Suppressing it would mean the handler branches on
"did anything actually change", and a role-change audit that silently omits
some role-change requests is worse than one with a redundant row.

### D7 — Two-call "Transfer & leave" vs. an atomic endpoint

The PRD specifies PATCH-then-DELETE from the client (ux-flow §Request
sequences). The alternative is a server-side compound —
`POST /fleets/{id}/ownership-transfer {successorId, leave: true}` — which makes
the whole thing atomic.

**Chosen: the two-call flow**, per the PRD, and it holds up:

- The intermediate state (two owners, actor still a member) is **a valid state
  of the system**, not a corruption. FR-2.5 permits any number of owners.
- It is **self-correcting from the UI**: reopening the dialog lands in ux-flow
  state 2 (owner, `ownerCount ≥ 2`), which is the plain leave with no picker.
  The user retries the DELETE and is done.
- The atomic endpoint would need its own copy of the same-fleet check, the owner
  check, the role allow-list, the zero-owner guard, and the removal guard — a
  second authorization path over the same invariant. That is where the real risk
  lives, not in the sequencing.

The one thing the two-call flow must not do is attempt the DELETE after a failed
PATCH (FR-3.8). That is enforced by sequencing them inside a single mutation
function with `await`, not by two independent mutations wired to each other's
`onSuccess`.

### D8 — Successor picker includes viewers

Open question 4, resolved: **the picker offers every active member other than
the leaver, regardless of role**, with the role shown as secondary text so the
choice is informed.

Excluding viewers would create a second dead end that the PRD did not budget
for: an owner whose only companion is a viewer would see an enabled Leave
button, an empty picker, and a permanently disabled confirm — strictly worse
than ux-flow state 4, which at least explains itself. FR-2.3 already permits
`viewer → owner`, so there is no invariant to protect.

### D9 — The stale-claim lag after a promotion

`role` is a JWT claim minted at login/refresh (`session/processor.go`). When
owner A promotes member B:

- **A** is unaffected (FR-4.4) — no refresh.
- **B**'s token still says `member` until B's next mint. Until then, B's own
  requests hit `authz.RequireOwner(identity)` — the token fast-path — and get
  403 even though the database says owner.

This is a **false negative, not a false positive**: it fails closed. It is the
mirror image of the case `RequireOwnerInFleet` exists to catch (a stale `owner`
claim the DB no longer backs, SEC-5), and it is bounded by the access-token
lifetime.

**Chosen: accept it, and do not drop the token fast-path.** The alternative —
removing `RequireOwner` and relying on the DB check alone — would make every
owner-only request pay a query before rejecting an obviously-unauthorized
caller, and would diverge from the guard order every other mutation in
fleet-service uses. B simply gains owner powers on their next refresh. The UI
shows nothing misleading in the meantime, because it renders owner actions off
the same stale claim.

### D10 — `alert-dialog` ownership

Open question 2, resolved by inspection of every worktree:

| Worktree | `components/ui/` contents |
|---|---|
| `task-011-platform-admin-console` | button, card, form, input, label, select, skeleton, switch, textarea |
| `task-012-vehicle-detail-redesign` | …plus `command.tsx`, `popover.tsx` |
| `task-013-media-card-image-variant` | same as base |
| this worktree | same as base |

Nobody is adding `alert-dialog`. `apps/web/package.json` carries
`@radix-ui/react-label`, `-react-select`, `-react-slot`, `-react-switch`; task-012
adds `-react-popover` and `cmdk`. **This task adds
`@radix-ui/react-alert-dialog` and `components/ui/alert-dialog.tsx`** with no
expected conflict beyond a `package.json` merge.

`alert-dialog` rather than plain `dialog`: every dialog in this task is a
confirm/cancel decision on a destructive or privilege-granting action, which is
precisely Radix's alert-dialog semantics (focus pinned to the cancel action,
no dismiss-on-outside-click, `role="alertdialog"`).

### D11 — `ListByIDs` shared with task-011

Open question 1, resolved: **keep the routes separate, share the provider
method.** task-011's `GET /internal/admin/users?ids=` is network-restricted and
platform-scoped; this task's `GET /auth/users?ids=` is JWT-protected and
fleet-scoped. Different trust boundaries, different scoping logic, same
underlying query.

`user.Provider.ListByIDs([]string) ([]Model, error)` is the shared unit. Whoever
merges second deletes their duplicate and keeps the other's — the method is a
plain `WHERE id IN (?)` with no scoping in it, so the two callers cannot
disagree about what it means. The `Attributes` shape (`user/rest.go:7-12`) is
already shared and needs no change.

---

## 3. auth-service: `GET /auth/users`

### 3.1 Request handling

Registered in the JWT group beside `/auth/me` (`auth-service/cmd/main.go:88-92`),
so SEC-2 is satisfied by placement rather than by a check that could be
forgotten.

```
GET /auth/users?ids=<uuid>,<uuid>,…
```

Validation, in order:

1. `ids` absent or empty after trimming → 422, `server.Detailed(server.ErrValidation, "ids is required")`.
2. More than 100 comma-separated entries → 422, naming the cap (SEC-3).
3. Duplicate ids are de-duplicated before the cap is applied and before the
   query, so `?ids=a,a,a,…` cannot inflate the work done.

Following the `errThemeValidation` precedent (`user/resource.go:24-29`), both
422 messages are **compile-time constants**. No caller-supplied string reaches
the response.

### 3.2 Scoping

```go
identity := auth.IdentityFromContext(req.Context())
if identity.ActiveFleetID == "" {
	server.WriteJSON(w, http.StatusOK, server.Document{Data: []server.Resource{}})   // FR-1.3
	return
}
memberIDs, err := members(req.Context(), identity.ActiveFleetID)
if err != nil { /* log, then errInternal → 500  (D4) */ }

allowed := intersect(requested, memberIDs)          // FR-1.2
users, err := proc.ListByIDs(allowed)               // FR-1.4: missing rows just don't come back
server.WriteJSON(w, http.StatusOK, server.Document{Data: user.TransformSlice(users)})
```

`TransformSlice` already returns `make([]server.Resource, 0, …)`, so an empty
result marshals as `[]` and never as `null`.

Note that `Attributes` carries `themePreference` alongside the three fields the
PRD lists. It is another member's UI preference — not sensitive, but not the
caller's business either. **The endpoint reuses `user.Transform` as-is**;
introducing a second transform to strip one cosmetic field would fork the
"keep `rest.go` and `types/models/user.ts` in step" contract that file's comment
establishes, for no security gain. Flagged here so the reviewer sees it was a
decision.

### 3.3 `membership.Client.FleetMemberIDs`

```go
// FleetMemberIDs returns the user ids of a fleet's active members, via
// fleet-service's existing internal endpoint. No new fleet-service route.
func (c *Client) FleetMemberIDs(ctx context.Context, fleetID string) ([]string, error)
```

Against `GET {base}/internal/fleets/{fleetID}/members`, decoding
`[]{user_id, role}` (`fleet-service/internal/membership/rest.go:44-48`) and
projecting `user_id`.

Three things the existing `Active` method gets away with that this one must not:

- **Path escaping.** `Active` concatenates `userID` straight into a query string.
  `fleetID` here comes from a validated JWT claim, so it is not attacker-shaped,
  but `url.PathEscape` costs nothing and stops the next caller from inheriting
  the habit.
- **Non-2xx handling.** Reuse `Active`'s discipline exactly (`client.go:40-52`):
  every non-2xx is an error carrying **status code only**, no body, no ids. A
  decoded-zero-value-with-nil-error is the specific failure that comment
  documents, and an empty member list would silently blank every name.
- **404 is an error here, not a sentinel.** `Active` maps 404 to a zero value
  because "user has no fleet" is a real state. For a fleet id lifted from a
  valid token, 404 means something is wrong; it must not become "this fleet has
  no members".

The shared `http.DefaultClient` has no timeout. `FleetMemberIDs` derives a
bounded context (`context.WithTimeout`, 5 s) from the request context so a
wedged fleet-service cannot pin an auth-service handler open. Left as a local
fix rather than a change to the shared client, to keep the blast radius inside
this task.

---

## 4. fleet-service: PATCH and the relaxed DELETE

### 4.1 Guard order

Both handlers share a shape. The single new idea is `isSelf`, computed once and
used for both authorization and event-type selection.

**PATCH `/fleets/{id}/members/{userId}`**

```
1. RequireSameFleet(identity, fleetID)                → 404
2. RequireOwner(identity)                             → 403   (token fast path)
3. proc.RequireOwnerInFleet(fleetID, identity.UserID) → 403   (DB authoritative, SEC-5)
4. proc.ValidateRoleChange(fleetID, targetUserID, role)
      ├ role ∉ {owner,member,viewer}                  → 422 (Detailed, allow-list named)
      ├ target missing / not active                   → 404
      └ target is sole owner && role != owner         → 409
5. adm.UpdateRole(target, role, identity.UserID)      → 200 + updated resource
```

**DELETE `/fleets/{id}/members/{userId}`** — step 2 becomes conditional:

```
1. RequireSameFleet(identity, fleetID)                → 404
2. isSelf := identity.UserID == targetUserID
   if !isSelf:
       RequireOwner(identity)                         → 403
       proc.RequireOwnerInFleet(…)                    → 403
   if isSelf: no role requirement                     (FR-3.1)
3. proc.GetMember(fleetID, targetUserID)              → 404
4. proc.ValidateRemoval(…)                            → 409  (unchanged)
5. adm.Remove(target, identity.UserID)                → 204
```

`RequireSameFleet` stays **outside** the `isSelf` branch. It is what makes
cross-fleet ids 404 instead of leaking existence, and self-ness must not be able
to bypass it. SEC-4 is then a property of the shape: the relaxed branch is
reachable only when the actor is deleting the row that names them.

Ordering note: the same-fleet check precedes the self check, so a request for
`DELETE /fleets/{someone-elses-fleet}/members/{me}` 404s rather than deleting
anything. `identity.UserID == targetUserID` is necessary but not sufficient.

### 4.2 `ValidateRoleChange`

Lives on `Processor` beside `ValidateRemoval`, which it deliberately mirrors:

```go
// ValidateRoleChange enforces FR-2.3, FR-2.4 and FR-2.6. It returns the target
// membership so the caller does not re-read it.
func (pr *Processor) ValidateRoleChange(fleetID, targetUserID, role string) (Model, error) {
	if !IsValidRole(role) {                                   // builder.go:12 — membership owns the vocabulary
		return Model{}, ErrInvalidRole
	}
	m, err := pr.p.GetByFleetAndUser(fleetID, targetUserID)
	if errors.Is(err, ErrNotFound) || (err == nil && m.Status() != "active") {
		return Model{}, server.ErrNotFound                    // FR-2.4, §0.2
	}
	if err != nil {
		return Model{}, err
	}
	if m.Role() == "owner" && role != "owner" {               // FR-2.6
		n, err := pr.p.CountOwners(fleetID)
		if err != nil {
			return Model{}, err
		}
		if n <= 1 {
			return Model{}, server.ErrConflict
		}
	}
	return m, nil
}
```

`IsValidRole` already exists and is documented as the single owner of the role
vocabulary (`builder.go:8-12`). Re-listing the roles in the processor would be
the third copy.

`ErrInvalidRole` is a **domain** sentinel; the resource layer renders it as
`server.Detailed(server.ErrValidation, "role must be one of owner, member,
viewer")`. This is the domain-error / transport-envelope pair the existing
`ErrInvalidTheme` ↔ `errThemeValidation` comment spells out
(`auth-service/internal/user/resource.go:26-27`) — the processor does not know
about HTTP, and the message is a constant.

FR-2.7 (no-op PATCH) needs no code: `m.Role() == role` passes every check above
and `UpdateRole` writes the same value back, returning 200 with the unchanged
resource.

### 4.3 Model and administrator

```go
// model.go — immutable transition, matching user.Model.WithLogin.
func (m Model) WithRole(role string) Model { m.role = role; return m }
```

Value receiver + value return: the copy *is* the new instance, which is what
makes it immutable without a builder.

```go
// administrator.go
func (a *dbAdministrator) UpdateRole(m Model, role, actorUserID string) (Model, error) {
	updated := m.WithRole(role)
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Entity{}).Where("id = ?", m.ID()).
			Update("role", role).Error; err != nil {
			return err
		}
		if a.record == nil {
			return nil
		}
		return a.record(tx, actorUserID, "member.role_changed", m.FleetID(), nil, map[string]any{
			"target_user_id": m.UserID(),
			"from_role":      m.Role(),
			"to_role":        role,
		})
	})
	if err != nil {
		return Model{}, err
	}
	return updated, nil
}
```

`Update("role", …)` rather than `Save(&entity)`: a full-row save would rewrite
`created_at` and `status` from a model that was read outside the transaction.
Narrow update, narrow window.

`Remove` is the same shape — `tx.Delete(&Entity{}, "id = ?", …)` then
`a.record(tx, actor, eventType, …)` where `eventType` is `member.left` when
`actorUserID == m.UserID()` and `member.removed` otherwise.

### 4.4 Wiring

`membership.InitializeRoutes(log, db)` gains the recorder, matching
`invite.InitializeRoutes`:

```go
membership.InitializeRoutes(log, db, activity.Record)(pr)
```

The nil-recorder guard keeps `processor_test.go` and any bare
`NewAdministrator(db)` construction working unchanged.

---

## 5. Web

### 5.1 Data flow

```
MemberList(fleetId, isOwner)
  ├─ useMembers(fleetId)              → Membership[]        (existing, unchanged)
  ├─ useUsers(memberUserIds)          → Record<id, UserAttributes>   NEW
  └─ useAuth()                        → user.id, role
        │
        ├─ derive: ownerCount, memberCount, isSelf, myRole
        └─ render rows + three dialogs
```

`useUsers` is a *second, independent* query rather than a `select` over the
first. That is what buys FR-1.7: `useMembers` succeeding and `useUsers` failing
is a normal, renderable state.

```ts
export const userKeys = {
  all: ['users'] as const,
  byIds: (ids: string[]) => [...userKeys.all, 'byIds', [...ids].sort().join(',')] as const,
};

export function useUsers(ids: string[]) {
  const sorted = useMemo(() => [...new Set(ids)].sort(), [ids]);
  return useQuery({
    queryKey: userKeys.byIds(sorted),
    queryFn: () => userService.listByIds(sorted),
    enabled: sorted.length > 0,
    staleTime: 60 * 1000,          // matches useMembers
    gcTime: 5 * 60 * 1000,
    select: (result) => Object.fromEntries(result.data.map((r) => [r.id, r.attributes])),
  });
}
```

Sorting + de-duping inside the hook makes the key stable regardless of the
order `useMembers` returns rows in — otherwise a reordered list refetches.

The 100-id cap is a server contract, not a client concern at this scale: a
household fleet is single-digit. `UserService.listByIds` does **not** chunk. If
the activity feed later needs more, chunking belongs there, where the id set is
actually unbounded — building it now would be speculative and untested.

Name resolution, once:

```ts
function displayFor(userId: string, users?: Record<string, UserAttributes>): string {
  const u = users?.[userId];
  return u?.displayName || u?.email || userId.slice(0, 8);   // FR-1.5
}
```

`||` not `??` — auth-service sends `""` for an unset `displayName`
(`Attributes` are plain Go strings), and `??` would let the empty string through
and render a blank row. This is the same `''`-vs-`null` trap
`absentAsNull`/`nullIfEmpty` were written for.

### 5.2 Row state derivation

Directly from ux-flow's matrix, computed once above the map:

```ts
const activeMembers = members ?? [];
const ownerCount    = activeMembers.filter((m) => m.attributes.role === 'owner').length;
const memberCount   = activeMembers.length;
const myRole        = activeMembers.find((m) => m.attributes.userId === user?.id)?.attributes.role;

const soleOwner     = myRole === 'owner' && ownerCount === 1;
const leaveBlocked  = soleOwner && memberCount === 1;                // state 4
const needsSuccessor= soleOwner && memberCount > 1;                  // state 3
```

`myRole` comes from the **members list**, not from `useAuth().role`. The list is
the database; the auth role is a token claim that can be stale (D9). The leave
flow's correctness depends on `ownerCount` and `myRole` agreeing, and they only
agree if both come from the same response.

`isOwner` (the prop, token-derived) keeps gating *visibility* of Make owner /
Remove, because that is what it already does and the server re-checks anyway.

### 5.3 Dialogs

One `alert-dialog` primitive, three usages, each driven by a single piece of
state so two dialogs can never be open at once:

```ts
type PendingAction =
  | { kind: 'remove';   userId: string; name: string }
  | { kind: 'promote';  userId: string; name: string }
  | { kind: 'leave' }
  | null;
```

Copy is transcribed verbatim from ux-flow §Dialog copy. State 4 renders no
dialog — a disabled button plus inline `text-sm text-muted-foreground`
explanation, per ux-flow.

The successor `<Select>` uses the existing `components/ui/select.tsx`
(`@radix-ui/react-select`, already a dependency). Options are
`activeMembers.filter(m => m.attributes.userId !== user?.id)`, labelled
`displayFor(...)` with the role as secondary text (D8). Confirm is disabled
while `!successorId` and while the mutation is pending.

### 5.4 Mutations

**`useUpdateMemberRole(fleetId)`** — `memberService.updateRole(fleetId, userId,
role)` → `PATCH` with `{data:{type:'memberships', attributes:{role}}}`.
Invalidates `memberKeys.lists()` and `fleetKeys.all` on settle. No token refresh
(FR-4.4). 409 → a specific toast about the sole-owner guard; everything else →
`apiError.message`.

**`useRemoveMember(fleetId)`** — extended, not replaced. The mutation variable
becomes an object so the hook can tell a self-leave from a removal without
re-deriving it:

```ts
useMutation({
  mutationFn: async ({ userId, isSelf }: { userId: string; isSelf: boolean }) => {
    await memberService.removeMember(fleetId, userId);
    if (isSelf) await mintAccessToken();      // FR-4.1
    return { isSelf };
  },
  onSuccess: async ({ isSelf }) => {
    if (isSelf) await queryClient.invalidateQueries({ queryKey: authKeys.all });
  },
  onSettled: () => { /* memberKeys.lists(), fleetKeys.all, userKeys.all — FR-4.3 */ },
});
```

`mintAccessToken`, not `refreshAccessToken`: the removal already committed
server-side, so a transient mint failure must not clear a still-valid token and
log the user out of a session they are mid-way through leaving. This is exactly
the reasoning `useAcceptInvite` records (`invites.ts:130-134`), and the mock in
`members.test.ts:72` already exists.

Navigation to `/onboarding` is **not** done by the hook. Invalidating
`authKeys.all` refetches `/auth/me`, which now returns `activeFleetId: null`,
and `RequireAuth` (`RequireAuth.tsx:50`) redirects on its own. A manual
`navigate('/onboarding')` would be a second, racing source of truth for the same
decision.

**Transfer & leave** is one function, not two chained mutations:

```ts
const onTransferAndLeave = async () => {
  await updateRole.mutateAsync({ userId: successorId, role: 'owner' });  // throws → stop
  await removeMember.mutateAsync({ userId: user.id, isSelf: true });     // FR-3.8
};
```

`mutateAsync` + `await` is what makes "if the promote fails, the delete is not
attempted" a property of control flow rather than of a callback wiring nobody
re-reads. The partial-failure state is left alone deliberately (D7): the dialog
reopens in state 2 and the user retries.

### 5.5 Files

| File | Change |
|---|---|
| `components/ui/alert-dialog.tsx` | new — shadcn wrapper over `@radix-ui/react-alert-dialog` |
| `services/api/UserService.ts` | new — `listByIds(ids)` → `GET /api/auth/users?ids=…` |
| `types/models/user.ts` | unchanged — `UserAttributes` already matches |
| `lib/hooks/api/users.ts` | new — `userKeys`, `useUsers` |
| `lib/hooks/api/members.ts` | `useUpdateMemberRole`; `useRemoveMember` takes `{userId, isSelf}` |
| `services/api/MemberService.ts` | `updateRole(fleetId, userId, role)` |
| `components/features/settings/MemberList.tsx` | names, "(you)", three actions, three dialogs |
| `components/features/settings/MemberList.test.tsx` | new |
| `apps/web/package.json` | `@radix-ui/react-alert-dialog` |

`MemberService.updateRole` is written explicitly rather than via
`BaseService.patch`, because `basePath` is the placeholder
`/api/fleet/memberships` that the class already documents as "not used
directly" — the real routes are all nested under a fleet.

---

## 6. What this design does not do

- **No soft delete for memberships** (§0.1). Departures are hard deletes; the
  activity log is the record.
- **No change to `GET /fleets/{id}/members`** (§0.2). Same shape, same rows.
- **No new fleet-service internal route.** `GET /internal/fleets/{id}/members`
  already exists and is consumed unchanged.
- **No activity-feed name resolution.** Open question 3: the feed keeps its
  current rendering this task; `useUsers` is built so the feed can adopt it
  later, and when it does, unresolvable actors (departed members) fall back to
  the 8-character id exactly as FR-1.5 specifies. Out of scope here because no
  requirement in §4 covers the feed and the PRD's §7 web list does not mention
  it.
- **No notification/outbox events** for removals or promotions. The PRD does not
  ask for them and notification-service is listed as unaffected.
- **No deployment manifest change.** The route rides the existing `/api/auth`
  IngressRoute; `FLEET_SERVICE_URL` is already configured.

---

## 7. Test plan

### auth-service

| Test | Asserts |
|---|---|
| `users_scoped_to_active_fleet` | caller in fleet A requesting a fleet-B id gets 200 with `data: []` — **not** 403, **not** 404 (SEC-1) |
| `users_omits_unknown_ids` | a fleet member with no `users` row is omitted, 200 (FR-1.4) |
| `users_no_active_fleet` | empty `ActiveFleetID` → 200 `data: []` (FR-1.3) |
| `users_missing_ids` / `users_empty_ids` | 422 (§5.1) |
| `users_over_cap` | 101 ids → 422 (SEC-3) |
| `users_requires_jwt` | route is inside the JWT group → 401 (SEC-2) |
| `users_fleet_lookup_failure` | gatherer returns error → 500, not empty 200 (D4) |
| `client_FleetMemberIDs_non_2xx` | 500 from fleet-service → error, not empty slice |
| `client_FleetMemberIDs_404` | error, not empty slice |

The scoping tests inject a stub `FleetMemberGatherer`, so they are fast unit
tests over the handler rather than integration tests over two services — the
injection in D3 is what makes that possible.

### fleet-service

| Test | Asserts |
|---|---|
| `patch_promotes_member` | 200; both memberships read `owner` (FR-2.5) |
| `patch_non_owner_forbidden` | member and viewer tokens → 403 |
| `patch_stale_owner_claim` | token says owner, DB says member → 403 (SEC-5) |
| `patch_invalid_role` | `"admin"` → 422 with the allow-list in `detail` (FR-2.3) |
| `patch_target_not_member` | 404 (FR-2.4) |
| `patch_demote_sole_owner` | 409 (FR-2.6) |
| `patch_noop` | same role → 200, unchanged resource (FR-2.7) |
| `patch_cross_fleet` | 404 before any role check |
| `delete_self_as_member` | 204 (FR-3.1) |
| `delete_self_as_viewer` | 204 |
| `delete_other_as_member` | 403 (SEC-4) |
| `delete_other_as_viewer` | 403 (SEC-4) |
| `delete_self_sole_owner` | 409, unchanged guard (FR-3.2) |
| `delete_other_owner_as_owner` | 204 (FR-3.3) |
| `delete_self_cross_fleet` | 404 — self-ness does not bypass `RequireSameFleet` |
| `activity_written_in_tx` | role change → one `member.role_changed`; removal → `member.removed`; self-leave → `member.left`; a forced recorder error rolls back the membership write (FR-5.2) |

The rollback test is the one that actually proves FR-5.2: a stub recorder that
returns an error, followed by asserting the membership row is still there.

### web

| Test | Asserts |
|---|---|
| name fallback chain | displayName → email → 8-char id (FR-1.5) |
| `useUsers` failure | list still renders with id fallbacks (FR-1.7) |
| "(you)" marker | own row only (FR-1.6) |
| remove confirmation | dialog names the member; DELETE fires only after confirm; cancel fires nothing |
| leave states 1/2 | plain dialog, no picker (FR-3.6, FR-3.9) |
| leave state 3 | picker present; confirm disabled until a successor is chosen (FR-3.7) |
| transfer & leave | PATCH then DELETE, in that order |
| transfer & leave, PATCH fails | **no DELETE is issued** (FR-3.8) |
| leave state 4 | button disabled + inline explanation, no dialog (FR-3.10) |
| self-leave refresh | `mintAccessToken` called and `authKeys` invalidated; not called when removing someone else (FR-4.1, FR-4.4) |
| Make owner visibility | rendered for owners on non-owner rows; absent for non-owners (FR-2.8) |

### Build

`make ci` — lint-check, vet, test, build, fe-test, fe-build. No manifest change,
so no `kustomize` re-render is required for this task.

---

## 8. Risks

| Risk | Mitigation |
|---|---|
| task-011 merges a conflicting `user.Provider.ListByIDs` | D11: identical signature, no scoping inside; second merger deletes their copy |
| task-012's `package.json` radix additions conflict | Trivial merge; different packages |
| `user.InitializeRoutes` signature change breaks a caller | One call site (`auth-service/cmd/main.go:91`) |
| Promoted user confused by unchanged permissions until refresh | D9: fails closed, bounded by token TTL; accepted |
| Transfer & leave leaves two owners on partial failure | D7: a valid state; the reopened dialog is state 2 and the retry completes it |
