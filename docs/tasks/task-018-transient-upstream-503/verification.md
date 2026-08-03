# task-018 — browser verification record

Verifies, against real Chromium (Playwright container
`mcr.microsoft.com/playwright:v1.62.1-noble`) and the live docker-compose
stack, the two behaviours `jsdom` cannot see:

1. The refresh path keeps the session alive on an upstream (fleet-service) outage.
2. The login page renders the outage notice with the correct CSS-driven danger styling.
3. Recovery: the rotated cookie survives the outage and is honored once fleet-service returns.

**Result: all three PASS.**

This is Task 9 Step 3 of `plan.md`, executed against a seeded user
(`bec47f6f-7ba4-4eb4-8a42-5358ca0a51fe`), a seeded fleet, and seeded
`auth.refresh_tokens` rows, per the seeding recipe in `context.md` /
`design.md`. Access tokens were minted matching `apps/auth-service/cmd/main.go`'s
`kid`/`iss`/`aud` configuration.

## Deviation from the plan's local-stack instructions

The stack as started was not fully healthy despite `docker compose ps` showing
every service up: the `web` container reported `unhealthy` and Traefik
excluded it from routing, so `http://localhost/` 404'd. Root cause is a
pre-existing defect in `deploy/compose/docker-compose.yml`, unrelated to
task-018 (confirmed via `git log` — not touched by any task-018 commit); see
`follow-ups.md` item 1. Worked around locally with a gitignored
`docker-compose.override.yml` (not committed, not part of this branch). After
the workaround, `http://localhost/` and `http://localhost/login` both
returned 200.

A second, unrelated finding surfaced during Check 1 — fleet-service accepts
an expired JWT. See `follow-ups.md` item 2.

## Check 1 — refresh path keeps the session

### HTTP-level (curl)

Baseline, fleet-service up — `POST /api/auth/refresh` with the seeded cookie
returned `200` with a fresh `accessToken`/`refreshToken`, confirming the
seeding was correct.

With fleet-service stopped (`docker compose stop fleet-service`), presenting
the rotated cookie from baseline:

```
STATUS=503
HTTP/1.1 503 Service Unavailable
Content-Length: 91
Content-Type: application/vnd.api+json
Date: Mon, 03 Aug 2026 11:59:09 GMT
Retry-After: 5
Set-Cookie: refresh_token=<TOKEN-B>; Path=/; Expires=Wed, 02 Sep 2026 11:59:09 GMT; HttpOnly; SameSite=Lax

{"errors":[{"status":"503","code":"service_unavailable","title":"internal server error"}]}
```

Verified:
- `Retry-After: 5` present.
- `Set-Cookie` value is non-empty and **different** from the presented cookie
  (rotated) — `<TOKEN-A>` → `<TOKEN-B>`.
- **Not** the clearing form: `Expires=Wed, 02 Sep 2026...` (30 days out), no
  `Max-Age=0`, no past `Expires`.
- Body: `status: "503"`, `code: "service_unavailable"`,
  `title: "internal server error"`, **no `detail` key**, and
  `grep -iE "membership|lookup|<user-id>"` against the body found **nothing**
  — redaction confirmed.

**PASS** (HTTP level).

### Real Chromium (browser half)

Method note: navigating client-side (rather than a full page reload) was
required so the pre-existing, task-018-unmodified `AuthContext.tsx`
`useEffect` (which clears the token on `useMe()` erroring) did not confound
the result — confirmed out of scope via `git diff main...HEAD --stat`
(`AuthContext.tsx` is not in the diff). The trigger used was the header
theme-toggle button (`PATCH /api/auth/me`, an auth-service-backed mutation),
because navigating to a fleet-service-backed page with an expired token did
not reproduce a 401 at all — see `follow-ups.md` item 2 for why.

Steps: seed a fresh, unconsumed refresh-token row; open a fresh browser
context; set the `refresh_token` cookie directly; boot the SPA authenticated
with a valid access token; overwrite `localStorage.access_token` with an
**expired** token (`exp` one hour in the past); click the theme-toggle button
to fire `PATCH /api/auth/me`.

