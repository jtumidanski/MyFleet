# Frontend Audit — task-011-platform-admin-console

- **Audit Scope:** 37 changed `.ts`/`.tsx` files in `apps/web/src`, range `f959cdc..7ba674a`
- **Guidelines Source:** `frontend-dev-guidelines` skill
- **Date:** 2026-08-02
- **Build:** PASS
- **Tests:** 236 passed, 0 failed (42 files)
- **Overall:** NEEDS-WORK

## Build & Test Results

```
vite v5.4.21 building for production...
✓ 1813 modules transformed.
✓ built in 4.48s
```

```
Test Files  42 passed (42)
     Tests  236 passed (236)
  Duration  3.73s
```

Note: `npm test -- --watchAll=false` fails — that is a Jest flag and this project
runs Vitest (`CACError: Unknown option --watchAll`). The correct gate is `npm test`.
Not a code defect; the audit checklist's command is stale for this repo.

## File Inventory

**Pages (5)** — `pages/admin/`: `AdminOverviewPage.tsx`, `AdminFleetsPage.tsx`,
`AdminPurgesPage.tsx`, `AdminAuditPage.tsx`, `AdminUsersPage.tsx` (+ 5 `.test.tsx`)

**Components (7)** — `components/admin/`: `RequirePlatformAdmin.tsx`, `AdminLayout.tsx`,
`BlastRadiusPanel.tsx`, `PurgeConfirmDialog.tsx` (+ 4 `.test.tsx`, + `postPurgeRouting.test.tsx`);
`components/AppLayout.tsx` (modified)

**UI primitives (3)** — `components/ui/`: `dialog.tsx`, `table.tsx`, `badge.tsx` (+ `badge.test.tsx`)

**Hooks (2)** — `lib/hooks/api/admin.ts` (+ `.test.ts`), `lib/hooks/api/auth.ts` (modified)

**Service (1)** — `services/api/AdminService.ts`

**Types (2)** — `types/models/admin.ts`, `types/models/user.ts` (modified)

**Other (2)** — `App.tsx` (route branch), `context/AuthContext.tsx` (modified),
`lib/admin/purgeStatus.ts` (+ `.test.ts`)

**Schemas:** none added (no forms in this feature).

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grep `: any`/`as any`/`<any>` over all 37 files — zero matches |
| FE-02 | No manual class concatenation | PASS | Zero matches; `cn()` used — `badge.tsx:37`, `dialog.tsx:42`, `AdminLayout.tsx:42` |
| FE-03 | No direct API client in components | PASS | Only `services/api/AdminService.ts:2` imports `apiClient`; zero imports in `pages/`/`components/` |
| FE-04 | No inline Zod schemas | PASS | Zero `z.object(` / `from "zod"` in scope (no forms added) |
| FE-05 | No spinners for content loading | PASS | Zero `animate-spin`; Skeleton used at `RequirePlatformAdmin.tsx:29`, `AdminOverviewPage.tsx:109-117`, `AdminFleetsPage.tsx:126-130,178-180`, `AdminPurgesPage.tsx:84-88`, `AdminAuditPage.tsx:69-73`, `AdminUsersPage.tsx:42-46` |
| FE-06 | No hardcoded colors | PASS | Zero palette-class matches. Tokens verified defined in BOTH themes: `index.css:33-48` (`:root`) and `index.css:78-93` (`.dark`) |
| FE-07 | No state mutation | PASS | Only `.sort()` is `BlastRadiusPanel.tsx:82-84`, applied to a fresh array from `Object.keys().filter()` |
| FE-08 | No default exports | PASS | Zero `export default` in scope |
| FE-09 | Error handling w/ `createErrorFromUnknown` | PASS (1 defect) | `admin.ts:114,144,169` each pair `createErrorFromUnknown` with `toast.error`. Defect: unhandled rejection at `AdminOverviewPage.tsx:97`+`:228` (see F-5) |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | Verified field-by-field vs Go — see "JSON:API Verification" below |
| FE-11 | Service pattern | PASS | `AdminService.ts:49-186` uses the documented direct-API-client pattern; singleton exported `:186` |
| FE-12 | Query key factory `as const` | PASS | `admin.ts:11-25` — every member terminates in `as const` |
| FE-13 | Forms use RHF + zodResolver | N/A | No forms added |
| FE-14 | Schema in `lib/schemas/` w/ inferred type | N/A | No schemas added |

