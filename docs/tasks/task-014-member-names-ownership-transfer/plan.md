# Member Names & Ownership Transfer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show fleet members by name, put every membership removal behind a confirmation dialog, let any member leave, and make fleet ownership transferable — without ever leaving a fleet with zero owners.

**Architecture:** Two independent server changes joined only in the SPA. auth-service gains a JWT-protected batch user lookup (`GET /auth/users?ids=`) scoped to the caller's active fleet by calling fleet-service's *existing* internal members endpoint. fleet-service gains `PATCH /fleets/{id}/members/{userId}` and relaxes its DELETE so a member can remove themselves; both writes record an activity event inside the same database transaction. The web member list fetches memberships and names as two independent queries and zips them, so a name-lookup failure degrades to id fallbacks instead of blanking the card.

**Tech Stack:** Go 1.x (chi, GORM, logrus), Postgres (SQLite in-memory for tests), React 18 + TypeScript, TanStack React Query v5, Radix UI / shadcn, Vitest + Testing Library.

## Global Constraints

- **Role vocabulary is owned by `membership.IsValidRole`** (`apps/fleet-service/internal/membership/builder.go:12`). Never re-list `owner|member|viewer` in a new validation branch — call `IsValidRole`.
- **Error messages that reach a response are compile-time constants.** No caller-supplied string, user id, or upstream response body may be interpolated into an error that renders into the JSON:API envelope or a log message. Precedent: `errThemeValidation` (`apps/auth-service/internal/user/resource.go:28`) and `membership.Client.Active` (`apps/auth-service/internal/membership/client.go:46-51`).
- **Domain error / transport envelope pair.** Processors return domain sentinels (`ErrInvalidRole`); resource handlers translate them into `server.Detailed`/`fmt.Errorf("%w: …", server.ErrValidation)` envelopes. Processors never know about HTTP.
- **Guard order for every fleet-scoped mutation:** `authz.RequireSameFleet` first (404, so cross-fleet existence never leaks), then the role guard, then the DB-authoritative `RequireOwnerInFleet`, then domain validation.
- **Activity is recorded inside the same `gorm` transaction as the domain write** via an injected `ActivityRecorder` function value, with a `nil` guard at the call site. Precedent: `invite.Administrator` (`apps/fleet-service/internal/invite/administrator.go:20-46`).
- **Memberships are hard-deleted.** `membership.Entity` has no `DeletedAt` and this task does not add one. The `activity_events` row is the only record that a departure happened.
- **`GET /fleets/{id}/members` response shape does not change.** No new attributes on the `memberships` resource.
- **Cross-service DB joins are forbidden (D2).** auth-service reaches fleet-service over HTTP only, and only in the auth→fleet direction that already exists.
- **`||` not `??` for name fallbacks.** Go marshals an unset `displayName` as `""`, and `??` would let the empty string through and render a blank row.
- **`ids` cap is 100**, applied after de-duplication.
- **Node is not always on `PATH`.** Before any `npm` command: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.
- **All work happens in this worktree**: `/home/tumidanski/source/MyFleet/.worktrees/task-014-member-names-ownership-transfer` on branch `task-014-member-names-ownership-transfer`. Never edit the main checkout.
- **No deployment manifest changes.** The new auth route rides the existing `/api/auth` IngressRoute; `FLEET_SERVICE_URL` is already configured. `make ci` still runs `manifests`, so do not break the overlays.

---

## File Structure

**fleet-service** (`apps/fleet-service/`)

| File | Responsibility |
|---|---|
| `internal/membership/model.go` | + `WithRole(role) Model` — immutable role transition |
| `internal/membership/processor.go` | + `ErrInvalidRole` sentinel, + `ValidateRoleChange` (FR-2.3/2.4/2.6) |
| `internal/membership/processor_test.go` | unit tests for `ValidateRoleChange` |
| `internal/membership/administrator.go` | + `ActivityRecorder` type, `WithActivityRecorder`, `UpdateRole`, `Remove`; `NewAdministrator` returns `*dbAdministrator` |
| `internal/membership/administrator_db_test.go` | **new** — in-memory SQLite tests for the two transactional writes + rollback |
| `internal/membership/resource.go` | + PATCH handler; DELETE guard restructured around `isSelf`; `InitializeRoutes` takes an `ActivityRecorder` |
| `internal/membership/resource_test.go` | **new** — HTTP-level tests over the real chi router |
| `cmd/main.go` | pass `activity.Record` into `membership.InitializeRoutes` |

**auth-service** (`apps/auth-service/`)

| File | Responsibility |
|---|---|
| `internal/user/provider.go` | + `ListByIDs(ids []string) ([]Model, error)` |
| `internal/user/provider_test.go` | tests for `ListByIDs` |
| `internal/user/processor.go` | + `ListByIDs` pass-through |
| `internal/user/resource.go` | + `FleetMemberGatherer` type, `GET /auth/users`, `parseUserIDs`, `intersect`; `InitializeRoutes` gains the gatherer parameter |
| `internal/user/users_resource_test.go` | **new** — handler tests with a stub gatherer |
| `internal/user/resource_test.go` | update `newAuthRouter` for the new signature |
| `internal/membership/client.go` | + `FleetMemberIDs(ctx, fleetID) ([]string, error)` |
| `internal/membership/client_test.go` | tests for the new method's failure modes |
| `cmd/main.go` | compose the gatherer from `fleetClient` and pass it in |

**web** (`apps/web/`)

| File | Responsibility |
|---|---|
| `package.json` | + `@radix-ui/react-alert-dialog` |
| `src/components/ui/alert-dialog.tsx` | **new** — shadcn wrapper over the Radix primitive |
| `src/services/api/UserService.ts` | **new** — `listByIds(ids)` |
| `src/lib/hooks/api/users.ts` | **new** — `userKeys`, `useUsers` |
| `src/lib/hooks/api/users.test.ts` | **new** — key stability + shape |
| `src/services/api/MemberService.ts` | + `updateRole(fleetId, userId, role)` |
| `src/lib/hooks/api/members.ts` | + `useUpdateMemberRole`; `useRemoveMember` takes `{ userId, isSelf }` |
| `src/lib/hooks/api/members.test.ts` | update the existing call; add self-leave refresh tests |
| `src/components/features/settings/MemberList.tsx` | names, "(you)", three actions, one dialog with three modes |
| `src/components/features/settings/MemberList.test.tsx` | **new** — the ux-flow state matrix |

---

## Task Order & Dependencies

```
Task 1 ──► Task 2 ──► Task 3        (fleet-service; self-contained)
Task 4 ──► Task 6                    (auth-service)
Task 5 ──► Task 6
Task 7 ──► Task 10                   (web)
Task 8 ──► Task 10
Task 9 ──► Task 10
Task 11 (full make ci) last
```

Tasks 1-3, 4-6 and 7-10 are three independent tracks; within a track the order is fixed.

---

### Task 1: `WithRole` and `ValidateRoleChange`

**Files:**
- Modify: `apps/fleet-service/internal/membership/model.go`
- Modify: `apps/fleet-service/internal/membership/processor.go`
- Test: `apps/fleet-service/internal/membership/processor_test.go`

**Interfaces:**
- Consumes: `IsValidRole(role string) bool` (`builder.go:12`), `Provider.GetByFleetAndUser`, `Provider.CountOwners`, `ErrNotFound` (`provider.go:9`).
- Produces:
  - `func (m Model) WithRole(role string) Model`
  - `var ErrInvalidRole = errors.New("invalid membership role")`
  - `func (pr *Processor) ValidateRoleChange(fleetID, targetUserID, role string) (Model, error)` — returns the target membership on success so the caller need not re-read it. Errors: `ErrInvalidRole`, `server.ErrNotFound`, `server.ErrConflict`.

- [ ] **Step 1: Write the failing tests**

Append to `apps/fleet-service/internal/membership/processor_test.go`:

```go
// activeMember builds the stub row shape ValidateRoleChange expects. Status is
// explicit because ValidateRoleChange requires "active": Status is vestigial
// today (every row is written active and never changed) and the check exists so
// the guard stays correct if a second status value is ever introduced.
func activeMember(role string) Model {
	return Model{id: "m-" + role, fleetID: "f1", userID: "u-" + role, role: role, status: "active"}
}

func TestValidateRoleChange_rejectsUnknownRole(t *testing.T) {
	stub := stubProvider{byFleetAndUser: map[string]Model{"f1:u-member": activeMember("member")}}
	p := NewProcessor(logrus.New(), stub)

	for _, role := range []string{"admin", "", "Owner", "superuser"} {
		if _, err := p.ValidateRoleChange("f1", "u-member", role); !errors.Is(err, ErrInvalidRole) {
			t.Errorf("role %q must be rejected with ErrInvalidRole, got %v", role, err)
		}
	}
}

// The role check runs BEFORE the lookup: an out-of-range value must cost no
// database round trip, mirroring user.Processor.UpdateTheme.
func TestValidateRoleChange_rejectsUnknownRoleWithoutLookup(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{})
	if _, err := p.ValidateRoleChange("f1", "nobody", "admin"); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("want ErrInvalidRole before the lookup, got %v", err)
	}
}

func TestValidateRoleChange_notFoundWhenTargetIsNotAMember(t *testing.T) {
	p := NewProcessor(logrus.New(), stubProvider{})
	if _, err := p.ValidateRoleChange("f1", "stranger", "owner"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("non-member target must be 404, got %v", err)
	}
}

// A non-active membership is not a target. Today no row is ever written with
// another status, so this asserts intent rather than current behaviour.
func TestValidateRoleChange_notFoundWhenTargetIsNotActive(t *testing.T) {
	inactive := Model{id: "m1", fleetID: "f1", userID: "u1", role: "member", status: "revoked"}
	stub := stubProvider{byFleetAndUser: map[string]Model{"f1:u1": inactive}}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u1", "owner"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("inactive target must be 404, got %v", err)
	}
}

// FR-2.6. The fleet must never be left with zero owners.
func TestValidateRoleChange_rejectsDemotingTheSoleOwner(t *testing.T) {
	stub := stubProvider{
		owners:         1,
		byFleetAndUser: map[string]Model{"f1:u-owner": activeMember("owner")},
	}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u-owner", "member"); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("demoting the sole owner must be 409, got %v", err)
	}
}

// FR-2.5: multiple owners are permitted, so demoting one of two is fine.
func TestValidateRoleChange_allowsDemotingOneOfTwoOwners(t *testing.T) {
	stub := stubProvider{
		owners:         2,
		byFleetAndUser: map[string]Model{"f1:u-owner": activeMember("owner")},
	}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u-owner", "member"); err != nil {
		t.Fatalf("demotion with a co-owner must pass, got %v", err)
	}
}

// Promotion never counts owners: adding an owner cannot orphan a fleet.
func TestValidateRoleChange_allowsPromotingWhenThereIsOneOwner(t *testing.T) {
	stub := stubProvider{
		owners:         1,
		byFleetAndUser: map[string]Model{"f1:u-member": activeMember("member")},
	}
	p := NewProcessor(logrus.New(), stub)

	m, err := p.ValidateRoleChange("f1", "u-member", "owner")
	if err != nil {
		t.Fatalf("promotion must pass, got %v", err)
	}
	if m.UserID() != "u-member" || m.Role() != "member" {
		t.Fatalf("must return the target AS IT IS TODAY so the caller can record from_role; got %+v", m)
	}
}

// FR-2.7: a no-op PATCH is a success, not a special case.
func TestValidateRoleChange_allowsSettingTheRoleAlreadyHeld(t *testing.T) {
	stub := stubProvider{
		owners:         1,
		byFleetAndUser: map[string]Model{"f1:u-owner": activeMember("owner")},
	}
	p := NewProcessor(logrus.New(), stub)

	if _, err := p.ValidateRoleChange("f1", "u-owner", "owner"); err != nil {
		t.Fatalf("owner -> owner must be a no-op success, got %v", err)
	}
}

func TestWithRole_returnsANewModelAndLeavesTheOriginalAlone(t *testing.T) {
	original := activeMember("member")
	updated := original.WithRole("owner")

	if updated.Role() != "owner" {
		t.Fatalf("WithRole did not apply the new role: %q", updated.Role())
	}
	if original.Role() != "member" {
		t.Fatalf("WithRole mutated the receiver; Model is immutable. original role = %q", original.Role())
	}
	if updated.ID() != original.ID() || updated.FleetID() != original.FleetID() ||
		updated.UserID() != original.UserID() || updated.Status() != original.Status() {
		t.Fatalf("WithRole changed a field other than role: %+v vs %+v", updated, original)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/membership/...`
Expected: FAIL — `p.ValidateRoleChange undefined`, `original.WithRole undefined`, `ErrInvalidRole undefined`.

- [ ] **Step 3: Add `WithRole`**

Append to `apps/fleet-service/internal/membership/model.go`:

```go
// WithRole returns a copy carrying the new role. Value receiver and value
// return: the copy IS the new instance, which is what makes the transition
// immutable without a builder. Matches user.Model.WithLogin in auth-service.
func (m Model) WithRole(role string) Model {
	m.role = role
	return m
}
```

- [ ] **Step 4: Add `ErrInvalidRole` and `ValidateRoleChange`**

In `apps/fleet-service/internal/membership/processor.go`, add the sentinel just below the imports:

```go
// ErrInvalidRole is the DOMAIN error for a role outside the vocabulary
// IsValidRole owns. The resource layer renders it as the 422 transport
// envelope; the processor knows nothing about HTTP. Same pairing as
// auth-service's user.ErrInvalidTheme / errThemeValidation.
var ErrInvalidRole = errors.New("invalid membership role")
```

Append the validator below `ValidateRemoval`:

```go
// ValidateRoleChange enforces FR-2.3 (role vocabulary), FR-2.4 (target must be
// an active member of this fleet) and FR-2.6 (a fleet is never left with zero
// owners). It returns the target membership AS IT IS TODAY so the caller does
// not re-read it and can record from_role in the activity payload.
//
// The vocabulary check runs before the lookup on purpose: an out-of-range role
// then costs no database round trip. Same ordering as user.Processor.UpdateTheme.
func (pr *Processor) ValidateRoleChange(fleetID, targetUserID, role string) (Model, error) {
	if !IsValidRole(role) {
		return Model{}, ErrInvalidRole
	}
	m, err := pr.p.GetByFleetAndUser(fleetID, targetUserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Model{}, server.ErrNotFound
		}
		return Model{}, err
	}
	// Status is vestigial today — every row is written "active" and never
	// changed — so this is a statement of intent, not a live branch. It costs
	// one comparison and keeps the guard correct if a second value appears.
	if m.Status() != "active" {
		return Model{}, server.ErrNotFound
	}
	// Only a demotion can orphan a fleet. Promotions never count owners.
	if m.Role() == "owner" && role != "owner" {
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

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/membership/... -v -run 'ValidateRoleChange|WithRole'`
Expected: PASS, all eight new tests.

