# Frontend Audit — task-014-member-names-ownership-transfer

- **Audit Scope:** TypeScript/React changes in `main...HEAD` (BASE `5ff93cd`, HEAD `d37afa5`)
- **Guidelines Source:** `frontend-dev-guidelines` skill (`.claude/skills/frontend-dev-guidelines/`)
- **Date:** 2026-08-02
- **Build:** PASS
- **Tests:** 321 passed, 0 failed (41 files)
- **Lint:** PASS (`eslint src --max-warnings 0`, clean)
- **Overall:** NEEDS-WORK

## Build & Test Results

```
> build
> tsc -b && vite build
vite v5.4.21 building for production...
✓ 1828 modules transformed.
dist/assets/index-PvIhjJ8C.js   617.53 kB │ gzip: 179.45 kB
✓ built in 3.70s
=== EXIT 0 ===
```

```
> test
> vitest run
 ✓ src/components/features/settings/MemberList.test.tsx (15 tests) 1433ms
 Test Files  41 passed (41)
      Tests  321 passed (321)
   Duration  3.87s
```

```
> lint
> eslint src --max-warnings 0
(no output — clean)
```

Pre-existing, unrelated warning: the vite bundle exceeds the 500 kB chunk-size
advisory. Present on `main` too; not introduced here.

## File Inventory

| File | Classification | Change |
|------|----------------|--------|
| `apps/web/src/components/features/settings/MemberList.tsx` | Component (feature) | M |
| `apps/web/src/components/features/settings/MemberList.test.tsx` | Test | A |
| `apps/web/src/components/ui/alert-dialog.tsx` | Component (ui primitive) | A |
| `apps/web/src/lib/hooks/api/users.ts` | Hook | A |
| `apps/web/src/lib/hooks/api/users.test.ts` | Test | A |
| `apps/web/src/lib/hooks/api/members.ts` | Hook | M |
| `apps/web/src/lib/hooks/api/members.test.ts` | Test | M |
| `apps/web/src/services/api/UserService.ts` | Service | A |
| `apps/web/src/services/api/MemberService.ts` | Service | M |
| `apps/web/src/lib/utils/displayName.ts` | Other (util) | A |
| `apps/web/src/index.css` | Other (theme tokens) | M |
| `apps/web/tailwind.config.ts` | Other (theme config) | M |
| `apps/web/package.json` / `package-lock.json` | Other (deps) | M |

