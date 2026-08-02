# task-013 manual verification

Two acceptance criteria are measurements, not assertions, and are not covered by
the automated suite.

## Before deploying

Run the duplicate-row pre-flight in `deploy-preflight.md` against each target
database. A duplicate `(media_object_id, variant)` pair makes the new unique
index fail to create, and media-service will not boot.

## 1. A pre-existing photo becomes sharp on the vehicles list

This is the defect task-007 deferred ("Deferred, note only" in its
`manual-verification.md`).

1. Pick a vehicle whose primary photo was uploaded before this task — it has
   `thumbnail` and `display` rows but no `card` row.
2. Open the vehicles list on a high-DPI display, in a window wide enough for
   `lg:grid-cols-3` (≥1024px), ideally at 1440px where the softness was first
   reported.
3. **First load:** the hero renders from the downgraded thumbnail. Expected —
   this is the current behaviour, not a regression.
4. Confirm a `card` row appeared:
   `SELECT variant, width, height FROM media.media_variants WHERE media_object_id = '<id>';`
   Expect three rows, with `card` at 768 on its longest edge.
5. Confirm the object's `thumbnail` and `display` rows are still there. If they
   are gone, lazy generation used `ReplaceForMediaObject` — a blocking defect.
6. Hard-reload (bypassing both the React Query 5-minute stale window and the
   `Cache-Control: private, max-age=300` browser cache). The hero should now be
   visibly sharper.
7. **Expect convergence over several visits, not one hard reload.** Three
   things compose against a single-reload expectation:
   - the downgraded thumbnail is cached under the *same* key and URL as the
     eventual card, by React Query (`staleTime` 5 min, `gcTime` 6 min,
     `refetchOnWindowFocus: false`) **and** by the browser
     (`Cache-Control: private, max-age=300`);
   - nothing signals that a downgrade occurred, so neither cache has a reason
     to invalidate itself on that basis;
   - the concurrency cap **drops** excess lazy-generation requests rather than
     queueing them, so a cold twelve-card grid schedules only about four
     generations per visit — the rest are simply not attempted, not queued for
     later.

   Net effect for a twelve-vehicle grid of pre-existing media: full sharpness
   takes roughly **three visits**, spaced past the five-minute cache window
   each time. Seeing a still-soft grid on visit two is convergence-in-progress,
   not a defect — do not mistake it for one. A hard-reload bypasses the browser
   cache and forces fresh requests, but it does **not** refill the concurrency
   cap, so it cannot make more than the capped number of generations happen at
   once. This is expected behaviour: PRD §9.6 (a shorter `staleTime` dedicated
   to `card`) is deliberately still open, and closing it is what would remove
   this multi-visit wait.

## 2. `card` is materially cheaper than `display` (NFR-1)

In DevTools → Network, request the same photo twice and compare transferred
sizes:

- `GET /api/media/<id>/content?variant=card`
- `GET /api/media/<id>/content?variant=display`

`card` must be substantially smaller. 768 vs 1280 on the longest edge is roughly
2.8x fewer pixels, so expect a large fraction of that in bytes. If `card` is
close to `display`, the variant is not earning its keep and the sizing decision
needs revisiting.

Record both numbers here when measured.

## 3. Nothing else changed

- The vehicle detail gallery (`MediaThumbnail`) still sends **no** `?variant=`
  parameter and renders as before.
- `GET /api/media/<id>/content?variant=display` for an object with no display
  row still returns 404 — the downgrade did not generalise.
- `GET /api/media/<id>/content?variant=bogus` still returns 400.