- [ ] **Step 6: Vet and commit**

```bash
go vet ./apps/fleet-service/internal/membership/...
git add apps/fleet-service/internal/membership/model.go \
        apps/fleet-service/internal/membership/processor.go \
        apps/fleet-service/internal/membership/processor_test.go
git commit -m "feat(fleet-service): validate membership role changes"
```

---

### Task 2: Transactional `UpdateRole` and `Remove` with activity recording

**Files:**
- Modify: `apps/fleet-service/internal/membership/administrator.go`
- Test (create): `apps/fleet-service/internal/membership/administrator_db_test.go`

**Interfaces:**
- Consumes: `Model.WithRole` (Task 1), `Entity` / `TableName() == "fleet.fleet_memberships"`, `Make(Entity) Model`.
- Produces:
  - `type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error`
  - `func NewAdministrator(db *gorm.DB) *dbAdministrator` (was: returned the `Administrator` interface)
  - `func (a *dbAdministrator) WithActivityRecorder(rec ActivityRecorder) *dbAdministrator`
  - `Administrator` interface gains `UpdateRole(m Model, role, actorUserID string) (Model, error)` and `Remove(m Model, actorUserID string) error`
  - Event types written: `"member.role_changed"`, `"member.removed"`, `"member.left"`

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/membership/administrator_db_test.go`:

```go
package membership

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newMembershipDB builds the in-memory harness. TableName is schema-qualified
// ("fleet.fleet_memberships") for Postgres; SQLite has no schemas, so attach an
// in-memory database aliased "fleet" so the qualified name resolves. Explicit
// DDL rather than Migration(db): the uniqueIndex tags on FleetID/UserID make
// GORM emit CREATE UNIQUE INDEX with the schema prefix stripped, which cannot
// resolve against an attached schema. Same workaround as
// auth-service/internal/user/provider_test.go and invite/resource_test.go.
func newMembershipDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	// KEEP IN SYNC WITH entity.go.
	if err := db.Exec(`CREATE TABLE fleet.fleet_memberships (
		id TEXT PRIMARY KEY, fleet_id TEXT, user_id TEXT, role TEXT, status TEXT,
		created_at DATETIME, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("create fleet.fleet_memberships: %v", err)
	}
	return db
}

func seedMembership(t *testing.T, db *gorm.DB, userID, role string) Model {
	t.Helper()
	m := NewBuilder().SetFleetID("f1").SetUserID(userID).SetRole(role).Build()
	created, err := NewAdministrator(db).Insert(m)
	if err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return created
}

func readRole(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()
	var role string
	if err := db.Raw("SELECT role FROM fleet.fleet_memberships WHERE id = ?", id).Scan(&role).Error; err != nil {
		t.Fatalf("read role: %v", err)
	}
	return role
}

func countRows(t *testing.T, db *gorm.DB, id string) int {
	t.Helper()
	var n int
	if err := db.Raw("SELECT COUNT(*) FROM fleet.fleet_memberships WHERE id = ?", id).Scan(&n).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// recorded captures one ActivityRecorder invocation.
type recorded struct {
	actorUserID string
	eventType   string
	fleetID     string
	vehicleID   *string
	payload     map[string]any
}

// spyRecorder returns a recorder that appends to calls, plus the slice.
func spyRecorder(calls *[]recorded) ActivityRecorder {
	return func(_ *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error {
		*calls = append(*calls, recorded{actorUserID, eventType, fleetID, vehicleID, payload})
		return nil
	}
}

func TestUpdateRole_writesTheRoleAndRecordsRoleChanged(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	updated, err := adm.UpdateRole(target, "owner", "u-actor")
	if err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if updated.Role() != "owner" {
		t.Fatalf("returned model role = %q, want owner", updated.Role())
	}
	if got := readRole(t, db, target.ID()); got != "owner" {
		t.Fatalf("persisted role = %q, want owner", got)
	}
	if len(calls) != 1 {
		t.Fatalf("want exactly one activity call, got %d", len(calls))
	}
	c := calls[0]
	if c.eventType != "member.role_changed" {
		t.Errorf("eventType = %q, want member.role_changed", c.eventType)
	}
	if c.actorUserID != "u-actor" || c.fleetID != "f1" {
		t.Errorf("actor/fleet = %q/%q, want u-actor/f1", c.actorUserID, c.fleetID)
	}
	if c.vehicleID != nil {
		t.Errorf("member events are fleet-level; vehicleID must be nil, got %v", *c.vehicleID)
	}
	// from_role/to_role are both recorded so the entry is self-contained — the
	// feed must not have to replay history to say what changed.
	if c.payload["target_user_id"] != "u-target" ||
		c.payload["from_role"] != "member" || c.payload["to_role"] != "owner" {
		t.Errorf("payload = %+v, want target_user_id/from_role/to_role", c.payload)
	}
}

// FR-2.7: a no-op PATCH still writes an audit entry. A role-change log that
// silently omits some role-change requests is worse than one with a redundant
// row, and suppressing it would mean branching on "did anything change".
func TestUpdateRole_recordsEvenWhenTheRoleIsUnchanged(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "owner")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if _, err := adm.UpdateRole(target, "owner", "u-actor"); err != nil {
		t.Fatalf("UpdateRole: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("a no-op role change must still be recorded; got %d calls", len(calls))
	}
	if calls[0].payload["from_role"] != "owner" || calls[0].payload["to_role"] != "owner" {
		t.Errorf("payload = %+v, want from_role == to_role == owner", calls[0].payload)
	}
}

// FR-5.2. The activity row is the ONLY evidence a membership changed —
// memberships are hard-deleted with no tombstone — so the write and the record
// must share a transaction. A recorder failure must roll the domain write back.
func TestUpdateRole_rollsBackTheRoleWhenRecordingFails(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	boom := errors.New("recorder exploded")
	adm := NewAdministrator(db).WithActivityRecorder(
		func(*gorm.DB, string, string, string, *string, map[string]any) error { return boom },
	)

	if _, err := adm.UpdateRole(target, "owner", "u-actor"); !errors.Is(err, boom) {
		t.Fatalf("UpdateRole error = %v, want the recorder's error", err)
	}
	if got := readRole(t, db, target.ID()); got != "member" {
		t.Fatalf("role = %q after a failed record; the write must roll back", got)
	}
}

// A bare administrator (no recorder) must keep working — every existing
// construction site does exactly that.
func TestUpdateRole_worksWithoutARecorder(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	if _, err := NewAdministrator(db).UpdateRole(target, "owner", "u-actor"); err != nil {
		t.Fatalf("UpdateRole without a recorder: %v", err)
	}
	if got := readRole(t, db, target.ID()); got != "owner" {
		t.Fatalf("persisted role = %q, want owner", got)
	}
}

// D6: member.removed vs member.left is decided by actor == target, the same
// predicate that relaxes the authorization guard, so the two cannot disagree.
func TestRemove_recordsMemberRemovedWhenAnOwnerRemovesSomeoneElse(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if err := adm.Remove(target, "u-owner"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if countRows(t, db, target.ID()) != 0 {
		t.Fatal("membership row still present; Remove must hard-delete it")
	}
	if len(calls) != 1 || calls[0].eventType != "member.removed" {
		t.Fatalf("want one member.removed call, got %+v", calls)
	}
	if calls[0].actorUserID != "u-owner" {
		t.Errorf("actor = %q, want u-owner", calls[0].actorUserID)
	}
	if calls[0].payload["target_user_id"] != "u-target" || calls[0].payload["role"] != "member" {
		t.Errorf("payload = %+v, want target_user_id and role", calls[0].payload)
	}
}

func TestRemove_recordsMemberLeftWhenTheActorRemovesThemselves(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-self", "viewer")

	var calls []recorded
	adm := NewAdministrator(db).WithActivityRecorder(spyRecorder(&calls))

	if err := adm.Remove(target, "u-self"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(calls) != 1 || calls[0].eventType != "member.left" {
		t.Fatalf("want one member.left call, got %+v", calls)
	}
	if calls[0].payload["role"] != "viewer" {
		t.Errorf("payload = %+v, want role", calls[0].payload)
	}
}

func TestRemove_rollsBackTheDeleteWhenRecordingFails(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	boom := errors.New("recorder exploded")
	adm := NewAdministrator(db).WithActivityRecorder(
		func(*gorm.DB, string, string, string, *string, map[string]any) error { return boom },
	)

	if err := adm.Remove(target, "u-owner"); !errors.Is(err, boom) {
		t.Fatalf("Remove error = %v, want the recorder's error", err)
	}
	if countRows(t, db, target.ID()) != 1 {
		t.Fatal("membership row gone after a failed record; the delete must roll back")
	}
}

func TestRemove_worksWithoutARecorder(t *testing.T) {
	db := newMembershipDB(t)
	target := seedMembership(t, db, "u-target", "member")

	if err := NewAdministrator(db).Remove(target, "u-owner"); err != nil {
		t.Fatalf("Remove without a recorder: %v", err)
	}
	if countRows(t, db, target.ID()) != 0 {
		t.Fatal("membership row still present")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/membership/...`
Expected: FAIL — `ActivityRecorder undefined`, `NewAdministrator(db).WithActivityRecorder undefined`, `UpdateRole`/`Remove` undefined.

- [ ] **Step 3: Extend the administrator**

Replace the top of `apps/fleet-service/internal/membership/administrator.go` (imports through `NewAdministrator`) with:

```go
package membership

import (
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

// Administrator is the write interface for membership data access.
// It also owns the cross-domain fleet onboarding transaction.
type Administrator interface {
	Insert(Model) (Model, error)
	// Delete removes a membership by id WITHOUT recording activity. Retained
	// because it is part of the existing contract; new call sites use Remove,
	// which is transactional and auditable.
	Delete(id string) error
	// UpdateRole writes a new role and appends member.role_changed in the SAME
	// transaction (FR-5.2).
	UpdateRole(m Model, role, actorUserID string) (Model, error)
	// Remove hard-deletes a membership and appends member.removed (or
	// member.left when the actor is the target) in the SAME transaction.
	Remove(m Model, actorUserID string) error
	// CreateFleetWithOwner creates a fleet + owner membership in one transaction.
	// Implements fleet.OnboardingAdmin.
	CreateFleetWithOwner(db *gorm.DB, fleetName, userID string) (fleet.Model, error)
}

// ActivityRecorder appends an activity event on the supplied tx (design §8.2).
// Injected as a function value so the membership package never imports the
// activity package. Satisfied by activity.Record.
type ActivityRecorder func(tx *gorm.DB, actorUserID, eventType, fleetID string, vehicleID *string, payload map[string]any) error

type dbAdministrator struct {
	db     *gorm.DB
	record ActivityRecorder
}

// NewAdministrator returns an Administrator backed by the given database.
// It returns the concrete type so WithActivityRecorder can be chained, matching
// invite.NewAdministrator.
func NewAdministrator(db *gorm.DB) *dbAdministrator { return &dbAdministrator{db: db} }

// WithActivityRecorder injects the recorder run inside UpdateRole and Remove.
// Leaving it nil is supported: tests and the onboarding path construct the
// administrator bare, and the call sites nil-check before recording.
func (a *dbAdministrator) WithActivityRecorder(rec ActivityRecorder) *dbAdministrator {
	a.record = rec
	return a
}
```

Then append the two new methods after `Delete`:

```go
// UpdateRole persists the new role and appends member.role_changed in the same
// transaction (FR-5.2).
//
// Update("role", …) rather than Save(&entity): a full-row save would rewrite
// created_at and status from a model that was read OUTSIDE this transaction.
// Narrow update, narrow window.
func (a *dbAdministrator) UpdateRole(m Model, role, actorUserID string) (Model, error) {
	updated := m.WithRole(role)
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Entity{}).Where("id = ?", m.ID()).Update("role", role).Error; err != nil {
			return err
		}
		if a.record == nil {
			return nil
		}
		// from_role comes off the pre-change model, so the entry is
		// self-contained: the feed never has to replay history.
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

// Remove hard-deletes the membership and appends the departure event in the
// same transaction.
//
// Memberships carry no deleted_at (see entity.go), so this activity row is the
// ONLY record that the membership ever existed. That is why the append is
// transactional rather than a best-effort follow-up write.
//
// The actor == target predicate that picks member.left over member.removed is
// the same one that relaxes the DELETE authorization guard, so the audit trail
// and the authorization decision cannot disagree.
func (a *dbAdministrator) Remove(m Model, actorUserID string) error {
	return a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&Entity{}, "id = ?", m.ID()).Error; err != nil {
			return err
		}
		if a.record == nil {
			return nil
		}
		if actorUserID == m.UserID() {
			return a.record(tx, actorUserID, "member.left", m.FleetID(), nil, map[string]any{
				"role": m.Role(),
			})
		}
		return a.record(tx, actorUserID, "member.removed", m.FleetID(), nil, map[string]any{
			"target_user_id": m.UserID(),
			"role":           m.Role(),
		})
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/internal/membership/... -v`
Expected: PASS — all nine new administrator tests plus the Task 1 and pre-existing tests.

- [ ] **Step 5: Confirm nothing else broke from the return-type change**

`NewAdministrator` now returns `*dbAdministrator` instead of the interface. The only call site is `apps/fleet-service/cmd/main.go:74`, where the value is passed as `fleet.OnboardingAdmin` — a concrete pointer still satisfies it.

Run: `go build ./... && go vet ./apps/fleet-service/...`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/membership/administrator.go \
        apps/fleet-service/internal/membership/administrator_db_test.go
git commit -m "feat(fleet-service): record member role changes and removals in-transaction"
```

---

### Task 3: PATCH endpoint and self-service DELETE

**Files:**
- Modify: `apps/fleet-service/internal/membership/resource.go`
- Modify: `apps/fleet-service/cmd/main.go:186`
- Test (create): `apps/fleet-service/internal/membership/resource_test.go`

**Interfaces:**
- Consumes: `ValidateRoleChange`, `ErrInvalidRole` (Task 1); `UpdateRole`, `Remove`, `ActivityRecorder`, `WithActivityRecorder` (Task 2); `authz.RequireSameFleet`, `authz.RequireOwner`; `Processor.RequireOwnerInFleet`, `Processor.GetMember`, `Processor.ValidateRemoval`.
- Produces: `func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, rec ActivityRecorder) func(chi.Router)` — **signature change**, third parameter added.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/membership/resource_test.go`:

```go
package membership

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// newMemberRouter builds the real chi router over a seeded in-memory database
// and returns both, so a test can drive a request and then read the row back
// rather than trusting the response echo.
func newMemberRouter(t *testing.T) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newMembershipDB(t)

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	// nil recorder: these tests assert authorization and validation, and the
	// activity path has its own coverage in administrator_db_test.go.
	r.Group(InitializeRoutes(log, db, nil))
	return r, db
}

// serveAs drives one request with a validated Identity on context, standing in
// for the JWT middleware the real router mounts upstream.
func serveAs(r chi.Router, method, path, body string, id auth.Identity) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req = req.WithContext(auth.WithIdentity(req.Context(), id))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func identity(userID, role, fleetID string) auth.Identity {
	return auth.Identity{UserID: userID, Email: userID + "@example.com", ActiveFleetID: fleetID, Role: role}
}

func patchBody(role string) string {
	return `{"data":{"type":"memberships","attributes":{"role":"` + role + `"}}}`
}

func patchRole(r chi.Router, fleetID, targetUserID, role string, id auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodPatch, "/fleets/"+fleetID+"/members/"+targetUserID, patchBody(role), id)
}

func deleteMember(r chi.Router, fleetID, targetUserID string, id auth.Identity) *httptest.ResponseRecorder {
	return serveAs(r, http.MethodDelete, "/fleets/"+fleetID+"/members/"+targetUserID, "", id)
}

func decodeDetail(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Errors []struct {
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if len(env.Errors) == 0 {
		return ""
	}
	return env.Errors[0].Detail
}

// --- PATCH ----------------------------------------------------------------

// FR-2.5: promoting does not demote the promoter. A fleet may hold any number
// of owners.
func TestPatchRole_promotesAMemberAndLeavesThePromoterAnOwner(t *testing.T) {
	r, db := newMemberRouter(t)
	owner := seedMembership(t, db, "u-owner", "owner")
	target := seedMembership(t, db, "u-member", "member")

	rec := patchRole(r, "f1", "u-member", "owner", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := readRole(t, db, target.ID()); got != "owner" {
		t.Fatalf("target role = %q, want owner", got)
	}
	if got := readRole(t, db, owner.ID()); got != "owner" {
		t.Fatalf("promoter role = %q, want owner — promotion must not demote", got)
	}
	if !strings.Contains(rec.Body.String(), `"role":"owner"`) {
		t.Errorf("response must carry the updated membership; got %s", rec.Body.String())
	}
}

func TestPatchRole_forbiddenForNonOwners(t *testing.T) {
	for _, role := range []string{"member", "viewer"} {
		r, db := newMemberRouter(t)
		seedMembership(t, db, "u-actor", role)
		seedMembership(t, db, "u-target", "member")

		rec := patchRole(r, "f1", "u-target", "owner", identity("u-actor", role, "f1"))
		if rec.Code != http.StatusForbidden {
			t.Errorf("PATCH as %s = %d, want 403", role, rec.Code)
		}
	}
}

// SEC-5. role is a JWT claim minted at login; the database is the authority.
// A token still claiming owner after a demotion must be rejected.
func TestPatchRole_forbiddenWhenTheOwnerClaimIsStale(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-actor", "member") // DB says member...
	seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "owner", identity("u-actor", "owner", "f1")) // ...token says owner
	if rec.Code != http.StatusForbidden {
		t.Fatalf("stale owner claim = %d, want 403", rec.Code)
	}
}

// FR-2.3. The message names the field and the allow-list without echoing the
// caller's input.
func TestPatchRole_rejectsAnUnknownRoleWithTheAllowList(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")
	seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "admin", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH role=admin = %d, want 422", rec.Code)
	}
	detail := decodeDetail(t, rec)
	for _, want := range []string{"role", "owner", "member", "viewer"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q must name %q", detail, want)
		}
	}
	if strings.Contains(detail, "admin") {
		t.Errorf("detail %q echoes the caller's input; the message must be a constant", detail)
	}
}

func TestPatchRole_notFoundWhenTheTargetIsNotAMember(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")

	rec := patchRole(r, "f1", "u-stranger", "owner", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PATCH against a non-member = %d, want 404", rec.Code)
	}
}

// FR-2.6.
func TestPatchRole_conflictWhenDemotingTheSoleOwner(t *testing.T) {
	r, db := newMemberRouter(t)
	owner := seedMembership(t, db, "u-owner", "owner")

	rec := patchRole(r, "f1", "u-owner", "member", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("demoting the sole owner = %d, want 409", rec.Code)
	}
	if got := readRole(t, db, owner.ID()); got != "owner" {
		t.Fatalf("role = %q after a rejected demotion, want owner", got)
	}
}

// FR-2.7.
func TestPatchRole_noOpSucceeds(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")
	target := seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "member", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("no-op PATCH = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := readRole(t, db, target.ID()); got != "member" {
		t.Fatalf("role = %q, want member", got)
	}
}

// RequireSameFleet runs first, so a cross-fleet request 404s before any role
// check can leak whether the fleet or the target exists.
func TestPatchRole_notFoundAcrossFleets(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")
	seedMembership(t, db, "u-target", "member")

	rec := patchRole(r, "f1", "u-target", "owner", identity("u-owner", "owner", "other-fleet"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-fleet PATCH = %d, want 404", rec.Code)
	}
}

// --- DELETE ---------------------------------------------------------------

// FR-3.1. Before this change the endpoint required owner at both layers, so a
// member or viewer had no way to leave a fleet at all.
func TestDeleteMember_selfRemovalIsAllowedForEveryRole(t *testing.T) {
	for _, role := range []string{"member", "viewer", "owner"} {
		r, db := newMemberRouter(t)
		// A co-owner so the "owner" case is not blocked by the sole-owner guard.
		seedMembership(t, db, "u-other-owner", "owner")
		self := seedMembership(t, db, "u-self", role)

		rec := deleteMember(r, "f1", "u-self", identity("u-self", role, "f1"))
		if rec.Code != http.StatusNoContent {
			t.Errorf("self-leave as %s = %d, want 204. Body: %s", role, rec.Code, strings.TrimSpace(rec.Body.String()))
			continue
		}
		if countRows(t, db, self.ID()) != 0 {
			t.Errorf("membership row still present after self-leave as %s", role)
		}
	}
}

// SEC-4. The relaxed branch must apply ONLY when the actor names their own row.
func TestDeleteMember_forbiddenWhenANonOwnerRemovesSomeoneElse(t *testing.T) {
	for _, role := range []string{"member", "viewer"} {
		r, db := newMemberRouter(t)
		seedMembership(t, db, "u-actor", role)
		other := seedMembership(t, db, "u-other", "member")

		rec := deleteMember(r, "f1", "u-other", identity("u-actor", role, "f1"))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s removing another member = %d, want 403", role, rec.Code)
		}
		if countRows(t, db, other.ID()) != 1 {
			t.Errorf("%s deleted another member's row; self-leave must not be a privilege escalation", role)
		}
	}
}

// FR-3.2: the sole-owner guard is unchanged and still reachable.
func TestDeleteMember_conflictWhenTheSoleOwnerLeaves(t *testing.T) {
	r, db := newMemberRouter(t)
	owner := seedMembership(t, db, "u-owner", "owner")

	rec := deleteMember(r, "f1", "u-owner", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("sole-owner self-leave = %d, want 409", rec.Code)
	}
	if countRows(t, db, owner.ID()) != 1 {
		t.Fatal("sole owner's row was deleted despite the 409")
	}
}

// FR-3.3: an owner may remove another owner. This cannot orphan the fleet —
// the actor is themselves an owner and remains.
func TestDeleteMember_ownerCanRemoveAnotherOwner(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner-a", "owner")
	b := seedMembership(t, db, "u-owner-b", "owner")

	rec := deleteMember(r, "f1", "u-owner-b", identity("u-owner-a", "owner", "f1"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("owner removing a co-owner = %d, want 204", rec.Code)
	}
	if countRows(t, db, b.ID()) != 0 {
		t.Fatal("co-owner's row still present")
	}
}

// RequireSameFleet stays OUTSIDE the isSelf branch: identity.UserID ==
// targetUserID is necessary but not sufficient.
func TestDeleteMember_selfRemovalDoesNotBypassTheSameFleetCheck(t *testing.T) {
	r, db := newMemberRouter(t)
	self := seedMembership(t, db, "u-self", "member")

	rec := deleteMember(r, "f1", "u-self", identity("u-self", "member", "other-fleet"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-fleet self-leave = %d, want 404", rec.Code)
	}
	if countRows(t, db, self.ID()) != 1 {
		t.Fatal("row deleted through a fleet the actor is not scoped to")
	}
}

func TestDeleteMember_notFoundWhenTheTargetIsNotAMember(t *testing.T) {
	r, db := newMemberRouter(t)
	seedMembership(t, db, "u-owner", "owner")

	rec := deleteMember(r, "f1", "u-stranger", identity("u-owner", "owner", "f1"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE against a non-member = %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/fleet-service/internal/membership/...`
Expected: FAIL — `InitializeRoutes` takes 2 args, not 3; no PATCH route registered.

- [ ] **Step 3: Rewrite the handler wiring**

Replace lines 1-84 of `apps/fleet-service/internal/membership/resource.go` with:

```go
package membership

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz"
)

// errRoleValidation is the TRANSPORT envelope for the domain's ErrInvalidRole.
// It names the field and the accepted values without echoing the caller's
// input, and it is a compile-time constant so no attacker-supplied string can
// reach the response. Same pairing as auth-service's errThemeValidation.
var errRoleValidation = fmt.Errorf("%w: role must be one of owner, member, viewer",
	server.ErrValidation)

// InitializeRoutes wires the JWT-protected membership endpoints under a fleet.
//
// rec is the activity recorder run inside the role-change and removal
// transactions (FR-5.2). Pass nil in tests that do not exercise the feed.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, rec ActivityRecorder) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db).WithActivityRecorder(rec)
	proc := NewProcessor(log, prov)
	return func(r chi.Router) {
		// GET /fleets/{id}/members — list fleet memberships (fleet-scoped)
		r.Get("/fleets/{id}/members", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			ms, err := proc.ListMembers(fleetID)
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})

		// PATCH /fleets/{id}/members/{userId} — change a membership's role.
		// Owner-only at BOTH layers; zero-owner guard in ValidateRoleChange.
		r.Patch("/fleets/{id}/members/{userId}", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
			Role string `json:"role"`
		},
		) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			targetUserID := chi.URLParam(req, "userId")

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Token-level gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check (stale-claim guard, SEC-5)
			if err := proc.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}

			target, err := proc.ValidateRoleChange(fleetID, targetUserID, attrs.Role)
			if err != nil {
				// Client errors are not incidents — do not log them.
				if errors.Is(err, ErrInvalidRole) {
					server.WriteError(w, errRoleValidation)
					return
				}
				server.WriteError(w, err)
				return
			}

			updated, err := adm.UpdateRole(target, attrs.Role, identity.UserID)
			if err != nil {
				log.WithError(err).Error("membership role update failed")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		}))

		// DELETE /fleets/{id}/members/{userId} — owner-only for OTHERS;
		// self-removal needs no role (FR-3.1). Sole-owner guard unchanged.
		r.Delete("/fleets/{id}/members/{userId}", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "id")
			targetUserID := chi.URLParam(req, "userId")

			// OUTSIDE the isSelf branch on purpose: this is what makes a
			// cross-fleet id 404 rather than leaking existence, and self-ness
			// must not be able to bypass it. identity.UserID == targetUserID is
			// necessary but not sufficient.
			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}

			// One predicate, two consequences: it relaxes the guard here and
			// picks member.left over member.removed in Administrator.Remove, so
			// the authorization decision and the audit trail cannot disagree.
			isSelf := identity.UserID == targetUserID
			if !isSelf {
				// Token-level gate (fast path)
				if err := authz.RequireOwner(identity); err != nil {
					server.WriteError(w, err)
					return
				}
				// Authoritative DB check (stale-claim guard, SEC-5)
				if err := proc.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
					server.WriteError(w, err)
					return
				}
			}

			targetMem, err := proc.GetMember(fleetID, targetUserID)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				server.WriteError(w, err)
				return
			}

			// Sole-owner self-removal guard (FR-3.2, unchanged)
			if err := proc.ValidateRemoval(fleetID, identity.UserID, targetUserID, targetMem.Role()); err != nil {
				server.WriteError(w, err)
				return
			}

			if err := adm.Remove(targetMem, identity.UserID); err != nil {
				log.WithError(err).Error("membership removal failed")
				server.WriteError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
```

- [ ] **Step 4: Update the wiring in `cmd/main.go`**

In `apps/fleet-service/cmd/main.go`, line 186, change:

```go
				membership.InitializeRoutes(log, db)(pr)
```

to:

```go
				membership.InitializeRoutes(log, db, activity.Record)(pr)
```

(`activity` is already imported at line 25.)

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./apps/fleet-service/... && go build ./... && go vet ./apps/fleet-service/...`
Expected: PASS, no build or vet output.

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/membership/resource.go \
        apps/fleet-service/internal/membership/resource_test.go \
        apps/fleet-service/cmd/main.go
git commit -m "feat(fleet-service): add role PATCH and allow members to leave a fleet"
```

---

### Task 4: `user.Provider.ListByIDs`

**Files:**
- Modify: `apps/auth-service/internal/user/provider.go`
- Modify: `apps/auth-service/internal/user/processor.go`
- Test: `apps/auth-service/internal/user/provider_test.go`

**Interfaces:**
- Consumes: `database.Query`, `Entity`, `Make(Entity) Model`.
- Produces:
  - `ListByIDs(ids []string) ([]Model, error)` on the `Provider` interface
  - `func (pr *Processor) ListByIDs(ids []string) ([]Model, error)` pass-through

