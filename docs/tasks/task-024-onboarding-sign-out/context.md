# Task 024 — Implementation Context

Companion to `plan.md`. Everything an implementer needs that is not a plan step:
where the code lives, why the decisions went the way they did, and what will
bite.

---

## 1. The problem in one paragraph

`RequireAuth` redirects an authenticated user with no `activeFleetId` to
`/onboarding`, exempting only `/onboarding` and `/invites/:token/accept`
(`FLEETLESS_ROUTES`, `RequireAuth.tsx:13`). Both of those pages are routed
**outside** `AppLayout` (`App.tsx:36-53`), so neither renders `FrameHeader` and
neither renders `ProfileMenu` — the only sign-out control in the product. The
user cannot navigate anywhere and cannot leave the account. This task adds a
control; it rebuilds none of the sign-out mechanics.

---

## 2. Key files

### Read before starting

| File | Why |
|---|---|
| `apps/web/src/context/AuthContext.tsx:55-60` | `logout()` — the whole behaviour being wired up: `logoutRequest()` → `clearAccessToken()` → `setHasToken(false)` → `queryClient.removeQueries(authKeys.all)` |
| `apps/web/src/components/RequireAuth.tsx:39-52` | Produces the `/login` redirect on its own once `hasToken` flips. This is why the new component needs no navigation code |
| `apps/web/src/components/frame/ProfileMenu.tsx` | The existing `useAuth()` call site and the shape the new component follows |
| `apps/web/src/components/frame/identityLines.ts` | The helper the new one deliberately disagrees with — **do not modify** |
| `apps/web/src/lib/hooks/api/auth.ts:65-71` | `logoutRequest()` — raw `fetch`, `credentials: 'include'`, and the `.catch(() => undefined)` that makes today's rejection branch unreachable |
| `apps/web/src/test/renderWithProviders.tsx` | Wraps `QueryClientProvider` + `MemoryRouter` only. **No `AuthProvider`** — this is the cause of the Task 4 breakage |
| `apps/web/src/components/frame/ProfileMenu.test.tsx:8-40` | The `mockAuth` + `vi.mock('.../AuthContext')` pattern the new tests copy |

### Created / modified

See `plan.md` § File Structure. Nine files: four new under
`apps/web/src/components/auth/` (which does not exist yet), one new page test,
two page edits, one page-test edit.

---

## 3. Decisions already made (do not relitigate)

**One shared component, not inline markup on each page** (FR-SHARED-1). The
in-flight guard, the error branch, and the a11y wiring are all logic;
duplicating them across two pages is how you get two subtly different sign-out
buttons. It also means `task-022-signout-failure-handling` — in flight, rewrites
this same call path — has **one** new call site to reconcile, not two.

**Local `useState`, not `useMutation`** for the in-flight flag. `logout()` calls
`queryClient.removeQueries` on the auth keys inside its own body, so a mutation
would be tearing down cache belonging to the client tracking it. One
`useState<boolean>` is the honest size of a call with one caller, no cache to
invalidate, and no retry policy.

**`signingOut` is never reset on success.** On success the component unmounts —
`hasToken` flips, `RequireAuth` re-renders, `<Navigate>` replaces the subtree. A
`finally { setSigningOut(false) }` would be a no-op or a post-unmount write, and
would briefly re-enable the button mid-redirect. Reset happens only in `catch`,
where the user is still signed in and must be able to retry.

**Email before display name** (FR-IDENT-2), the opposite of `identityLines()`.
The profile-menu header is a greeting; this footer is a disambiguation read by
someone who may be in the wrong account. Two Google accounts routinely share a
display name; the email is the identifying field. Collapsing the two helpers
behind one parameterised function is explicitly deferred (PRD §9.3).

**`null`, not `''` or `'Account'`**, when there is nothing to show. The omission
becomes a type-level fact at the call site instead of a `!== ''` convention.

**Identical copy on both pages.** PRD §9.2 leaves the door open to
`Wrong account? Sign out` on the invite error screen. Taking it now means a
`variant` prop on a component whose entire value is having none. The accessible
name `Sign out` — the only part that is a requirement — is the same either way.

**Mock `AuthContext` in `OnboardingPage.test.tsx`; do NOT add `AuthProvider` to
`renderWithProviders`.** The latter would fix four tests and change the wrapper
every component test in the repo uses, mounting a live `useMe` query in each one
whose `enabled: !!getAccessToken()` makes behaviour depend on ambient
`localStorage`.

---

## 4. Things that will bite

**R1 — The four existing `OnboardingPage` tests break. Certain, not probable.**
`renderWithProviders` has no `AuthProvider`, so the moment `OnboardingPage`
renders `SignedInFooter` all four throw
`useAuth must be used within an AuthProvider`. Task 4 Step 1 handles it up
front. Do not discover this at `make ci`.