### JSON:API Verification (field-by-field vs Go)

Checked `types/models/admin.ts` against `apps/fleet-service/internal/admin/rest.go`,
`browse.go` and `stats.go`. **All names and nullabilities match.**

| TS interface | Go source | Result |
|---|---|---|
| `AdminStatsAttributes:25-39` | `rest.go:23-37` | 13/13 match; all `*int` → `number \| null` |
| `AdminVehicleCounts:15-18` | `stats.go:19-22` | `active`, `pending_purge` match |
| `AdminFleetAttributes:41-54` | `rest.go:63-73` | 9/9 match; `*time.Time` → `string \| null` |
| `AdminFleetDetailAttributes:85-92` | `rest.go:103-110` | Go embeds `fleetAttributes` (flattened in JSON); TS `extends` — correct |
| `AdminMemberRow:56-64` | `browse.go:76-83` | 6/6 match |
| `AdminVehicleRow:66-76` | `browse.go:86-95` | 8/8 match |
| `AdminInviteRow:78-83` | `browse.go:98-103` | 4/4 match |
| `AdminUserAttributes:133-139` | `rest.go:129-135` | 5/5 match |
| `AdminUserFleetRow:127-131` | `browse.go:134-138` | 3/3 match |
| `PurgeOperationAttributes:94-109` | `rest.go:156-170` | 13/13 match; note `purge_after` is non-pointer in Go → `string` in TS (correct, unlike the nullable one on the fleet) |
| `AuditEventAttributes:113-125` | `rest.go:204-216` | 11/11 match |

Vocabulary also verified: `PurgeStatus:11` vs `entity.go:14-17`; `PurgeScope:12` vs
`manifest.go:18-20`; `AuditAction:111` vs `entity.go:73-76`. All exact.

`platformAdmin` meta key verified camelCase: `auth.ts:28` reads `doc.meta?.platformAdmin`,
and `apps/auth-service/internal/user/resource_test.go:126` asserts the wire fragment
`"platformAdmin":true`. Match.

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | Every `onClick` in the feature is on `<Button type="button">`; `button.tsx:7` includes `cursor-pointer` in the CVA base. Navigation uses `<Link>` (`AdminLayout.tsx:55`, `AdminFleetsPage.tsx:141,194`) which is a native `<a>`. No `TableRow`/`div` has a handler; no `DialogTrigger`/`PopoverTrigger` exists in the feature |

### Dark-mode legibility (scrutinised statically)

`badge.tsx:22-25` uses the `-subtle` / `-subtle-foreground` / `-border` token trio rather
than the bare `--danger`/`--success`/etc. This is the correct choice: the bare tokens are
sized for TEXT on `--background`, and on dark `--danger` sits at `0 90.6% 70.8%`
(`index.css:86`) — the 400 band — which would be illegible as a chip fill.

The subtle trio inverts correctly across themes:

| Token | Light (`index.css`) | Dark (`index.css`) |
|---|---|---|
| `--danger-subtle` / `-foreground` | `0 93.3% 94.1%` : `0 70% 35.3%` (`:42-43`) | `0 45% 15%` : `0 90% 80%` (`:87-88`) |
| `--warning-subtle` / `-foreground` | `48 96.5% 88.8%` : `22.7 82.5% 31.4%` (`:38-39`) | `30 45% 14%` : `43 90% 76%` (`:83-84`) |
| `--success-subtle` / `-foreground` | `140.6 84.2% 92.5%` : `142.8 64.2% 24.1%` (`:34-35`) | `142 40% 14%` : `142 70% 78%` (`:79-80`) |
| `--info-subtle` / `-foreground` | `214.3 94.6% 92.7%` : `226 70.7% 40.2%` (`:46-47`) | `217 45% 17%` : `213 92% 80%` (`:91-92`) |