> **Merge note (D11):** `task-011-platform-admin-console` specifies the same method for its admin route. The signature here is deliberately identical and carries **no scoping** — it is a plain `WHERE id IN (?)`. Whoever merges second deletes their duplicate and keeps the other's.

- [ ] **Step 1: Write the failing tests**

Append to `apps/auth-service/internal/user/provider_test.go`:

```go
func seedUserWith(t *testing.T, db *gorm.DB, id, sub, email, name string) {
	t.Helper()
	e := Entity{ID: id, GoogleSub: sub, Email: email, DisplayName: name, ThemePreference: ThemeSystem}
	if err := db.Create(&e).Error; err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func TestListByIDs_returnsOnlyTheRequestedUsers(t *testing.T) {
	db := newTestDB(t)
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "u2", "sub-2", "two@example.com", "Two")
	seedUserWith(t, db, "u3", "sub-3", "three@example.com", "Three")

	ms, err := NewProvider(db).ListByIDs([]string{"u1", "u3"})
	if err != nil {
		t.Fatalf("ListByIDs: %v", err)
	}
	got := map[string]string{}
	for _, m := range ms {
		got[m.ID()] = m.DisplayName()
	}
	if len(got) != 2 || got["u1"] != "One" || got["u3"] != "Three" {
		t.Fatalf("got %+v, want exactly u1 and u3", got)
	}
}

// FR-1.4: an id with no users row is simply absent. The handler treats that as
// a normal result, not an error, so the provider must not invent one.
func TestListByIDs_omitsUnknownIDsWithoutError(t *testing.T) {
	db := newTestDB(t)
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	ms, err := NewProvider(db).ListByIDs([]string{"u1", "ghost"})
	if err != nil {
		t.Fatalf("ListByIDs must not error on an unknown id: %v", err)
	}
	if len(ms) != 1 || ms[0].ID() != "u1" {
		t.Fatalf("got %+v, want only u1", ms)
	}
}

// An empty argument must not become `WHERE id IN ()` — some drivers turn that
// into a syntax error, and the caller's "nothing allowed" case is legitimate.
func TestListByIDs_returnsEmptyForNoIDs(t *testing.T) {
	db := newTestDB(t)
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	ms, err := NewProvider(db).ListByIDs(nil)
	if err != nil {
		t.Fatalf("ListByIDs(nil): %v", err)
	}
	if len(ms) != 0 {
		t.Fatalf("got %d rows, want 0", len(ms))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/auth-service/internal/user/...`
Expected: FAIL — `NewProvider(db).ListByIDs undefined`.

- [ ] **Step 3: Add `ListByIDs` to the provider**

In `apps/auth-service/internal/user/provider.go`, add to the `Provider` interface:

```go
	// ListByIDs returns the users matching any of the given internal user ids.
	// Unknown ids are simply absent from the result — not an error. There is NO
	// scoping in here: every caller applies its own (the fleet-scoped
	// /auth/users route intersects first; an admin route would not).
	ListByIDs(ids []string) ([]Model, error)
```

and the implementation below `GetBySub`:

```go
// ListByIDs looks users up by primary key in a single query.
//
// An empty id list short-circuits: `WHERE id IN ()` is not portable SQL, and
// "the caller is allowed to see nobody" is a legitimate outcome the /auth/users
// intersection produces routinely.
func (s *dbProvider) ListByIDs(ids []string) ([]Model, error) {
	if len(ids) == 0 {
		return []Model{}, nil
	}
	return database.Query(func() ([]Model, error) {
		var es []Entity
		if err := s.db.Where("id IN ?", ids).Find(&es).Error; err != nil {
			return nil, err
		}
		out := make([]Model, 0, len(es))
		for _, e := range es {
			out = append(out, Make(e))
		}
		return out, nil
	})()
}
```

- [ ] **Step 4: Add the processor pass-through**

Append to `apps/auth-service/internal/user/processor.go`:

```go
// ListByIDs resolves a batch of internal user ids. Scoping is the caller's job:
// GET /auth/users intersects the requested ids against the caller's fleet
// BEFORE calling this, so nothing here can leak a user from another fleet.
func (pr *Processor) ListByIDs(ids []string) ([]Model, error) {
	return pr.p.ListByIDs(ids)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./apps/auth-service/internal/user/... -v -run ListByIDs`
Expected: PASS — three tests.

- [ ] **Step 6: Build, vet and commit**

```bash
go build ./... && go vet ./apps/auth-service/...
git add apps/auth-service/internal/user/provider.go \
        apps/auth-service/internal/user/processor.go \
        apps/auth-service/internal/user/provider_test.go
git commit -m "feat(auth-service): add batch user lookup by id"
```

> If `go build` reports another type failing to satisfy `user.Provider` (a stub in `processor_test.go` or `newPrincipalResolver`'s parameter), add the same `ListByIDs` method to that stub returning `nil, nil`. Do not widen the interface's consumers.

---

### Task 5: `membership.Client.FleetMemberIDs`

**Files:**
- Modify: `apps/auth-service/internal/membership/client.go`
- Test: `apps/auth-service/internal/membership/client_test.go`

**Interfaces:**
- Consumes: fleet-service `GET /internal/fleets/{fleetID}/members` returning `[{"user_id":"…","role":"…"}]` (`apps/fleet-service/internal/membership/rest.go:46-49`). **No new fleet-service route.**
- Produces: `func (c *Client) FleetMemberIDs(ctx context.Context, fleetID string) ([]string, error)`

- [ ] **Step 1: Write the failing tests**

Append to `apps/auth-service/internal/membership/client_test.go`:

```go
func TestFleetMemberIDs_projectsUserIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/fleets/f1/members" {
			t.Errorf("path = %q, want /internal/fleets/f1/members", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"user_id":"u1","role":"owner"},{"user_id":"u2","role":"viewer"}]`))
	}))
	defer srv.Close()

	ids, err := NewClient(srv.URL).FleetMemberIDs(context.Background(), "f1")
	if err != nil {
		t.Fatalf("FleetMemberIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "u1" || ids[1] != "u2" {
		t.Fatalf("got %v, want [u1 u2]", ids)
	}
}

func TestFleetMemberIDs_returnsEmptyForAFleetWithNoMembers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	ids, err := NewClient(srv.URL).FleetMemberIDs(context.Background(), "f1")
	if err != nil || len(ids) != 0 {
		t.Fatalf("got %v err %v, want an empty slice and no error", ids, err)
	}
}

// The failure Active was written to catch, in a place where it bites harder:
// fleet-service's error envelope is JSON, so without an explicit status check a
// 500 decodes into an empty slice with err == nil, and every member name
// silently disappears from the settings card with nothing in the logs.
func TestFleetMemberIDs_failsClosedOnANon2xx(t *testing.T) {
	for _, status := range []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusUnauthorized,
	} {
		c := serving(t, status, fleetErrorEnvelope)
		ids, err := c.FleetMemberIDs(context.Background(), "f1")
		if err == nil {
			t.Errorf("status %d returned ids %v with no error; it must fail closed", status, ids)
		}
	}
}

// Active maps 404 to a zero value because "this user has no fleet" is a real
// state. Here the fleet id came off a validated token, so a 404 means something
// is wrong — it must NOT become "this fleet has no members".
func TestFleetMemberIDs_treats404AsAnError(t *testing.T) {
	c := serving(t, http.StatusNotFound, "")
	if ids, err := c.FleetMemberIDs(context.Background(), "f1"); err == nil {
		t.Fatalf("404 returned ids %v with no error; a fleet id from a valid token must exist", ids)
	}
}

