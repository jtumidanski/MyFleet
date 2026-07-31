# Backend Audit — media-service (+ shared-go/server)

- **Scope:** Go diff `16e4aa7..a8d5221` on `task-002-k3s-cluster-deployment`
- **Packages:** `apps/media-service/internal/mediaobject`, `apps/media-service/internal/storage`, `apps/media-service/cmd`, `packages/shared-go/server`
- **Guidelines Source:** backend-dev-guidelines skill
- **Date:** 2026-07-30
- **Build:** PASS
- **Vet:** PASS
- **Tests:** PASS (all packages ok, `-race`)
- **Overall:** NEEDS-WORK — 2 blocking defects

## Build & Test Results

```
$ go build github.com/jtumidanski/myfleet/...      # exit 0, no output
$ go vet   github.com/jtumidanski/myfleet/...      # exit 0, no output
$ go test -race github.com/jtumidanski/myfleet/... # all ok / [no test files]
ok  github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject
ok  github.com/jtumidanski/myfleet/apps/media-service/internal/storage
ok  github.com/jtumidanski/myfleet/packages/shared-go/server
```

The objective gate passes. Both blocking findings below are behaviours the test suite
does not exercise, because both new tests that would cover them substitute a fake for
the real MinIO SDK.

---

## BLOCKING

### B1 — Unknown-length upload allocates 528 MiB into a 256 MiB pod; single request OOM-kills media-service

The `size = -1` handling is not merely a "judgement call" — it is a remote pod kill.

Chain of evidence:

| Step | Location | Fact |
|---|---|---|
| 1 | `apps/media-service/internal/mediaobject/resource.go:86-89` | `size := req.ContentLength; if size < 0 \|\| size > maxUploadBytes { size = -1 }` |
| 2 | `apps/media-service/internal/mediaobject/processor.go:143` | `size` passed through to `PutObject` unchanged |
| 3 | `apps/media-service/internal/storage/minio.go:107-110` | options are `minio.PutObjectOptions{ContentType: contentType}` → `PartSize: 0` |
| 4 | minio-go v7.1.0 `api-put-object.go:366-373` | `size < 0` → `putObjectMultipartStreamNoLength` |
| 5 | minio-go v7.1.0 `api-put-object.go:398`, `:423` | `OptimalPartInfo(-1, 0)` then `buf := make([]byte, partSize)` |
| 6 | measured | `OptimalPartInfo(-1, 0)` → `partSize = 553648128` bytes = **528.0 MiB**, totalParts 9930 |
| 7 | `deploy/k8s/base/media-service/deployment.yaml:56` | pod memory **limit: 256Mi** |

Measurement was taken by running the real exported function against the real module:

```
UNKNOWN SIZE (-1, partSize=0): totalParts=9930 partSize=553648128 bytes (528.0 MiB) lastPart=385875968 err=<nil>
```

The allocation happens **after `newUploadID` and before the read loop** — i.e. before a
single body byte is consumed. `http.MaxBytesReader` bounds *bytes read*; it does nothing
about a buffer allocated up front. 528 MiB requested against a 256 MiB cgroup limit is an
immediate OOMKill.

**Two trigger paths, neither of them exotic:**

1. Any request with **no `Content-Length`** (chunked transfer encoding) → `req.ContentLength == -1` → `size = -1`. This is a legitimate client shape, not an attack.
2. Any request advertising **`Content-Length` greater than the cap** → the branch's own line at `resource.go:87` deliberately converts it to `-1`. So the defensive branch added to handle oversized uploads is precisely what turns a request that should cost a cheap 413 into a pod kill. A 1-byte body with `Content-Length: 99999999` is sufficient.

**Reachability:** `deploy/k8s/overlays/main/ingressroute.yaml:85` routes
`Host(myfleet.tumidanski.com) && PathPrefix(/api/media)` to the service from the public
internet. `deploy/k8s/base/routing/middlewares.yaml` defines only four `stripprefix`
middlewares — there is no `buffering` middleware and no `maxRequestBodyBytes` anywhere,
so nothing caps or normalizes the body at the gateway. Requires only an authenticated
member/owner token (`resource.go:77`).

