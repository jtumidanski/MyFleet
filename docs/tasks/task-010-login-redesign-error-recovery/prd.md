# Login Page Redesign & Recoverable OAuth Failures — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02

---

## 1. Overview

`/login` is the only screen an unauthenticated visitor sees, and it is currently a
centred shadcn `Card` on `bg-muted` carrying a title, one sentence, and one outline
button (`apps/web/src/pages/LoginPage.tsx`). It works, but it reads as a placeholder:
it says nothing about what MyFleet does, gives no feedback when the button is pressed,
and cannot report a failure because it has nowhere to put one.

The second half of the problem is in `auth-service`. Every failure path in
`callbackHandler` (`apps/auth-service/internal/oidc/resource.go`) terminates with
`http.Error(...)`, which renders a bare `text/plain` page at the callback URL with no
branding, no navigation, and no way back into the app. There are nine such exits. The
most likely one is not an error at all: a user who clicks **Cancel** on Google's
consent screen returns with no `code` parameter and lands on a plaintext
`missing code or state` 400. The OAuth flow has no recoverable failure mode.

This task rewrites the page to a typographic, card-less treatment ("Direction D",
chosen from four explored alternatives) and gives it three real states — ready,
redirecting, and failed — backed by an auth-service change that redirects failures
to `/login#error=<code>` instead of dead-ending. It also resolves an explicitly
deferred decision from task-003: that PRD's open question #3 and `FR-TOGGLE-7` scoped
the theme control to the authenticated shell and left the pre-auth toggle for a later
call. This task makes that call — the login page gets one.

## 2. Goals

Primary goals:

- Replace the login page's card layout with a typographic, left-aligned composition
  that states what MyFleet is before asking for a Google account.
- Give the single control real states, so pressing it produces feedback and a failed
  round-trip produces an explanation.
- Make every OAuth callback failure land back on `/login` inside the SPA rather than
  on a plaintext error page.
- Distinguish a deliberate cancel from a genuine failure, and present it neutrally.
- Let a signed-out visitor change the theme.

Non-goals:

- The `return_to` / post-login return-path work currently in flight on another
  branch. See §7.4 — this task must not reimplement it.
- Visual changes to `/onboarding` or `/invites/:token/accept`. They keep their
  current card treatment; revisit once Direction D is proven in the real app.
- Any change to the `--destructive` token or the semantic status palette established
  in task-003. This task consumes the existing `--danger-*` callout family.
- Any change to token minting, refresh rotation, cookie handling, or the state-cookie
  signature scheme.
- Adding a non-Google identity provider, or any "sign in with email" affordance.

## 3. User Stories

- As a first-time visitor, I want the login page to tell me what MyFleet tracks so
  that I can decide whether it is worth connecting my Google account.
- As a visitor pressing "Continue with Google", I want the button to acknowledge the
  press so that I do not click twice while the redirect is in flight.
- As a visitor who changed my mind at Google's consent screen, I want to land back on
  a working sign-in page so that I can try again or leave, rather than reading a
  plaintext error.
- As a visitor whose sign-in genuinely failed, I want to be told that it failed and
  that nothing was saved so that I know retrying is safe.
- As a visitor who prefers a dark UI, I want to set the theme before signing in so
  that the first screen is not a flash of the wrong theme.
- As an operator, I want callback failures to keep logging exactly what they log today
  so that redirecting the user does not cost me diagnostics.

## 4. Functional Requirements

### 4.1 Page composition (FR-PAGE-*)

- **FR-PAGE-1** — `/login` renders without a `Card`. The root is a full-height
  (`min-h-screen`) flex column, vertically centred, with content left-aligned against
  a responsive left inset. Content sits in a max-width container so the headline does
  not span an ultrawide viewport.
- **FR-PAGE-2** — The page background is `bg-background`, not `bg-muted`. The card
  that previously justified the muted ground is gone.
- **FR-PAGE-3** — An eyebrow row sits above the headline: the existing `<BrandMark>`
  beside the word "MyFleet", set small, uppercase, letter-spaced, in
  `text-muted-foreground`.
- **FR-PAGE-4** — The headline reads `Your cars.` / `One place.` on two lines. The
  first line is `text-foreground`; the second is `text-muted-foreground`. It is set
  large, semibold, with tight tracking and leading, and capped near `11ch` so it wraps
  predictably on narrow viewports.