// Status code only: the body is upstream-controlled and the fleet id must not
// ride along into a log line as an address.
func TestFleetMemberIDs_errorCarriesNoIDAndNoBody(t *testing.T) {
	c := serving(t, http.StatusInternalServerError, `{"errors":[{"detail":"secret-internal-detail"}]}`)
	_, err := c.FleetMemberIDs(context.Background(), "fleet-abc")
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), "fleet-abc") || strings.Contains(err.Error(), "secret-internal-detail") {
		t.Fatalf("error %q must carry neither the fleet id nor the upstream body", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/auth-service/internal/membership/...`
Expected: FAIL — `c.FleetMemberIDs undefined`.

- [ ] **Step 3: Implement the method**

In `apps/auth-service/internal/membership/client.go`, extend the imports to:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)
```

and append:

```go
// fleetMemberLookupTimeout bounds one auth→fleet hop. The Client shares
// http.DefaultClient, which has NO timeout, so without this a wedged
// fleet-service pins an auth-service handler open indefinitely.
const fleetMemberLookupTimeout = 5 * time.Second

// FleetMemberIDs returns the user ids of a fleet's active members via
// fleet-service's EXISTING internal endpoint (GET /internal/fleets/{id}/members).
// No new fleet-service route is introduced, and the call direction is auth→fleet,
// the direction that already exists — so no import or dependency cycle.
//
// Unlike Active, 404 is an ERROR here, not a sentinel. Active maps 404 to a zero
// value because "this user has no fleet" is a real state; the fleet id passed
// here comes from a validated token, so its absence is a fault. Letting it mean
// "no members" would silently blank every name in the member list.
func (c *Client) FleetMemberIDs(ctx context.Context, fleetID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, fleetMemberLookupTimeout)
	defer cancel()

	// PathEscape even though fleetID is a validated JWT claim and so is not
	// attacker-shaped: it costs nothing and stops the next caller inheriting
	// Active's raw-concatenation habit.
	endpoint := c.base + "/internal/fleets/" + url.PathEscape(fleetID) + "/members"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	// Every non-2xx is an error. fleet-service's error envelope is JSON, so
	// without this a 500 decodes cleanly into an empty slice with err == nil —
	// indistinguishable from a fleet that really has no members.
	//
	// Status code and a fixed description only: the body is upstream-controlled
	// and the fleet id must not ride along into a log line.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("fleet member lookup failed with status %d", res.StatusCode)
	}

	var rows []struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.UserID)
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./apps/auth-service/internal/membership/... -v`
Expected: PASS — five new tests plus the existing `Active` tests.

If `strings` is not yet imported in `client_test.go`, add it.

- [ ] **Step 5: Commit**

```bash
go vet ./apps/auth-service/...
git add apps/auth-service/internal/membership/client.go \
        apps/auth-service/internal/membership/client_test.go
git commit -m "feat(auth-service): resolve a fleet's member ids from fleet-service"
```

---

### Task 6: `GET /auth/users`

**Files:**
- Modify: `apps/auth-service/internal/user/resource.go`
- Modify: `apps/auth-service/internal/user/resource_test.go` (signature update only)
- Modify: `apps/auth-service/cmd/main.go:91`
- Test (create): `apps/auth-service/internal/user/users_resource_test.go`

**Interfaces:**
- Consumes: `Processor.ListByIDs` (Task 4), `Client.FleetMemberIDs` (Task 5), `TransformSlice` (`rest.go:18`), `auth.IdentityFromContext`, `errInternal` (`resource.go:19`).
- Produces:
  - `type FleetMemberGatherer func(ctx context.Context, fleetID string) ([]string, error)`
  - `func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, members FleetMemberGatherer) func(chi.Router)` — **signature change**, third parameter added.
  - Route `GET /auth/users?ids=a,b,c` → `{"data":[{"type":"users","id":"…","attributes":{…}}]}`

- [ ] **Step 1: Write the failing tests**

Create `apps/auth-service/internal/user/users_resource_test.go`:

```go
package user

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// newUsersRouter builds the real router with a STUB gatherer, so the scoping
// rules are unit-testable at the handler without standing up fleet-service.
// That injectability is the whole point of the function-value parameter.
func newUsersRouter(t *testing.T, members FleetMemberGatherer) (chi.Router, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, members))
	return r, db
}

func gatherer(ids ...string) FleetMemberGatherer {
	return func(context.Context, string) ([]string, error) { return ids, nil }
}

func getUsers(r chi.Router, query, activeFleetID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/auth/users"+query, nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        "caller",
		Email:         "caller@example.com",
		ActiveFleetID: activeFleetID,
		Role:          "owner",
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// decodeIDs pulls the resource ids out of the JSON:API document, and fails if
// `data` is missing entirely — `{"data":[]}` and `{}` are different answers.
func decodeIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var doc struct {
		Data *[]struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if doc.Data == nil {
		t.Fatalf("response has no `data` key; an empty result must marshal as [], not be omitted. Body: %s",
			strings.TrimSpace(rec.Body.String()))
	}
	out := make([]string, 0, len(*doc.Data))
	for _, d := range *doc.Data {
		out = append(out, d.ID)
	}
	return out
}

func TestAuthUsers_returnsRequestedFleetMembers(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1", "u2"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "u2", "sub-2", "two@example.com", "Two")

	rec := getUsers(r, "?ids=u1,u2", "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/users = %d, want 200. Body: %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	ids := decodeIDs(t, rec)
	if len(ids) != 2 {
		t.Fatalf("got %v, want both users", ids)
	}
	if !strings.Contains(rec.Body.String(), "One") || !strings.Contains(rec.Body.String(), "one@example.com") {
		t.Errorf("attributes must carry displayName and email; got %s", rec.Body.String())
	}
}

// SEC-1, the security property of the whole endpoint. A caller in fleet A
// asking about a user in fleet B must get a response INDISTINGUISHABLE from
// asking about an id that does not exist: 200 with the id absent. Not 403 (that
// confirms the user exists), not 404 (same), not an error of any kind — any of
// those turn this into a membership oracle.
func TestAuthUsers_silentlyOmitsUsersOutsideTheCallersFleet(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1")) // only u1 is in the caller's fleet
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "other", "sub-o", "other@example.com", "Other Fleet Person")

	rec := getUsers(r, "?ids=u1,other", "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 — a foreign id must not change the status", rec.Code)
	}
	ids := decodeIDs(t, rec)
	if len(ids) != 1 || ids[0] != "u1" {
		t.Fatalf("got %v, want only u1", ids)
	}
	if strings.Contains(rec.Body.String(), "other@example.com") {
		t.Fatal("a user outside the caller's fleet leaked into the response")
	}
}

// The other half of SEC-1: an id nobody has must produce the SAME response
// shape as an id belonging to another fleet.
func TestAuthUsers_foreignAndNonexistentIDsAreIndistinguishable(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")
	seedUserWith(t, db, "other", "sub-o", "other@example.com", "Other")

	foreign := getUsers(r, "?ids=other", "f1")
	ghost := getUsers(r, "?ids=does-not-exist", "f1")

	if foreign.Code != ghost.Code {
		t.Fatalf("status differs: foreign %d vs nonexistent %d", foreign.Code, ghost.Code)
	}
	if foreign.Body.String() != ghost.Body.String() {
		t.Fatalf("body differs, which makes this a membership oracle:\nforeign: %s\nghost:   %s",
			foreign.Body.String(), ghost.Body.String())
	}
}

// FR-1.4: a fleet member with no users row is omitted, not an error.
func TestAuthUsers_omitsFleetMembersWithNoUserRow(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1", "orphan"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	rec := getUsers(r, "?ids=u1,orphan", "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ids := decodeIDs(t, rec); len(ids) != 1 || ids[0] != "u1" {
		t.Fatalf("got %v, want only u1", ids)
	}
}

// FR-1.3. The gatherer must not even be consulted — there is no fleet to ask
// about, and calling it would be a pointless round trip on every fleetless load.
func TestAuthUsers_emptyDataWhenTheCallerHasNoActiveFleet(t *testing.T) {
	called := false
	r, db := newUsersRouter(t, func(context.Context, string) ([]string, error) {
		called = true
		return nil, nil
	})
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	rec := getUsers(r, "?ids=u1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ids := decodeIDs(t, rec); len(ids) != 0 {
		t.Fatalf("got %v, want an empty data array", ids)
	}
	if called {
		t.Error("the fleet lookup ran for a caller with no fleet")
	}
}

func TestAuthUsers_rejectsMissingOrEmptyIDs(t *testing.T) {
	r, _ := newUsersRouter(t, gatherer("u1"))

	for _, query := range []string{"", "?ids=", "?ids=%20", "?ids=,,,"} {
		rec := getUsers(r, query, "f1")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("query %q = %d, want 422", query, rec.Code)
		}
	}
}

// SEC-3. The cap bounds both the response size and the work per request.
func TestAuthUsers_rejectsMoreThan100IDs(t *testing.T) {
	r, _ := newUsersRouter(t, gatherer("u1"))

	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "u" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + strings.Repeat("y", i/26)
	}
	rec := getUsers(r, "?ids="+strings.Join(ids, ","), "f1")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("101 ids = %d, want 422", rec.Code)
	}
}

// De-duplication happens BEFORE the cap, so a repeated id cannot inflate the
// work done or trip the limit on its own.
func TestAuthUsers_deduplicatesBeforeApplyingTheCap(t *testing.T) {
	r, db := newUsersRouter(t, gatherer("u1"))
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	repeated := make([]string, 300)
	for i := range repeated {
		repeated[i] = "u1"
	}
	rec := getUsers(r, "?ids="+strings.Join(repeated, ","), "f1")
	if rec.Code != http.StatusOK {
		t.Fatalf("300 copies of one id = %d, want 200", rec.Code)
	}
	if ids := decodeIDs(t, rec); len(ids) != 1 {
		t.Fatalf("got %v, want a single u1", ids)
	}
}

// D4. Returning an empty 200 on a downstream failure would make a fleet-service
// outage indistinguishable from a fleet with no members — exactly the class of
// bug the membership.Client comment was written after. A 500 is visible in
// metrics and logs; a silent empty array is not. FR-1.7 still holds either way:
// the SPA renders id fallbacks regardless of which it gets.
func TestAuthUsers_returns500WhenTheFleetLookupFails(t *testing.T) {
	r, db := newUsersRouter(t, func(context.Context, string) ([]string, error) {
		return nil, errors.New("fleet-service is down")
	})
	seedUserWith(t, db, "u1", "sub-1", "one@example.com", "One")

	rec := getUsers(r, "?ids=u1", "f1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "fleet-service is down") {
		t.Fatal("the downstream error text reached the client; errInternal must render a bare 500")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./apps/auth-service/internal/user/...`
Expected: FAIL — `FleetMemberGatherer` undefined and `InitializeRoutes` takes 2 args.

- [ ] **Step 3: Add the route**

In `apps/auth-service/internal/user/resource.go`:

Extend the imports with `"context"` and `"strings"`.

Add below `errThemeValidation`:

```go
// maxUserIDs bounds both the response size and the work done per request
// (SEC-3). Applied AFTER de-duplication, so a repeated id cannot trip it.
const maxUserIDs = 100

// Both messages are compile-time constants: no caller-supplied string reaches
// the response, matching errThemeValidation above.
var (
	errIDsRequired = fmt.Errorf("%w: ids is required", server.ErrValidation)
	errTooManyIDs  = fmt.Errorf("%w: ids accepts at most 100 values", server.ErrValidation)
)

// FleetMemberGatherer returns the user ids of the active members of a fleet.
//
// Injected as a function value so the user package never imports the
// fleet-service membership client — the same constraint that produced
// PrincipalResolver (see auth-service/cmd/main.go Decision 1). It is also what
// makes the scoping rules unit-testable without standing up fleet-service.
type FleetMemberGatherer func(ctx context.Context, fleetID string) ([]string, error)

// parseUserIDs splits, trims and de-duplicates the `ids` query parameter.
// De-duplication precedes the cap so `?ids=a,a,a,…` cannot inflate the work
// done or trip the limit by itself.
func parseUserIDs(raw string) ([]string, error) {
	seen := make(map[string]bool)
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		v := strings.TrimSpace(part)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errIDsRequired
	}
	if len(out) > maxUserIDs {
		return nil, errTooManyIDs
	}
	return out, nil
}

// intersect keeps only the requested ids that are also fleet members, in the
// order they were requested. This is the whole of the scoping rule (FR-1.2):
// an id outside the set is dropped silently, so it is indistinguishable from an
// id that does not exist (SEC-1).
func intersect(requested, allowed []string) []string {
	set := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		set[a] = true
	}
	out := make([]string, 0, len(requested))
	for _, r := range requested {
		if set[r] {
			out = append(out, r)
		}
	}
	return out
}
```

Change the signature and add the handler. Replace:

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
```

with:

```go
// InitializeRoutes wires GET /auth/me (design §8.1, FR-AUTH-3), PATCH /auth/me
// (PRD §5.2) and GET /auth/users (task-014 §3).
//
// members resolves the caller's fleet roster for the /auth/users scoping rule.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, members FleetMemberGatherer) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db))
	return func(r chi.Router) {
```

and append this handler inside the returned closure, after the `PATCH /auth/me` registration:

```go
		// GET /auth/users?ids=a,b,c — resolve display names for a batch of user
		// ids, scoped to the caller's active fleet.
		//
		// Registered in this JWT-protected group, so SEC-2 is satisfied by
		// PLACEMENT rather than by a check that could be forgotten.
		//
		// Omission is the only failure mode: an id in another fleet, an id with
		// no users row, and a malformed id all produce the same 200 with the id
		// absent. There is deliberately no response shape meaning "that user
		// exists but you may not see them" (SEC-1).
		r.Get("/auth/users", func(w http.ResponseWriter, req *http.Request) {
			requested, err := parseUserIDs(req.URL.Query().Get("ids"))
			if err != nil {
				// Client errors are not incidents — do not log them.
				server.WriteError(w, err)
				return
			}

			id := auth.IdentityFromContext(req.Context())
			if id.ActiveFleetID == "" {
				// FR-1.3. Short-circuit before the hop: there is no fleet to ask
				// about, and the answer is the same either way.
				server.WriteJSON(w, http.StatusOK, server.Document{Data: []server.Resource{}})
				return
			}

			memberIDs, err := members(req.Context(), id.ActiveFleetID)
			if err != nil {
				// A 500, NOT an empty 200 (D4): an empty array would make a
				// fleet-service outage indistinguishable from a fleet with no
				// members, and only one of those shows up in metrics.
				log.WithError(err).Error("auth/users fleet member lookup failed")
				server.WriteError(w, errInternal)
				return
			}

			allowed := intersect(requested, memberIDs)
			ms, err := proc.ListByIDs(allowed)
			if err != nil {
				log.WithError(err).Error("auth/users lookup failed")
				// Deliberately not WriteError(w, err): the envelope puts
				// err.Error() in the title, which would leak database internals.
				server.WriteError(w, errInternal)
				return
			}

			// TransformSlice returns make([]server.Resource, 0, …), so an empty
			// result marshals as [] and never as null.
			//
			// Attributes also carries themePreference — another member's UI
			// preference, not sensitive but not the caller's business either.
			// Reusing Transform as-is is deliberate: a second transform to strip
			// one cosmetic field would fork the "keep rest.go and
			// types/models/user.ts in step" contract for no security gain.
			server.WriteJSON(w, http.StatusOK, server.Document{Data: TransformSlice(ms)})
		})
```

- [ ] **Step 4: Update the existing router helper**

In `apps/auth-service/internal/user/resource_test.go`, line 42, change:

```go
	r.Group(InitializeRoutes(log, db))
```

to:

```go
	// nil gatherer: these tests drive /auth/me only, which never consults it.
	r.Group(InitializeRoutes(log, db, nil))
```

- [ ] **Step 5: Wire it in `cmd/main.go`**

In `apps/auth-service/cmd/main.go`, replace line 91:

```go
				user.InitializeRoutes(log, db)(pr)
```

with:

```go
				// Decision 1 again: compose the concrete fleet client into a
				// function value so the user package never imports it.
				user.InitializeRoutes(log, db, func(ctx context.Context, fleetID string) ([]string, error) {
					return fleetClient.FleetMemberIDs(ctx, fleetID)
				})(pr)
```

(`context` is already imported at line 4.)

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./apps/auth-service/... && go build ./... && go vet ./apps/auth-service/...`
Expected: PASS — ten new handler tests plus the existing `/auth/me` suite; no build or vet output.

- [ ] **Step 7: Verify SEC-2 by inspecting the route's placement**

There is no unit test for "401 without a JWT": the handler tests mount a bare
chi router with an `Identity` injected directly, and the middleware that would
reject an unauthenticated request lives in `cmd/main.go`. SEC-2 is satisfied by
**placement** — the route is registered inside the group that applies
`authmw.JWT` — so confirm the placement by reading it:

```bash
sed -n '86,100p' apps/auth-service/cmd/main.go
```

Expected: the `user.InitializeRoutes(...)` call you edited in Step 5 sits inside
the `r.Group(func(pr chi.Router) { pr.Use(authmw.JWT(...)); … })` block, on the
same `pr` router — **not** in a sibling `AddRouteInitializer`. `/auth/users`
carries no auth check of its own; if it ever moves out of that group it becomes
world-readable.

- [ ] **Step 8: Commit**

```bash
git add apps/auth-service/internal/user/resource.go \
        apps/auth-service/internal/user/resource_test.go \
        apps/auth-service/internal/user/users_resource_test.go \
        apps/auth-service/cmd/main.go
git commit -m "feat(auth-service): add fleet-scoped batch user lookup endpoint"
```

---

### Task 7: `alert-dialog` primitive

**Files:**
- Modify: `apps/web/package.json`
- Create: `apps/web/src/components/ui/alert-dialog.tsx`

**Interfaces:**
- Consumes: `cn` (`apps/web/src/lib/utils.ts`), `buttonVariants` (`apps/web/src/components/ui/button.tsx`).
- Produces: `AlertDialog`, `AlertDialogTrigger`, `AlertDialogPortal`, `AlertDialogOverlay`, `AlertDialogContent`, `AlertDialogHeader`, `AlertDialogFooter`, `AlertDialogTitle`, `AlertDialogDescription`, `AlertDialogAction`, `AlertDialogCancel`.

> **Why alert-dialog and not dialog (D10):** every dialog in this task is a confirm/cancel decision on a destructive or privilege-granting action, which is exactly Radix's alert-dialog semantics — focus pinned to cancel, no dismiss-on-outside-click, `role="alertdialog"`. No other in-flight worktree adds this component; expect at most a `package.json` merge.

- [ ] **Step 1: Install the dependency**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm install --workspace apps/web @radix-ui/react-alert-dialog@^1.1.0
```

Verify `apps/web/package.json` gained `"@radix-ui/react-alert-dialog"` under `dependencies` and that `package-lock.json` was updated.

- [ ] **Step 2: Create the component**

Create `apps/web/src/components/ui/alert-dialog.tsx`:

```tsx
import * as React from 'react';
import * as AlertDialogPrimitive from '@radix-ui/react-alert-dialog';
import { cn } from '../../lib/utils';
import { buttonVariants } from './button';

const AlertDialog = AlertDialogPrimitive.Root;
const AlertDialogTrigger = AlertDialogPrimitive.Trigger;
const AlertDialogPortal = AlertDialogPrimitive.Portal;

const AlertDialogOverlay = React.forwardRef<
  React.ElementRef<typeof AlertDialogPrimitive.Overlay>,
  React.ComponentPropsWithoutRef<typeof AlertDialogPrimitive.Overlay>
>(({ className, ...props }, ref) => (
  <AlertDialogPrimitive.Overlay
    ref={ref}
    className={cn('fixed inset-0 z-50 bg-black/80', className)}
    {...props}
  />
));
AlertDialogOverlay.displayName = AlertDialogPrimitive.Overlay.displayName;

const AlertDialogContent = React.forwardRef<
  React.ElementRef<typeof AlertDialogPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof AlertDialogPrimitive.Content>
>(({ className, ...props }, ref) => (
  <AlertDialogPortal>
    <AlertDialogOverlay />
    <AlertDialogPrimitive.Content
      ref={ref}
      className={cn(
        'fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg translate-x-[-50%] translate-y-[-50%] gap-4 border bg-background p-6 shadow-lg sm:rounded-lg',
        className,
      )}
      {...props}
    />
  </AlertDialogPortal>
));
AlertDialogContent.displayName = AlertDialogPrimitive.Content.displayName;

function AlertDialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div className={cn('flex flex-col space-y-2 text-center sm:text-left', className)} {...props} />
  );
}
AlertDialogHeader.displayName = 'AlertDialogHeader';

function AlertDialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn('flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2', className)}
      {...props}
    />
  );
}
AlertDialogFooter.displayName = 'AlertDialogFooter';

const AlertDialogTitle = React.forwardRef<
  React.ElementRef<typeof AlertDialogPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof AlertDialogPrimitive.Title>
