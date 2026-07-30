# Runbook — deploying MyFleet to the bee k3s cluster

MyFleet runs in namespace `myfleet` on bee, served by the cluster's k3s Traefik
at `192.168.23.230` on two hostnames:

- `https://myfleet.tumidanski.com` — Cloudflare-proxied, TLS terminates at the edge
- `http://myfleet.home` — LAN, plain HTTP

Nothing stateful is deployed. Postgres, Kafka and MinIO are pre-existing shared
cluster services.

Argo CD syncs `deploy/k8s/overlays/main` from `main`. The steps below are the
one-time bootstrap; none of it is automated, by design — a PreSync hook would
put credential handling in the manifests.

> **Ordering:** if Argo CD syncs before step 1, all four Go services
> `CrashLoopBackOff` on a missing schema. That is expected and self-resolves
> once the DDL below runs. No manifest change is needed.

## 1. Postgres — role, database, schemas

GORM `AutoMigrate` creates tables inside these schemas on first start, but it
does **not** create the schemas. This step is a hard prerequisite.

Pick a password and keep it for step 3.

```sh
POD=$(kubectl -n postgres get pods -l app=postgres -o jsonpath='{.items[0].metadata.name}')

# -i is required: kubectl exec does not attach local stdin without it, so a
# heredoc piped in without -i silently delivers nothing to psql — no error,
# just a database that was never created.
kubectl -n postgres exec -i "$POD" -- psql -U postgres <<'SQL'
CREATE ROLE myfleet LOGIN PASSWORD '<password>';
CREATE DATABASE myfleet OWNER myfleet;
SQL

kubectl -n postgres exec -i "$POD" -- psql -U postgres -d myfleet <<'SQL'
CREATE SCHEMA IF NOT EXISTS auth         AUTHORIZATION myfleet;
CREATE SCHEMA IF NOT EXISTS fleet        AUTHORIZATION myfleet;
CREATE SCHEMA IF NOT EXISTS media        AUTHORIZATION myfleet;
CREATE SCHEMA IF NOT EXISTS notification AUTHORIZATION myfleet;
SQL
```

Verify:

```sh
kubectl -n postgres exec "$POD" -- psql -U postgres -d myfleet -c '\dn'
```

Expected: the four schemas, all owned by `myfleet`.

## 2. MinIO — bucket and a scoped user

The credential must not be able to reach any `atlas-*` bucket, so it gets a
policy scoped to `myfleet-media` alone rather than the root user.

Write the policy:

```sh
cat > /tmp/myfleet-media-policy.json <<'JSON'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::myfleet-media",
        "arn:aws:s3:::myfleet-media/*"
      ]
    }
  ]
}
JSON
```

Then create the bucket and user:

```sh
kubectl -n minio port-forward svc/minio 9000:9000 &
# Backgrounding port-forward and immediately using the tunnel races its
# startup on a slow connection. Wait until it actually accepts connections
# (bash's /dev/tcp avoids depending on nc being installed):
until (exec 3<>/dev/tcp/localhost/9000) 2>/dev/null; do sleep 0.5; done
mc alias set bee http://localhost:9000 <root-user> <root-pass>
mc mb bee/myfleet-media
mc admin user add bee myfleet <minio-secret>
mc admin policy create bee myfleet-media-rw /tmp/myfleet-media-policy.json
mc admin policy attach bee myfleet-media-rw --user myfleet
```

Verify the scoping — the first must succeed, the second must be denied:

```sh
mc alias set beemyfleet http://localhost:9000 myfleet <minio-secret>
mc ls beemyfleet/myfleet-media
mc ls beemyfleet/atlas-assets   # must fail with AccessDenied
```

Media bytes are proxied through media-service, so MinIO is never reachable from
a browser and this credential never leaves the cluster.

## 3. Kubernetes Secrets

The main overlay ships no Secrets — they are applied out-of-band so Argo CD does
not manage them and `prune: true` cannot remove them.

Copy the template, fill it in, apply it, and keep the copy out of git:

```sh
cp deploy/k8s/secrets.example.yaml /tmp/myfleet-secrets.yaml
# edit /tmp/myfleet-secrets.yaml — replace every REPLACE_ME
kubectl create namespace myfleet --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -n myfleet -f /tmp/myfleet-secrets.yaml
shred -u /tmp/myfleet-secrets.yaml
```

| Secret | Keys |
|---|---|
| `auth-service-secret` | `DATABASE_URL`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `JWT_PRIVATE_KEY_PEM`, `OIDC_STATE_SECRET` |
| `fleet-service-secret` | `DATABASE_URL` |
| `media-service-secret` | `DATABASE_URL`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` |
| `notification-service-secret` | `DATABASE_URL` |

Generate the two auth-service values with:

```sh
openssl genrsa 2048        # JWT_PRIVATE_KEY_PEM
openssl rand -hex 32       # OIDC_STATE_SECRET
```

## 4. DNS

- **Cloudflare:** an `A`/`CNAME` record for `myfleet.tumidanski.com`, proxied,
  pointing at the same origin as `tumidanski.com`.
- **Pi-hole:** a local record `myfleet.home` → `192.168.23.230`, on **both**
  Pi-hole servers.

## 5. Google Cloud Console

Register this as an authorised redirect URI on the OAuth client:

```
https://myfleet.tumidanski.com/api/auth/callback
```

This is external and cannot be automated or verified from the repo. Login stays
broken until it is done.

Traefik's `auth-stripprefix` middleware strips only the `/api` mount point
(`deploy/k8s/base/routing/middlewares.yaml`), and the OIDC callback route inside
auth-service is `/auth/callback`
(`apps/auth-service/internal/{user,session,oidc}/resource.go`), so the public
path is `/api/auth/callback` — not the doubled `/api/auth/auth/callback` an
earlier draft of this runbook assumed. This must match
`GOOGLE_REDIRECT_URL` in `deploy/k8s/overlays/main/patches/auth-service-config.yaml`
exactly, or login fails in production.

## 6. Argo CD

Add `argocd-myfleet.yml` to the `~/source/k3s/bee` repo — a separate repo and a
separate commit from the MyFleet PR:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: myfleet-main
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/jtumidanski/MyFleet.git
    targetRevision: main
    path: deploy/k8s/overlays/main
  destination:
    server: https://kubernetes.default.svc
    namespace: myfleet
  syncPolicy:
    automated:
      selfHeal: true
      prune: true
    syncOptions:
      - ServerSideApply=true
      - CreateNamespace=true
```

