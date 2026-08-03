# Sign Out From The Fleetless Pages — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give an authenticated, fleetless user a way to end their session from `/onboarding` and from the `/invites/:token/accept` error state, via one shared component that calls the existing `useAuth().logout()`.

**Architecture:** One new directory `apps/web/src/components/auth/` holds a pure label resolver (`accountLabel.ts`) and a presentational component (`SignedInFooter.tsx`). The component reads `{ user, logout }` from `useAuth()`, renders `Signed in as <account>` plus a `Not you? Sign out` button, and performs **no navigation** — the redirect to `/login` falls out of `RequireAuth` re-rendering once `AuthContext.logout()` flips `hasToken`. Two pages mount it. Nothing else in the auth path is touched.

**Tech Stack:** React 18 + TypeScript, Vite, Vitest + jsdom, @testing-library/react + user-event, shadcn/ui `Button`, Tailwind, `sonner` for toasts, `@myfleet/shared-ts` for `createErrorFromUnknown`.

## Global Constraints

- **Do not modify** `apps/web/src/components/frame/identityLines.ts`, `apps/web/src/components/RequireAuth.tsx`, `apps/web/src/context/AuthContext.tsx`, `apps/web/src/lib/hooks/api/auth.ts`, `apps/web/src/components/frame/ProfileMenu.tsx`, or `apps/web/src/test/renderWithProviders.tsx`. (PRD §2 non-goals, design §9.)
- **No backend, API, data-model, or `deploy/k8s` change.** This task is `apps/web` only.
- The new component must **never** call `clearAccessToken()`, touch `localStorage`, issue its own `fetch`, or call `useNavigate` / assign `window.location`. (FR-SIGNOUT-1, FR-SIGNOUT-3.)
- Accessible name of the control is exactly **`Sign out`**. The words **`Not you?`** are adjacent static text and must not be part of that accessible name. (FR-A11Y-1, FR-A11Y-2.)
- Identity line copy is exactly **`Signed in as {account}`**, omitted entirely when there is no account label. Never render `Signed in as` with nothing after it, and never a placeholder like `Account`. (FR-IDENT-1, FR-IDENT-3.)
- No `<h1>`–`<h6>` element anywhere in the new component. (FR-A11Y-3.)
- Tailwind utility classes and existing shadcn/ui primitives only. No new dependency, no bespoke CSS file. (NFR-4.)
- **No hardcoded palette classes** (`bg-gray-*`, `text-white`, `text-red-*`, …). `apps/web/src/test/conventions.test.ts` fails the build on these repo-wide. Use semantic tokens (`text-muted-foreground`).
- jsdom cannot see CSS (`vite.config.ts` sets `css: false`). Every assertion is on role, accessible name, rendered text, request, or resulting route — **never** on visual styling. (NFR-5.)
- Node is not always on `PATH`. Before any `npm`/`npx`/`make fe-*` command, run:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`
- All work happens in the worktree `/home/tumidanski/source/MyFleet/.worktrees/task-024-onboarding-sign-out` on branch `task-024-onboarding-sign-out`. Paths below are relative to that root.

---

## File Structure

| File | Status | Responsibility |
|---|---|---|
| `apps/web/src/components/auth/accountLabel.ts` | create | Pure `email → displayName → null` resolver |
| `apps/web/src/components/auth/accountLabel.test.ts` | create | Unit tests for the resolver |
| `apps/web/src/components/auth/SignedInFooter.tsx` | create | The affordance: identity line + `Not you? Sign out` |
| `apps/web/src/components/auth/SignedInFooter.test.tsx` | create | Component behaviour, a11y, in-flight, error branch |
| `apps/web/src/components/auth/signOutFlow.test.tsx` | create | Integration: real `AuthProvider` + real `RequireAuth` + stubbed `fetch` |
| `apps/web/src/pages/OnboardingPage.tsx` | modify | Mount the footer as the last child of the page column |
| `apps/web/src/pages/OnboardingPage.test.tsx` | modify | Add the `useAuth` mock (mandatory — see Task 4) + two footer cases |
| `apps/web/src/pages/InviteAcceptPage.tsx` | modify | Mount the footer in the **error** branch only |
| `apps/web/src/pages/InviteAcceptPage.test.tsx` | create | The page has no test file today |

`src/components/auth/` does not exist yet — Task 1 creates it. The name is deliberate: `components/frame/` means "lives in the app shell", and this component exists precisely because these two pages are **outside** the shell.

---

## Task 1: `accountLabel` — the identity resolver

**Files:**
- Create: `apps/web/src/components/auth/accountLabel.ts`
- Test: `apps/web/src/components/auth/accountLabel.test.ts`

**Interfaces:**
- Consumes: `User` from `apps/web/src/types/models/user.ts` — `JsonApiResource<UserAttributes>` where `UserAttributes = { email: string; displayName: string; avatarUrl: string; themePreference: ThemePreference }`.
- Produces: `export function accountLabel(user: User | null | undefined): string | null` — Task 2 imports this.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/auth/accountLabel.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { accountLabel } from './accountLabel';
import type { User } from '../../types/models/user';

function user(overrides: Partial<User['attributes']> = {}): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      displayName: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: '',
      themePreference: 'system',
      ...overrides,
    },
  };
}

describe('accountLabel', () => {
  it('returns the email when there is one', () => {
    expect(accountLabel(user({ displayName: '' }))).toBe('ada@example.com');
  });

  // The deliberate divergence from components/frame/identityLines.ts, which
  // orders displayName first. This assertion IS the requirement (FR-IDENT-2):
  // the footer disambiguates an account, and two Google accounts routinely
  // share one display name.
  it('prefers the email over the display name when both are present', () => {
    expect(accountLabel(user())).toBe('ada@example.com');
  });

  it('falls back to the display name when there is no email', () => {
    expect(accountLabel(user({ email: '' }))).toBe('Ada Lovelace');
  });

  it('treats a whitespace-only email as absent', () => {
    expect(accountLabel(user({ email: '   ' }))).toBe('Ada Lovelace');
  });

  it('trims surrounding whitespace off the value it returns', () => {
    expect(accountLabel(user({ email: '  ada@example.com  ' }))).toBe('ada@example.com');
  });

  // FR-IDENT-3: null, not '' and not 'Account'. The caller omits the whole
  // identity line on null; a placeholder would defeat the point of the line.
  it('returns null when both fields are empty', () => {
    expect(accountLabel(user({ email: '', displayName: '' }))).toBeNull();
  });

  it('returns null when both fields are whitespace only', () => {
    expect(accountLabel(user({ email: ' ', displayName: '\t' }))).toBeNull();
  });

  it('returns null for a null user', () => {
    expect(accountLabel(null)).toBeNull();
  });

  it('returns null for an undefined user', () => {
    expect(accountLabel(undefined)).toBeNull();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
cd apps/web && npx vitest run src/components/auth/accountLabel.test.ts
```

