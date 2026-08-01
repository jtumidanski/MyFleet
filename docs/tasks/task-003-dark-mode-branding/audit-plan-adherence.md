# Plan Audit — task-003-dark-mode-branding

**Plan Path:** `docs/tasks/task-003-dark-mode-branding/plan.md`
**Audit Date:** 2026-07-31
**Branch:** `task-003-dark-mode-branding`
**Base Branch:** `main` (diff range `352e8c1..41c5d3b`, 25 commits, 60 files)
**Scope:** plan adherence only — read-only, no files mutated other than this report.

## Executive Summary

All 17 tasks in `plan.md` were implemented. Every file the plan's File Structure
table names exists with the content the plan specified, every named symbol is
present, and every test the plan wrote out verbatim is in the tree and enumerated
below. Task 17 is `PARTIAL` by design: its steps 4–5 need a browser, were not
executed, and the checklist is recorded at
`docs/tasks/task-003-dark-mode-branding/manual-verification.md`. Nothing was
silently skipped, stubbed, or deferred without a record.

No Critical or Important findings. Two Low findings, both cosmetic/process. The
branch ships several things beyond the plan — all additive test coverage and one
accessibility fix — itemised in "Shipped beyond the plan" below.

## Task Completion

| # | Task | Status | Evidence |
|---|---|---|---|
| 1 | Theme allow-list | DONE | `apps/auth-service/internal/user/theme.go:7-18` (constants + `IsValidTheme`); test `theme_test.go:5-26`, 7 subtests, matches plan verbatim |
| 2 | Persist preference on user record | DONE | `model.go:13` (`ErrInvalidTheme`), `model.go:22,31,43-46`; `entity.go:19` (column), `entity.go:29-38` (`Make` normalisation), `entity.go:40-42` (`ToEntity`); `builder.go:13,20`; test DDL `provider_test.go:44`; tests `entity_test.go:8-69` (5 tests as specified) |
| 3 | `Processor.UpdateTheme` | DONE | `processor.go:54-63` — validate-before-read exactly as planned; tests `processor_test.go:131-178` (3 tests, 6 subtests) |
| 4 | Expose `themePreference` on the resource | DONE | `rest.go:11,15`; harness generalised at `resource_test.go:31-42` (`newAuthRouter`) and `:46-62` (`serve`); FR-TEST-3 regression `resource_test.go:93-117` |
| 5 | `PATCH /auth/me` | DONE | `resource.go:28-29` (`errThemeValidation` wrapping `server.ErrValidation`), `resource.go:76-102` (route on the JWT group, no user identifier in path/body); tests `resource_test.go:119-197` (4 tests incl. the 422 matrix and the no-echo title check) |
| 6 | Semantic status tokens | DONE | `apps/web/src/index.css:26-49` (light) and `:72-94` (dark) — all 32 values byte-identical to the plan; `tailwind.config.ts:51-75`; `types/models/user.ts:5-6,15`; `contrast.md` created. Independently confirmed the classes actually compile: `.bg-danger-subtle`, `.text-danger-subtle-foreground`, `.border-danger-border`, `.text-success`, `.text-warning`, `.bg-{success,warning,info}-subtle`, `.text-info-subtle-foreground` all present in `apps/web/dist/assets/index-5mURdli4.css` |
| 7 | Pure theme helpers | DONE | `src/lib/theme.ts:13,18,22-24,34-41,44-52,58-61,68-70` — all 7 exports; tests `theme.test.ts`, 11 cases matching the plan's 11 |
| 8 | `ThemeContext` | DONE | `src/context/ThemeContext.tsx:43-101` (`hasLocalOverride` ref at `:55`, unconditional media subscription at `:66-71`); `matchMedia` stub `src/test/setup.ts:34-84` incl. `setPrefersDark`/`resetMatchMedia`/`mediaListenerCount`; tests `ThemeContext.test.tsx`, 9 cases matching the plan's 9 |
| 9 | `useUpdateTheme` | DONE | `src/lib/hooks/api/auth.ts:65-74` (token-absent resolves `null`, no request) and `:89-106` (`onMutate` cache write, **no** `onError` rollback, as required by FR-PERSIST-5); tests `auth.test.ts`, 3 cases |
| 10 | `ThemeToggle` | DONE | `src/components/ThemeToggle.tsx:9-13` (cycle), `:17-21` (icon keyed on preference), `:40-47` (state change + mutation + toast), `:50-59` (`variant="ghost"`, `aria-label`, `title`); tests `ThemeToggle.test.tsx`, 5 cases |
| 11 | `ThemeSync` and the provider tree | DONE | `src/components/ThemeSync.tsx:14-51` (validates the wire value at `:36`, clears the override on sign-out at `:40-48`, renders `null`); tree `AppProviders.tsx:44-54` matches the required nesting exactly, `ThemedToaster` at `:17-20`; tests `ThemeSync.test.tsx`, 5 cases (plan specified 4 — see additions below) |
| 12 | Pre-paint script + first guard test | DONE | `apps/web/index.html:33-42` — synchronous, inline, `try/catch`, narrows to `system`, class name is a literal, placed before `type="module"` at `:46`; guards `conventions.test.ts:16-69`, 6 cases (plan specified 5) |
| 13 | Icon generator | DONE | `tools/generate-icons.py` (matches plan text apart from the documented budget-scoping fix in `e1628c5`); `tools/generate-icons.sh` matches plan text **byte-for-byte**; all six assets committed under `apps/web/public/` totalling 37,245 B (36.4 KiB) against the 100 KB budget; `src/components/brandMarkPath.ts:6-7` generated and its `d` string appears verbatim in `public/favicon.svg:6` |
| 14 | `BrandMark`, manifest, `<head>` wiring | DONE | `index.html:7-10` (icon/alternate/apple-touch/manifest links) and `:18-19` (both `theme-color` metas); `public/site.webmanifest` incl. the maskable entry; `src/components/BrandMark.tsx:15-21` (`currentColor`, `aria-hidden`, no hardcoded px); guards `conventions.test.ts:74-108` |
| 15 | `AppLayout` — tokens, brand mark, toggle | DONE | `src/components/AppLayout.tsx:28` (`bg-card` sidebar with the plan's rationale comment retained), `:30` (`<BrandMark className="h-5 w-5" />`), `:43-44` (`bg-accent` active / hover), `:59` (`<ThemeToggle />` immediately left of Sign out), `:60` (`variant="outline"` Button). No palette classes in the file |
| 16 | Convert remaining hardcoded colours | DONE | All 8 files converted: `PlaceholderPage.tsx:7`, `ActivityEventIcon.tsx:15`, `SeverityChip.tsx:10-24`, `MaintenanceQueueView.tsx:35,38,47,55,60,75`, `FleetOverviewWidget.tsx:27-40`, `OverdueMaintenanceWidget.tsx:28`, `UpcomingMaintenanceWidget.tsx:28`, `packages/ui-components/src/StatusBadge.tsx:3-13`. Guard `conventions.test.ts:113-154`. The stale "no shadcn semantic equivalent" comment FR-CONVERT-3 required removing is gone (`SeverityChip.tsx:10-12`) |
| 17 | Full verification | PARTIAL *(by design)* | Steps 1–3 and 6 done: `make ci` green, container serves all seven asset paths with 200 + non-HTML content types, asset budget 37,245 < 102,400 B, `git status` clean. `CLAUDE.md` correctly left unchanged (plan says skip unless a build command changed — none did). Steps 4–5 need a browser, were **not** executed, and are recorded as a checklist at `manual-verification.md:1-116` |

**Completion Rate:** 17/17 tasks (100%); 16 `DONE`, 1 `PARTIAL` by design
**Skipped without approval:** 0
**Partial implementations:** 1 (Task 17, steps 4–5, deferred with a recorded checklist)

## Skipped / Deferred Tasks

**Task 17, steps 4 (visual pass) and 5 (cross-device persistence check)** — not
executed; they require a browser and a running backend. `manual-verification.md`
records both as an explicit, per-item checklist with pass/fail criteria. This is
the only outstanding gate on the branch. Impact: the token conversions, the
Radix `select` dark-mode surface, the toast theming (FR-3P-1/2/3), and the
cross-profile persistence proof rest on reasoning and unit tests rather than on
observation. Nothing else in the plan depends on them.

## Accepted Deviations — Verified as Claimed

Each of the eight deviations supplied with this audit was checked against source;
all eight are what they claim to be.

1. **422 not 400.** `resource.go:28-29` wraps `server.ErrValidation`;
   `resource_test.go:157-162` asserts 422 for all four bad-input shapes.
   `packages/shared-go` is absent from the branch diff — unmodified as stated.
2. **`administrator.go` unchanged.** Absent from the diff; `administrator.go:24-30`
   already does `db.Save(&e)` and `ToEntity` (`entity.go:41`) carries the column.
3. **`ThemeSync` bridges.** `ThemeSync.tsx:14-51`; neither `AuthContext` nor
   `ThemeContext` imports the other.
4. **`NewBuilder()` seeds the default.** `builder.go:13`; pinned by
   `entity_test.go:65-69`.
5. **`generate-icons.sh` prefers Python.** Script matches the plan's text exactly;
   the rationale comment (`generate-icons.sh:6-13`) explains the inversion of
   design §8.2.
6. **Sidebar uses `bg-card`.** `AppLayout.tsx:28`, with the plan's "do not
   simplify back to `bg-muted`" comment preserved at `:23-27`.
7. **`@types/node` devDependency.** `apps/web/package.json` devDependencies;
   type-only, required by `conventions.test.ts:1-3`.
8. **Task 17 manual pass deferred.** `manual-verification.md`.

## Shipped Beyond the Plan

All additive, none contradicting the plan. Listed for completeness, not as findings.

| Addition | Location | Note |
|---|---|---|
| `AppLayout.test.tsx` (4 tests) | `apps/web/src/components/AppLayout.test.tsx` | Plan wrote no test for Task 15 |
| `AppProviders.test.tsx` (1 test) | `apps/web/src/components/providers/AppProviders.test.tsx:26-56` | Renders the real provider tree end to end; catches a tree reordering that `ThemeSync.test.tsx` cannot, because that file mocks `useAuth` |
| Extra `ThemeSync` sign-out test | `ThemeSync.test.tsx:104-143` | Exercises the real signed-in → signed-out → signed-in transition |
| `MEDIA_QUERY` drift guard | `ThemeContext.tsx:22`, `conventions.test.ts:63-68` | Also replaces the test's hardcoded `'myfleet.theme'` with an import of `THEME_STORAGE_KEY` (`conventions.test.ts:5,20,25`), closing a gap where a storage-key rename would have silently reinstated the flash |
| Empty-root guard in the palette scan | `conventions.test.ts:132-139` | Prevents a moved directory from making the FR-CONVERT-10 assertion pass trivially |
| AA fix: overdue-row body text | `MaintenanceQueueView.tsx:55,60` | `text-muted-foreground` on `bg-danger-subtle` measured 3.89:1 (AA fail) → `text-danger-subtle-foreground` at 6.80:1/8.14:1; recorded in `contrast.md:33-50`. A genuine defect the plan's own conversion would have shipped |
| Toast copy change | `ThemeToggle.tsx:44`, test `:89` | "next time you sign in" → "next time you reload". Also more accurate: `writeCachedTheme` persists the failed choice locally, so it is the next `ThemeSync` adoption on reload — not sign-in — that reverts it. Code and test changed together |
| `ThemeSync.wasSignedIn` moved to its own effect | `ThemeSync.tsx:29-31` | Plan set the flag inside the adopt effect; a user with a corrupted stored preference would then sign out without ever setting it, leaving a stale override to suppress adoption on the next sign-in. Rationale is in the comment at `:18-24` |
| `generate-icons.py` budget scoping | `tools/generate-icons.py:139-150` | FR-PERF-4 total now sums the six icon files rather than everything in `public/` |

## Build & Test Results

Not re-run — the controller's verification is authoritative and no doubt arose
that it does not already answer.

| Component | Build | Tests | Vet/Lint | Notes |
|---|---|---|---|---|
| `apps/auth-service` | PASS | PASS | PASS | via `make ci` |
| `apps/web` | PASS | PASS (115/115) | PASS | via `make ci` |
| `packages/shared-ts` | PASS | PASS (7/7) | PASS | via `make ci` |
| `deploy/k8s` manifests | PASS | n/a | n/a | `make ci` manifests target; no manifest change expected or made |
| Container asset serving | PASS | n/a | n/a | all 7 paths 200 + non-HTML content type |

One independent check was run for this audit (read-only): the semantic token
classes are confirmed present in the built stylesheet
`apps/web/dist/assets/index-5mURdli4.css`, which proves the Tailwind
registration in Task 6 reaches both `apps/web/src` and the
`packages/ui-components/src` content glob — `bg-success-subtle` originates only
in `StatusBadge.tsx`, so its presence in the bundle demonstrates the cross-package
glob works. No test covers that.

## Findings

### Low — plan checkboxes never marked complete

All 96 `- [ ]` step checkboxes in `plan.md` remain unchecked despite the work
being finished (`plan.md:82` through `plan.md:3322`). This is a documentation
hygiene issue: a reader arriving at the plan cold cannot tell the branch is done,
and a future `/execute-task` resumption would have no progress signal. No code
impact.

### Low — `tools/generate-icons.py` is not executable

`tools/generate-icons.py` is mode `0644` but carries a `#!/usr/bin/env python3`
shebang (`generate-icons.py:1`), so invoking it directly fails. Harmless in
practice: the only supported entry point is `tools/generate-icons.sh` (mode
`0755`), which calls `exec python3 "$ROOT/tools/generate-icons.py"`
(`generate-icons.sh:24`) and never needs the bit. Plan Task 13 step 3 only
chmod'd the shell script, so this matches the plan as written.

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE *(conditional on the manual browser pass)*

The implementation is a faithful execution of the plan. Where it departs from the
plan's literal text it does so in one direction only — additional test coverage,
one real accessibility fix, and guards that close gaps the plan's own tests left
open — and every departure carries an in-code rationale and a matching commit
message. The only thing standing between this branch and "genuinely done" is the
browser-dependent Task 17 checklist, which is honestly recorded rather than
quietly dropped.

## Action Items

1. Execute `manual-verification.md` — the Task 17 step 4 visual pass and step 5
   cross-device persistence check — before merging. This is the sole outstanding
   plan item.
2. *(Optional, hygiene)* Tick the completed checkboxes in `plan.md` so the
   document reflects the branch state.
3. *(Optional, cosmetic)* `chmod +x tools/generate-icons.py`, or drop its
   shebang, so the file's mode and its first line agree.
