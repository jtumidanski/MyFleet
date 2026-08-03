# Sign Out From The Fleetless Pages — Design

Version: v1
Status: Approved for planning
Created: 2026-08-03
Inputs: `prd.md` (v1), `ux-flow.md`

---

## 1. Shape of the change

The mechanics of signing out already exist and are explicitly out of scope
(PRD §2 non-goals). What this task adds is a *mount point*: one presentational
component, rendered by two pages that live outside `AppLayout`, whose only
behaviour is to call the `logout()` it already gets from `useAuth()`.

Everything downstream of that call is existing machinery:

```
SignedInFooter  ──onClick──▶  useAuth().logout()          (AuthContext.tsx:55)
                                  │
                                  ├─▶ logoutRequest()      POST /api/auth/logout
                                  ├─▶ clearAccessToken()
                                  ├─▶ setHasToken(false)   ◀── the only thing that matters
                                  └─▶ queryClient.removeQueries(authKeys.all)
                                          │
                                          ▼
                              AuthProvider re-renders
                              isAuthenticated → false
                                          │
                                          ▼
                              RequireAuth returns <Navigate to="/login" replace>
                                                          (RequireAuth.tsx:39-43)
```

No navigation code is added anywhere on that path. That is FR-SIGNOUT-3, and it
is what keeps this task small: the redirect is a *consequence* of the context
update, not something the new component arranges.

### 1.1 New and touched files

| File | Status | Purpose |
|---|---|---|
| `apps/web/src/components/auth/accountLabel.ts` | new | Pure `email → displayName → null` resolver |
| `apps/web/src/components/auth/accountLabel.test.ts` | new | Unit tests for the resolver |
| `apps/web/src/components/auth/SignedInFooter.tsx` | new | The affordance (FR-SHARED-1) |
| `apps/web/src/components/auth/SignedInFooter.test.tsx` | new | Component behaviour, a11y, error/in-flight |
| `apps/web/src/components/auth/signOutFlow.test.tsx` | new | Integration: real provider, real guard, real request |
| `apps/web/src/pages/OnboardingPage.tsx` | edit | Mount the footer (FR-ONBOARD-1) |
| `apps/web/src/pages/InviteAcceptPage.tsx` | edit | Mount the footer in the error branch (FR-INVITE-1) |
| `apps/web/src/pages/OnboardingPage.test.tsx` | edit | Add the `useAuth` mock — see §6.1, this is not optional |
| `apps/web/src/pages/InviteAcceptPage.test.tsx` | new | The page currently has no test file at all |

`src/components/auth/` does not exist yet; this task creates it. The directory
name is deliberate: `components/frame/` is "things that live in the app shell",
and the whole point of this component is that it lives *outside* the shell, so
it does not belong there.

`identityLines.ts`, `RequireAuth.tsx`, `AuthContext.tsx`, `lib/hooks/api/auth.ts`
and `renderWithProviders.tsx` are all unmodified.

---

## 2. Component design

### 2.1 `accountLabel(user): string | null`

```ts
// apps/web/src/components/auth/accountLabel.ts
export function accountLabel(user: User | null | undefined): string | null
```

Precedence **email → displayName → `null`**, both values trimmed, whitespace-only
treated as absent (FR-IDENT-2). `null` — not `''`, not `'Account'` — is the
signal that the identity line must be omitted entirely (FR-IDENT-3). A nullable
return makes the omission a type-level fact at the call site instead of a
`!== ''` convention the next editor has to notice.

The file carries a comment recording *why* it disagrees with
`components/frame/identityLines.ts`, which orders `displayName → email →
"Account"` (FR-IDENT-4): the profile-menu header is a greeting, where the
friendly name is the right first line and `"Account"` is an acceptable
placeholder; this footer is a *disambiguation* shown to someone who may be in
the wrong account, where the email is the identifying field, a display name is
not unique, and a placeholder actively defeats the purpose. Deduplicating them
behind one parameterised helper is deferred (PRD §9.3) and would be a mistake
to do now: the two would share a signature and immediately disagree on every
axis that matters.

### 2.2 `SignedInFooter`

No props (FR-SHARED-2). Reads `{ user, logout }` from `useAuth()`, exactly as
`ProfileMenu` does. Returns `null` when `user` is falsy (FR-SHARED-3).

