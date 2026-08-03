# Backend Audit — media-service (task-021-downgraded-variant-cache-control)

- **Service Path:** apps/media-service
- **Scope:** Diff only — `d3c9eaaf949e6b5dfb439379e990a30007eee2f1..48c86c2`, package `apps/media-service/internal/mediaobject`
- **Guidelines Source:** backend-dev-guidelines skill
- **Date:** 2026-08-03
- **Build:** PASS
- **Tests:** all packages pass (see below)
- **Overall:** PASS

## Build & Test Results

```
$ go build ./apps/media-service/...
(no output — success)

$ go test ./apps/media-service/... -count=1
ok  	github.com/jtumidanski/myfleet/apps/media-service/cmd	0.017s
ok  	github.com/jtumidanski/myfleet/apps/media-service/internal/admin	0.012s
ok  	github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject	0.051s
ok  	github.com/jtumidanski/myfleet/apps/media-service/internal/mediavariant	0.021s
ok  	github.com/jtumidanski/myfleet/apps/media-service/internal/processedevents	0.005s
ok  	github.com/jtumidanski/myfleet/apps/media-service/internal/processing	1.187s
ok  	github.com/jtumidanski/myfleet/apps/media-service/internal/storage	0.019s
ok  	github.com/jtumidanski/myfleet/apps/media-service/internal/variantfailures	0.006s
```

## Scope Note

`mediaobject` has `model.go`/`entity.go`/`builder.go` and is therefore a domain
package, but this audit does not re-run the full DOM-01..DOM-19 mechanical
checklist against the whole package — the task instructions scope this review
to the four changed files (`processor.go`, `processor_test.go`, `resource.go`,
`resource_test.go`) and the specific design questions raised in the task brief
(SEC boundary/cache-control correctness, domain/transport separation). Checks
below are the ones the diff actually touches or that the task brief calls out
by name.

## Diff-Scoped Checklist Results

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-12 | No cross-domain/business logic in handler | PASS | `apps/media-service/internal/mediaobject/resource.go:208-219` — the handler's only new logic is a single `if info.Downgraded` branch selecting an HTTP cache directive; the fact itself (`Downgraded`) is computed entirely in `processor.go:441`, not derived from anything the handler inspects on its own. |
| DOM-13 | Handlers call processor only, not providers/DB | PASS | `apps/media-service/internal/mediaobject/resource.go:163` calls `proc.Content(...)`; no provider or `db.*` call was added to the diff. |
| DOM-14 | No direct entity creation/writes in handler | PASS | Diff to `resource.go` adds no `db.Create`/`db.Save`/`db.Delete` — confirmed via `git diff`, only a `Cache-Control` header computation was added (`resource.go:208-219`). |
| DOM-19 | Table-driven / behavior-verifying tests | PASS | `processor_test.go:685-1092` extends existing `t.Run`/table-style tests with `Downgraded` assertions on every call site (true/false and zero-value cases); `resource_test.go:769-810` adds `TestGetContent_downgradedCardIsNotStored`, a full header-set assertion (including a header-count check to catch any stray new header). |
| N/A | Transform/TransformSlice, RegisterInputHandler, os.Getenv, builder/entity checks | OUT OF SCOPE | Not touched by this diff; `mediaobject/resource.go` in this codebase does not use `server.RegisterHandler`/`RegisterInputHandler` or JSON:API `Transform` for this endpoint at all — it is a raw binary content stream registered via a `chi` router (`resource.go:155-219`), pre-existing and unrelated to this change. Flagging this pattern would relitigate a pre-existing architectural choice outside the diff, per task instructions. |

## Domain/Transport Boundary Assessment

The task brief's stated design intent: `ContentInfo.Downgraded` is a statement
of fact carrying no HTTP vocabulary; the handler owns the policy decision.

- **PASS** — `ContentInfo.Downgraded bool` (`processor.go:132`) is a plain
  bool with no HTTP-flavored type, constant, or string value anywhere near it.
  The doc comment at `processor.go:125-131` explicitly states the field
  "carries no HTTP vocabulary."
