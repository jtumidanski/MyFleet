# Frontend Audit — task-003-dark-mode-branding

- **Audit Scope:** `git diff 352e8c1..41c5d3b -- 'apps/web/**' 'packages/ui-components/**'` (28 `.ts`/`.tsx` files)
- **Guidelines Source:** `.claude/skills/frontend-dev-guidelines` (SKILL.md + anti-patterns, architecture-overview, patterns-react-query, patterns-forms-validation, patterns-service-layer, patterns-types, patterns-styling, patterns-components, testing-guide)
- **Date:** 2026-07-31
- **Build:** PASS (`make ci` exit 0 — verified by controller, not re-run)
- **Tests:** 115/115 `apps/web`, 7/7 `packages/shared-ts` (verified by controller, not re-run)
- **Overall:** NEEDS-WORK

## Build & Test Results

Per the invoking controller, already verified and not re-run here:

```
make ci exits 0
  apps/web            115/115 passed
  packages/shared-ts    7/7   passed
  lint-check (--max-warnings 0)  clean
  build (tsc -b && vite build)   clean
  format:check                   clean
```

Only two read-only verifications were run locally, both to settle specific doubts:

1. Tailwind's default ring-offset colour — `node_modules/tailwindcss/lib/corePlugins.js:3837`:
   `"--tw-ring-offset-color": theme("ringOffsetColor.DEFAULT", "#fff")`.
2. A replay of `conventions.test.ts`'s directory walk, to prove it reaches
   `packages/ui-components/src/StatusBadge.tsx` (it does — exactly one file).

## File Inventory

**Page**
- `apps/web/src/pages/PlaceholderPage.tsx`

**Component**
- `apps/web/src/components/AppLayout.tsx`
- `apps/web/src/components/BrandMark.tsx`
- `apps/web/src/components/ThemeSync.tsx`
- `apps/web/src/components/ThemeToggle.tsx`
- `apps/web/src/components/providers/AppProviders.tsx`
- `apps/web/src/components/features/activity/ActivityEventIcon.tsx`
- `apps/web/src/components/features/dashboard/widgets/FleetOverviewWidget.tsx`
- `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx`
- `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx`
- `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx`
- `apps/web/src/components/features/vehicles/maintenance/SeverityChip.tsx`
- `packages/ui-components/src/StatusBadge.tsx`

**Hook**
- `apps/web/src/lib/hooks/api/auth.ts`

**Service** — none. This repo has no `services/api/` layer; hooks call `apiClient` directly (`apps/web/src/lib/api/client.ts`). FE-11 is therefore N/A.

**Schema** — none. No Zod, no forms in scope. FE-13/FE-14 N/A.

**Type**
- `apps/web/src/types/models/user.ts`

