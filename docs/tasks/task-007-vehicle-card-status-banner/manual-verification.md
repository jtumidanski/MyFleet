# task-007 — Manual verification

jsdom performs no layout, so the checks below cannot be automated in the unit
suite. Run them against the app before opening the PR.

Start the app, sign in, and open `/vehicles` with at least one vehicle in each of
the four statuses, one with a photo, one without, and one with a VIN.

## The Carfax regression (FR-6.6) — the highest-value check here

- [ ] Click the Carfax button. Carfax opens in a new tab and the app stays on
      `/vehicles` — it does **not** navigate to the vehicle detail page.
      A missing `relative z-10` on the Carfax wrapper breaks exactly this, looks
      completely fine, and no unit test can catch it.

## Navigation (FR-6.2, FR-6.4, FR-6.8)

- [ ] Clicking anywhere on the card body — photo, banner, stat strip — opens the
      detail page.
- [ ] Middle-click on the card body opens the detail page in a new tab.
- [ ] Cmd/ctrl-click does the same.
- [ ] Right-click on the card body offers the browser's link context menu.
- [ ] Tab reaches the card link first, then Carfax. Each shows a visible focus
      ring, and the card link's ring is **not clipped** by the card's edge.

## Layout (FR-1.2, FR-1.3, FR-2.3)

- [ ] Cards in the same grid row have equal height, regardless of status, photo
      presence, VIN presence, or missing mileage.
- [ ] No horizontal overflow at the single-column breakpoint (narrow the window).
- [ ] The card does not visibly jump when a photo finishes loading.
- [ ] A long nickname truncates and does not widen the card.

## Banner (FR-3.x, NFR-11)

- [ ] Overdue and Upcoming cards are tinted; Healthy and Inactive are not.
- [ ] Toggle dark mode: both tinted treatments stay legible in both themes.
- [ ] Each banner shows an icon as well as text.

## Network (FR-2.2, NFR-2, NFR-3)

- [ ] In the network panel, each photo request carries `?variant=thumbnail`.
- [ ] Loading `/vehicles` issues exactly one request for the list plus one per
      distinct media id — no per-vehicle metadata or schedule request.
- [ ] Nothing contacts carfax.com before an explicit click.

## Roles (FR-6.10, NFR-12)

- [ ] Repeat the navigation checks as a `viewer`-role user; behaviour is
      identical.

## Deferred, note only (design D5, D8)

- [ ] Judge whether the 320px `thumbnail` variant reads soft in the hero box at
      `lg:grid-cols-3` on a high-DPI display. If it does, that is a
      variant-sizing task in `media-service`, not a change here.
- [ ] Judge whether three columns still read well now that the card is taller.