Captured network sequence (fleet-service stopped throughout):
```
[request] PATCH /api/auth/me
[response] 401 /api/auth/me            <- the expired token
[request] POST /api/auth/refresh
[response] 503 /api/auth/refresh       <- refresh has no Bearer, uses the cookie
```

Final state:
```
FINAL_PATHNAME=/
FINAL_TOKEN=<EXPIRED-ACCESS-TOKEN>   (unchanged — still the expired token, never cleared)
FINAL_TOKEN_MATCHES_EXPIRED=true
COOKIE_JAR_REFRESH_TOKEN=<TOKEN-C>   (rotated, non-empty)
COOKIE_JAR_REFRESH_TOKEN_CHANGED=true
```

`window.location.pathname` stayed `/` — no navigation to `/login`.
`localStorage.access_token` was still present (unchanged, not cleared by the
503). The browser's cookie jar picked up the server's rotated
`refresh_token` (confirmed via `context.cookies()`, since `Set-Cookie` is not
exposed to `fetch`'s JS-visible headers).

**PASS** (browser level).

## Check 2 — login page outage notice

Fresh browser context (no cookies, no localStorage). Navigated to
`http://localhost/login#error=service_unavailable`.

```
PATHNAME=/login
HASH=""
```
Fragment stripped on arrival — confirmed.

```
ALERT_COUNT=1
ALERT_TEXT="Sign-in is temporarily unavailable. Nothing was saved — try again in a moment."
```
Exact match to the expected copy, on the element carrying `role="alert"`.

```
ALERT_COMPUTED_STYLE={
  "backgroundColor":"rgb(254, 226, 226)",
  "color":"rgb(153, 27, 27)",
  "borderColor":"rgb(254, 202, 202)",
  "borderWidth":"1px",
  "role":"alert",
  "className":"rounded-md border border-danger-border bg-danger-subtle p-3 text-sm text-danger-subtle-foreground"
}
MUTED_COMPUTED_STYLE={"backgroundColor":"rgba(0, 0, 0, 0)","color":"rgb(100, 116, 139)"}
```
`getComputedStyle` on the alert element shows an actual reddish
background/border/text (the resolved `--danger-subtle` / `--danger-border` /
`--danger-subtle-foreground` CSS custom properties from
`apps/web/src/index.css`), visually distinct from a neutral
muted-foreground paragraph elsewhere on the same page. This is the CSS-only
assertion `jsdom` cannot make.

```
BUTTONS=["","Try again"]
```
The primary button's accessible text is **"Try again"** (the empty string is
the icon-only theme-toggle button, no text content) — not "Continue with
Google".

Screenshot: [`login-outage.png`](./login-outage.png) — the red banner above
the "Try again" button.

**PASS.**

## Check 3 — recovery

`docker compose start fleet-service`, waited for `health=healthy`.

`POST /api/auth/refresh` presenting the **exact cookie value written
alongside the 503** in Check 1
(`<TOKEN-C>`, captured from the browser's
cookie jar):

```
STATUS=200
Set-Cookie: refresh_token=<TOKEN-D>; Path=/; Expires=Wed, 02 Sep 2026 12:08:07 GMT; HttpOnly; SameSite=Lax
```
Body carried a fresh `accessToken` + `refreshToken`.

DB check on the family afterward — no row has `revoked_at` set:
```
                  id                  | revoked_at |          consumed_at
--------------------------------------+------------+-------------------------------
 db26faac-7b0a-4cae-9582-325ca1147687 |            |
 01ccf57d-0faf-41d0-9a4d-b40e2c21b626 |            | 2026-08-03 12:08:07.268321+00
 308e37ec-95f6-4638-8807-682a2b473dc9 |            | 2026-08-03 12:07:13.882549+00
 db93f0d6-d7f6-42c1-ba25-d4062833c82f |            |
```
The session was never revoked; reuse detection was never tripped. This is the
acceptance criterion the rotated-cookie-on-503 behavior exists to satisfy.

**PASS.**

## Out-of-scope findings

Two defects surfaced during this verification pass, neither caused by
task-018. See `follow-ups.md` for the full write-up:

1. `deploy/compose/docker-compose.yml`'s `web` service healthcheck/Traefik
   port mismatch (targets `80`, nginx binds `8080`).
2. `fleet-service` accepts an expired JWT.
