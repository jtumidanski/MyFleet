# Frontend Audit — task-024-onboarding-sign-out

- **Audit Scope:** 9 changed files under `apps/web/src` (branch `task-024-onboarding-sign-out` vs. merge base `f782de2`)
- **Guidelines Source:** frontend-dev-guidelines skill
- **Date:** 2026-08-03
- **Build:** PASS (per prior verified run — not re-run this pass)
- **Tests:** 750 passed, 0 failed (per prior verified run — not re-run this pass)
- **Overall:** PASS

## Build & Test Results

Per task instructions, `make ci` was already verified green for this branch (lint-check, vet, test, build, fe-test, fe-build, manifests, carfax-template; 93 test files / 750 tests in `apps/web`) and a real-browser Playwright check confirmed visual subordination and truncation behavior. This audit did not re-run those checks — reading raised no doubt that would justify a focused re-run.

## File Inventory

- `apps/web/src/components/auth/accountLabel.ts` — Other (pure helper function, JSON:API `User` → label)
- `apps/web/src/components/auth/accountLabel.test.ts` — Test
- `apps/web/src/components/auth/SignedInFooter.tsx` — Component (`components/auth/`, new shared presentational+logic component)
- `apps/web/src/components/auth/SignedInFooter.test.tsx` — Test
- `apps/web/src/components/auth/signOutFlow.test.tsx` — Test (integration; real `AuthProvider` + `RequireAuth`)
- `apps/web/src/pages/OnboardingPage.tsx` — Page (6-line diff: mounts `SignedInFooter`)
- `apps/web/src/pages/OnboardingPage.test.tsx` — Test
- `apps/web/src/pages/InviteAcceptPage.tsx` — Page (7-line diff: mounts `SignedInFooter` on error state only)
- `apps/web/src/pages/InviteAcceptPage.test.tsx` — Test

