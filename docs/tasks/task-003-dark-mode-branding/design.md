# Dark Mode & Application Branding — Design

Version: v1
Status: Approved
Created: 2026-07-31
PRD: [`prd.md`](./prd.md) · Icon spec: [`icon-assets.md`](./icon-assets.md)

---

## 1. Scope of this document

The PRD fixes *what* is built. This document fixes *how*: the module boundaries on the
frontend, the data flow between auth state and theme state, the exact token values, the
Go persistence path, and the icon generation toolchain. It also records four places where
implementation must deviate from the PRD, each with the reason.

Everything here was verified against the working tree at `2004ce8`; no value below is
recalled from memory.

## 2. Decisions taken during design

The PRD's §9 open questions are resolved as follows.

| # | Question | Decision |
|---|---|---|
| 1 | Icon rasterisation toolchain | Neither `rsvg-convert`, ImageMagick, nor Inkscape is present in this environment. Python 3 with Pillow 12.2 is. A single Python generator computes the mark geometry and emits *every* artefact — SVG, PNGs, ICO, and the in-app path constant. See §8. |
| 2 | Mark design | Three right-pointing filled chevrons on a shared vertical centreline, tapering in scale — a fleet in motion. See §8.1. |
| 3 | Pre-auth toggle | Authenticated shell only, exactly as FR-TOGGLE-7 specifies. Pre-auth pages inherit the cached/system theme. |
| 4 | Existing-user default | `system`, as the PRD argues. No change. |
| 5 | CSP | No CSP exists today. §10 records the deployment note FR-SEC-5 asks for; nothing in this task is structured around a hypothetical future policy. |

## 3. Deviations from the PRD

These four are deliberate. Each is a case where the PRD's letter conflicts with what is
actually in the repository.

### 3.1 Validation returns 422, not 400

PRD §5.2 specifies `400` for an absent, empty, or out-of-range `themePreference`.
`packages/shared-go/server/errors.go` has no `400` sentinel — `StatusFor` maps
`ErrUnauthorized`→401, `ErrForbidden`→403, `ErrNotFound`→404, `ErrConflict`→409,
`ErrGone`→410, `ErrRequestEntityTooLarge`→413, `ErrValidation`→422, everything else→500.
Worse, `server.RegisterInputHandler` (`packages/shared-go/server/handler.go:42`) already
writes `ErrValidation` — a 422 — for a malformed body. Honouring the PRD literally would
mean adding an `ErrBadRequest` to shared-go and shipping a route where malformed JSON is
422 while an invalid enum value is 400.

**Decision:** all client-side validation failures on `PATCH /auth/me` return **422** via
`server.ErrValidation`. `shared-go` is untouched. The PRD's acceptance criteria that name
400 read as 422.

### 3.2 No new `Administrator` method

PRD §7 lists `internal/user/administrator.go` as changing to add "persistence for the
updated preference". The existing `Administrator.Update(Model) (Model, error)` already
does a full `db.Save(&e)` of the entity, and `ToEntity` will carry the new column. No new
method, no interface change. `administrator.go` is not modified at all.

### 3.3 The theme provider does not call the network

PRD §7 has `src/context/AuthContext.tsx` pushing the server's `themePreference` into the
theme provider on a successful `me`. Same data flow, different housing: the push lives in
a purpose-built `ThemeSync` component rather than inside `AuthContext`. See §4.2 — this
keeps `AuthContext` unaware that theming exists and `ThemeContext` unaware that auth
exists, and it is the difference between a theme provider that can be unit-tested with a
bare `render()` and one that needs a `QueryClientProvider` plus a token fixture.

### 3.4 The builder seeds the default rather than relying on the column default

FR-DATA-1 leans on the Postgres column default to fill `theme_preference` for new rows.
`ProvisionFromGoogle` inserts via `NewBuilder()...Build()`, which would leave the field as
`""`, and whether GORM omits a zero-valued column with a `default:` tag and reads the value
back via `RETURNING` is version- and dialect-dependent. `NewBuilder()` therefore seeds
`themePreference: ThemeSystem` explicitly. The column default stays (it is what backfills
*existing* rows during `AutoMigrate`), but no code path depends on it at insert time.

