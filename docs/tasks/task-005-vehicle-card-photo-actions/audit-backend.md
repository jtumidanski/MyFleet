# Backend Audit — media-service + shared-go (task-005-vehicle-card-photo-actions)

- **Scope:** Go changes on `task-005-vehicle-card-photo-actions` (`352e8c1..84b025e`)
- **Packages audited:** `apps/media-service/internal/mediaobject`, `apps/media-service/internal/mediavariant`, `apps/media-service/cmd`, `packages/shared-go/server`
- **Guidelines Source:** `backend-dev-guidelines` skill
- **Date:** 2026-07-31
- **Build:** PASS
- **Tests:** changed packages PASS (3 packages, 0 failed)
- **Overall:** NEEDS-WORK

## Build & Test Results

```
cd apps/media-service && go build ./...   -> BUILD_OK
cd apps/media-service && go vet ./...     -> VET_OK
go test ./internal/mediaobject/... ./internal/mediavariant/... -count=1
  ok  .../internal/mediaobject    0.031s
  ok  .../internal/mediavariant   0.006s
cd packages/shared-go && go build ./... && go test ./server/... -count=1
  ok  .../packages/shared-go/server  0.004s
```

Full suite not re-run (`make ci` was independently confirmed green).

## Design-Intent Verification

| Intent | Status | Evidence |
|--------|--------|----------|
| `mediaobject` never imports `mediavariant` | PASS | `go list -deps ./internal/mediaobject` returns only `internal/storage` + `internal/mediaobject`. `grep -rn mediavariant internal/mediaobject/` matches comments only (`processor.go:53`, `contentvariant.go:6`, `processor_test.go:363`). Port declared at `processor.go:60-62`, adapted at `cmd/main.go:162-170`. |
| Object resolved + fleet-scoped BEFORE any variant lookup or store read; cross-fleet is 404 | PASS | `processor.go:263` calls `GetByID` (→ `getActive` + `AuthorizeAccess`, `processor.go:82-87`) before the `want != ContentOriginal` block at `processor.go:268`. Asserted at `processor_test.go:328-348` (lookup calls == 0, store calls == 0) and `resource_test.go:556-578` (HTTP 404). |
| Lookup **error** → 500; **miss** → fallback | PASS | Error returned unmasked `processor.go:270-275`; miss logged Debug + falls through `processor.go:277-279`; asserted `processor_test.go:353-365`, `processor_test.go:266-290`. |
| Variant row whose object is missing → fallback, logged warn | PASS | `processor.go:294-298`; asserted `processor_test.go:296-323` (store called twice: variant key then original). |
| A served variant carries **no** `Content-Length` | PASS | `processor.go:293` returns `ContentInfo{ContentType, Served}` with `Size` left zero; handler gates the header on `info.Size > 0` at `resource.go:182-184`. Asserted `resource_test.go:465-487`. |
| Unknown `?variant=` → 400, exact lowercase match | PASS | `contentvariant.go:23-35` (`default: return "", server.ErrBadRequest`); `resource.go:156-160` rejects before touching the processor. Asserted `contentvariant_test.go:33-38` (`Thumbnail`, `THUMBNAIL`, `bogus`, `small`, `" thumbnail"`) and `resource_test.go:531-551` (400 + `bad_request` + zero store calls). |
| 400/413 status + code plumbing | PASS | `server/errors.go:6` (`ErrBadRequest`), `errors.go:18-19` (`StatusFor` → 400), `server/server.go:15-16` (`bad_request`), `server.go:26-27` (`payload_too_large`); pinned by `errors_test.go:26-42`. |
| `ListByMediaObject` retained | Not a finding (per instruction) | `mediavariant/provider.go:23-33` |