>(({ className, ...props }, ref) => (
  <AlertDialogPrimitive.Title
    ref={ref}
    className={cn('text-lg font-semibold', className)}
    {...props}
  />
));
AlertDialogTitle.displayName = AlertDialogPrimitive.Title.displayName;

const AlertDialogDescription = React.forwardRef<
  React.ElementRef<typeof AlertDialogPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof AlertDialogPrimitive.Description>
>(({ className, ...props }, ref) => (
  <AlertDialogPrimitive.Description
    ref={ref}
    className={cn('text-sm text-muted-foreground', className)}
    {...props}
  />
));
AlertDialogDescription.displayName = AlertDialogPrimitive.Description.displayName;

const AlertDialogAction = React.forwardRef<
  React.ElementRef<typeof AlertDialogPrimitive.Action>,
  React.ComponentPropsWithoutRef<typeof AlertDialogPrimitive.Action>
>(({ className, ...props }, ref) => (
  <AlertDialogPrimitive.Action ref={ref} className={cn(buttonVariants(), className)} {...props} />
));
AlertDialogAction.displayName = AlertDialogPrimitive.Action.displayName;

const AlertDialogCancel = React.forwardRef<
  React.ElementRef<typeof AlertDialogPrimitive.Cancel>,
  React.ComponentPropsWithoutRef<typeof AlertDialogPrimitive.Cancel>
>(({ className, ...props }, ref) => (
  <AlertDialogPrimitive.Cancel
    ref={ref}
    className={cn(buttonVariants({ variant: 'outline' }), 'mt-2 sm:mt-0', className)}
    {...props}
  />
));
AlertDialogCancel.displayName = AlertDialogPrimitive.Cancel.displayName;

export {
  AlertDialog,
  AlertDialogPortal,
  AlertDialogOverlay,
  AlertDialogTrigger,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogFooter,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogAction,
  AlertDialogCancel,
};
```

- [ ] **Step 3: Verify it type-checks and the app still builds**

Run:
```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web build
```
Expected: build succeeds.

If `buttonVariants` is not exported from `apps/web/src/components/ui/button.tsx`, export it there (`export { Button, buttonVariants }`) rather than duplicating the class strings.

- [ ] **Step 4: Commit**

```bash
git add apps/web/package.json package-lock.json apps/web/src/components/ui/alert-dialog.tsx
git commit -m "feat(web): add alert-dialog primitive"
```

---

### Task 8: `UserService.listByIds` and the `useUsers` hook

**Files:**
- Create: `apps/web/src/services/api/UserService.ts`
- Create: `apps/web/src/lib/hooks/api/users.ts`
- Test (create): `apps/web/src/lib/hooks/api/users.test.ts`

**Interfaces:**
- Consumes: `BaseService`/`ListResult` (`services/api/BaseService.ts`), `UserAttributes` (`types/models/user.ts`), the `GET /api/auth/users?ids=` route from Task 6.
- Produces:
  - `export const userService` with `listByIds(ids: string[]): Promise<ListResult<UserAttributes>>`
  - `export const userKeys = { all, byIds(ids) }`
  - `export function useUsers(ids: string[])` → `UseQueryResult<Record<string, UserAttributes>>`

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/lib/hooks/api/users.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import React from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { userKeys, useUsers } from './users';
import { userService } from '../../../services/api/UserService';

vi.mock('../../../services/api/UserService', () => ({
  userService: { listByIds: vi.fn() },
}));

function makeWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return React.createElement(QueryClientProvider, { client: queryClient }, children);
  };
}

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(userService.listByIds).mockResolvedValue({
    data: [
      {
        type: 'users',
        id: 'u1',
        attributes: {
          email: 'one@example.com',
          displayName: 'One',
          avatarUrl: '',
          themePreference: 'system',
        },
      },
    ],
    meta: undefined,
  });
});

describe('userKeys', () => {
  it('is hierarchical', () => {
    expect(userKeys.all).toEqual(['users']);
    expect(userKeys.byIds(['a', 'b'])).toEqual(['users', 'byIds', 'a,b']);
  });

  // The key must not depend on the order useMembers happens to return rows in,
  // or a reordered list refetches every render.
  it('is stable under reordering and duplication', () => {
    expect(userKeys.byIds(['b', 'a'])).toEqual(userKeys.byIds(['a', 'b']));
  });
});

describe('useUsers', () => {
  it('indexes the response by user id', async () => {
    const { result } = renderHook(() => useUsers(['u1']), { wrapper: makeWrapper(newClient()) });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.u1.displayName).toBe('One');
  });

  // Sorting and de-duping happen inside the hook, so callers can pass the raw
  // membership order without thinking about it.
  it('sorts and de-duplicates the ids it requests', async () => {
    const { result } = renderHook(() => useUsers(['u2', 'u1', 'u1']), {
      wrapper: makeWrapper(newClient()),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(userService.listByIds).toHaveBeenCalledWith(['u1', 'u2']);
  });

  // An empty id list means "the member list has not arrived yet". Firing a
  // request for it would be a guaranteed 422 from the server.
  it('does not fire a request when there are no ids', () => {
    const { result } = renderHook(() => useUsers([]), { wrapper: makeWrapper(newClient()) });

    expect(userService.listByIds).not.toHaveBeenCalled();
    expect(result.current.fetchStatus).toBe('idle');
  });

  // FR-1.7: a failed name lookup must be a normal, renderable state, not a
  // thrown error — the member list still renders with id fallbacks.
  it('surfaces a failure as an error state with undefined data', async () => {
    vi.mocked(userService.listByIds).mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => useUsers(['u1']), { wrapper: makeWrapper(newClient()) });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.data).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run:
```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- users.test
```
Expected: FAIL — cannot resolve `./users` or `../../../services/api/UserService`.

- [ ] **Step 3: Create the service**

Create `apps/web/src/services/api/UserService.ts`:

```ts
/**
 * UserService — auth-service user endpoints.
 *
 * Backend route (apps/auth-service/internal/user/resource.go, gateway-prefixed):
 *   GET /api/auth/users?ids=a,b,c — batch display-name lookup
 *
 * The endpoint is scoped to the CALLER'S ACTIVE FLEET server-side: ids outside
 * that fleet, and ids with no user row, are silently omitted from `data`. A
 * shorter response than the request asked for is therefore normal, never an
 * error condition to surface.
 */
import { apiClient } from '../../lib/api/client';
import { BaseService, type ListResult } from './BaseService';
import type { UserAttributes } from '../../types/models/user';

class UserService extends BaseService<UserAttributes> {
  protected readonly resourceType = 'users';
  protected readonly basePath = '/api/auth/users';

  /**
   * GET /api/auth/users?ids=a,b,c
   *
   * Deliberately does NOT chunk. The server caps `ids` at 100 and a household
   * fleet is single-digit, so chunking here would be speculative and untested.
   * If the activity feed later needs it, it belongs there — that is where the
   * id set is actually unbounded.
   */
  async listByIds(ids: string[]): Promise<ListResult<UserAttributes>> {
    const query = ids.map((id) => encodeURIComponent(id)).join(',');
    return this.listAt(`${this.basePath}?ids=${query}`);
  }
}

export const userService = new UserService();
```

`apiClient` is imported for parity with the other services; if lint flags it as unused, drop the import — `listAt` already routes through it.

- [ ] **Step 4: Create the hook**

Create `apps/web/src/lib/hooks/api/users.ts`:

```ts
/**
 * React Query hooks for user display-name resolution (task-014).
 *
 * This is a SECOND, INDEPENDENT query alongside useMembers rather than a
 * `select` over it. That independence is what buys FR-1.7: "memberships loaded,
 * names failed" is a normal renderable state, and the member list falls back to
 * shortened ids instead of going blank.
 */
import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { userService } from '../../../services/api/UserService';
import type { UserAttributes } from '../../../types/models/user';

// ---------------------------------------------------------------------------
// Query key factory
// ---------------------------------------------------------------------------

/**
 * all          -> ['users']
 * byIds(ids)   -> ['users', 'byIds', 'a,b,c']
 *
 * The ids are sorted into the key so it does not depend on the order
 * useMembers happens to return rows in — otherwise a reordered list refetches.
 */
export const userKeys = {
  all: ['users'] as const,
  byIds: (ids: string[]) => [...userKeys.all, 'byIds', [...ids].sort().join(',')] as const,
};

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

/**
 * GET /api/auth/users?ids=… — resolves a batch of user ids to their profiles,
 * indexed by id.
 *
 * Sorting and de-duplication happen here so callers can pass the raw membership
 * order. `staleTime` matches useMembers: the two are rendered together and a
 * shorter window here would refetch names on every members cache hit.
 */
export function useUsers(ids: string[]) {
  const sorted = useMemo(() => [...new Set(ids)].sort(), [ids]);
  return useQuery({
    queryKey: userKeys.byIds(sorted),
    queryFn: () => userService.listByIds(sorted),
    enabled: sorted.length > 0,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result): Record<string, UserAttributes> =>
      Object.fromEntries(result.data.map((r) => [r.id, r.attributes])),
  });
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- users.test`
Expected: PASS — seven tests.

- [ ] **Step 6: Lint and commit**

```bash
npm run -w apps/web lint
git add apps/web/src/services/api/UserService.ts \
        apps/web/src/lib/hooks/api/users.ts \
        apps/web/src/lib/hooks/api/users.test.ts
git commit -m "feat(web): resolve member display names from auth-service"
```

---

### Task 9: `useUpdateMemberRole` and self-aware `useRemoveMember`

**Files:**
- Modify: `apps/web/src/services/api/MemberService.ts`
- Modify: `apps/web/src/lib/hooks/api/members.ts`
- Test: `apps/web/src/lib/hooks/api/members.test.ts`

**Interfaces:**
- Consumes: `userKeys` (Task 8), `memberKeys`, `fleetKeys`, `authKeys`, `mintAccessToken` (`lib/api/refresh.ts`), the PATCH route from Task 3.
- Produces:
  - `memberService.updateRole(fleetId: string, userId: string, role: FleetRole): Promise<Membership>`
  - `useUpdateMemberRole(fleetId: string)` — variables `{ userId: string; role: FleetRole }`
  - `useRemoveMember(fleetId: string)` — variables **change** from `string` to `{ userId: string; isSelf: boolean }`

- [ ] **Step 1: Write the failing tests**

In `apps/web/src/lib/hooks/api/members.test.ts`:

Extend the `MemberService` mock (line 56-60) to:

```ts
vi.mock('../../../services/api/MemberService', () => ({
  memberService: {
    removeMember: vi.fn().mockResolvedValue(undefined),
    updateRole: vi.fn().mockResolvedValue({
      id: 'm1',
      type: 'memberships',
      attributes: { fleetId: 'f1', userId: 'user-1', role: 'owner', status: 'active' },
    }),
  },
}));
```

Update the import on line 21 to `import { memberKeys, useRemoveMember, useUpdateMemberRole } from './members';`, and add `import { userKeys } from './users';` and `import { memberService } from '../../../services/api/MemberService';`.

Update the existing `useRemoveMember` call (line 128) from `result.current.mutate('user-1')` to:

```ts
      result.current.mutate({ userId: 'user-1', isSelf: false });
