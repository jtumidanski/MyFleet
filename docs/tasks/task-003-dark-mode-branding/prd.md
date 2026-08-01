# Dark Mode & Application Branding — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-07-31

---

## 1. Overview

The MyFleet web app ships with a complete dark colour palette that nothing ever activates. `apps/web/tailwind.config.ts:5` sets `darkMode: ['class']` and `apps/web/src/index.css:29-51` defines a full `.dark` token block, but no code ever adds the `dark` class to the document, so every user sees the light theme regardless of their operating-system preference. This task turns that dormant palette into a real, user-controllable theme system.

"Remember it for the user" is the load-bearing half of the request. A browser-local toggle would reset every time the user signs in from their phone, a second laptop, or a fresh profile — for a household platform where the same person moves between devices, that reads as broken. The theme preference therefore becomes part of the user's identity record in `auth-service`, returned with `GET /auth/me` and updated through a new `PATCH /auth/me`, with a `localStorage` mirror used purely to paint the correct theme on first frame before the identity query resolves.

Dark mode is only convincing if every surface participates. Eight `.tsx` files plus `packages/ui-components/src/StatusBadge.tsx` currently hardcode Tailwind palette classes (`bg-gray-50`, `text-red-700`, `bg-amber-100`, …) instead of semantic tokens. Left alone they render as light-on-light or unreadable low-contrast smears once the background goes dark. This task introduces semantic status tokens (success / warning / danger / info) with light and dark values and converts those call sites.

Finally, the app has no icon of any kind: `apps/web/index.html` declares no favicon, there is no `apps/web/public/` directory, and browsers fall back to a blank document glyph. The sidebar brand is the bare text string `MyFleet` (`apps/web/src/components/AppLayout.tsx:18`). This task adds a generated vector mark used consistently as the browser favicon, the installed-app icon, and the in-app header brand.

## 2. Goals

Primary goals:

- A user can choose **Light**, **Dark**, or **System** and the entire application honours that choice.
- The choice persists **per user, server-side**, so it follows them to any browser or device where they sign in.
- The correct theme is applied **before first paint** — no flash of the wrong theme on load or refresh.
- **System** tracks the OS setting live, updating without a reload when the OS switches (e.g. at sunset).
- Every existing screen is legible and meets contrast requirements in both themes; no hardcoded palette colours remain in application code.
- The app has a distinct icon: browser tab favicon, iOS home-screen icon, installable web-app icons, and an in-app header mark.

Non-goals:

- Redesigning the visual language, spacing, typography, or component library. The existing shadcn token values in `index.css` stand as-is; only the new status tokens are added.
- Additional themes beyond light/dark (no high-contrast, sepia, or per-fleet custom branding).
- Theming any surface outside `apps/web` — no emails, no PDF/document output, no service-rendered HTML.
- Print stylesheets.
- Server-side rendering or any change to the nginx static-serving model.
- A general-purpose user-preferences subsystem. Exactly one preference field is added; a broader preferences API is out of scope.

## 3. User Stories

- As a signed-in user, I want to switch the app to dark mode from the header so that I can use MyFleet comfortably at night without leaving the page I am on.
- As a signed-in user, I want my theme choice to be remembered when I sign in on my phone so that I do not have to set it again on every device.
- As a user whose OS is set to dark mode, I want MyFleet to open in dark mode by default so that it matches every other app on my machine.
- As a user who has chosen System, I want the app to follow my OS when it flips at sunset so that I never have to touch the toggle.
- As a returning user, I want the app to open directly in my chosen theme so that I am not blinded by a white flash before the dark theme loads.
- As a user reviewing overdue maintenance in dark mode, I want severity chips and status badges to remain readable so that I can tell urgent items from informational ones.
- As a user, I want a recognisable MyFleet icon in my browser tab so that I can find it among many open tabs.
- As a mobile user, I want to add MyFleet to my home screen and see a proper icon so that it feels like a real app.

## 4. Functional Requirements

### 4.1 Theme model