Every pair keeps a lightness gap ≥ 55 points in both themes. All four are mapped in
`tailwind.config.ts:53-73`. No token referenced by `badge.tsx` is undefined. PASS.

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS (gaps) | 5/5 pages, 4/4 admin components, `badge.tsx`, `admin.ts`, `purgeStatus.ts` all have tests. Missing: `AdminService.ts`, `dialog.tsx`, `table.tsx` (see F-8) |
| FE-17 | Mocks updated when services changed | N/A | Repo has no `__mocks__/` directory; tests use inline `vi.mock` (e.g. `postPurgeRouting.test.tsx:29-36`) |

---

## Findings

### F-1 (CRITICAL) — The operator's typed confirmation is never sent to the server

The purge dialog gates its confirm button on an exact match of what the operator typed
(`PurgeConfirmDialog.tsx:89`, `const matches = typed === confirmationPhrase;` — correctly
no trim, no case fold). But `typed` is component-local state (`:77`) and is **never
passed out**. `onConfirm` takes no argument (`:42`), and both call sites send the
*expected* phrase instead of the typed one:

- `AdminFleetsPage.tsx:349` — `{ scope: 'fleet', target_type: 'fleet', target_id: id, confirmation: fleet.name }`
- `AdminOverviewPage.tsx:97` — `createPurge.mutateAsync({ scope: 'system', confirmation: SYSTEM_CONFIRMATION })`

The server gate is real and exact — `confirmation.go:30-46` (`MatchConfirmation`, returns
`ErrConfirmationMismatch` → 409) invoked at `processor.go:137`. But because the client
always transmits the correct string, **that 409 can never fire from a user typo**. The
409 branch at `admin.ts:118` is unreachable via the UI.

The consequence is that the disabled button is not a courtesy backed by a server control —
it is the *only* control, and it is client-side. Anything that bypasses the button state
(a React DevTools prop edit, a replayed request, a future refactor that calls
`createPurge.mutate` from elsewhere) reaches an irreversible system purge with no
server-side phrase check standing in the way. Defence-in-depth for the highest-stakes
control in the product is nullified.