No Schema (`lib/schemas/`) or Type (`types/`) files were changed.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | WARN | Sole match is `members.test.ts:117` `ReturnType<typeof vi.spyOn<any, any>>` with an `eslint-disable-next-line @typescript-eslint/no-explicit-any` at `:116`. **Pre-existing** — identical at `main:apps/web/src/lib/hooks/api/members.test.ts:108-109`; untouched by this branch. Zero `any` in all newly authored code. |
| FE-02 | No manual class concatenation | PASS | Every dynamic className goes through `cn()`: `alert-dialog.tsx:16,31,42,50,63,75,85,95`. `MemberList.tsx` uses only static string literals (`:129,143,149,150,155,163,219,270,299`) — no `+` and no template-literal className anywhere in scope. |
| FE-03 | No direct API client calls in components | PASS | Only `lib/api/client` import in scope is `MemberService.ts:12` (a service — correct layer). `MemberList.tsx:13-30` imports only ui primitives, hooks, context and the util. |
| FE-04 | No inline Zod schemas in components | PASS | No `zod` import and no `z.object(`/`z.string(` in any changed file. |
| FE-05 | No spinners for content loading | PASS | Zero `animate-spin` in scope. Loading uses `Skeleton` — `MemberList.tsx:127-135`. |
| FE-06 | No hardcoded colors | PASS | Semantic tokens only: `bg-destructive text-destructive-foreground` (`MemberList.tsx:219,299`), `text-muted-foreground` (`:138,155,157,289`), `bg-background` (`alert-dialog.tsx:31`). The scrim uses a **new semantic token** `bg-overlay/80` (`alert-dialog.tsx:16`) backed by `--overlay` in `index.css:13-15` (light) and `:62-64` (dark) and registered at `tailwind.config.ts:23` — not a raw color. |
| FE-07 | No state mutation | PASS | Both `.sort()` calls operate on fresh copies: `users.ts:27` `[...ids].sort()`, `users.ts:43` `[...new Set(ids)].sort()`. `MemberList.tsx:62,74` use `.filter()`. The `.push()` at `MemberList.test.tsx:285,289` is a local test-fixture array, not state. |
| FE-08 | No default exports for components | PASS | Named exports only: `MemberList.tsx:47`, `alert-dialog.tsx:101-113`, `displayName.ts:14`, `users.ts:42`, `members.ts:69,128`. Zero `export default` in scope. |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | `members.ts:105` (`useRemoveMember.onError`) and `members.ts:138` (`useUpdateMemberRole.onError`), each surfacing via `toast.error` (`:110,114,140,142`) plus the mint-failure toast at `:87-89`. The three bare `catch {}` in `MemberList.tsx:86,95,119` are deliberate no-ops — the mutation `onError` already toasted; documented at `:87-88`, `:97`, `:121-124`. |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS (with note) | `UserAttributes` (`types/models/user.ts:10-15`) and `MembershipAttributes` (`types/models/membership.ts:7-12`) are both wrapped as `JsonApiResource<A>` (`user.ts:18`, `membership.ts:14`); `BaseService.listAt` returns `Array<JsonApiResource<A>>` (`BaseService.ts:26-29`). **Note:** `MembershipAttributes.role` is `string`, not the `FleetRole` union — see Non-Blocking. |
| FE-11 | Service extends `BaseService` | PASS | `UserService.ts:15` `class UserService extends BaseService<UserAttributes>`; `MemberService.ts:17` `class MemberService extends BaseService<MembershipAttributes>`. Both exported as singletons (`UserService.ts:33`, `MemberService.ts:63`). Nested action routes correctly go through the protected `listAt` (`UserService.ts:29`, `MemberService.ts:25`) or `apiClient` directly with the JSON:API envelope (`MemberService.ts:56`), matching the documented direct-client escape hatch. |
| FE-12 | Query key factory uses `as const` | PASS | `members.ts:30-34` (`all`, `lists()`, `list()`) and `users.ts:25-28` (`all`, `byIds()`) — every branch terminates in `as const` and builds hierarchically via spread. Asserted by `members.test.ts:37-41` and `users.test.ts:42-51`. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No `<form>` was introduced. The successor picker (`MemberList.tsx:274-288`) is a single controlled `Select` in a confirmation dialog with no submit; its one rule ("a successor is required") is enforced by disabling the action at `:300`. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No Zod schema added or changed on this branch. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | **FAIL** | Buttons are fine — `button.tsx:7` carries `cursor-pointer` in the CVA base, and `alert-dialog.tsx:85,95` route `AlertDialogAction`/`AlertDialogCancel` through `buttonVariants()`. But the successor picker does not: **`MemberList.tsx:275`** `<SelectTrigger id="successor">` resolves to `select.tsx` `SelectTrigger`, whose className carries only `disabled:cursor-not-allowed` and no `cursor-pointer`; and **`MemberList.tsx:283`** `<SelectItem>` resolves to `select.tsx` `SelectItem`, which sets `cursor-default` **explicitly**. Options render as `<div role="option">` — unambiguously the "custom interactive element" case in `patterns-styling.md:232-236`. Root cause is the shared, unchanged `components/ui/select.tsx`; the fix belongs there and also repairs the three pre-existing call sites. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS (with gap) | `MemberList.test.tsx` — 15 tests covering the FR-1.5 fallback chain (`:92-111`), the empty-string `\|\|` case (`:115-121`), name-lookup failure degrading to id fallbacks (`:124-134`), `(you)` (`:137-150`), remove-confirm and cancel (`:155-188`), promote (`:193-224`), and all four ux-flow leave states (`:229-356`). `users.test.ts` — 5 tests (`:41-92`). `members.test.ts` adds 6 tests for the new hook surface (`:159-286`). `alert-dialog.tsx` is a shadcn primitive exercised through `MemberList.test.tsx`. **Gap:** `lib/utils/displayName.ts` has no colocated unit test, while its only sibling in that folder does (`lib/utils/download.test.ts`) — see Non-Blocking. |
| FE-17 | Mocks updated when services changed | PASS | `MemberService.updateRole` added to both mocks: `members.test.ts:62-67` and `MemberList.test.tsx:22`. New `UserService` mocked at `MemberList.test.tsx:26-28` and `users.test.ts:8-10`. The breaking `useRemoveMember` signature change (`(userId: string)` → `({ userId, isSelf })`, `members.ts:72`) is reflected at every call site: `MemberList.tsx:84,118` and `members.test.ts:136,165,183,201,225`. No `__mocks__/` directory exists in this repo — inline `vi.mock` is the convention. |

## Dependency Change Review

The `@radix-ui/react-select` `^2.2.6 → ^2.3.7` bump (`package.json:16`) was verified
against the lockfile, not taken on faith.

**The stated fix is correct and confirmed.** On `main`, `@radix-ui/react-select@2.2.6`
pinned `@radix-ui/react-focus-scope@1.1.7`; the new `@radix-ui/react-alert-dialog@1.1.23`
requires `1.1.16`, which would have forced npm to nest a **second** copy. On HEAD there is
exactly one installed `node_modules/@radix-ui/react-focus-scope@1.1.16`, shared by both
`react-dialog` and `react-select` — a single module-level focus-scope stack, which is what
makes a `Select` inside an `AlertDialog` stop ping-ponging focus. Bumping the direct
dependency is preferable to an npm `overrides` pin, which would silently rot.

