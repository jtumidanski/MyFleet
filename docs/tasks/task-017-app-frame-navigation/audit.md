# Plan Audit — task-017-app-frame-navigation

**Plan Path:** docs/tasks/task-017-app-frame-navigation/plan.md
**Audit Date:** 2026-08-03
**Branch:** task-017-app-frame-navigation
**Base Branch:** main (merge-base `a4cdab59146ae33a3845e391ba1fc0d3b9a983a4`)
**Range audited:** `a4cdab5..5c54029` — 23 commits, 44 files, `apps/web` + `package-lock.json` + task docs only

## Executive Summary

All 16 plan tasks were implemented; none was silently skipped, stubbed, or deferred. Every
artifact the plan names exists at the path it names, with the behaviour it specifies, and
each is backed by tests that assert the behaviour rather than merely its presence. An
independent run of the branch's 19 affected test files gives 147/147 passing.

Three findings, all non-blocking. (1) One row of the plan's own Verification Checklist is
overstated — "Danger band renders unchanged with its tokens intact" credits Task 15's band
tests, which assert only text; no test asserts the band's token classes. The claim itself is
true (verified from the diff: the band is byte-identical to pre-branch), but it is proven by
inspection, not by the suite. (2) Every one of the plan's 96 step checkboxes is still
`- [ ]`; the plan was never marked off, so the document alone cannot tell a reader what was
done. (3) A parked cosmetic defect from the final fix wave — two test comments cite a
nonexistent `risks.md`.

