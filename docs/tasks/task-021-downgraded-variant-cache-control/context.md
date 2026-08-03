# task-021 — Implementation Context

Companion to `plan.md`. Everything an implementer needs that is not a step:
where the code lives, what the surrounding machinery does, which decisions are
already settled, and which observations were verified against source rather
than assumed.

Line numbers are as of commit `01ab457` (branch `task-021-downgraded-variant-cache-control`).

---

## 1. The change in one paragraph

`Processor.Content` serves thumbnail bytes when a `?variant=card` request finds
no card, and the response it returns is byte-for-byte indistinguishable from a
genuine `?variant=thumbnail` response. The handler therefore stamps it with the
same `Cache-Control: private, max-age=300` as everything else, and the browser
stores the soft image under the sharp image's URL for five minutes — long after
the card generation the downgrade schedules has completed. The fix is to stop
discarding a fact the producer already has: `ContentInfo` gains `Downgraded
bool`, `Content` sets it at the one substituting call site, and the handler
serves those responses `private, no-store`. Three lines of production code.

## 2. Files that matter

All under `apps/media-service/internal/mediaobject/`.

| File | Why it matters |
|---|---|
| `processor.go` | `ContentInfo` at `:110-131`; `Processor.Content` at `:392-420`; `openVariant` at `:427-470`. The two edits are here. |
| `resource.go` | The `GET /media/{id}/content` handler at `:156-215`. `proc.Content` is called at `:164` — the **only** call site in the service. `Cache-Control` is set at `:207-208`. |
| `processor_test.go` | The downgrade matrix at `:891`; present-card at `:1016`; the fault case at `:1046`; explicit-thumbnail at `:696`; original at `:667`. All fixtures needed already exist. |
| `resource_test.go` | `thumbnailRouter` fixture at `:697`; the three `private, max-age=300` regression guards at `:313`, `:737`, `:764`. |
| `contentvariant.go` | `ParseContentVariant` at `:24-39`. Relevant because `""` maps to `ContentOriginal`. |

**Not touched, deliberately:** `internal/processing/card.go` (the generator and
its saturation drop), everything under `apps/web/`, all deployment manifests.

## 3. How the downgrade path actually works

Read `Processor.Content` top to bottom:

1. `GetByID(id, identityFleetID)` — fleet-scoping gate, returns 404 cross-fleet.
2. `want == ContentOriginal` → `openOriginal`, returns early. A request with no
   `?variant=` parameter lands here, because `ParseContentVariant("")` returns
   `ContentOriginal`.
3. `openVariant(ctx, m, want)` — success returns early with the variant's info.
4. `want != ContentCard || !errors.Is(err, server.ErrNotFound)` → return the
   error. This is what stops the downgrade generalising to `display`, and what
   stops a fault hiding behind a thumbnail.
5. `scheduleCard(m)` — nil-safe via `nopCardGenerator`, gated on `StatusReady` +
   `ClassImage` inside `scheduleCard` itself.
6. `Debug` log — retained verbatim.
7. `openVariant(ctx, m, ContentThumbnail)` — its result is currently returned
   directly. This is the return the plan rewrites.

`openVariant` maps **both** "no row" and "row present, object missing from the
store" to `server.ErrNotFound` (`:436-440` and `:465-470`). That is why store
drift on the card key downgrades and therefore sets the flag: a substitution
genuinely occurred, and the existing test at `processor_test.go:969` already
covers the path.

## 4. Settled decisions — do not relitigate

From design §2. Each was considered and rejected with a reason:

- **Not a fourth return value** `(ContentInfo, io.ReadCloser, bool, error)` —
  every call site pays for a fact one branch produces, and the boolean is
  positionally unlabelled.
- **Not a sentinel error** — overloads the error channel with something that is
  not a failure.
- **Not re-derived in the handler** — impossible, not merely fragile. A card and
  a thumbnail are both `image/jpeg`, both have `Size == 0`, and both build the
  same disposition. There is nothing to compare against.
- **Not a `context` value** — invisible coupling for data with an obvious home.
- **Not `ContentInfo.CacheControl string`** — puts an HTTP header value in a
  domain type; the processor would be making a transport policy decision, and a
  second transport would inherit a field that means nothing to it. This is
  `CLAUDE.md`'s service-boundary rule applied to data rather than calls.
- **Not a `cacheControlFor(bool) string` helper** — premature at one branch and
  one caller. Extract it if a third policy appears.
- **Not a `downgraded bool` parameter threaded into `openVariant`** — this is the
  load-bearing decision (design D2). `openVariant` opens the rendition it was
  asked for, so from where it stands nothing was substituted. Pushing the fact
  down would make an explicit `?variant=thumbnail` and a downgraded response
  indistinguishable at exactly the layer that cannot tell them apart — the bug
  this task fixes, reintroduced one function lower.
- **`no-store`, not a short `max-age`** — no magic number to tune, and the thing
  being cached is by definition the wrong image. `no-cache` with revalidation
  needs `ETag`, which media-service does not emit for variants.
- **`private, no-store`, not bare `no-store`** — `private` is the statement that
  these are per-fleet authorized bytes. Keeping it means the two policies differ
  in exactly one token.

