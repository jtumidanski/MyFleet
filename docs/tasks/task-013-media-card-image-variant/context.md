# Media `card` Image Variant — Implementation Context

Companion to `plan.md`. Everything here was verified against the source in this worktree, not recalled — line numbers are as of commit `f279c4d`.

---

## 1. Why this task exists

The vehicles list renders each vehicle's primary photo as a full-width 16:9 hero (`VehicleCard.tsx:102`, `boxClassName="aspect-[16/9] w-full rounded-none"`), but `VehiclePhotoThumbnail.tsx:81` requests the `thumbnail` variant, whose max edge is 320px. Stretched across a hero box that is 240–540 CSS px wide, it is visibly soft.

task-007 saw this and deliberately deferred it, recording in its design D5 that serving `display` (1280px) for N cards is "precisely the cost task-005 §4.7 existed to avoid", and that if the result reads soft "that is a variant-sizing task in `media-service`, not a change here". Its `manual-verification.md` carried the item forward under "Deferred, note only". **This task is that deferred work** — closing that item is an explicit acceptance criterion.

---

## 2. Key files

### `apps/media-service`

| File | What matters about it |
|---|---|
| `internal/processing/worker.go` | `thumbnailMaxEdge`/`displayMaxEdge` at 29-32; `handle` at 126; the spec-slice loop at 169-175; `decodeOriginal` (216) and `generateVariant` (235) are `*Worker` methods to be made package-level; `ResizeDims` (45) already handles 768 with no new branch |
| `internal/mediavariant/model.go` | `Variant` constants at 8-11, with the `// max edge NNN` comment convention |
| `internal/mediavariant/entity.go` | `Entity` (10-19); `Migration` is bare `AutoMigrate` (23) |
| `internal/mediavariant/administrator.go` | Only `ReplaceForMediaObject` — transactional **delete-then-insert** (19-32). The trap |
| `internal/mediavariant/provider.go` | `GetByMediaObjectAndVariant` uses `First()` (55); `ListByMediaObject` (36) hands every row to any caller |
| `internal/mediavariant/provider_test.go` | `newVariantTestDB` builds `media.media_variants` with **raw SQL** (27-38), not AutoMigrate |
| `internal/mediaobject/contentvariant.go` | `ContentVariant` constants (10-14); `ParseContentVariant` with `default: server.ErrBadRequest` (33-35) |
| `internal/mediaobject/processor.go` | `VariantLookup` port (66-68); `NewProcessor` (155); `Content` (335-391) inlines the whole variant path; the no-fallback rationale lives in the doc comment at 320-329 |
| `internal/mediaobject/resource.go` | `InitializeRoutes` (45); `Cache-Control: private, max-age=300` on every content response (208) |
| `internal/processedevents/processedevents.go` | The ledger `variantfailures` is modelled on: `Entity` + `Store`, `clause.OnConflict{DoNothing: true}` (42) |
| `cmd/main.go` | Migration list (36-41); `MEDIA_WORKERS` via `config.GetInt` (86); `variantLookup` adapter (178-189) — the port pattern to copy |
| `cmd/entityguard_test.go` | Runs `entityguard.Analyze("../internal")` over the whole tree; a new package is covered the moment it exists |

### `apps/web`

| File | What matters about it |
|---|---|
| `src/types/models/media.ts:18` | `MediaVariant` union — a mirror of `contentvariant.go` |
| `src/components/features/vehicles/VehiclePhotoThumbnail.tsx:81` | `useMediaContentUrl(mediaId, 'thumbnail')` — the one line that changes |
| `src/components/features/vehicles/media/MediaThumbnail.tsx:42` | `useMediaContentUrl(mediaId)` — **no variant**, so it sends no query parameter. Not touched |
| `src/lib/hooks/api/media.ts:17-28` | `mediaKeys.content(id, variant)` already carries the variant, so `card` and `thumbnail` bytes cannot be served in place of one another |
| `src/lib/hooks/api/media.ts:150-162` | `useMediaContentUrl`: `staleTime` 5 min, `gcTime` 6 min |
| `src/services/api/MediaService.ts:67-74` | `getContentBlob`; `variant === 'original'` sends **no** query parameter at all |
| `src/services/api/MediaService.test.ts:21-40` | Pins the no-parameter form and the two variant forms |