Expected: FAIL — `Failed to resolve import "./accountLabel"`.

- [ ] **Step 3: Write the implementation**

Create `apps/web/src/components/auth/accountLabel.ts`:

```ts
import type { User } from '../../types/models/user';

/**
 * The account shown in `Signed in as …` on the fleetless pages (FR-IDENT-2).
 *
 * email → displayName → null.
 *
 * This deliberately DISAGREES with components/frame/identityLines.ts, which
 * orders displayName → email → "Account". The two are not duplicates and must
 * not be collapsed (FR-IDENT-4):
 *
 *   - The profile-menu header is a GREETING. A friendly name is the right first
 *     line there, and "Account" is an acceptable stand-in when there is none.
 *   - This footer is a DISAMBIGUATION, read by someone who may have signed in
 *     with the wrong account. The email is the identifying field, a display
 *     name is not unique (two Google accounts routinely share one), and a
 *     placeholder actively defeats the question the line exists to answer.
 *
 * `null` rather than '' or 'Account' so that "there is nothing to show" is a
 * type-level fact at the call site instead of a `!== ''` convention the next
 * editor has to notice.
 */
export function accountLabel(user: User | null | undefined): string | null {
  const email = (user?.attributes.email ?? '').trim();
  if (email) return email;

  const displayName = (user?.attributes.displayName ?? '').trim();
  if (displayName) return displayName;

  return null;
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd apps/web && npx vitest run src/components/auth/accountLabel.test.ts
```

