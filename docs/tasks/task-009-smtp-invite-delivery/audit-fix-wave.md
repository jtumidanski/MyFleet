# task-009-smtp-invite-delivery — code review fix wave

Follow-up commits addressing four findings from the formal code review of
branch `task-009-smtp-invite-delivery`. The PR was already open; these land
as additional commits on the same branch.

## Finding 1 (blocking) — `Resend` write-path TOCTOU

**File:** `apps/fleet-service/internal/invite/administrator.go`, `Resend`
method (originally lines 90–125).

**Change:**
- `apps/fleet-service/internal/invite/administrator.go:103` — the UPDATE's
  `WHERE` clause now reads `id = ? AND accepted_at IS NULL` instead of just
  `id = ?`. This closes the race where a concurrent `Accept` lands between
  the handler's `proc.GetByID` read and the `Resend` transaction: the
  handler's own `inv.AcceptedAt() != nil` check only sees the stale
  pre-transaction read and cannot catch it.
- `apps/fleet-service/internal/invite/administrator.go:129` — `res.RowsAffected`
  is now checked. When it is `0` (row deleted, or accepted concurrently — the
  two are indistinguishable from `RowsAffected` alone), the method returns
  `server.ErrConflict` **before** building the fabricated `updated` Model and
  **before** calling the `emitCreated` emitter, so no rotated token and no
  `invite.created` event are produced for a row that either no longer exists
  or is already claimed.
- **Error mapping decision:** `server.ErrConflict` (409), not
  `server.ErrNotFound` (404). Documented inline at
  `apps/fleet-service/internal/invite/administrator.go:118-127`: the handler
  already returns 409 for the accepted-invite case it *can* see
  (`inv.AcceptedAt() != nil`, checked in `resource.go` just before calling
  `Resend`), so mapping the race that slips past that check to the same 409
  keeps the two paths consistent, and avoids asserting "this ID never
  existed" for a row that may still be sitting there, just accepted. A
  client that reissues after a fresh `GET` will see 404 if the row is truly
  gone.

**Tests added** (`apps/fleet-service/internal/invite/administrator_test.go`):
- `TestResend_deletedRowDoesNotRotateOrEmit` — inserts an invite, deletes it
  via `adm.Delete`, then calls `adm.Resend` with the stale pre-delete Model.
  Asserts: `errors.Is(err, server.ErrConflict)`, the emitter was not invoked
  (`events == 0`), the invite table has 0 rows, and the outbox table still
  has exactly 1 row (only the one from `Insert` — `Resend` added none).
- `TestResend_concurrentlyAcceptedDoesNotRotateOrEmit` — inserts an invite,
  stamps `accepted_at` directly on the row (simulating a concurrent
  `Accept`), then calls `adm.Resend` with the pre-accept Model (which still
  reports `AcceptedAt() == nil`). Asserts: `errors.Is(err, server.ErrConflict)`,
  the emitter was not invoked, `tok-1` (the original token) still resolves,
  `tok-2` (the would-be rotated token) does **not** resolve
  (`errors.Is(err, ErrNotFound)`), and the outbox still has exactly 1 row.

Both reuse the existing `newInviteDB`, `newInvite`, `countRows` helpers; no
new test infrastructure was added.

## Finding 2 (blocking) — no HTTP-layer tests for the invite resource

**File created:** `apps/fleet-service/internal/invite/resource_test.go`.

Built an `httptest` + chi router harness (`inviteTestRouter`) mounting the
real `InitializeRoutes` over the in-memory sqlite DB from `newInviteDB`, with
`OwnerChecker` stubbed via the existing `stubOwnerChecker{}` from
`internal_test.go` (same package). Authenticated requests are built with
`auth.WithIdentity(ctx, auth.Identity{...})`, following the pattern already
used by `apps/media-service/internal/mediaobject/resource_test.go`'s
`memberRequest` and `apps/auth-service/internal/user/resource_test.go` — no
new production seams were added.

All three named behaviours were reached:

1. **Cross-fleet 404** — `TestResend_crossFleetInviteIs404NotForbidden`.
   Creates a real invite in fleet `f1`, then calls resend as an owner of
   fleet `f2` naming their own fleet in the path (the "path-pair mismatch"
   case in `resource.go`). Asserts the response is 404, and that it is
   **byte-for-byte identical** to the 404 returned for a genuinely
   nonexistent invite ID requested the same way — pinning the
   "indistinguishable from nonexistent" property the review called out.