```

Then append these tests inside the `describe('mutation invalidation contracts — real hooks', …)` block:

```ts
  // FR-4.1: role and active_fleet_id are JWT claims minted at login. After the
  // actor's OWN membership disappears, the token still claims the fleet, so the
  // SPA must re-mint before /auth/me is believed.
  //
  // mintAccessToken, NOT refreshAccessToken: the removal already committed
  // server-side, so a transient mint failure must not clear a still-valid token
  // and log the user out of a session they are mid-way through leaving. Same
  // reasoning as useAcceptInvite.
  it('useRemoveMember mints a fresh token and refetches identity on a self-leave', async () => {
    const { result } = renderHook(() => useRemoveMember('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'me', isSelf: true });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mintAccessToken).toHaveBeenCalledTimes(1);

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: authKeys.all }));
  });

  // FR-4.4: removing SOMEONE ELSE leaves the actor's own claims untouched, so
  // re-minting would be a pointless round trip on every removal.
  it('useRemoveMember does not mint a token when removing another member', async () => {
    const { result } = renderHook(() => useRemoveMember('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'someone-else', isSelf: false });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mintAccessToken).not.toHaveBeenCalled();

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).not.toContainEqual(expect.objectContaining({ queryKey: authKeys.all }));
  });

  // FR-4.3: names are keyed off the member list, so a membership change must
  // drop the cached name map too.
  it('useRemoveMember invalidates the user name cache', async () => {
    const { result } = renderHook(() => useRemoveMember('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'user-1', isSelf: false });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: userKeys.all }));
  });

  it('useUpdateMemberRole PATCHes the role and invalidates members and fleets', async () => {
    const { result } = renderHook(() => useUpdateMemberRole('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'user-1', role: 'owner' });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(memberService.updateRole).toHaveBeenCalledWith('f1', 'user-1', 'owner');

    const calls = invalidateSpy.mock.calls.map((c) => c[0]);
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: memberKeys.lists() }));
    expect(calls).toContainEqual(expect.objectContaining({ queryKey: fleetKeys.all }));
  });

  // FR-4.4 again: promoting someone else does not touch the actor's claims.
  it('useUpdateMemberRole does not mint a token', async () => {
    const { result } = renderHook(() => useUpdateMemberRole('f1'), {
      wrapper: makeWrapper(queryClient),
    });

    await act(async () => {
      result.current.mutate({ userId: 'user-1', role: 'owner' });
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mintAccessToken).not.toHaveBeenCalled();
  });
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run -w apps/web test -- members.test`
Expected: FAIL — `useUpdateMemberRole` is not exported.

- [ ] **Step 3: Add `updateRole` to the service**

In `apps/web/src/services/api/MemberService.ts`, extend the header comment's route list with:

```
 *   PATCH  /api/fleet/fleets/{id}/members/{userId} — change a member's role (owner-only)
```

update the imports to:

```ts
import type { JsonApiDocument, JsonApiResource } from '@myfleet/shared-ts';
import { apiClient } from '../../lib/api/client';
import { BaseService, type ListResult } from './BaseService';
import type { MembershipAttributes } from '../../types/models/membership';
import type { FleetRole } from '../../types/models/user';
```

and add the method inside the class:

```ts
  /**
   * PATCH /api/fleet/fleets/{fleetId}/members/{userId}
   *
   * Written out rather than routed through BaseService.patch: `basePath` is the
   * placeholder this class already documents as "not used directly" — every
   * real membership route is nested under a fleet.
   *
   * Returns 409 when the change would leave the fleet with zero owners.
   */
  async updateRole(
    fleetId: string,
    userId: string,
    role: FleetRole,
  ): Promise<JsonApiResource<MembershipAttributes>> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<MembershipAttributes>>>(
      `/api/fleet/fleets/${fleetId}/members/${userId}`,
      {
        method: 'PATCH',
        body: JSON.stringify({ data: { type: this.resourceType, attributes: { role } } }),
      },
    );
    return doc.data;
  }
```

- [ ] **Step 4: Rewrite the mutations**

In `apps/web/src/lib/hooks/api/members.ts`, extend the imports:

```ts
import { mintAccessToken } from '../../api/refresh';
import { fleetKeys } from './fleets';
import { authKeys } from './auth';
import { userKeys } from './users';
import type { FleetRole } from '../../../types/models/user';
```

Replace the whole `useRemoveMember` function with:

```ts
/**
 * DELETE /api/fleet/fleets/{fleetId}/members/{userId}.
 *
 * Removing ANOTHER member is owner-only; removing YOURSELF is allowed for every
 * role (FR-3.1). `isSelf` is a mutation variable rather than something the hook
 * re-derives, because it decides two things at once — whether to re-mint the
 * session, and (server-side) whether the removal is logged as member.left or
 * member.removed. Re-deriving it here would create a second source of truth.
 *
 * Sole-owner guard: HTTP 409 is surfaced as toast.error.
 */
export function useRemoveMember(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ userId, isSelf }: { userId: string; isSelf: boolean }) => {
      await memberService.removeMember(fleetId, userId);
      if (isSelf) {
        // FR-4.1. active_fleet_id and role are JWT claims fixed at mint time,
        // so the token in hand still claims a fleet the user just left.
        //
        // mintAccessToken, NOT refreshAccessToken: the removal already
        // committed server-side, so a transient mint failure must not clear a
        // still-valid token — that would log the user out on the path that just
        // succeeded. Same reasoning as useAcceptInvite.
        await mintAccessToken();
      }
      return { isSelf };
    },
    onSuccess: async ({ isSelf }) => {
      if (isSelf) {
        // Refetching /auth/me is what routes the user onward: the refreshed
        // token resolves to an empty active_fleet_id and RequireAuth redirects
        // to onboarding on its own. A manual navigate() here would be a second,
        // racing source of truth for the same decision.
        await queryClient.invalidateQueries({ queryKey: authKeys.all });
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: memberKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
      // Names are keyed off the member id set, so they go stale with it.
      void queryClient.invalidateQueries({ queryKey: userKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      // HTTP 409 = sole-owner guard. The new UI prevents reaching it in the
      // normal flow, but the endpoint still returns it and a stale member list
      // can still get here.
      if (apiError.status === 409) {
        toast.error(
          'Cannot remove the sole owner of the fleet. Transfer ownership first, or delete the fleet.',
        );
      } else {
        toast.error(apiError.message || 'Could not remove member');
      }
    },
  });
}

/**
 * PATCH /api/fleet/fleets/{fleetId}/members/{userId} — change a member's role.
 * Owner-only, enforced server-side at both the token and database layers.
 *
 * No session refresh (FR-4.4): the actor's own claims are untouched. The
 * PROMOTED user's token still says `member` until their next mint, which fails
 * CLOSED — they gain owner powers on their next refresh.
 */
export function useUpdateMemberRole(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ userId, role }: { userId: string; role: FleetRole }) =>
      memberService.updateRole(fleetId, userId, role),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: memberKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: fleetKeys.all });
    },
    onError: (err) => {
      const apiError = createErrorFromUnknown(err);
      if (apiError.status === 409) {
        toast.error('This fleet would be left with no owner. Promote someone else first.');
      } else {
        toast.error(apiError.message || 'Could not change this member’s role');
      }
    },
  });
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- members.test`
Expected: PASS — all pre-existing tests plus the five new ones.

- [ ] **Step 6: Type-check, lint and commit**

```bash
npm run -w apps/web build && npm run -w apps/web lint
git add apps/web/src/services/api/MemberService.ts \
        apps/web/src/lib/hooks/api/members.ts \
        apps/web/src/lib/hooks/api/members.test.ts
git commit -m "feat(web): add role-change mutation and self-aware member removal"
```

> `MemberList.tsx` still calls `removeMember.mutate(m.attributes.userId)` at this point and will fail type-check. If `npm run -w apps/web build` errors there, change that one call to `removeMember.mutate({ userId: m.attributes.userId, isSelf })` as a stopgap — Task 10 replaces the file wholesale.

---

### Task 10: MemberList — names, "(you)", and the three confirmations

**Files:**
- Rewrite: `apps/web/src/components/features/settings/MemberList.tsx`
- Test (create): `apps/web/src/components/features/settings/MemberList.test.tsx`

**Interfaces:**
- Consumes: `useMembers`, `useRemoveMember`, `useUpdateMemberRole` (Task 9), `useUsers` (Task 8), the alert-dialog primitives (Task 7), `useAuth` (`context/AuthContext`), `Select` family (`components/ui/select.tsx`).
- Produces: `export function MemberList({ fleetId, isOwner }: MemberListProps)` — props unchanged, so `SettingsPage.tsx:52` needs no edit. Also exports `displayFor(userId: string, users?: Record<string, UserAttributes>): string`, available for reuse when the activity feed adopts name resolution later; the tests below exercise it through the rendered list rather than directly.

**Behaviour — the ux-flow state matrix, which the tests enumerate:**

| # | myRole | ownerCount | memberCount | Leave button | Dialog |
|---|---|---|---|---|---|
| 1 | member / viewer | any | any | Enabled | Plain leave confirmation |
| 2 | owner | ≥ 2 | any | Enabled | Plain leave confirmation |
| 3 | owner | 1 | ≥ 2 | Enabled | Leave **with successor picker** |
| 4 | owner | 1 | 1 | **Disabled** | None — inline explanation |

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/components/features/settings/MemberList.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { memberService } from '../../../services/api/MemberService';
import { userService } from '../../../services/api/UserService';
import { MemberList } from './MemberList';
import type { Membership } from '../../../types/models/membership';

vi.mock('../../../services/api/MemberService', () => ({
  memberService: {
    listByFleet: vi.fn(),
    removeMember: vi.fn().mockResolvedValue(undefined),
    updateRole: vi.fn(),
  },
}));

vi.mock('../../../services/api/UserService', () => ({
  userService: { listByIds: vi.fn() },
}));

// Both exports: the API client imports refreshAccessToken from this module, so
// a partial mock would break its import.
vi.mock('../../../lib/api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('fresh-token'),
  refreshAccessToken: vi.fn().mockResolvedValue('fresh-token'),
}));

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// useAuth supplies the caller's identity. 'me' is the authenticated user in
// every test below. MemberList reads only `user.id` — every role decision it
// makes comes from the members LIST, not from this claim (see the component's
// myRole comment) — but the object is filled out so the mock type-checks.
vi.mock('../../../context/AuthContext', () => ({
  useAuth: () => ({
    user: {
      id: 'me',
      type: 'users',
      attributes: {
        email: 'me@example.com',
        displayName: 'Me',
        avatarUrl: '',
        themePreference: 'system',
      },
    },
    activeFleetId: 'f1',
    role: 'owner',
    isAuthenticated: true,
    isLoading: false,
  }),
}));

function membership(userId: string, role: string): Membership {
  return {
    type: 'memberships',
    id: 'm-' + userId,
    attributes: { fleetId: 'f1', userId, role, status: 'active' },
  };
}

function userRow(id: string, displayName: string, email: string) {
  return {
    type: 'users' as const,
    id,
    attributes: { displayName, email, avatarUrl: '', themePreference: 'system' as const },
  };
}

function seed(members: Membership[], users: ReturnType<typeof userRow>[] = []) {
  vi.mocked(memberService.listByFleet).mockResolvedValue({ data: members, meta: undefined });
  vi.mocked(userService.listByIds).mockResolvedValue({ data: users, meta: undefined });
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(memberService.removeMember).mockResolvedValue(undefined);
  vi.mocked(memberService.updateRole).mockResolvedValue(membership('other', 'owner'));
});

describe('MemberList — names', () => {
  // FR-1.5. Before this task the card showed three indistinguishable UUIDs and
  // nobody could tell who they were about to remove.
  it('renders displayName, then email, then a shortened id', async () => {
    seed(
      [
        membership('me', 'owner'),
        membership('has-email-only-0000', 'member'),
        membership('has-nothing-0000', 'viewer'),
      ],
      [
        userRow('me', 'Jane Doe', 'jane@example.com'),
        userRow('has-email-only-0000', '', 'sam@example.com'),
      ],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText(/Jane Doe/)).toBeInTheDocument();
    expect(screen.getByText('sam@example.com')).toBeInTheDocument();
    // Neither name nor email: the first 8 characters of the id.
    expect(screen.getByText('has-noth')).toBeInTheDocument();
  });

  // An unset displayName arrives as "" from Go, not null. `??` would let the
  // empty string through and render a blank row.
  it('falls through an empty-string displayName to the email', async () => {
    seed([membership('u1', 'member')], [userRow('u1', '', 'blank@example.com')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText('blank@example.com')).toBeInTheDocument();
  });

  // FR-1.7. A name-service failure must not blank the members card.
  it('still renders the list with id fallbacks when the name lookup fails', async () => {
    vi.mocked(memberService.listByFleet).mockResolvedValue({
      data: [membership('abcdefgh-1234', 'member')],
      meta: undefined,
    });
    vi.mocked(userService.listByIds).mockRejectedValue(new Error('auth-service down'));

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText('abcdefgh')).toBeInTheDocument();
  });

  // FR-1.6.
  it('marks the authenticated user with (you)', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('me', 'Jane Doe', 'jane@example.com'), userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    expect(await screen.findByText(/Jane Doe \(you\)/)).toBeInTheDocument();
    expect(screen.getByText('Sam Ito')).toBeInTheDocument();
  });
});

describe('MemberList — removing another member', () => {
  // The bug this closes: one click fired DELETE with no confirmation at all.
  it('does not fire the DELETE until the dialog is confirmed', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /remove sam ito/i }));

    expect(await screen.findByText(/Remove Sam Ito from this fleet\?/i)).toBeInTheDocument();
    expect(memberService.removeMember).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: /^remove$/i }));

    await waitFor(() => expect(memberService.removeMember).toHaveBeenCalledWith('f1', 'other'));
  });

  it('fires nothing when the dialog is cancelled', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /remove sam ito/i }));
    await userEvent.click(await screen.findByRole('button', { name: /cancel/i }));

    await waitFor(() =>
      expect(screen.queryByText(/Remove Sam Ito from this fleet\?/i)).not.toBeInTheDocument(),
    );
    expect(memberService.removeMember).not.toHaveBeenCalled();
  });
});

describe('MemberList — Make owner', () => {
  // FR-2.8.
  it('is offered to owners on non-owner rows and confirms before PATCHing', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /make sam ito an owner/i }));

    expect(await screen.findByText(/Make Sam Ito an owner\?/i)).toBeInTheDocument();
    expect(memberService.updateRole).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole('button', { name: /^make owner$/i }));

    await waitFor(() => expect(memberService.updateRole).toHaveBeenCalledWith('f1', 'other', 'owner'));
  });

  it('is hidden from non-owners and never offered on an owner row', async () => {
    seed(
      [membership('me', 'member'), membership('boss', 'owner')],
      [userRow('boss', 'Jane Doe', 'jane@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner={false} />);

    await screen.findByText('Jane Doe');
    expect(screen.queryByRole('button', { name: /make .* an owner/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^remove /i })).not.toBeInTheDocument();
  });
});

describe('MemberList — leaving', () => {
  // ux-flow state 1. Before this task a member had no way to leave at all.
  it('offers a plain leave confirmation to a member', async () => {
    seed([membership('me', 'member'), membership('boss', 'owner')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner={false} />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));

    expect(await screen.findByText(/Leave this fleet\?/i)).toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();

    // Scoped to the dialog: the row button is ALSO named "Leave", so an
    // unscoped query matches two elements and throws.
    const dialog = within(screen.getByRole('alertdialog'));
    await userEvent.click(dialog.getByRole('button', { name: /^leave$/i }));

    await waitFor(() => expect(memberService.removeMember).toHaveBeenCalledWith('f1', 'me'));
    expect(memberService.updateRole).not.toHaveBeenCalled();
  });

  // ux-flow state 2: an owner with a co-owner leaves without picking anyone.
  it('offers a plain leave confirmation to one of two owners', async () => {
    seed([membership('me', 'owner'), membership('co', 'owner')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));

    expect(await screen.findByText(/Leave this fleet\?/i)).toBeInTheDocument();
    expect(screen.queryByText(/only owner/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  });

  // ux-flow state 3 / FR-3.7. Without this the sole owner hits a 409 with
  // nothing they can do about it — the dead end this task exists to close.
  it('requires a successor before a sole owner can leave', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));

    expect(await screen.findByText(/only owner/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /transfer & leave/i })).toBeDisabled();
  });

  // FR-3.8 ordering: promote first, then delete.
  it('promotes the successor and then removes the leaver', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );
    const order: string[] = [];
    vi.mocked(memberService.updateRole).mockImplementation(async () => {
      order.push('patch');
      return membership('other', 'owner');
    });
    vi.mocked(memberService.removeMember).mockImplementation(async () => {
      order.push('delete');
    });

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));
    await userEvent.click(await screen.findByRole('combobox'));
    await userEvent.click(await screen.findByRole('option', { name: /Sam Ito/ }));
    await userEvent.click(screen.getByRole('button', { name: /transfer & leave/i }));

    await waitFor(() => expect(order).toEqual(['patch', 'delete']));
    expect(memberService.updateRole).toHaveBeenCalledWith('f1', 'other', 'owner');
    expect(memberService.removeMember).toHaveBeenCalledWith('f1', 'me');
  });

  // FR-3.8, the half that matters: a failed promote must NOT be followed by the
  // delete, or the fleet is orphaned. Sequencing this with awaits inside one
  // function is what makes that a property of control flow rather than of
  // callback wiring nobody re-reads.
  it('does not remove the leaver when the promote fails', async () => {
    seed(
      [membership('me', 'owner'), membership('other', 'member')],
      [userRow('other', 'Sam Ito', 'sam@example.com')],
    );
    vi.mocked(memberService.updateRole).mockRejectedValue(new Error('boom'));

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));
    await userEvent.click(await screen.findByRole('combobox'));
    await userEvent.click(await screen.findByRole('option', { name: /Sam Ito/ }));
    await userEvent.click(screen.getByRole('button', { name: /transfer & leave/i }));

    await waitFor(() => expect(memberService.updateRole).toHaveBeenCalled());
    expect(memberService.removeMember).not.toHaveBeenCalled();
  });

  // D8: the picker offers viewers too. Excluding them would create a SECOND
  // dead end — an owner whose only companion is a viewer would see an enabled
  // Leave button, an empty picker and a permanently disabled confirm.
  it('offers viewers as successors', async () => {
    seed(
      [membership('me', 'owner'), membership('watcher', 'viewer')],
      [userRow('watcher', 'Val Watcher', 'val@example.com')],
    );

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    await userEvent.click(await screen.findByRole('button', { name: /^leave$/i }));
    await userEvent.click(await screen.findByRole('combobox'));

    expect(await screen.findByRole('option', { name: /Val Watcher/ })).toBeInTheDocument();
  });

  // ux-flow state 4 / FR-3.10. The one case with no path forward: there is
  // nobody to hand the fleet to, and deleting a fleet is out of scope.
  it('disables Leave for a sole owner who is the only member, with an explanation', async () => {
    seed([membership('me', 'owner')], [userRow('me', 'Jane Doe', 'jane@example.com')]);

    renderWithProviders(<MemberList fleetId="f1" isOwner />);

    const leave = await screen.findByRole('button', { name: /^leave$/i });
    expect(leave).toBeDisabled();
    expect(screen.getByText(/only member of this fleet/i)).toBeInTheDocument();

    await userEvent.click(leave);
    expect(screen.queryByText(/Leave this fleet\?/i)).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run -w apps/web test -- MemberList`
