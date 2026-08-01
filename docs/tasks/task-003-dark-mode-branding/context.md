# Dark Mode & Application Branding — Implementation Context

Companion to [`plan.md`](./plan.md). The plan says *what to do*; this says *what
you need to know about this repository before doing it*. Everything below was
verified against the working tree at `1088d95`, not recalled.

---

## 1. Where you are

- Worktree: `/home/tumidanski/source/MyFleet/.worktrees/task-003-dark-mode-branding`
- Branch: `task-003-dark-mode-branding`
- Never edit the main checkout at `/home/tumidanski/source/MyFleet` — this task
  has a worktree, so all work belongs here.
- The git stash stack is shared with every other worktree and other sessions may
  push or pop concurrently. Use a temporary WIP commit instead of `git stash`.

## 2. Build and verification

```sh
make ci        # lint-check, vet, test, build, fe-test, fe-build, manifests
```

Individually: `make vet`, `make test`, `make build` (Go); `make fe-test`,
`make fe-build` (web); `make lint-check` (the check-only lint CI runs);
`make lint` fixes what it can.

**Node is not always on `PATH`.** If `npm` is missing:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

`make ci` includes `manifests`, which renders the `deploy/k8s` overlays. **This
task changes no manifests**, so that step should be a no-op — if it fails,
something unrelated is broken and it is not yours to fix.

Container build: `docker build -f apps/web/Dockerfile .` — the build context is
the repo root for every service, `apps/web` included.

## 3. Key files, and what already exists

### Backend — `apps/auth-service/internal/user/`

The package follows the repo's domain layering: immutable `Model`, `Builder`,
`Entity` (GORM), `Provider` (reads), `Administrator` (writes), `Processor`
(orchestration), `rest.go` (transport DTO), `resource.go` (routes).

| File | State today |
|---|---|
| `model.go` | 6 unexported fields, accessors, `WithLogin`. `ErrNotFound` sentinel. |
| `entity.go` | 8 columns, `Make`, `ToEntity`, `Migration` (calls `AutoMigrate`). |
| `builder.go` | Five setters + `Build`. `NewBuilder` seeds only the UUID. |
| `provider.go` | `GetByID` (primary key) and `GetBySub` (google_sub) — heavily commented, do not conflate. |
| `administrator.go` | `Insert` / `Update`, both full-entity `db.Create` / `db.Save`. |
| `processor.go` | `GetByID`, `ProvisionFromGoogle`. |
| `rest.go` | `Attributes` (3 fields), `Transform`, `TransformSlice`. |
| `resource.go` | `GET /auth/me` only. `errInternal` sentinel at line 18. |

### Frontend — `apps/web/`

| File | State today |
|---|---|
| `index.html` | 12 lines. No favicon, no manifest, no inline script. |
| `src/index.css` | `:root` + `.dark` token blocks, complete but never activated. |
| `tailwind.config.ts` | `darkMode: ['class']` already set; `content` already includes `packages/ui-components/src`. |
| `src/components/providers/AppProviders.tsx` | `QueryClientProvider` → `AuthProvider` + `<Toaster />`. |
| `src/context/AuthContext.tsx` | `useMe`, token capture, `logout` clearing `authKeys.all`. |
| `src/lib/hooks/api/auth.ts` | `authKeys`, `MeResult`, `fetchMe`, `useMe`, `logoutRequest`. Query only, no mutation. |
| `src/types/models/user.ts` | `UserAttributes` (3 fields), annotated as mirroring `rest.go`. |
| `src/test/setup.ts` | `MemoryStorage` polyfill only. No `matchMedia` stub. |
| `src/components/AppLayout.tsx` | Bare text brand, hand-rolled sign-out button, six hardcoded-colour sites. |
| `apps/web/public/` | **Does not exist.** |

## 4. Decisions already made (do not relitigate)

These come from the PRD and design and are settled:

| Question | Answer |
|---|---|
| Rasterisation toolchain | Python 3 + Pillow. Verified present at 12.2.0; `rsvg-convert`, `magick`, `convert`, `inkscape` are all **absent**. |
| Mark design | Three right-pointing filled chevrons on a shared centreline, tapering 1.0 / 0.82 / 0.64. |
| Pre-auth toggle | None. Authenticated shell only (FR-TOGGLE-7). |
| Existing-user default | `system`. |
| CSP | None exists. Recorded as a deployment note; nothing is structured around a hypothetical policy. |
| Validation status | **422, not 400** (see §5). |
| `administrator.go` | Unchanged. |
| Theme provider and the network | The provider never calls the network; `ThemeSync` bridges. |
| New-row default | Seeded by `NewBuilder`, not by the column default. |