2. **429 rate-limit wiring**, both routes:
   - `TestCreateInvite_perFleetLimitIs429` — `Limits{CreatePerWindow: 1, ...}`,
     first create 201s, second (same fleet) 429s.
   - `TestResendInvite_cooldownIs429` — `Limits{ResendCooldown: time.Hour, ...}`,
     an immediate resend right after create (whose `updated_at` was just
     stamped) 429s.
3. **422 validation wiring** — `TestCreateInvite_malformedEmailIs422`. Posts
   a display-name-wrapped address (`"Bob <bob@example.com>"`), which
   `ValidateInviteEmail` rejects because the raw input doesn't equal
   `mail.ParseAddress`'s parsed addr-spec exactly; asserts 422.

Nothing was left uncovered — all three behaviours named in the finding are
exercised at the HTTP layer, and no production code was changed to make
this possible.

## Finding 3 — no error logging in the invite resource

**File:** `apps/fleet-service/internal/invite/resource.go`. Logging added at
every branch that can carry an *unexpected* (backend) failure, matching the
`log.WithError(err).Error("<action>")` idiom from
`apps/fleet-service/internal/fuel/resource.go`, plus `trace_id` /
`invite_id` / `fleet_id` / `user_id` fields via `logrus.Fields` (the
correlation id is captured once per handler via
`telemetry.CorrelationIDFromContext(req.Context())`, mirroring how
`vehicle/resource.go:97` does it).

Log sites added (file:line refers to the `.Error(...)` call):

| Handler | Line | Guarded against logging on |
|---|---|---|
| create | `resource.go:75` "check invite owner" | `server.ErrForbidden` (routine 403) |
| create | `resource.go:105` "check invite create limit" | `server.ErrTooManyRequests` (routine 429) |
| create | `resource.go:116` "generate invite token" | always logs (crypto/rand failure is never routine) |
| create | `resource.go:135` "create invite" | always logs (administrator/DB failure) |
| list | `resource.go:153` "list invites" | always logs (pure DB read, no business-error branch here) |
| delete | `resource.go:171` "get invite" | `server.ErrNotFound` (routine 404) short-circuits before this |
| delete | `resource.go:192` "check invite owner" | `server.ErrForbidden` |
| delete | `resource.go:202` "delete invite" | always logs |
| resend | `resource.go:225` "get invite" | `server.ErrNotFound` |
| resend | `resource.go:258` "check invite owner" | `server.ErrForbidden` |
| resend | `resource.go:282` "generate invite token" | always logs |
| resend | `resource.go:296` "resend invite" | `server.ErrConflict` (the Finding-1 TOCTOU race is now a routine outcome) |
| accept | `resource.go:320` "get invite by token" | `server.ErrNotFound` |
| accept | `resource.go:337` "accept invite" | always logs |

Routine client-caused branches left **unlogged** by design (validation
failures, the two `authz.RequireSameFleet`/`authz.RequireOwner` fast-path
gates, the `inv.FleetID() != fleetID` path-pair-mismatch 404, the accepted-
before-cooldown 409, and the resend cooldown 429 — none of these can carry
an unexpected backend error, only a deterministic business decision).

**Token-safety constraint honored:** no log call anywhere in the diff
includes the invite token, the whole `Model`, or the whole request struct —
only `invite_id`, `fleet_id`, `user_id`, and `trace_id` string fields. The
one branch with no invite id available (`accept`'s token-lookup failure at
`resource.go:320`) logs only `trace_id`, deliberately omitting the token
that would otherwise be the only identifying value in scope. This mirrors
the existing mechanical guarantee enforced elsewhere on the branch by
`apps/notification-service/internal/mailconsumer/consume_test.go`'s
`TestHandle_neverLogsTheToken` — no equivalent test was added in this
package since the finding did not ask for one and no log call here has a
route to the token in the first place (unlike the mail consumer, which
handles the token value directly).

## Finding 4 (trivial) — button label casing

**File:** `apps/web/src/components/features/settings/InviteList.tsx:70`.
Changed `Copy link` → `Copy Link` to match the title case of the sibling
`Resend` / `Revoke` buttons.