The MyFleet repository is public, so no `repo-creds` Secret is needed.

`prune: true` is on from the start. Atlas deferred it because Kustomize-hashed
ConfigMaps left orphans; this overlay generates no hashed resources. Anything
that must survive a manifest drop needs
`argocd.argoproj.io/sync-options: Prune=false`.

Apply it:

```sh
kubectl apply -f argocd-myfleet.yml
```

Every `newTag` in `deploy/k8s/overlays/main/kustomization.yaml` seeds as
`latest` until the first `bump-overlay` run pins them to a real SHA (see
below). This is currently moot in the normal flow because the `main` pipeline
publishes all five images (including `ghcr.io/jtumidanski/myfleet-web`)
before Argo CD ever syncs. It matters for a fresh cluster or a from-scratch
replay of this bootstrap: if the Application is applied before any image has
been pushed to GHCR under that tag, the `web` Deployment (or any service)
`ImagePullBackOff`s until the corresponding image lands — self-resolving once
CI publishes, same shape as the schema-missing `CrashLoopBackOff` above.

### How images get pinned after the first sync

`.github/workflows/main.yml`'s `main` push pipeline runs `build-test` →
`publish` (image build/push, matrix of 5: the four Go services plus `web`),
then fans out into two jobs that both depend only on `publish` and run in
parallel: `trivy` (a CRITICAL/HIGH vulnerability gate, same matrix,
`fail-fast: false` so one service's findings don't hide another's) and `tag`
(pushes the next semver tag). Trivy does **not** gate `tag` — a failing scan
still lets the version tag land. The only job downstream of both is
`bump-overlay`, which declares `needs: [publish, trivy]` (not `tag`). A
failing Trivy scan on any image blocks `bump-overlay`, so
`deploy/k8s/overlays/main/kustomization.yaml` keeps its previous `newTag` and
the cluster keeps running the last good SHA — the gate fails closed, even
though the SHA already got a version tag.

`bump-overlay` also carries a job-scoped `concurrency: {group: bump-overlay,
cancel-in-progress: false}` so overlapping runs (two merges to `main` in quick
succession) queue rather than race, and a stale-run guard: before rewriting
`newTag`, it checks that `main`'s current tip still matches the SHA this run
built; if a later merge's `bump-overlay` already landed while this run was
still going through `build-test`/`publish`/`trivy`, it exits cleanly instead of
pinning the overlay backwards to a stale commit.

## 7. Post-deploy verification

```sh
kubectl -n myfleet get deploy
kubectl -n myfleet logs deploy/auth-service | grep -i migrat
curl -H 'Host: myfleet.home' http://192.168.23.230/api/fleet/healthz
curl -H 'Host: myfleet.home' http://192.168.23.230/ -o /dev/null -w '%{http_code}\n'
curl -H 'Host: myfleet.home' http://192.168.23.230/vehicles -o /dev/null -w '%{http_code}\n'
```

Expected: all five Deployments `Available` with no `ImagePullBackOff`;
AutoMigrate completes against the `auth` schema; `/api/fleet/healthz` returns
200; `/` and the deep link `/vehicles` both return 200 — the deep link proves
the catch-all priority is right.

Then, in a browser on `https://myfleet.tumidanski.com`:

- complete a full Google login round-trip and confirm a session cookie is set
- upload a vehicle photo and confirm the object lands in `myfleet-media`
- confirm a commit to `main` bumps the overlay SHA and Argo CD rolls the
  affected Deployments with no manual intervention

## Known constraints

**`myfleet.home` cannot hold a session.** `COOKIE_SECURE=true` is required for
the Cloudflare host, and browsers do not send `Secure` cookies over plain HTTP.
The LAN host is useful for `/healthz`, `/readyz` and confirming the stack is up
without depending on Cloudflare, but not for using the app. Sessions are
per-host anyway. Fixing this properly means a real certificate on
`myfleet.home`, not a config change.

**Cloudflare caps request bodies.** Media uploads are proxied through
media-service, so they traverse Cloudflare. `MEDIA_MAX_UPLOAD_BYTES` defaults to
25 MiB, under Cloudflare's free-plan ceiling; a client hitting the service limit
gets a `413`, and one hitting the edge limit is rejected before Traefik.

**No `myfleet` Postgres backup story.** The database is created by hand on a
shared Postgres whose backup policy is out of scope here. Worth a follow-up.

**Argo CD `selfHeal` reverts manual edits.** Anything changed with
`kubectl edit` in the `myfleet` namespace is undone on the next sync.
Out-of-band Secrets are unaffected, being untracked.

**A second MyFleet environment on this broker would cross-talk.** Kafka topics
are a flat namespace and the consumer group is fixed at `notification`, so two
environments would share both. Isolation means making those constants
configurable — a code change, not a manifest change.
