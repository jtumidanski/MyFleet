# Manual verification checklist — task-003 dark mode & branding

Plan Task 17 steps 4 and 5. Every automated gate on this branch is green
(`make ci`, the container asset check, the FR-CONVERT-10 grep, the three
convention guards). The items below require a browser and were **not
executed** by the agent that produced the rest of the verification record —
they are the only remaining gate before this branch is genuinely done.

### A. Visual pass — FR-3P-3 and PRD §10 "Visual completeness" (both Light and Dark)

Start the app: `npm run -w apps/web dev`, backend up via `make up`.

For **each** of the following, switch the theme toggle through Light and Dark
and re-check the item in both states. A failure is: illegible text (need to
eyeball obvious low-contrast), a token clearly not switching (e.g. still
showing a light-mode color while `dark` class is active), or a hardcoded
white/black patch that breaks the surface.

1. **Every route** — Dashboard, Vehicles, Vehicle Detail, Maintenance, Fuel,
   Activity, Notifications, Settings, Login, Onboarding, Invite Accept. Look
   for: any element that stays light-themed (white card, black text on dark
   bg) when the rest of the page is dark, or vice versa. Failure = any single
   surface not participating in the theme.
2. **Sidebar, nav links (including active-state), header, Sign-out button.**
   Look for: the active nav link having a visible distinct treatment from
   inactive links in both themes; header background/text legible in dark.
   Failure = active state indistinguishable from inactive, or Sign-out button
   unreadable/invisible in either theme.
3. **Severity chips** (Urgent / Recommended / Info) and **status badges**
   (Healthy / Upcoming Maintenance / Overdue / Inactive). Look for: each of
   the 3 severities and 4 statuses being both individually readable (text vs.
   fill contrast) and mutually distinguishable (no two severities/statuses
   looking the same) in both themes. Failure = any pair that reads as the
   same color, or text that disappears into its own chip fill.
4. **`MaintenanceQueueView` overdue callout** — confirm in dark mode the
   callout fill is a dark, saturated red-family fill (the `-subtle` token,
   "-100 band"), **not** a near-white or washed-out fill. Failure = the
   callout reads as a pale/white box in dark mode.
5. **Toasts, both success and error.** Trigger one of each (e.g. save a form
   for success, submit invalid data for error). Look for: readable text on
   the toast background in both themes. Failure = toast text illegible
   against its own background in either theme.
6. **Radix `select` dropdown content in dark mode.** Open any `<Select>`
   (e.g. a form with a dropdown) in Dark mode. Look for: the dropdown panel
   itself (which portals to `document.body`) picking up dark surface/text
   tokens — not rendering as a stray white box floating over a dark page.
   Failure = the popover content is light-themed while the rest of the app is
   dark.
7. **Form inputs, focus rings, skeletons, card borders.** Tab into a few
   inputs to trigger focus rings; observe a loading skeleton if one is
   reachable; check card borders are visible-but-subtle in both themes.
   Failure = invisible focus ring, skeleton indistinguishable from
   background, or borders that vanish entirely in dark mode.
8. **Toggle behaviour — three-step cycle.** Click the theme toggle
   repeatedly and confirm it cycles Light → Dark → System → Light (or
   whatever the defined order is) — three distinct states, not just a
   two-way flip. Failure = only two states reachable, or a state skipped.
9. **OS-theme-follow behaviour.** While the toggle is on **System**, flip the
   OS/browser dark-mode setting and confirm the app updates live without a
   manual reload. While the toggle is on **Light** (not System), flip the OS
   setting and confirm the app does **not** change. Failure = System doesn't
   follow OS live, or Light changes when OS flips (should be pinned).
10. **Hard refresh on Dark shows no flash.** With the preference set to Dark,
    hard-refresh the page (Ctrl+Shift+R / Cmd+Shift+R) and watch closely for
    a flash of light-themed content before dark styles apply. Failure = any
    visible white flash before the dark theme paints.

### B. Cross-device/browser persistence check — the core point of the feature

11. **Server-side persistence, not `localStorage`.** In Browser Profile A,
    set the theme to Dark via the toggle, then sign out. Sign in from a
    **different browser profile** (Profile B — a genuinely separate cookie
    jar/profile, not just a new tab) using the same user account. The app
    must open in **Dark** on first paint. Failure = Profile B opens in Light
    or System, proving the preference lives only in `localStorage` rather
    than being fetched from the server on login.
12. **Bogus `localStorage` value is ignored.** In a fresh browser profile,
    open devtools, run `localStorage['myfleet.theme'] = 'purple'`, then
    reload. The app must load normally, following System theme (or whatever
    the safe fallback is) — not crash, not get stuck, not honor the bogus
    value. Failure = a crash, blank page, or the app treating `'purple'` as a
    valid theme state.
13. **`localStorage` fully blocked still works.** In a fresh profile, block
    all site data/cookies for the app's origin (browser site settings →
    block cookies/site data), then reload. The app must still boot
    successfully, and the theme toggle must still function for the
    remainder of the session (even if the choice won't persist across a
    future reload without storage). Failure = the app fails to boot, throws
    on any `localStorage` access, or the toggle is non-functional with
    storage blocked.

---

## Summary — pass/fail by brief item

| Item | Result |
|---|---|
| Step 1: `make ci` (lint-check, vet, test, build, fe-test, fe-build, manifests) | **PASS** |
| Step 2: container serves assets, all 200 + non-HTML content-type | **PASS** (brief's example `-p 8099:80` is wrong — actual container port is 8080; corrected and re-verified) |
| Step 3: asset budget under 102400 bytes | **PASS** — 37,736 bytes |
| Step 4: manual visual pass (Light/Dark, all routes, Radix select, etc.) | **NOT EXECUTED** — requires a browser; checklist above provided for a human |
| Step 5: cross-device persistence check | **NOT EXECUTED** — requires a browser; checklist above provided for a human |
| Step 6: commit outstanding changes | **N/A** — working tree already clean, nothing to commit |
| FR-CONVERT-10 grep | **PASS** — zero matches, exit 1 |
| contrast.md — 16 pairings recorded and correct | **PASS** — independently recomputed, matches exactly |

## What I could not verify and why

- Steps 4 and 5 (manual visual/browser pass) — I cannot drive a browser in
  this environment. Provided as an ordered manual checklist above; not
  simulated or claimed as done.
- The already-verified icon totals/favicon claims (37,245 bytes, 16×16/32×32
  in `favicon.ico`, `d` string match) were taken as given per the task
  instructions rather than independently re-derived byte-for-byte; the
  `du -cb` total I measured (37,736 bytes across all of `public/`, a
  superset) is consistent with, not contradictory to, that figure.