```tsx
const [signingOut, setSigningOut] = useState(false);

const signOut = async () => {
  setSigningOut(true);
  try {
    await logout();
    // No setSigningOut(false), and no navigation — see §2.3.
  } catch (err) {
    toast.error(createErrorFromUnknown(err).message || 'Could not sign out');
    setSigningOut(false);
  }
};
```

Markup:

```tsx
<div className="flex w-full max-w-md flex-col items-center gap-1 text-sm">
  {label !== null && (
    <p className="max-w-full truncate text-muted-foreground">Signed in as {label}</p>
  )}
  <p className="text-muted-foreground">
    Not you?{' '}
    <Button
      type="button"
      variant="link"
      size="sm"
      className="h-auto p-0 align-baseline text-muted-foreground underline"
      disabled={signingOut}
      onClick={() => void signOut()}
    >
      Sign out
    </Button>
  </p>
</div>
```

Notes on the markup, each tied to a requirement:

- `variant="link"` + `h-auto p-0` gives a real `<button>` with no filled surface
  and no button-sized box, so it reads as subordinate to `Create Fleet`
  (FR-ONBOARD-3, FR-A11Y-1). `text-muted-foreground` overrides the variant's
  `text-primary` for the same reason.
- `Not you?` is a sibling text node inside the surrounding `<p>`, so it is not
  part of the button's accessible name (FR-A11Y-2). `<button>` is phrasing
  content, so nesting it in a `<p>` is valid.
- `truncate` on the identity line, with width bounded by the `max-w-md` column,
  matches `ProfileMenu.tsx:58` (FR-IDENT-5).
- `disabled` is the real attribute, so it reaches assistive technology, and
  `buttonVariants`' base class already carries `disabled:pointer-events-none
  disabled:opacity-50` (FR-A11Y-4, FR-SIGNOUT-5).
- No `<h*>` anywhere (FR-A11Y-3). No hardcoded palette classes, which
  `src/test/conventions.test.ts` enforces repo-wide.

`onClick={() => void signOut()}` is not the thing FR-SIGNOUT-2 forbids. That
requirement is about the *promise from `logout()`* being awaited rather than
floated — it is, inside `signOut`, which is why the rejection can be caught at
all. `void` on the outer handler is only there because a DOM event handler
returning a promise is a lint error; the rejection has already been handled one
frame down, so nothing escapes (FR-SIGNOUT-4).

### 2.3 Why `signingOut` is never reset on success

On success the component unmounts: `hasToken` flips, `RequireAuth` re-renders,
and `<Navigate>` replaces the whole subtree. A `finally { setSigningOut(false) }`
would therefore be either a no-op or a post-unmount state write, and — more to
the point — it would briefly re-enable the button during the redirect. Leaving
the flag latched is the correct state: the control is disabled from the click
until the component ceases to exist. Reset happens only on the error path, where
the user is still signed in and must be able to retry.

---

## 3. Alternatives considered

### 3.1 One shared component vs. inline markup on each page

**Chosen: one component (FR-SHARED-1).** The PRD mandates it, and §1.2 gives the
reason — task-022 rewrites this same call path, and one new call site is one
reconciliation instead of two. Independently of 022 it is the right call: the
in-flight guard, the error branch, and the a11y wiring are all logic, and
duplicating logic across two pages that will drift is the standard way this
becomes two subtly different sign-out buttons.

Rejected: inline `<Button onClick={logout}>` on each page. Cheaper by about
thirty lines and wrong within a month.

### 3.2 Where the pending/error state lives

**Chosen: local `useState` inside the component.**

Rejected: `useMutation({ mutationFn: logout })`. It would give `isPending` and
`isError` for free and would match the codebase's habit of routing writes
through React Query. But `logout()` calls `queryClient.removeQueries` on the
auth keys as part of its own body, so the mutation would be tearing down cache
belonging to the client that is tracking it — coupling with no upside for a
call that has exactly one caller, no cache to invalidate, and no retry policy.
One `useState<boolean>` is the honest size of this.

### 3.3 Copy: shared string vs. per-page variant

**Chosen: identical `Not you? / Sign out` on both pages.** PRD §9.2 leaves the
door open to `Wrong account?` on the invite error screen specifically, where the
cause is known. Taking that door now means a `variant` prop, which contradicts
FR-SHARED-2's "no required props" spirit and adds a branch to a component whose
entire value is that it has none. The accessible name `Sign out` — the only part
that is actually a requirement — is identical either way. If a designer asks for
it later, it is an optional prop with a default, added in five minutes.

### 3.4 Identity line: email alone vs. `Ada Lovelace (ada@example.com)`

**Chosen: email alone** — PRD §9.1 resolved as specified, no change to
FR-IDENT-2. The user reading this line is answering one question: *which
account am I in?* The email answers it; the display name does not (two Google
accounts routinely share one). The compound form is longer, truncates sooner on
a phone, and truncates from the *right* — which is precisely where the
disambiguating part of an email address lives. `ProfileMenu` already shows both
lines for users who have a fleet; this footer is not trying to be that.

### 3.5 Proving the request actually goes out

Acceptance criteria demand two things that a mocked-`logout` unit test cannot
show: that `POST /api/auth/logout` is genuinely issued, and that the redirect is
produced by `RequireAuth` rather than by hidden navigation.

**Chosen: a third test file, `signOutFlow.test.tsx`, that mounts the real
`AuthProvider`, the real `RequireAuth`, and a two-route tree, with `fetch`
stubbed at the boundary.** See §6.3.

Rejected: asserting `logout` was called on a mock and calling it done. That
passes just as happily against an implementation that clears `localStorage`
itself — the exact failure NFR-1 exists to prevent.

Rejected: mounting the real `AppRoutes` (the `postPurgeRouting.test.tsx`
pattern). It is the right pattern when the *nesting* is what's under test, which
was that test's whole point. Here it drags in every page's data layer to prove a
property of two components, and the thing it would additionally prove — that
`/onboarding` sits under `RequireAuth` — is already pinned by that existing test.

---

## 4. Page integration

### 4.1 `OnboardingPage`

One line, last child of the existing page-level flex column
(`OnboardingPage.tsx:100-132`), after both cards:

```tsx
  </Card>
  <SignedInFooter />