- **FR-PAGE-5** — A hairline rule (`bg-border`, 1px) separates the headline from the
  call-to-action row.
- **FR-PAGE-6** — The call-to-action row contains the sign-in button and, beside or
  beneath it, a scopes line naming exactly what MyFleet receives: name, email address,
  and profile photo.
- **FR-PAGE-7** — A footer row sits below, in `text-muted-foreground`, carrying a
  short descriptor of the product and a note that sign-in is via Google.
- **FR-PAGE-8** — The sign-in button is a solid (primary) button carrying the Google
  "G" mark. Google's branding terms require the mark on the button; the current
  text-only button does not satisfy them.
- **FR-PAGE-9** — The layout is legible from a 320px-wide viewport up. The headline
  scales down rather than overflowing, and the button remains full-width-or-natural
  without clipping.
- **FR-PAGE-10** — The page renders correctly in both light and dark themes using
  existing tokens only. No new CSS custom properties are introduced.

### 4.2 Page states (FR-STATE-*)

- **FR-STATE-1** — **Ready.** Default. Button enabled, labelled "Continue with
  Google", scopes line visible, no callout.
- **FR-STATE-2** — **Redirecting.** Entered on button press. The button is disabled
  and shows a spinner with a label indicating the handoff to Google. This state covers
  the gap between the click and the browser leaving the page; it is never exited
  in-place, because `login()` performs a full navigation.
- **FR-STATE-3** — The redirecting state must make a second activation impossible
  (`disabled`), so a double-click cannot start two OAuth flows.
- **FR-STATE-4** — **Failed.** Entered when the page loads with a recognised `#error=`
  fragment. A callout renders above the button using the existing `--danger-subtle` /
  `--danger-subtle-foreground` / `--danger-border` family from `index.css`. It states
  that sign-in did not complete and that nothing was saved. The button relabels to
  "Try again".
- **FR-STATE-5** — **Cancelled** is presented separately from failed. It renders a
  neutral, non-alarming line (e.g. "Sign-in cancelled.") in `text-muted-foreground`,
  *not* the danger callout, and the button keeps its normal "Continue with Google"
  label. Cancelling is a choice, not a fault.
- **FR-STATE-6** — An unrecognised or malformed `error` code is treated as
  `server_error` (generic failure callout). The page must never render an
  attacker-supplied string from the fragment.
- **FR-STATE-7** — The error fragment is stripped from the URL after it is read, via
  `history.replaceState`, so a reload or a shared URL does not resurrect a stale
  message. This mirrors the existing behaviour of `captureTokenFromHash`.
- **FR-STATE-8** — Reading the error fragment must not interfere with
  `captureTokenFromHash`, which no-ops unless `access_token=` is present. The two
  fragment consumers are mutually exclusive by construction.
- **FR-STATE-9** — The existing redirect-when-already-authenticated behaviour is
  preserved: an authenticated visitor landing on `/login` is still bounced into the
  app.

### 4.3 Pre-auth theme control (FR-PRETOGGLE-*)

- **FR-PRETOGGLE-1** — `/login` renders a theme control in the top-right corner of the
  viewport.
- **FR-PRETOGGLE-2** — It cycles `light` → `dark` → `system` → `light` and shows the
  icon for the current *preference*, identical to the authenticated control
  (task-003 FR-TOGGLE-2, FR-TOGGLE-3).
- **FR-PRETOGGLE-3** — It must **not** issue the authenticated `PATCH` that
  `ThemeToggle` fires via `useUpdateTheme()`. There is no session on `/login`; the
  request would 401 and surface a spurious "Couldn't save your theme preference"
  toast on every click.
- **FR-PRETOGGLE-4** — To satisfy FR-PRETOGGLE-3, the presentational half of
  `ThemeToggle` is extracted into a component that takes the current preference and an
  `onSelect` callback. `ThemeToggle` keeps the mutation and remains the authenticated
  header control; the login page wires the extracted component directly to
  `setPreference` from `ThemeContext`.
- **FR-PRETOGGLE-5** — The extraction must not make `ThemeToggle` or `ThemeContext`
  aware of auth. Task-003 §3.3 deliberately routed server-preference adoption through
  `ThemeSync` to keep that boundary; this task preserves it.