- **FR-THEME-1** — The theme preference is one of exactly three values: `light`, `dark`, `system`. Any other value is invalid.
- **FR-THEME-2** — The default for a user who has never chosen is `system`.
- **FR-THEME-3** — The *resolved* theme is derived from the preference: `light` → light; `dark` → dark; `system` → dark when `window.matchMedia('(prefers-color-scheme: dark)').matches` is true, otherwise light.
- **FR-THEME-4** — The resolved theme is applied by adding or removing the `dark` class on `document.documentElement`. No other mechanism (inline styles, data attributes on `<body>`, CSS-in-JS) is used, because `tailwind.config.ts` is already configured for class-based dark mode.
- **FR-THEME-5** — When the preference is `system`, a `change` listener on the media query updates the resolved theme live, with no page reload. The listener is registered with `addEventListener('change', …)` and removed on unmount.
- **FR-THEME-6** — When the preference is `light` or `dark`, OS changes are ignored.

### 4.2 First-paint application (no flash)

- **FR-FLASH-1** — A small synchronous inline script in the `<head>` of `apps/web/index.html` applies the `dark` class before the app bundle loads and before first paint. It must not be `defer`red, `async`, or moved into a module.
- **FR-FLASH-2** — That script reads the cached preference from `localStorage` under the key `myfleet.theme`. If the key is absent, unreadable, or holds a value outside the three valid values, it falls back to `system` behaviour (media query).
- **FR-FLASH-3** — The script must be defensive: any exception (e.g. `localStorage` blocked by browser privacy settings) is swallowed and the app falls back to the light theme rather than failing to boot. Wrapping in `try { … } catch {}` is required.
- **FR-FLASH-4** — Loading the app while signed out (login page, invite-accept page) applies the cached/system theme identically. Theming is not gated on authentication.

### 4.3 Persistence and synchronisation

- **FR-PERSIST-1** — The authoritative store is the user's row in `auth-service`. `GET /auth/me` returns the preference; `PATCH /auth/me` updates it (see §5).
- **FR-PERSIST-2** — `localStorage['myfleet.theme']` is a **cache, not a source of truth**. It exists solely to satisfy FR-FLASH-1.
- **FR-PERSIST-3** — On a successful `GET /auth/me`, the server value is applied and written to `localStorage`, overwriting any cached value. Rationale: the server is authoritative, and this is what makes the preference propagate to a new device on first sign-in.
- **FR-PERSIST-4** — When the user changes the theme, the change is applied **optimistically**: local state and `localStorage` update immediately and the DOM class flips without waiting for the network. The `PATCH` is issued in the background.
- **FR-PERSIST-5** — If the `PATCH` fails, the user's choice **remains applied for the session** (their intent is honoured) and a non-blocking error toast is shown via the existing `sonner` toaster: *"Couldn't save your theme preference. It'll reset next time you sign in."* The local state is **not** rolled back. Rationale: reverting the visible theme under the user's cursor is more jarring than a preference that fails to stick.
- **FR-PERSIST-6** — On a successful `PATCH`, the React Query cache for `authKeys.me()` is updated so that a later refetch does not momentarily revert the theme.
- **FR-PERSIST-7** — Signing out clears `authKeys.all` from the query cache (existing behaviour in `AuthContext.logout`). The `localStorage` theme cache is **not** cleared on sign-out, so the login page retains the theme the user was last using on that device.
- **FR-PERSIST-8** — A theme change made while signed out (if a toggle is reachable there) updates only `localStorage`. No `PATCH` is attempted without a token.

### 4.4 Toggle control

- **FR-TOGGLE-1** — The control is an icon button in the application header (`AppLayout`), placed to the left of the existing display-name / Sign-out cluster.
- **FR-TOGGLE-2** — Activating it cycles the preference in a fixed order: `light` → `dark` → `system` → `light`.
- **FR-TOGGLE-3** — The icon reflects the **current preference**, not the resolved theme: `Sun` for `light`, `Moon` for `dark`, `Monitor` for `system` (all from the already-installed `lucide-react`).
- **FR-TOGGLE-4** — The button has an `aria-label` that names both the current state and the action, e.g. `"Theme: system. Switch to light."` The label updates as the preference cycles, so screen-reader users can operate the cycle without sighted feedback.
- **FR-TOGGLE-5** — The button has a `title` attribute matching the current preference for a hover tooltip.
- **FR-TOGGLE-6** — The button is keyboard reachable in normal tab order and shows a visible focus ring using the existing `--ring` token.
- **FR-TOGGLE-7** — The control renders only inside `AppLayout` (authenticated shell). Pre-auth pages have no toggle in this task; they inherit the cached/system theme per FR-FLASH-4.

### 4.5 Semantic status tokens

