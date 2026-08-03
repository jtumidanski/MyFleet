# Sign Out From The Fleetless Pages — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-03
---

## 1. Overview

A user who authenticates but has no active fleet is trapped in the application.
`RequireAuth` redirects every route to `/onboarding` for such a user
(`apps/web/src/components/RequireAuth.tsx:50`), exempting only `/onboarding`
itself and `/invites/:token/accept` (`FLEETLESS_ROUTES`, line 13). Both of those
pages are routed outside `AppLayout` (`apps/web/src/App.tsx:36-53`), so neither
renders `FrameHeader` and neither renders `ProfileMenu` — the only sign-out
control in the product. The result is a state with no exit: the user cannot
navigate anywhere else, and cannot leave the account they are in. The remaining
options are to create a fleet they did not want, or to hand-clear `localStorage`.

This is not a hypothetical corner. It is the ordinary outcome of signing in with
the wrong Google account, and it is the *designed* landing state for anyone whose
invite has not yet been accepted. The invite-accept page makes it sharper still:
when acceptance fails because the invite was addressed to a different email —
one of the cases `InviteAcceptPage.tsx:37` renders from the error `detail` — the
page's only control is "Go to Dashboard", which for a fleetless user routes
straight back to `/onboarding`. The user is told they used the wrong account and
then offered no way to change it.

This task adds a sign-out affordance to both fleetless pages. The mechanics of
signing out already work and are not being rebuilt: `AuthContext.logout()`
(`apps/web/src/context/AuthContext.tsx:55`) revokes server-side, clears the
access token, flips `hasToken`, and drops the auth queries; the resulting
re-render makes `RequireAuth` return `<Navigate to="/login">` on its own. What is
missing is a control that calls it.

### 1.1 Why the server round-trip matters here

The access token lives in `localStorage` and the refresh token lives in an
`HttpOnly` cookie. Only the server can invalidate the latter. A control that
merely cleared local state would produce a UI that says "signed out" over a
session that is still resumable — precisely the gap that matters on the shared
or borrowed machine where "I signed in with the wrong account" most often
happens. Every requirement below routes through `logout()` for this reason;
none of them may reach for `clearAccessToken()` directly.

### 1.2 Relationship to task-022

`task-022-signout-failure-handling` is in flight and rewrites this same path. It
makes `logoutRequest()` capable of failing (today `apps/web/src/lib/hooks/api/auth.ts:70`
terminates the fetch with `.catch(() => undefined)`, so the function has exactly
one possible outcome) and adds error handling at the `ProfileMenu` call site.

The two tasks are deliberately independent and branch from `main`. This one is
specified so that whichever lands second needs no rework: the new control
handles a rejected `logout()` from the outset (FR-SIGNOUT-4), which is inert
against today's swallow-everything transport and becomes live the moment 022
merges. The one structural commitment that makes this cheap is FR-SHARED-1 —
a single shared component, so there is one new call site to reconcile, not two.

## 2. Goals

Primary goals:

- Give an authenticated, fleetless user a way to leave their account from
  `/onboarding` without creating a fleet.
- Do the same for the `/invites/:token/accept` error state, where the wrong
  account is an explicitly reported cause of failure.
- Make the signed-in identity visible on those pages, so the user can tell
  *which* account they are about to leave.
- Ensure sign-out from these pages revokes the session server-side, identically
  to `ProfileMenu`.

Non-goals:

- Changing `RequireAuth`, `FLEETLESS_ROUTES`, or the redirect rules.
- Changing `AuthContext.logout()`, `logoutRequest()`, or the auth-service logout
  handler. Failure *reporting* in those layers is task-022's scope.
- Adding a "switch account" one-click flow that chains logout into login.
- Adding `ProfileMenu`, `FrameHeader`, or any part of `AppLayout` to the
  fleetless pages.
- Any backend, API, or data-model change.

## 3. User Stories

- As a user who signed in with the wrong Google account, I want to sign out from
  the onboarding page so that I can sign back in as the right account without
  creating a fleet I do not want.
- As an invitee whose accept failed because the invite was sent to a different
  address, I want to sign out from the error screen so that I can retry the
  invite link as the invited account.
- As a user on a shared machine, I want signing out from onboarding to end my
  session on the server, so that closing the browser does not leave a resumable
  session behind.
- As a first-run user who legitimately intends to create a fleet, I want the
  sign-out control to stay visually subordinate to "Create Fleet", so that it
  does not compete with the action I actually came for.

## 4. Functional Requirements

### 4.1 Shared component