---

## 3. Decisions already made (do not relitigate)

Resolved in the PRD interview:

- **768px max edge.** 640 falls short at 2x above a ~1024px viewport, leaving the reported defect partly unfixed; 960 is only 1.33x narrower than the existing 1280 `display`, at which point "just use `display`" is the simpler answer. One fixed variant serves every breakpoint — `srcset` is a non-goal.
- **Lazy, asynchronous backfill**, not a sweep job or event replay. Event replay is blocked concretely: `worker.handle` short-circuits when `obj.Status() == StatusReady` (`worker.go:149-157`), and every pre-existing image is `ready`, so republishing `media.uploaded` would record the event and generate nothing.
- **Downgrade is `card → thumbnail` only.** A general next-smaller-available ladder would let a detail view asking for `display` silently receive a 768px image with no way to detect it.

Resolved in design:

- **Uniqueness in the database** (composite `uniqueIndex` + `OnConflict` upsert), not only the in-process single-flight map. `replicas: 1` still means a rolling update transiently runs two pods with two independent maps. The rejected alternative — a deterministic UUIDv5 row ID with `ON CONFLICT (id)` — does not work, because `ReplaceForMediaObject` inserts with `uuid.NewString()`, so a worker-written and a lazily-written `card` row would have different IDs and would not conflict.
- **A dedicated `variantfailures` ledger**, not a column on `media_objects` (variant-scoped state on the object aggregate, and `Administrator.Update` saves a full entity built from the immutable `Model`, so an out-of-band flag would be silently erased) and not a sentinel row in `media_variants` (a row that looks servable but is not, handed out by `ListByMediaObject`).
- **`Generate` is a functional option, not a seventh positional parameter.** The dependency is genuinely optional, and sixteen existing `NewProcessor` call sites would otherwise change for no behavioural reason.
- **The ledger check runs inside the goroutine, not in `Generate`.** Checking synchronously would put a database round trip on the *common* path — every downgraded response for the whole lazy-fill period — to guard a case that should never occur. The observable contract ("a second request does not re-attempt decoding") holds exactly either way.

Deliberate deviation from the PRD, flagged rather than silent:

- PRD §7 lists `deploy/k8s` as unaffected because the new key has an in-code default. The plan **does** add `MEDIA_LAZY_VARIANT_CONCURRENCY` to `base/media-service/configmap.yaml`, because `MEDIA_WORKERS` and `MEDIA_MAX_UPLOAD_BYTES` both have in-code defaults and both appear there, and the ConfigMap is where an operator looks for the knob. Additive and value-preserving.

---

## 4. The four ways to get this wrong

1. **Using `ReplaceForMediaObject` for the lazy write.** It deletes every row for the media object before inserting, so writing one card model would destroy that object's `thumbnail` and `display`. Guarded by `TestUpsert_leavesOtherVariantsUntouched` and `TestCardGenerator_doesNotDestroyExistingVariants` (both marked ★ in the plan).
2. **`created_at` in the upsert's `DoUpdates`, or `UpdateAll: true`.** Either rewrites the row's age on every regeneration — the same column-wipe class task-006 fixed, in a new disguise. `entityguard` will **not** catch it: it recognises `.Save(` call sites only (`entityguard.go:18-21`) and names `db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&e)` as the exact call shape it cannot see. Guarded by `TestUpsert_preservesCreatedAt`.
3. **Forgetting the `UNIQUE` constraint in the SQLite test DDL.** `provider_test.go` builds the table with raw SQL, so the GORM tag and that DDL are two independent statements of the same schema. Without it, `Upsert`'s `ON CONFLICT` fails with *"does not match any PRIMARY KEY or UNIQUE constraint"* — and a suite that omitted it while production had it would be green on a path production rejects.
4. **Letting the downgrade generalise.** The guard is the literal `want != ContentCard` check plus the "display missing → still 404" test. That test is the only thing standing between this change and a detail view silently receiving a smaller rendition than it asked for.

---

## 5. Deployment hazard

