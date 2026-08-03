# task-018 — follow-ups

Items surfaced during task-018's implementation, browser verification, and
code review that are out of this branch's scope. None are fixed here. Recorded
so they can become their own issues rather than vanishing with a gitignored
scratch file.

## From browser verification (`verification.md`)

### 1. `deploy/compose/docker-compose.yml` — `web` service is unreachable as committed

The `web` service's healthcheck (`wget http://localhost/`) and the Traefik
`traefik.http.services.web.loadbalancer.server.port` label both target port
`80`, but `apps/web/Dockerfile` builds `nginx-unprivileged`, which binds
`:8080` per `apps/web/nginx.conf` (`listen 8080;`). As a result the container
reports `unhealthy`, Traefik's Docker provider excludes it, and
`http://localhost/` 404s on a stack `docker compose ps` otherwise reports
fully up. Separately, nginx's IPv6-listen patch
(`docker-entrypoint.d/10-listen-on-ipv6-by-default.sh`) silently no-ops
because the container's root filesystem is read-only, so nginx only binds
`0.0.0.0:8080`, not `[::]:8080` — a healthcheck retargeted at `:8080` must use
`127.0.0.1`, not `localhost` (which resolves `::1` first).

Worked around during verification with a private, gitignored
`docker-compose.override.yml` (repointing the healthcheck and the Traefik
port label at `8080`/`127.0.0.1`). No tracked file was modified. The base file
is still broken for the next person following `docs/runbooks/local-debugging.md`.

**Recommendation:** fix `deploy/compose/docker-compose.yml`'s `web`
healthcheck and Traefik port label to `8080`, and address the healthcheck
host (`127.0.0.1` vs `localhost`).

### 2. `fleet-service` accepts an expired JWT — security gap

`GET /api/fleet/fleets/{id}/activity` with a token whose `exp` claim is an
hour in the past returned `200`. The identical token against
`GET /api/auth/me` correctly returned `401`. This is a live authorization gap
in `fleet-service`, more serious than item 1 above — likely a
JWT-parsing/validation-option difference between how `fleet-service` and
`auth-service` invoke the shared `packages/shared-go/auth` middleware, or a
leeway/clock-skew setting. Not investigated further here (out of scope for a
verification-only pass on task-018).

**Recommendation:** file as a security issue and investigate the
`fleet-service` JWT validation path against `auth-service`'s.

## From code review (`audit.md`)

Deliberate, adjudicated deferrals — not blocking, not to be fixed on this
branch:

### 3. Every `>= 500` response, including the new `503`, still logs at ERROR

`server.WriteError` (`packages/shared-go/server/jsonapi.go`) logs every
`status >= 500` at Error level with no exemption for `503`. The refresh
handler's transient branch already logs its own `Warn` line
(`"resolve principal on refresh: upstream unavailable"`), so a transient
refresh failure now emits both the intended WARN and a generic ERROR line.
FR-REFRESH-6's "does not inflate the error rate" is only partly met. This is
shared behaviour across roughly 190 `WriteError` call sites, so the fix
belongs in `shared-go/server`, not in `auth-service`.

### 4. `refresh.ts` still maps non-503 transport failures to the `dead` bucket