- **FR-SHARED-1.** A single new component renders the affordance, and both pages
  import it. There must be exactly one call site of `logout()` added by this
  task. Suggested name and location:
  `apps/web/src/components/auth/SignedInFooter.tsx`.
- **FR-SHARED-2.** The component takes no required props. It reads the user from
  `useAuth()` directly, as `ProfileMenu` does.
- **FR-SHARED-3.** The component renders nothing (`null`) when there is no
  authenticated user. It is only ever mounted under `RequireAuth`, so this is
  defence-in-depth rather than a live path, and it keeps the component safe to
  render unconditionally by its callers.

### 4.2 Content and identity

- **FR-IDENT-1.** The component renders two pieces of content: an identity line
  reading `Signed in as {account}`, and an interactive control labelled
  `Sign out`, preceded by the text `Not you?`.
- **FR-IDENT-2.** `{account}` resolves by a pure, separately unit-tested helper
  with this precedence: **email → displayName → omit**. The email comes from
  `user.attributes.email`, the display name from `user.attributes.displayName`
  (see `apps/web/src/types/models/user.ts`); both are trimmed before use and an
  all-whitespace value counts as absent.
- **FR-IDENT-3.** When neither email nor display name is available, the identity
  line is omitted entirely and the `Not you? Sign out` control still renders. The
  page must never display `Signed in as` followed by nothing, nor a placeholder
  like `Signed in as Account`.
- **FR-IDENT-4.** The existing `identityLines()` helper
  (`apps/web/src/components/frame/identityLines.ts`) must **not** be reused here,
  and must not be modified. It orders `displayName → email → "Account"` because a
  menu header is a greeting; this footer is a disambiguation, where the email is
  the identifying field and a display name is not unique. The two helpers
  disagree on purpose; the new one carries a comment saying so.
- **FR-IDENT-5.** The identity line truncates rather than wrapping to a second
  line or widening its container, matching `ProfileMenu`'s treatment of the same
  values (`ProfileMenu.tsx:58`).

### 4.3 Sign-out behaviour

- **FR-SIGNOUT-1.** Activating the control calls `logout()` from `useAuth()`.
  It must not call `clearAccessToken()`, manipulate `localStorage`, or issue its
  own request.
- **FR-SIGNOUT-2.** The returned promise is awaited, not floated. `void logout()`
  — the current `ProfileMenu.tsx:64` form — is explicitly not acceptable for the
  new call site.
- **FR-SIGNOUT-3.** On success, the component performs **no** navigation. The
  redirect to `/login` is produced by `RequireAuth` re-rendering once `hasToken`
  flips (`RequireAuth.tsx:39-43`). No `useNavigate` call, no `window.location`
  assignment. A test must prove the redirect actually occurs by this route
  rather than assuming it.
- **FR-SIGNOUT-4.** If `logout()` rejects, the failure is surfaced to the user
  via an error toast (`sonner`, as used at `OnboardingPage.tsx:95`), the user is
  left signed in, and the rejection does not escape as an unhandled promise
  rejection. Under today's transport this branch is unreachable; it is required
  anyway so that task-022 lights it up without a change here.
- **FR-SIGNOUT-5.** While a sign-out is in flight the control is disabled, so a
  double-click cannot issue two logout requests.

### 4.4 Onboarding page

- **FR-ONBOARD-1.** `OnboardingPage` renders the component once, below the fleet
  card, inside the existing page-level flex column (`OnboardingPage.tsx:100-132`).
  It appears after both the `PendingInvites` card and the `Set Up Your Fleet`
  card.
- **FR-ONBOARD-2.** It renders on every visit to `/onboarding` — including when
  `PendingInvites` renders nothing, which is the first-run case.
- **FR-ONBOARD-3.** The control is styled subordinate to the `Create Fleet`
  submit button: small, muted-foreground text, no filled button surface. It must
  remain a real `<button>` (see FR-A11Y-1), not an anchor or a click-handled
  `<span>`.

### 4.5 Invite-accept page

- **FR-INVITE-1.** `InviteAcceptPage` renders the component in its **error**
  state only (`InviteAcceptPage.tsx:64-73`), below the existing
  `Go to Dashboard` button.
- **FR-INVITE-2.** It is not rendered in the `pending` or `success` states. Both
  are transient and neither traps the user; adding a sign-out control to a
  screen that is about to redirect is noise.
- **FR-INVITE-3.** The existing error copy, the `detail`-over-`message`
  precedence at `InviteAcceptPage.tsx:37`, and the `Go to Dashboard` button are
  unchanged. This requirement adds a control; it does not restyle the screen.

### 4.6 Accessibility

- **FR-A11Y-1.** The sign-out control is a `<button type="button">` with the
  accessible name `Sign out`. It is reachable and activatable by keyboard
  (Tab / Enter / Space) with no custom key handling.
