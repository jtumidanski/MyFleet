# Local debugging runbook

How to stand up MyFleet locally and debug UI defects against real services.
Written after a session where a green 514-test suite hid two browser-only bugs;
the parts that mattered are marked **why**.

---

## 1. Bring up the stack

```sh
cd deploy/compose
[ -f .env ] || cp .env.example .env
docker compose --env-file .env up -d --build
docker compose --env-file .env ps        # all services should be healthy
```

`make up` runs the same thing.

### `.env` values you must fill

| Key | Notes |
|---|---|
| `JWT_PRIVATE_KEY_PEM` | **PKCS#1**, double-quoted with `\n` escapes (below) |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Copy from `deploy/k8s/secrets.yaml` (gitignored) |
| `GOOGLE_REDIRECT_URL` | Must match a URI registered in the Google console **exactly** |
| `APP_BASE_URL` | Where auth-service sends the browser after login |

Generate the signing key:

```sh
openssl genrsa -traditional 2048 > /tmp/key.pem
```

**`-traditional` is required.** OpenSSL 3 defaults to PKCS#8
(`BEGIN PRIVATE KEY`), but `apps/auth-service/cmd/main.go` calls
`x509.ParsePKCS1PrivateKey`, which needs `BEGIN RSA PRIVATE KEY`.

Put it in `.env` as a **double-quoted** single line with literal `\n`:

```sh
JWT_PRIVATE_KEY_PEM="-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n-----END RSA PRIVATE KEY-----\n"
```

Compose's dotenv parser expands `\n` only inside double quotes.
`config.MustGet` does no unescaping, and `pem.Decode` needs real newlines.
Verify with `docker compose --env-file .env config | grep -A3 JWT_PRIVATE_KEY_PEM`
— you should see a block scalar, not one line of `\n`.

Keep the key **outside the repo**: `deploy/compose/.env` is gitignored,
`deploy/compose/*.pem` is not.

---

## 2. Frontend with hot reload

Run the UI from Vite, not the `web` container, so edits reload instantly.
`apps/web/vite.config.ts` already proxies `/api` → `http://localhost` (Traefik).

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run dev -w apps/web -- --host 127.0.0.1 --port 5173
```

Open <http://localhost:5173>. Backend calls flow
Vite → Traefik :80 → service.

---

## 3. Signing in

### Real Google login (preferred — exercises `/auth/me`, refresh, provisioning)

Add the callback to the OAuth client's **Authorized redirect URIs** in the
Google Cloud Console, then set `.env` to match:

```
GOOGLE_REDIRECT_URL=http://localhost:5173/api/auth/callback
APP_BASE_URL=http://localhost:5173
```

Google matches `redirect_uri` **character for character** — the port is part of
it, and `localhost` ≠ `127.0.0.1`. Test the URI without a browser:

```sh
LOC=$(curl -s -o /dev/null -w '%{redirect_url}' http://localhost:5173/api/auth/login/google)
curl -s -L "$LOC" | grep -oE '<title>[^<]*</title>|Error [0-9]+[^<"]{0,40}'
```

`Error 400: redirect_uri_mismatch` → not registered.
`<title>Sign in - Google Accounts</title>` → good.

### Minted token (no Google round-trip; handy for seeded fixtures)

Services validate signature + expiry only
(`packages/shared-go/auth/middleware.go`), so a token signed with the same key
is indistinguishable from a real one. Sign with `openssl` — no crypto library
needed:

```python
# header {"alg":"RS256","typ":"JWT","kid":"kid-1"}
# claims {"sub","email","active_fleet_id","role","iss":"myfleet-auth",
#         "aud":"myfleet","iat","exp"}
# sig = openssl dgst -sha256 -sign key.pem  over  b64(header).b64(claims)
```

Give it a long expiry (hours). The real TTL is 15 minutes and this path has no
refresh cookie, so a short token strands you mid-session. Visit
`http://localhost:5173/#access_token=<jwt>` — the SPA stores it and strips the
fragment.

Seed a user by hand (`auth.users`, `fleet.fleets`, `fleet.fleet_memberships`;
`role='owner'`, `status='active'`), then create vehicles/photos through the API
so the normal write paths run.

---

## 4. Driving a real browser

**jsdom cannot see CSS.** It loads no stylesheet, so a Tailwind variant can
never match there. A test suite that is entirely jsdom will happily stay green
while a class makes the whole UI unclickable. When a defect is "works in tests,
broken in the browser", get a real browser.

Playwright's own Chromium needs system libs that require root here. Use the
Playwright container instead — it has everything and `--network host` reaches
both the dev server and Traefik:

```sh
mkdir -p /tmp/drive && cd /tmp/drive && npm init -y && npm i playwright@latest
# write your script, then:
docker run --rm --network host -v /tmp/drive:/tmp/drive -w /tmp/drive \
  mcr.microsoft.com/playwright:v1.62.1-noble node script.mjs
```

Match the image tag to the installed `playwright` version.

### The diagnostic that ends "why doesn't this click?"

`document.elementFromPoint` at the element's centre tells you exactly which
element the browser hands the event to:

```js
const el = document.querySelector('[cmdk-item]');
const r = el.getBoundingClientRect();
const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
console.log('hit is the target?', el.contains(hit) || hit === el);
// then walk el.parentElement upward logging getComputedStyle(x).pointerEvents
```

If the hit is an ancestor rather than the target, something above it is
intercepting — or the target itself is `pointer-events: none`.

### Gotchas that cost time

- **`data-[foo]:` in Tailwind is attribute-PRESENCE.** React stringifies
  `data-*`, so `data-disabled={false}` renders `data-disabled="false"` and
  the variant matches. cmdk does this; Radix omits the attribute instead.
  Gate on the value (`data-[disabled=true]:`) for cmdk, presence for Radix.
- **A modal Radix Popover `aria-hidden`s the page behind it**, so Playwright's
  `getByRole` cannot see the trigger while it is open. Use a CSS locator.
- **`/login` redirects when authenticated** — test it in a fresh context.
- Always `.trim()` file reads before `.split(/\s+/)`, or a trailing newline
  yields an empty id and the app correctly shows a different page.

---

## 5. Verifying a fix

1. **Reproduce first.** If you cannot reproduce it, you cannot know you fixed it.
2. **Check the test can fail.** Revert the fix, watch it go red, restore it.
   A test that passes both ways proves nothing.
3. **Assert on stored state, not the response.** `Administrator.Update` returns
   a model built in memory; it agreed with the caller even when the write was
   dropped. Re-read through the Provider.
4. **Go end to end.** A frontend + handler fix tested green while the DB write
   was still a no-op three layers down.

```sh
make ci   # lint-check, vet, test, build, fe-test, fe-build, manifests
```

---

## 6. Known local-stack traps

| Symptom | Cause |
|---|---|
| Every `/api/*` route 404s, all containers healthy | Traefik's Docker provider pinned client API v1.24; daemons ≥ Docker 25 refuse it. Pinned to `traefik:v3.7.10`. Check `docker compose logs traefik \| grep -i error`. |
| Cookies rejected over plain HTTP | `COOKIE_SECURE` unset → auth-service defaults to `true`. Compose passes it now. |
| `panic: required env var "GOOGLE_CLIENT_ID" is not set` | auth-service needs it even when bypassing OIDC; any placeholder boots it. |
| `500` on create with a bogus `categoryId` | Neither POST nor PATCH validates category existence; the FK raises. Expect `422`, get `500`. Not yet fixed. |

Useful endpoints: Traefik dashboard <http://localhost:8081>, Mailpit
<http://localhost:8025/mail>, MinIO console <http://localhost:9001>.