- **PASS** — The only two places that ever set `Downgraded = true` or leave it
  `false` are inside `Processor.Content` (`processor.go:441`) and the two
  `ContentInfo{...}` literals in `openOriginal` (`processor.go:552-556`) and
  `openVariant` (`processor.go:481-484`), neither of which reference it (so it
  defaults `false`). Per the task's stated (and here verified) design
  decision, `openVariant` deliberately never sets the flag — confirmed by
  reading the full body of `openVariant` (`processor.go:459-495`): no
  `Downgraded` assignment appears there. Only `Content` — the one call site
  that knows a substitution occurred — sets it, at `processor.go:441`, after
  the second `openVariant` call succeeds.
- **PASS** — The HTTP-vocabulary decision (`"private, no-store"` vs.
  `"private, max-age=300"`) is made exactly once, in the handler, at
  `resource.go:208-219`. No `cacheControlFor(bool)` helper was extracted — a
  single `if/else` with one caller, consistent with the task's noted settled
  decision not to extract one.
- **PASS (error-path safety)** — When the thumbnail fallback itself 404s, the
  function returns `ContentInfo{}, nil, err` (`processor.go:434`) *before* the
  `Downgraded = true` assignment on the following line group
  (`processor.go:441`), so a zero `ContentInfo` is never mistakenly stamped
  `Downgraded: true` and then discarded on an error path. `processor_test.go`
  asserts `info != (ContentInfo{})` on exactly this path (e.g.
  `processor_test.go` card+no-thumbnail case) to catch a regression here.

## Security Review (Cache-Control on per-fleet authorized bytes)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-CACHE-01 | `private` retained on both branches | PASS | `resource.go:209` (`"private, max-age=300"`) and `resource.go:217` (`"private, no-store"`) both start with `private`; the pre-existing comment at `resource.go:207` ("Per-fleet authorized bytes — never store in a shared cache") is preserved unchanged directly above the new conditional, and the diff's own added comment reiterates "`private` is unconditional; only the freshness half varies" (`resource.go:208`). Neither branch can produce a directive lacking `private`. |
| SEC-CACHE-02 | `no-store` is the correct directive for the downgraded case | PASS | `no-store` forbids any cache (private or shared, browser disk cache included) from persisting the response at all, which is strictly stronger than `private` alone (`private` still permits a browser to retain and reuse the bytes for up to `max-age`). Since the substituted thumbnail bytes are indistinguishable on the wire from a real card response (`resource_test.go:774-808` asserts the header/body set is otherwise byte-identical), any caching — even browser-private caching — would let a stale, soft-resolution image sit under the card URL until the browser's cache naturally evicts it, with nothing able to invalidate it early once the real card is generated seconds later. `no-store` is the only directive that prevents that persistence; `private, max-age=0` or `private, must-revalidate` would still permit a conditional-GET round trip. |
| SEC-CACHE-03 | No new information disclosure / oracle introduced | PASS | The `Downgraded` fact never reaches the response body or any header value directly (only a coarser Cache-Control string), so a client cannot distinguish "explicitly requested thumbnail" from "downgraded card" by inspecting the response — this is asserted directly by `resource_test.go:774-808`, which checks the exact 4-header set and fails if a 5th header (e.g., an `X-Downgraded` marker) were ever added. |

## Summary

### Blocking (must fix)
- None.

### Non-Blocking (should fix)
- None identified within the diff scope. The pre-existing `mediaobject`
  resource layer's use of a raw `chi` router and hand-rolled handlers (rather
  than `server.RegisterHandler`/`RegisterInputHandler` and JSON:API
  `Transform`) is a deviation from the standard DOM-04/05/08/17 scaffolding
  pattern, but it predates this diff, is unchanged by it, and is out of scope
  per the task's explicit focus on the four listed files and the
  cache-control/domain-boundary questions.
