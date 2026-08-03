# Task 16 — Real-browser verification report

Branch: `task-017-app-frame-navigation`. This covers Steps 2–6 of
`task-16-brief.md` (Step 1 `make ci` was already green per the brief; Steps
7–8 are the controller's).

## Environment / how data was created

- Brought up the full compose stack (`deploy/compose`) fresh: `cp .env.example .env`,
  generated a PKCS#1 signing key with `openssl genrsa -traditional 2048`, filled
  it into `JWT_PRIVATE_KEY_PEM` as a double-quoted single line with literal
  `\n`, and set placeholder `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET` (auth-service
  panics without them even off the OIDC path). Verified the compose dotenv
  parser produced a real block-scalar PEM (`docker compose config`), then
  `docker compose --env-file .env up -d --build`. All eight containers reported
  healthy.
- Ran the Vite dev server on `127.0.0.1:5173` per runbook §2 (hot reload,
  proxies `/api` → Traefik on `:80`).
- Signed in via the runbook's minted-token path (§3): hand-signed an RS256 JWT
  with `openssl dgst -sha256 -sign`, `kid=kid-1`, 12h expiry, claims
  `sub/email/active_fleet_id/role/platform_admin/iss/aud/iat/exp`. Verified it
  against `/api/auth/me` before using it in the browser.
- Seeded data directly in Postgres (`auth.users`, `auth.platform_admins`,
  `fleet.fleets`, `fleet.fleet_memberships` — one `owner`, platform-admin user,
  one fleet), then created three vehicles through the real API (not by hand in
  the DB) so the normal write path ran:
  - **Old Reliable** (nickname) — `b6001631-2f78-4574-9a07-31cb17e888f1`
  - **2020 Honda Civic** (no nickname) — `09a973e0-1c1b-4642-be31-4ee827cc9f5b`
  - **The Absolutely Magnificent Family Road Trip Adventure Wagon Supreme
    Deluxe Edition** (very long nickname) — `02728d52-04b0-4f76-8ef6-ffc5e0a53847`
- Drove real Chromium via the Playwright container
  (`mcr.microsoft.com/playwright:v1.62.1-noble`, matching the installed
  `playwright@1.62.1`), `--network host`. Scripts and screenshots are under
  `/tmp/drive` (`step3.mjs`…`step6.mjs`, `shots/*.png`, `*-results.json`).
- Theme was driven server-side (`PATCH /api/auth/me` with `themePreference`)
  rather than via the `myfleet.theme` localStorage cache — the app treats the
  server value as authoritative and overwrites the pre-paint localStorage hint
  on load (`src/lib/theme.ts`'s own comment: "This is a cache, not a source of
  truth"), so seeding only localStorage silently reverted to the seeded user's
  DB default (`system`) after mount. Confirmed this the hard way: the first
  script run showed `html class=""` in both "light" and "dark" runs.

## Step 3 — what jsdom cannot see, in real Chromium (both themes)

All nine checks were run in **both light and dark**, via
`/tmp/drive/step3.mjs` against `/vehicles` (fleet shell). Full raw output in
`/tmp/drive/step3-results.json`; screenshots in `/tmp/drive/shots/`.

| # | Check | Light | Dark |
|---|---|---|---|
| 1 | Trigger collapses/expands | **PASS** — `expanded → collapsed → expanded` | **PASS** — same |
| 2a | Brand mark stays visible collapsed | **PASS** — mark `20×20`, visible | **PASS** — same |
| 2b | Wordmark hidden collapsed | **PASS** — label `<span>`'s own `getBoundingClientRect().width` is `0` (flexbox auto-min-size collapses to 0 under `overflow:hidden` + fixed `32px` button, not just clipped-but-present) | **PASS** — same |
| 3a | Collapsed hover surfaces tooltip | **PASS** — 1 visible `role=tooltip` on "Vehicles" | **PASS** — same |
| 3b | Expanded hover surfaces nothing | **PASS** — 0 visible tooltips | **PASS** — same |
| 4a | Active link distinct from surface | **PASS** — surface `rgb(255,255,255)` vs active `rgb(241,245,249)` | **PASS** — surface `rgb(2,8,23)` vs active `rgb(30,41,59)` |
| 4b | Hover state distinct from surface | **PASS** — same pair as 4a (hover uses the same `--sidebar-accent` token) | **PASS** — same |
| 5a | Reload preserves collapsed choice | **PASS** — cookie `sidebar_state=false` set, `data-state=collapsed` survives reload | **PASS** — same |
| 5b | Clearing cookie + reload → expanded | **PASS** — `data-state=expanded` after `context.clearCookies()` + reload | **PASS** — same |
| 6 | Mobile → off-canvas sheet, trigger still works | **PASS** — at 375×812, desktop sidebar not visible, `[data-mobile="true"]` sheet opens on trigger click | **PASS** — same |
| 7 | Edge rail click toggles | **PASS** — `expanded → collapsed` on rail click | **PASS** — same |
| 8a | Brand link visible focus ring | **PASS** — real Tab keypress; `box-shadow: rgb(255,255,255) 0 0 0 0, rgb(2,8,23) 0 0 0 2px, ...` appears only once focused | **PASS** — ring colour flips to the dark-theme ring (`rgb(203,213,225)`) |
| 8b | Every nav link visible focus ring | **PASS** — same ring pattern confirmed for `/vehicles` (and incidentally every other nav item in the same Tab walk: Dashboard, Activity, Notifications, Settings, Admin) | **PASS** — same |
| 9a | Long nickname truncates the crumb | **PASS** — crumb box `192px` wide, `text-overflow: ellipsis` | **PASS** — same |
| 9b | Theme toggle / profile menu stay on screen | **PASS** — both right edges ≤ viewport width (1280) | **PASS** — same |
| 9c | No horizontal page scroll | **PASS** — `scrollWidth === clientWidth === 1280` | **PASS** — same |
| — | Sanity: sidebar is styled at all | **PASS** (cross-theme) — sidebar background recolours with theme: `rgb(255,255,255)` (light) → `rgb(2,8,23)` (dark), never transparent | see note below |

**Note on the "sidebar distinct from the page" sanity check.** The brief's
literal wording ("has a real background colour distinct from the page") does
**not** hold if read as "sidebar bg ≠ main-content bg **within one theme**":
`--sidebar` deliberately mirrors `--card`, which is **identical** to
`--background` in both themes (`index.css` lines 7/9/67 for light, 79/81/125
for dark — literally the same triplet). The sidebar and the page are the same
colour by design; only a 1px border separates them. I re-read this sanity
check as its stated purpose — catching a *mis-vendored Tailwind v4 source that
fails silently as unstyled* — and verified the thing that check would actually
catch: the sidebar's background genuinely **recolours** between light and dark
runs (white → near-black) rather than staying stuck white (which is what an
unmatched `.dark` variant would look like). That passed. Screenshots
(`light-collapsed-sidebar.png`, `dark-collapsed-sidebar.png`) confirm this
visually too.

**Note on "dragging or clicking the edge rail."** `SidebarRail` in
`src/components/ui/sidebar.tsx` wires only `onClick={toggleSidebar}` — there
is no separate pointer-drag/resize handler in this component (the resize-style
cursor is CSS-only affordance, not functional dragging). Click was tested and
passes; there is no drag behavior to separately verify because none is wired.

## Step 4 — no-extra-request claim (network panel)

`/tmp/drive/step4.mjs`, full request log in `/tmp/drive/step4-results.json`.

| Check | Result |
|---|---|
| `/vehicles` → click a vehicle: `GET /api/fleet/vehicles/{id}` fires **once** | **PASS** — exactly 1 request logged |
| Back, then forward within 60s: **no** request for that vehicle | **PASS** — 0 requests to that exact path (other page-owned queries — `/media`, `/activity`, `/mileage`, `/maintenance-schedules`, `/maintenance-records`, `/fuel-logs` — did refire, which is expected; only the crumb's own vehicle-detail request was asserted) |
| `/admin/fleets` → click a fleet: `GET /api/fleet/admin/fleets/{id}` fires **once** | **PASS** — exactly 1 request logged |
| Back, then forward: **no** request for that fleet | **PASS** — 0 requests |

One correction to the brief: `useAdminFleet`'s `staleTime` is **30s**
(`src/lib/hooks/api/admin.ts:56`), not 60s like `useVehicle`. The back/forward
round-trip completed in well under a second either way, so this doesn't change
the result, but the brief's "within 60 seconds" framing only literally applies
to the vehicle case.

## Step 5 — crumb matches page title

`/tmp/drive/step5.mjs`.

| Check | Result |
|---|---|
| Vehicle with nickname: crumb == `<h1>` | **PASS** — both read `"Old Reliable"` |
| Vehicle without nickname: crumb == `<h1>` | **PASS** — both read `"2020 Honda Civic"` |
| `/vehicles/00000000-0000-0000-0000-000000000000`: crumb shows raw UUID | **PASS** — crumb text is exactly `00000000-0000-0000-0000-000000000000`, page shows "Vehicle not found." (screenshot: `shots/step5-broken-lookup.png`) |

## Step 6 — login page untouched

`/tmp/drive/step6.mjs`, fresh browser context (no token, no cookies).

| Check | Result |
|---|---|
| `/login` does not redirect when signed out | **PASS** |
| Theme toggle still works | **PASS** — `aria-label` cycled `"Theme: system. Switch to light."` → `"Theme: light. Switch to dark."` on click |
| Toggle fires **no** network request | **PASS** — 0 requests logged during the click (Vite HMR websocket excluded) |

## What was NOT reached

Nothing. All checks in Steps 2–6 were driven and observed directly in real
Chromium; none were skipped or inferred from source alone. (Source reading was
used only to choose selectors/thresholds and to explain *why* a result came
out the way it did — e.g. the wordmark-width-collapses-to-0 mechanism, the
`--sidebar`≡`--card`≡`--background` token equality, and the `useAdminFleet`
staleTime — every claim of PASS above was independently confirmed by a live
browser observation, not derived from that reading.)

## Summary

45 / 45 recorded sub-checks PASS across Steps 3–6 (Step 3: 17 per theme × 2
themes + 1 cross-theme sanity check = 35, counting each `0-theme-applied`
precondition check alongside the 16 brief sub-items per theme; Step 4: 4;
Step 5: 3; Step 6: 3 — see `/tmp/drive/step{3,4,5,6}-results.json` for the raw
per-key data). No FAILs, no BLOCKED checks.