**Other**
- `apps/web/src/context/ThemeContext.tsx` (context)
- `apps/web/src/lib/theme.ts` (pure helpers)
- `apps/web/src/components/brandMarkPath.ts` (generated constant)
- `apps/web/tailwind.config.ts`, `apps/web/src/index.css`, `apps/web/index.html`, `apps/web/package.json`
- Tests: `AppLayout.test.tsx`, `ThemeSync.test.tsx`, `ThemeToggle.test.tsx`, `ThemeContext.test.tsx`, `theme.test.ts`, `auth.test.ts`, `AppProviders.test.tsx`, `conventions.test.ts`, `test/setup.ts`

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grepped `: any` / `as any` / `<any>` across all 28 in-scope files — zero matches. Casts used are narrow and typed: `ThemeSync.test.tsx:26` (`as User`, needed to feed the invalid `'purple'` case), `test/setup.ts:78` (`as unknown as MediaQueryList` for the matchMedia polyfill), `theme.ts:22` (`VALID as readonly string[]`). |
| FE-02 | No manual class concatenation | **FAIL** | `packages/ui-components/src/StatusBadge.tsx:16` — `` className={`inline-flex rounded px-2 py-0.5 text-xs font-medium ${VARIANT[status]}`} ``. **Pre-existing**: `git show 352e8c1:packages/ui-components/src/StatusBadge.tsx` has the byte-identical line; this diff only changed the `VARIANT` map (lines 8–11). `cn()`'s deps (`clsx`, `tailwind-merge`) are absent from `packages/ui-components/package.json`, so fixing it needs a dependency addition. Benign in practice — the `VARIANT` classes (`bg-*-subtle`, `text-*-subtle-foreground`) do not collide with the base classes, so there is nothing for `twMerge` to resolve. All in-scope `apps/web` files use `cn()` (`AppLayout.tsx:40`) or plain literals. |
| FE-03 | No direct API client calls in components | PASS | No component or page imports `lib/api/client`. The only `apiClient` import in scope is `apps/web/src/lib/hooks/api/auth.ts:3`, which is the hook layer. `ThemeToggle.tsx:5` reaches the network solely through `useUpdateTheme`. |
| FE-04 | No inline Zod schemas in components | PASS | Zero `z.object(` / `z.string(` / `zodResolver` matches across scope — no forms were touched. |
| FE-05 | No spinners for content loading | PASS | Zero `animate-spin` matches. `MaintenanceQueueView.tsx:23-31` and `FleetOverviewWidget.tsx:12-21` both use `<Skeleton>`. |
| FE-06 | No hardcoded colors | PASS (with note) | Zero palette-class matches across the 28 files. Conversions verified individually: `AppLayout.tsx:30,42-44,54,56`, `SeverityChip.tsx:16,20,24`, `MaintenanceQueueView.tsx:39,47,55,60,76`, `FleetOverviewWidget.tsx:32,36,40`, `OverdueMaintenanceWidget.tsx:28`, `UpcomingMaintenanceWidget.tsx:28`, `ActivityEventIcon.tsx:15`, `PlaceholderPage.tsx:7`, `StatusBadge.tsx:8-11`. Every token used is defined in both `:root` (`index.css:33-48`) and `.dark` (`index.css:78-93`) and mapped in `tailwind.config.ts:51-74`. **Note**: three hex literals remain outside the Tailwind cascade and are unavoidable there — `public/favicon.svg:3-4` (a standalone asset has no access to app CSS variables) and `index.html:16-17` (`theme-color` metas; the manifest format has no media-query support). Both are documented at `index.html:9-14` as manually coupled to `--background`. See **Important #1** for a fourth, avoidable one. |
| FE-07 | No state mutation | PASS | Zero `.push(` / `.splice(` / `.sort(` / `.reverse(` matches. `auth.ts:94-104` builds a fully spread replacement (`{...previous, user: {...previous.user, attributes: {...previous.user.attributes, themePreference}}}`). `ThemeContext.tsx:78-91` uses `setPreferenceState` with plain values; the two `useRef`s hold booleans, not state. |
| FE-08 | No default exports for components | PASS | The only `export default` in scope is `tailwind.config.ts:3`, which is the required shape for a Tailwind config, not a component. All components are named exports: `BrandMark.tsx:16`, `ThemeSync.tsx:14`, `ThemeToggle.tsx:34`, `ThemeContext.tsx:41`, `AppProviders.tsx:20`. |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | The mutation's transport path terminates in `packages/shared-ts/src/apiClient.ts:50` — `if (!res.ok) throw createErrorFromUnknown({status, body})`. The rejection surfaces to the user via `toast.error(...)` at `ThemeToggle.tsx:53-55`. The two bare `catch` blocks in `lib/theme.ts:38,47` are storage-availability guards, not error swallowing — they are the documented FR-FLASH-3 behaviour (blocked `localStorage` must not break boot), each carries an explanatory comment, and both are covered by tests (`theme.test.ts:69-77`, `theme.test.ts:91-99`). |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/user.ts:16` — `export type User = JsonApiResource<UserAttributes>`; `themePreference: ThemePreference` added at line 14 inside `attributes`. Verified against the backend rather than assumed: `apps/auth-service/internal/user/rest.go:11` declares `ThemePreference string \`json:"themePreference"\``, and `apps/auth-service/internal/user/resource_test.go:119` pins the accepted PATCH envelope as `{"data":{"type":"users","attributes":{"themePreference":%q}}}` — byte-for-byte what `auth.ts:71` sends. |
| FE-11 | Service extends `BaseService` | N/A | No `services/api/` layer exists in this repo; the documented direct-client pattern is used via `apps/web/src/lib/api/client.ts`. Consistent with the pre-existing `useMe`/`logoutRequest` in the same file. |
| FE-12 | Query key factory uses `as const` | PASS | `auth.ts:7-10` — `all: ['auth'] as const`, `me: () => ['auth','me'] as const`. `useUpdateTheme` writes through the factory (`auth.ts:94`), never a literal. Sign-out removal at `AuthContext.tsx:56` uses `authKeys.all`, which prefix-matches `authKeys.me()`. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | N/A | No form components in scope. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No Zod schemas in scope. Runtime validation is done with a hand-written type guard, `theme.ts:24-26`, which is appropriate for a three-value union and is exercised directly by `theme.test.ts:11-26`. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | The only interactive element added is `ThemeToggle.tsx:50-58`, which renders `<Button>`; `components/ui/button.tsx:7` carries `cursor-pointer` in the CVA base string. `AppLayout.tsx:56` converts the raw `<button>` Sign-out to the same `<Button>`, so it gains the affordance it previously lacked. `AppLayout.tsx:36` `NavLink` renders a native `<a href>`. `BrandMark.tsx:17` is a non-interactive `aria-hidden` `<svg>`. No clickable `<div>`s, rows, or `render`-prop triggers were introduced. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS (with warnings) | New non-trivial units all have dedicated suites: `theme.ts`→`theme.test.ts` (13 cases), `ThemeContext.tsx`→`ThemeContext.test.tsx` (9), `ThemeSync.tsx`→`ThemeSync.test.tsx` (5), `ThemeToggle.tsx`→`ThemeToggle.test.tsx` (5), `auth.ts`→`auth.test.ts` (3), `AppLayout.tsx`→`AppLayout.test.tsx` (4), `AppProviders.tsx`→`AppProviders.test.tsx` (1 end-to-end provider-tree test). `AppProviders.test.tsx:26-56` is the strongest test in the set — it renders the real `QueryClientProvider > ThemeProvider > AuthProvider > ThemeSync` nesting with only `fetch` stubbed, so a provider reordering fails it. `BrandMark.tsx` has no own suite but is a pure SVG wrapper covered indirectly by `AppLayout.test.tsx:60-65`. The class-only conversions (`SeverityChip`, the three widgets, `ActivityEventIcon`, `StatusBadge`) had no tests before this change and still have none; they are covered by the `conventions.test.ts:113-154` palette guard. See **Minor #4/#5/#6** for three tests that assert less than their comments claim. |
| FE-17 | Mocks updated when services changed | N/A | `find apps packages -name '__mocks__'` returns nothing — this repo has no shared mock directory. `UserAttributes.themePreference` is non-optional, so every `User` fixture had to be updated or the build would break; the three fixtures in scope all carry it (`auth.test.ts:36`, `ThemeSync.test.tsx:22-23`, `AppProviders.test.tsx:39`). |

