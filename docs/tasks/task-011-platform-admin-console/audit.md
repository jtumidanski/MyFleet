# task-011 — code review

Three reviewers were dispatched in parallel over the full branch
(`f959cdc..7ba674a`, 42 commits): plan adherence, backend guidelines (DOM/SUB/SEC)
and frontend guidelines (FE-*). Their full reports are in `audit-plan-adherence.md`,
`audit-backend.md` and `audit-frontend.md`.

All three returned **NEEDS-WORK**. Every Critical and Important finding below was
reproduced before being fixed, and the fix verified afterwards.

## Critical — all fixed

| # | Finding | Fix |
|---|---|---|
| C1 | A cancelled purge was reaped anyway. A failed downstream restore left `status = partial`, and `ListDue` selected `pending, partial` — so the reaper destroyed the data the operator had asked to keep, marked the operation `reaped`, and thereafter answered "permanently deleted" while local rows sat intact. | `cancelled_at` is now stamped when a cancel is **requested**, before any work, and `ListDue` excludes it. Intent is recorded separately from outcome. |
| C2 | `Retry` re-purged a partially-cancelled operation — its guard rejected only `reaped`/`cancelled`, and a partial cancel is `partial`. | Guard now checks `CancelledAt()`, not just status. |
| F-1 | The operator's typed confirmation never reached the server: `onConfirm` took no argument and both call sites sent the *expected* phrase. The server's exact comparison could never fire, so the client-side disabled button was the only real gate — the inverse of the stated design. | `onConfirm(typed)` forwards the actual keystrokes; both call sites pass them through. Test asserts the transmitted value. |

C1 and C2 shared one root cause: `partial` meant both "purge partly applied" and
"cancel partly applied". Both came from the plan, which specified the cancel status
rule and `ListDue`'s predicate in different tasks and never reconciled them.

## Important — all fixed

| # | Finding | Fix |
|---|---|---|
| I1 | A media row spared because its bytes could not be removed was spared by NULLing `purge_operation_id` — which is also the key the retry uses. The row and its bytes were stranded permanently. | `ReapSparing` excludes the row from the DELETE while it **keeps** its operation id. |
| I2 | The `internal-deny` rules existed only in `overlays/main`. `local` rendered zero and compose had none, so both dev environments exposed the unauthenticated destructive routes — notification-service genuinely reachable, since its stripprefix removes the full prefix. | Middleware + four priority-200 rules added to the local overlay; equivalent routers added to compose. |
| I3 | `resource.go` reached past the processor into `proc.d.Provider` for three reads. | Added `ListOperations`, `GetOperation`, `ListAuditEvents` on the processor. |
| F-2 | `peopleCount={stats.users ?? 0}` rendered "This affects 0 people" when auth-service was unreachable — the em-dash-not-zero rule broken at the most consequential render site. | `peopleCount` is nullable and renders "an unknown number of people". |
| F-3 / P-1 | The R5 residual-risk test rebuilt App.tsx's route tree by hand, so renesting `/admin` under `RequireAuth` would not have failed it. | `AppRoutes` is exported and the test mounts the **real** tree. Verified by renesting in `App.tsx`, which now fails it. |
| F-4 | Members/vehicles tables rendered bare headers when empty. | Empty-state guards. |
| F-5 | `void confirmSystemPurge()` could reject unhandled. | Rejection caught; the mutation already toasts. |
| P-3 | FR-ADMIN-FLEET-6 requires the platform-admin flag on `/admin/users`; it was omitted, and the stated reason was wrong — `auth.platform_admins` and `auth.users` share a schema, so one LEFT JOIN supplies it. The `created` column had also been dropped undisclosed. | Flag supplied by a LEFT JOIN matching `Provider.IsAdmin` exactly, threaded through to a badge. `created` column restored. |
| P-4 | Owner-email search was specified, commented as implemented, and advertised in the UI placeholder — but absent, and unimplementable as designed (the SQL has already narrowed to name matches before the auth lookup). | Code, comment and placeholder now agree on fleet-name search. A real owner-email search needs a search endpoint on auth-service; noted as outstanding. |
| P-5 | The audit `?actor=` filter was supported end to end but hardcoded to `''` in the page. | Filter input wired up. |

## Undisclosed deviations the reviewers caught

Five changes I had not reported: the `RequireSameFleet(` call-only match in the
arch test (necessary, but the most substantive change to that assertion), the
dropped `created` column, the absent owner-email search, the dead actor filter,
and — separately — my claim that the R5 probe proved the test guarded `App.tsx`,
which it did not.

## Accepted as-is

- **Manifest arch tests walk only files named `entity.go`.** Three real tables
  escape (`media.processed_events`, `notification.processed_events`, `outbox`) and
  their `excludedTables` entries are never exercised. Faithful to the plan;
  inherited plan-level flaw, not an execution failure.
- **`TestAdminTreeIsSeparate` walks only `fleet-service/internal`.** shared-go and
  the other three services are unguarded by it.
- **`adminclient` widening** opens a narrow hole: a handler placed in that package
  could read `PlatformAdmin` unnoticed. It holds no handlers today.

## Still not verified

Two of the PRD's manual acceptance checks remain unverified — no stack could be
brought up (no `.env`, and the OIDC flow needs real Google credentials):

1. An actually-stopped notification-service returning 200 from `/admin/stats`.
   Covered at unit level only.
2. Legibility in both themes. Tokens were reviewed statically and found sound —
   `--danger` at 70.8% lightness on dark would be illegible as a fill, which is why
   the `-subtle` trio is correct — but nothing has been looked at in a browser.

The third ("a system purge leaves accounts intact") was converted from a manual
check into `TestNoManifestReachesTheAuthSchema`.
