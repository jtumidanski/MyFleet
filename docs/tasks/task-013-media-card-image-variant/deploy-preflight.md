# task-013 deploy pre-flight — unique index on `media.media_variants`

This task adds `uniqueIndex:ux_media_variants_object_variant` over
`(media_object_id, variant)`. GORM `AutoMigrate` runs inside
`database.Connect`, and a migration failure is `log.Fatal` in `main` — so if a
target database already holds duplicate rows, **media-service will not boot**.

Duplicates should not exist: `ReplaceForMediaObject` is a transactional
delete-then-insert. But two concurrent redeliveries under READ COMMITTED can
each delete a snapshot that does not include the other's inserts. Unlikely, not
impossible — so check before deploying.

## 1. Check each target database

```sql
SELECT media_object_id, variant, count(*)
FROM media.media_variants
GROUP BY 1, 2 HAVING count(*) > 1;
```

Zero rows → nothing to do; deploy.

Also record the row count while you're connected:

```sql
SELECT count(*) FROM media.media_variants;
```

`AutoMigrate` builds the new unique index with a plain `CREATE UNIQUE INDEX` —
no `CONCURRENTLY` — which takes a lock blocking writes to
`media.media_variants` for the build's duration, including the old pods'
variant writes if the rollout is still in flight. At current scale that is
expected to be milliseconds, but "expected" is not "verified": recording the
row count now turns that into a checked fact rather than an assumption, so a
future incident review has an actual number instead of a guess.

## 2. If it returns rows, de-dupe (keep the newest `created_at`)

```sql
DELETE FROM media.media_variants v
USING media.media_variants keep
WHERE v.media_object_id = keep.media_object_id
  AND v.variant        = keep.variant
  AND (v.created_at, v.id) < (keep.created_at, keep.id);
```

Re-run the check in step 1 — it must return zero rows — then deploy.

The newest row is the right one to keep: it was written by the most recent
successful generation, so its `object_key` is the one whose bytes are certain to
be in storage.

## 3. Roll media-service to completion before rolling web

`apps/web` and `apps/media-service` are separate Deployments that roll
independently. The pre-branch `ParseContentVariant` on an old media-service pod
rejects `?variant=card` with a **400** — not a 404 — so there is no downgrade
and no fallback for that response. If web reaches new pods before
media-service does, every vehicles-list hero request from a new web pod sends
`?variant=card` to a media-service pod that does not recognise it yet. With
React Query's `retry: 1` that is two failed requests per tile before the UI
gives up and shows a "Photo unavailable" placeholder — on every vehicle card,
fleet-wide, for as long as the rollouts overlap.

**Roll media-service to completion first.** The backend accepting
`?variant=card` is a strict prerequisite for the frontend requesting it —
confirm media-service's rollout has finished (e.g. `kubectl rollout status
deployment/media-service` returns without further polling) before starting the
web rollout.

It self-resolves once media-service finishes rolling, so if you see
"Photo unavailable" appear on vehicle cards network-wide immediately after a
deploy, check whether media-service is still mid-rollout before treating it as
a data problem — that is the signature of this ordering being reversed.

## 4. Clearing a stale permanent-failure ledger entry

Lazy card generation records a permanent failure with reason
`original-missing` when the original object is not in storage. If that
original is later restored — a backup restore, or an accidental-deletion fix —
the ledger entry still blocks generation forever; nothing about restoring the
original clears it automatically. Reach for this when a `media_variant_failures`
row with reason `original-missing` exists for media whose original has since
been confirmed present again:

```sql
DELETE FROM media.media_variant_failures WHERE media_object_id = '<id>';
```

The next request for that object's card variant then regenerates normally —
there is no separate retry flag to flip.