---

## Points Specifically Requested

### `localStorage` validated everywhere it is read — CONFIRMED

There are exactly two readers, and both reject out-of-range input before it can influence anything:

- `apps/web/src/lib/theme.ts:34-40` — `readCachedTheme()` gates on `isThemePreference(raw)` (`theme.ts:24-26`, allow-list at `theme.ts:22`) and returns `null` otherwise. `ThemeContext.tsx:50` supplies `?? 'system'`. Covered by `theme.test.ts:57-60` and `ThemeContext.test.tsx:52-56`.
- `apps/web/index.html:36-37` — `if (p !== 'light' && p !== 'dark') p = 'system';` narrows to a known-good default rather than rejecting, so a corrupted value and an absent key take the identical path.

Neither path concatenates the stored value into a class name, URL, or markup: `theme.ts:69` toggles the string literal `'dark'`, and `index.html:41` adds the literal `'dark'`. The server value is validated too — `ThemeSync.tsx:41` runs `isThemePreference(serverPreference)` before adopting, tested at `ThemeSync.test.tsx:147-154`.

### Toggle accessible name and keyboard reachability — CONFIRMED

`ThemeToggle.tsx:50-58` renders `<Button type="button">`, which resolves to a native `<button>` (`ui/button.tsx:38`) and is therefore tab-reachable and Enter/Space-activatable with no extra wiring. `aria-label` (`ThemeToggle.tsx:57`) names both the current state and the next action — `"Theme: light. Switch to dark."` — which is what makes a three-way cycle operable without sighted feedback; the icon is `aria-hidden` (`ThemeToggle.tsx:59`) so it does not compete. `AppLayout.test.tsx:71` finds it by role and accessible name, and `ThemeToggle.test.tsx:43-53` pins all three cycle labels.

### Other sub-AA pairings — none found beyond the one already fixed