Expected: PASS — 9 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/auth/accountLabel.ts apps/web/src/components/auth/accountLabel.test.ts
git commit -m "feat(web): add accountLabel resolver for the fleetless sign-out footer"
```

---

## Task 2: `SignedInFooter` — the affordance

**Files:**
- Create: `apps/web/src/components/auth/SignedInFooter.tsx`
- Test: `apps/web/src/components/auth/SignedInFooter.test.tsx`

**Interfaces:**
- Consumes: `accountLabel(user)` from Task 1. `useAuth()` from `apps/web/src/context/AuthContext.tsx`, which returns `AuthContextValue = { user: User | null; activeFleetId: string | null; role: FleetRole | null; platformAdmin: boolean; isAuthenticated: boolean; isLoading: boolean; login: (returnTo?: string) => void; logout: () => Promise<void> }`. `Button` from `apps/web/src/components/ui/button.tsx`. `createErrorFromUnknown` from `@myfleet/shared-ts`.
- Produces: `export function SignedInFooter(): JSX.Element | null` — takes **no props**. Tasks 3, 4 and 5 mount it as `<SignedInFooter />`.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/auth/SignedInFooter.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { toast } from 'sonner';
import { SignedInFooter } from './SignedInFooter';
import type { AuthContextValue } from '../../context/AuthContext';
import type { User } from '../../types/models/user';

// The pattern eleven other test files use (ProfileMenu.test.tsx:8-11): mock the
// context module rather than wrapping in a real AuthProvider, which would mount
// a live useMe query whose `enabled` depends on ambient localStorage.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

function user(overrides: Partial<User['attributes']> = {}): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      displayName: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: '',
      themePreference: 'system',
      ...overrides,
    },
  };
}

function renderFooter(overrides: Partial<AuthContextValue> = {}) {
  mockAuth.mockReturnValue({
    user: user(),
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn().mockResolvedValue(undefined),
    ...overrides,
  });
  return render(<SignedInFooter />);
}

describe('SignedInFooter', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockAuth.mockReset();
  });

  it('names the account by email', () => {
    renderFooter();
    expect(screen.getByText('Signed in as ada@example.com')).toBeInTheDocument();
  });

  it('falls back to the display name when there is no email', () => {
    renderFooter({ user: user({ email: '' }) });
    expect(screen.getByText('Signed in as Ada Lovelace')).toBeInTheDocument();
  });

  // FR-IDENT-3. The control must survive the missing-identity case — this is
  // the user who is MOST stuck, not least.
  it('omits the identity line entirely when there is neither, keeping the control', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockResolvedValue(undefined);
    renderFooter({ user: user({ email: '', displayName: '' }), logout });

    expect(screen.queryByText(/Signed in as/)).not.toBeInTheDocument();

    await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));
    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-A11Y-2: `Not you?` is adjacent text, so the accessible name is exactly
  // 'Sign out'. getByRole with an exact string name is the assertion — it fails
  // if the words leak into the button.
  it('exposes the control as a button named exactly "Sign out"', () => {
    renderFooter();

    const button = screen.getByRole('button', { name: 'Sign out' });
    expect(button).toHaveAttribute('type', 'button');
    expect(screen.getByText(/Not you\?/)).toBeInTheDocument();
  });

  // FR-A11Y-3: no heading element, so the host page's heading outline is
  // unchanged by mounting this.
  it('renders no heading element', () => {
    const { container } = renderFooter();
    expect(container.querySelector('h1, h2, h3, h4, h5, h6')).toBeNull();
  });

  // FR-A11Y-1: keyboard reachable and activatable with no custom key handling.
  it('is reachable by Tab and activated by Enter', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockResolvedValue(undefined);
    renderFooter({ logout });

    await userEvents.tab();
    expect(screen.getByRole('button', { name: 'Sign out' })).toHaveFocus();

    await userEvents.keyboard('{Enter}');
    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-SIGNOUT-1: the ONE thing a click may do.
  it('calls logout exactly once on click', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockResolvedValue(undefined);
    renderFooter({ logout });

    await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));

    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-SIGNOUT-5 / FR-A11Y-4: the real `disabled` attribute, so a double-click
  // cannot issue two revocations and assistive tech is told the control is off.
  it('disables the control while sign-out is in flight, so a double click issues one logout', async () => {
    const userEvents = userEvent.setup();
    // Never settles: the component stays in its in-flight state for the whole
    // test, which is exactly the window being asserted on.
    const logout = vi.fn(() => new Promise<void>(() => {}));
    renderFooter({ logout });

    const button = screen.getByRole('button', { name: 'Sign out' });
    await userEvents.click(button);

    expect(button).toBeDisabled();

    await userEvents.click(button);
    expect(logout).toHaveBeenCalledTimes(1);
  });

  // FR-SIGNOUT-4. Unreachable through today's transport — logoutRequest()
  // terminates its fetch with `.catch(() => undefined)` (lib/hooks/api/auth.ts:70),
  // so logout() has exactly one outcome. Injecting a rejecting logout through
  // the mocked context is the only way to reach the branch, and the test stays
  // valid once task-022 makes the branch live.
  it('reports a failed sign-out, leaves the user signed in, and lets nothing escape', async () => {
    const escaped: unknown[] = [];
    const onUnhandled = (reason: unknown) => escaped.push(reason);
    process.on('unhandledRejection', onUnhandled);

    try {
      const userEvents = userEvent.setup();
      const logout = vi.fn().mockRejectedValue(new Error('network down'));
      renderFooter({ logout });

      await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));

      expect(toast.error).toHaveBeenCalledWith('network down');
      // Re-enabled: the user is still signed in and must be able to retry.
      expect(screen.getByRole('button', { name: 'Sign out' })).toBeEnabled();

      // One macrotask boundary is enough for Node to have reported any
      // rejection that reached the top level.
      await new Promise((resolve) => setTimeout(resolve, 0));
      expect(escaped).toEqual([]);
    } finally {
      process.off('unhandledRejection', onUnhandled);
    }
  });

  it('falls back to generic copy when the failure carries no message', async () => {
    const userEvents = userEvent.setup();
    const logout = vi.fn().mockRejectedValue(new Error(''));
    renderFooter({ logout });

    await userEvents.click(screen.getByRole('button', { name: 'Sign out' }));

    expect(toast.error).toHaveBeenCalledWith('Could not sign out');
  });

  // FR-SHARED-3: defence-in-depth, so callers can mount it unconditionally.
  it('renders nothing when there is no authenticated user', () => {
    const { container } = renderFooter({ user: null });
    expect(container).toBeEmptyDOMElement();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd apps/web && npx vitest run src/components/auth/SignedInFooter.test.tsx
```

Expected: FAIL — `Failed to resolve import "./SignedInFooter"`.

- [ ] **Step 3: Write the implementation**

Create `apps/web/src/components/auth/SignedInFooter.tsx`:

```tsx
import { useState } from 'react';
import { toast } from 'sonner';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { useAuth } from '../../context/AuthContext';
import { Button } from '../ui/button';
import { accountLabel } from './accountLabel';

/**
 * The way out of an account from the fleetless pages (FR-SHARED-1).
 *
 * `/onboarding` and `/invites/:token/accept` are routed OUTSIDE AppLayout
 * (App.tsx), so neither renders FrameHeader and neither renders ProfileMenu —
 * which was, until this component, the only sign-out control in the product. A
 * user who signed in with the wrong Google account had no exit but creating a
 * fleet they did not want or hand-clearing localStorage.
 *
 * It performs NO navigation (FR-SIGNOUT-3). `logout()` flips `hasToken` in
 * AuthContext, the provider re-renders, and RequireAuth's own
 * `<Navigate to="/login" replace>` branch does the rest. Adding a useNavigate
 * here would duplicate a redirect that already happens.
 *
 * It also never clears the token itself (FR-SIGNOUT-1 / NFR-1): the refresh
 * token is an HttpOnly cookie only the server can invalidate, and a control
 * that merely cleared local state would say "signed out" over a session that is
 * still resumable — precisely the gap that matters on the shared machine where
 * "I signed in with the wrong account" most often happens.
 */
export function SignedInFooter() {
  const { user, logout } = useAuth();
  const [signingOut, setSigningOut] = useState(false);

  if (!user) return null;

  const label = accountLabel(user);

  const signOut = async () => {
    setSigningOut(true);
    try {
      await logout();
      // Deliberately no setSigningOut(false) and no navigation on success: the
      // component is about to be unmounted by RequireAuth's redirect, so a
      // reset would be either a no-op or a post-unmount write, and it would
      // briefly re-enable the button mid-redirect. The flag stays latched from
      // the click until the component ceases to exist.
    } catch (err) {
      toast.error(createErrorFromUnknown(err).message || 'Could not sign out');
      setSigningOut(false);
    }
  };

  return (
    <div className="flex w-full max-w-md flex-col items-center gap-1 text-sm">
      {label !== null && (
        <p className="max-w-full truncate text-muted-foreground">Signed in as {label}</p>
      )}
      <p className="text-muted-foreground">
        {/* `Not you?` is a sibling text node, NOT inside the button, so the
            accessible name stays exactly "Sign out" (FR-A11Y-2). A <button> is
            phrasing content, so nesting it in a <p> is valid HTML. */}
        Not you?{' '}
        <Button
          type="button"
          variant="link"
          size="sm"
          // link + h-auto p-0 gives a real <button> with no filled surface and
          // no button-sized box, so it reads as subordinate to "Create Fleet"
          // (FR-ONBOARD-3). text-muted-foreground overrides the variant's
          // text-primary for the same reason.
          className="h-auto p-0 align-baseline text-muted-foreground underline"
          disabled={signingOut}
          // `void` on the handler only because a DOM handler returning a
          // promise is a lint smell. The promise from logout() IS awaited, one
          // frame down inside signOut — which is what makes the rejection
          // catchable at all (FR-SIGNOUT-2).
          onClick={() => void signOut()}
        >
          Sign out
        </Button>
      </p>
    </div>
  );
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd apps/web && npx vitest run src/components/auth/SignedInFooter.test.tsx
```

Expected: PASS — 11 tests.

- [ ] **Step 5: Prove the in-flight guard is not vacuous**

Temporarily delete the `disabled={signingOut}` prop from `SignedInFooter.tsx`, re-run:

```bash
cd apps/web && npx vitest run src/components/auth/SignedInFooter.test.tsx
```

Expected: FAIL — `disables the control while sign-out is in flight…` reports the button is not disabled and `logout` was called twice. **Restore the prop** and re-run to confirm PASS before continuing.

- [ ] **Step 6: Prove the error branch is not vacuous**

Temporarily replace the body of `signOut` with `await logout();` alone (no try/catch), re-run:

```bash
cd apps/web && npx vitest run src/components/auth/SignedInFooter.test.tsx
```

