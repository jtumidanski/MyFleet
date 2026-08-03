# UX Flow — Member List Actions

Supporting doc for `prd.md`. The Leave action has four distinct states depending
on the actor's role and the fleet's owner count; this table is the authoritative
enumeration.

## Leave action state matrix

Let `myRole` be the authenticated user's role, `ownerCount` the number of active
owners in the fleet, and `memberCount` the number of active members.

| # | myRole | ownerCount | memberCount | Leave button | Dialog |
|---|---|---|---|---|---|
| 1 | member / viewer | any | any | Enabled | Plain leave confirmation |
| 2 | owner | ≥ 2 | any | Enabled | Plain leave confirmation (FR-3.9) |
| 3 | owner | 1 | ≥ 2 | Enabled | Leave **with successor picker** (FR-3.7) |
| 4 | owner | 1 | 1 | **Disabled** | None — inline explanation (FR-3.10) |

Row 4 is the only case with no path forward. The fleet has exactly one person
and they own it; there is nobody to transfer to and deleting the fleet is out of
scope for this task.

## Row layout

```
Members
──────────────────────────────────────────────────────────
Jane Doe  (you)                                   [Leave]
owner

Sam Ito                            [Make owner]  [Remove]
member

sam2@example.com                   [Make owner]  [Remove]
viewer

a1b2c3d4                           [Make owner]  [Remove]
member
──────────────────────────────────────────────────────────
```

Rows two through four show the FR-1.5 fallback chain in action: display name,
then email, then the shortened id.

`Make owner` and `Remove` render only when the authenticated user is an owner.
`Leave` renders on the authenticated user's own row for every role.

## Dialog copy

**Remove another member** (FR-3.5)

> **Remove {name} from this fleet?**
> They will immediately lose access to all of this fleet's vehicles, maintenance
> records, and photos. You can invite them back later.
>
> `[Cancel]` `[Remove]` ← destructive

**Leave — states 1 and 2** (FR-3.6)

> **Leave this fleet?**
> You will lose access to all of this fleet's vehicles, maintenance records, and
> photos. Rejoining requires a new invite from an owner.
>
> `[Cancel]` `[Leave]` ← destructive

**Leave — state 3, sole owner** (FR-3.7)

> **Leave this fleet?**
> You are the only owner. Choose who takes over before you go.
>
> New owner: `[ Select a member ▾ ]`
>
> You will lose access to all of this fleet's vehicles, maintenance records, and
> photos. Rejoining requires a new invite.
>
> `[Cancel]` `[Transfer & leave]` ← destructive, disabled until a successor is chosen

**Make owner** (FR-2.8)

> **Make {name} an owner?**
> They will be able to invite and remove members, rename the fleet, and grant
> ownership to others. You remain an owner.
>
> `[Cancel]` `[Make owner]`

**Leave — state 4, disabled**

Inline beneath the disabled button, not a dialog:

> You are the only member of this fleet, so there is nobody to hand it to.

## Request sequences

**Plain leave** (states 1, 2)

```
DELETE /api/fleet/fleets/{fleetId}/members/{myUserId}   → 204
POST   /api/auth/refresh                                → new token, empty active_fleet_id
                                                        → route to /onboarding
```

**Transfer & leave** (state 3) — FR-3.8

```
PATCH  /api/fleet/fleets/{fleetId}/members/{successorId}
       { data: { type: "memberships", attributes: { role: "owner" } } }   → 200
DELETE /api/fleet/fleets/{fleetId}/members/{myUserId}                     → 204
POST   /api/auth/refresh                                                  → route to /onboarding
```

If the PATCH fails, stop — do not attempt the DELETE. If the PATCH succeeds and
the DELETE fails, the fleet now has two owners and the user is still a member.
Reopening the dialog then lands in state 2 (plain leave, no picker), which is
correct and lets the user retry.

**Make owner** (standalone)

```
PATCH  /api/fleet/fleets/{fleetId}/members/{targetId}   → 200
```

No session refresh — the actor's own claims are unchanged (FR-4.4).