Recommendation: **READY_TO_MERGE**. None of the three findings affects shipped behaviour.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | Dependencies + `--sidebar-*` token family | DONE | Three Radix deps under `dependencies` in `apps/web/package.json:17,22,25`; tokens in `apps/web/src/index.css:67-74` (light) and `:125-132` (dark); registered in `apps/web/tailwind.config.ts:76-85` with the `DEFAULT` key the shadcn source needs. Tests: `apps/web/src/test/sidebarTokens.test.ts:50-90` (8 tests). |
| 2 | `useIsMobile` hook | DONE | `apps/web/src/lib/hooks/useIsMobile.ts:18-32`, breakpoint 768. Tests `useIsMobile.test.ts:14-45` incl. the exactly-at-breakpoint boundary and unsubscribe-on-unmount. |
| 3 | Vendor `separator.tsx` / `tooltip.tsx` | DONE | `apps/web/src/components/ui/separator.tsx`, `tooltip.tsx`; tests at `separator.test.tsx:5-24`, `tooltip.test.tsx:13-31`. |
| 4 | Vendor `sidebar.tsx` with the cookie deviation | DONE | `apps/web/src/components/ui/sidebar.tsx` (735 lines). All three mandated deviations present and commented: `SidebarInset` as a `div` (`sidebar.tsx:1024-1030` — plan.md:1024 rationale carried in-file), `SidebarRail` `aria-hidden`, cookie **read** path. Tests `sidebar.test.tsx:35-110` prove the read by pre-setting the cookie and asserting derived state, not just the write. |
| 5 | Vendor `dropdown-menu.tsx` + Radix jsdom stubs | DONE | `apps/web/src/components/ui/dropdown-menu.tsx`; stubs appended at `apps/web/src/test/setup.ts:102-113` (`scrollIntoView`, `hasPointerCapture`, `setPointerCapture`, `releasePointerCapture`). Tests `dropdown-menu.test.tsx:32-90` cover click-open, Enter-open, ArrowDown, Escape + focus return. |
| 6 | Vendor `breadcrumb.tsx` | DONE | `apps/web/src/components/ui/breadcrumb.tsx`; both deviations commented in-file (`BreadcrumbPage` without `role="link"`; capitalised `aria-label="Breadcrumb"`, comment added in `4ef0ce3`). Tests `breadcrumb.test.tsx:28-51`. |
| 7 | Route→trail table | DONE | `apps/web/src/components/frame/breadcrumbTrails.ts:43-56` (12 rows), `resolveTrail` at `:72-78` using `matchPath` with `end: true` as designed. Tests `breadcrumbTrails.test.ts` — 12 per-route rows checked as `[label, to]` tuples against a hand-written literal table (`:38-104`), so a swapped target fails; plus null cases, rooting, and target-is-a-known-pattern invariants. |
| 8 | Resolving crumbs | DONE | `crumbs/VehicleNameCrumb.tsx:11-32`, `crumbs/FleetNameCrumb.tsx:10-23`. Three mandated states each (skeleton / name / raw id on failure). Source-pin test at `VehicleNameCrumb.test.tsx:85-100` reads both `VehicleDetailPage.tsx` and the crumb and asserts the identical title rule string. |
| 9 | `AppBreadcrumb` | DONE | `frame/AppBreadcrumb.tsx:36-69`. 8 tests at `AppBreadcrumb.test.tsx:24-71`, incl. renders-nothing outside both shells and last-crumb `aria-current`. |
| 10 | `identityLines` + `ProfileMenu` | DONE | `frame/identityLines.ts:18-25`; `frame/ProfileMenu.tsx:26-67`. 6 + 9 tests. Avatar-set, no-avatar-icon, and avatar-`onError`-falls-back-to-icon all covered (`ProfileMenu.test.tsx:85-119`). |
| 11 | `BrandLink` | DONE | `frame/BrandLink.tsx:25-43`. 5 tests incl. accessible name and the `/admin` target for the console (`BrandLink.test.tsx:20-66`). |
| 12 | `FrameNav` | DONE | `frame/FrameNav.tsx:26-68`; `FrameNavItem` defined once at `:11-17`. 7 tests incl. **both** directions of the `end`-flag failure mode plus a positive control (`FrameNav.test.tsx:48-67`). |
| 13 | `FrameHeader` | DONE | `frame/FrameHeader.tsx:19-30`, fixed `h-14`. Ordering test at `FrameHeader.test.tsx:54-83` uses `compareDocumentPosition` to pin the breadcrumb strictly between trigger and theme toggle — this was strengthened in `3a5cfd8` after the original button-role-only query was found blind to the breadcrumb. |
| 14 | Rewrite `AppLayout` | DONE | `components/AppLayout.tsx:40-63`. 13 tests. `platformAdmin` gate survives (`:41-42`, tests `AppLayout.test.tsx:186-201`); exactly-one-`<main>` asserted (`:171-176`); `p-6` asserted (`:162`). The Task-1 duplicate `bg-card` comment is gone — rationale now lives at `index.css:52-66`. |
| 15 | Rewrite `AdminLayout` | DONE | `components/admin/AdminLayout.tsx:36-91`. 12 tests. Danger band byte-identical to pre-branch (confirmed: the band `<div>` at `AdminLayout.tsx:80` does not appear as a changed line in `git diff a4cdab5..HEAD`). "Back to my fleet" → `/` asserted at `AdminLayout.test.tsx:108`. `postPurgeRouting.test.tsx` unmodified by the branch (confirmed absent from the diff's file list) and passes 3/3. |
| 16 | Full verification | DONE | Step 1 `make ci` PASS. Steps 3–6 driven in real Chromium in both themes, 45/45 sub-checks PASS, report at `.superpowers/sdd/plan/task-16-browser-report.md`. Step 7 code review ran per-task and whole-branch; step 8 fix wave landed as `5c54029`. |

**Completion Rate:** 16/16 tasks (100%)
**Skipped without approval:** 0
**Partial implementations:** 0

## Skipped / Deferred Tasks

None. Every task produced the artifacts its **Files** section names, and no task's output is
a stub — there are no `TODO`/`FIXME`/`XXX`/`HACK` markers anywhere in the new `frame/`,
`ui/sidebar.tsx`, `ui/dropdown-menu.tsx`, `ui/breadcrumb.tsx`, or `useIsMobile.ts`.

The five minors the execution ledger records as **deferred** were re-triaged here and all
five hold as non-blocking:

1. `sidebarTokens.test.ts` `block()` uses the first `}` as block terminator. Latent
   fragility only — both blocks it parses are flat custom-property lists.
2. Empty-name fallback in the crumbs. The ledger's argument against fixing here is sound:
   `VehicleAttributes.make/model/year` are non-optional and the fleet builder rejects empty
   names server-side, so both paths need a contract violation to reach; and
   `VehicleDetailPage`'s `<h1>` has identical exposure, so guarding the crumb alone would
   break the FR-CRUMBNAME-2 source pin at `VehicleNameCrumb.test.tsx:97`.
3. Both crumbs repeat a 2-line loading shell. Plan-mandated (FR-CRUMBNAME-7 keeps them
   separate for rules-of-hooks).
4. React Router v6→v7 future-flag stderr warnings. Confirmed pre-existing and repo-wide —
   they appear in this branch's test output but no test file on this branch introduced them.
5. `identityLines` missing the "display name present, email absent" case. **Now resolved** —
   `identityLines.test.ts:36` covers it, added in the final fix wave (`5c54029`).

## Verification Checklist Audit

Every row of the plan's own checklist (plan.md:4220-4255) was re-checked against source.
Thirty of the thirty-one rows hold. The exception:

| Row | Status | Finding |
|---|---|---|
| "Danger band renders unchanged with its tokens intact — Task 15 band tests" | **OVERSTATED** | `AdminLayout.test.tsx` has exactly one `toHaveClass` assertion and it is `p-6` (`:158`). The two band tests (`:71`, `:79`) assert only that the text "platform admin" and "up to 15 minutes" are present. Nothing in the suite asserts `bg-danger-subtle`, `border-danger-border`, or `text-danger-subtle-foreground`. The underlying *claim* is true — the band is byte-identical to pre-branch — but it is proven by diff inspection, not by a test, so a future change swapping the band to `--destructive` would go green. This is a third overstated row beyond the two caught during execution (Task 7's target-blind trail test, fixed in `c148143`; Task 13's breadcrumb-blind ordering test, fixed in `3a5cfd8`). |

Two rows warrant a note without being wrong:

- **"Active/hover distinct from the surface — Task 1 distinctness test (values)"** holds:
  `sidebarTokens.test.ts:71` asserts `--sidebar-accent !== --sidebar` in both themes, and the
  browser run measured `rgb(255,255,255)` vs `rgb(241,245,249)` (light) and `rgb(2,8,23)` vs
  `rgb(30,41,59)` (dark). Separately worth recording, because it reads as a surprise: the
  *sidebar surface itself* is not distinct from the page — `--sidebar` mirrors `--card`,
  which equals `--background` in both themes (`index.css:67` vs `:7`/`:9`). Only a 1px
  border separates sidebar from content. That is deliberate and documented at
  `index.css:52-66`; the browser verifier flagged it honestly rather than glossing it.
- **"No additional network request — Task 16 step 4"** holds, with one correction the
  verifier surfaced: `useAdminFleet`'s `staleTime` is 30s (`lib/hooks/api/admin.ts:56`), not
  the 60s the plan's step-4 text implies. `useVehicle` is 60s (`api/vehicles.ts:30`). The
  measured round-trip was sub-second, so the result is unaffected — but plan.md:4183's
  "within 60 seconds" framing is only literally true for the vehicle case.

## Build & Test Results

| Service | Build | Tests | Vet | Notes |
|---------|-------|-------|-----|-------|
| apps/web (this branch's 19 affected test files) | n/a | **PASS** 147/147 | n/a | Run independently by this audit: `npx vitest run --root apps/web` over `src/components/frame`, the five vendored `ui/*.test.tsx`, both layout tests, `postPurgeRouting.test.tsx`, `sidebarTokens.test.ts`, `useIsMobile.test.ts`. 19 files, 147 tests, 2.74s, zero failures. |
| Full repo (`make ci`) | PASS | PASS | PASS | Not re-run by this audit — a run was in flight. Prior recorded result: exit 0 across lint-check, vet, test, build, fe-test, fe-build, manifests, carfax-template; frontend suite 89 files / 720 tests; `format:check` clean repo-wide. Nothing found in this audit contradicts that. |
| Go services | n/a | n/a | n/a | No Go files changed — the diff is `apps/web`, `package-lock.json`, and task docs only. |
| deploy/k8s | n/a | n/a | n/a | No manifest changed. |

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE

The implementation matches the plan task-for-task. Where it deviates it does so upward and
with a recorded reason: the Task 14 test scopes `getByText('Vehicles')` to the breadcrumb
landmark because the brief's unscoped query became genuinely ambiguous once `FrameNav` and
`AppBreadcrumb` first rendered together — a strictly stronger assertion, and the same
scoping was correctly carried into Task 15 (`AdminLayout.test.tsx:126`). The cross-task
seams that a task-by-task execution is most likely to drop were all verified holding:
`App.tsx` is untouched (so `/admin` is still a route-tree sibling), and the twelve routes it
declares are pinned against `TRAILS` by a test that extracts them from `App.tsx` as source
(`breadcrumbTrails.test.ts:216-258`) with a count guard so it cannot pass vacuously.

## Action Items

None blocking. In descending order of value:

1. **Tighten or re-word the danger-band checklist row.** Either add a token assertion to
   `AdminLayout.test.tsx` (`expect(band).toHaveClass('bg-danger-subtle', 'border-danger-border',
   'text-danger-subtle-foreground')`) or amend plan.md:4252 to credit diff inspection rather
   than the band tests. The former is three lines and closes the gap permanently.
2. **Mark the plan off.** All 96 step checkboxes in plan.md are `- [ ]` and none is `- [x]`.
   The work is done; the document does not say so, which will mislead anyone who reads
   plan.md without the ledger beside it.
3. **Fix the two `risks.md` citations** in `AppLayout.test.tsx:34` and
   `AdminLayout.test.tsx:152`. No such file exists — the two-main-landmarks risk is
   plan.md:1029. Cosmetic, zero behavioural or coverage impact; parked during the fix wave
   and correctly so.
4. **Correct plan.md:4183's "within 60 seconds"** to note that the admin-fleet half of that
   check is governed by a 30s `staleTime`.

---

# Frontend Guidelines Audit (FE-*) — task-017-app-frame-navigation

- **Audit Scope:** `a4cdab59146ae33a3845e391ba1fc0d3b9a983a4..HEAD`, `apps/web` only — 39 files, 23 commits
- **Guidelines Source:** `frontend-dev-guidelines` skill (`.claude/skills/frontend-dev-guidelines/`)
- **Date:** 2026-08-03
- **Build:** PASS (not re-run by this auditor — recorded at `.superpowers/sdd/plan/progress.md:130`, `make ci` exit 0 after the fix wave)
- **Tests:** 89 files / 720 tests passing (`.superpowers/sdd/plan/progress.md:89`, `:124`)
- **Overall:** PASS

## Build & Test Results

Phase 1 was **not executed by this auditor** — the controller reported a `make ci`
re-run in flight and instructed against a concurrent run. Evidence relied upon:

- `.superpowers/sdd/plan/progress.md:87-89` — "`make ci` PASSES on this branch (controller ran directly, exit 0) — lint-check, vet, test, build, fe-test, fe-build … format:check clean repo-wide."
- `.superpowers/sdd/plan/progress.md:124` — "Suite 701 -> 720 tests."
- `.superpowers/sdd/plan/progress.md:130` — "`make ci` PASSES again, exit 0."

This is second-hand evidence. If the in-flight run reports otherwise, the overall
status flips to FAIL per the Phase 1 gate.

## File Inventory

**Component — first-party, new (`src/components/frame/`)** — the primary review target:

- `apps/web/src/components/frame/FrameHeader.tsx`
- `apps/web/src/components/frame/ProfileMenu.tsx`
- `apps/web/src/components/frame/BrandLink.tsx`
- `apps/web/src/components/frame/FrameNav.tsx`
- `apps/web/src/components/frame/AppBreadcrumb.tsx`
- `apps/web/src/components/frame/crumbs/VehicleNameCrumb.tsx`
- `apps/web/src/components/frame/crumbs/FleetNameCrumb.tsx`

**Other — first-party, new (pure modules under `frame/`)**

- `apps/web/src/components/frame/identityLines.ts`
- `apps/web/src/components/frame/breadcrumbTrails.ts`

**Component — first-party, rewritten**

- `apps/web/src/components/AppLayout.tsx`
- `apps/web/src/components/admin/AdminLayout.tsx`

**Component — vendored shadcn (`src/components/ui/`)**, judged as vendored code per
the documented decision in `context.md` §5:

- `apps/web/src/components/ui/sidebar.tsx`
- `apps/web/src/components/ui/dropdown-menu.tsx`
- `apps/web/src/components/ui/tooltip.tsx`
- `apps/web/src/components/ui/separator.tsx`
- `apps/web/src/components/ui/breadcrumb.tsx`

**Hook** — `apps/web/src/lib/hooks/useIsMobile.ts` (a UI hook, not a `lib/hooks/api/`
React Query hook — no query keys or services involved)

**Other** — `apps/web/src/index.css`, `apps/web/tailwind.config.ts`,
`apps/web/src/test/setup.ts`, `apps/web/package.json`, plus 15 `*.test.ts(x)` files.

**Not in scope (unchanged):** `services/api/`, `lib/schemas/`, `types/models/`,
`lib/hooks/api/`, `pages/`. No Service, Schema, or Type files changed on this branch.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grepped `: any`, `as any`, `<any>`, `any[]` across all 11 new/rewritten `.tsx` + `useIsMobile.ts` + `sidebarTokens.test.ts` — zero matches. Dynamic inputs are properly typed: `identityLines.ts:18` takes `User \| null \| undefined`; `sidebar.tsx:35` returns `boolean \| undefined`; the `React.CSSProperties` casts (`sidebar.tsx:144`, `:205`, `:656`) are the documented shadcn idiom for custom properties, not `any`. |
| FE-02 | No manual class concatenation | PASS | Grepped `className={"` and ``className={` `` across all new/rewritten files — zero matches. Every conditional class goes through `cn()`: `AppBreadcrumb.tsx:52` (`cn(!isLast && 'hidden sm:inline-flex')`), `sidebar.tsx:146`, `:231`, `:563`, `dropdown-menu.tsx:71`, `breadcrumb.tsx:22`, `separator.tsx:14`, `tooltip.tsx:26`. The only backtick in a non-test new file is `sidebar.tsx:39`, a cookie-name prefix, not a class string. |
| FE-03 | No direct API client calls in components | PASS | Grepped `lib/api/client` across `components/frame`, `AppLayout.tsx`, `AdminLayout.tsx` — zero matches. Data reaches the crumbs through the hook layer only: `VehicleNameCrumb.tsx:1` imports `useVehicle` from `lib/hooks/api/vehicles`; `FleetNameCrumb.tsx:1` imports `useAdminFleet` from `lib/hooks/api/admin`. |
| FE-04 | No inline Zod schemas in components | PASS | Grepped `z.object(`, `z.string(`, `from 'zod'` across all new/rewritten component files — zero matches. No form or validation code on this branch. |
| FE-05 | No spinners for content loading | PASS | Grepped `animate-spin` across all new/rewritten files — zero matches. The two loading states use skeleton styling: `VehicleNameCrumb.tsx:19` and `FleetNameCrumb.tsx:16` render `animate-pulse rounded-md bg-muted`, matching `ui/skeleton.tsx:5` verbatim. Both carry an in-file comment (`VehicleNameCrumb.tsx:14-17`) explaining why the classes are inlined rather than `<Skeleton>` used directly: `Skeleton` is a `<div>` and would nest invalidly inside `BreadcrumbPage`'s `<span>`. Correct behaviour, correct reason. |
| FE-06 | No hardcoded colors | PASS | Grepped the full Tailwind palette (`bg\|text\|border\|ring\|from\|to\|via\|fill\|stroke\|divide\|outline\|decoration\|shadow\|accent\|caret\|placeholder`-`white\|black\|slate\|gray\|…`) across all new/rewritten files — zero matches, **including the five vendored files**. Every surface uses a semantic token: `AdminLayout.tsx:80` uses `border-danger-border bg-danger-subtle text-danger-subtle-foreground`; `sidebar.tsx:255` uses `bg-sidebar`; `sidebar.tsx:513` uses `bg-sidebar-accent`. The new `--sidebar-*` family is registered as semantic Tailwind colours at `tailwind.config.ts:76-85` and defined in both themes in `index.css` (light `:52-72`, dark `:123-133`). Independently enforced repo-wide by `src/test/conventions.test.ts` ("no hardcoded palette classes"), whose allowlist was **not** widened by this branch — it still contains only the two pre-existing `dialog.tsx`/`sheet.tsx` scrim lines. |
| FE-07 | No state mutation | PASS | Grepped `.push(`, `.splice(`, `.sort(`, `.reverse(`, `.unshift(`, `.pop(`, `.shift(` across all new/rewritten non-test source — zero matches. `AppLayout.tsx:42` builds the conditional nav immutably: `const nav = platformAdmin ? [...NAV, ADMIN_ENTRY] : NAV;`. The only two `.push(` hits are `breadcrumbTrails.test.ts:233,238`, building a local array inside a test helper — not React state. |
| FE-08 | No default exports for components | PASS | Grepped `export default` across all new/rewritten `.tsx` + `useIsMobile.ts` — zero matches. All named: `FrameHeader.tsx:19`, `ProfileMenu.tsx:26`, `BrandLink.tsx:25`, `FrameNav.tsx:26`, `AppBreadcrumb.tsx:36`, `VehicleNameCrumb.tsx:11`, `FleetNameCrumb.tsx:10`, `AppLayout.tsx:40`, `AdminLayout.tsx:36`, `useIsMobile.ts:18`; the vendored files use terminal named `export { … }` blocks (`sidebar.tsx:710-735`, `breadcrumb.tsx:99-107`, `dropdown-menu.tsx:155-171`, `tooltip.tsx:34`, `separator.tsx:23`). |
| FE-09 | Error handling with `createErrorFromUnknown` | **WARN** | Grepped `.catch(` and `try {` across `components/frame`, `AppLayout.tsx`, `AdminLayout.tsx` — zero matches, so the letter of the check ("each catch uses `createErrorFromUnknown()`") is vacuously satisfied. But there is one unguarded async call: `ProfileMenu.tsx:64` — `onSelect={() => void logout()}`. `AuthContext.tsx:55-60` defines `logout` as `async () => { await logoutRequest(); … }` with no internal catch, so a failing `logoutRequest()` becomes an unhandled rejection with no toast and no error state. **Not a branch regression** — byte-identical to the pre-branch call sites at `AppLayout.tsx:61` and `AdminLayout.tsx:70` in `a4cdab5`; the code moved into `ProfileMenu` unchanged. Non-blocking; see Summary. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | No `types/models/` file changed on this branch, and every consumer respects `{ id, attributes }`. `identityLines.ts:1` imports `User` from `types/models/user`, defined at `types/models/user.ts:17` as `JsonApiResource<UserAttributes>`; access is `user?.attributes.displayName` / `.email` (`identityLines.ts:19-20`) and `user?.attributes.avatarUrl` (`ProfileMenu.tsx:31`). `VehicleNameCrumb.tsx:22,29` reads `data?.attributes` then `attributes.nickname/year/make/model`; `FleetNameCrumb.tsx:19,22` reads `data?.attributes` then `attributes.name`. No bare-field access anywhere. |
| FE-11 | Service extends `BaseService` | **N/A** | No file under `services/api/` is in the diff (`git diff --stat a4cdab5..HEAD -- apps/web` — 39 files, none under `services/`). |
| FE-12 | Query key factory uses `as const` | PASS | No new query key factory was introduced; the two crumbs deliberately reuse existing ones so React Query dedupes with the page (`VehicleNameCrumb.tsx:7-9`, `FleetNameCrumb.tsx:7-8`). Both reused factories are `as const`: `lib/hooks/api/vehicles.ts:15-21` (`detail: (id: string) => [...vehicleKeys.details(), id] as const`) and `lib/hooks/api/admin.ts:12-26` (`fleet: (id: string) => [...adminKeys.fleets(), 'detail', id] as const`). |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | **N/A** | No form component in the diff; grep for `useForm` across the changed files returns zero. |
| FE-14 | Schema in `lib/schemas/` with inferred type | **N/A** | No file under `lib/schemas/` is in the diff. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | Exactly one click handler exists across all first-party frame files (grep for `onClick\|onSelect\|role="button"\|onKeyDown` over `components/frame`, `AppLayout.tsx`, `AdminLayout.tsx`, excluding tests): `ProfileMenu.tsx:64`'s `DropdownMenuItem onSelect`. Radix renders that as a `div[role="menuitem"]` — a non-`<button>`/`<a>` interactive element, and therefore exactly the case FE-15 exists for. It is covered: `dropdown-menu.tsx:72` includes **`cursor-pointer`** in the item's base class string. Worth calling out — upstream shadcn ships `cursor-default` there, so this is an undocumented sixth deviation from upstream, in the compliant direction. Every other interactive surface resolves to a native element: `SidebarTrigger` (`FrameHeader.tsx:22`) renders `<Button>` (`sidebar.tsx:273`), whose CVA base carries `cursor-pointer` (`button.tsx:7`); the `DropdownMenuTrigger asChild` target is also a `<Button>` (`ProfileMenu.tsx:37`); and every navigation target is a React Router `<Link>` rendering `<a href>` — `FrameNav.tsx:56`, `BrandLink.tsx:30`, `AdminLayout.tsx:58`, `AppBreadcrumb.tsx:59` — which the UA stylesheet gives a pointer. Tailwind v3 preflight additionally sets `button, [role="button"] { cursor: pointer }` (`node_modules/tailwindcss/src/css/preflight.css:342-346`), so the non-`asChild` `SidebarMenuButton` / `SidebarRail` paths are covered too. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | Every new first-party module has a co-located test file, 1:1 with no gaps — `frame/AppBreadcrumb.test.tsx`, `frame/BrandLink.test.tsx`, `frame/FrameHeader.test.tsx`, `frame/FrameNav.test.tsx`, `frame/ProfileMenu.test.tsx`, `frame/breadcrumbTrails.test.ts`, `frame/identityLines.test.ts`, `frame/crumbs/FleetNameCrumb.test.tsx`, `frame/crumbs/VehicleNameCrumb.test.tsx`, `lib/hooks/useIsMobile.test.ts`, `test/sidebarTokens.test.ts`. Both rewritten layouts have their tests rewritten alongside (`AppLayout.test.tsx` +118, `admin/AdminLayout.test.tsx` +101 in the diffstat). Even the five vendored primitives each got a test (`ui/sidebar.test.tsx`, `ui/dropdown-menu.test.tsx`, `ui/breadcrumb.test.tsx`, `ui/tooltip.test.tsx`, `ui/separator.test.tsx`), which the checklist does not require. Suite grew 701 → 720 (`progress.md:124`). |
| FE-17 | Mocks updated when services changed | **N/A** | No service changed (see FE-11), and this repo has no `__mocks__/` directory anywhere under `apps/web` (`find . -name "__mocks__" -type d` → empty) — it mocks inline via `vi.mock`, which `context.md` §6 documents as the established style. Nothing to diff. |

## Summary

### Blocking (must fix)

None. All 13 applicable checks pass; 4 are N/A for lack of in-scope files.

### Non-Blocking (should fix)

- **FE-09 — `ProfileMenu.tsx:64` swallows a failed sign-out.** `void logout()` discards
  the promise, and `AuthContext.tsx:55-60` has no internal catch, so a network failure
  during `logoutRequest()` produces a silent unhandled rejection: no `toast.error()`, no
  `createErrorFromUnknown()`, and the user is left looking at a menu that appeared to do
  nothing. The fix is a `.catch` calling `createErrorFromUnknown(err, 'Failed to sign out')`
  surfaced via `toast.error()`. **Explicitly pre-existing** — identical at `AppLayout.tsx:61`
  and `AdminLayout.tsx:70` in `a4cdab5`; this branch relocated it without changing it.
  Consolidating both copies into one `ProfileMenu` has, if anything, made it a one-line fix
  instead of a two-line one.

### Observations (no action required)

- **`SidebarRail` shows a resize cursor, not a pointer.** `sidebar.tsx:316-317` sets
  `cursor-w-resize`/`cursor-e-resize`, yet `sidebar.tsx:312` wires only `onClick` — there is
  no drag handler, a mismatch the browser verification independently noticed
  (`progress.md:100-101`). Read strictly, FE-15's "every element that responds to a click
  must communicate clickability via `cursor-pointer`" is unmet. Not raised as a finding:
  this is verbatim upstream shadcn in a vendored file, the rail is `aria-hidden="true"` and
  `tabIndex={-1}` (`sidebar.tsx:310-311`) so it is a redundant pointer-only convenience
  duplicating the labelled `SidebarTrigger`, and `context.md` §5 makes keeping the vendored
  files diffable against upstream an explicit decision.
- **Title-case rule (`patterns-components.md:277-297`) — three labels are sentence case:**
  "Audit log" (`breadcrumbTrails.ts:28`, `AdminLayout.tsx:25`), "Back to my fleet"
  (`AdminLayout.tsx:57,60`), "Sign out" (`ProfileMenu.tsx:64`). All three are carried
  verbatim from the pre-branch shells (`a4cdab5:AdminLayout.tsx:13,59,71`), and "Audit log"
  matches the untouched page title at `pages/admin/AdminAuditPage.tsx:50`. Changing them
  here would make the crumb and the page heading disagree. Repo-wide copy pass, own task.
- **Documented decisions verified as documented, not flagged:** relative imports throughout
  (no `@/` alias — `context.md` §3); inert `animate-in`/`fade-in-0`/`zoom-in-95` classes in
  `tooltip.tsx:26` and `dropdown-menu.tsx:39,56` with `tailwindcss-animate` uninstalled,
  flagged in-file at `tooltip.tsx:15-18`; the five upstream deviations each commented at
  their site (`sidebar.tsx:22-34` cookie read, `sidebar.tsx:293-300` rail `aria-hidden`,
  `sidebar.tsx:330-336` `SidebarInset` as `<div>`, `breadcrumb.tsx:55-61` `BreadcrumbPage`,
  `breadcrumb.tsx:6-12` capitalised `aria-label`).
- **Landmark hygiene confirmed by hand:** exactly one `<main>` per shell
  (`AppLayout.tsx:57`, `AdminLayout.tsx:85`), made possible by rendering `SidebarInset` as a
  `<div>` (`sidebar.tsx:337-339`); `FrameNav.tsx:36` adds the `<nav>` the primitive omits,
  with a per-shell `aria-label` ("Main" `AppLayout.tsx:51`, "Admin" `AdminLayout.tsx:44`) so
  the two shells' landmarks stay distinguishable from the breadcrumb's own
  `aria-label="Breadcrumb"` (`breadcrumb.tsx:14`).