## Domain Checklist — `mediaobject`

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `internal/mediaobject/builder.go:38-46` — `Build()` validates fleetID/uploadedByUserID/bucket/objectKey |
| DOM-02 | `ToEntity()` | PASS | `internal/mediaobject/entity.go:48` |
| DOM-03 | `Make(Entity)` | WARN | `entity.go:30` — exists but signature is `Make(Entity) Model`, not `(Model, error)`. Pre-existing, service-wide convention; untouched by this branch |
| DOM-04 | `Transform` | PASS | `internal/mediaobject/rest.go:18` |
| DOM-05 | `TransformSlice` | PASS | `rest.go:36` — exists; service has no list endpoint, so no inline loop exists to violate |
| DOM-06 | Processor takes `FieldLogger` | PASS | `processor.go:119` — `NewProcessor(log logrus.FieldLogger, ...)`; field at `processor.go:112` |
| DOM-07 | Handlers pass request logger, never `StandardLogger()` | PASS (equivalent) | `resource.go:44-45` — the injected `logrus.FieldLogger` is threaded into the processor at route-init. `grep -rn "logrus.StandardLogger" apps/media-service` → zero matches. This codebase's `server` package has no `HandlerDependency`/`d.Logger()` |
| DOM-08 | POST/PATCH use `RegisterInputHandler[T]` | WARN | `resource.go:48` uses `server.RegisterInputHandler` for `POST /media`. `POST /media/{id}/confirm` at `resource.go:118` is a raw `r.Post` — it carries no request body, and it is pre-existing/untouched by this branch |
| DOM-09 | Transform errors handled | PASS (N/A) | `rest.go:18` — `Transform` returns a single value; there is no error to discard. No `_, _ :=` in `resource.go` |
| DOM-10 | Lazy providers | WARN (pre-existing) | `mediaobject/provider.go:24,35` execute eagerly and return concrete values rather than `database.Provider[T]`. `database.Query` exists (`packages/shared-go/database/query.go:6`) and is used elsewhere (`apps/auth-service/internal/user/provider.go:35`). Not touched by this branch — see the FAIL on `mediavariant` below for the new code |
| DOM-11 | No `os.Getenv()` in handlers | PASS | `grep -rn "os.Getenv" apps/media-service` → zero matches |
| DOM-12 | No cross-domain logic in handlers | PASS | `resource.go:153-193` calls only `ParseContentVariant` + `proc.Content`; the cross-domain reach is a port resolved inside the processor at `processor.go:269` and bound in the composition root at `cmd/main.go:125` |
| DOM-13 | Handlers don't call providers | PASS | `resource.go` references `NewProvider` only at `:45` to construct the processor; all handler bodies call `proc.*` |
| DOM-14 | No direct entity writes in handlers | PASS | `grep -n "db.Create\|db.Save\|db.Delete" internal/mediaobject/resource.go` → zero matches |
| DOM-15 | `administrator.go` exists | PASS | `internal/mediaobject/administrator.go` present; writes reached via `processor.go:142,188,226,342` |
| DOM-16 | Domain error → HTTP status | PASS | `server.WriteError` at `resource.go:158,163` → `server/jsonapi.go:29-37` → `StatusFor` (`errors.go:16-37`): 400 `contentvariant.go:34`, 404 `processor.go:321,367,376`, 409 `processor.go:93,103,183`, 410 `processor.go:373`, 413 `resource.go:89` |
| DOM-17 | JSON:API interface on REST models | WARN (pre-existing) | `rest.go:19-21` uses `server.Resource{Type, ID, Attributes}` instead of `GetName()/GetID()/SetID()`. Service-wide convention; untouched |
| DOM-18 | Flat request models | PASS | `resource.go:48-51` — flat anonymous struct; the `{data:{attributes}}` envelope is unwrapped by `server/handler.go:42-53` |
| DOM-19 | Table-driven tests | WARN | `grep -rn "t.Run" internal/mediaobject internal/mediavariant` → zero matches. The new tests are one func per behaviour (`processor_test.go:199,231,266,296,328,353`; `resource_test.go:438,465,489,510,531,556`). `contentvariant_test.go:15-38` is map-driven but without subtests |