Three code comments assert the opposite of the shipped behaviour and should be corrected
alongside the fix: `PurgeConfirmDialog.tsx:28-29` ("The server's 409 on a mismatched
phrase is the actual control"), `AdminService.ts:124-126`, and `processor.go:136`.

**Fix:** change `onConfirm` to `(confirmation: string) => void`, call
`onConfirm(typed)` at `PurgeConfirmDialog.tsx:170`, and have both pages forward that
value into `confirmation`. Add a test asserting the mutation receives the *typed* string —
no current test checks the transmitted `confirmation` value.

### F-2 (IMPORTANT) — A null user count renders as "This affects 0 people."

`AdminOverviewPage.tsx:223` — `peopleCount={stats.users ?? 0}`. `stats.users` is
`number | null` (`types/models/admin.ts:34`), where null means "auth-service could not be
asked", not zero. `PurgeConfirmDialog.tsx:109` then renders
`<p className="font-medium">This affects {peopleCount} people.</p>` → **"This affects 0
people."** on the confirmation screen for an irreversible platform-wide purge.

This is the stated em-dash requirement (FR-ADMIN-UI-6, documented at
`types/models/admin.ts:20-24`) violated at the single most consequential render site in
the feature. It also directly contradicts the sibling helper 18 lines below it:
`systemCounts` at `AdminOverviewPage.tsx:241-246` gets the identical decision right
(`if (typeof v === 'number') out[key] = v;`) with a comment (`:237-239`) explaining that
showing 0 for an unreachable source "would understate what is about to be deleted".

**Fix:** widen `peopleCount` at `PurgeConfirmDialog.tsx:39` to `number | null` and render
an em dash or "an unknown number of people" when null.

Every *other* nullable render site is correct — notably the strict-null form at
`AdminOverviewPage.tsx:70` (`value === null || value === undefined`) and `:76`
(`{unavailable ? '—' : value}`), which correctly keeps a real `0` as `0`.

### F-3 (IMPORTANT) — `postPurgeRouting.test.tsx` cannot catch the regression it names

The test's docstring (`postPurgeRouting.test.tsx:12-15`) states its purpose as guarding
"risks.md R5's residual risk: a future refactor renesting /admin under RequireAuth. That
would compile, pass every other test..." — but the test **rebuilds the route tree by hand**
(`:53-86`) rather than importing `App`, as it concedes at `:21-22`.

So if someone moves the `/admin` route in `App.tsx` inside the `RequireAuth` block, this
test still constructs its own sibling tree and still passes. It would pass every other
test too. The named regression is therefore unguarded.

To its credit the test is **not vacuous** about the property it *does* test: the control
assertion at `:119-122` renders the same fleetless-admin identity through `RequireAuth`
and asserts it *is* redirected to `/onboarding`, which proves the exemption is real rather
than the redirect being absent from the tree. That part is sound.

The structure it asserts does currently hold: `App.tsx:72-86` declares `/admin` as a
sibling of the `RequireAuth`/`AppLayout` block that closes at `App.tsx:62`, wrapped only
in `RequirePlatformAdmin` (`:75-77`). `RequirePlatformAdmin.tsx:24-41` requires
`isAuthenticated` and `platformAdmin` and never reads `activeFleetId`.

**Fix:** add one assertion that imports the real `App` (or the exported route array) and
asserts `/admin` is not a descendant of `RequireAuth` — e.g. render `App` at
`/admin/purges` with a fleetless admin identity and assert no `/onboarding` redirect.

### F-4 (IMPORTANT) — Missing empty states on the fleet detail tables

`AdminFleetsPage.tsx:237-267` (members) and `:281-297` (vehicles) render
`<TableBody>{fleet.vehicles.map(...)}</TableBody>` with no length guard. A fleet with zero
vehicles — an ordinary state for a newly created fleet — shows the
`Vehicle / Mileage / Status` header row over a completely empty body with no message.

The correct pattern already exists in this changeset at `AdminUsersPage.tsx:71-72`
(`{a.fleets.length === 0 ? <span className="text-sm text-muted-foreground">None</span> : ...}`).
Invites avoid the problem differently and acceptably by hiding the whole card
(`AdminFleetsPage.tsx:301`).

### F-5 (IMPORTANT) — Unhandled promise rejection on a failed system purge

`AdminOverviewPage.tsx:97` awaits `createPurge.mutateAsync(...)`; `:228` invokes the
wrapper as `onConfirm={() => void confirmSystemPurge()}`. `mutateAsync` rejects on
failure regardless of the hook's `onError`, and `void` discards the promise with no
`.catch`. A failed system purge therefore produces an unhandled rejection.

User-visible behaviour is not broken — the toast still fires from `admin.ts:118-124` and
the dialog correctly stays open because `setConfirmOpen(false)` at `:106` is never
reached — so this is console noise rather than a silent failure. `AdminFleetsPage.tsx:347-352`
already models the clean alternative (`mutate` with an `onSuccess` callback).

### F-6 (MINOR) — Empty-state copy ignores the active filter

`AdminPurgesPage.tsx:94` renders "No purges yet." and `AdminAuditPage.tsx:79` renders
"No audit events yet." unconditionally, while both pages have active filter state
(`AdminPurgesPage.tsx:45`, `AdminAuditPage.tsx:42`). Filtering to a status with no matches
on a platform that *has* run purges misreports a filtered-to-empty result as an empty
dataset.

### F-7 (MINOR) — `counts` type is stricter than the runtime guards assume

`types/models/admin.ts:90` declares `counts: Record<string, number>` (non-optional), but
`AdminFleetsPage.tsx:327` passes `error={!fleet.counts}` and `:343` passes
`counts={fleet.counts ?? {}}` — both guarding an `undefined` the type says cannot occur.
Either the type should be optional or the guards are dead code. It fails safe today:
`BlastRadiusPanel.tsx:62-76` withholds the purge button entirely rather than showing zeros.

### F-8 (MINOR) — Untested modules

No test files for `services/api/AdminService.ts`, `components/ui/dialog.tsx`, or
`components/ui/table.tsx`. `dialog.tsx` and `table.tsx` are thin Radix/markup wrappers and
are reasonably covered transitively. `AdminService.ts` has real logic worth pinning —
`pageParams` (`:39-42`) encodes the `page[number]`/`page[size]` convention its own
docstring (`:24-25`) flags as a correction to the PRD's sketch.

---

## Positive Findings Worth Recording

- **Dialog a11y is sound.** `dialog.tsx:14-56` builds on `@radix-ui/react-dialog`, which
  supplies focus trapping, focus restoration to the trigger, Escape-to-close and
  `aria-modal`. `PurgeConfirmDialog.tsx:100` uses `DialogTitle` and `:101`
  `DialogDescription`, so Radix wires `aria-labelledby`/`aria-describedby` automatically.
  The input is labelled via `Label htmlFor="purge-confirmation"` / `Input id=...`
  (`:152-154`). Nothing hand-rolled.
- **Exact-match gating is correct and tested** at the component level:
  `PurgeConfirmDialog.tsx:89` plus tests "keeps confirm unavailable until the phrase
  matches exactly" (`PurgeConfirmDialog.test.tsx:22`) and "does not accept a phrase with
  trailing whitespace" (`:40`). The box is also cleared on open (`:82-84`) so a second
  purge in a session cannot start with the button already live. The gap is F-1 —
  transmission, not gating.
- **Mutations invalidate on settle, not success** — `admin.ts:110-112`, `:140-142`,
  `:165-167` all use `onSettled`, correctly reasoned in the docstring at `:100-105`
  (a create that errors may still have stamped locally and marked the operation partial).
- **staleTime near destructive controls is deliberately short** — 30s for stats/fleets/
  users/audit (`admin.ts:38,47,56,64,93`), 15s for the purge queue (`:72`).
  `usePurgeOperation` (`:76-82`) sets none and so inherits React Query's default of 0 —
  always-stale, refetch on mount — which is the *safest* setting here, not a gap;
  `AppProviders.tsx:34-41` sets no default `staleTime`.
- **Post-system-purge cache handling is considered** — `AdminOverviewPage.tsx:101`
  clears rather than invalidates (avoiding a frame of stale fleet data) and `:105`
  refetches `/auth/me` so the shell reflects the now-null `activeFleetId` immediately.

## Summary

### Blocking (must fix)
- **F-1 (FE-09 / correctness):** typed confirmation never transmitted; server's exact-match
  409 gate is unreachable from the UI. `PurgeConfirmDialog.tsx:42,89,170`;
  `AdminFleetsPage.tsx:349`; `AdminOverviewPage.tsx:97`.
- **F-2 (FE-10 / FR-ADMIN-UI-6):** `peopleCount={stats.users ?? 0}` renders "This affects
  0 people." for a null count. `AdminOverviewPage.tsx:223`; `PurgeConfirmDialog.tsx:39,109`.

### Non-Blocking (should fix)
- **F-3 (FE-16):** `postPurgeRouting.test.tsx:53-86` mirrors the route tree instead of
  importing `App`, so it cannot catch the renesting regression its docstring names.
- **F-4 (FE-16 / empty states):** no empty-state guard on the members and vehicles tables.
  `AdminFleetsPage.tsx:237-267,281-297`.
- **F-5 (FE-09):** unhandled promise rejection. `AdminOverviewPage.tsx:97,228`.
- **F-6:** filter-blind empty copy. `AdminPurgesPage.tsx:94`; `AdminAuditPage.tsx:79`.
- **F-7:** `counts` type vs. runtime guards. `types/models/admin.ts:90`;
  `AdminFleetsPage.tsx:327,343`.
- **F-8 (FE-16):** no tests for `AdminService.ts`, `dialog.tsx`, `table.tsx`.
