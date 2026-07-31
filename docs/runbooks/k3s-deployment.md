# Runbook — deploying MyFleet to the bee k3s cluster

MyFleet runs in namespace `myfleet` on bee, served by the cluster's k3s Traefik
at `192.168.23.230` on three hostnames:

- `https://myfleet.tumidanski.com` — Cloudflare-proxied. Cloudflare presents its
  own edge certificate to the browser and connects to the origin over TLS, so
  the origin must serve :443 with the `myfleet-tls` certificate. An earlier
  version of this runbook claimed TLS "terminates at the edge" and configured
  :80 only; the result was a `404 page not found` on every public request.
- `https://myfleet.tumidanski.me` — LAN, direct to Traefik, self-signed cert
  (browser warning). Mirrors `homehub.tumidanski.me`. Being real TLS, it is the
  only LAN host that *could* hold a session — but it cannot log in today:
  `APP_BASE_URL` and `GOOGLE_REDIRECT_URL` are pinned to the `.com` host
  (`deploy/k8s/overlays/main/patches/auth-service-config.yaml`), so a login
  started here round-trips through `.com` and lands the cookie on the wrong
  host. See *Known constraints*.
- `http://myfleet.home` — LAN, plain HTTP. Useful for health checks.

Both `.me` and `.home` resolve to `192.168.23.230` directly; only the `.com`
host goes through Cloudflare.

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
# startup on a slow connection. Wait until it actually accepts connections.
# Use curl, not `exec 3<>/dev/tcp/...`: /dev/tcp is a bash-only feature and
# this project's shell is zsh, where the redirect always fails and the loop
# spins forever instead of falling through.
until curl -sf --max-time 2 http://localhost:9000/minio/health/live >/dev/null; do sleep 0.5; done
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

Copy the template to `deploy/k8s/secrets.yaml`, fill it in, and apply it. That
filename is gitignored (`.gitignore`), so the filled-in copy lives beside the
manifests it belongs to without any risk of being committed — the same pattern
as `home-hub`:

```sh
cp deploy/k8s/secrets.example.yaml deploy/k8s/secrets.yaml
# edit deploy/k8s/secrets.yaml — replace every REPLACE_ME
kubectl create namespace myfleet --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -n myfleet -f deploy/k8s/secrets.yaml
```

Keep the file: re-applying it is how you rotate a credential or fill in the
Google OAuth values later. Confirm it is ignored with
`git check-ignore -v deploy/k8s/secrets.yaml` before you put anything real in it,
and restrict it — it holds every production credential in plaintext, forever:

```sh
chmod 600 deploy/k8s/secrets.yaml
```

This is a deliberate trade against the old `/tmp` + `shred -u` flow: the
credentials now persist so that re-apply and rotation are one command, at the
cost of a long-lived plaintext file inside the working tree. Two controls back
that up — `.gitignore` keeps it out of git, and the root `.dockerignore` keeps
it out of image layers (the build context is the repo root for every service).

| Secret | Keys |
|---|---|
| `auth-service-secret` | `DATABASE_URL`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `JWT_PRIVATE_KEY_PEM`, `OIDC_STATE_SECRET` |
| `fleet-service-secret` | `DATABASE_URL` |
| `media-service-secret` | `DATABASE_URL`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY` |
| `notification-service-secret` | `DATABASE_URL` |
| `myfleet-tls` | `tls.crt`, `tls.key` — cert for the `websecure` route set |

`myfleet-tls` is not optional. Without it Traefik registers no router on :443,
and every request arriving over TLS — Cloudflare to the origin, or a browser
straight to `myfleet.tumidanski.me` — falls through to Traefik's default
`404 page not found`. Generate it covering all three hostnames:

```sh
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout myfleet-tls.key -out myfleet-tls.crt \
  -subj "/CN=myfleet.tumidanski.com" \
  -addext "subjectAltName=DNS:myfleet.tumidanski.com,DNS:myfleet.tumidanski.me,DNS:myfleet.home"

# Fill in the myfleet-tls stanza that secrets.example.yaml already ships — it is
# the last document in your copied secrets.yaml — with these two base64 blobs:
base64 -w0 myfleet-tls.crt   # -> data.tls.crt
base64 -w0 myfleet-tls.key   # -> data.tls.key