## Domain Checklist — `mediavariant`

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `internal/mediavariant/builder.go:27-32` — validates mediaObjectID/variant/objectKey |
| DOM-02 | `ToEntity()` | PASS | `internal/mediavariant/entity.go:40` |
| DOM-03 | `Make(Entity)` | WARN | `entity.go:26` — `Make(Entity) Model`, no error return (same service-wide deviation) |
| DOM-04/05 | `Transform` / `TransformSlice` | N/A | No REST surface — package has no `rest.go` or `resource.go` |
| DOM-06/07 | Processor logger | N/A | No `processor.go`; writes are driven by `internal/processing/worker.go` |
| DOM-10 | Lazy providers | **FAIL** | `provider.go:35-45` — the new `GetByMediaObjectAndVariant` executes `p.db...First(&e)` eagerly and returns `(Model, bool, error)`. Guidelines mandate `database.Query[T]` (`patterns-provider.md`, `file-responsibilities.md` §provider.go); the helper exists at `packages/shared-go/database/query.go:6` and is used at `apps/auth-service/internal/user/provider.go:35`. Additionally the query uses bare `p.db` with no `WithContext` (see Important #2) |
| DOM-11/12/13/14 | Handler rules | N/A | No handlers in this package |
| DOM-15 | `administrator.go` exists | PASS | `internal/mediavariant/administrator.go` |
| DOM-16/17/18 | REST rules | N/A | No REST surface |
| DOM-19 | Table-driven tests | WARN | `provider_test.go:60-77` and `:83-95` are two named funcs; no `t.Run`, no case table |

## Support Packages

- `apps/media-service/internal/storage` — MinIO client; `GetObject` eagerly probes so a nil error genuinely means readable (`minio.go:176-194`), which is what lets `resource.go:187` commit a 200. Unchanged this branch.
- `apps/media-service/internal/processing`, `internal/processedevents` — worker/idempotency; unchanged this branch.
- `packages/shared-go/server` — shared transport helpers; changes limited to `errors.go` + `server.go` status/code additions, both test-pinned.

## Security Review

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | JWT validated, not `ParseUnverified` | PASS (unchanged) | `cmd/main.go:123` — `authmw.JWT(keyfn)` with the JWKS keyfunc built at `mustJWKSKeyfunc` (`main.go:174+`). No `ParseUnverified` in the diff |
| SEC-02 | Revocation on validated tokens | N/A | media-service issues/revokes no tokens |
| SEC-03 | No open redirect / no untrusted path construction | PASS | The `?variant=` value never reaches a URL or an object key: it is matched against a closed set at `contentvariant.go:24-35`, converted to `mediavariant.Variant` for a parameterised `WHERE` at `provider.go:37`, and the key actually read comes from the persisted row (`cmd/main.go:169` → `processor.go:281`). Path traversal via `?variant=../` is a 400 |
| SEC-04 | No hardcoded secrets | PASS | `cmd/main.go:49` — `config.MustGet("MINIO_SECRET_KEY")`; grep for secret/password/api_key across the changed files finds no literals |
| SEC-05 | Fleet isolation on the new path | PASS | Variant lookup is keyed by the already-authorized media object id (`processor.go:269`), and authorization precedes it (`processor.go:263`). 404, never 403 — `processor.go:82-87` |

## Summary

### Blocking (must fix)

- **DOM-10 — `apps/media-service/internal/mediavariant/provider.go:35-45`:** the new `GetByMediaObjectAndVariant` is an eager query returning a concrete `(Model, bool, error)` instead of a lazy `database.Query[T]` provider. The guidelines list eager provider execution as an anti-pattern, and the helper is available (`packages/shared-go/database/query.go:6`, in use at `apps/auth-service/internal/user/provider.go:35`).

### Important (should fix)

1. **A variant-lookup failure returns 500 with no server-side log — `apps/media-service/internal/mediaobject/resource.go:161-165`.** The design deliberately propagates a `Lookup` error rather than falling back, on the grounds that "serving the original instead would hide a real fault" (`processor.go:271-273`). But the handler writes the error straight to the client with no `log.WithError(...)`, and there is no request/error-logging middleware to catch it: `main.go:120-132` installs only `telemetry.CorrelationID`, and `packages/shared-go/server/handler.go:17-19` adds none. The sibling handlers in the same file do log (`resource.go:64`, `resource.go:110`). Net effect: the fault the design refuses to mask is invisible to operators anyway.
2. **The `VariantLookup` port carries no `context.Context` and the query drops it — `processor.go:61`, `mediavariant/provider.go:37`.** `Lookup(mediaObjectID, variant string)` has no ctx parameter, so `Content` (which holds one, `processor.go:262`) cannot pass it, and the implementation queries with bare `p.db`. `file-responsibilities.md` §provider.go requires `db.WithContext(ctx)`, and the testing guide's pre-commit checklist repeats it verbatim. The port is described as "the same shape as ObjectStore" (`processor.go:52`), but `ObjectStore` does take a ctx on every method (`processor.go:37-38`). Consequence: the variant read on the content hot path is not cancelled when the client disconnects and carries no correlation.

### Non-Blocking (minor)

3. **`ContentInfo.Served` has no production reader — `processor.go:76`.** Set at `processor.go:293` and `:328`, read only in `processor_test.go:644,682,705,741`. `anti-patterns.md` lists "leaving dead code after refactoring".
4. **A fallback is cached under the variant URL — `resource.go:186`.** A `?variant=thumbnail` request that falls back to the original still returns `Cache-Control: private, max-age=300`, indistinguishable from a real variant, so a client holds the original for up to 5 minutes after the worker finishes. `Served` (#3) is precisely the signal that could shorten or vary that directive and is unused.
5. **`ref.ObjectKey == ""` is unguarded — `processor.go:281`,** while `ref.ContentType == ""` is guarded at `:285-290`. An empty key reaches `storage.GetObject`, which will not answer `ErrObjectNotFound`, so it becomes a 500 rather than the fallback the design mandates for anything unservable.
6. **The composition-root adapter is untested — `cmd/main.go:162-170`.** `apps/media-service/cmd/` contains only `main.go`. Every test substitutes a hand-written `fakeVariants` (`processor_test.go:42-55`), so the real `mediavariant.Variant(variant)` conversion — which silently couples `mediaobject`'s constants (`contentvariant.go:11-14`) to `mediavariant`'s (`model.go:9-11`) with no compile-time link — is exercised by nothing. The two sets do match today (both `"thumbnail"`/`"display"`), but drift would compile, degrade every variant request to the original, and announce itself only through a Debug-level log (`processor.go:278`).
7. **DOM-19 — no table-driven tests in either changed package.** `grep -rn "t.Run"` over `internal/mediaobject` and `internal/mediavariant` returns zero matches; the guidelines ask for `tests := []struct{...}` with `t.Run`.
8. **Pre-existing, carried forward (not introduced here):** `Make(Entity) Model` without an error return (`mediaobject/entity.go:30`, `mediavariant/entity.go:26`); `server.Resource` in place of the `GetName()/GetID()/SetID()` JSON:API interface (`mediaobject/rest.go:19`); `POST /media/{id}/confirm` registered as a raw `r.Post` (`resource.go:118`); `mediaobject/provider.go:24,35` eager providers. These are service-wide conventions predating this branch.