## 5. Deviations from the PRD, and why

Four are from design §3; two more were settled while writing the plan. All six
are deliberate.

1. **Validation returns 422, not the PRD's 400.** `packages/shared-go/server/errors.go`
   has no 400 sentinel — `StatusFor` maps `ErrValidation`→422 and everything
   unrecognised→500. Worse, `server.RegisterInputHandler`
   (`packages/shared-go/server/handler.go:50`) already writes `ErrValidation` for
   a malformed body. Honouring the PRD literally would mean adding an
   `ErrBadRequest` to `shared-go` and shipping a route where malformed JSON is
   422 while an invalid enum value is 400. `shared-go` stays untouched.

2. **No new `Administrator` method.** `Update(Model) (Model, error)` already does
   a full `db.Save(&e)`, and `ToEntity` carries the new column. PRD §7 lists
   `administrator.go` as changing; it does not.

3. **The theme provider does not call the network.** PRD §7 has `AuthContext`
   pushing the server value into the provider. Same data flow, different
   housing: `ThemeSync`. This keeps `AuthContext` unaware theming exists and
   `ThemeContext` unaware auth exists, and it is the difference between a
   provider unit-testable with a bare `render()` and one needing a
   `QueryClientProvider` plus a token fixture.

4. **The builder seeds the default rather than relying on the column default.**
   FR-DATA-1 leans on the Postgres default to fill new rows.
   `ProvisionFromGoogle` inserts via `NewBuilder()…Build()`, which would leave
   the field `""`, and whether GORM omits a zero-valued column carrying a
   `default:` tag — then reads it back via `RETURNING` — is version- and
   dialect-dependent. The column default stays (it is what backfills *existing*
   rows during `AutoMigrate`), but no insert path depends on it.

5. **`generate-icons.sh` prefers Python over an SVG rasteriser** — inverting
   design §8.2's stated order. Only the Python path can regenerate `favicon.svg`
   and `brandMarkPath.ts`, because the geometry table, not the SVG, is the single
   source. Preferring a rasteriser would mean that on a machine with ImageMagick
   installed, a geometry change silently re-rasters a *stale* SVG while leaving
   the SVG and the path constant stale too. The rasteriser remains a degraded
   fallback that warns about what it cannot produce.

6. **The sidebar uses `bg-card`, not `bg-muted`.** FR-CONVERT-1 offers either.
   `--muted` and `--accent` are *identical values* in both themes
   (`210 40% 96.1%` light, `217.2 32.6% 17.5%` dark), so a `bg-muted` sidebar
   with `bg-accent` active links and `hover:bg-accent` hovers would flatten the
   whole nav into one colour. This is recorded because it looks like an
   arbitrary choice and is not.

## 6. Traps this codebase has already set

**The test DDL is hand-maintained.** `apps/auth-service/internal/user/provider_test.go`'s
`newTestDB` builds `auth.users` with explicit `CREATE TABLE` marked *"KEEP IN
SYNC WITH entity.go"*. It cannot call `AutoMigrate`: the `uniqueIndex` tags on
`GoogleSub`/`Email` make GORM emit an unqualified `CREATE UNIQUE INDEX … ON
users`, which cannot resolve against the attached in-memory `auth` schema and
fails with *"no such table: main.users"*. Adding a column to `Entity` without
adding it there breaks every `provider_test.go` and `resource_test.go` test.

**`server.WriteError` copies `err.Error()` into the response title.** This is
documented at `resource.go:18` and is why `errInternal` exists. Passing a raw
persistence error would publish database internals to any authenticated caller
(FR-SEC-3). It is also why the validation sentinel is a package-level
`fmt.Errorf` with a compile-time-constant message — the caller's raw input can
never reach the response.

**`sub` is overloaded.** The JWT `sub` claim carries our *internal user id*;
Google's `sub` is a different value that only the OAuth callback holds. Calling
`GetBySub(id.UserID)` compiles, silently returns `ErrNotFound`, and 404s every
authenticated request — which the SPA reads as logged-out, producing an
unbreakable login loop. `PATCH /auth/me` must use `GetByID` via
`proc.UpdateTheme`. `provider.go` and `provider_test.go` carry long comments
about this; they are worth reading before touching either lookup.

**jsdom's `matchMedia` does not fire `change`.** Without the stub the plan adds
to `src/test/setup.ts`, FR-TEST-5's live-update case cannot be written at all.