- **FR-TOKEN-1** — Four semantic colour families are added to `apps/web/src/index.css`, each defined in both the `:root` (light) and `.dark` blocks, following the existing `H S% L%` triplet convention (values without the `hsl()` wrapper):
  - `--success`, `--success-subtle`, `--success-subtle-foreground`, `--success-border`
  - `--warning`, `--warning-subtle`, `--warning-subtle-foreground`, `--warning-border`
  - `--danger`, `--danger-subtle`, `--danger-subtle-foreground`, `--danger-border`
  - `--info`, `--info-subtle`, `--info-subtle-foreground`, `--info-border`
- **FR-TOKEN-2** — The bare token (e.g. `--danger`) is intended for **text and numerals on the page background**. The `-subtle` / `-subtle-foreground` / `-border` trio is intended for **chips and callout blocks** (background / text / border respectively).
- **FR-TOKEN-3** — These are registered in `apps/web/tailwind.config.ts` under `theme.extend.colors` following the existing `DEFAULT` + variant object shape used by `primary`, `card`, etc., yielding classes such as `text-danger`, `bg-danger-subtle`, `text-danger-subtle-foreground`, `border-danger-border`.
- **FR-TOKEN-4** — The existing `destructive` tokens are **left untouched**. `destructive` is shadcn's button/alert semantic and is already used by form components; `danger` is the status-indication family. Conflating them would change existing button styling.
- **FR-TOKEN-5** — Dark-theme values must not be naive inversions. Chip backgrounds in dark mode use low-lightness, low-to-moderate-saturation fills with high-lightness foregrounds (the mirror of the light `-100` background / `-800` text pattern), chosen to satisfy FR-A11Y-1.

### 4.6 Hardcoded-colour conversion

Every site below moves to semantic tokens. This list is exhaustive as of `main` at `352e8c1`; the implementation must re-verify with a repo-wide grep (FR-CONVERT-10).

- **FR-CONVERT-1** — `apps/web/src/components/AppLayout.tsx` — sidebar (`border-gray-200 bg-gray-50`), nav links (`bg-gray-200 text-gray-900`, `text-gray-600 hover:bg-gray-100`), header (`border-gray-200`), display name (`text-gray-500`), sign-out button (`border-gray-300 hover:bg-gray-50`) → `border-border`, `bg-muted`/`bg-card`, `bg-accent text-accent-foreground`, `text-muted-foreground hover:bg-accent`, `border-input hover:bg-accent`. The sign-out button should adopt the existing `Button` component variant if it maps cleanly.
- **FR-CONVERT-2** — `apps/web/src/components/features/activity/ActivityEventIcon.tsx:15` — `bg-gray-100` → `bg-muted`.
- **FR-CONVERT-3** — `apps/web/src/components/features/vehicles/maintenance/SeverityChip.tsx` — the `severityConfig` map's `bg-red-100 text-red-800 border-red-200` / amber / blue triplets → `bg-danger-subtle text-danger-subtle-foreground border-danger-border` and the warning / info equivalents. The existing code comment claiming these colours "have no shadcn semantic equivalent" is superseded and must be updated to reference the new token families.
- **FR-CONVERT-4** — `apps/web/src/components/features/dashboard/widgets/FleetOverviewWidget.tsx:31,35,39` — `text-green-600` / `text-amber-600` / `text-red-600` → `text-success` / `text-warning` / `text-danger`.
- **FR-CONVERT-5** — `apps/web/src/components/features/dashboard/widgets/OverdueMaintenanceWidget.tsx:28` — `text-red-700` → `text-danger`.
- **FR-CONVERT-6** — `apps/web/src/components/features/dashboard/widgets/UpcomingMaintenanceWidget.tsx:28` — `text-amber-700` → `text-warning`.
- **FR-CONVERT-7** — `apps/web/src/components/features/vehicles/maintenance/MaintenanceQueueView.tsx:38,46,74` — `text-red-700`, `border-red-200 bg-red-50`, `text-amber-700` → `text-danger`, `border-danger-border bg-danger-subtle`, `text-warning`.
- **FR-CONVERT-8** — `apps/web/src/pages/PlaceholderPage.tsx:8` — `text-gray-500` → `text-muted-foreground`.
- **FR-CONVERT-9** — `packages/ui-components/src/StatusBadge.tsx` — the `VARIANT` map's four `bg-*-100 text-*-800` pairs → `bg-success-subtle text-success-subtle-foreground`, warning, danger, and `bg-muted text-muted-foreground` for `Inactive`. Note this package is a shared workspace already listed in the web app's Tailwind `content` globs (`tailwind.config.ts:9`), so the new classes are picked up without config changes.
- **FR-CONVERT-10** — After conversion, a repo-wide grep for hardcoded palette classes across `apps/web/src/**/*.tsx` and `packages/ui-components/src/**/*.tsx` must return no matches. Pattern: `(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)`.