Expected: FAIL — `reports a failed sign-out…` reports `toast.error` was not called and/or `escaped` is non-empty. **Restore the try/catch** and re-run to confirm PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/components/auth/SignedInFooter.tsx apps/web/src/components/auth/SignedInFooter.test.tsx
git commit -m "feat(web): add SignedInFooter sign-out control for the fleetless pages"
```

---

## Task 3: Prove the server round-trip and the redirect route

The acceptance criteria demand two things a mocked-`logout` unit test cannot show: that `POST /api/auth/logout` is genuinely issued (NFR-1 — a client-only token clear must not pass), and that the redirect to `/login` is produced by `RequireAuth` rather than by hidden navigation (FR-SIGNOUT-3). This task mounts the real `AuthProvider`, the real `RequireAuth`, and stubs only `fetch`.

`SignedInFooter` — not `OnboardingPage` — is the guarded route's element on purpose: `OnboardingPage` would drag `usePendingInvites` and the fleet-creation form into a test about the auth round-trip. That the page mounts the footer is Task 4's job.

**Files:**
- Create: `apps/web/src/components/auth/signOutFlow.test.tsx`

**Interfaces:**
- Consumes: `SignedInFooter` (Task 2). `AuthProvider` from `../../context/AuthContext`. `RequireAuth` from `../RequireAuth`. `setAccessToken` / `clearAccessToken` from `../../lib/api/token`.
- Produces: nothing — test-only.

Facts this test depends on, all verified in source:
- `apiClient` is constructed with `baseUrl: ''` (`lib/api/client.ts:17-21`), so `fetchMe` calls `fetch('/api/auth/me', …)` with that exact string.
- `ApiClient.request` reads `res.status === 204 ? null : await res.json()` and throws unless `res.ok` (`packages/shared-ts/src/apiClient.ts`), so a stub needs only `{ ok, status, json }`.
- `useMe` has `enabled: !!getAccessToken()` (`lib/hooks/api/auth.ts:56`), so the token must be set **before** render.
- `logoutRequest` uses raw `fetch` with `credentials: 'include'` (`lib/hooks/api/auth.ts:65-71`).
- `/onboarding` is a `FLEETLESS_ROUTE` (`RequireAuth.tsx:13`), so a fleetless user is not bounced off it.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/components/auth/signOutFlow.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AuthProvider } from '../../context/AuthContext';
import { RequireAuth } from '../RequireAuth';
import { SignedInFooter } from './SignedInFooter';
import { clearAccessToken, setAccessToken } from '../../lib/api/token';

/**
 * The one test in this task that mounts real machinery: the real AuthProvider,
 * the real RequireAuth, a real two-route tree, with fetch stubbed at the
 * boundary and nothing else mocked.
 *
 * Asserting `logout` was called on a mock would pass just as happily against an
 * implementation that cleared localStorage itself — the exact failure NFR-1
 * exists to prevent. And because this tree contains no useNavigate, no
 * window.location write, and no route element other than RequireAuth's own
 * subject, the ONLY thing that can put "login screen" on the page is
 * RequireAuth's <Navigate> (FR-SIGNOUT-3).
 */

// The API client's 401-retry path imports refreshAccessToken from this module;
// stubbing it keeps a retry out of a test about one request.
vi.mock('../../lib/api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('new-token'),
  refreshAccessToken: vi.fn().mockResolvedValue(null),
}));

function meResponse() {
  return {
    ok: true,
    status: 200,
    json: async () => ({
      data: {
        id: 'u1',
        type: 'users',
        attributes: {
          email: 'ada@example.com',
          displayName: 'Ada Lovelace',
          avatarUrl: '',
          themePreference: 'system',
        },
      },
      // The fleetless identity this whole task exists for.
      meta: { activeFleetId: null, role: null, platformAdmin: false },
    }),
  };
}

function stubFetch() {
  const fetchMock = vi.fn(async (input: unknown) => {
    const url = String(input);
    if (url === '/api/auth/me') return meResponse();
    if (url === '/api/auth/logout') return { ok: true, status: 204, json: async () => null };
    throw new Error(`unexpected fetch: ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function renderGuardedFooter() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/onboarding']}>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<div>login screen</div>} />
            <Route
              path="/onboarding"
              element={
                <RequireAuth>
                  <SignedInFooter />
                </RequireAuth>
              }
            />
          </Routes>
        </AuthProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('signing out from a fleetless page', () => {
  beforeEach(() => {
    localStorage.clear();
    // Before render: useMe's `enabled` is evaluated on the first render.
    setAccessToken('token-123');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    clearAccessToken();
    localStorage.clear();
  });

  it('issues POST /api/auth/logout with the refresh cookie attached', async () => {
    const userEvents = userEvent.setup();
    const fetchMock = stubFetch();
    renderGuardedFooter();

    await userEvents.click(await screen.findByRole('button', { name: 'Sign out' }));

    const logoutCall = await waitFor(() => {
      const call = fetchMock.mock.calls.find(([url]) => String(url) === '/api/auth/logout');
      expect(call).toBeDefined();
      return call as unknown as [string, RequestInit];
    });

    expect(logoutCall[1].method).toBe('POST');
    // The HttpOnly refresh cookie is the entire reason this round-trip exists:
    // only the server can invalidate the refresh-token family, and the browser
    // only attaches the cookie on credentialled requests.
    expect(logoutCall[1].credentials).toBe('include');
  });

  it('lands on /login by way of RequireAuth, with no navigation code of its own', async () => {
    const userEvents = userEvent.setup();
    stubFetch();
    renderGuardedFooter();

    await userEvents.click(await screen.findByRole('button', { name: 'Sign out' }));

    expect(await screen.findByText('login screen')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  // The control that gives the assertion above its meaning: before the click,
  // the guard is letting the fleetless user sit on /onboarding rather than
  // redirecting. Without this, "login screen appeared" could mean the guard
  // was redirecting all along.
  it('does not redirect the fleetless user before they ask to sign out', async () => {
    stubFetch();
    renderGuardedFooter();

    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.queryByText('login screen')).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test**

```bash
cd apps/web && npx vitest run src/components/auth/signOutFlow.test.tsx
```

Expected: PASS — 3 tests. (`SignedInFooter` already exists from Task 2, so this is a characterisation test of the assembled system rather than a red-first test. Step 3 is what proves it can fail.)

- [ ] **Step 3: Prove the round-trip assertion is not vacuous**

Temporarily change `SignedInFooter.tsx`'s `signOut` to call `clearAccessToken()` from `../../lib/api/token` instead of `await logout()` — the client-only sign-out NFR-1 forbids. Re-run:

```bash
cd apps/web && npx vitest run src/components/auth/signOutFlow.test.tsx
```

Expected: FAIL — `issues POST /api/auth/logout…` reports no matching call. **Revert `SignedInFooter.tsx` to the Task 2 version** (`git checkout -- apps/web/src/components/auth/SignedInFooter.tsx`) and re-run to confirm PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/components/auth/signOutFlow.test.tsx
git commit -m "test(web): prove fleetless sign-out revokes server-side and redirects via RequireAuth"
```

---

## Task 4: Mount the footer on `OnboardingPage`

**Files:**
- Modify: `apps/web/src/pages/OnboardingPage.tsx` (add import; add `<SignedInFooter />` as the last child of the page column at lines 100-132)
- Modify: `apps/web/src/pages/OnboardingPage.test.tsx` (add the `useAuth` mock; add two cases)

**Interfaces:**
- Consumes: `SignedInFooter` from `../components/auth/SignedInFooter` (Task 2).
- Produces: nothing new for later tasks.

**This breaks the four existing `OnboardingPage` tests, and that is expected.** `renderWithProviders` wraps only `QueryClientProvider` and `MemoryRouter` — deliberately **not** `AuthProvider` (`src/test/renderWithProviders.tsx:36-42`). `OnboardingPage` does not consume `useAuth()` today, so its tests pass. The moment it renders `SignedInFooter`, all four throw `useAuth must be used within an AuthProvider`. The fix is the `vi.mock` block below — **not** adding `AuthProvider` to `renderWithProviders`, which would mount a live `useMe` query in every component test in the repo and make behaviour depend on ambient `localStorage`.

- [ ] **Step 1: Write the failing test**

Edit `apps/web/src/pages/OnboardingPage.test.tsx`.

First, add these two type imports after the existing `import type { Invite } …` line:

```ts
import type { AuthContextValue } from '../context/AuthContext';
import type { User } from '../types/models/user';
```

Then add this block immediately after the existing `vi.mock('../lib/api/refresh', …)` block:

```ts
// OnboardingPage now renders SignedInFooter, which consumes useAuth().
// renderWithProviders deliberately does NOT wrap AuthProvider, so the context
// module is mocked here — the pattern LoginPage.test.tsx, AppLayout.test.tsx
// and ProfileMenu.test.tsx already use.
const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function signedInUser(overrides: Partial<User['attributes']> = {}): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      displayName: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: '',
      themePreference: 'system',
      ...overrides,
    },
  };
}

function fleetlessAuth(): AuthContextValue {
  return {
    user: signedInUser(),
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn().mockResolvedValue(undefined),
  };
}
```

Then extend the existing `beforeEach` so it reads:

```ts
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(inviteService.listPending).mockResolvedValue({ data: [], meta: undefined });
  mockAuth.mockReturnValue(fleetlessAuth());
});
```

Finally, append these two cases inside the existing `describe('OnboardingPage', …)` block, after the last test:

```tsx
  // FR-ONBOARD-1/2. The exit from the account has to be here on the first-run
  // path too — the user who signed in with the wrong Google account has no
  // pending invite, and before this footer existed their only options were to
  // create a fleet they did not want or hand-clear localStorage.
  it('offers a way out of the account when nothing is pending', async () => {
    renderWithProviders(<OnboardingPage />);

    expect(await screen.findByLabelText(/fleet name/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.getByText('Signed in as ada@example.com')).toBeInTheDocument();
  });

  it('offers a way out of the account when an invite is waiting', async () => {
    vi.mocked(inviteService.listPending).mockResolvedValue({
      data: [pendingInvite()],
      meta: undefined,
    });

    renderWithProviders(<OnboardingPage />);

    expect(await screen.findByText(/Tumidanski Household/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
  });
```

- [ ] **Step 2: Run the tests to verify the new cases fail**

```bash
cd apps/web && npx vitest run src/pages/OnboardingPage.test.tsx
```

Expected: 4 existing tests PASS, 2 new tests FAIL with `Unable to find an accessible element with the role "button" and name "Sign out"`.

- [ ] **Step 3: Write the implementation**

Edit `apps/web/src/pages/OnboardingPage.tsx`.

Add the import after the existing `import { Input } from '../components/ui/input';` line:

```ts
import { SignedInFooter } from '../components/auth/SignedInFooter';
```

Then add the footer as the last child of the page-level flex column — change the closing of `OnboardingPage`'s returned JSX from:

```tsx
        </CardContent>
      </Card>
    </div>
  );
}
```

to:

```tsx
        </CardContent>
      </Card>
      {/* Outside PendingInvites, which returns null when nothing is waiting —
          so FR-ONBOARD-2 (it renders on the first-run path too) falls out
          structurally rather than needing a condition. The column already has
          gap-4 and items-center, so no layout change is needed. */}
      <SignedInFooter />
    </div>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd apps/web && npx vitest run src/pages/OnboardingPage.test.tsx
```

Expected: PASS — 6 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/pages/OnboardingPage.tsx apps/web/src/pages/OnboardingPage.test.tsx
git commit -m "feat(web): offer sign-out from the onboarding page"
```

---

## Task 5: Mount the footer on the invite-accept error state

**Files:**
- Modify: `apps/web/src/pages/InviteAcceptPage.tsx` (add import; add `<SignedInFooter />` to the error branch at lines 64-73 only)
- Create: `apps/web/src/pages/InviteAcceptPage.test.tsx` (the page has no test file today)

**Interfaces:**
- Consumes: `SignedInFooter` from `../components/auth/SignedInFooter` (Task 2).
- Produces: nothing new for later tasks.

Why this page is in scope: `Go to Dashboard` re-enters `RequireAuth`, which sends a fleetless user straight back to `/onboarding`. On the "this invite was sent to a different email address" error, the existing button is the one that loops and the new footer is the one that resolves the problem. The error copy, the `detail`-over-`message` precedence at line 37, and the existing button are all unchanged (FR-INVITE-3) — this task adds a control, it does not restyle the screen.

`useParams` is what supplies the token, so the test must render the page under a real `<Route path="/invites/:token/accept">`. Rendering it bare would leave `token` undefined, the effect would return early, and the page would sit in `pending` forever.

- [ ] **Step 1: Write the failing test**

Create `apps/web/src/pages/InviteAcceptPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { screen } from '@testing-library/react';
import { Route, Routes } from 'react-router-dom';
import { ApiError } from '@myfleet/shared-ts';
import { renderWithProviders } from '../test/renderWithProviders';
import { inviteService } from '../services/api/InviteService';
import { InviteAcceptPage } from './InviteAcceptPage';
import type { AuthContextValue } from '../context/AuthContext';
import type { User } from '../types/models/user';
import type { Invite } from '../types/models/invite';

const navigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return { ...actual, useNavigate: () => navigate };
});

vi.mock('../services/api/InviteService', () => ({
  inviteService: { listPending: vi.fn(), acceptInvite: vi.fn() },
}));

// useAcceptInvite mints a token on success, and the API client's 401 retry path
// imports refreshAccessToken from the same module.
vi.mock('../lib/api/refresh', () => ({
  mintAccessToken: vi.fn().mockResolvedValue('new-token'),
  refreshAccessToken: vi.fn().mockResolvedValue('new-token'),
}));

// useAcceptInvite raises its own toast on failure; there is no <Toaster> here.
vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }));

const mockAuth = vi.fn<() => AuthContextValue>();
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockAuth(),
}));

function signedInUser(): User {
  return {
    id: 'u1',
    type: 'users',
    attributes: {
      displayName: 'Ada Lovelace',
      email: 'ada@example.com',
      avatarUrl: '',
      themePreference: 'system',
    },
  };
}

function acceptedInvite(): Invite {
  return {
    type: 'invites',
    id: 'i1',
    attributes: {
      fleetId: 'f1',
      fleetName: 'Tumidanski Household',
      email: 'ada@example.com',
      role: 'member',
      token: 'tok-abc',
      expiresAt: '2099-01-01T00:00:00Z',
      invitedByUserId: 'u2',
    },
  };
}

// useParams supplies the token, so the page must be mounted under its real
// route pattern — rendered bare, `token` is undefined, the effect returns
// early, and the page never leaves `pending`.
function renderAcceptPage() {
  return renderWithProviders(
    <Routes>
      <Route path="/invites/:token/accept" element={<InviteAcceptPage />} />
    </Routes>,
    { route: '/invites/tok-abc/accept' },
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  mockAuth.mockReturnValue({
    user: signedInUser(),
    activeFleetId: null,
    role: null,
    platformAdmin: false,
    isAuthenticated: true,
    isLoading: false,
    login: vi.fn(),
    logout: vi.fn().mockResolvedValue(undefined),
  });
});

describe('InviteAcceptPage', () => {
  // FR-INVITE-1. This is the sharpest form of the trap: the screen tells the
  // user they used the wrong account, and its only control routes a fleetless
  // user straight back to /onboarding.
  it('offers a way out of the account on the error state', async () => {
    vi.mocked(inviteService.acceptInvite).mockRejectedValue(
      new ApiError(409, 'conflict', 'Wrong account for this invite', 'sent to another address'),
    );

    renderAcceptPage();

    expect(await screen.findByText('Could not accept invite')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.getByText('Signed in as ada@example.com')).toBeInTheDocument();
  });

  // FR-INVITE-3: adding a control must not disturb the screen it is added to.
  //
  // The asserted text is the ApiError's `message`, not its `detail`. That is
  // current behaviour, verified rather than assumed: InviteAcceptPage re-wraps
  // an already-wrapped ApiError through createErrorFromUnknown, which only
  // reads `detail` off a raw { status, body } envelope — an ApiError instance
  // falls to the `e instanceof Error` branch and arrives with
  // detail === undefined. So `detail || message` yields the envelope's title.
  // Pre-existing, out of scope here (this task must not touch this screen's
  // error handling), and recorded in context.md as a follow-up candidate.
  it('leaves the existing error copy and Go to Dashboard button intact', async () => {
    vi.mocked(inviteService.acceptInvite).mockRejectedValue(
      new ApiError(409, 'conflict', 'Wrong account for this invite', 'sent to another address'),
    );

    renderAcceptPage();

    expect(await screen.findByText('Wrong account for this invite')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Go to Dashboard' })).toBeInTheDocument();
  });

  // FR-INVITE-2: transient, traps nobody, so a sign-out control here is noise.
  it('offers no sign-out while the accept is still in flight', async () => {
    vi.mocked(inviteService.acceptInvite).mockImplementation(() => new Promise<Invite>(() => {}));

    renderAcceptPage();

    expect(await screen.findByText('Accepting invite…')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  it('offers no sign-out on the success state', async () => {
    vi.mocked(inviteService.acceptInvite).mockResolvedValue(acceptedInvite());

    renderAcceptPage();

    expect(await screen.findByText('You have joined the fleet!')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the test to verify the error case fails**

```bash
cd apps/web && npx vitest run src/pages/InviteAcceptPage.test.tsx
```

Expected: `offers a way out of the account on the error state` FAILS with `Unable to find an accessible element with the role "button" and name "Sign out"`. The other three PASS.

- [ ] **Step 3: Write the implementation**

Edit `apps/web/src/pages/InviteAcceptPage.tsx`.

Add the import after the existing `import { Button } from '../components/ui/button';` line:

```ts
import { SignedInFooter } from '../components/auth/SignedInFooter';
```

Then change the final (error-state) return from:

```tsx
  return (
    <div className="flex flex-col items-center justify-center min-h-[40vh] gap-4">
      <XCircle className="h-8 w-8 text-destructive" />
      <p className="text-base font-medium">Could not accept invite</p>
      <p className="text-sm text-muted-foreground">{errorMessage}</p>
      <Button variant="outline" onClick={() => navigate('/')}>
        Go to Dashboard
      </Button>
    </div>
  );
```

to:

```tsx
  return (
    <div className="flex flex-col items-center justify-center min-h-[40vh] gap-4">
      <XCircle className="h-8 w-8 text-destructive" />
      <p className="text-base font-medium">Could not accept invite</p>
      <p className="text-sm text-muted-foreground">{errorMessage}</p>
      <Button variant="outline" onClick={() => navigate('/')}>
        Go to Dashboard
      </Button>
      {/* Error state only (FR-INVITE-1/2). "Go to Dashboard" re-enters
          RequireAuth, which sends a fleetless user straight back to
          /onboarding — so on a wrong-account failure the existing button is the
          one that loops and this footer is the one that resolves it. The
          pending and success states are transient and trap nobody. */}
      <SignedInFooter />
    </div>
  );
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd apps/web && npx vitest run src/pages/InviteAcceptPage.test.tsx
```

Expected: PASS — 4 tests.

- [ ] **Step 5: Prove the state-scoping assertions are not vacuous**

Temporarily move `<SignedInFooter />` into the `status === 'pending'` branch's `<div>` as well. Re-run:

```bash
cd apps/web && npx vitest run src/pages/InviteAcceptPage.test.tsx
```

Expected: FAIL — `offers no sign-out while the accept is still in flight`. **Remove it from the pending branch** and re-run to confirm PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/pages/InviteAcceptPage.tsx apps/web/src/pages/InviteAcceptPage.test.tsx
git commit -m "feat(web): offer sign-out from the invite-accept error state"
```

---

## Task 6: Full verification, visual check, and audit

**Files:**
- Create: `docs/tasks/task-024-onboarding-sign-out/audit.md` (written by the reviewer agent)
- Modify: none by hand, unless a check below turns something up.

- [ ] **Step 1: Confirm the untouchable files are untouched**

```bash
git diff --name-only main...HEAD
```

Expected: exactly these nine paths, and no others —

```
apps/web/src/components/auth/SignedInFooter.test.tsx
apps/web/src/components/auth/SignedInFooter.tsx
apps/web/src/components/auth/accountLabel.test.ts
apps/web/src/components/auth/accountLabel.ts
apps/web/src/components/auth/signOutFlow.test.tsx
apps/web/src/pages/InviteAcceptPage.test.tsx
apps/web/src/pages/InviteAcceptPage.tsx
apps/web/src/pages/OnboardingPage.test.tsx
apps/web/src/pages/OnboardingPage.tsx
```

plus the three planning docs under `docs/tasks/task-024-onboarding-sign-out/`. If `identityLines.ts`, `RequireAuth.tsx`, `AuthContext.tsx`, `lib/hooks/api/auth.ts`, `ProfileMenu.tsx` or `renderWithProviders.tsx` appears, revert that file — it is a Global Constraint violation.

- [ ] **Step 2: Confirm the new component adds no navigation and no network of its own**

```bash
grep -nE "useNavigate|window\.location|clearAccessToken|localStorage|fetch\(|useQuery|useMutation|apiClient" \
  apps/web/src/components/auth/SignedInFooter.tsx apps/web/src/components/auth/accountLabel.ts
```

Expected: no output. The navigation and token halves are FR-SIGNOUT-1 / FR-SIGNOUT-3; the query half is NFR-2 (the component consumes the already-cached `useMe()` result via `useAuth()` and must issue nothing on render). This is a belt-and-braces check on top of `signOutFlow.test.tsx`, which proves the positive property.

- [ ] **Step 3: Run the whole CI gate**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: PASS through `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, `carfax-template`. Paste the tail of the output into the commit or the audit — do not claim it passed without it.

If `lint-check` flags the `onClick={() => void signOut()}` line or the `no-unused-vars` on a test helper, fix the lint finding rather than disabling the rule.

- [ ] **Step 4: Visual check — the one thing jsdom cannot see**

FR-ONBOARD-3 requires the control to read as subordinate to `Create Fleet`, and NFR-5 rules that out of the test suite (`vite.config.ts` sets `css: false`, and the standing project note is that jsdom cannot see CSS). Drive a real browser per `docs/runbooks/local-debugging.md`:

- Bring up the stack and sign in as a user with **no fleet**, landing on `/onboarding`.
- Confirm: `Signed in as <email>` renders muted and small; `Not you? Sign out` sits below the fleet card; the sign-out control has no filled button surface and does not compete with `Create Fleet`.
- Confirm the identity line **truncates** rather than wrapping or widening its container at a narrow (mobile) viewport (FR-IDENT-5). Use a long email if the test account's is short.
- Click `Sign out` and confirm the browser lands on `/login`.
- Sign back in through the OAuth flow and confirm the user lands per the existing rules (acceptance criterion 2).

Record what you saw — including the viewport you checked truncation at — in the audit notes.

- [ ] **Step 5: Run the frontend guidelines audit**

Per NFR-3 and CLAUDE.md's "Code Review Before PR" rule, dispatch the `frontend-guidelines-reviewer` agent over the changed TypeScript/React files, with findings written to `docs/tasks/task-024-onboarding-sign-out/audit.md`.

- [ ] **Step 6: Resolve audit findings and commit**

Fix anything the audit raises, re-run `make ci`, then:

```bash
git add docs/tasks/task-024-onboarding-sign-out/audit.md
git commit -m "docs(task-024): frontend guidelines audit"
```

Do not open a PR with unresolved findings in `audit.md`.

---

## Acceptance criteria coverage

| Acceptance criterion (PRD §10) | Where it is proven |
|---|---|
| Fleetless user can sign out from `/onboarding` and reach `/login` without creating a fleet | Task 3 (`lands on /login by way of RequireAuth`), Task 4, Task 6 Step 4 |
| Signing back in through OAuth lands per existing rules | Task 6 Step 4 (manual — no automated OAuth round-trip exists) |
| `/onboarding` shows `Signed in as <email>` with and without pending invites | Task 4, both new cases |
| Display name but no email → `Signed in as <displayName>` | Task 1 case 3, Task 2 case 2 |
| Neither → no identity line, control still present and functional | Task 1 cases 6-7, Task 2 case 3 |
| Invite-accept error state renders the control; pending and success do not | Task 5, all four cases |
| Sign-out issues `POST /api/auth/logout`, asserted against the request | Task 3 case 1 (+ Step 3 proves it can fail) |
| Rejected `logout()` → error toast, still signed in, no unhandled rejection | Task 2 error case (+ Step 6 proves it can fail) |
| Control disabled in flight; double-click issues one request | Task 2 in-flight case (+ Step 5 proves it can fail) |
| Keyboard-reachable `<button>` named `Sign out`; `Not you?` excluded | Task 2 keyboard + accessible-name cases |
| Redirect driven by `RequireAuth`, no `useNavigate` / `window.location` | Task 3 cases 2-3, Task 6 Step 2 |
| `identityLines.ts`, `RequireAuth.tsx`, `AuthContext.tsx`, `auth.ts` unmodified | Task 6 Step 1 |
| `make ci` passes | Task 6 Step 3 |
| `frontend-guidelines-reviewer` audit with no unresolved findings | Task 6 Steps 5-6 |
