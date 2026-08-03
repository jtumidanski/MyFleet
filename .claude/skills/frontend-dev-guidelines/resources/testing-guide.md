# Frontend Testing Guide

## Overview

The runner is **Vitest** with React Testing Library and a jsdom environment (`apps/web/package.json:8` — `"test": "vitest run"`). There is no `__tests__/` directory anywhere; every test is a sibling of its subject: `components/PageHeader.tsx` + `components/PageHeader.test.tsx`, `lib/hooks/api/mileage.ts` + `lib/hooks/api/mileage.test.ts`.

`@testing-library/jest-dom` is a real matcher library and its import is **not** drift, despite the name. It is loaded as `@testing-library/jest-dom/vitest` (`src/test/setup.ts:1`) and declared in `tsconfig.app.json:21`. The *runner* API is Vitest's throughout: `vi.fn`, `vi.mock`, `vi.spyOn`, `vi.importActual`.

## Configuration

There is no standalone runner config file. Test config lives in the Vite config:

```typescript
// apps/web/vite.config.ts:19-24
test: {
  globals: true,
  environment: 'jsdom',
  setupFiles: ['./src/test/setup.ts'],
  css: false,
},
```

`globals: true` is why `describe` / `it` / `expect` work without an import in some files — though the convention is still to import them explicitly from `vitest`, as every test in the tree does.

**`css: false` matters when you write an assertion.** jsdom does not evaluate stylesheets, so a test can only assert that a *class* is present, never that it renders a particular way. `PageHeader.test.tsx:78-82` says this out loud: the test pins the class, not the 32px, and it will not catch a caller passing a taller control. If a bug is genuinely CSS-driven, the test suite cannot see it — drive a real browser instead.

`src/test/setup.ts` installs the jsdom gaps this app trips over, each with the reason recorded:
- a `localStorage` polyfill, because Node 22's experimental built-in shadows jsdom's and throws (`setup.ts:3-5`)
- a driveable `matchMedia`, because jsdom's stub never fires `change`, which would make the theme code untestable (`setup.ts:34-37`); tests flip it with `setPrefersDark()`
- a `ResizeObserver` stub, because cmdk's `<Command>` measures its list on mount (`setup.ts:86-89`)
- `scrollIntoView` and the pointer-capture methods, because Radix menus call them (`setup.ts:102-107`)

When a library throws `X is not defined` before any assertion runs, the fix is a minimal no-op stub here — not a mock of the component under test.

## Rendering: use `renderWithProviders`

`src/test/renderWithProviders.tsx` is the project's entry point for rendering anything that uses a hook or a `<Link>`. A bare `render()` on such a component throws — there is no `QueryClientProvider` and no router above it.

```typescript
// src/test/renderWithProviders.tsx:30-44
export function renderWithProviders(
  ui: ReactElement,
  options: RenderWithProvidersOptions = {},
): RenderResult & { queryClient: QueryClient } {
  const { queryClient = createTestQueryClient(), route = '/', ...renderOptions } = options;
  // ... wraps in QueryClientProvider + MemoryRouter, returns the client
}
```

- `route` seeds `MemoryRouter`'s initial entry (`renderWithProviders.tsx:22,39`). Prefer it over mocking `react-router-dom` outright.
- The returned `queryClient` lets a test seed or inspect the cache.
- `createTestQueryClient()` turns retries **off** (`renderWithProviders.tsx:7-16`) so a test asserting an error state reaches it on the first rejection instead of waiting out the app's `retry: 1`. Do not hand-roll a wrapper that omits this — the test will pass but take a second longer for no reason, or time out.

A component with no hooks and no links may use plain `render()` — `PageHeader.test.tsx:10` does.

## Unit tests (pure functions)

Exported pure helpers are tested directly, no rendering:

```typescript
// lib/hooks/api/mileage.test.ts
import { describe, it, expect } from 'vitest';
import { getLatestMileage } from './mileage';

describe('getLatestMileage', () => {
  it('returns the mileage of the record with the latest recordedAt', () => {
    // ...
  });
});
```

`lib/vehicleStats.test.ts` and `lib/carfax.test.ts` follow the same shape. Extracting a helper *so that* it can be tested this way — as `mileage.ts:26-32` does with `getLatestMileage` — is preferred over asserting the same logic through a rendered component.

## Component tests

```typescript
// components/features/vehicles/VehicleList.test.tsx:1-5
import { describe, it, expect } from 'vitest';
import { screen } from '@testing-library/react';
import { renderWithProviders } from '../../../test/renderWithProviders';
import { VehicleList } from './VehicleList';
import type { Vehicle } from '../../../types/models/vehicle';
```

Note the import specifiers: relative, at whatever depth the file sits. There is no `@/` alias configured in `tsconfig.app.json` or `vite.config.ts`, so an `@/`-prefixed import does not resolve.

Assertions go through roles and accessible names, not implementation details:

```typescript
expect(screen.getByRole('link', { name: '2019 Honda Civic' })).toBeInTheDocument();
expect(screen.queryByText(/No vehicles yet/)).not.toBeInTheDocument();
```