## 4. Frontend architecture

### 4.1 Module boundaries

Five new units, each with one job. The boundaries are chosen so that the only piece that
needs a heavy test harness is the 20-line bridge.

| Unit | Responsibility | Depends on |
|---|---|---|
| `src/lib/theme.ts` | Pure functions and the storage-key constant. No React, no network. The only DOM it touches is the one `classList` call in `applyThemeClass`. | nothing |
| `src/context/ThemeContext.tsx` | Client theme state: current preference, resolved theme, the `matchMedia` subscription, and the `dark` class on `<html>`. **No network, no auth.** | `lib/theme` |
| `src/lib/hooks/api/auth.ts` → `useUpdateTheme()` | The `PATCH` and its React Query cache write. Nothing else. | `apiClient`, `authKeys` |
| `src/components/ThemeToggle.tsx` | The header control. Composes state + mutation, owns the failure toast. | `ThemeContext`, `useUpdateTheme`, `sonner` |
| `src/components/ThemeSync.tsx` | One-way bridge: server preference → theme state. Renders `null`. | `AuthContext`, `ThemeContext` |

`src/lib/theme.ts` exports:

```ts
export const THEME_STORAGE_KEY = 'myfleet.theme';
export type ResolvedTheme = 'light' | 'dark';

export function isThemePreference(v: unknown): v is ThemePreference;
export function readCachedTheme(): ThemePreference | null;   // null on absent/invalid/throw
export function writeCachedTheme(p: ThemePreference): void;   // swallows throws
export function resolveTheme(p: ThemePreference, prefersDark: boolean): ResolvedTheme;
export function applyThemeClass(r: ResolvedTheme): void;
```

`resolveTheme` takes `prefersDark` as a parameter rather than reading `matchMedia` itself.
That is what makes FR-TEST-4's "system resolution in both media states" a two-line test
instead of a jsdom mocking exercise.

`ThemePreference` (`'light' | 'dark' | 'system'`) lives in
`src/types/models/user.ts` because it is part of the API contract and that file already
declares itself a mirror of `apps/auth-service/internal/user/rest.go`. `ResolvedTheme`
lives in `lib/theme.ts` because it never crosses the wire. Keeping them in separate files
is the cheapest available defence against the collapse the PRD §6.3 warns about.

### 4.2 Provider tree and the auth→theme data flow

`ThemeProvider` must sit above `<Toaster />` (FR-3P-2) and above `AppLayout`, but the
authoritative preference arrives from `useMe()`, which lives *below* it. The bridge
resolves this without either context importing the other:

```
QueryClientProvider                    (owns the query cache)
└── ThemeProvider                      (client theme state; no network)
    ├── AuthProvider                   (owns useMe)
    │   ├── ThemeSync                  (useAuth() → useTheme().adoptServerPreference)
    │   └── children                   (router, AppLayout, ThemeToggle)
    └── ThemedToaster                  (useTheme() → <Toaster theme={resolvedTheme} />)
```

`ThemedToaster` is a three-line local component inside `AppProviders.tsx`; `<Toaster>`
cannot read the context from where `AppProviders` itself renders it, because that call site
is outside `ThemeProvider`'s subtree.

`ThemeContext` exposes two distinct write paths, and the distinction is load-bearing:

- `setPreference(p)` — **user intent.** Sets state, writes `localStorage`, flips the class.
- `adoptServerPreference(p)` — **server truth.** Same three effects, but semantically an
  echo of what the server already holds, so no `PATCH` follows.

Neither issues a request. `ThemeToggle` is the only place where a state change and a
mutation are fired together, which means there is exactly one file to read to understand
when the network is touched.

### 4.3 Optimistic update and the failure path

FR-PERSIST-4 (optimistic), FR-PERSIST-5 (no rollback on failure), and FR-PERSIST-6 (cache
update) interact in a way worth stating explicitly, because the textbook React Query
optimistic pattern would violate FR-PERSIST-5.