### 4.7 Third-party surface

- **FR-3P-1** — The `sonner` `<Toaster />` in `apps/web/src/components/providers/AppProviders.tsx:26` renders its own portal outside the token-styled tree and must be passed the resolved theme via its `theme` prop (`'light' | 'dark'`). Without this, toasts render light-on-light in dark mode.
- **FR-3P-2** — Consequently, the theme provider must sit **above** `<Toaster />` in the provider tree so the toaster can read the resolved theme.
- **FR-3P-3** — Radix-based components (`select`, `switch`, `label`) render into portals but are styled with the project's own token classes, so they need no extra wiring. This must be visually confirmed rather than assumed — `select.tsx` in particular renders a floating content panel.

### 4.8 Branding and icons

- **FR-ICON-1** — A single-colour vector mark is designed for MyFleet: geometric, no lettering, legible when rendered at 16×16 CSS pixels. It is the sole source for every rasterised icon.
- **FR-ICON-2** — An `apps/web/public/` directory is created. Vite copies its contents verbatim into `dist/`, which the existing Dockerfile already ships (`COPY apps/web ./apps/web` then `COPY --from=build /app/apps/web/dist`) and nginx already serves from `/usr/share/nginx/html`. No Dockerfile or nginx change is required.
- **FR-ICON-3** — The asset set and the `<head>` wiring are specified in [`icon-assets.md`](./icon-assets.md).
- **FR-ICON-4** — `favicon.svg` embeds a `prefers-color-scheme` media query so the mark stays legible against both light and dark browser chrome in browsers that support SVG favicons.
- **FR-ICON-5** — `favicon.ico` is provided as the fallback for browsers without SVG favicon support and declared with `rel="alternate icon"` so SVG-capable browsers prefer the vector.
- **FR-ICON-6** — `apple-touch-icon.png` is 180×180 with an **opaque** background. iOS does not honour transparency and composites it onto black, which would render a dark mark invisible.
- **FR-ICON-7** — A `site.webmanifest` declares `name`, `short_name`, `theme_color`, `background_color`, `display: "standalone"`, `start_url: "/"`, and the 192/512/512-maskable icons.
- **FR-ICON-8** — Two `<meta name="theme-color">` tags are declared with `media="(prefers-color-scheme: light)"` → `#ffffff` and `media="(prefers-color-scheme: dark)"` → `#020817`. These values are the rendered equivalents of the existing `--background` tokens (`0 0% 100%` and `222.2 84% 4.9%`).
- **FR-ICON-9** — A `BrandMark` React component renders the mark inline as SVG using `currentColor`, so it inherits the surrounding text colour and needs no separate dark variant.
- **FR-ICON-10** — `AppLayout`'s sidebar brand (`apps/web/src/components/AppLayout.tsx:18`) renders `<BrandMark />` alongside the "MyFleet" wordmark. The mark is decorative next to visible text and therefore carries `aria-hidden="true"`.
- **FR-ICON-11** — The `<title>` in `index.html` stays `MyFleet`; per-page titles are out of scope.
- **FR-ICON-12** — PNG rasters are **generated once and committed** to the repository. A helper script (`tools/generate-icons.sh`) documents and reproduces the generation from `favicon.svg`, but neither CI nor the Docker build may depend on it or on any image-processing binary being installed.

## 5. API Surface

Both endpoints live in `apps/auth-service/internal/user/resource.go`, inside the existing JWT-protected route group wired at `apps/auth-service/cmd/main.go:91-96`. The browser reaches them at `/api/auth/me` — the gateway strips the `/api/<service>` prefix.

### 5.1 `GET /auth/me` (modified)

Unchanged semantics; one new attribute.

**Response 200** — `application/vnd.api+json`

```json
{
  "data": {
    "type": "users",
    "id": "1f9c…",
    "attributes": {
      "email": "user@example.com",
      "displayName": "Jane Doe",
      "avatarUrl": "https://…",
      "themePreference": "system"
    }
  },
  "meta": { "activeFleetId": "8a2e…", "role": "owner" }
}
```