- **FR-PRETOGGLE-6** — A preference set pre-auth persists to `localStorage` and is
  adopted-or-overridden after sign-in by the existing `ThemeSync` rules. No change to
  `FR-PERSIST-3` / `FR-PERSIST-5` / `FR-PERSIST-7` behaviour is required or permitted.
- **FR-PRETOGGLE-7** — This requirement set supersedes task-003 `FR-TOGGLE-7` ("The
  control renders only inside `AppLayout`") and closes task-003 open question #3. The
  task-003 PRD is not edited; the supersession is recorded here.

### 4.4 Callback failure redirects (FR-ERR-*)

- **FR-ERR-1** — Every failure exit in `callbackHandler` redirects (HTTP 302) to
  `AppBaseURL + LoginPath + "#error=" + <code>` instead of calling `http.Error`.
- **FR-ERR-2** — `Dependencies` gains a `LoginPath` field, wired in
  `apps/auth-service/cmd/main.go` as `config.Get("APP_LOGIN_PATH", "/login")`,
  matching the existing `HomePath` / `OnboardingPath` pattern.
- **FR-ERR-3** — The error code travels in the URL **fragment**, not the query string,
  matching how the access token is already delivered. Fragments are not sent to the
  server, so the code stays out of access logs and `Referer` headers.
- **FR-ERR-4** — Exactly four codes are defined. They are deliberately coarse so that
  no internal failure detail reaches the browser:

  | Code | Meaning |
  | --- | --- |
  | `cancelled` | The user declined at Google's consent screen. |
  | `invalid_state` | Missing/mismatched state or a failed state-cookie check. |
  | `auth_failed` | Code exchange, ID-token verification, or nonce check failed. |
  | `server_error` | Provisioning, membership resolution, or token minting failed. |

- **FR-ERR-5** — The nine current `http.Error` exits map as follows. The `cancelled`
  code has no row here because it originates from the new pre-check in FR-ERR-6, which
  returns before any of these exits is reached:

  | Current exit | Code |
  | --- | --- |
  | `missing code or state` (400) | `invalid_state` |
  | `invalid state` (400) | `invalid_state` |
  | code exchange failure (502) | `auth_failed` |
  | ID-token verification failure (401) | `auth_failed` |
  | nonce mismatch (401) | `auth_failed` |
  | `ProvisionFromGoogle` failure (500) | `server_error` |
  | membership `Resolve` failure (500) | `server_error` |
  | `MintAccess` failure (500) | `server_error` |
  | `IssueRefresh` failure (500) | `server_error` |

- **FR-ERR-6** — A new explicit check runs before the `code`/`state` check: if Google
  returned an `error` query parameter (e.g. `access_denied`), the handler redirects
  with `cancelled`. Today this case falls into the generic missing-code 400.
- **FR-ERR-7** — Server-side logging is unchanged. Every `log.WithError(...).Error(...)`
  call that exists today still fires with the same message and fields. Redirecting the
  user must not reduce operator diagnostics.
- **FR-ERR-8** — The state cookie is cleared on the *early* failure exits too. Today
  `clearStateCookie` runs only after `verifyStateCookie` succeeds, so the `cancelled`
  pre-check, the missing-`code`/`state` exit, and the `invalid state` exit all leave a
  signed state cookie in the browser until its 10-minute TTL expires. Since these paths
  now return the user to a page offering "Try again", the abandoned attempt's cookie
  must be cleared rather than left to collide with the next one.
- **FR-ERR-9** — The redirect target is composed from server configuration
  (`AppBaseURL`, `LoginPath`) and a fixed code from a closed set. No part of it derives
  from user input, so this introduces no open-redirect surface. The existing
  `safeReturnPath` guard is not involved and must not be weakened.

### 4.5 Accessibility (FR-A11Y-*)

- **FR-A11Y-1** — The failure callout is announced to assistive technology
  (`role="alert"`), so a screen-reader user learns the attempt failed without
  re-reading the page.
- **FR-A11Y-2** — The disabled redirecting state communicates its status
  non-visually; a spinner alone is insufficient.
- **FR-A11Y-3** — The pre-auth theme control carries the same `aria-label` and `title`
  contract as the authenticated one (task-003 FR-TOGGLE-4, FR-TOGGLE-5) and is
  keyboard reachable with a visible focus ring using `--ring` (FR-TOGGLE-6).
- **FR-A11Y-4** — The Google "G" mark is decorative (`aria-hidden`); the button's text
  supplies the accessible name.
- **FR-A11Y-5** — Headline, body, callout, and muted footer text all meet WCAG AA
  contrast in both themes.

## 5. API Surface

No new endpoints. One modified endpoint's failure behaviour:

**`GET /api/auth/callback`** — unchanged on success (302 to
`destination + #access_token=<jwt>`).

On failure, changed from `text/plain` error bodies to:

```
HTTP/1.1 302 Found
Location: {APP_BASE_URL}{APP_LOGIN_PATH}#error={code}
```

`code` ∈ `{cancelled, invalid_state, auth_failed, server_error}`.

No JSON:API resources are involved — this is a browser-redirect endpoint, not a data
endpoint, so JSON:API conventions do not apply.

**Fragment contract consumed by the SPA:**

| Fragment | Consumer | Behaviour |
| --- | --- | --- |
| `#access_token=<jwt>` | `lib/api/token.ts` (existing) | Store token, strip hash |
| `#error=<code>` | new `lib/auth/loginError.ts` | Map to message, strip hash |

New configuration: `APP_LOGIN_PATH`, default `/login`. Not added to any manifest —
`APP_HOME_PATH` and `APP_ONBOARDING_PATH` are likewise absent from `deploy/` and rely
on their Go defaults. Adding it to `deploy/` is out of scope.

## 6. Data Model

No entities, fields, relationships, or migrations. Nothing is persisted by this task
beyond the existing `localStorage` theme preference, whose schema is unchanged.

## 7. Service Impact

### 7.1 `apps/web`

| File | Change |
| --- | --- |
| `src/pages/LoginPage.tsx` | Rewritten to Direction D with three states |
| `src/pages/LoginPage.test.tsx` | New — covers composition, states, error fragment |
| `src/components/ThemeToggleButton.tsx` | New — presentational half extracted from `ThemeToggle` |
| `src/components/ThemeToggle.tsx` | Reduced to the mutation wrapper around the above |
| `src/components/ThemeToggleButton.test.tsx` | New — covers the extracted component |
| `src/components/GoogleMark.tsx` | New — the Google "G", decorative |
| `src/lib/auth/loginError.ts` | New — parse/strip `#error=`, map code to message |
| `src/lib/auth/loginError.test.ts` | New |

No new dependencies. `lucide-react` (spinner/icons) and the existing `Button` are
already present.

### 7.2 `apps/auth-service`

| File | Change |
| --- | --- |
| `internal/oidc/resource.go` | `LoginPath` on `Dependencies`; nine `http.Error` exits become redirects; new Google-`error` check |
| `internal/oidc/resource_test.go` | New cases per failure path asserting 302 + fragment |
| `cmd/main.go` | Wire `APP_LOGIN_PATH` |

### 7.3 `deploy/`

No changes. The kustomize overlays are unaffected; both still render and both server
dry-runs still apply, but nothing in this task alters a manifest.

### 7.4 Conflict warning — in-flight `return_to` work

At the time this PRD was written, a substantial uncommitted change existed on branch
`docs/project-readme` implementing post-login return paths: `safeReturnPath`,
`destination()`, a fifth field in the signed state cookie, `login(from)` in
`LoginPage.tsx`, and a matching `LoginPage.test.tsx`. **None of it is on `main`**, and
this worktree is branched from `main` @ `e9a76e6`, so this task's base does not
contain it.

Consequences the implementer must plan for:

1. `LoginPage.tsx` will conflict. The Direction D rewrite must preserve whatever
   signature `login()` ends up with — on `main` today it takes no argument; after the
   return_to work lands it takes `from`.
2. `LoginPage.test.tsx` is created by both branches. Reconcile rather than overwrite.
3. `resource.go`'s `callbackHandler` is touched by both. The return_to work changes the
   *success* path and the state-cookie shape; this task changes the *failure* exits.
   They are adjacent but not semantically overlapping.
4. `resource_test.go` is created by both branches.

Rebase onto `main` once the return_to work merges, before opening this task's PR.

## 8. Non-Functional Requirements

**Security**

- The error code is a fixed enum rendered through a lookup table; the raw fragment
  value is never interpolated into the DOM (FR-STATE-6).
- Codes are coarse by design — no exception text, no HTTP status, no service name
  reaches the browser (FR-ERR-4).
- The redirect target derives entirely from server config (FR-ERR-9).
- Fragment rather than query keeps codes out of server logs and `Referer` (FR-ERR-3).
- No change to token lifetime, cookie flags, `SameSite`, or the HMAC state scheme.

**Observability**

- Failure logging is byte-for-byte unchanged (FR-ERR-7). The redirect is additive to
  the existing `log.WithError(...)` calls, not a replacement.

**Performance**

- No images, no webfonts, no new bundle dependencies. The page should not measurably
  change `dist/assets/index-*.js`, which is already flagged at ~553 kB.

**Compatibility**

- An old SPA build receiving a new `#error=` fragment ignores it harmlessly and shows
  the ready state — the fragment is simply unread. No lockstep deploy is required.

## 9. Open Questions

1. Exact wording of the failure callout and the cancelled line. Proposed: "Google
   didn't complete the sign-in. Nothing was saved. Try again, or use a different
   account." and "Sign-in cancelled." Settle during design.
2. Whether the footer row (FR-PAGE-7) earns its place or is noise once the page is
   built at real size. It is the most cuttable element in the composition.
3. Whether `invalid_state` deserves distinct copy from `auth_failed`. Both are
   "something went wrong in the handshake" from the user's point of view; the split
   exists for log correlation, not for the reader.
4. Whether the pre-auth theme control should be visually quieter than the header one
   given it is the only chrome on an otherwise bare page.

## 10. Acceptance Criteria

- [ ] `/login` renders with no `Card`, left-aligned, on `bg-background` (FR-PAGE-1,
      FR-PAGE-2).
- [ ] Headline reads `Your cars.` / `One place.` with the second line muted
      (FR-PAGE-4).
- [ ] Sign-in button is solid, carries the Google mark, and names the three scopes
      beside it (FR-PAGE-6, FR-PAGE-8).
- [ ] Pressing the button disables it and shows a redirecting state (FR-STATE-2,
      FR-STATE-3).
- [ ] Loading `/login#error=auth_failed` renders the danger callout with
      `role="alert"` and relabels the button "Try again" (FR-STATE-4, FR-A11Y-1).
- [ ] Loading `/login#error=cancelled` renders a neutral muted line, not the callout,
      and leaves the button label unchanged (FR-STATE-5).
- [ ] Loading `/login#error=<garbage>` renders the generic failure callout and does not
      echo the supplied string (FR-STATE-6).
- [ ] The error fragment is stripped from the URL after being read (FR-STATE-7).
- [ ] A theme control is present on `/login`, cycles all three preferences, and fires
      **no** network request (FR-PRETOGGLE-1, FR-PRETOGGLE-3). Verified by asserting no
      `PATCH` is issued.
- [ ] `ThemeToggle` in `AppLayout` still fires its mutation and still toasts on failure
      — task-003's existing tests pass unmodified (FR-PRETOGGLE-4).
- [ ] All nine `callbackHandler` failure exits return 302 with the correct
      `#error=` fragment; `grep -c "http.Error" internal/oidc/resource.go` returns 0
      (FR-ERR-1, FR-ERR-5).
- [ ] A callback carrying Google's `?error=access_denied` redirects with `cancelled`
      (FR-ERR-6).
- [ ] Existing failure log lines still fire, asserted in `resource_test.go`
      (FR-ERR-7).
- [ ] The early failure exits clear the state cookie, asserted by checking for a
      `Set-Cookie` expiring `oidc_state` on the `cancelled` and `invalid_state`
      responses (FR-ERR-8).
- [ ] `APP_LOGIN_PATH` defaults to `/login` and overrides correctly (FR-ERR-2).
- [ ] Page is legible and correctly themed at 320px, in light and dark (FR-PAGE-9,
      FR-PAGE-10).
- [ ] `make ci` passes.
- [ ] Both kustomize overlays still render and both server dry-runs still apply
      (unchanged by this task, but re-verified per CLAUDE.md).
- [ ] Code review run via `superpowers:requesting-code-review` before the PR, with
      findings recorded in this folder's `audit.md`.