No `services/api/`, `lib/hooks/api/`, `lib/schemas/`, or `types/` files were changed by this branch.

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Grepped all 9 files for `: any` / `as any` — zero matches. |
| FE-02 | No manual class concatenation | PASS | Grepped for `className={"` — zero matches. `SignedInFooter.tsx:52,69` use plain string literals or none; no concatenation anywhere in scope (`cn()` isn't even needed since no conditional classes appear). |
| FE-03 | No direct API client calls in components | PASS | Grepped for `lib/api/client` — zero matches. `SignedInFooter.tsx:4` imports `useAuth` from `context/AuthContext`, not the API client directly. |
| FE-04 | No inline Zod schemas in components | PASS | Grepped for `z.object(` / `z.string(` — zero matches. No schema changes in this diff. |
| FE-05 | No spinners for content loading | PASS | Grepped for `animate-spin` — zero matches in the 5 non-test new/changed component/page lines added by this diff (`OnboardingPage.tsx`'s pre-existing spinner at line 126 is on the submit button, unchanged by this diff; `InviteAcceptPage.tsx`'s pending-state spinner at line 49 is pre-existing and untouched). |
| FE-06 | No hardcoded colors | PASS | Grepped for `bg-\|text-(white\|black\|gray-N\|red-N\|...)` — zero matches. `SignedInFooter.tsx:52,54,56,69` use only semantic classes (`text-muted-foreground`). Repo-wide enforcement also lives in `apps/web/src/test/conventions.test.ts:113-133`, which scans all `.tsx` under `src` (including this new file) and is part of the already-green `fe-test` run. |
| FE-07 | No state mutation | PASS | Grepped for `.push(`/`.splice(`/`.sort(` — zero matches. `SignedInFooter.tsx:30,37,47` use `useState`/`setSigningOut` scalar boolean updates only. |
| FE-08 | No default exports for components | PASS | Grepped for `export default` — zero matches. `SignedInFooter.tsx:28` (`export function SignedInFooter()`), `accountLabel.ts:23` (`export function accountLabel`) — named exports throughout. |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | `SignedInFooter.tsx:45-48` — `catch (err) { toast.error(createErrorFromUnknown(err).message \|\| 'Could not sign out'); setSigningOut(false); }`. `OnboardingPage.tsx:94-97` (pre-existing, unchanged by diff) — same pattern. `InviteAcceptPage.tsx:31-40` (pre-existing `onError`, unchanged by diff) — same pattern, `detail \|\| message` variant (see known pre-existing bug note below). |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `accountLabel.ts:24,27` reads `user?.attributes.email` / `user?.attributes.displayName` against the existing `User = JsonApiResource<UserAttributes>` shape (`apps/web/src/types/models/user.ts:18`, unchanged). No new model introduced. |
| FE-11 | Service extends `BaseService` | N/A | No `services/api/` files changed by this branch. |
| FE-12 | Query key factory uses `as const` | N/A | No `lib/hooks/api/` files changed by this branch. `SignedInFooter.tsx` calls `logout()` from `AuthContext` (pre-existing), not a new query hook. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | PASS | `OnboardingPage.tsx:83-86` — `useForm<CreateFleetInput>({ resolver: zodResolver(createFleetSchema), ... })`, pre-existing and untouched by this diff; the 6 added lines (`OnboardingPage.tsx:13,137` plus the comment block) only mount `SignedInFooter`, which is not a form. |
| FE-14 | Schema in `lib/schemas/` with inferred type | N/A | No schema files touched; `createFleetSchema` (imported, pre-existing) already lives in `lib/schemas/fleet.ts` per `OnboardingPage.tsx:7`. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | `SignedInFooter.tsx:61-78` renders the sign-out control via the shared `<Button variant="link" ...>` primitive. `components/ui/button.tsx:7` bakes `cursor-pointer` into the base CVA class string applied to every variant (`inline-flex ... cursor-pointer`), so the `link` variant inherits it without any extra `className` needed. No other interactive/clickable elements (divs, popover triggers, etc.) were introduced by this diff. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | `SignedInFooter.tsx` ↔ `SignedInFooter.test.tsx` (11 cases: identity line, fallback, omission, a11y name, no-heading, keyboard, click, double-click guard, error/toast, generic-message fallback, null-user no-render) + `signOutFlow.test.tsx` (3 integration cases with real `AuthProvider`/`RequireAuth`). `accountLabel.ts` ↔ `accountLabel.test.ts` (9 cases). `OnboardingPage.tsx` ↔ `OnboardingPage.test.tsx` (new cases at lines 138-160 covering the added footer). `InviteAcceptPage.tsx` ↔ `InviteAcceptPage.test.tsx` (new case at lines 96-106 plus regression cases at 130-146 asserting the footer is absent on pending/success states). |
| FE-17 | Mocks updated when services changed | N/A | No `services/api/` interfaces changed; repo has no shared `__mocks__/` directory convention (confirmed: `find apps/web/src -iname __mocks__` returns nothing), so per-test `vi.mock('../../context/AuthContext', ...)` calls are the established pattern and are present and consistent with `ProfileMenu.test.tsx`/`LoginPage.test.tsx`/`AppLayout.test.tsx`, as referenced in `SignedInFooter.test.tsx:9-11` and `OnboardingPage.test.tsx:28-31`. |

## Deliberate Decisions Reviewed (no FE-* violation found)

- **`accountLabel` orders email → displayName → null** (`accountLabel.ts:6`), disagreeing with `components/frame/identityLines.ts:19-21` (displayName → email → 'Account'). No FE-* check requires a single shared identity-formatting helper; `patterns-types.md`'s "Helper Functions on Models" pattern explicitly sanctions small standalone functions colocated with a model. Not a violation.
- **Local `useState` for `signingOut`** (`SignedInFooter.tsx:30`) instead of `useMutation`. `patterns-react-query.md` mandates React Query for *server state*; the actual network call (`logoutRequest()`) is encapsulated inside `AuthContext.logout()` (`context/AuthContext.tsx:52-57`), which already performs its own `queryClient.removeQueries`. The component's boolean is local UI state (button disablement), not server state — no FE-* violation.
- **`signingOut` never reset on success** (`SignedInFooter.tsx:40-44`) — UX/architecture judgment call, not a checklist item.
- **Tests mock `AuthContext` rather than wrapping `AuthProvider`** — matches the established pattern cited in-line (`SignedInFooter.test.tsx:9-11`); `testing-guide.md`'s "Common Mocks" section sanctions mocking context/service modules generally.

## Known Pre-Existing Issue (out of scope, not scored)

`InviteAcceptPage.tsx:38` — `apiError.detail || apiError.message` yields the envelope's `title` for every invite conflict because `createErrorFromUnknown` (`packages/shared-ts/src/errors.ts:23-37`) only reads `detail` off a raw `{status, body}` envelope, and a re-wrapped `ApiError` instance falls to the `instanceof Error` branch (line 35) with `detail` undefined. Confirmed by reading `errors.ts` directly. Per instructions this predates the branch, is spec-excluded from this screen's error-handling, and is already asserted as current behavior with an explanatory comment in `InviteAcceptPage.test.tsx:108-117`. Not scored against this change.

## Summary

### Blocking (must fix)
- None.

### Non-Blocking (should fix)
- None. All FE-* checks that apply to the changed files pass with direct file:line evidence; checks not applicable to this diff (FE-11, FE-12, FE-14, FE-17) are correctly N/A because no service/hook/schema files were touched.