</div>
```

The column already has `gap-4` and `items-center`, so no layout change is
needed. It is outside `PendingInvites`, which returns `null` when there is
nothing pending — that is what makes FR-ONBOARD-2 (renders on the first-run
path too) fall out structurally rather than needing a condition.

### 4.2 `InviteAcceptPage`

One line, in the error branch only (`InviteAcceptPage.tsx:64-73`), after the
`Go to Dashboard` button. The `pending` and `success` branches are untouched
(FR-INVITE-2) — both are transient and neither traps anyone. Error copy, the
`detail`-over-`message` precedence at line 37, and the existing button are all
unchanged (FR-INVITE-3).

The `ux-flow.md` cycle is worth restating because it is the reason this page is
in scope at all: `Go to Dashboard` re-enters the guard, which sends a fleetless
user straight back to `/onboarding`. On the "this invite was sent to a different
email address" error, the existing button is the one that loops and the new
footer is the one that resolves the problem.

---

## 5. Error handling

| Condition | Behaviour |
|---|---|
| `logout()` resolves | Component unmounts via `RequireAuth`; no toast, no navigation call |
| `logout()` rejects | `toast.error(...)`, user stays signed in, button re-enabled, no unhandled rejection |
| No `user` in context | Renders `null` |
| Double-click | Second click hits a `disabled` button; one request |

The rejection branch is **unreachable today**: `logoutRequest()` terminates its
fetch with `.catch(() => undefined)` (`lib/hooks/api/auth.ts:70`), so `logout()`
has exactly one outcome. It is built anyway (FR-SIGNOUT-4) so that task-022
lights it up without a change here. Its test therefore injects a rejecting
`logout` through the mocked context — the only way to reach the branch until 022
lands, and the test stays valid afterwards.

Message extraction uses `createErrorFromUnknown(err).message` with a
`'Could not sign out'` fallback, matching `OnboardingPage.tsx:94-95`.

---

## 6. Test strategy

Per NFR-5 and the standing note that jsdom cannot see CSS: every assertion is on
role, accessible name, rendered text, request, or resulting route. Nothing
asserts on the visual subordination in FR-ONBOARD-3 — that is verified by eye.

### 6.1 The existing-test breakage (must be handled, not discovered)

`renderWithProviders` deliberately wraps only `QueryClientProvider` and
`MemoryRouter` — **not `AuthProvider`**. `OnboardingPage` does not currently
consume `useAuth()`, so its four existing tests pass. The moment it renders
`SignedInFooter`, all four throw `useAuth must be used within an AuthProvider`.

**Chosen fix: add the `vi.mock('../context/AuthContext')` + `mockAuth` block to
`OnboardingPage.test.tsx`**, the pattern already used by eleven test files
(`LoginPage.test.tsx:12-15`, `AppLayout.test.tsx`, `ProfileMenu.test.tsx`, …).

Rejected: adding `AuthProvider` to `renderWithProviders`. It would fix these
four tests and change the wrapper every component test in the repo uses —
mounting a real `useMe` query in each one, whose `enabled: !!getAccessToken()`
makes behaviour depend on ambient `localStorage` state. Large blast radius,
worse isolation, for a problem two mocked lines solve.

### 6.2 Unit tests

`accountLabel.test.ts` — pure, no rendering:

- email present → email
- no email, display name present → display name
- both present → **email** (the deliberate divergence from `identityLines`)
- whitespace-only email with a real display name → display name
- both absent / whitespace-only → `null`
- `null` / `undefined` user → `null`

`SignedInFooter.test.tsx` — mocked `useAuth` and mocked `sonner`, following
`ProfileMenu.test.tsx`'s `mockAuth` shape:

- renders `Signed in as ada@example.com`
- renders `Signed in as Ada Lovelace` when there is no email
- renders **no** identity line when there is neither, while `Sign out` is still
  present and still calls `logout` (FR-IDENT-3)
- `getByRole('button', { name: 'Sign out' })` resolves — i.e. `Not you?` is not
  part of the accessible name (FR-A11Y-2)
- keyboard: `Tab` reaches it, `Enter` activates it (FR-A11Y-1)
- click calls `logout` exactly once
- while a deferred `logout` is in flight the button has `disabled`; two rapid
  clicks produce one call (FR-SIGNOUT-5)
- rejecting `logout` → `toast.error` called, button re-enabled, and the test
  registers an `unhandledrejection` listener asserting nothing escaped
  (FR-SIGNOUT-4)
- `user: null` → renders nothing (FR-SHARED-3)
- source-level check that the component imports neither `useNavigate` nor
  touches `window.location` is **not** written as a string-grep; §6.3 proves the
  positive property instead

### 6.3 `signOutFlow.test.tsx` — the integration test

The one test that mounts real machinery. Shape:

```tsx
setAccessToken('t');                       // so useMe is enabled
vi.mock('../../lib/api/refresh', ...);     // keep the 401 retry path out of it
globalThis.fetch = vi.fn(...)              // /api/auth/me → user + meta{activeFleetId:null}
                                           // /api/auth/logout → 204

