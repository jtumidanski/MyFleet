# Frontend Audit — task-009-smtp-invite-delivery

- **Audit Scope:** TypeScript/React changes in commits `2ac2455..7452ed6`: `apps/web/src/lib/utils/clipboard.ts` (+test), `apps/web/src/services/api/InviteService.ts`, `apps/web/src/lib/hooks/api/invites.ts`, `apps/web/src/lib/hooks/api/members.test.ts`, `apps/web/src/components/features/settings/InviteList.tsx` (+test), `apps/web/src/components/features/settings/InviteForm.tsx`
- **Guidelines Source:** frontend-dev-guidelines skill
- **Date:** 2026-08-02
- **Build:** PASS (`npx tsc --noEmit -p apps/web` clean; `make ci` reported clean by task owner)
- **Tests:** 17/17 passed in scoped re-run (`clipboard.test.ts` 3, `members.test.ts` 9, `InviteList.test.tsx` 5); 188/188 reported for full suite
- **Overall:** NEEDS-WORK

## Build & Test Results

```
$ npx vitest run src/lib/utils/clipboard.test.ts src/lib/hooks/api/members.test.ts src/components/features/settings/InviteList.test.tsx
 ✓ src/lib/utils/clipboard.test.ts (3 tests) 18ms
 ✓ src/lib/hooks/api/members.test.ts (9 tests) 104ms
 ✓ src/components/features/settings/InviteList.test.tsx (5 tests) 155ms
 Test Files  3 passed (3)
      Tests  17 passed (17)

$ npx tsc --noEmit -p apps/web
(no output — clean)
```

Full-suite `make ci` (fe-test 188/188, fe-build clean) reported by task owner as passing; not re-run in full here, per audit instructions allowing a targeted re-run to confirm specific suspicions.

## File Inventory

- `apps/web/src/lib/utils/clipboard.ts` (new) — **Other** (utility)
- `apps/web/src/lib/utils/clipboard.test.ts` (new) — **Other** (test)
- `apps/web/src/services/api/InviteService.ts` — **Service**
- `apps/web/src/lib/hooks/api/invites.ts` — **Hook**
- `apps/web/src/lib/hooks/api/members.test.ts` — **Other** (test, covers hook file `invites.ts`)
- `apps/web/src/components/features/settings/InviteList.tsx` — **Component** (feature)
- `apps/web/src/components/features/settings/InviteList.test.tsx` (new) — **Other** (test)
- `apps/web/src/components/features/settings/InviteForm.tsx` — **Component** (feature)