Grepped every use of the new families (12 sites). Every `-subtle` fill is paired with its matching `-subtle-foreground`:
`SeverityChip.tsx:16,20,24`; `StatusBadge.tsx:8-10`; `MaintenanceQueueView.tsx:47` + `:55,60`.
The remaining text inside the one `bg-danger-subtle` container is `MaintenanceQueueView.tsx:51`, which inherits `--card-foreground` (`222.2 84% 4.9%` light / `210 40% 98%` dark) against `--danger-subtle` — both far above AA. `StatusBadge.tsx:11` (`Inactive`) uses `bg-muted`/`text-muted-foreground`, the pairing `--muted-foreground` was calibrated for, not a `-subtle` fill. The bare tokens (`text-success|warning|danger`) all sit on `--background`/`--card` per `contrast.md`. No new instance of the `text-muted-foreground`-on-`-subtle` mistake exists.

### `conventions.test.ts` genuinely reaches `packages/ui-components` — CONFIRMED

Replayed the walk from `conventions.test.ts:117-126` with the same `resolve(WEB_ROOT, '../../packages/ui-components/src')`: it returns exactly `packages/ui-components/src/StatusBadge.tsx`. The per-root `expect(files.length).toBeGreaterThan(0)` at `conventions.test.ts:137-139` means a moved or renamed directory fails loudly rather than passing vacuously — the right guard given `make fe-test` never runs that package's own suite. See **Minor #6** for two blind spots in the regex/extension filter.

---

## Summary

### Blocking (must fix)

- **FE-02** — `packages/ui-components/src/StatusBadge.tsx:16` builds `className` by template-string interpolation instead of `cn()`. This is the single checklist FAIL and it is **pre-existing and untouched** by this diff; flagged because the file is in scope and the checklist criterion is "zero matches".

### Important (should fix — outside the FE-* IDs, inside the brief's dark-mode/a11y remit)

1. **Focused buttons draw a white halo in dark mode.** `apps/web/src/components/ui/button.tsx:7` applies `focus-visible:ring-offset-2` but never sets `focus-visible:ring-offset-background`. `apps/web/tailwind.config.ts` defines no `ringOffsetColor`, so Tailwind falls back to `#fff` (`node_modules/tailwindcss/lib/corePlugins.js:3837`). Against `--background: 222.2 84% 4.9%` that is a bright 2px white ring between the button edge and the `--ring` ring on every keyboard-focused button in the app — including the `ThemeToggle` this task adds (`ThemeToggle.tsx:50-58`). The other four focusable primitives in the repo all set it correctly (`ui/input.tsx:10`, `ui/textarea.tsx:11`, `ui/select.tsx:17`, `ui/switch.tsx:11`), so `button.tsx` is the lone outlier. `button.tsx` is outside the diff, but making dark mode reachable is what turns a dormant default into a visible defect, so it belongs to this change. One-word fix: add `focus-visible:ring-offset-background` at `button.tsx:7`.

### Non-Blocking (should fix)

2. **The sidebar fill is now a no-op.** `AppLayout.tsx:30` uses `bg-card`, but `--card` and `--background` hold identical values in *both* themes (`index.css:9` vs `index.css:7`; `index.css:55` vs `index.css:53`). The pre-change `bg-gray-50` gave the sidebar a visible tint against a white main area; `bg-card` gives none, so the panel is delineated only by `border-r border-border`. The recorded rationale (avoid `bg-muted` because `--muted` and `--accent` are equal, which would flatten the nav states) correctly rules out `bg-muted` but does not establish that `bg-card` produces a panel. The nav states themselves are fine — `--accent` differs from `--card` in both themes, so active (`AppLayout.tsx:43`) and hover (`AppLayout.tsx:44`) still read. Consider `bg-muted/40` or a dedicated `--sidebar` token if a distinct panel is wanted.

3. **`SeverityChip` "Urgent" now shares its fill with the overdue row it sits in.** `SeverityChip.tsx:16` is `bg-danger-subtle`; `MaintenanceQueueView.tsx:47` fills the enclosing row with the same `bg-danger-subtle`. Before the change these were distinct (`bg-red-100` chip on a `bg-red-50` row). The chip is now separated from its row only by `border-danger-border`, which is ~1.1:1 against the fill in light mode (`0 96.3% 89.4%` vs `0 93.3% 94.1%`). Text stays legible (6.80:1 light / 8.14:1 dark, `contrast.md`), so this is an affordance regression, not an AA failure. Only the urgent severity inside the overdue queue is affected.