`mediavariant.Migration` is `db.AutoMigrate(&Entity{})`, run inside `database.Connect`, and a failure there is `log.Fatal` in `main`. If a target database already holds duplicate `(media_object_id, variant)` rows, `CREATE UNIQUE INDEX` fails and **media-service will not boot**.

Duplicates should not exist — `ReplaceForMediaObject` is a transactional delete-then-insert — but two concurrent redeliveries under READ COMMITTED can each delete a snapshot that does not include the other's inserts. Unlikely, not impossible. The check and de-dupe procedure are in `deploy-preflight.md`, written in Task 2 and referenced from `manual-verification.md`.

Failing loudly at boot is correct behaviour for a data-integrity change. It must simply not be a surprise.

---

## 6. Dependency direction

`mediaobject` must not import `processing`, and does not today. The established shape is the `VariantLookup` port: the interface is declared in the consumer (`processor.go:66`), the implementation lives in the composition root (`cmd/main.go:178`), and the two sibling domain packages stay independent. `CardGenerator` crosses the same way.

```
                       cmd/main.go  (composition root)
                        │        │
        variantLookup ──┘        └── cardGenerator
              │                              │
              ▼                              ▼
   mediavariant.Provider          processing.CardGenerator
                                        │      │       │
                                        │      │       └── variantfailures.Store
                                        │      └────────── mediavariant.Administrator
                                        └───────────────── storage (ObjectStore)

   mediaobject ── declares VariantLookup, CardGenerator ── imports neither impl
```

---

## 7. Task dependency order

Tasks are written to be executed in order; the ones that can be reordered are noted.

```
1 (variant)  ──┬──► 4 (extract decode/build) ──► 5 (CardGenerator) ──┐
               │                                                     ├──► 7 (composition root)
2 (Upsert) ────┼─────────────────────────────────────────────────────┤
               │                                                     │
3 (ledger) ────┴─────────────────────────────────────────────────────┘

1 ──► 6 (downgrade + port) ──► 7
1 ──► 8 (frontend)

7, 8 ──► 9 (full verification)
```

- **Task 2 must not precede Task 1**: both edit `worker_test.go`'s `fakeVariantAdmin`, and Task 2's `Upsert` stub is what keeps `processing` compiling once the interface widens.
- **Tasks 2, 3 and 6 are mutually independent** and could run in any order among themselves.
- **Task 8 is independent of everything after Task 1** — the frontend only needs the backend to accept `?variant=card`. It can ship before the lazy machinery exists; a missing card row simply downgrades to thumbnail, which is today's rendering.

---

## 8. Verification

```sh
make ci                                          # lint-check, vet, test, build, fe-test, fe-build
go test ./apps/media-service/internal/processing/ -race -count=2
kustomize build deploy/k8s/overlays/local > /dev/null
kustomize build deploy/k8s/overlays/main  > /dev/null
```

`-race` is not optional for `processing`: the concurrency in `CardGenerator` is the feature, and `-count=2` defeats the test cache, which is what catches an ordering-dependent single-flight test.

Node may need loading first: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`.

Two acceptance criteria are measurements rather than assertions and are deferred to `manual-verification.md`: that a pre-existing photo reads sharp at `lg:grid-cols-3` on a high-DPI display after the lazy fill, and that the `card` variant's transferred size is materially smaller than the same photo's `display` (NFR-1).

Per CLAUDE.md, run `superpowers:requesting-code-review` (or `/audit-plan`) before opening a PR — it is not optional even when the plan looks complete.

---

## 9. Left open on purpose

- **PRD §9.6** — whether the `card` query should use a shorter `staleTime`. v1 answer is no: a pre-existing photo stays soft for the first visit and sharpens on a later one, which is strictly better than today and needs no new invalidation machinery. Note that the effective window is bounded by **both** the React Query `staleTime` (5 min) and the response's `Cache-Control: private, max-age=300` (`resource.go:208`) — the PRD analyses only the first.
- **PRD §9.7** — whether `VehicleDetailPage` wants `card` too. Not checked, not in scope.
- **FR-5.7** — no response header signals that a downgrade occurred. The client cannot distinguish, but the failure mode is a slightly soft image (today's behaviour), not a broken or oversized one. Revisit only if a caller needs to act on it.