- `themePreference` is always present and always one of `light` | `dark` | `system`. A row holding an empty or unrecognised value is normalised to `system` on read (FR-DATA-4), so clients never receive an out-of-range value.
- Existing error behaviour is preserved verbatim: `404` when the user row is missing, bare `500` (via the `errInternal` sentinel) on lookup failure. The deliberate non-leaking of database internals in the current handler must not regress.

### 5.2 `PATCH /auth/me` (new)

Updates the calling user's own preferences. There is no user id in the path — the target is always the `sub` claim of the validated JWT. This makes horizontal privilege escalation structurally impossible rather than a check that could be forgotten.

**Request** — `application/vnd.api+json`

```json
{ "data": { "type": "users", "attributes": { "themePreference": "dark" } } }
```

Implemented with `server.RegisterInputHandler` over an attributes struct, matching the `PATCH /fleets/{id}` precedent at `apps/fleet-service/internal/fleet/resource.go:69`.

**Response 200** — the full user resource, identical in shape to `GET /auth/me`'s `data` (no `meta` block; active fleet and role are token-derived and unaffected by this call).

**Error cases**

| Condition | Status | Notes |
|---|---|---|
| No / invalid / expired JWT | `401` | Enforced by the existing `authmw.JWT` middleware on the route group; not re-implemented in the handler. |
| `themePreference` absent or empty | `400` | Explicit validation. The field is required; PATCH-as-partial-update is not supported for a single-field resource. |
| `themePreference` not in `light`\|`dark`\|`system` | `400` | Validated against an allow-list in the domain layer, not the transport layer. |
| Malformed JSON body | `400` | Handled by `server.RegisterInputHandler`. |
| User row not found for the token's `sub` | `404` | Same sentinel path as `GET /auth/me`. |
| Persistence failure | `500` | Must render the bare `errInternal` sentinel, **not** the underlying error — `server.WriteError` copies `err.Error()` into the response title and would otherwise publish database internals to any authenticated caller. This is the exact trap documented at `apps/auth-service/internal/user/resource.go:18`. |

Validation errors must name the offending field and the accepted values without echoing the caller's raw input back into the response.

## 6. Data Model

### 6.1 `auth.users` — new column

| Column | Type | Constraints | Notes |
|---|---|---|---|
| `theme_preference` | `varchar` | `NOT NULL`, `DEFAULT 'system'` | One of `light`, `dark`, `system`. |

- **FR-DATA-1** — Added as a field on `apps/auth-service/internal/user/entity.go`'s `Entity` with tag `gorm:"not null;default:'system'"`. The existing `Migration` function already calls `db.AutoMigrate(&Entity{})`, which issues `ADD COLUMN … DEFAULT 'system' NOT NULL`; pre-existing rows are backfilled by Postgres. No hand-written migration is needed.
- **FR-DATA-2** — The value is **not** a database enum. A `varchar` plus application-layer validation keeps the allow-list in one place and avoids a migration to add a future value.
- **FR-DATA-3** — `themePreference` is **not** added to the JWT claims. It changes more often than a token's lifetime and has no authorisation meaning; putting it in the token would make a theme change require a token refresh to take effect elsewhere.
- **FR-DATA-4** — Read-side normalisation: `user.Make` maps an empty or unrecognised stored value to `system`. This is defence-in-depth against a row written before the column existed or by an out-of-band edit.

### 6.2 Domain model

Following the immutable-model convention already established in `apps/auth-service/internal/user/model.go`:

- `Model` gains an unexported `themePreference` field and a `ThemePreference()` accessor.
- A `WithThemePreference(pref string) Model` method returns a copy, mirroring the existing `WithLogin` pattern. It does not validate; validation is the processor's job so that invalid input produces a typed domain error rather than a silently ignored mutation.
- A named type or exported constants for the three values (e.g. `ThemeLight`, `ThemeDark`, `ThemeSystem`) plus an `IsValidTheme(string) bool` predicate live in the `user` package and are the single source of the allow-list.
- `Transform` in `rest.go` adds `themePreference` to `Attributes` with JSON tag `themePreference`, matching the existing lowerCamelCase convention.
- The provider/administrator split is preserved: the administrator gains an update path for the column; the processor validates then delegates.

### 6.3 Frontend types

- `apps/web/src/types/models/user.ts` — `UserAttributes` gains `themePreference: ThemePreference`, where `export type ThemePreference = 'light' | 'dark' | 'system'`. The file's comment already pins it as mirroring `apps/auth-service/internal/user/rest.go`; keep that annotation accurate.
- A separate `ResolvedTheme = 'light' | 'dark'` type distinguishes the user's preference from the computed outcome. Collapsing the two is the most likely source of bugs here — `system` is a valid preference but never a valid resolved value.