**Blast radius is wider than the changed files suggest.** The bump moved 35 transitive
packages (all of `@radix-ui/react-popper`, `-dismissable-layer`, `-portal`, `-collection`,
`-primitive`, plus `@floating-ui/*`). `@radix-ui/react-select` backs four components:

| Consumer | Select exercised by a test? |
|----------|------------------------------|
| `MemberList.tsx` | Yes — `MemberList.test.tsx:295,318,338` open the combobox |
| `MaintenanceRecordForm.tsx` | Yes — `MaintenanceRecordForm.test.tsx` ("offers only the categories of the requested kind") |
| `InviteForm.tsx` | **No** — `InviteForm.test.tsx:41-47` types the email and submits; the role `Select` is never opened |
| `MaintenanceScheduleForm.tsx` | **No** — no test file exists for this component |

Half the surface affected by a 35-package dependency move has no automated coverage.

## Summary

### Blocking (must fix)

- **FE-15** — `MemberList.tsx:275` (`SelectTrigger`) and `MemberList.tsx:283` (`SelectItem`)
  render without `cursor-pointer`; `SelectItem` sets `cursor-default` outright. Fix in
  `components/ui/select.tsx` (add `cursor-pointer` to the `SelectTrigger` base and swap
  `cursor-default` → `cursor-pointer` on `SelectItem`), which also repairs `InviteForm`,
  `MaintenanceRecordForm` and `MaintenanceScheduleForm`.

### Non-Blocking (should fix)

- **Title casing** (`patterns-components.md:277-297` — "Button labels ... use title case"):
  `MemberList.tsx:185` and `:253` render "Make owner", `:306` renders "Transfer & leave",
  and the field label at `:272` reads "New owner". Guideline form is "Make Owner",
  "Transfer & Leave", "New Owner". Note this copy is transcribed verbatim from
  `ux-flow.md:76,84` — the design doc and the guideline disagree; resolve in one place.
- **`isOwner` contradicts the component's own reasoning.** `MemberList.tsx:64-67` argues at
  length that the members list, not the token claim, is the source of truth for role — then
  gates Remove and Make owner on the `isOwner` prop (`:174`), which `SettingsPage.tsx:15-16`
  derives from `useAuth().role`, a JWT claim. The list-derived `myRole` is already computed
  at `MemberList.tsx:68`. Consequence: a just-promoted user (stale `member` token) sees no
  owner actions despite the list saying otherwise; a just-demoted user sees actions that
  will 403. Not a security hole — the server enforces both — but the two role sources should
  be one.
- **`activeMembers` is not filtered to active** (`MemberList.tsx:61` — `members ?? []`). The
  whole leave-state matrix (`ownerCount:62`, `memberCount:63`, `soleOwner:70`,
  `leaveBlocked:72`) rests on it. The endpoint behind it is the **unfiltered** one:
  `resource.go:43` → `processor.go:51` `ListMembers` → `provider.go:42` `ListByFleetID`
  (no status predicate), while a filtered `ListActiveByFleetID` (`provider.go:56`,
  `WHERE status = 'active'`) exists and is used elsewhere. Harmless today — removal
  hard-deletes (`administrator.go:60,104`) and status is "vestigial" per
  `processor.go:103` — but the name asserts a filter the code does not apply, and the
  matrix silently miscounts the day status stops being vestigial.
  `MembershipAttributes.status` is already exposed, so the filter is one line.
- **`MembershipAttributes.role` is `string`** (`types/models/membership.ts:10`) while the
  new mutation takes the `FleetRole` union (`members.ts:131`, `types/models/user.ts:22`).
  Every branch of the state matrix compares `role === 'owner'` (`MemberList.tsx:62,68,176`)
  against an untyped string, so a typo compiles. Narrowing the model to `FleetRole` would
  type-check the matrix.
- **Raw `<label>` instead of the `Label` primitive.** `MemberList.tsx:271` hand-rolls
  `<label className="text-sm font-medium">` — it is the only raw `<label>` in the entire
  app, and `components/ui/label.tsx` exports a `Label` whose CVA base is that exact class
  string plus `leading-none peer-disabled:*`.
- **No colocated test for `displayName.ts`.** `displayFor` (`displayName.ts:14-17`) encodes
  a three-branch fallback and a documented `||`-not-`??` subtlety, but is covered only
  transitively through `MemberList.test.tsx:92-134`. The only other file in `lib/utils/`
  ships `download.test.ts`.
- **Untested Select consumers after the Radix bump.** `InviteForm.tsx` and
  `MaintenanceScheduleForm.tsx` both render a `Select` and neither has a test that opens
  one — see Dependency Change Review.
- **FE-01 residue.** `members.test.ts:116-117` carries an eslint-suppressed `any`. It
  predates this branch (`main:...:108-109`) and is not a regression, but the file is now
  in scope; `ReturnType<typeof vi.spyOn>` or a `MockInstance` type would clear it.
