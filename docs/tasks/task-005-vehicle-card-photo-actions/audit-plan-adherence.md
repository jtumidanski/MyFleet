# Plan Adherence Audit — task-005-vehicle-card-photo-actions

**Plan Path:** `docs/tasks/task-005-vehicle-card-photo-actions/plan.md`
**Audit Date:** 2026-07-31
**Branch:** `task-005-vehicle-card-photo-actions`
**Base:** `main` @ `352e8c1`
**Branch head:** `84b025e`
**Reviewer:** plan-adherence-reviewer (read-only; no files other than this report were written)

## Executive Summary

All 12 tasks and all 88 steps in the plan were executed. Every code block the plan
specifies verbatim was applied verbatim — I diffed the plan's Go, TypeScript, YAML and
nginx blocks against the files on disk and found no textual drift in any of them. Eleven
commits map one-to-one onto Tasks 1-11, plus the expected `84b025e` Prettier fix from
Task 12.

Verification re-run independently for this audit: `go build`, `go vet`, `go test -count=1`
across `github.com/jtumidanski/myfleet/...` all clean, and `make ci` (lint-check, vet,
test, build, fe-test, fe-build, manifests) exits `0`.

**Three deviations exist, all previously approved and all correctly recorded** — the Task
4/5 commit reshuffle, the browser-dependent steps reported as NOT VERIFIED, and Task 12
Steps 5-6 being handled by the controlling session. One further deviation was found that
was *not* in the approved list but *is* recorded in its own task report with a stated
reason (Task 4, four pre-existing tests updated for the signature change). **No silent
skips, no silent partial implementations, and no undocumented judgement calls found.**

## Task Completion

