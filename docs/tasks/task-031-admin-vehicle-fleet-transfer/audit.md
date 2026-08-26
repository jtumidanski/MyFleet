# Task 031 — Admin Vehicle Fleet Transfer: Audit

Branch `task-031-admin-vehicle-fleet-transfer`, 28 commits from `e7ef18e` to `9fbf7e1`.

Executed via `superpowers:subagent-driven-development`: 16 planned tasks, each with a fresh
implementer and an independent scoped review, then a whole-branch review and one fix wave.

## Verification

All gates verified at `9fbf7e1`:

| Gate | Result |
|---|---|
| `make ci` | PASS (exit 0) |
| `kustomize build overlays/local` | PASS |
| `kustomize build overlays/main` | PASS |
| main `kubectl apply --dry-run=server` | PASS |
| local `kubectl apply --dry-run=server` | PASS |

`check-manifests.sh` confirms `internal-deny` at priority 200 on **both** entrypoints for
auth/fleet/media/notification. This feature added `/internal/admin/reassign-fleet` to
media-service and notification-service, but both already exposed `/internal/*`, so no new
service joined that set and no deploy change was required.

## Final whole-branch review

Five passes over the 15,770-line branch diff. Result: **0 Critical, 3 Important, 4 Minor**.
All three Important were fixed in a single wave (`980ef9a`, `c805321`, `7844975`, `9fbf7e1`)
and the scoped re-review verdicted all four ADDRESSED, merge readiness **Ready**.

### Important findings, fixed

1. **Compensation was keyed on "returned success", not "was issued."** `done.media`/`done.notif`
   were set only after a `Reassign` returned without error. A transport failure occurring *after*
   the downstream had committed its UPDATE left the flag false, so no reversal was issued and data
   was stranded on the destination fleet while the operator was told "nothing was moved" — the exact
   split-brain design D4 exists to prevent. Now recorded as intent-to-call, before the call. Both
   endpoints are idempotent, so reversing a move that never happened is a harmless no-op.
2. **The two new activity event types rendered as an unlabelled "Event" 📋.** `vehicle.transferred_out`
   and `vehicle.transferred_in` had no `EVENT_META` entries, so in both fleets' user-facing feeds the
   source fleet's only record that the car left read "Event". No task in the plan owned
   `activityEventMeta.ts`, which is why fifteen scoped reviews all passed.
3. **media-service's reassign had no source-fleet predicate**, moving any named media id regardless
   of current owner. Combined with the pre-existing unchecked `mediaId` on media attach, this turned
   a leaked UUID into a read-access grant. Now gated on the owning fleet.

A promoted Minor was also fixed: a nil reassigner silently skipped, producing a *committed* transfer
with data left on the source fleet, reported as `0`, with no error anywhere.

## Residual findings — open, for the human

**R-1 — the compensating reversal has no ownership predicate.** `transfer_processor.go:610` reverses
with `WHERE id IN ? AND fleet_id = <dest>`. If a fleet-a member attaches a media UUID owned by fleet-b
and an admin transfers that vehicle a→b, the forward call correctly refuses (the object is not in
fleet-a) but a downstream failure triggers a compensation that *does* match it, moving fleet-b's object
into fleet-a. **Not a regression** — the pre-fix reversal had the identical outcome — but fix 1 makes
compensation fire on transport failures too, so the path is reachable more often than before.

**R-2 — `apps/media-service/internal/vehiclemedia/resource.go` remains open.**
`POST /vehicles/{id}/media` still accepts an arbitrary `mediaId` with no ownership check. This is the
root cause of both Important 3 and R-1; the source-fleet predicate is defence in depth, not a repair.
Deliberately out of scope for this task.

## Product note

**The widget prune currently matches zero rows in production.** The `config.vehicleId` key is fixed by
design D5, but `apps/web/src/types/models/dashboard.ts:16` types widget config as an open
`Record<string, unknown>` and nothing in the web app writes `vehicleId` today. The code is correct and
dormant; it activates the moment a widget type starts pinning vehicles.

## Deferred minors — all triaged CAN SHIP

Reassign counts are destination read-backs rather than rows-moved (corrected honestly to the operator
via `meta.count_semantics`); `members.test.ts` mocks `createErrorFromUnknown` itself so certifies
nothing about the real conversion; `ACTIONS`/`ACTION_LABELS` are hand-maintained parallel lists (both
independently pinned by tests); the audit page records the source/destination fleets but does not
display them (FR-XFER-AUDIT-4 only requires recoverability); the media reassign's `IN ?` is unbounded;
`transferAttributes(r)` is a positional struct conversion.

## Rulings taken on the human's behalf

Twenty-two rulings were made during execution rather than blocking. The full list, with cost-if-wrong
for each, is in the session handoff. The load-bearing ones:

- **R15** — `validate()` now rejects a same-fleet spec. A same-fleet transfer would have hard-deleted
  that fleet's widgets pinned to the vehicle. Task 9 rejects it earlier with a 422, so the guard should
  never surface; it sits at the destructive call site as defence in depth.
- **R18** — ratified an out-of-brief change to shared `server.WriteError`. The plan's premise
  ("WriteError carries the detail either way") was false: it had no 5xx branch at all, so the 503
  rollback sentences never reached the console. Scoped to 503 only; 500 redaction intact; verified
  against all 43 `Detailed(` call sites.
- **R19 / R22** — the transfer response returns an envelope exposing `meta`, and the dialog does not
  auto-close on success. Both exist so the honest count sentence actually reaches the screen.
- **R20** — added tests for branches across the app that the `createErrorFromUnknown` fix made live.
- **R14** — a source category shadowing a system category is copied, not reused, to preserve the
  source category's own `description`.

## Plan defects found during execution

Five defects in `plan.md` would have shipped broken code or failed to compile:

1. Task 8's test helper `post` collided with an existing function in the same package — the package
   would not have compiled.
2. Tasks 14 and 15 called test helpers (`renderPage`, `mockEvents`) that do not exist; one would have
   thrown a TypeError by rendering with no `useAuditEvents` mock.
3. Task 13's brief nested the destination picker inside the `preview.data` branch, but the preview hook
   is gated on a destination being chosen — the picker would have been unreachable in production. The
   brief's own tests hid it because the mock returned data unconditionally.
4. `count_semantics` does not exist on the preview response, so the dialog as specified could never
   have displayed it.
5. Six reviewer-grade defects were mandated by the plan text: two dead statements existing only to
   justify unused imports, a swallowed insert error, two vacuous assertions, and a `!=` comparison on
   `any` that panics on a non-nil BLOB.