**Why tests miss it:** `TestStoreContent_unknownLengthRecordsActualByteCount`
(`processor_test.go`) is the one test that drives `size = -1`, and it asserts against
`fakeStore` (`processor_test.go`, `PutObject` doing `io.ReadAll`). The fake never
allocates, so the test passes while the production path is fatal.

**Fix (both halves needed):**
- In `resource.go`, reject early rather than downgrading to `-1`: if
  `req.ContentLength > maxUploadBytes`, return `server.ErrRequestEntityTooLarge`
  before touching the body.
- In `storage.PutObject` (`minio.go:107-110`), set an explicit `PartSize` in
  `minio.PutObjectOptions` so the genuinely-chunked path is bounded. Minimum accepted is
  5 MiB (`absMinPartSize`); 16 MiB matches the SDK's `minPartSize`. Note that even 16 MiB
  × concurrent uploads needs to be weighed against the 256 Mi limit.

### B2 — `GET /media/{id}/content` returns **200 with an empty body** when the object has no bytes in MinIO

`minio.Client.GetObject` is lazy. `api-get-object.go:32-55` performs bucket/object *name*
validation only, then hands off to a goroutine; the actual `c.getObject` call is issued on
the **first read request** (`api-get-object.go:98-100`). A key that does not exist
therefore returns `err == nil`.

- `apps/media-service/internal/storage/minio.go:114-120` — returns that error verbatim, so nil.
- `apps/media-service/internal/mediaobject/processor.go:213-217` — `Content` treats nil as success and returns the `ReadCloser`.
- `apps/media-service/internal/mediaobject/resource.go:144-157` — headers and `w.WriteHeader(http.StatusOK)` are committed **before** the first byte is read; the subsequent `io.Copy` failure is only `log.Warn` ("headers are already written, so the status cannot be changed").

**This is reachable through the ordinary API, not an edge case.** `POST /media` creates the
row in `uploaded` with `size = 0` and writes nothing to MinIO
(`processor.go:84-105`). If the client then never PUTs the content — or the PUT failed,
which `StoreContent` leaves in exactly this state by returning before `Update`
(`processor.go:143-145`, asserted by
`TestStoreContent_storeFailurePropagatesAndLeavesSizeUntouched`) — a subsequent
`GET /media/{id}/content` yields **200 OK, correct `Content-Type`, zero-length body**.
Because `m.Size()` is 0, the `Content-Length` guard at `resource.go:147` doesn't even fire,
so it is a 200 chunked response with no bytes.

Pre-branch, the client followed a presigned URL and received MinIO's real 404. This branch
converts a hard, visible error into a silent success — the client gets a broken image with
no way to distinguish it from a successful empty file.

**Fix:** prime the object before committing the status line — `Stat()` it, or read the first
chunk into a buffer — and map `NoSuchKey` to 404 (and other failures to 500) *before*
`w.WriteHeader`.

---

## NON-BLOCKING