## 7. Service Impact

### `apps/auth-service` (Go)

- `internal/user/entity.go` — new `ThemePreference` column, `Make` normalisation, `ToEntity` mapping.
- `internal/user/model.go` — field, accessor, `WithThemePreference`, theme constants, `IsValidTheme`.
- `internal/user/administrator.go` — persistence for the updated preference.
- `internal/user/processor.go` — `UpdateTheme(userID, pref string) (Model, error)`: validate → load → mutate → persist. Returns a typed validation error distinguishable from `ErrNotFound`.
- `internal/user/rest.go` — `themePreference` in `Attributes` and `Transform`.
- `internal/user/resource.go` — new `PATCH /auth/me` route on the existing protected group.
- No new dependencies, no new environment variables, no config changes, no Kubernetes manifest changes, no gateway routing changes (the path prefix `/auth` is already routed).

### `apps/web` (React/TS)

- `index.html` — inline pre-paint theme script, favicon/manifest/apple-touch links, dual `theme-color` metas.
- `public/` (new) — icon assets and `site.webmanifest` (see `icon-assets.md`).
- `src/index.css` — the four new semantic token families in both `:root` and `.dark`.
- `tailwind.config.ts` — the new colour families under `theme.extend.colors`.
- `src/context/ThemeContext.tsx` (new) — `ThemeProvider` + `useTheme()` exposing `{ preference, resolvedTheme, setPreference }`.
- `src/lib/theme.ts` (new) — pure helpers: `readCachedTheme()`, `writeCachedTheme()`, `resolveTheme(pref, prefersDark)`, `applyThemeClass(resolved)`, and the `myfleet.theme` storage-key constant. Keeping these pure and separately exported is what makes the provider testable without a DOM-heavy harness.
- `src/components/ThemeToggle.tsx` (new) — the header cycle button.
- `src/components/BrandMark.tsx` (new) — inline SVG mark.
- `src/components/AppLayout.tsx` — brand mark in the sidebar, toggle in the header, full token conversion.
- `src/components/providers/AppProviders.tsx` — `ThemeProvider` inserted above `AuthProvider` and `<Toaster />`; `theme` prop passed to `<Toaster />`.
- `src/lib/hooks/api/auth.ts` — `useUpdateTheme()` mutation issuing `PATCH /api/auth/me` through the existing `apiClient`, with an `authKeys.me()` cache update on success.
- `src/context/AuthContext.tsx` — on a successful `me` result, push the server's `themePreference` into the theme provider (FR-PERSIST-3).
- `src/test/setup.ts` — a `window.matchMedia` stub, following the existing in-memory `localStorage` polyfill pattern in that file. jsdom's `matchMedia` does not support `addEventListener`-driven change events, so the stub must expose `matches`, `addEventListener`, `removeEventListener`, and a way for tests to fire a change.
- The seven feature/page files listed in §4.6.

### `packages/ui-components`

- `src/StatusBadge.tsx` — token conversion (FR-CONVERT-9).

### `tools/`

- `generate-icons.sh` (new) — documented, reproducible rasterisation from `favicon.svg`. Not invoked by CI or Docker.

### Not affected

`fleet-service`, `media-service`, `notification-service`, `packages/shared-go`, `packages/dto-go`, `packages/shared-ts`, `deploy/k8s`.

## 8. Non-Functional Requirements

### Accessibility

- **FR-A11Y-1** — All text meets WCAG 2.1 AA contrast in **both** themes: 4.5:1 for body text, 3:1 for large text (≥18.66px bold or ≥24px) and for UI component boundaries. This explicitly covers every new status token pairing — `-subtle-foreground` on `-subtle` — and the bare status tokens on `--background` and on `--card`.
- **FR-A11Y-2** — Colour is never the only signal. The severity chips and status badges already carry text labels; conversion must not replace any label with colour alone.
- **FR-A11Y-3** — The toggle satisfies FR-TOGGLE-4 through FR-TOGGLE-6 (label, tooltip, keyboard reachability, visible focus).
- **FR-A11Y-4** — Theme switching triggers no layout shift: only colour values change, never spacing, sizing, or font metrics.

### Performance