`apps/web/src/lib/api/refresh.ts`'s `catch` block maps a thrown `fetch`
(offline, DNS, TLS, or a 502/504 that isn't the classified 503) to
`{ status: 'dead' }`, which still clears the token and logs the user out —
the same defect class task-018 fixes for 503, just not for these shapes.
Documented as an intentional, accepted scope boundary at `design.md:241`.

### 5. `oidc/resource.go`'s `ProvisionFromGoogle` failure still redirects `#error=server_error` during a database outage

One call earlier than `d.Resolve` (which now correctly redirects
`#error=service_unavailable` on a transient failure),
`ProvisionFromGoogle`'s failure path is unchanged and still redirects
`#error=server_error` even when the underlying cause is the same kind of
database outage. A consistency gap between two adjacent failure points in the
same handler.

### 6. `CODES` in `loginError.ts` is hand-maintained and not compiler-linked to `LoginErrorCode`

`apps/web/src/lib/auth/loginError.ts`'s `CODES` is typed `readonly string[]`,
so adding a `LoginErrorCode` union member without adding the matching string
compiles clean and silently degrades that code to `server_error`. A guard
test (`loginError.test.ts`) currently covers this specific instance, but the
structure permits recurrence. Deriving `const CODES = Object.keys(NOTICES)`
(since `NOTICES` is already `Record<LoginErrorCode, LoginErrorNotice>` and
therefore exhaustiveness-checked) would eliminate the class of bug rather
than relying on the guard test.

### 7. `fleetLookupTimeout` is a mutable package var written by a test

`apps/auth-service/internal/membership/client.go`'s `fleetLookupTimeout` was
changed from a `const` to a package-level `var` so
`TestActive_boundsAHangAndClassifiesItTransient` can lower it during the
test. Safe today only because nothing in that file uses `t.Parallel()`; a
future parallel test in the same package would race on this var.

### 8. `RetryAfter`'s composition with `Detailed` is untested

`packages/shared-go/server`'s `RetryAfter` wrapper is designed to compose
with `Detailed` in either order (both implement `Unwrap`, and `errors.As`
walks the whole chain) — hand-traced correct in the design, but no test
exercises both wrappers applied to the same error together.

### 9. `Active`'s request-construction error is returned unredacted

`apps/auth-service/internal/membership/client.go`'s `Active` returns the
`http.NewRequestWithContext` error bare. On a malformed base URL that is a
`*url.Error` whose message includes the endpoint with `?user_id=`, reaching
`session/resource.go` or `oidc/resource.go` at Error level. Narrow — only
reachable via startup misconfiguration, and `QueryEscape` means the user id
itself cannot break parsing — but the redaction applied a dozen lines below
(the transport-failure unwrap to `urlErr.Err`) is not applied here.

### 10. The underlying database error is discarded entirely

`cmd/main.go`'s principal-resolver wrapping neither wraps nor logs the driver
error on a local infrastructure failure, so a Postgres outage leaves
auth-service with a fixed `"user lookup failed"` / `"platform admin lookup
failed"` message and no SQLSTATE anywhere in its logs. This is the direct,
correct consequence of the stated redaction invariant (no driver text may
reach a log line) — recorded only so the diagnosability cost is a known
trade-off, not fixed here.

## From the final re-review

### 11. A cold reload during an outage still bounces to `/login`

The fix that keeps the access token on a 503 (`3bb7147`) stops an
**in-session** refresh from logging the user out — that is what
`verification.md` Check 1 exercised, via a client-side navigation. It does
not, by itself, stop a **cold page reload** made while the outage is still in
progress: `AuthContext.tsx` computes
`isAuthenticated = hasToken && !!data?.user`, and on a fresh page load
`me.data` is `undefined` until `useMe` resolves, so `RequireAuth` still
navigates to `/login` regardless of whether the token survives underneath.
What the fix genuinely buys in this case is that the token is still in
`localStorage` when the user lands back on `/login` — so recovery is a
reload once `fleet-service` returns, not a full Google sign-in round-trip.
Not fixed here; would need `isAuthenticated`/`RequireAuth` to tolerate a
loading identity query rather than treating "not yet resolved" the same as
"not authenticated".

### 12. A 503 without a JSON:API body clears the token instead of surviving

`AuthContext.tsx`'s guard (`me.error instanceof ApiError && me.error.status
=== 503`) only recognizes a 503 that `createErrorFromUnknown` could parse a
JSON:API envelope from. `ApiClient.request` (`packages/shared-ts/src/apiClient.ts`)
reads the body with `res.json().catch(() => null)`; if an intermediary
answers 503 with an HTML body instead of JSON — e.g. Traefik returning its
own 503 when a service has no healthy endpoints, rather than the request
ever reaching auth-service — `body` is `null`, and `createErrorFromUnknown`
falls through to `ApiError(0, 'unknown')`. The `status === 503` guard misses
that (`status` is `0`), so the token is cleared and the user is signed out
for an outage the code was specifically written to survive. auth-service's
own `/auth/me` handler never emits a 503, so this gap is infrastructure-level
only — reachable through Traefik/the reverse proxy, not through
auth-service's own responses. Not fixed here.
