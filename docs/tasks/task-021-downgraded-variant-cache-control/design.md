# Downgraded Card Variant — Cache Identity Fix — Design

Task: task-021-downgraded-variant-cache-control
PRD: `docs/tasks/task-021-downgraded-variant-cache-control/prd.md` (v1, approved)
Issue: #29
Status: Approved for planning
Created: 2026-08-03
---

## 1. The shape of the problem

`Processor.Content` already knows, at the moment it happens, that it is serving
the wrong image. The knowledge is right there in the branch:

```go
// processor.go:405-419
if want != ContentCard || !errors.Is(err, server.ErrNotFound) {
    return ContentInfo{}, nil, err
}
pr.scheduleCard(m)
pr.log.WithField("media_id", m.ID()).
    Debug("serving the thumbnail in place of an unavailable card variant")
return pr.openVariant(ctx, m, ContentThumbnail)
```

That knowledge then evaporates. The value returned from the downgrade branch is
byte-for-byte indistinguishable from the value returned by a genuine
`?variant=thumbnail` request, so by the time control reaches the handler at
`resource.go:207-208` there is nothing left to condition on and every response
gets the same `private, max-age=300`.

The fix is therefore not "add caching logic" — it is **stop discarding a fact
the producer already has**. Everything below follows from that framing: the
domain layer reports what happened, the transport layer decides what that means
for HTTP, and neither re-derives the other's business.

The one behavioural consequence — a soft image is no longer stored under the
sharp image's URL — falls out of that for free.

## 2. Architecture

Three touch points, one direction of data flow, no new types:

```
Processor.Content ──(ContentInfo{Downgraded: true})──▶ GET /media/{id}/content handler
     │                                                            │
     │ sets the flag on exactly one                               │ selects Cache-Control
     │ branch: card miss → thumbnail                              │ from the flag
     ▼                                                            ▼
 openVariant stays ignorant                          private, no-store  |  private, max-age=300
 (it does not know why it was called)
```

| Layer | File | Responsibility |
|---|---|---|
| Domain value | `internal/mediaobject/processor.go` | `ContentInfo` gains `Downgraded bool` — a statement of fact about the bytes, with no HTTP vocabulary in it. |
| Domain logic | `internal/mediaobject/processor.go` | `Content` sets the flag at the one site that performs a substitution. |
| Transport | `internal/mediaobject/resource.go` | Translates the fact into a cache policy. |

There is exactly one caller of `Processor.Content` in the service
(`resource.go:164`), so the blast radius of the signature-preserving change is a
single handler.

### D1 — Carry the fact on `ContentInfo`, not beside it

`ContentInfo` already exists for precisely this purpose: it is described in its
own doc comment as "the bytes actually being served, which are not always the
media object's own metadata." A substitution is the most extreme case of that
sentence being true, so the flag belongs in the struct rather than alongside it.

Alternatives considered:

- **A fourth return value** — `(ContentInfo, io.ReadCloser, bool, error)`. Every
  call site pays for a fact only one branch produces, and the boolean is
  positionally unlabelled at the call site. Rejected.
- **A sentinel wrapper error** — return the response *and* an
  `ErrDowngraded`-flavoured error. Overloads the error channel with something
  that is not a failure; the handler's `if err != nil` guard would have to grow a
  special case. Rejected.
- **Re-derive in the handler** — compare `want == ContentCard` against something
  about the bytes. There is nothing to compare against: a card and a thumbnail
  are both `image/jpeg`, both have `Size == 0`, and both build the same
  disposition. Not merely fragile — not possible. Rejected.
- **A `context` value stashed by the processor** — invisible coupling through a
  side channel for data that has an obvious home in the return value. Rejected.

`Downgraded bool` also preserves two properties the existing code depends on:
the zero value is the correct default, so `openOriginal`, `openVariant`, and the
existing `return ContentInfo{}, nil, err` sites need no edit; and the struct stays
comparable, so the equality assertion at `processor_test.go:763`
(`if info != (ContentInfo{})`) keeps compiling.

### D2 — Set the flag in `Content`, never in `openVariant`

The flag is set on the value returned by the *second* `openVariant` call, at the
call site, inside `Content`:

```go
info, rc, err := pr.openVariant(ctx, m, ContentThumbnail)
if err != nil {
    // No thumbnail either: a 404, and no response to mark.
    return ContentInfo{}, nil, err
}
info.Downgraded = true
return info, rc, nil
```