- **FR-PERF-1** — The inline pre-paint script must be trivially small (target under 500 bytes minified) and must not block on anything beyond a `localStorage` read and a `matchMedia` call.
- **FR-PERF-2** — Theme switching performs no network request on the render path. The `PATCH` is fire-and-forget relative to the visual update (FR-PERSIST-4).
- **FR-PERF-3** — No measurable increase in bundle size beyond the new components; no new runtime dependency may be added for theming. `lucide-react`, `sonner`, and the existing context patterns cover everything required.
- **FR-PERF-4** — Committed icon assets stay under 100 KB in aggregate.

### Security

- **FR-SEC-1** — `PATCH /auth/me` derives its target user solely from the validated JWT `sub` claim. No user identifier is accepted from the request path, body, or query. A caller cannot modify another user's record.
- **FR-SEC-2** — `themePreference` is validated against a closed allow-list before it reaches persistence. It is never interpolated into a query, a class name, or any HTML/CSS sink.
- **FR-SEC-3** — Error responses must not leak database internals. The `errInternal` sentinel pattern at `apps/auth-service/internal/user/resource.go:18` is followed for the new handler; `server.WriteError(w, err)` with a raw persistence error is prohibited.
- **FR-SEC-4** — The value read from `localStorage` is attacker-controllable (any script or extension on the origin can write it). It must be validated against the allow-list before use, and the pre-paint script must only ever use it to toggle a fixed class name — never to build a class string, a URL, or injected markup.
- **FR-SEC-5** — The inline pre-paint script is the only inline script in the document. If a Content-Security-Policy is introduced later it will need a hash or nonce for this script; record this in the deployment notes so it is not discovered as a breakage.

### Observability

- **FR-OBS-1** — `PATCH /auth/me` failures log at error level with the `user_id` field, consistent with the existing `auth/me lookup failed` log. Validation rejections log at debug or not at all — they are client errors, not incidents.
- **FR-OBS-2** — No theme value is treated as sensitive, but log lines must not include the request body wholesale.

### Testing

