# task-005 — Code Review Record

Consolidated index for the review of `task-005-vehicle-card-photo-actions`.
Four reviewers audited the branch in parallel after all twelve plan tasks
landed; every finding was ruled on by the human partner, fixed, and re-reviewed.

| Report | Reviewer | Verdict as first issued |
| --- | --- | --- |
| [audit-plan-adherence.md](./audit-plan-adherence.md) | `plan-adherence-reviewer` | PASS — 12/12 tasks, 88/88 steps, no silent gaps |
| [audit-backend.md](./audit-backend.md) | `backend-guidelines-reviewer` | NEEDS-WORK — 1 blocking, 2 important |
| [audit-frontend.md](./audit-frontend.md) | `frontend-guidelines-reviewer` | NEEDS-WORK — 3 important |
| [audit-whole-branch.md](./audit-whole-branch.md) | whole-branch reviewer | Ready to merge — 1 important, 4 minor |
| [audit-fix-wave.md](./audit-fix-wave.md) | fix implementer | All 13 rulings applied |

**Outcome: all 13 findings ADDRESSED**, verified by a scoped re-review that
mutation-tested the claims it doubted rather than trusting them — reverting
each fix was confirmed to fail the test that pins it. No new breakage was
introduced by the fix diff. `make ci` passes end to end and both
`kubectl apply --dry-run=server` runs are green against `bee`.

## Rulings where project guidelines overrode the plan

Three findings sat in code `plan.md` specifies verbatim. The human partner
ruled that guidelines and correctness govern, so the plan text was overridden:

- **DOM-10** — `mediavariant.GetByMediaObjectAndVariant` was an eager
  `(Model, bool, error)` query; converted to the lazy `database.Query[T]`
  provider, following `auth-service/internal/user/provider.go`.
- **Context propagation** — `VariantLookup.Lookup` took no `context.Context`,
  so the query ran on a bare `*gorm.DB` despite the port's own comment claiming
  it mirrored `ObjectStore`. `ctx` is now threaded through to
  `db.WithContext(ctx)`.
- **Blocking bootstrap** — `main.tsx` gated `createRoot().render()` on the
  config fetch, so a wedged `/config/config.json` meant a blank page for the
  full 2s timeout on every route. The app now mounts immediately and the config
  is observed via `useSyncExternalStore`, so a value arriving after mount still
  reaches the UI.

## The variant fallback became a 404

The largest behavioural change, and a reversal of a plan decision. Previously a
requested `thumbnail` with no variant row fell back to the **full original**
(up to 25 MiB), so a twelve-card grid could retain ~300 MB of blobs — and
permanently so for undecodable media, because the worker aborts the whole
message on a `generateVariant` error.

`GET /media/{id}/content?variant=thumbnail|display` now returns **404** when the
variant cannot be served. The card already renders its neutral placeholder on
error, so no client change was needed.

The whole-branch reviewer originally proposed surfacing `ContentInfo.Served` as
a response header instead. That was rejected on cost: `apiClient.requestBlob`
returns a bare `Blob` and discards the response, so the header approach would
have meant changing a package every service depends on and re-shaping the
hook's fenced-off object-URL effect.

**Unchanged and non-negotiable:** a request with no `variant` parameter, and
`?variant=original`, still serve the original bytes with their `Content-Length`.
`resource_test.go`'s `TestGetContent_originalIsUnaffectedByAnUnservableVariant`
pins both against the same object that 404s for a thumbnail.

## Still owed — needs a browser

No browser was available in this session, so these were reported as
**NOT VERIFIED** rather than assumed. They are the only outstanding items:

1. The card's image request is `?variant=thumbnail`, returns 200 with
   `Cache-Control: private, max-age=300` and no `Content-Length`.
2. A request with no `variant` still returns the original with its
   `Content-Length`.
3. `?variant=bogus` returns 400 with `"code": "bad_request"`.
4. `GET /config/config.json` returns 200 with `Cache-Control: no-cache`.
5. No request to any `carfax.com` host before the button is clicked.
6. No horizontal overflow at the single-column breakpoint; cards align across a
   row with and without photos; both buttons reachable by Tab with a focus ring.
7. **Skeleton height** — `h-40` (160px) is computed against a card of 166px
   (2 border + 32 `p-4` + 80 thumbnail + 12 `mt-3` + 40 button); Tailwind has no
   step between 160 and 176. Never observed in a browser.
8. **Grid density** — whether `lg:grid-cols-3` still reads well at the taller
   card height (PRD §9.4). The class was deliberately left unchanged.
9. **PRD criterion #24** — the action row is `justify-end`, so a card with one
   button hugs the right edge while a card with two places the detail button
   left of Carfax. Whether that satisfies "same position" is a visual judgement.

## PRD corrections carried

`prd.md` §10 still contradicts the shipped code in two places, per designs D1
and D3. The code is correct; the PRD was not amended:

- **Cross-fleet reads** are `404`, not the PRD's `403`. `403` would confirm that
  an id exists in another fleet — the existence oracle `AuthorizeAccess` exists
  to prevent.
- **Criterion #29** asks that `VITE_CARFAX_URL_TEMPLATE` change the URL at build
  time. Runtime config replaced it: editing the `web-config` ConfigMap needs no
  rebuild, which is strictly stronger but answers a different question.