**Test file checked:** `apps/web/src/components/features/settings/InviteList.test.tsx`
uses the case-insensitive matcher `screen.getByRole('button', { name: /copy link/i })`
in three places (lines 57, 70, 94, 102) — no change needed there; verified
by re-running the frontend test suite (see below).

## Verification

All commands run from
`/home/tumidanski/source/MyFleet/.worktrees/task-009-smtp-invite-delivery`.

```
$ go build github.com/jtumidanski/myfleet/...
(no output — success)

$ go vet github.com/jtumidanski/myfleet/...
(no output — success)

$ go test github.com/jtumidanski/myfleet/apps/fleet-service/... -v
...
--- PASS: TestInsert_commitsInviteAndOutboxTogether (0.00s)
--- PASS: TestInsert_rollsBackWhenEmitFails (0.00s)
--- PASS: TestResend_rotatesTokenAndEmitsFreshEvent (0.00s)
--- PASS: TestResend_rollsBackWhenEmitFails (0.00s)
--- PASS: TestResend_deletedRowDoesNotRotateOrEmit (0.00s)
--- PASS: TestResend_concurrentlyAcceptedDoesNotRotateOrEmit (0.00s)
--- PASS: TestInternalGetInvite_returnsTokenAndFleetName (0.00s)
--- PASS: TestInternalGetInvite_unknownIDIs404 (0.00s)
--- PASS: TestInternalGetInvite_returnsAcceptedInvites (0.00s)
--- PASS: TestInternalGetInvite_missingFleetDegradesToEmptyName (0.00s)
--- PASS: TestInternalRouteAbsentFromJWTTree (0.00s)
--- PASS: TestAccept_rejectsEmailMismatch (0.00s)
--- PASS: TestAccept_rejectsExpired (0.00s)
--- PASS: TestAccept_rejectsAlreadyAccepted (0.00s)
--- PASS: TestAccept_okWhenValid (0.00s)
--- PASS: TestValidateInviteEmail (0.00s)
--- PASS: TestCheckCreateLimit (0.00s)
--- PASS: TestCheckResendCooldown (0.00s)
--- PASS: TestMake_carriesUpdatedAt (0.00s)
--- PASS: TestResend_crossFleetInviteIs404NotForbidden (0.00s)
--- PASS: TestCreateInvite_perFleetLimitIs429 (0.00s)
--- PASS: TestResendInvite_cooldownIs429 (0.00s)
--- PASS: TestCreateInvite_malformedEmailIs422 (0.00s)
ok  	github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite	0.023s
... (all other apps/fleet-service/internal/* packages also PASS)
PASS — 82 tests, 0 failures across apps/fleet-service/...

$ export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && make ci
Now using node v22.22.2 (npm v10.9.7)
./tools/lint.sh --check         → 0 issues (x6 packages)
prettier --check                → All matched files use Prettier code style!
eslint src --max-warnings 0     → OK
go vet github.com/jtumidanski/myfleet/...   → OK
go test -race github.com/jtumidanski/myfleet/...   → all ok (incl. apps/fleet-service/internal/invite)
go build github.com/jtumidanski/myfleet/...  → OK
npm run -w apps/web test        → Test Files 31 passed (31), Tests 188 passed (188)
npm run -w packages/shared-ts test → Test Files 2 passed (2), Tests 7 passed (7)
npm run -w apps/web build       → built in 4.94s
./tools/check-manifests.sh      → manifest checks passed (main overlay: no PVC/Secret/ClusterRole/placeholders;
                                    no local-only mail config; IngressRoute route-set parity: 7/7 routes,
                                    internal-deny present at priority 200 on both entrypoints)
./tools/check-carfax-template.sh → carfax template check passed

$ echo $?   (captured separately as MAKE_CI_EXIT via PIPESTATUS-safe redirection)
MAKE_CI_EXIT:0
```

`make ci` exited 0.

## Nothing was skipped

All four findings were addressed with production-code fixes plus tests
(Findings 1, 2, 4) or logging-only changes (Finding 3, which is
observability, not behavior — verified against the existing test suite,
which continued to pass unchanged). No new production-code seams were added
purely for testability; Finding 2's HTTP-layer harness reuses
`auth.WithIdentity` and `stubOwnerChecker`, both of which already existed on
this branch for other tests in the same package.