shred -u myfleet-tls.key myfleet-tls.crt
```

> **Do not append the Secret with `>>`.** `kubectl create secret … -o yaml`
> emits a document with no leading `---`, so appending it to a `secrets.yaml`
> that already ends with a complete document fuses the two into a single
> malformed mapping. YAML resolves the duplicate keys in favour of the last
> one, `kubectl apply` reports only `secret/myfleet-tls created`, and the four
> `*-service-secret` Secrets are silently dropped with no error — every Go
> service then `CrashLoopBackOff`s on a missing `DATABASE_URL`. Edit the
> existing stanza instead. If you do append something by hand, prefix it with
> its own `---` and verify with:
>
> ```sh
> kubectl apply -f deploy/k8s/secrets.yaml --dry-run=client
> ```
>
> Expect **five** lines of output, not one.

Self-signed is sufficient behind Cloudflare **Full (non-strict)**: Cloudflare
accepts any origin certificate and the browser only ever sees Cloudflare's own
edge certificate. Under **Full (strict)** this is rejected and you need a
Cloudflare Origin Certificate instead. Direct LAN access to
`myfleet.tumidanski.me` warns, exactly as `homehub.tumidanski.me` does.

Generate the two auth-service values with:

```sh
openssl genrsa -traditional 2048   # JWT_PRIVATE_KEY_PEM
openssl rand -hex 32               # OIDC_STATE_SECRET
```

`-traditional` is mandatory. auth-service parses the key with
`x509.ParsePKCS1PrivateKey` (`apps/auth-service/cmd/main.go:112`), which accepts
only PKCS#1 — a PEM banner reading `BEGIN RSA PRIVATE KEY`. OpenSSL 3.x's bare
`genrsa` emits PKCS#8, whose banner reads `BEGIN PRIVATE KEY` (no `RSA`), and
auth-service then `CrashLoopBackOff`s on `x509: failed to parse private key (use
ParsePKCS8PrivateKey instead for this key format)` — *after* logging `database
connected and migrated`, so the healthy-looking DB line is not evidence the
service came up. Check the first line of the PEM before applying:

```sh
head -1 myfleet-jwt.pem   # must contain RSA; if not, re-run with -traditional
```

(The banners are written here without their surrounding dashes on purpose: the
full five-dash form trips the `private-key` rule in the `gitleaks` CI job, which
cannot tell prose about a key from an actual key.)

`JWT_PRIVATE_KEY_PEM` must also be a real multi-line YAML block scalar (`|`),
not a single line with `\n` escapes: `pem.Decode` needs actual newlines.

## 4. DNS and the Cloudflare TLS mode

- **Cloudflare:** an `A`/`CNAME` record for `myfleet.tumidanski.com`, proxied,
  pointing at the same origin as `tumidanski.com`.
- **`myfleet.tumidanski.me`** → `192.168.23.230`, alongside the existing
  `homehub.tumidanski.me` record. This one is LAN-only: it resolves straight to
  the private Traefik address and never traverses Cloudflare. Skipping it leaves
  the `.me` host rules in the IngressRoute dead.
- **Pi-hole:** a local record `myfleet.home` → `192.168.23.230`, on **both**
  Pi-hole servers.

**Check the zone's SSL/TLS encryption mode — the deployment depends on it.**
Cloudflare → *SSL/TLS* → *Overview*:

| Mode | Result |
|---|---|
| **Full** | Correct. Cloudflare connects to the origin on :443 and accepts the self-signed `myfleet-tls` cert. |
| **Full (strict)** | Public traffic fails with **526**. The self-signed cert is rejected; you need a Cloudflare Origin Certificate in `myfleet-tls` instead. |
| **Flexible** | Cloudflare connects on :80 instead. It works, but the `:443` route set is unused and TLS between Cloudflare and the origin is off. |

Verify the origin end independently of Cloudflare, since a wrong mode and a
wrong DNS record produce the same symptom:

```sh
curl -sk -H 'Host: myfleet.tumidanski.com' https://192.168.23.230/ -o /dev/null -w '%{http_code}\n'
```

200 means the origin is correct and anything still failing publicly is
Cloudflare-side.

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
kubectl -n myfleet get ingressroute      # BOTH myfleet-routes and myfleet-routes-tls

# Every check runs against :80 AND :443. The two ports are served by two
# separate IngressRoutes (see deploy/k8s/overlays/main/ingressroute.yaml) — a
# `tls` section makes a Traefik router TLS-only, so one object cannot cover
# both. Testing only :80 is how the original "404 page not found" on
# https://myfleet.tumidanski.com went unnoticed: :80 was 200 the whole time.
# A shell function, not a `C="curl …"` string: zsh does not word-split unquoted
# parameters, so the string form runs only under bash and errors out here with
# `command not found: curl -sk -H …`.
probe() {  # probe <scheme> <host> <path> <label>
  curl -sk -H "Host: $2" --path-as-is "$1://192.168.23.230$3" \
    -o /dev/null -w "  $4 %{http_code}\n"
}

for scheme in http https; do
  for host in myfleet.tumidanski.com myfleet.tumidanski.me myfleet.home; do
    echo "$scheme $host"
    probe "$scheme" "$host" /api/fleet/healthz                  'healthz  '
    probe "$scheme" "$host" /                                   '/        '
    probe "$scheme" "$host" /vehicles                           '/vehicles'
    probe "$scheme" "$host" /api/fleet/internal/maintenance/due 'deny-a   '
    probe "$scheme" "$host" /api/fleetinternal/maintenance/due  'deny-b   '
  done
done

# The cert must carry all three hostnames as SANs, or Cloudflare Full (strict)
# and direct browser access to myfleet.tumidanski.me both break.
echo | openssl s_client -connect 192.168.23.230:443 \
  -servername myfleet.tumidanski.com 2>/dev/null |
  openssl x509 -noout -subject -ext subjectAltName
```