render(
  <QueryClientProvider client={qc}>
    <MemoryRouter initialEntries={['/onboarding']}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<div>login screen</div>} />
          <Route path="/onboarding" element={<RequireAuth><SignedInFooter /></RequireAuth>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>
  </QueryClientProvider>
);
```

Assertions:

1. `fetch` was called with `/api/auth/logout` and `{ method: 'POST' }` —
   satisfying "asserted against the request, not inferred from the UI landing on
   `/login`" and NFR-1.
2. `login screen` appears. Because the tree contains no `useNavigate`, no
   `window.location` write, and no route element other than `RequireAuth`'s
   subject, the only thing that could have produced that transition is
   `RequireAuth`'s own `<Navigate>` (FR-SIGNOUT-3).
3. `credentials: 'include'` is on the logout request — the HttpOnly refresh
   cookie is the whole reason the round-trip exists (§1.1 of the PRD).

Deliberately *not* asserted here: that `OnboardingPage` mounts the footer. That
is §6.4's job, and mixing them would make one test responsible for two
independent failures.

`SignedInFooter` rather than `OnboardingPage` is the guarded route's element on
purpose — `OnboardingPage` would pull `usePendingInvites` and the fleet-creation
form into a test about the auth round-trip.

### 6.4 Page tests

`OnboardingPage.test.tsx` (edited): keeps its four existing cases, gains the
auth mock from §6.1, plus one case each for — the footer renders with pending
invites present, and the footer renders when nothing is pending (FR-ONBOARD-2,
the first-run path that is easiest to break).

`InviteAcceptPage.test.tsx` (new — the page has none today): error state renders
`Sign out`; `pending` state does not; `success` state does not (FR-INVITE-2);
`Go to Dashboard` and the `detail`-derived error text are still present
(FR-INVITE-3). `setTimeout` in the success branch means the success case needs
fake timers or an explicit assertion before the 2 s redirect fires.

### 6.5 Verification

`make ci` (lint-check, vet, test, build, fe-test, fe-build), then the
`frontend-guidelines-reviewer` audit into
`docs/tasks/task-024-onboarding-sign-out/audit.md` per NFR-3.

---

## 7. Requirement traceability

| Requirement | Where it lands |
|---|---|
| FR-SHARED-1/2/3 | §1.1, §2.2 — one component, no props, `null` on no user |
| FR-IDENT-1/2/3 | §2.1, §2.2 — `accountLabel`, conditional identity line |
| FR-IDENT-4 | §2.1 — divergence comment; `identityLines.ts` untouched |
| FR-IDENT-5 | §2.2 — `truncate` inside the bounded column |
| FR-SIGNOUT-1 | §1 — `useAuth().logout()` only |
| FR-SIGNOUT-2 | §2.2 — `await logout()` inside `signOut` |
| FR-SIGNOUT-3 | §1, §6.3 — no navigation code; proven by the integration test |
| FR-SIGNOUT-4 | §2.2, §5, §6.2 — catch → toast, no escape |
| FR-SIGNOUT-5 | §2.2, §2.3 — latched `signingOut` |
| FR-ONBOARD-1/2/3 | §4.1, §2.2 |
| FR-INVITE-1/2/3 | §4.2 |
| FR-A11Y-1/2/3/4 | §2.2 |
| NFR-1 | §6.3 assertion 1 and 3 |
| NFR-2 | §2.2 — `useAuth()` only; no query of its own |
| NFR-3 | §6.5 |
| NFR-4 | §2.2 — Tailwind + existing `Button` |
| NFR-5 | §6 preamble |

---

## 8. Risks

**R1 — The four existing `OnboardingPage` tests break.** Certain, not a risk in
the probabilistic sense: `renderWithProviders` has no `AuthProvider`. Mitigated
by §6.1 being part of the plan rather than something discovered at `make ci`.

**R2 — Merge overlap with task-022.** Both branch from `main` and both touch the
sign-out path. 022 changes `logoutRequest()`/`AuthContext` and `ProfileMenu`;
this task changes neither, and adds one call site that already handles a
rejection. Whichever lands second should have no textual conflict outside a
possible import-ordering nit. The one thing that would create real conflict —
this task "fixing" `logoutRequest`'s `.catch` — is explicitly out of scope.

**R3 — `ApiClient` response handling in the integration test.** `fetchMe` goes
through `@myfleet/shared-ts`'s `ApiClient`, so the stubbed `/api/auth/me`
`Response` must satisfy whatever that client expects (content type, envelope).
`logoutRequest` uses raw `fetch`, so the assertion that matters is unaffected.
If the `me` stub proves fussy, seeding the identity into the `QueryClient` cache
under `authKeys.me()` is the fallback — it keeps the real `AuthProvider`,
`logout()`, and `RequireAuth` in the loop, which is the point of the test.

**R4 — Footer visually competes with `Create Fleet`.** Not testable in jsdom
(NFR-5). Mitigation is the styling in §2.2 — muted, link-variant, `p-0`, below
the fold of the card — plus a visual check during execution.

---

## 9. Out of scope

Restating the PRD's non-goals, plus what this design adds to them:

- `RequireAuth`, `FLEETLESS_ROUTES`, and the redirect rules — unchanged.
- `AuthContext.logout()`, `logoutRequest()`, the auth-service handler — unchanged.
  In particular the `.catch(() => undefined)` at `auth.ts:70` stays; it is
  task-022's.
- `ProfileMenu` is **not** migrated to `await logout()` here, even though
  FR-SIGNOUT-2 makes its `void logout()` look inconsistent. That call site is
  task-022's scope, and touching it would manufacture the conflict R2 avoids.
- No "switch account" chained logout→login flow.
- No `AppLayout`/`FrameHeader`/`ProfileMenu` on the fleetless pages.
- No collapsing of `identityLines()` and `accountLabel()` (PRD §9.3).
- No backend, API, data-model, or `deploy/k8s` change.