**`useParams` in `InviteAcceptPage`.** Rendering the page bare leaves `token`
undefined, so the effect returns early and the page sits in `pending` forever.
The new test mounts it under a real `<Route path="/invites/:token/accept">`.

**Clicking a disabled button.** jsdom does not dispatch `click` on a disabled
form control, and user-event v14 suppresses it too — so the double-click test
works without CSS. Do not try to assert on `pointer-events`; `css: false` in
`vite.config.ts` means jsdom sees no stylesheet at all.

**Unhandled-rejection detection.** Node reports an unhandled rejection at a
microtask-queue checkpoint, so the assertion needs one macrotask boundary
(`await new Promise((r) => setTimeout(r, 0))`) before checking. Use
`process.on('unhandledRejection')`; a jsdom `window` listener does not fire.

**`ApiClient` response stubs** (design R3 — resolved). `ApiClient.request` reads
`res.status === 204 ? null : await res.json()` and throws unless `res.ok`
(`packages/shared-ts/src/apiClient.ts`), and `apiClient` is built with
`baseUrl: ''` (`lib/api/client.ts:17-21`). So a stub needs only
`{ ok, status, json }` and the URL is the literal `/api/auth/me`. No fallback to
cache-seeding is needed.

**`useMe`'s `enabled` is read on first render.** `setAccessToken('token-123')`
must run **before** `render()` in `signOutFlow.test.tsx`, not after.

**Merge overlap with task-022.** Both branch from `main` and both touch the
sign-out path. 022 changes `logoutRequest()`/`AuthContext` and `ProfileMenu`;
this task changes neither and adds a call site that already handles a rejection.
The one thing that would create real conflict — this task "fixing"
`logoutRequest`'s `.catch(() => undefined)` — is explicitly out of scope.
`ProfileMenu`'s `void logout()` is likewise **not** migrated here even though
FR-SIGNOUT-2 makes it look inconsistent.

---

## 5. Discovered during planning — pre-existing, out of scope

**`createErrorFromUnknown` drops `detail` when handed an already-wrapped
`ApiError`.** Verified empirically, not inferred:

```
new ApiError(409, 'conflict', 'conflict-title', 'wrong email address')
  → createErrorFromUnknown(…)
  → { message: 'conflict-title', detail: undefined, status: 0 }
```

`createErrorFromUnknown` reads `detail` off a raw `{ status, body }` envelope
(`packages/shared-ts/src/errors.ts:22-33`); an `ApiError` instance has no
`.body`, so it falls to the `e instanceof Error` branch and loses `detail`,
`status`, and `code`.

Consequence: `InviteAcceptPage.tsx:37`'s `apiError.detail || apiError.message`
resolves to `message` for every real API failure, so the user reads the
envelope's `title` — which the comment at that line says is the literal
`"conflict"` for every invite conflict — instead of the `detail` that
distinguishes already-accepted, expired, and wrong-account. The same re-wrap
happens in `useAcceptInvite`'s `onError` toast (`lib/hooks/api/invites.ts`).

**Not fixed here.** FR-INVITE-3 forbids changing this screen's error handling,
and the fix belongs in `@myfleet/shared-ts` (an early
`if (e instanceof ApiError) return e`), which is a shared-package change with
its own blast radius. `InviteAcceptPage.test.tsx` asserts current behaviour with
a comment recording why. **Worth a follow-up task** — it is a user-visible copy
bug on the exact screen this task is trying to make less confusing.

---

## 6. Verification

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci     # lint-check, vet, test, build, fe-test, fe-build, manifests, carfax-template
```

Single test file during development:

```sh
cd apps/web && npx vitest run src/components/auth/SignedInFooter.test.tsx
```

No `deploy/k8s` change, so no `kustomize build` / `--dry-run=server` pass is
needed for this task.

**jsdom cannot see CSS.** FR-ONBOARD-3 (visual subordination to `Create Fleet`)
and FR-IDENT-5 (truncation) are verified by eye in a real browser per
`docs/runbooks/local-debugging.md`, not by the suite. That check is Task 6
Step 4 and it is not optional.

Several plan steps deliberately break the implementation to prove a test goes
red before restoring it (Task 2 Steps 5-6, Task 3 Step 3, Task 5 Step 5). These
exist because assertions about disabled states, caught rejections, and
"issues a request" pass trivially against the wrong implementation. Do not skip
them.

---

## 7. Out of scope

- `RequireAuth`, `FLEETLESS_ROUTES`, the redirect rules.
- `AuthContext.logout()`, `logoutRequest()`, the auth-service logout handler.
- `ProfileMenu` — including its `void logout()`.
- `identityLines()` — no reuse, no modification, no collapsing with
  `accountLabel()`.
- The `createErrorFromUnknown` re-wrap bug in §5.
- A "switch account" chained logout→login flow.
- Any `AppLayout` / `FrameHeader` / `ProfileMenu` on the fleetless pages.
- Any backend, API, data-model, or `deploy/k8s` change.