Expected: all five Deployments `Available` with no `ImagePullBackOff`;
AutoMigrate completes against the `auth` schema; `/api/fleet/healthz` returns
200; `/` and the deep link `/vehicles` both return 200 — the deep link proves
the catch-all priority is right; both `/api/fleet/internal/...` and
`/api/fleetinternal/...` return **403**. The second is not redundant: it has
no `/` between `fleet` and `internal`, which is exactly the shape a
mandatory-slash edit to the deny regex's `[^/]*/*` would stop catching, since
`fleet-stripprefix` strips the literal string `/api/fleet` rather than a path
segment.

Because the route set is duplicated across `myfleet-routes` (:80) and
`myfleet-routes-tls` (:443), the deny checks must pass on **both** ports. A
route added or edited on one entrypoint but not the other leaves the
unauthenticated `/internal/*` surface exposed on whichever port lost the deny
rule, and a :80-only check would report all-clear.

A 200 on either is a security incident, not a cosmetic bug: fleet-service's
`/internal/*` routes carry no JWT, and `/internal/maintenance/due` returns every
non-ok maintenance schedule across every fleet with no parameters at all. These
two checks are not optional: the control fails open — if the `internal-deny`
Middleware object is missing while the IngressRoute still exists (first Argo
CD sync ordering, or a partial prune), Traefik logs that the middleware
doesn't exist and disables the whole router rather than just refusing
traffic, so the request falls through to the priority-100 `/api/fleet` route
and comes back 200 with the same unauthenticated cross-fleet dump. If either
curl returns 200, check that the `internal-deny` Middleware exists in the
`myfleet` namespace (`kubectl -n myfleet get middleware internal-deny`) and
that the deny route still has a higher `priority` than the `/api/fleet` route.

Then, in a browser on `https://myfleet.tumidanski.com`:

- complete a full Google login round-trip and confirm a session cookie is set
- upload a vehicle photo and confirm the object lands in `myfleet-media`
- confirm a commit to `main` bumps the overlay SHA and Argo CD rolls the
  affected Deployments with no manual intervention

## Known constraints

**`myfleet.home` cannot hold a session — use `myfleet.tumidanski.me` instead.**
`COOKIE_SECURE=true` is required for the Cloudflare host, and browsers do not
send `Secure` cookies over plain HTTP. `myfleet.tumidanski.me` now solves this
for LAN use: it is real TLS (self-signed, so you click through one browser
warning), which means `Secure` cookies work and you can actually use the app
off-Cloudflare. Note that logging in there still requires
`https://myfleet.tumidanski.me/api/auth/callback` to be registered as an
authorised redirect URI, and `GOOGLE_REDIRECT_URL` currently points at the
`.com` host — so a `.me` login round-trip needs both changed.
`myfleet.home` remains useful for `/healthz`, `/readyz` and confirming the stack
is up without depending on Cloudflare, but not for using the app. Sessions are
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