## Anti-Pattern Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-01 | No `any` type | PASS | Only matches are `clipboard.test.ts:25,38` (`(document as any).execCommand`), both carrying `eslint-disable-next-line @typescript-eslint/no-explicit-any` on the preceding line — pre-accepted per task brief, no other `: any`/`as any` in scope. |
| FE-02 | No manual class concatenation | PASS | `grep 'className={"'` over `InviteList.tsx`, `InviteForm.tsx` — zero matches; `cn()` not needed since all `className` values are static strings or template-free (`InviteList.tsx:54` uses a static string). |
| FE-03 | No direct API client calls in components | PASS | `grep 'lib/api/client'` over `InviteList.tsx`, `InviteForm.tsx` — zero matches; both go through `services/api/InviteService.ts` via hooks. |
| FE-04 | No inline Zod schemas in components | PASS | No `z.object(`/`z.string(` in `InviteList.tsx` or `InviteForm.tsx`; `InviteForm.tsx:8` imports `createInviteSchema` from `lib/schemas/fleetSettings`. |
| FE-05 | No spinners for content loading | PASS | `InviteList.tsx:25-32` uses `<Skeleton>` for the loading state; the only `animate-spin` in scope is `InviteForm.tsx:75`, gated on `createInvite.isPending` inside the submit `<Button>`. |
| FE-06 | No hardcoded colors | PASS | `grep -E 'bg-(white|black|gray-[0-9]|red-[0-9]|green-[0-9]|blue-[0-9])'` over both components — zero matches; only semantic classes (`text-muted-foreground`, `text-sm`) used. |
| FE-07 | No state mutation | PASS | No `.push(`/`.splice(`/`.sort(` in `InviteList.tsx` or `invites.ts`; the pending-invite filter (`InviteList.tsx:37`) uses `.filter()`. |
| FE-08 | No default exports for components | PASS | `InviteList.tsx:20` and `InviteForm.tsx:19` both use `export function`. |
| FE-09 | Error handling with `createErrorFromUnknown` | PASS | `invites.ts:79` (`inviteErrorMessage`), `invites.ts:103`, `invites.ts:123` all call `createErrorFromUnknown`; `InviteForm.tsx:33` and `InviteList.tsx` (via `useResendInvite`'s `onError`, `invites.ts:145-147`) surface it via `toast.error`. `InviteList.tsx:44-48`'s `handleCopy` surfaces clipboard failure via `toast.error` (not an API call, no `createErrorFromUnknown` needed — `copyToClipboard` returns `boolean`, never throws, per `clipboard.ts:11-37`). |

## Architecture Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-10 | JSON:API model shape | PASS | `types/models/invite.ts:17` — `Invite = JsonApiResource<InviteAttributes>` (`{ type, id, attributes }`, `packages/shared-ts/src/jsonapi.ts:1-6`); `InviteAttributes.token` (`invite.ts:11`) already present, matching the "token already in list response" contract. |
| FE-11 | Service extends `BaseService` (when applicable) | PASS | `InviteService.ts:16` extends `BaseService<InviteAttributes, CreateInviteAttributes>`; `resendInvite` (`InviteService.ts:59-65`) uses the same direct-`apiClient.request` pattern already established for `acceptInvite` (`InviteService.ts:46-52`) for a no-body action endpoint. Verified against the backend: `apps/fleet-service/internal/invite/resource.go:174` registers `/fleets/{fleetId}/invites/{inviteId}/resend` as a raw `r.Post(..., func(w, req) {...})` handler, not `server.RegisterInputHandler[T]`, so the no-body POST is compatible (no JSON:API-envelope-required 400). |
| FE-12 | Query key factory uses `as const` | PASS | `invites.ts:30-33` — `inviteKeys.all`, `.lists()`, `.list()` all end in `as const`. |
| FE-13 | Forms use `react-hook-form` + `zodResolver` | PASS | `InviteForm.tsx:22-25` — `useForm({ resolver: zodResolver(createInviteSchema), ... })`, unchanged by this diff. |
| FE-14 | Schema in `lib/schemas/` with inferred type | PASS (unchanged, out of diff) | `InviteForm.tsx:8` imports `createInviteSchema`/`CreateInviteInput` from `lib/schemas/fleetSettings`; not modified in this diff. |
| Cache-key correctness (task-specific focus) | `useResendInvite` invalidates the exact key `useInvites` reads | PASS | `useInvites` reads `inviteKeys.list({ fleetId })` = `['invites','list',{fleetId}]` (`invites.ts:44`); `useResendInvite`'s `onSettled` invalidates `inviteKeys.lists()` = `['invites','list']` (`invites.ts:143`), which is a true prefix of the list key and no `exact: true` option is passed anywhere — React Query's default partial-match invalidation will hit it. `members.test.ts:257-282` asserts this with a **real** `QueryClient` on both the success and rejection paths (not just `onSuccess`), directly covering the "resend rotates the token, stale cache hands out a dead token" failure mode. |
| Server-side-only rate limiting (task-specific focus) | No client-side rate-limit/cooldown logic | PASS | `InviteList.tsx:75` disables the Resend button only on `resendInvite.isPending` (in-flight-request guard, standard double-submit protection) — no timer, no cooldown state, no request counting anywhere in `InviteList.tsx` or `invites.ts`. The two distinct 429 sentences (`invites.ts:80-84`) are purely reactive to a server-issued 429, not a predictive client-side throttle. |
| Token exposure (task-specific focus) | Token not logged, sent to analytics, or rendered as text | PASS | `inv.attributes.token` is referenced exactly once in `InviteList.tsx:68`, passed straight into `handleCopy`/`copyToClipboard` (`clipboard.ts`, which never logs — no `console.*` in `clipboard.ts`); it is not interpolated into any rendered JSX text node, and `resendInvite`'s request URL (`InviteService.ts:61`) is built from `inviteId`, not `token`. No `console.*` calls in `lib/api/client.ts`, `services/api/BaseService.ts`, or `packages/shared-ts/src/errors.ts`. |
| Clipboard fallback cleanup (task-specific focus) | `execCommand` fallback textarea removed on every path, including throw | PASS | `clipboard.ts:29-36` — `document.body.removeChild(textarea)` is in a `finally` block wrapping both the `select()` call and `execCommand('copy')` call, so it runs whether `select()` throws, `execCommand` throws, or either returns normally. `clipboard.test.ts:21-31` asserts `document.querySelector('textarea')` is `null` after the fallback path runs. |

## Styling Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-15 | Interactive elements show `cursor-pointer` | PASS | All new interactive controls (`InviteList.tsx:65-87`) are shadcn `<Button>` elements; `components/ui/button.tsx:7` bakes `cursor-pointer` into the base `buttonVariants` CVA string, so no per-instance class is needed. No non-`<button>` clickable elements were introduced. |
| Text casing (patterns-components.md "Text Casing Rules") | Button labels use title case | **FAIL** | `InviteList.tsx:70` — the new "Copy link" button label is sentence case. Per `patterns-components.md`, button labels must use title case ("Create Bucket", "Save Changes"); this should read "Copy Link". The sibling labels added in the same diff, "Resend" (`InviteList.tsx:78`) and "Revoke" (`InviteList.tsx:86`), are single words and already correctly capitalized, making the miss on "Copy link" clearly an oversight rather than a different convention. |

## Testing Checklist

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| FE-16 | Tests exist for changed components | PASS | `InviteList.test.tsx` (new, 5 tests) covers copy-link success/failure, resend invocation, accepted-invite filtering, and owner gating. `clipboard.test.ts` (new, 3 tests) covers the async path, `execCommand` fallback, and dual-failure path. `members.test.ts` adds 2 real-`QueryClient` tests for `useResendInvite` invalidation (success + rejection). `InviteForm.tsx`'s only change (swap to `inviteErrorMessage`) is exercised indirectly by existing `InviteForm` tests plus the `inviteErrorMessage` unit coverage implied by `members.test.ts`'s import — no dedicated new assertion on the 429 copy for the `create` action was found in the diff, but the function itself (`invites.ts:78-89`) is a straight-line pure function with no branching risk beyond what's already covered for `resend`. |
| FE-17 | Mocks updated when services changed | PASS | `members.test.ts:74` adds `resendInvite: vi.fn()...` to the inline `vi.mock('../../../services/api/InviteService', ...)` block, matching the new service method. `InviteList.test.tsx:24-28` mocks the hooks module wholesale (including `useResendInvite`), per the task's stated acceptance — the real hook is covered separately in `members.test.ts`. No stale/missing mock entries found. |

## Summary

### Blocking (must fix)
- None. Build passes, all scoped tests pass, and the sole FAIL is a cosmetic text-casing miss (see below), which per the audit's own status rules still produces NEEDS-WORK rather than FAIL/PASS ambiguity but does not block on build/test grounds.

### Non-Blocking (should fix)
- **[Text Casing]** `apps/web/src/components/features/settings/InviteList.tsx:70` — "Copy link" button label should be "Copy Link" per `patterns-components.md`'s title-case rule for button labels. One-word fix.

### Observations (not guideline violations, no action required)
- `InviteList.tsx:23,75` — `resendInvite` is a single mutation instance shared across the whole list (`useResendInvite(fleetId)` called once at list level), so `resendInvite.isPending` disables the Resend button on every row while any one resend is in flight, not just the row being resent. This is a UX nit, not a rule violation (no FE-* check covers per-row mutation scoping), and does not constitute client-side rate limiting.