- **FR-TEST-1** — Go: table-driven tests for `IsValidTheme` and `Processor.UpdateTheme` (each valid value, empty string, unknown value, unknown user).
- **FR-TEST-2** — Go: `httptest` coverage of `PATCH /auth/me` for 200 (value persisted and echoed), 400 (invalid and empty values), and 404 (unknown user), following the existing `apps/auth-service/internal/user/resource_test.go` harness.
- **FR-TEST-3** — Go: a regression test asserting `GET /auth/me` returns `themePreference` and that a row with an empty stored value surfaces as `system` (FR-DATA-4).
- **FR-TEST-4** — Frontend: unit tests for the pure helpers in `src/lib/theme.ts`, covering `system` resolution in both media states, invalid cached values falling back to `system`, and `localStorage` throwing.
- **FR-TEST-5** — Frontend: `ThemeProvider` tests for initial resolution from cache, the server value overriding the cache on `me` success, live media-query change propagation while on `system`, and media changes being ignored while on `light`/`dark`.
- **FR-TEST-6** — Frontend: `ThemeToggle` tests for the three-step cycle, the icon and `aria-label` at each step, and the mutation being called with the new value.
- **FR-TEST-7** — Frontend: a test asserting a failed `PATCH` leaves the local theme applied and surfaces a toast (FR-PERSIST-5).
- **FR-TEST-8** — A guard test asserting `index.html` still contains the pre-paint script, so a future edit cannot silently reintroduce the flash.
- **FR-TEST-9** — A guard test (or lint rule) enforcing FR-CONVERT-10's grep returning no matches, so new hardcoded palette classes cannot creep back in.
- **FR-TEST-10** — `make ci` passes: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`.

## 9. Open Questions

1. **Icon rasterisation toolchain.** FR-ICON-12 requires committed PNGs generated from the SVG, but no image-processing binary is currently a documented prerequisite of this repo. The design phase should confirm which tool is available in the dev environment (`rsvg-convert`, ImageMagick, or a one-off `sharp` invocation via `npx`) and record it in `generate-icons.sh`. This does not block implementation — it only affects how the committed assets are produced.
2. **Mark design.** §4.8 constrains the mark (single-colour, geometric, no lettering, legible at 16px) but does not fix a concrete shape. The design phase should settle on one. If the user has a preference, it should be captured before implementation.
3. **Pre-auth toggle.** FR-TOGGLE-7 scopes the control to the authenticated shell. If the login page should also expose a toggle, that is a small additive change but needs a decision.
4. **Existing-user default.** Every current user gets `system` via the column default (FR-DATA-1). If the intent were instead to preserve the light theme those users are accustomed to, the default would need to be `light`. `system` is specified on the grounds that a user whose OS is dark almost certainly wants a dark app, and the toggle makes it one click to change.
5. **CSP.** FR-SEC-5 notes the inline script will need a hash/nonce if a CSP is added later. No CSP exists today; confirm whether one is planned in the near term so the script can be structured accordingly.

## 10. Acceptance Criteria

### Theme behaviour

- [ ] With the OS in dark mode, a fresh browser profile loads MyFleet in dark mode, with no light flash at any point during load or hard refresh.
- [ ] With the OS in light mode, a fresh profile loads in light mode.
- [ ] The header toggle cycles Light → Dark → System → Light, and the icon changes to Sun / Moon / Monitor accordingly.
- [ ] With the preference on System, changing the OS appearance updates the app immediately without a reload.
- [ ] With the preference on Light, changing the OS appearance to dark leaves the app in light mode.
- [ ] Setting Dark, signing out, and signing in again from a **different browser** yields dark mode — proving server-side persistence, not `localStorage`.
- [ ] Setting Dark and hard-refreshing yields dark mode with no flash.
- [ ] With `localStorage` blocked by browser settings, the app still loads and the toggle still works for the session.
- [ ] A corrupted `localStorage['myfleet.theme']` value (e.g. `"purple"`) does not break loading; the app falls back to system behaviour.
- [ ] With the network offline, toggling the theme still changes the appearance immediately and shows the save-failure toast.

### Visual completeness

- [ ] In dark mode, every route renders legibly: Dashboard, Vehicles, Vehicle Detail, Maintenance, Fuel, Activity, Notifications, Settings, Login, Onboarding, Invite Accept.
- [ ] Sidebar, nav links (including the active state), header, and sign-out button are correct in both themes.
- [ ] Severity chips (Urgent / Recommended / Info) and status badges (Healthy / Upcoming Maintenance / Overdue / Inactive) are readable in both themes and remain visually distinguishable from one another.
- [ ] Dashboard widget headings and the Fleet Overview counters use status tokens and read correctly in both themes.
- [ ] The overdue-maintenance callout blocks in `MaintenanceQueueView` render with an appropriate dark fill rather than a near-white one.
- [ ] Toasts (success and error) render with the correct theme.
- [ ] Radix `select` dropdown content renders correctly in dark mode (portal check, FR-3P-3).
- [ ] Form inputs, focus rings, skeletons, and card borders are correct in both themes.
- [ ] `grep -rE '(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)' apps/web/src packages/ui-components/src --include='*.tsx'` returns no matches.
- [ ] Every new token pairing has a recorded contrast ratio meeting FR-A11Y-1, in both themes.

### API

- [ ] `GET /auth/me` includes `themePreference` for every user, including those created before this change.
- [ ] `PATCH /auth/me` with `{"themePreference":"dark"}` returns 200, echoes the updated resource, and the value survives a subsequent `GET`.
- [ ] `PATCH /auth/me` with an invalid value returns 400 and does not modify the stored value.
- [ ] `PATCH /auth/me` with an empty/absent `themePreference` returns 400.
- [ ] `PATCH /auth/me` without a bearer token returns 401.
- [ ] A forced persistence failure returns a bare 500 with no database detail in the response body.
- [ ] `auth-service` starts against an existing database and `AutoMigrate` adds the column with existing rows defaulted to `system`.

### Branding

- [ ] The browser tab shows the MyFleet mark, not a blank document icon, in Chrome, Firefox, and Safari.
- [ ] The favicon is legible against both light and dark browser chrome.
- [ ] "Add to Home Screen" on iOS shows the mark on an opaque background, not a black square.
- [ ] The web manifest validates and the app is installable with correct name and icons.
- [ ] The mark appears in the sidebar next to the "MyFleet" wordmark and adapts to the theme via `currentColor`.
- [ ] The browser UI theme colour matches the app background in both themes.
- [ ] Icon assets total under 100 KB.

### Build

- [ ] `make ci` passes.
- [ ] `docker build -f apps/web/Dockerfile .` succeeds and the resulting image serves `/favicon.svg`, `/favicon.ico`, `/apple-touch-icon.png`, and `/site.webmanifest` with 200 responses.
- [ ] `npm run format:check` passes (the new `.css` and `.tsx` files are inside the existing prettier globs).