4. **`ThemeToggle.test.tsx:96-101` would pass with the behaviour it guards removed.** The comment above it cites FR-TOGGLE-3 ("the icon tracks the PREFERENCE, not the resolved theme"), but the body sets the preference to `system` and asserts only `screen.getByRole('button').querySelector('svg')` is truthy. All three `META` entries (`ThemeToggle.tsx:18-20`) render an svg, so the test passes if `META` were re-keyed on `resolvedTheme` — the exact regression it claims to catch — or if all three icons were identical. A real assertion would render each preference and compare something distinguishing (the `<path d>`, or a `data-icon` attribute).

5. **`AppLayout.test.tsx:63-64` asserts less than it says.** `screen.getByText('MyFleet')` resolves to the brand `<div>` (`AppLayout.tsx:31`), so `.parentElement` is the `<aside>` and `querySelector('svg')` scans the entire sidebar. The test would still pass with the mark moved into the nav list. It does catch outright removal of `BrandMark`, which is most of the value, but "beside the wordmark" is not what it checks.

6. **The palette guard has two blind spots.** `conventions.test.ts:115`'s regex omits `indigo|violet|purple|fuchsia|pink|rose|sky|cyan|teal|lime|stone`, so `bg-indigo-500` would sail through. `conventions.test.ts:120` restricts the walk to `.tsx`, so a `cva` variant map or class-name constant living in a `.ts` file — the common shadcn shape, and the shape `SeverityChip`/`StatusBadge` would take if extracted — is never scanned. Neither is a live violation today (FE-06 is clean), but both weaken the guard that is the sole protection for `packages/ui-components`.

7. **`auth.ts` exports no invalidation helper.** `patterns-react-query.md` ("Invalidation Helper Pattern") says every hook file exports one; `auth.ts` has no `useInvalidateAuth`. Pre-existing shape of the file, not introduced here. Separately, `useUpdateTheme` (`auth.ts:89-107`) has no `onSettled` invalidation, which departs from the documented mutation pattern — but that omission is deliberate and correct: an invalidation would refetch `me` and let `ThemeSync` re-adopt a stale value, which is precisely what FR-PERSIST-5 forbids. The `onMutate`-only shape and the missing `onError` rollback are both sound as designed and documented at `auth.ts:76-88`.

### Design decisions evaluated and accepted

- **`ThemeContext` network-/auth-unaware, bridged by `ThemeSync`** — sound, and the tests prove the payoff: `ThemeContext.test.tsx:23-29` renders the provider with a bare `render()`, no `QueryClientProvider`, no token fixture. `AppProviders.test.tsx` covers the seam the isolation opens up (a provider reordering), which is the right complement.
- **No `onError` rollback in `useUpdateTheme`** — correct. A rollback would restore the pre-change value into `authKeys.me()`, `ThemeSync.tsx:39-43` would see it change and re-adopt it, and the theme would flip under the user. `ThemeToggle.tsx:52-56` owns the toast instead, verified at `ThemeToggle.test.tsx:76-92` (asserts the class *and* the cache write both survive the failure).
- **Duplicated pre-paint logic in `index.html`** — unavoidable; a module import in `<head>` is async by definition. The guard tests are real: `conventions.test.ts:20` interpolates the imported `THEME_STORAGE_KEY` and `:67` the imported `MEDIA_QUERY`, and `:63-68` correctly scopes the media-query check to the script body rather than the whole file (the `theme-color` metas contain the same substring and would have made a whole-file check pass vacuously). `:33-39` and `:43-49` pin synchronicity and the try/catch.
- **`--destructive` untouched, distinct from `danger`** — confirmed: `index.css:21,67` unchanged, and no in-scope file mixes the two.
- **`@types/node` in devDependencies** — type-only, forced by `conventions.test.ts:1-3`'s `node:fs`/`node:url`/`node:path` imports. No runtime dependency added; `apps/web/package.json` `dependencies` is unchanged.
- **`wasSignedIn` set on any authenticated render** (`ThemeSync.tsx:36-38`) rather than inside the adopt effect — correct, and the reasoning in the comment holds: a user whose stored `themePreference` failed validation would otherwise sign out without ever setting the flag, stranding a stale override that would suppress adoption on the next sign-in too. `ThemeSync.test.tsx:104-143` exercises the full sign-in → override → sign-out → sign-in cycle with three distinct values, so it genuinely proves the override was cleared rather than coincidentally matching.
