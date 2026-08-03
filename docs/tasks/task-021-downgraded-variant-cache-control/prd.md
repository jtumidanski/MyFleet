# Downgraded Card Variant — Cache Identity Fix — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-03
Issue: #29
Supersedes: task-013 PRD §9.6 (deferred)
---

## 1. Overview

When a client requests `GET /media/{id}/content?variant=card` and no `card` variant
has been generated yet, media-service serves the `thumbnail` bytes instead. This
downgrade was introduced deliberately in task-013 (#26) so that media uploaded
before the `card` variant existed still renders something rather than 404ing, and
the response carries no signal that a substitution happened — same 200, same
shape, no header.

The downgrade itself is correct. The problem is what happens afterwards: the
handler stamps every content response with `Cache-Control: private, max-age=300`
(`apps/media-service/internal/mediaobject/resource.go:208`), so the *soft* image
is stored in the browser's HTTP cache under the *sharp* image's URL for five
minutes. Because the card generation the downgrade schedules typically completes
within seconds, the browser then spends the rest of that five-minute window
serving a stale thumbnail for a card that already exists on the server. Nothing
can invalidate it, because nothing recorded that the cached entry was a
substitute.

This task carries the downgrade fact out of `Processor.Content` — which already
knows, since it is the branch that calls `scheduleCard` — and into the handler,
so that downgraded responses alone are marked `no-store`. Sharp responses keep
their 300-second cache untouched. The result is that a page reload after
generation completes fetches the real card instead of replaying the thumbnail,
which is what makes a cold grid converge in one revisit rather than several.

This is a quality-of-first-impression fix, not a correctness fix. The system
already converges on its own; this shortens how long that takes.

## 2. Goals

Primary goals:

- Carry a "this response was downgraded" fact from `Processor.Content` to the
  HTTP handler via `ContentInfo`.
- Serve downgraded responses with `Cache-Control: private, no-store`, so no cache
  stores soft bytes under the sharp identity.
- Leave the cache policy of every non-downgraded response (`original`,
  `thumbnail`, `display`, and a genuinely-present `card`) exactly as it is today
  at `private, max-age=300`.
- Keep the downgrade otherwise unobservable to clients — no new response header,
  no status change, no body change.

Non-goals:

- **No frontend changes.** `apps/web` is out of scope entirely. React Query's
  `staleTime`/`gcTime` for the `card` key are not tuned in this task (see §9.1
  for why the fix still works without that).
- **No new response header.** Exposing the downgrade to the client (e.g.
  `X-Media-Variant-Downgraded`) was considered and rejected for this task; it
  changes the public response contract and is only worth it if the
  `Cache-Control` change alone proves insufficient in practice.
- **No change to the generation concurrency policy.** `CardGenerator.Generate`
  keeps dropping on saturation rather than queueing
  (`apps/media-service/internal/processing/card.go:100-107`). Load-shedding under
  pressure is the right default and changing it is a separate risk decision.
- **No new metrics or log levels.** The existing `Debug` line in
  `Processor.Content` is the only instrumentation; the downgrade is expected
  behaviour during lazy fill, not an incident.
- **No backfill job.** A one-off sweep generating `card` for all pre-existing
  media would make this convergence question moot, and is a reasonable follow-up,
  but it is not this task.
- **No change to what the downgrade serves.** Still `thumbnail`, still scoped to
  `card` only, still 404 for a missing `display`.

## 3. User Stories

- As a fleet member opening a vehicle grid whose photos predate the `card`
  variant, I want the sharp card image to appear on my next visit rather than
  several visits later, so the app does not look persistently blurry.
- As a backend engineer reading `Processor.Content`, I want the downgrade fact to
  be part of the value the function returns, so a caller that needs to behave
  differently for a substituted response can do so without re-deriving the
  condition.
- As an operator, I want the change to be invisible in request volume terms for
  media that has its `card` variant, so the existing 300-second cache continues to
  absorb repeat loads for the overwhelming majority of traffic.

## 4. Functional Requirements

### FR-DG-1 — `ContentInfo` carries the downgrade fact

`ContentInfo` (`apps/media-service/internal/mediaobject/processor.go:110-131`)
gains a boolean field, `Downgraded`, documented as: the bytes being served are a
smaller rendition than the caller asked for. It is `false` on every path except
FR-DG-2.

The field must be a plain `bool` with a `false` zero value, so every existing
construction site of `ContentInfo` (`openOriginal`, `openVariant`, and the
zero-value error returns) remains correct without modification.

### FR-DG-2 — Only the card→thumbnail downgrade sets the flag

`Processor.Content` sets `Downgraded = true` on exactly one path: the branch that
falls through after `openVariant(ctx, m, ContentCard)` returns
`server.ErrNotFound`, calls `scheduleCard(m)`, and re-opens as
`ContentThumbnail` (`processor.go:405-419`).

Specifically:

- A request for `?variant=card` where the card **exists** returns
  `Downgraded = false`.
- A request for `?variant=thumbnail` returns `Downgraded = false` **even though
  the bytes are identical to a downgraded response.** The caller got what it
  asked for; nothing was substituted.
- A request for `?variant=display` that 404s returns an error, not a downgraded
  response — the downgrade must not generalise (this is already enforced by
  `processor.go:410-412` and covered by an existing test at
  `processor_test.go:961`).
- A request for `?variant=original` or with no `variant` parameter returns
  `Downgraded = false`.
- A downgrade attempt where the **thumbnail is also missing** returns
  `server.ErrNotFound` and a zero `ContentInfo`; there is no response to mark.
- A card row that exists but whose object is missing from storage
  (`storage.ErrObjectNotFound`) downgrades and therefore sets `Downgraded = true`,
  consistent with the existing test at `processor_test.go:969`.
- A database or store **fault** (any non-`ErrNotFound` error) still propagates as
  a fault. It does not downgrade and does not set the flag.

### FR-DG-3 — The handler sets `no-store` on downgraded responses

The content handler (`resource.go:207-208`) selects its `Cache-Control` value
from `info.Downgraded`:

| `info.Downgraded` | `Cache-Control` |
|---|---|
| `false` | `private, max-age=300` (unchanged) |
| `true` | `private, no-store` |

`private` is retained in both cases. These are per-fleet authorized bytes and must
never enter a shared cache regardless of freshness policy; `no-store` alone would
lose the statement of intent that the existing comment at `resource.go:207`
makes.

### FR-DG-4 — All other headers are unchanged

A downgraded response continues to carry exactly the headers it carries today:
`Content-Type` and `Content-Disposition` from the thumbnail's own `ContentInfo`,
`X-Content-Type-Options: nosniff`, and no `Content-Length` (variants record no
byte count, so `info.Size` is 0 and the header is omitted). Only `Cache-Control`
differs.

### FR-DG-5 — Scheduling behaviour is untouched

`scheduleCard` is still called on every downgrade, with the same eligibility rules
(`StatusReady` + `ClassImage`), and `CardGenerator` retains its single-flight
reservation, its saturation drop, and its permanent-failure ledger check. This
task changes how the *response* is cached, not how work is scheduled.

## 5. API Surface

**No public API change.** `GET /media/{id}/content?variant=card` keeps its status
codes, body, and header set. The only observable difference is the value of the
`Cache-Control` header on responses that were already, and remain, indistinguishable
from a normal 200 in every other respect.

This is a deliberate property: a client cannot detect the downgrade, and none
needs to. The cache-policy change is enforced by the browser without any client
code being aware of it.

## 6. Data Model

**No data model change.** No new tables, columns, or migrations. `media_variants`
is read exactly as it is today; the permanent-failure ledger
(`variantfailures`) is untouched.

## 7. Service Impact

### `apps/media-service` — the only service changed

| File | Change |
|---|---|
| `internal/mediaobject/processor.go` | Add `Downgraded bool` to `ContentInfo` (FR-DG-1); set it on the downgrade return in `Content` (FR-DG-2). |
| `internal/mediaobject/resource.go` | Select `Cache-Control` from `info.Downgraded` (FR-DG-3). |
| `internal/mediaobject/processor_test.go` | Extend the existing downgrade-matrix tests to assert the flag; add negative cases for explicit `thumbnail` and a present `card`. |
| `internal/mediaobject/resource_test.go` | Add a handler test asserting `private, no-store` on a downgraded response. The three existing assertions of `private, max-age=300` (lines 313, 737, 764) must continue to pass unmodified — they are the regression guard for FR-DG-3's `false` row. |

### `apps/web` — not changed

Out of scope by decision. See §9.1.

### Deployment

No configuration change, no new environment variable, no manifest change. The
existing `MEDIA_LAZY_VARIANT_CONCURRENCY` knob keeps its meaning and default of 4
(`apps/media-service/cmd/main.go:108`).

## 8. Non-Functional Requirements

### Performance

`no-store` on downgraded responses means repeat renders of the *same* soft image
within a single page view re-fetch rather than hitting the browser cache. This is
acceptable and bounded:

- Thumbnail bytes are the smallest rendition the service stores.
- The population affected is only media that predates the `card` variant *and*
  has not yet had its card generated — a set that shrinks monotonically toward
  empty as lazy fill proceeds.
- Media with a present `card` — which is all newly uploaded media, and eventually
  all media — is entirely unaffected and keeps `max-age=300`.

There is no added latency on the request path: the flag is a struct field set on
a branch that already exists, with no additional I/O.

### Security

No change to the authorization model. `AuthorizeAccess` still runs before any
content is opened, and `private` is retained on every response, so per-fleet bytes
remain ineligible for shared caches under both policies.

### Observability

The existing `Debug` log in the downgrade branch is retained verbatim. No counter,
no gauge, no log-level change (per §2 non-goals).

## 9. Open Questions

### 9.1 — Does this actually fix anything without the frontend change? (Resolved: yes)

The concern raised in issue #29 is that React Query caches the downgraded blob
under the `'card'` key with `staleTime: 5 * 60 * 1000`
(`apps/web/src/lib/hooks/api/media.ts:158-159`), so a server-side cache change
might be masked by the client-side one.

This was verified against source and the answer is that the fix works:

- The `QueryClient` is constructed in a `useState` initializer inside
  `AppProviders` (`apps/web/src/components/providers/AppProviders.tsx:32-39`)
  with **no persister configured**. React Query's cache is in-memory and per app
  instance.
- Therefore a full page load starts with an empty React Query cache and issues a
  real `fetch` for every visible card.
- What determines whether that `fetch` reaches the network or is served stale is
  the **browser HTTP cache** — which is precisely the layer `no-store` controls.

So across page loads, which is the scenario the issue describes ("three visits
spaced past a five-minute window"), the server-side change is both necessary and
sufficient. The React Query `staleTime` only governs staleness *within* a single
uninterrupted session, where a five-minute-old soft image is a much smaller
problem.

> Note: issue #29 cites `apps/web/src/lib/providers/AppProviders.tsx`. The actual
> path is `apps/web/src/components/providers/AppProviders.tsx`. The
> `refetchOnWindowFocus: false` and `retry: 1` defaults the issue describes are
> correct.

### 9.2 — Should `no-store` or a short `max-age` be used? (Resolved: `no-store`)

`no-store` was chosen over a short `max-age` (e.g. 10s) because it needs no
magic number to justify or tune, and because the thing being cached is by
definition the wrong image. A short `max-age` would preserve some
duplicate-render absorption within a page view, but the bytes are small enough
that this is not worth the tuning surface. `no-cache` with revalidation was also
rejected: it only pays off with `ETag` support, which media-service does not emit
for variants.

### 9.3 — Should the generation cap queue rather than drop? (Deferred)

Issue #29 raises this independently. A twelve-card cold grid schedules roughly
four generations per visit because `Generate` drops on semaphore saturation. A
small bounded queue would converge a cold grid faster. This is deliberately not
part of this task: it changes a load-shedding policy that exists to protect the
pod under read traffic, and it deserves its own risk assessment. Track separately.

### 9.4 — Should there be a backfill? (Deferred)

Generating `card` for all pre-existing media in one sweep would eliminate the
lazy-fill window entirely and make both this task and §9.3 largely moot for the
existing corpus. It is the more complete fix but a materially larger one
(migration tooling, throughput control, failure handling). Worth considering as a
follow-up.

## 10. Acceptance Criteria

Behaviour:

- [ ] `ContentInfo` has a `Downgraded bool` field, documented, with `false` as its
      zero value.
- [ ] `Processor.Content` returns `Downgraded = true` for a `?variant=card`
      request where no card variant exists and a thumbnail does.
- [ ] `Processor.Content` returns `Downgraded = true` when the card row exists but
      its object is missing from storage.
- [ ] `Processor.Content` returns `Downgraded = false` for: a present `card`, an
      explicit `?variant=thumbnail`, `?variant=original`, and a request with no
      `variant` parameter.
- [ ] A `?variant=display` miss still returns `server.ErrNotFound` with a zero
      `ContentInfo` — no downgrade, no flag.
- [ ] A database or storage **fault** still returns a 500-class error and does not
      downgrade.
- [ ] A `?variant=card` request where neither card nor thumbnail exists still
      returns 404.

HTTP:

- [ ] A downgraded response carries `Cache-Control: private, no-store`.
- [ ] Every non-downgraded content response still carries
      `Cache-Control: private, max-age=300`.
- [ ] A downgraded response's `Content-Type`, `Content-Disposition`,
      `X-Content-Type-Options`, status code, and absent `Content-Length` are
      byte-identical to what it returns today.

Scope discipline:

- [ ] No files under `apps/web/` are modified.
- [ ] No new response header is introduced.
- [ ] `apps/media-service/internal/processing/card.go` is unmodified.
- [ ] No new metric, no log-level change, no new configuration.

Verification:

- [ ] The three existing `private, max-age=300` assertions in `resource_test.go`
      (lines 313, 737, 764) pass without being edited.
- [ ] Each new test is proven able to fail: revert the production change and
      confirm the test goes red before landing it.
- [ ] `make ci` passes.