`useUpdateTheme` writes the new value into `authKeys.me()` in `onMutate` and — deliberately
— **does not roll the cache back in `onError`.** Rolling back would make the next
`ThemeSync` pass re-adopt the old value and flip the theme out from under the user, which
is precisely what FR-PERSIST-5 forbids. The cache is knowingly optimistic-but-wrong until
a genuine refetch; the toast tells the user exactly that ("It'll reset next time you sign
in").

That leaves one residual hazard: a background refetch during the session would return the
old server value and `ThemeSync` would adopt it. `ThemeContext` therefore keeps a
`hasLocalOverride` ref, set by `setPreference` and never by `adoptServerPreference`. Once
the user has made a choice this session, `adoptServerPreference` is a no-op. The ref starts
`false`, so the first `me` result of a session always wins — which is exactly FR-PERSIST-3,
and exactly what makes the preference land on a newly signed-in device.

`logout()` already calls `queryClient.removeQueries({ queryKey: authKeys.all })`;
`ThemeSync` clears `hasLocalOverride` when `user` transitions to `null`, so the next
sign-in adopts fresh. Per FR-PERSIST-7 the `localStorage` cache is *not* cleared, so the
login page keeps the theme the device was last using.

`useUpdateTheme` guards on `getAccessToken()` and resolves without a request when absent
(FR-PERSIST-8). Since FR-TOGGLE-7 confines the toggle to `AppLayout`, this guard is
defence-in-depth rather than a live path.

### 4.4 Pre-paint script

A single inline `<script>` in `<head>`, before the module script, no `defer`/`async`:

```html
<script>
  try {
    var p = localStorage.getItem('myfleet.theme');
    if (p !== 'light' && p !== 'dark') p = 'system';
    var d = p === 'dark' || (p === 'system' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches);
    if (d) document.documentElement.classList.add('dark');
  } catch (e) {}
</script>
```

Well under the 500-byte budget (FR-PERF-1). Three properties matter:

- The allow-list is applied by *narrowing to a known-good default*, not by rejecting bad
  input — a corrupted `"purple"` and an absent key take the identical `system` path
  (FR-FLASH-2, FR-SEC-4).
- The class name is a literal. The storage value is never concatenated into a class,
  URL, or markup (FR-SEC-4).
- The whole body is in `try/catch`, so `localStorage` blocked by privacy settings leaves
  the document in the light theme rather than failing to boot (FR-FLASH-3).

This duplicates `resolveTheme`'s logic and cannot be shared — a module import in `<head>`
is asynchronous by definition and would reintroduce the flash. FR-TEST-8's guard test is
the mitigation: it asserts `index.html` still contains the script, so a future cleanup
cannot silently delete it.

### 4.5 Third-party surfaces

`sonner` renders into its own portal **and** computes its own colours from a `theme` prop,
so it needs the explicit `theme={resolvedTheme}` wiring (FR-3P-1).

Radix (`select`, `switch`, `label`) needs nothing. Its portals mount into `document.body`,
which is a descendant of `document.documentElement` — where the `dark` class lives — so
the token cascade reaches them by construction. This is a structural argument, not an
assumption, but FR-3P-3 still requires visual confirmation of `select`'s floating content
panel, which is the one Radix surface that renders detached from its trigger.

## 5. Semantic status tokens

### 5.1 Values

Four families, `H S% L%` triplets without the `hsl()` wrapper, matching the existing
convention in `apps/web/src/index.css`.

**`:root` (light)** — chosen so the bare token clears 4.5:1 on `--background`/`--card`
(both `0 0% 100%`), and each `-subtle-foreground` clears 4.5:1 on its `-subtle` fill:

| Token | success | warning | danger | info |
|---|---|---|---|---|
| *(bare)* | `142.4 71.8% 29.2%` | `26 90.5% 37.1%` | `0 72.2% 41.1%` | `224.3 76.3% 48%` |
| `-subtle` | `140.6 84.2% 92.5%` | `48 96.5% 88.8%` | `0 93.3% 94.1%` | `214.3 94.6% 92.7%` |
| `-subtle-foreground` | `142.8 64.2% 24.1%` | `22.7 82.5% 31.4%` | `0 70% 35.3%` | `226 70.7% 40.2%` |
| `-border` | `141 78.9% 85.1%` | `48 96.6% 76.7%` | `0 96.3% 89.4%` | `213.3 96.9% 87.3%` |

**`.dark`** — not inversions. The fills are low-lightness/moderate-saturation and the
foregrounds high-lightness, mirroring the light `-100`/`-800` relationship rather than
negating it (FR-TOKEN-5). Bare tokens move to the 400-band because a 700-band colour on a
`222.2 84% 4.9%` background fails badly:

| Token | success | warning | danger | info |
|---|---|---|---|---|
| *(bare)* | `141.7 76.6% 73.1%` | `43.3 96.4% 56.3%` | `0 90.6% 70.8%` | `213.1 93.9% 67.8%` |
| `-subtle` | `142 40% 14%` | `30 45% 14%` | `0 45% 15%` | `217 45% 17%` |
| `-subtle-foreground` | `142 70% 78%` | `43 90% 76%` | `0 90% 80%` | `213 92% 80%` |
| `-border` | `142 35% 24%` | `32 40% 25%` | `0 40% 26%` | `217 40% 28%` |

Every pairing above was selected against a computed contrast estimate, all comfortably
above 4.5:1 (the tightest is light `--warning` on white at roughly 4.8:1). **These are
design targets, not measurements.** FR-A11Y-1 requires recorded ratios, so implementation
must compute the actual ratio for each of the sixteen pairings — bare-on-`--background`,
bare-on-`--card`, and `-subtle-foreground`-on-`-subtle`, in both themes — and record them
in the task folder. If any pairing misses, adjust lightness only; hue and saturation are
what keep the four families distinguishable from one another (FR-A11Y-2's companion
concern).

`--destructive` is untouched (FR-TOKEN-4). `danger` and `destructive` coexist: the former
indicates status, the latter styles destructive controls.

### 5.2 Tailwind registration

Added to `theme.extend.colors` in the existing `DEFAULT` + variant shape:

```ts
success: {
  DEFAULT: 'hsl(var(--success))',
  subtle: 'hsl(var(--success-subtle))',
  'subtle-foreground': 'hsl(var(--success-subtle-foreground))',
  border: 'hsl(var(--success-border))',
},
```

yielding `text-success`, `bg-success-subtle`, `text-success-subtle-foreground`, and
`border-success-border`. `packages/ui-components/src` is already in the `content` globs
(`tailwind.config.ts:9`), so `StatusBadge` picks the classes up with no config change.

## 6. Conversion of hardcoded colours

The PRD's §4.6 list was re-verified with the FR-CONVERT-10 grep against the working tree.
It is exhaustive and correct — 21 matches across the 9 files named, and nothing else. The
`ui/` primitives are already fully tokenised.

Two conversions change appearance slightly and should not be flagged as regressions:

- `FleetOverviewWidget`'s counters move from the `-600` band to `--success`/`--warning`/
  `--danger`, which sit at the `-700` band. Slightly darker in light mode; the reason is
  that `text-amber-600` on white is about 3.1:1 and fails FR-A11Y-1 for body-weight text.
- `MaintenanceQueueView`'s callout moves from `bg-red-50` to `bg-danger-subtle`, which is
  the `-100` band. A marginally stronger fill in light mode, and the only value that gives
  the dark theme somewhere sane to land (FR-CONVERT-7 / the "near-white callout"
  acceptance criterion).

`SeverityChip`'s comment asserting these colours "have no shadcn semantic equivalent" is
now false and must be rewritten to point at the new families (FR-CONVERT-3).

`AppLayout`'s sign-out button maps cleanly onto the existing primitive:
`<Button variant="outline" size="sm">`. That variant is already
`border border-input bg-background hover:bg-accent hover:text-accent-foreground` and
carries `focus-visible:ring-ring`. The theme toggle uses `variant="ghost" size="icon"`,
which satisfies FR-TOGGLE-6's visible focus ring without any new CSS.

## 7. Backend design

### 7.1 Files and shape

New file `apps/auth-service/internal/user/theme.go`:

```go
const (
    ThemeLight  = "light"
    ThemeDark   = "dark"
    ThemeSystem = "system"
)

func IsValidTheme(s string) bool
```

Plain `string` constants, not a named type. A named type would have to thread through
`Attributes`, `Entity`, and every test fixture for no benefit the allow-list does not
already provide, and no other enum in this codebase is modelled as a named type.

| File | Change |
|---|---|
| `theme.go` (new) | Constants + `IsValidTheme`. The single source of the allow-list. |
| `model.go` | `themePreference` field, `ThemePreference()` accessor, `WithThemePreference(string) Model` (copy-returning, **no validation** — validation is the processor's job, per PRD §6.2). `ErrInvalidTheme` sentinel. |
| `entity.go` | `ThemePreference string \`gorm:"not null;default:'system'"\``; `Make` normalises empty/unknown → `ThemeSystem` (FR-DATA-4); `ToEntity` maps it. |
| `builder.go` | `NewBuilder()` seeds `ThemeSystem`; `SetThemePreference` added for symmetry. See §3.4. |
| `processor.go` | `UpdateTheme(userID, pref string) (Model, error)`. |
| `rest.go` | `ThemePreference string \`json:"themePreference"\`` in `Attributes`; wired in `Transform`. |
| `resource.go` | `PATCH /auth/me`. |
| `administrator.go` | **Unchanged** — see §3.2. |

### 7.2 `Processor.UpdateTheme`

```
validate pref against IsValidTheme  → ErrInvalidTheme
provider.GetByID(userID)            → ErrNotFound propagates
model.WithThemePreference(pref)
administrator.Update(model)
```

Validation happens first, before the read, so an invalid value costs no database round
trip and — more importantly — cannot leave a partially-applied state. The three error
outcomes are distinguishable at the call site: `ErrInvalidTheme`, `ErrNotFound`, and
anything else.

### 7.3 `PATCH /auth/me`

Registered on the same protected group as `GET /auth/me`, inside the existing
`InitializeRoutes` closure so both share the one `Processor`:

```go
pr.Patch("/auth/me", server.RegisterInputHandler(func(w http.ResponseWriter, req *http.Request, attrs struct {
    ThemePreference string `json:"themePreference"`
}) {
    id := auth.IdentityFromContext(req.Context())
    m, err := proc.UpdateTheme(id.UserID, attrs.ThemePreference)
    ...
}))
```

The target user is `id.UserID` — the validated JWT `sub`. There is no path parameter, no
body field, and no query parameter carrying a user identifier, so horizontal privilege
escalation is not a check that could be forgotten but a shape that cannot express the
attack (FR-SEC-1).

Error mapping:

| Condition | Rendered as | Status |
|---|---|---|
| Missing/invalid JWT | `authmw.JWT` middleware, not the handler | 401 |
| Malformed body | `server.ErrValidation` from `RegisterInputHandler` | 422 |
| Empty or invalid `themePreference` | `errInvalidTheme` (below) | 422 |
| User not found | `server.ErrNotFound` | 404 |
| Persistence failure | `errInternal` — the existing sentinel at `resource.go:18` | 500 |

`WriteError` copies `err.Error()` into the response `title`, which is the trap already
documented at `resource.go:18`. Two consequences:

- The persistence-failure branch renders `errInternal`, never the underlying error
  (FR-SEC-3), and logs at error level with `user_id` (FR-OBS-1) — mirroring the existing
  `auth/me lookup failed` line.
- To satisfy §5.2's "name the offending field and the accepted values", the validation
  branch renders a package-level wrapped sentinel rather than the bare `ErrValidation`:

  ```go
  var errInvalidTheme = fmt.Errorf("%w: themePreference must be one of light, dark, system",
      server.ErrValidation)
  ```

  `errors.Is` still finds `ErrValidation`, so `StatusFor` returns 422, and the title names
  the field and the allow-list. Because the message is a compile-time constant, the
  caller's raw input can never be echoed back.

Validation rejections are not logged (FR-OBS-1); they are client errors, not incidents.

### 7.4 Migration

`Migration` already calls `db.AutoMigrate(&Entity{})`, which issues
`ADD COLUMN theme_preference varchar NOT NULL DEFAULT 'system'`. Postgres backfills
existing rows. No hand-written migration, no new `deploy/k8s` change, no new environment
variable, no gateway change — `/auth` is already routed and the JWT group already exists.

`themePreference` stays out of the JWT (FR-DATA-3): it changes more often than a token
lives and carries no authorisation meaning.

## 8. Branding

### 8.1 The mark

Three right-pointing filled chevrons on a shared vertical centreline, tapering in scale —
readable as motion, and as more than one thing moving.

Specified parametrically rather than as literal path data, because the generator computes
the geometry (§8.2) and a hand-transcribed `d` string in this document would be a second
source of truth that could drift:

- `viewBox="0 0 24 24"`, centreline `y = 12`.
- Three chevrons, apexes advancing left-to-right, scales `1.0 / 0.82 / 0.64`.
- Filled polygons with uniform horizontal thickness, not strokes. Fills survive
  downscaling and ICO rasterisation predictably; a stroke's effective weight depends on
  how the rasteriser resolves `stroke-width` against a scaled viewBox.
- The whole cluster fits inside the inner 80% of the canvas, so the maskable icon needs no
  separate geometry — only a different canvas margin (FR-ICON-2 / the maskable safe zone).
- No lettering, no gradients, one colour (FR-ICON-1).

`favicon.svg` carries the `prefers-color-scheme` block FR-ICON-4 requires: fill `#020817`
against light browser chrome, `#f8fafc` against dark. The PNG rasters are the mark in
`#020817` on an **opaque `#ffffff`** background — required for `apple-touch-icon.png`
(FR-ICON-6, iOS composites onto black) and applied uniformly to the manifest icons so all
four rasters come off one code path.

### 8.2 Generation: one generator, four outputs

The environment has Python 3 and Pillow 12.2 and **none** of `rsvg-convert`, `magick`,
`convert`, or `inkscape`. Rather than depend on a binary that is not there, or on `npx`
reaching the network mid-build, `tools/generate-icons.py` computes the chevron polygons
once and emits everything from that one geometry:

| Output | How |
|---|---|
| `apps/web/public/favicon.svg` | Path `d` string serialised from the computed polygons, wrapped with the `prefers-color-scheme` style block |
| `apps/web/public/apple-touch-icon.png` (180) | `ImageDraw.polygon` at 4× supersample, LANCZOS downsample |
| `apps/web/public/icon-192.png`, `icon-512.png` | same |
| `apps/web/public/icon-512-maskable.png` | same, with the safe-zone margin |
| `apps/web/public/favicon.ico` | Pillow's native multi-size ICO save, 16 + 32 |
| `apps/web/src/components/brandMarkPath.ts` | The same `d` string as an exported constant, with a "generated — do not edit" header |

`tools/generate-icons.sh` stays as the documented entry point FR-ICON-12 asks for. It
prefers a true SVG rasteriser when one is installed (`rsvg-convert`, then
`magick`/`convert`) and otherwise runs the Python generator, failing with a message naming
all three if neither Python/Pillow nor a rasteriser is available. Neither CI nor
`apps/web/Dockerfile` invokes it; all outputs are committed.

This is a controlled deviation from icon-assets.md's "the SVG is the sole source for every
raster". The single source is preserved — it is simply the generator's geometry table
rather than the SVG file, which is itself one of the generated artefacts.

`BrandMark.tsx` imports the generated constant and renders it with `fill="currentColor"`,
so it inherits surrounding text colour and needs no dark variant (FR-ICON-9). It takes a
`className` for sizing and carries `aria-hidden="true"` in `AppLayout`, where the visible
"MyFleet" wordmark already provides the accessible name (FR-ICON-10).

One guard test asserts the constant in `brandMarkPath.ts` appears verbatim in
`public/favicon.svg` — cheap insurance against someone hand-editing one and not the other.

### 8.3 Delivery

No build change. Vite copies `apps/web/public/**` into `dist/`; the Dockerfile already
does `COPY apps/web ./apps/web` and `COPY --from=build /app/apps/web/dist`; nginx serves
`root /usr/share/nginx/html` with `try_files $uri $uri/ /index.html`, so each asset
resolves as a real file before the SPA fallback (FR-ICON-2).

Flat two-colour geometry keeps the aggregate well under the 100 KB budget (FR-PERF-4);
the 512 raster is the only one large enough to matter and compresses to single-digit KB.

## 9. Testing

### Go

- `IsValidTheme` — table-driven over the three valid values, `""`, and an unknown value.
- `Processor.UpdateTheme` — each valid value persists; `""` and an unknown value return
  `ErrInvalidTheme` **and leave the stored value unchanged**; an unknown user returns
  `ErrNotFound`.
- `PATCH /auth/me` via the existing `serveMe`-style `httptest` harness in
  `resource_test.go` (real chi router, real database, `auth.WithIdentity` context):
  200 with the value echoed and surviving a subsequent `GET`; 422 for empty and invalid;
  404 for an unknown user. The harness is generalised to take a method and body rather
  than being duplicated.
- Regression: `GET /auth/me` includes `themePreference`, and a row seeded with `''`
  surfaces as `system` (FR-DATA-4).

### Frontend

`src/test/setup.ts` gains a `window.matchMedia` stub alongside the existing `MemoryStorage`
polyfill. jsdom's implementation does not support `addEventListener`-driven change events,
so the stub exposes `matches`, `addEventListener`, `removeEventListener`, and a test-only
helper to flip `matches` and fire `change`. Without that helper, FR-TEST-5's live-update
case is untestable.

- `lib/theme.ts` — pure-function tests: `system` in both media states, invalid and absent
  cached values, `localStorage` throwing on read and on write.
- `ThemeProvider` — initial resolution from cache; server value overriding the cache on
  the first `me`; live media change while on `system`; media change ignored while on
  `light`/`dark`; listener removed on unmount.
- `ThemeToggle` — the three-step cycle, the icon and `aria-label` at each step, the
  mutation called with the new value.
- Failure path — a rejected `PATCH` leaves the theme applied and surfaces the toast
  (FR-PERSIST-5).

### Guard tests

`src/test/conventions.test.ts`, reading files from disk with `fs`:

- `index.html` still contains the pre-paint script (FR-TEST-8).
- The FR-CONVERT-10 grep returns nothing across `apps/web/src/**` and
  `packages/ui-components/src/**`. The test must exclude its own file, which necessarily
  contains the pattern.
- `brandMarkPath.ts`'s constant appears in `public/favicon.svg` (§8.2).

`make ci` — `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests` —
is the gate. No manifest changes are expected, but `manifests` runs regardless.

## 10. Deployment notes

- **CSP.** No Content-Security-Policy exists today. When one is introduced, the pre-paint
  script in `apps/web/index.html` is the only inline script in the document and will need
  a hash or a nonce. Removing it to satisfy a policy would reintroduce the theme flash;
  the correct fix is the hash (FR-SEC-5).
- **`theme-color` coupling.** The two `<meta name="theme-color">` values (`#ffffff`,
  `#020817`) are hand-maintained renderings of the `--background` tokens. If those tokens
  change, the metas must change with them. Nothing enforces this.
- No new dependency, no new environment variable, no Kubernetes manifest change, no
  gateway routing change.

## 11. Risks

| Risk | Mitigation |
|---|---|
| Contrast values in §5.1 are design estimates, not measurements | FR-A11Y-1 requires the sixteen ratios to be computed and recorded during implementation; adjust lightness only if a pairing misses |
| Pre-paint script drifts from `resolveTheme` | Guard test pins its presence; the logic is four lines and both sides are covered by tests |
| `hasLocalOverride` masks a legitimate server-side change made on another device mid-session | Accepted. Cross-device live sync is not a requirement, and the alternative violates FR-PERSIST-5 |
| Generator geometry drifts from `brandMarkPath.ts` after a hand-edit | Guard test compares the constant against `favicon.svg` |
| Radix `select`'s portal content misbehaves in dark mode | Structurally it cannot (portals mount under `<html>`), but FR-3P-3 mandates visual confirmation rather than accepting the argument |