- **FR-A11Y-2.** `Not you?` is adjacent static text, not part of the button's
  accessible name.
- **FR-A11Y-3.** Neither the identity line nor the footer uses a heading element,
  so the page's heading outline is unchanged.
- **FR-A11Y-4.** The disabled in-flight state (FR-SIGNOUT-5) uses the `disabled`
  attribute so it is exposed to assistive technology, not a pointer-events or
  opacity-only treatment.

## 5. API Surface

No new or modified endpoints. The task adds a second caller of the existing
`POST /api/auth/logout`, reached through `AuthContext.logout()` →
`logoutRequest()` (`apps/web/src/lib/hooks/api/auth.ts:64-71`). Request shape,
response shape, and error cases are unchanged.

## 6. Data Model

No change. No new entities, fields, or migrations. The component reads
`UserAttributes.email` and `UserAttributes.displayName`, both of which already
exist (`apps/web/src/types/models/user.ts`).

## 7. Service Impact

| Service | Change |
|---|---|
| `apps/web` | New `SignedInFooter` component + pure account-label helper, each with tests. Edits to `OnboardingPage.tsx` and `InviteAcceptPage.tsx` to mount it. |
| `apps/auth-service` | None. |
| `apps/fleet-service` | None. |
| `deploy/k8s` | None. |

## 8. Non-Functional Requirements

- **NFR-1 (Security).** Sign-out from these pages revokes the refresh-token
  family server-side via the existing endpoint. A client-only token clear is not
  an acceptable implementation (see §1.1).
- **NFR-2 (No new network cost).** The component issues no request of its own on
  render. It consumes the already-cached `useMe()` result via `useAuth()`.
- **NFR-3 (Guidelines).** Implementation follows the `frontend-dev-guidelines`
  FE-\* checklist and will be audited by `frontend-guidelines-reviewer`.
- **NFR-4 (Styling).** Tailwind utility classes and existing shadcn/ui
  primitives only; no new dependency, no bespoke CSS file.
- **NFR-5 (Test isolation).** Per the project's standing note that jsdom cannot
  see CSS, tests assert on behaviour, roles, and rendered text — never on the
  visual subordination required by FR-ONBOARD-3, which is verified by eye.

## 9. Open Questions

1. Should the identity line show the email even when a display name exists
   (`Signed in as Ada Lovelace (ada@example.com)`)? FR-IDENT-2 currently shows
   the email alone, on the grounds that it is the disambiguating field. The
   two-part form is more informative but longer, and truncates sooner on mobile.
   Deferred to design.
2. Exact copy. `Not you? Sign out` is the working text; a designer may prefer
   `Wrong account? Sign out` on the invite error screen specifically, where the
   cause is known. Not blocking — the requirement is that a sign-out control
   exists with the accessible name `Sign out`.
3. Whether a future task should collapse `identityLines()` and the new helper
   behind one parameterised function. Explicitly deferred; FR-IDENT-4 keeps them
   separate for now.

## 10. Acceptance Criteria

- [ ] A signed-in user with no active fleet, sitting on `/onboarding`, can sign
      out and reach `/login` without creating a fleet.
- [ ] After that sign-out, signing back in through the OAuth flow works and
      lands the user according to the existing rules.
- [ ] `/onboarding` shows `Signed in as <email>` for a user with an email, for
      both the has-pending-invites and no-invites cases.
- [ ] A user with a display name but no email shows `Signed in as <displayName>`.
- [ ] A user with neither shows no identity line, and the `Sign out` control is
      still present and functional.
- [ ] The invite-accept **error** state renders the control; the pending and
      success states do not.
- [ ] Sign-out issues `POST /api/auth/logout` — asserted against the request, not
      inferred from the UI landing on `/login`.
- [ ] A rejected `logout()` raises an error toast, leaves the user signed in, and
      produces no unhandled rejection.
- [ ] The control is disabled while sign-out is in flight; a double-click issues
      one request.
- [ ] The control is a keyboard-reachable `<button>` with accessible name
      `Sign out`; `Not you?` is not part of that name.
- [ ] Redirect to `/login` is driven by `RequireAuth`, with no `useNavigate` or
      `window.location` call in the new component.
- [ ] `identityLines.ts`, `RequireAuth.tsx`, `AuthContext.tsx`, and
      `auth.ts` are unmodified by this task.
- [ ] `make ci` passes.
- [ ] `frontend-guidelines-reviewer` audit recorded in
      `docs/tasks/task-024-onboarding-sign-out/audit.md` with no unresolved
      findings.