## 5. Verified against source (do not re-derive)

These were checked while planning; they are why the plan says what it says.

- **`Content` has exactly one caller** — `resource.go:164`. `grep -rn
  'proc.Content\|\.Content(ctx' apps/media-service/` confirms it. Blast radius of
  the signature-preserving change is one handler.
- **`info`, `rc`, `err` are already declared** in `Content` by `info, rc, err :=
  pr.openVariant(ctx, m, want)`, so Task 1 Step 6's rewrite uses `=`, not `:=`.
- **`ContentInfo` must stay comparable** — `processor_test.go:763` does `if info
  != (ContentInfo{})`. A `bool` field keeps that compiling; a slice or map field
  would not.
- **`openVariant`'s success return is a composite literal** (`:459-462`), which
  is what makes Task 1 Step 8's inversion (temporarily setting `Downgraded: true`
  there) a one-line edit.
- **`thumbnailRouter` seeds no `card` ref** (`resource_test.go:707-710` — only
  `thumbnail` and `display`), so `?variant=card` against it already downgrades.
  No new fixture is needed for the handler test.
- **No handler test currently exercises `?variant=card`** — `grep -n
  'variant=card' resource_test.go` returns nothing. The new test is genuinely
  new coverage, not a rewrite.
- **A downgraded response's disposition is `inline; filename="photo.png"`** —
  `seedStoredObject` uploads `photo.png` as `image/png`, and `ContentDisposition`
  is built from the *class* the thumbnail's `image/jpeg` resolves to plus the
  model's original filename. `TestGetContent_variantIsHardenedLikeTheOriginal`
  asserts the same value at `:662`.
- **`ParseContentVariant("")` returns `ContentOriginal`** (`contentvariant.go:26-27`),
  so "no `variant` parameter" and "`?variant=original`" are the same processor
  path. Writing two processor tests for them would be duplicate coverage; the
  distinct handler-level case is already covered by
  `TestGetContent_noVariantParamIsUnchanged`.
- **The frontend needs no change** — the `QueryClient` is built in a `useState`
  initializer in `apps/web/src/components/providers/AppProviders.tsx` with no
  persister, so React Query's cache is in-memory per app instance. A full page
  load starts empty and issues a real `fetch`; what decides whether that fetch
  reaches the network is the browser HTTP cache, which is exactly what
  `no-store` controls. (Issue #29 cites `apps/web/src/lib/providers/AppProviders.tsx`
  — that path does not exist.)

## 6. Build and test commands

The repo is per-service Go modules under a root `go.work`; `make` targets use
fully-qualified package paths, so run everything from the worktree root and
never `cd` into `apps/media-service`.

```bash
# One test
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run TestName -v
# The package
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -v
# The service
go test -race github.com/jtumidanski/myfleet/apps/media-service/...
# Everything CI runs: lint-check vet test build fe-test fe-build manifests carfax-template
make ci
```

`make ci` includes `fe-test` and `fe-build`, so Node must be loaded first — it is
not always on `PATH`:

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

No deployment manifest changes in this task, so no separate `kustomize` render or
`kubectl --dry-run=server` pass is needed beyond the `manifests` target `make ci`
already runs.

## 7. Project rules that bite on this task

- **Prove tests can fail.** Revert the production change, confirm red, restore.
  Applies to every new assertion. A compile error does not count as red for a
  positive assertion — it would have appeared regardless of what was asserted.
  This is why Task 1 Step 8 does two inversions, not one.
- **Assert on stored state, not on the shape of a response** — here the
  processor tests assert the returned `ContentInfo`, and the handler test asserts
  the wire headers. Both layers, separately.
- **Code review before PR is mandatory**, even when the plan looks complete
  (`CLAUDE.md`). `plan-adherence-reviewer` + `backend-guidelines-reviewer`; no
  frontend reviewer, since no TS/React file changes.
- **Worktree discipline** — all work happens in
  `.worktrees/task-021-downgraded-variant-cache-control`. Never edit the main
  checkout. Never use bare `git stash`/`git stash pop`; the stash stack is shared
  with every other worktree.

## 8. Known unrelated issue in this workspace

A SessionStart hook reports a **task-number collision on 022**:
`task-022-purge-variant-cleanup` and `task-022-signout-failure-handling` both
claim the number. It does not affect task-021 and is not this task's work, but it
will keep surfacing on every phase command until someone renumbers one of them
(`tools/task-numbers.sh list`).

## 9. Deferred follow-ups

Named in the PRD, deliberately not in this plan:

- **§9.3 — the generation cap drops rather than queues.** A twelve-card cold grid
  schedules roughly four generations per visit because `CardGenerator.Generate`
  drops on semaphore saturation. A small bounded queue would converge faster, but
  it changes a load-shedding policy that protects the pod under read traffic and
  deserves its own risk assessment.
- **§9.4 — no backfill.** Generating `card` for all pre-existing media in one
  sweep would eliminate the lazy-fill window entirely and make both this task and
  §9.3 largely moot for the existing corpus. Materially larger: migration tooling,
  throughput control, failure handling.
- **The `X-Media-Variant-Downgraded` header.** Revisit only if the
  `Cache-Control` change alone proves insufficient in practice.