**`Storage.prototype` may not be the polyfill's prototype.** `src/test/setup.ts`
installs a plain `MemoryStorage` class instance, not a real `Storage`. If
`vi.spyOn(Storage.prototype, 'getItem')` does not intercept, use
`vi.spyOn(Object.getPrototypeOf(localStorage) as Storage, 'getItem')`.

**`fe-test` does not run `packages/ui-components`.** The Makefile runs
`apps/web` and `packages/shared-ts` only, even though `ui-components` has a test
script. `StatusBadge`'s conversion is covered by the `conventions.test.ts` guard
in `apps/web`, which walks into `packages/ui-components/src` from disk. This is
pre-existing and out of scope to fix.

**Prettier's globs are narrow.** `apps/web/src/**/*.{ts,tsx,css}`,
`apps/web/*.{ts,js}`, `packages/*/src/**/*.{ts,tsx}`. Not covered:
`index.html`, `public/site.webmanifest`, `tools/*.py`, `tools/*.sh`. But
`src/components/brandMarkPath.ts` **is** covered even though it is generated —
the generator must emit Prettier-clean output, or `npm run format` must run
after generating (and the path string must survive unchanged).

**ESLint lints `apps/web/src` with `--max-warnings 0`.** Test files get Node
globals via a config override on `**/*.{test,spec}.{ts,tsx}` and
`src/test/**/*.{ts,tsx}`. An empty `catch {}` is fine *only* if it contains a
comment — `no-empty` ignores blocks with comments.

## 7. Verified facts you can rely on

- **FR-CONVERT-10 grep, re-run against this tree:** 21 matches across the 9
  files the PRD names. The list is exhaustive and correct; the `ui/` primitives
  are already fully tokenised.
- **Contrast:** all sixteen required text pairings clear 4.5:1. Tightest is
  light `--warning` on white at **5.01:1**. Design §5.1 guessed "roughly 4.8:1";
  the computed value is better. **No token value needs adjusting.** Full table
  in `contrast.md` (created by plan Task 6).
- **`--background` and `--card` are identical** in both themes, which is why the
  "sixteen pairings" count works out: 4 families × 2 measurements × 2 themes.
- **Icon output:** 36.4 KiB total against a 100 KB budget. `favicon.ico` carries
  both 16×16 and 32×32 entries. The generated `d` string appears verbatim in
  `favicon.svg`. All verified by running the exact generator in the plan.
- **Maskable shrink factor:** 0.828, computed at generation time rather than
  hardcoded, so a geometry change cannot silently push the mark outside
  Android's safe circle.
- **No existing test builds a `User` fixture**, so adding a required
  `themePreference` to `UserAttributes` breaks nothing.
- **`PATCH /fleets/{id}`** (`apps/fleet-service/internal/fleet/resource.go:69`)
  is the `RegisterInputHandler` precedent to copy.

## 8. Task dependency order

Backend 1→5 and frontend 6→12 are independent of each other and could run in
parallel. Within each chain the order matters:

```
1 theme.go ─→ 2 model/entity/builder ─→ 3 processor ─→ 4 rest.go ─→ 5 PATCH route

6 tokens+types ─→ 7 lib/theme ─→ 8 ThemeContext ─→ 10 ThemeToggle ─┐
                              └→ 9 useUpdateTheme ────────────────┤
                                                     11 ThemeSync ─┤
                                        12 pre-paint + guard test ─┤
              13 icon generator ─→ 14 BrandMark/manifest/head ─────┤
                                                                   ├→ 15 AppLayout
                                                                   └→ 16 conversions ─→ 17 verify
```

Task 15 needs both `ThemeToggle` (10) and `BrandMark` (14). Task 16's guard test
must land after Task 15, or it fails on `AppLayout`'s own colours.

## 9. What "done" looks like

- `make ci` green.
- `docker build -f apps/web/Dockerfile .` succeeds and the running container
  returns 200 with a non-HTML content type for `/favicon.svg`, `/favicon.ico`,
  `/apple-touch-icon.png`, and `/site.webmanifest`. An HTML response means the
  request fell through to the SPA fallback and the asset is not in the image.
- The FR-CONVERT-10 grep returns nothing.
- `contrast.md` records a measured ratio for all sixteen pairings.
- The manual visual pass in plan Task 17 is complete, including the FR-3P-3
  Radix `select` check in dark mode and the cross-browser persistence check that
  proves the preference is server-side rather than `localStorage`.

## 10. Before opening a PR

Per `CLAUDE.md`, run the code-review step — `/audit-plan` or
`superpowers:requesting-code-review` — before opening a PR. Do not skip it even
when the plan looks fully checked off. Both a Go and a frontend reviewer apply
here; findings go to `docs/tasks/task-003-dark-mode-branding/audit.md`.