Fixtures are built by a local factory function typed against the real model (`VehicleList.test.tsx:7-13`), so a change to `VehicleAttributes` breaks the test at compile time rather than at runtime.

## Hook tests

Hooks are exercised with `renderHook` under a `QueryClientProvider`, with the service module mocked so no network call is needed:

```typescript
// lib/hooks/api/mileage.test.ts:1-15
import { describe, it, expect, vi } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { mileageKeys, getLatestMileage, useMileageRecords } from './mileage';

// Mock the service module so no network call is needed; each test controls
// what page(s) resolve.
vi.mock('../../../services/api/MileageService', () => ({
  mileageService: {
    listByVehicle: vi.fn(),
  },
}));
```

Key factories are asserted against their literal resolved shape (`mileage.test.ts:38-48`, `vehicles.test.ts:5-10`). That is what makes a key-factory change visible rather than silently re-partitioning the cache.

## Mocking

**Mock the concrete module, by relative path.** Every `vi.mock` factory is declared inline in the test file that needs it. There is no `services/api` barrel to mock, and no directory of shared manual mocks anywhere under `apps/web`.

```typescript
// pages/OnboardingPage.test.tsx:15-16
vi.mock('../services/api/InviteService', () => ({
  inviteService: { listPending: vi.fn(), acceptInvite: vi.fn() },
}));
```

The specifier must match the one the *subject* uses, at the depth the *test file* sits — this is the most common reason a mock silently fails to apply.

**Router** — prefer `renderWithProviders`' `route` option. When a test genuinely needs to observe navigation, mock partially so the rest of the module still works:

```typescript
// pages/OnboardingPage.test.tsx:10-13
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return { ...actual, useNavigate: () => navigate };
});
```

**Toast (sonner)** — mock the shape the subject actually calls:

```typescript
// components/features/settings/MemberList.test.tsx:37
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
```

`sonner`'s `toast` is callable *and* has methods, so a subject using both needs `Object.assign(vi.fn(), { error: vi.fn(), success: vi.fn() })` (`CategoryCombobox.test.tsx:37-41`).

When a service method's signature changes, every `vi.mock` of that module has to change with it in the same commit — nothing type-checks a mock factory's shape against the real module. `FE-17` checks this.

## Convention tests

`src/test/conventions.test.ts` is executable guideline enforcement: a test that fails when a documented convention is silently removed. It is the model to follow when a rule matters but types cannot express it.

What it currently pins:
- the pre-paint theme script in `index.html` — present, synchronous, wrapped in try/catch, under 500 bytes, and using the same storage key and media query as `ThemeContext` (`conventions.test.ts:16-69`). The comment records why the duplication cannot be removed.
- the brand mark being byte-identical in `brandMarkPath.ts` and `favicon.svg` (`:74-83`)
- **no hardcoded palette classes** in any `.tsx` under `apps/web/src` or `packages/ui-components/src` (`:113-178`) — this is `FE-06`'s executable half. It has a two-line allowlist, exempted by file *and* exact text, with the reasoning written out; do not widen it by relaxing the regex.
- every authenticated page rendering its title via `PageHeader` rather than a hand-written `<h1>` (`:184-202`)

Two details worth copying: the palette scan asserts each root walked produced at least one file, so a moved directory fails loudly instead of passing trivially (`:150-157`); and each `expect` carries a message telling the reader what to do instead (`:176`).

## Testing rules

1. **Mock the modules the subject imports** — services, `sonner`, and `react-router-dom` only when `route` will not do.
2. **Use `waitFor` for anything async** — a resolved query, a settled mutation.
3. **Query by role and accessible name**, not by class or test id.
4. **Use `userEvent` over `fireEvent`** for interactions.
5. **Reset mocks between tests** — `vi.clearAllMocks()` in `beforeEach`.
6. **Cover loading, error, and success** — the error path is the one `createTestQueryClient`'s `retry: false` exists to make reachable.
7. **Assert dialog open/close behaviour** — controlled components must respect the `open` prop.
8. **Assert accessibility, not markup** — `getByRole`, accessible names, `toHaveAccessibleName`.
9. **Say why in a comment when the assertion is non-obvious.** The tests in this tree carry the requirement ID or the defect they came from (`PageHeader.test.tsx:6-8`, `VehicleList.test.tsx:31-33`); that is what keeps a later reader from "tidying away" a load-bearing assertion.

## Running tests

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22   # if npm is not on PATH
make fe-test
make fe-build
```

Use `make fe-test`, **not** `npm run -w apps/web test`. The Makefile target runs three workspaces (`Makefile:25-28`): `apps/web`, `packages/shared-ts`, and `packages/ui-components`. The comment above it and `.github/workflows/pr.yml:107-114` record why — `shared-ts` owns `fetchAuthenticated`, the single 401-refresh path every SPA call goes through, and its tests previously ran in no automated gate at all; `ui-components` was in the same position.

## Pre-commit checklist

- [ ] `make fe-test` passes
- [ ] `make fe-build` passes (this runs `tsc -b`, so it is also the type check)
- [ ] `make ci` passes before opening a PR