| # | Task | Status | Evidence |
|---|------|--------|----------|
| 1 | `server.ErrBadRequest` and the `codeFor` gaps | DONE | `packages/shared-go/server/errors.go:6` (sentinel), `:18-19` (StatusFor 400 case, first in the switch as specified); `packages/shared-go/server/server.go:12-32` — all nine `codeFor` cases present including the new `400 → bad_request` and `413 → payload_too_large`; tests `errors_test.go:5` and `:26` match the plan's blocks exactly. Commit `1cb2826`. |
| 2 | `mediavariant.GetByMediaObjectAndVariant` | DONE | `apps/media-service/internal/mediavariant/provider.go:12-15` (interface, with the "miss is not an error" contract comment), `:35-45` (implementation, `WHERE media_object_id = ? AND variant = ?`, `gorm.ErrRecordNotFound → (Model{}, false, nil)`); `provider_test.go:60` and `:83` are the two specified tests verbatim. `ListByMediaObject` retained at `provider.go:23` as instructed. Commit `40b417b`. |
| 3 | `ParseContentVariant` | DONE | `apps/media-service/internal/mediaobject/contentvariant.go` — type + three constants at `:8-14`, parser at `:23-36` with exact-lowercase matching and `server.ErrBadRequest` default; `contentvariant_test.go:14` covers the four accepted values and all five rejected shapes (`Thumbnail`, `THUMBNAIL`, `bogus`, `small`, `" thumbnail"`). Commit `36e5cb3`. |
| 4 | `VariantLookup` port + variant-aware `Processor.Content` | DONE | `processor.go:45-48` (`VariantRef`), `:60-62` (`VariantLookup`), `:69-79` (`ContentInfo`), `:111-120` (struct field + 5-arg `NewProcessor`), `:262-308` (`Content`), `:311-330` (`openOriginal`). Order of operations verified: `GetByID` at `:264` precedes any `Lookup` (`:270`) or `GetObject` (`:281`). Lookup error propagates at `:274`; store-drift falls back with a `Warn` at `:297-299`; miss logs `Debug` at `:277-278`; `Size` deliberately left 0 for a variant at `:293`. Test fakes at `processor_test.go:309-330` (`getBodies`/`missing`/`getCalls`) and `:365-375` (`fakeVariants`); all six specified `TestContent_*` cases present at `:623`, `:655`, `:690`, `:720`, `:752`, `:777`. All 14 `NewProcessor(logrus.New()…)` call sites pass a fifth argument — zero still end in `store)`. Commit `6b8b39c`. |
| 5 | Route, composition-root adapter, HTTP tests | DONE | `resource.go:44-45` (new signature + 5-arg `NewProcessor`), `:156` (`ParseContentVariant` on the query value, before `proc.Content`), `:179-187` (headers from `ContentInfo`: `Content-Type` when non-empty, `Content-Length` only when `Size > 0`, unconditional `Cache-Control: private, max-age=300`, then `WriteHeader(200)`). No `Vary` header added, as specified. `cmd/main.go:156-168` (`variantLookup` adapter, verbatim) and `:124` (route registration). `resource_test.go:58` / `:66` (`testRouter` / `testRouterWithVariants` split), `:254` / `:266` (helpers), and all six HTTP cases at `:287`, `:314`, `:338`, `:359`, `:380`, `:405`. Commits `6b8b39c` (Steps 1-2, see Deviation 1) + `7e4fcd4` (Steps 3-8). |
| 6 | Frontend media-variant plumbing | DONE | `types/models/media.ts:9` (`MediaVariant` union); `services/api/MediaService.ts:6` (type import), `:17` (route comment updated), `:65-67` (`getContentBlob` with the `original → no query parameter` rule); `lib/hooks/api/media.ts:9` (import), `:22-28` (variant-aware `content()` with `contents()` prefix preserved), `:130-137` (hook signature + `queryKey`/`queryFn`). `MediaThumbnail` confirmed untouched (`git diff --stat 352e8c1..HEAD -- apps/web/src/components/features/media/` is empty), as the plan requires. `media.test.ts:24-25` asserts both key shapes. Commit `5c9ed3d`. |
| 7 | Runtime configuration (`config.json`) | DONE | `lib/config/runtimeConfig.ts` — interface `:15-17`, `DEFAULT_RUNTIME_CONFIG` `:24-26` (template byte-identical to the plan's Global Constraint), `RUNTIME_CONFIG_URL` `:28`, `FETCH_TIMEOUT_MS = 2000` `:34`, zod schema with per-field then object `.catch()` `:40-44`, `parseRuntimeConfig` `:46-48`, module-level `latched` `:50`, `getRuntimeConfig` `:57-59`, `loadRuntimeConfig` `:67-86` (AbortController + `clearTimeout` in `finally`, single `console.warn`, always resolves). `runtimeConfig.test.ts` has all 9 specified cases. `public/config/config.json` present with the same template. `nginx.conf:16-21` — `location = /config/config.json` with `Cache-Control: no-cache`, placed above `location /`. `main.tsx` matches the plan block verbatim. Commit `65e0f6f`. |
| 8 | Ship the config as a ConfigMap | DONE | `deploy/k8s/base/web/configmap.yaml` (ConfigMap `web-config`, key `config.json`, real default value not a placeholder); `deploy/k8s/base/web/deployment.yaml:70-84` (directory mount at `/usr/share/nginx/html/config`, `readOnly: true`, no `subPath`, plus the `web-config` volume); `deploy/k8s/base/kustomization.yaml:29`. Re-verified for this audit: `make ci` runs `tools/check-manifests.sh` — both overlays render, `main` still ships no PVC/Secret/ClusterRole/ClusterRoleBinding and no placeholders, IngressRoute route-set parity holds. Step 5's server dry-runs against the `bee` context are recorded with full object listings in `.superpowers/sdd/plan/task-8-report.md:156-…`, including `configmap/web-config created (server dry run)` on the `main` overlay. Commit `14f9ec8`. |
| 9 | The Carfax URL builder | DONE | `lib/carfax.ts:2` (`VIN_PLACEHOLDER`), `:23-36` (`buildCarfaxUrl` — trim, empty→null, missing-placeholder→null, `split/join` with `encodeURIComponent`, `new URL(...).protocol !== 'https:'`→null inside `try`, parse failure→null). `carfax.test.ts` has all 9 specified cases including the `javascript:`/`http:`/`data:` scheme rejections. Commit `2280480`. |
| 10 | Test helpers and `VehiclePhotoThumbnail` | DONE | `src/test/renderWithProviders.tsx` and `src/test/objectUrl.ts` both verbatim. `media.test.ts:9` now imports the shared `stubObjectUrl`; the local copy is gone (only the import and its three usages at `:137`, `:138`, `:142` remain). `VehiclePhotoThumbnail.tsx:11` (`BOX = 'h-20 w-20 shrink-0 rounded-md'`, the exact constrained class string), `:26-38` (`PhotoPlaceholder` with `role="img"` + `aria-label`), `:63` (`useMediaContentUrl(mediaId, 'thumbnail')`), `:66-81` (the four states in the specified order). No toast call anywhere in the file. `VehiclePhotoThumbnail.test.tsx` has all 5 cases. Commit `8990984`. |
| 11 | Rebuild `VehicleCard` | DONE (Step 6 NOT VERIFIED — no browser; approved) | `VehicleCard.tsx` matches the plan's block verbatim: wrapping `<Link>` deleted, `min-w-0` on both flex children (`:42` and `:44`), `VehiclePhotoThumbnail` at `:36`, action row `:63`, `Button asChild` + router `<Link>` at `:69-73`, conditional plain `<a>` with `target="_blank" rel="noopener noreferrer"` at `:79-89`, both `aria-label`s naming `title`, no `requireWrite`/role gating, no prefetch or `onMouseEnter`. `VehicleCard.test.tsx` has all 10 cases. `VehicleList.tsx:15` is `h-44` — see "Judgement calls left open" below. Commit `3de9b88`. |
| 12 | Whole-branch verification | DONE (Steps 4-6 out of scope by approval) | Steps 1-3 executed and evidenced in `.superpowers/sdd/plan/task-12-report.md`, including the full `make ci` tail, both server dry-runs, and a 34-row acceptance-criteria walk with the 403→404 correction stated explicitly rather than silently ticked. Step 1's one failure (Prettier print-width in `runtimeConfig.test.ts`) was fixed and committed separately as `84b025e`, staged file-by-file rather than `git add -A`. |

**Completion rate:** 12/12 tasks (100%). All 88 numbered steps accounted for.
**Skipped without approval:** 0
**Partial implementations:** 0
**Silent deviations:** 0

## The three approved deviations — confirmed handled correctly

### 1. Task 4 also applied Task 5 Steps 1-2 (no broken commit in history)

Confirmed, and attributed correctly. `git show --stat 6b8b39c` (the Task 4 commit) touches
four files, including `resource.go` (+31/-…) and `resource_test.go` (+15/-…) — i.e. the
`InitializeRoutes` signature change, the full content-handler rewrite, and the
`testRouter`/`testRouterWithVariants` split. `git show --stat 7e4fcd4` (Task 5) then
touches only `cmd/main.go` and `resource_test.go` (+177), i.e. Steps 3 and 6.

The plan's own Task 4 Step 7 anticipated this: *"If you want a green gate before moving on,
do Steps 1-2 of Task 5 first."* The work is present, the history compiles at every commit,
and `.superpowers/sdd/plan/task-4-report.md:42` states plainly which of Task 5's steps it
absorbed and which remain. **Not a gap.**

### 2. Browser-dependent steps reported as NOT VERIFIED

Confirmed, and no judgement was made silently.

- **Task 11 Step 6** — `task-11-report.md` lists all six browser checks individually, each
  prefixed "NOT VERIFIED — requires a browser", and explicitly raises the two judgement
  calls the step asked for rather than resolving them.
- **`VehicleList.tsx:15` skeleton is `h-44`, un-nudged.** Verified against the diff:
  `git diff 352e8c1..HEAD -- apps/web/src/components/features/vehicles/VehicleList.tsx`
  shows exactly one changed line, `h-28` → `h-44`. Matches the plan's Step 5 value with no
  adjustment.
- **Grid-column class unchanged.** That same diff is the *only* hunk in the file — the
  `grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3` class at `VehicleList.tsx:13` and
  `:30` is byte-identical to `main`. The report states the density question is left open
  for the human partner.
- **Task 12 Step 4** — `task-12-report.md` marks the step "NOT PERFORMED", confirms no dev
  server / `make up` / browser was started, and lists all five network-panel checks
  verbatim as NOT VERIFIED. The acceptance-criteria table separately flags criteria #12,
  #15, #17, #24 and #25 as having a browser-dependent component rather than claiming them
  outright — including an honest note on #24 that `justify-end` means the detail button's
  x-offset genuinely does differ with and without a second button, which is a visual call
  the static evidence cannot settle.

**No gap. The reports do exactly what was ruled.**

### 3. Task 12 Steps 5-6 handled by the controlling session

Confirmed absent from Task 12's work and stated as such in `task-12-report.md`. Correct.

## Fourth deviation — recorded, not silent

**Task 4: four pre-existing `TestContent_*` tests were also updated.** Not listed in the
approved set, so flagged here for completeness — but it is documented with a reason at
`.superpowers/sdd/plan/task-4-report.md:44`, not silent.

Changing `Content`'s signature necessarily broke `TestContent_returnsBytesAndModelForOwnFleet`,
`TestContent_crossFleetIs404`, `TestContent_missingObjectIs404` and
`TestContent_otherStorageFailuresAreNot404` (`processor_test.go:501`, `:532`, `:555`, `:580`) —
they called the 3-arg `Content` and treated its first return as a `Model`. The fix was
minimal and mechanical: add `ContentOriginal` as the fourth argument, and read
`info.ContentType` instead of `m.ContentType()`. No assertion or behaviour changed. The
plan's own Task 4 Step 7 asserts the package compiles after Step 6 except for
`resource.go`/`resource_test.go`, which is only true once these four are updated — so this
was compile-fixing inside a file the task already owns, not scope creep. **Correct call,
correctly recorded.**

## Judgement calls the plan invited and that remain open

These are **not** adherence failures — the plan asked for a browser and there is none — but
they are live decisions the human partner still owes:

1. **`h-44` skeleton height** — left at the plan's specified value. If the skeleton-to-content
   transition visibly jumps, `VehicleList.tsx:15` is a one-token change.
2. **`lg:grid-cols-3` density** — PRD §9.4 flagged whether three columns still read well now
   that a card is ~176px rather than ~112px. Untouched, as instructed.
3. **PRD criterion #29** (`VITE_CARFAX_URL_TEMPLATE` settable at build time) is NOT MET as
   literally written, and Task 12's report says so. It is superseded by design D3's runtime
   `config.json`. Verified independently: `grep -rn "VITE_CARFAX\|import\.meta\.env" apps/web/src`
   returns exactly one hit, a prose comment at `runtimeConfig.ts:6` — no build-time env
   mechanism exists anywhere. The Global Constraint "No `import.meta.env` / `VITE_*` variable
   is introduced anywhere" holds.

## Build & Test Results

Re-run independently for this audit from the worktree root.

| Target | Build | Tests | Vet | Notes |
|--------|-------|-------|-----|-------|
| Go workspace (`github.com/jtumidanski/myfleet/...`) | PASS | PASS | PASS | `go build` / `go vet` / `go test -count=1` all clean, no output. Includes `media-service` and `shared-go`. |
| `apps/web` | PASS | PASS | n/a | `tsc -b && vite build` succeeds; 15 test files / 99 tests pass. |
| `packages/shared-ts` | PASS | PASS | n/a | 2 files / 7 tests. |
| `deploy/k8s` | PASS | n/a | n/a | `tools/check-manifests.sh`: both overlays render; `main` has no PVC/Secret/ClusterRole/ClusterRoleBinding/placeholders; IngressRoute parity holds. |
| **`make ci` (whole gate)** | **PASS** | **PASS** | **PASS** | Exit code `0`. |

Not run by this audit: `kubectl apply --dry-run=server` (recorded as PASS for both overlays
in the Task 8 and Task 12 reports, against the `bee` context) and the Docker builds (no
`docker-bake.hcl` in this repo; container builds are per-service `docker build` and are not
part of `make ci`).

The `(!) Some chunks are larger than 500 kB` note in the Vite output is a pre-existing
advisory, not a regression — the single bundle already exceeded 500 kB before this branch.

## Extra commit

`84b025e` — Prettier re-wrapping of three `vi.stubGlobal(...)` expressions in
`runtimeConfig.test.ts`, caught by Task 12's whole-branch `make ci` (each earlier task's
gate ran against a smaller working tree). Pure formatting; no assertion changed. Legitimate
and correctly scoped.

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE, subject to the three open human judgement calls above
  (none of which block correctness, and all of which were escalated rather than guessed).

## Action Items

None required for plan adherence. For the human partner, in priority order:

1. Look at `/vehicles` in a browser and settle the two visual calls: the `h-44` skeleton
   height (`apps/web/src/components/features/vehicles/VehicleList.tsx:15`) and whether
   `lg:grid-cols-3` still reads well at the new card height.
2. Run the five Task 12 Step 4 network-panel checks against a live stack (`?variant=thumbnail`
   with `Cache-Control: private, max-age=300` and no `Content-Length`; the no-variant original
   with its `Content-Length`; `?variant=bogus` → 400 `bad_request`; `/config/config.json` →
   200 `no-cache`; no `carfax.com` request before a click).
3. Decide whether PRD §10 should be amended so criterion #29 and the cross-fleet `403`
   criterion reflect design D3 and D1 rather than leaving the PRD contradicting the shipped
   behaviour.