This is the load-bearing decision of the whole design. The tempting alternative
is to thread a `downgraded bool` parameter into `openVariant`, or to have
`openVariant` infer it — and both are wrong for the same reason: **`openVariant`
opens the rendition it was asked for, so from where it stands nothing was ever
substituted.** Only `Content` knows that the caller asked for `card` and is
getting `thumbnail`. Pushing the fact down the stack would mean an explicit
`?variant=thumbnail` request and a downgraded one become indistinguishable again
at exactly the layer that has no way to tell them apart — which is the bug this
task exists to fix, reintroduced one function lower.

`ContentInfo` is returned by value, so mutating the local copy before returning
it is safe and touches nothing else.

The `if err != nil` guard is not redundant with `openVariant`'s own zero-value
returns. It is the thing that makes PRD FR-DG-2's last two rows structurally
true rather than incidentally true: a missing thumbnail and a store fault both
return a zero `ContentInfo` with `Downgraded == false`, because the assignment
is unreachable on those paths. Writing `info.Downgraded = true` unconditionally
before the error check would set the flag on a zero struct being discarded —
harmless today, and a trap the first time someone inspects `info` on an error
path.

### D3 — The handler owns the cache policy; the domain does not

`resource.go` selects the header value inline:

```go
// Per-fleet authorized bytes — never store in a shared cache. private is
// unconditional; only the freshness half varies.
cacheControl := "private, max-age=300"
if info.Downgraded {
    // The bytes under this URL are a stand-in for a card that is usually
    // generated within seconds of this request. Storing them would pin the
    // soft image to the sharp image's URL for the whole max-age window with
    // nothing able to invalidate it.
    cacheControl = "private, no-store"
}
w.Header().Set("Cache-Control", cacheControl)
```

The rejected alternative worth naming: **`ContentInfo.CacheControl string`**,
computed by the processor and set verbatim by the handler. It is fewer lines and
it is the wrong shape — it puts an HTTP header value in a domain type, so the
processor would be making a transport policy decision it has no business making,
and a second transport (a signed-URL redirect, a gRPC reader) would inherit a
field that means nothing to it. This is the "don't break service boundaries by
having one layer call another's internals" rule in `CLAUDE.md` applied to data
rather than to calls: the domain reports *what happened*, transport decides
*what that means on the wire*.

A package-level `cacheControlFor(downgraded bool) string` helper was also
considered and rejected as premature: one branch, one caller, and the handler
test already covers both rows of the table. Extract it if a third policy appears.

### D4 — `private` survives in both branches

`private, no-store` rather than bare `no-store`. `no-store` alone is stricter in
practice, but `private` is the *statement* that these are per-fleet authorized
bytes, and it is the thing a reviewer reads to confirm the authorization model
was considered. Keeping both halves means the two policies differ in exactly one
token, which is also what makes the diff and the tests easy to read.

## 3. Data flow, end to end

A cold `?variant=card` request for media uploaded before the card variant
existed:

1. Handler parses `variant=card`, calls `proc.Content(ctx, id, fleetID, ContentCard)`.
2. `GetByID` + `AuthorizeAccess` — unchanged, still the fleet-scoping gate.
3. `openVariant(card)` → `server.ErrNotFound` (no row, or a row with no object).
4. `want == ContentCard` **and** the error is `ErrNotFound` → downgrade branch.
5. `scheduleCard(m)` — unchanged (FR-DG-5). Generation usually completes in seconds.
6. `Debug` log — unchanged, verbatim.
7. `openVariant(thumbnail)` → `(info, rc, nil)`; `info.Downgraded = true`.
8. Handler writes `Content-Type`, `X-Content-Type-Options`, `Content-Disposition`;
   omits `Content-Length` (`info.Size == 0`); writes `Cache-Control: private, no-store`;
   `200`; copies bytes.
9. The browser renders the soft image and **stores nothing**.
10. On the next page load the React Query cache is empty (it is in-memory, per app
    instance, no persister — PRD §9.1), so a real `fetch` goes out; the HTTP cache
    has no entry to answer it; the request reaches the service; the card now exists;
    step 3 succeeds and the response is `private, max-age=300`.

Every other path — `original`, no parameter, `thumbnail`, `display`, a present
`card` — reaches step 8 with `Downgraded == false` and is byte-identical to
today.

## 4. Failure modes and error handling

No new error paths. The design's correctness rests on the flag being *absent* in
every case that is not a substitution, so the interesting analysis is the
negative space:

| Situation | Result | Why the flag is false |
|---|---|---|
| Card exists | `Downgraded = false` | The downgrade branch is never entered. |
| `?variant=thumbnail` | `Downgraded = false` | First `openVariant` succeeds; `Content` returns early at `processor.go:401-403`. The caller got what it asked for. |
| `?variant=display` misses | `ErrNotFound`, zero `ContentInfo` | `want != ContentCard` fails the guard at `processor.go:410`. The downgrade must not generalise. |
| `?variant=original` / no parameter | `Downgraded = false` | `Content` returns from `openOriginal` before the variant path. |
| Card row present, object missing | `Downgraded = true` | `openVariant` maps `storage.ErrObjectNotFound` to `server.ErrNotFound`, so this downgrades — correctly, since a substitution genuinely occurred. |
| Card missing **and** thumbnail missing | `ErrNotFound`, zero `ContentInfo` | The `err != nil` guard in D2. Nothing was served, so there is nothing to mark. |
| Database or store **fault** | Propagates as a 500-class error | `!errors.Is(err, server.ErrNotFound)` fails the guard. A fault must never hide behind a thumbnail. |
| No `CardGenerator` wired | `Downgraded = true` | `scheduleCard` is nil-safe (`nopCardGenerator`, covered at `processor_test.go:1188`); the response path is unaffected. |

The `no-store` policy itself has no failure mode that costs correctness. Its
worst case is redundant fetches of the smallest rendition the service stores,
over a population that shrinks monotonically toward empty as lazy fill proceeds.

## 5. Testing strategy

Two test files, both extending fixtures that already exist. The tests are
structured to prove the *negative space* above, not just the happy path — a
change that flagged everything would satisfy the positive assertion alone.

### `processor_test.go` — the fact

- Extend the downgrade-matrix subtests in
  `TestContent_cardDowngradesToThumbnailOnly` (`:891`) to assert
  `info.Downgraded == true`, including the store-drift subtest at `:969`.
- Assert `Downgraded == false` on the present-card path (`:1014`), on an explicit
  `?variant=thumbnail`, on `?variant=original`, and on a no-parameter request.
  The explicit-thumbnail case is the one that would catch a D2 regression: its
  bytes are identical to a downgraded response, so nothing but the flag
  distinguishes them.
- Assert the zero `ContentInfo` on the display miss (`:961`) and on the
  fault case (`TestContent_cardLookupErrorIs500WithNoDowngrade`, `:1046`).

### `resource_test.go` — the policy

The existing `thumbnailRouter` fixture seeds `thumbnail` and `display` refs and
**no `card`**, so `?variant=card` against it already produces a downgraded
response. No new fixture is needed.

- New test: a downgraded card response carries `Cache-Control: private, no-store`
  and is otherwise identical to today — `200`, `thumb-bytes`, `Content-Type:
  image/jpeg`, no `Content-Length`, `X-Content-Type-Options: nosniff`, and no
  new header (FR-DG-4).
- The three existing `private, max-age=300` assertions (`:313`, `:737`, `:764`)
  are the regression guard for the `false` row and **must pass unedited**. If a
  plan step wants to touch them, the production change is wrong.

### Proving the tests can fail

Per the project's standing rule, each new assertion is proven red before landing:
revert `info.Downgraded = true` and confirm the processor assertions fail; revert
the handler branch and confirm the `no-store` assertion fails. A test that never
went red has not been shown to test anything.

### Full verification

`make ci`. No deployment manifests change, so no `kustomize` render is required.

## 6. Scope boundaries

Held deliberately, from the PRD's non-goals:

- **`apps/web/` is untouched.** The React Query cache is in-memory with no
  persister, so a page load starts empty and the browser HTTP cache is the layer
  that decides the outcome. `staleTime` tuning would change behaviour only
  *within* one uninterrupted session, where a five-minute-old soft image is a
  much smaller problem.
- **No new response header.** `X-Media-Variant-Downgraded` would change the
  public response contract to expose a fact no client needs; the browser enforces
  the fix without any client being aware of it. Revisit only if `Cache-Control`
  alone proves insufficient in practice.
- **`processing/card.go` is unmodified.** The saturation drop stays a drop
  (PRD §9.3).
- **No metric, no log-level change, no configuration, no migration, no backfill**
  (PRD §9.4).

## 7. Why this is worth doing at all

It is a quality-of-first-impression fix, not a correctness fix — the system
already converges without it. What it changes is how long convergence takes:
today a cold grid can serve a stale thumbnail for five minutes past the moment
the sharp card exists, and it takes several visits to work through that. After
this change, one revisit is enough. The cost is three lines of production code on
a branch that already exists, with no additional I/O on the request path.