- **N1 — Oversized upload does a full 25 MiB round-trip to MinIO before the 413.** A multipart upload is initiated (`newUploadID`) and parts are streamed until `MaxBytesReader` trips, then aborted by the deferred `abortMultipartUpload`. The early-rejection half of the B1 fix removes this too.
- **N2 — Response `Content-Length` comes from the DB row, not the object.** `resource.go:147-148` uses `m.Size()`. If row and object ever disagree the response is malformed. Low likelihood given the write path; noting it because the header is now this service's responsibility, where MinIO used to own it.
- **N3 — No `X-Content-Type-Options: nosniff` / `Content-Disposition` on the content route.** `Content-Type` is client-supplied at init (`resource.go:49` → `processor.go:93` → `builder.go:32`) and `Build()` (`builder.go:38-45`) validates only fleetID/userID/bucket/objectKey — never contentType. The branch moves attacker-chosen bytes under an attacker-chosen Content-Type onto the API origin, where they previously came from MinIO's origin. **Severity is materially limited**, not theoretical hand-waving: auth is Bearer-header-only (`packages/shared-go/auth/middleware.go:18-19`), there is no cookie, so a browser cannot be navigated to this URL with ambient credentials, and the web client consumes it as a blob URL (opaque origin). Hardening item, not a live XSS.
- **N4 — DOM-19: new tests are not table-driven.** `grep` for `t.Run(` across `apps/media-service/internal/mediaobject/*_test.go` returns nothing. Consistent with the package's pre-existing style; the new tests are otherwise well-targeted and assert the right invariants.
- **N5 — Pre-existing, out of scope:** `Make(Entity) Model` returns no error (`entity.go:30`, DOM-03 wants `(Model, error)`) and `Transform` returns a bare `server.Resource` (`rest.go:18`, so DOM-09's "check the Transform error" is inapplicable). The branch did not introduce these and in fact *reduced* this surface by deleting `TransformWithUploadURL`/`TransformWithDownloadURL`.

---

## Checklist Results — `mediaobject` (domain package)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists | PASS | `builder.go:16` `NewBuilder()`, fluent setters `:24-35`, `Build()` validates `:38-45` |
| DOM-02 | `ToEntity()` on Model | PASS | `entity.go:48` |
| DOM-03 | `Make(Entity)` | WARN | `entity.go:30` — exists, returns `Model` not `(Model, error)`; pre-existing (N5) |
| DOM-04 | `Transform` | PASS | `rest.go:18` |
| DOM-05 | `TransformSlice` | PASS | `rest.go:36`; no inline transform loops in `resource.go` |
| DOM-06 | Processor takes `FieldLogger` | PASS | `processor.go:77` `log logrus.FieldLogger` |
| DOM-07 | Handlers pass `d.Logger()` | PASS | `resource.go:45` passes injected `log`; no `logrus.StandardLogger()` in package |
| DOM-08 | POST/PATCH use `RegisterInputHandler` | PASS | `resource.go:48` for `POST /media`. `PUT /media/{id}/content` (`:74`) is a raw octet-stream body and legitimately cannot use the JSON:API input handler |
| DOM-09 | Transform errors handled | N/A | `Transform` returns no error (`rest.go:18`); no `_, _ :=` discards present |
| DOM-10 | Providers lazy | PASS | `provider.go:24,35` — direct `db.First`, no eager `FixedProvider` wrapping |
| DOM-11 | No `os.Getenv()` in handlers | PASS | zero matches in package; cap read once at `cmd/main.go:91` and injected via `InitializeRoutes` |
| DOM-12 | No cross-domain logic in handlers | PASS | handlers call only `proc.*` |
| DOM-13 | Handlers don't call providers | PASS | no `NewProvider`/provider calls in handler bodies |
| DOM-14 | No direct entity creation in handlers | PASS | zero `db.Create`/`db.Save`/`db.Delete` in `resource.go` |
| DOM-15 | `administrator.go` for writes | PASS | `administrator.go:11-18` `Insert`/`Update`/`UpdateInTx`/`SoftDelete`; `StoreContent` writes via `a.Update` (`processor.go:146`) |
| DOM-16 | Error → HTTP status mapping | PASS | `getActive` `processor.go:252-266` (404 deleted / 410 purgeable), `AuthorizeAccess` `:41-46` (cross-fleet → 404), `server.StatusFor` `errors.go:15-34` |
| DOM-17 | JSON:API interface on REST models | PASS | `server.Resource` with `Type`/`ID` (`rest.go:19-21`) — package uses the shared envelope, not per-model methods |
| DOM-18 | Flat request models | PASS | `resource.go:48-51` anonymous flat struct, no Data/Type/Attributes nesting |
| DOM-19 | Table-driven tests | FAIL | no `t.Run(` in any `mediaobject/*_test.go`; see N4 (non-blocking, pre-existing style) |

**Immutable model discipline — PASS.** `WithStatus` (`model.go:45`), `WithSize` (`model.go:51`),
`WithSoftDelete` (`model.go:57`) all take a value receiver and return a copy. `StoreContent`
composes correctly: `pr.a.Update(m.WithSize(counted.n))` (`processor.go:146`).

## Points You Asked Me To Scrutinize

- **Tenancy — PASS on the production path.** `StoreContent` calls `getActive` then `AuthorizeAccess` before touching storage (`processor.go:132-138`); `Content` goes through `GetByID` which does the same (`processor.go:209`, `:194-203`). `getActive` (`processor.go:252-266`) returns 404 for deleted and 410 for purgeable. Cross-fleet → 404 via `AuthorizeAccess` (`processor.go:41-46`). Verified in the source, not only in tests.
- **Content-Type provenance — PASS.** `processor.go:143` passes `m.ContentType()` from the loaded row. The request's own content type is never read in the PUT handler (`resource.go:74-101`). A client cannot relabel someone else's bytes. (Whether the *owner* can mislabel their own is N3.)
- **413 mapping — mechanically correct, but gated behind B1.** minio-go returns the reader error **unwrapped**: `api-put-object.go:432-435` (`if rerr != nil && rerr != io.ErrUnexpectedEOF && rerr != io.EOF { return UploadInfo{}, rerr }`), so `errors.As(err, &*http.MaxBytesError)` at `resource.go:33` does match and `server.StatusFor` maps it to 413 (`errors.go:27-28`). Note the cap can *only* ever trip on the `size = -1` path — for a valid `Content-Length` under the cap, `net/http` bounds the body at `Content-Length` first, so `MaxBytesReader` never fires. The 413 therefore lives entirely on the path that B1 makes fatal.
- **Resource lifecycle — `Close` PASS, streaming PASS, mid-copy failure is B2's root.** `rc` is closed on every path: the error return precedes acquisition (`resource.go:138-141`) and `defer rc.Close()` is registered immediately after (`resource.go:142`). Bytes are streamed via `io.Copy`, not buffered. The "copy fails after headers are on the wire" case is real and is exactly what B2 exploits — the code correctly observes it cannot change the status, but the fix is to not commit the status that early.
- **Error-wrapping consistency — PASS.** New code returns bare sentinels/pass-through errors, matching the package's existing style (`processor.go:140`, `:144`).
- **SEC — nothing beyond the gateway block.** `PresignPut`/`PresignGet` removal (`storage/minio.go`) strictly narrows surface; no public MinIO exposure. No hardcoded secrets in the diff. No open redirect. `MEDIA_MAX_UPLOAD_BYTES` is properly wired (`deploy/k8s/base/media-service/configmap.yaml:14` = `26214400`, matching the `cmd/main.go:91` default). The fleet-service `/internal/*` matter is A3 and gateway-blocked; I found nothing the block does not cover. **B1 is the one genuine new attack surface this branch opens.**

## Verdict

**NEEDS-WORK — do not merge as-is.**

Build, vet, and `-race` tests are all green, and the architecture of the change is sound:
tenancy is enforced on the real path, Content-Type provenance is correct, the ReadCloser is
closed everywhere, immutable-model discipline is clean, and deleting the presign plumbing
genuinely narrows the attack surface. The DOM checklist is essentially clean.

But the two defects below are both reachable through ordinary, non-malicious client
behaviour, and both are invisible to the test suite because the tests that cover those code
paths substitute a fake for the MinIO SDK.

**Must fix before merge:**
1. **B1** — 528 MiB buffer on the `size = -1` path against a 256 MiB pod limit. A single chunked request, or one with an oversized `Content-Length`, OOM-kills media-service from the public internet. Reject oversized `Content-Length` up front, and set an explicit `PartSize` in `storage.PutObject`.
2. **B2** — `GET /media/{id}/content` answers 200 with an empty body for an object whose bytes were never stored. Stat or prime the object before writing the status line.

**Can be deferred:** N1 (subsumed by B1's fix), N2, N3, N4, N5.
