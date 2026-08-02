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