Expected: FAIL — no names rendered, no dialogs, no Leave button for non-owners.

- [ ] **Step 3: Rewrite the component**

Replace `apps/web/src/components/features/settings/MemberList.tsx` entirely with:

```tsx
/**
 * MemberList — fleet members by name, with guarded membership actions.
 *
 * Three actions, each behind its own confirmation:
 *   - Remove {name}  — owners only, on other members' rows
 *   - Make {name} an owner — owners only, on non-owner rows
 *   - Leave — every member, on their own row
 *
 * The Leave action has four states (ux-flow.md). A sole owner with company must
 * name a successor before going; a sole owner who is ALSO the only member has
 * nobody to hand the fleet to and the button is disabled with an explanation.
 */
import { useMemo, useState } from 'react';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '../../ui/alert-dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../../ui/select';
import { useMembers, useRemoveMember, useUpdateMemberRole } from '../../../lib/hooks/api/members';
import { useUsers } from '../../../lib/hooks/api/users';
import { useAuth } from '../../../context/AuthContext';
import type { UserAttributes } from '../../../types/models/user';

interface MemberListProps {
  fleetId: string;
  isOwner: boolean;
}

/**
 * One dialog at a time, by construction: two dialogs cannot be open at once
 * because there is only one piece of state to hold them.
 */
type PendingAction =
  | { kind: 'remove'; userId: string; name: string }
  | { kind: 'promote'; userId: string; name: string }
  | { kind: 'leave' }
  | null;

/**
 * FR-1.5 fallback chain: displayName, then email, then the first 8 characters
 * of the id.
 *
 * `||` not `??`: auth-service marshals an unset displayName as "" (Attributes
 * are plain Go strings), and `??` would let the empty string through and render
 * a blank row.
 */
export function displayFor(userId: string, users?: Record<string, UserAttributes>): string {
  const u = users?.[userId];
  return u?.displayName || u?.email || userId.slice(0, 8);
}

export function MemberList({ fleetId, isOwner }: MemberListProps) {
  const { user } = useAuth();
  const { data: members, isLoading } = useMembers(fleetId);
  const memberIds = useMemo(
    () => (members ?? []).map((m) => m.attributes.userId),
    [members],
  );
  // A SECOND, independent query — not a select over useMembers. That is what
  // makes "memberships loaded, names failed" renderable (FR-1.7).
  const { data: users } = useUsers(memberIds);

  const removeMember = useRemoveMember(fleetId);
  const updateRole = useUpdateMemberRole(fleetId);

  const [pending, setPending] = useState<PendingAction>(null);
  const [successorId, setSuccessorId] = useState('');

  const activeMembers = members ?? [];
  const ownerCount = activeMembers.filter((m) => m.attributes.role === 'owner').length;
  const memberCount = activeMembers.length;
  // myRole comes from the MEMBERS LIST, not useAuth().role. The list is the
  // database; the auth role is a token claim that can be stale. The leave flow
  // is only correct if myRole and ownerCount agree, and they only agree if both
  // come from the same response.
  const myRole = activeMembers.find((m) => m.attributes.userId === user?.id)?.attributes.role;

  const soleOwner = myRole === 'owner' && ownerCount === 1;
  const needsSuccessor = soleOwner && memberCount > 1; // ux-flow state 3
  const leaveBlocked = soleOwner && memberCount === 1; // ux-flow state 4

  const successorOptions = activeMembers.filter((m) => m.attributes.userId !== user?.id);
  const busy = removeMember.isPending || updateRole.isPending;

  const closeDialog = () => {
    setPending(null);
    setSuccessorId('');
  };

  const confirmRemove = async (userId: string) => {
    try {
      await removeMember.mutateAsync({ userId, isSelf: false });
      closeDialog();
    } catch {
      // useRemoveMember surfaces the failure as a toast; leave the dialog open
      // so the user can retry or cancel.
    }
  };

  const confirmPromote = async (userId: string) => {
    try {
      await updateRole.mutateAsync({ userId, role: 'owner' });
      closeDialog();
    } catch {
      // Toasted by useUpdateMemberRole.
    }
  };

  /**
   * FR-3.8. One function with sequential awaits, NOT two mutations wired to each
   * other's onSuccess: "if the promote fails, the delete is not attempted" is
   * then a property of control flow rather than of callback wiring nobody
   * re-reads.
   *
   * If the promote succeeds and the delete fails, the fleet has two owners and
   * the user is still a member — a valid state, not a corruption. Reopening the
   * dialog lands in ux-flow state 2 (plain leave, no picker) and the retry
   * completes it.
   */
  const confirmLeave = async () => {
    if (!user) return;
    try {
      if (needsSuccessor) {
        await updateRole.mutateAsync({ userId: successorId, role: 'owner' });
      }
      await removeMember.mutateAsync({ userId: user.id, isSelf: true });
      closeDialog();
    } catch {
      // Toasted by the hooks. Navigation onward is not done here: invalidating
      // authKeys refetches /auth/me, which now reports no active fleet, and
      // RequireAuth redirects to onboarding on its own.
    }
  };

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  if (activeMembers.length === 0) {
    return <p className="text-sm text-muted-foreground">No members found.</p>;
  }

  return (
    <>
      <ul className="divide-y">
        {activeMembers.map((m) => {
          const userId = m.attributes.userId;
          const isSelf = userId === user?.id;
          const name = displayFor(userId, users);
          return (
            <li key={m.id} className="flex items-center justify-between gap-4 py-3">
              <div className="space-y-0.5">
                <div className="text-sm font-medium">
                  {name}
                  {isSelf && ' (you)'}
                </div>
                <div className="text-xs text-muted-foreground capitalize">{m.attributes.role}</div>
                {isSelf && leaveBlocked && (
                  <p className="text-sm text-muted-foreground">
                    You are the only member of this fleet, so there is nobody to hand it to.
                  </p>
                )}
              </div>

              <div className="flex shrink-0 items-center gap-2">
                {isSelf ? (
                  <Button
                    variant="destructive"
                    size="sm"
                    disabled={leaveBlocked || busy}
                    onClick={() => setPending({ kind: 'leave' })}
                  >
                    Leave
                  </Button>
                ) : (
                  isOwner && (
                    <>
                      {m.attributes.role !== 'owner' && (
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={`Make ${name} an owner`}
                          disabled={busy}
                          onClick={() => setPending({ kind: 'promote', userId, name })}
                        >
                          Make owner
                        </Button>
                      )}
                      <Button
                        variant="destructive"
                        size="sm"
                        aria-label={`Remove ${name}`}
                        disabled={busy}
                        onClick={() => setPending({ kind: 'remove', userId, name })}
                      >
                        Remove
                      </Button>
                    </>
                  )
                )}
              </div>
            </li>
          );
        })}
      </ul>

      <AlertDialog open={pending !== null} onOpenChange={(open) => !open && closeDialog()}>
        <AlertDialogContent>
          {pending?.kind === 'remove' && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Remove {pending.name} from this fleet?</AlertDialogTitle>
                <AlertDialogDescription>
                  They will immediately lose access to all of this fleet&apos;s vehicles,
                  maintenance records, and photos. You can invite them back later.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                  disabled={busy}
                  onClick={(e) => {
                    // Keep the dialog mounted while the request is in flight so
                    // a failure can be retried from it.
                    e.preventDefault();
                    void confirmRemove(pending.userId);
                  }}
                >
                  Remove
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}

          {pending?.kind === 'promote' && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Make {pending.name} an owner?</AlertDialogTitle>
                <AlertDialogDescription>
                  They will be able to invite and remove members, rename the fleet, and grant
                  ownership to others. You remain an owner.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  disabled={busy}
                  onClick={(e) => {
                    e.preventDefault();
                    void confirmPromote(pending.userId);
                  }}
                >
                  Make owner
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}

          {pending?.kind === 'leave' && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>Leave this fleet?</AlertDialogTitle>
                <AlertDialogDescription>
                  {needsSuccessor
                    ? 'You are the only owner. Choose who takes over before you go.'
                    : "You will lose access to all of this fleet's vehicles, maintenance records, and photos. Rejoining requires a new invite from an owner."}
                </AlertDialogDescription>
              </AlertDialogHeader>

              {needsSuccessor && (
                <div className="space-y-2">
                  <label className="text-sm font-medium" htmlFor="successor">
                    New owner
                  </label>
                  <Select value={successorId} onValueChange={setSuccessorId}>
                    <SelectTrigger id="successor">
                      <SelectValue placeholder="Select a member" />
                    </SelectTrigger>
                    <SelectContent>
                      {/* D8: viewers are offered too. Excluding them would leave
                          an owner whose only companion is a viewer with an empty
                          picker and no way out. */}
                      {successorOptions.map((m) => (
                        <SelectItem key={m.id} value={m.attributes.userId}>
                          {displayFor(m.attributes.userId, users)} — {m.attributes.role}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-sm text-muted-foreground">
                    You will lose access to all of this fleet&apos;s vehicles, maintenance records,
                    and photos. Rejoining requires a new invite.
                  </p>
                </div>
              )}

              <AlertDialogFooter>
                <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
                <AlertDialogAction
                  className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                  disabled={busy || (needsSuccessor && !successorId)}
                  onClick={(e) => {
                    e.preventDefault();
                    void confirmLeave();
                  }}
                >
                  {needsSuccessor ? 'Transfer & leave' : 'Leave'}
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- MemberList`
Expected: PASS — all fourteen tests.

If the Radix `Select` does not open under jsdom (it needs `PointerEvent` APIs jsdom lacks), add these polyfills to the top of the test file rather than weakening the assertion — the picker behaviour is FR-3.7 and must stay covered:

```ts
beforeAll(() => {
  window.HTMLElement.prototype.scrollIntoView = vi.fn();
  window.HTMLElement.prototype.hasPointerCapture = vi.fn();
  window.HTMLElement.prototype.releasePointerCapture = vi.fn();
});
```

(import `beforeAll` from `vitest`).

- [ ] **Step 5: Run the whole web suite, type-check and lint**

```bash
npm run -w apps/web test && npm run -w apps/web build && npm run -w apps/web lint
```
Expected: all green. `SettingsPage.tsx` needs no change — `MemberList`'s props are unchanged.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/components/features/settings/MemberList.tsx \
        apps/web/src/components/features/settings/MemberList.test.tsx
git commit -m "feat(web): show member names and guard membership actions"
```

---

### Task 11: Full verification

**Files:** none — this task only runs checks.

- [ ] **Step 1: Format the frontend**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run format
git diff --stat
```

Commit any formatting-only changes:

```bash
git add -A && git commit -m "style: prettier" || echo "nothing to format"
```

- [ ] **Step 2: Run the full CI target**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests` and `carfax-template` all pass. Paste the failing output into the fix rather than re-running blind if any step fails.

- [ ] **Step 3: Confirm the manifests are untouched**

This task changes no deployment manifest — the new auth route rides the existing `/api/auth` IngressRoute and `FLEET_SERVICE_URL` is already configured.

```bash
git diff --name-only main...HEAD -- deploy/
```
Expected: no output. If anything is listed, it was not asked for by this task — remove it.

- [ ] **Step 4: Verify the acceptance criteria have test coverage**

Cross-check `prd.md` §10 against the suites, then confirm the full set runs green:

```bash
go test ./apps/fleet-service/internal/membership/... ./apps/auth-service/... -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL))' | tail -60
npm run -w apps/web test
```

- [ ] **Step 5: Final commit if anything moved**

```bash
git status --short
git log --oneline main..HEAD
```

Expected: ten feature commits (plus any formatting commit), a clean tree, and `git rev-parse --show-toplevel` ending in `/.worktrees/task-014-member-names-ownership-transfer`.

---

## Post-Implementation

Before opening a PR, run the code-review step per CLAUDE.md:

```
superpowers:requesting-code-review
```

It dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer` (Go changed) and `frontend-guidelines-reviewer` (TS changed), which write to `docs/tasks/task-014-member-names-ownership-transfer/audit.md`.

**Known cross-branch reconciliation (do not resolve unilaterally):**

- `task-011-platform-admin-console` also defines `user.Provider.ListByIDs`. Same signature, no scoping inside — whoever merges second deletes their duplicate and keeps the other's.
- `task-012-vehicle-detail-redesign` adds `@radix-ui/react-popover` and `cmdk` to the same `package.json`. Different packages; a plain merge.
