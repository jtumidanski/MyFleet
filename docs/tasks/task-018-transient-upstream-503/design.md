# Transient Upstream Failures Must Not Log Users Out — Design

Task: task-018 · PRD: [`prd.md`](./prd.md) · Issue: [#15](https://github.com/jtumidanski/MyFleet/issues/15)
Status: Draft for review

---

## 1. Shape of the change

One fact — *"the answer is unknown because something upstream is unavailable"* —
has to travel from the place it is known (`membership.Client.Active`, which sees
the status code) to four places that must act on it:

| Consumer | Action on transient |
| --- | --- |
| `session.refreshHandler` | `503` + `Retry-After: 5`, **preserve** the (rotated) refresh cookie |
| `oidc.callbackHandler` | `302 … #error=service_unavailable` |
| `packages/shared-go/server` | map to `503`, emit the header, keep the body redacted |
| `apps/web` | do not clear the access token, do not redirect to `/login` |

Everything else in the task follows from choosing the carrier for that fact and
resolving the rotation-vs-cookie ordering problem (PRD FR-REFRESH-5). Those are
Decisions 1 and 3 below; the rest is mechanism.

Total surface: ~6 Go files, ~4 TS files, no schema, no manifests, no
`fleet-service` change.

---

## 2. Decisions

### D1 — The classification carrier is `server.ErrServiceUnavailable`

`membership.Client.Active` wraps its transient failures with the same sentinel
`packages/shared-go/server` is gaining for FR-SHARED-1:

```go
return Membership{}, fmt.Errorf("%w: active membership lookup failed with status %d",
    server.ErrServiceUnavailable, res.StatusCode)
```

Callers classify with `errors.Is(err, server.ErrServiceUnavailable)`.

**Alternatives considered.**

*(a) A `membership.ErrUpstreamUnavailable` sentinel.* Rejected: FR-CLASSIFY-7
requires `session` and `oidc` to classify **without importing the concrete
membership client** — the whole point of injecting `session.PrincipalResolver`
is that those packages stay decoupled from it. A sentinel in `membership` would
force exactly the import the injection exists to prevent.

*(b) A new `packages/shared-go/upstream` package holding one sentinel.*
Rejected as a package for a single `var`, with no second consumer in sight.

*(c) The chosen option.* `session`, `oidc` and `membership` all already depend on
`shared-go/server` (or trivially can — `session/resource.go:12` does today), so
the sentinel is reachable from every consumer with no new coupling.

**Is it wrong for an outbound client to import an HTTP-*response* package?** The
codebase has already answered no: `apps/fleet-service/internal/mediaclient/client.go:87`
wraps a failed upstream call with `server.ErrValidation` for exactly this
reason. `shared-go/server` is this project's shared error *vocabulary*, not just
its response writer, and following the established pattern beats inventing a
second one.

**The bonus that falls out.** `StatusFor` maps the sentinel to `503`
automatically, so any handler that forwards a wrapped resolver error verbatim
gets the right status with no new mapping code. The refresh handler still
branches explicitly — it has to, for the cookie — but nothing silently
mis-statuses if a future caller forwards the error.

### D2 — `Retry-After` rides on a wrapper error, mirroring `Detailed`

```go
// errors.go
func RetryAfter(base error, seconds int) error { return &retryAfterError{base: base, seconds: seconds} }

type retryAfterError struct {
    base    error
    seconds int
}

func (e *retryAfterError) Error() string   { return e.base.Error() }
func (e *retryAfterError) Unwrap() error   { return e.base }
func (e *retryAfterError) RetryAfter() int { return e.seconds }
```

`WriteError` picks it up through an anonymous-interface `errors.As`, exactly as
it already does for `Detail() string` (`jsonapi.go:108-111`), and sets the header
**before** `WriteJSON` commits the header block:

```go
func WriteError(w http.ResponseWriter, err error) {
    status := StatusFor(err)
    // Before WriteJSON: WriteJSON calls WriteHeader, after which header
    // mutations are silently discarded.
    var ra interface{ RetryAfter() int }
    if errors.As(err, &ra) && ra.RetryAfter() > 0 {
        w.Header().Set("Retry-After", itoa(ra.RetryAfter()))
    }
    …unchanged…
}
```

`> 0` guard: a zero or negative value is a caller bug, and emitting
`Retry-After: 0` would tell an intermediary to hammer immediately. Silently
omitting the header is the safe failure.

**Alternative rejected:** a parallel `WriteErrorWithRetryAfter(w, err, secs)`
entry point. FR-SHARED-3 already steers away from it, and the reason is sound —
the header is a property *of the error*, so it belongs on the error value where
it can be constructed once at the point of knowledge and carried through any
number of intermediate returns. A second entry point also doubles the surface
every future response concern (`Warning`, `Location`) would have to be added to.

`RetryAfter` and `Detailed` compose in either order; `errors.As` walks the whole
chain. Not needed here — a `503` emits no `detail` (FR-SHARED-4) — but it means
neither wrapper has to know about the other.

### D3 — FR-REFRESH-5: write the rotated cookie, *then* `503`

This is the highest-risk item in the task. The two candidates from PRD Open
Question 1:

**Option A — resolve before rotating.** Reorder the handler so `resolve` runs
first and `proc.Rotate` only runs on success. Nothing is consumed, so the
browser's cookie stays valid by construction.

**Option B — keep the ordering; write the rotated value to the cookie before
responding `503`.** ✅ **Chosen.**

```go
principal, err := resolve(req.Context(), userID)
if err != nil {
    if errors.Is(err, server.ErrServiceUnavailable) {
        log.WithError(err).Warn("resolve principal on refresh: upstream unavailable")
        // The rotation already committed to the store. Persist the new value to
        // the browser or the next attempt replays a consumed token, which
        // Processor.Rotate treats as reuse and answers by revoking the whole
        // family — the logout this entire task exists to prevent.
        SetRefreshCookie(w, newRaw, cookieSecure)
        server.WriteError(w, server.RetryAfter(err, refreshRetryAfterSeconds))
        return
    }
    log.WithError(err).Error("resolve principal on refresh")
    clearRefreshCookie(w, cookieSecure)
    server.WriteError(w, server.ErrUnauthorized)
    return
}
```

**Why B over A.**

1. **A moves an outbound HTTP call ahead of the token-validity check on a
   public, unauthenticated endpoint.** `POST /auth/refresh` takes whatever cookie
   the caller presents. Today a garbage token costs one indexed hash lookup and
   returns `401`. Under A it would cost a `fleet-service` round trip — up to the
   full 5s timeout budget — *before* anyone establishes the caller holds a valid
   token. That is a request-amplification vector against the exact upstream this
   task is trying to protect, and it puts a 5-second handler on an unauthenticated
   path.
2. **A needs a validate-without-consuming path that does not exist.** `resolve`
   needs a `userID`, which only `Rotate` produces. A would require a new
   `Processor` method doing `FindByHash` + reuse + expiry checks without
   consuming, after which `Rotate` repeats the same lookup — two round trips on
   the success path, plus a TOCTOU window between validation and rotation that
   the single-statement version does not have.
3. **B's mechanism is unremarkable.** `Set-Cookie` is honoured on any response
   status; the SPA's `fetch(…, { credentials: 'include' })` stores it like any
   other. Nothing about `503` is special here.

**Consequence, stated explicitly (PRD asks for this).** After a `503` the store
and the browser both hold the *new* token. The retry once `fleet-service`
recovers presents that new token, `Rotate` finds it unconsumed, and the refresh
succeeds. Reuse detection is never engaged and the family is never revoked —
acceptance criterion 4.

**Residual risk, and why it is not a regression.** If the `503` response never
reaches the browser (connection dropped mid-response), the browser keeps the old
value while the store has advanced, and the *next* attempt does trip reuse. That
is identical to what already happens when a `200` is lost in transit — the
rotate-then-respond shape has always had this property. B introduces no new
failure mode; A would remove it only for this one branch while adding the two
costs above.

**Rejected variant of B:** clearing nothing at all and skipping `SetRefreshCookie`
(i.e. today's cookie behaviour minus the clear). That satisfies FR-REFRESH-2's
letter — the cookie is not cleared — while guaranteeing the family-revoking
replay the criterion forbids. It is the trap FR-REFRESH-5 was written to catch.

### D4 — `Active`'s classification rules

Rewritten error handling, in order:

| Condition | Result | Transient? |
| --- | --- | --- |
| `http.NewRequestWithContext` error | returned bare | **no** — a malformed base URL is our bug, retrying cannot fix it |
| `hc.Do` error | wrapped | **yes** — covers connection refused, DNS, TLS, *and* deadline exceeded |
| `404` | `(Membership{}, nil)` | n/a — unchanged, load-bearing (FR-CLASSIFY-4) |
| `>= 500` | wrapped, message keeps the status | **yes** |
| `429` | wrapped, message keeps the status | **yes** |
| other non-2xx (`400`, `403`, …) | today's bare `fmt.Errorf` | **no** (FR-CLASSIFY-3) |
| 2xx, JSON decode fails | wrapped, fixed text | **yes** (FR-CLASSIFY-5) |
| 2xx, decodes | `(m, nil)` | n/a |

Wrapping `hc.Do`'s error covers FR-TIMEOUT-2 without a separate
`errors.Is(err, context.DeadlineExceeded)` test: the timeout surfaces as a
`*url.Error` out of `Do`, which is already in the transient bucket. A caller
cancellation (`context.Canceled`, browser navigated away) lands there too and
also yields `503` — harmless, since nobody is listening.

**FR-CLASSIFY-6 (no widening).** Every wrap uses `%w` plus *our* fixed text and,
where relevant, the numeric status. The upstream body is never read into a
message, and `userID` never appears in one. The `hc.Do` case is the one place an
upstream-influenced string (the URL, from config; the transport error, from Go)
enters the chain — it already did before this change, it never reaches a response
body (`WriteError` redacts everything `>= 500`), and it goes only to the error
log.

### D5 — One timeout constant, covering both calls

`fleetMemberLookupTimeout` (`client.go:65`) is renamed `fleetLookupTimeout`,
moved above `Active`, and applied to `Active` with the same
`context.WithTimeout` / `defer cancel()` shape `FleetMemberIDs` already uses. Two
call sites, one constant, one literal `5 * time.Second` in the package —
FR-TIMEOUT-1's "prefer a single shared constant". The doc comment moves with it
and drops the name of the single function it used to describe.

`Client` keeps `http.DefaultClient`. Per-request context deadlines bound the hop
just as well as `http.Client.Timeout` and, unlike it, do not mutate a shared
global other packages may be using.

### D6 — SPA: throw a typed error, change no signature

PRD Open Question 2. Chosen shape:

```ts
// refresh.ts — internal
type RefreshOutcome =
  | { status: 'ok'; token: string }
  | { status: 'dead' }         // 401, unparseable body, missing token, network error
  | { status: 'unavailable' }; // 503

let inflight: Promise<RefreshOutcome> | null = null;
```

- `requestToken` returns `{ status: 'unavailable' }` on `res.status === 503` and
  `{ status: 'dead' }` on every other non-`ok` (and on `catch`).
- **The dedupe is unchanged in kind.** `inflight` still holds a promise that
  *resolves* — never rejects — so every concurrent caller collapses onto one
  POST and `.finally` clears the slot exactly as today. This is why the outcome
  is a value rather than a rejection at this layer: a rejected shared promise is
  where unhandled-rejection and double-settle bugs live.
- `mintAccessToken(): Promise<string | null>` — **signature and behaviour
  unchanged.** `ok` → token, everything else → `null`, never clears (FR-SPA-7).
  Its two callers (`invites.ts:154`, `members.ts:85`) are untouched.
- `refreshAccessToken(): Promise<string | null>` — `ok` → token; `dead` →
  `clearAccessToken()` then `null` (today's behaviour); `unavailable` → **throw**
  `new ApiError(503, 'service_unavailable', 'Service temporarily unavailable')`
  and **do not clear** (FR-SPA-3).

**`packages/shared-ts` changes: none.** This is the point of the choice. A
rejection from `await this.opts.onRefresh()` (`apiClient.ts:36`) propagates out
of `fetchAuthenticated`, out of `request` / `requestBlob`, and reaches the caller
as an `ApiError` with `status: 503` — which *is* FR-SPA-6, with no code written.
`onRefresh: () => Promise<string | null>` — public surface — needs no widening,
so FR-SPA-5 holds by construction: the one-shot `401` contract is not touched and
no retry loop is introduced.

**Alternatives rejected.**

*Widen `onRefresh` to return a result object.* Breaks a published type in
`shared-ts` and forces every implementor to handle a third case, to express
something the existing rejection channel already carries.

*Return `null` and set a module-level "was unavailable" flag.* Action at a
distance; racy under the very concurrency the dedupe exists to manage.

*Add a `retryable: boolean` to `ApiError`.* PRD §7 floats it as a maybe. YAGNI —
`status === 503` is the same predicate with no new public field, and no consumer
needs to distinguish a retryable `503` from any other `503`.

**Known consequence.** `AppProviders.tsx:37` sets `queries: { retry: 1 }`, so a
query that hits this path retries once — at most one extra `/auth/refresh` POST
per affected query, several seconds later. Bounded and consistent with the
"no retry storm" NFR; mutations default to no retries. Not worth special-casing.

**Existing tests keep passing.** `refresh.test.ts`'s `jsonResponse` fake carries
no `status` field, so `res.status === 503` is `undefined === 503` → `false` →
`dead`, which is what those tests already assert.

### D7 — Login notice tone is `danger`, not `neutral`

PRD Open Question 3 guesses `'neutral'` is "the pragmatic answer". Reading
`LoginPage.tsx` says otherwise: `tone` is not purely cosmetic there.

```ts
const failed = notice?.tone === 'danger';   // LoginPage.tsx:44
```

`failed` drives three things — `role="alert"`, the danger-band styling, and the
primary button's label (`'Try again'` vs `'Continue with Google'`,
`LoginPage.tsx:105`). For a transient outage, two of those three are exactly
right: the user *should* be told sign-in failed, and the button *should* say
"Try again". `'neutral'` is reserved for `cancelled` — a deliberate user choice
with no retry implied — and would leave an outage rendering as muted body text
under a "Continue with Google" button, understating a failure the user has to
act on.

So: `service_unavailable` gets `tone: 'danger'`, and the message carries the
nuance the colour cannot:

> "Sign-in is temporarily unavailable. Nothing was saved — try again in a moment."

Accepted cost: the band is red for something that is not the user's fault. The
alternative — a third tone, or splitting `failed` off `tone` into its own field —
is a design-system change the PRD explicitly puts out of remit, for a purely
visual gain.

`NOTICES` keys on `LoginErrorCode`, so adding the union member makes the compiler
demand the entry; `CODES` is a hand-maintained `readonly string[]` and must be
updated in the same edit or `isLoginErrorCode` silently degrades the new code to
`server_error` (`loginError.ts:58`). That degradation is also what makes shipping
the backend first safe (FR-SPA-1).

### D8 — `newPrincipalResolver` needs no change; it needs a test

PRD §7 says the resolver "must propagate classification through its wrapping."
It does not wrap — `main.go:181-183` returns the error bare — so `errors.Is`
already reaches through it and FR-CLASSIFY-7 is satisfied as-is.

The risk is not today's code, it is tomorrow's: someone adding
`fmt.Errorf("resolve membership: %v", err)` (`%v`, not `%w`) silently breaks
every classification downstream, and nothing fails. So the change here is a
**regression test**, not an edit: with a stub membership client returning a
`server.ErrServiceUnavailable`-wrapped error, assert
`errors.Is(resolverErr, server.ErrServiceUnavailable)`.

### D9 — Local infrastructure failures: recommended extension, easy to cut

`newPrincipalResolver` calls three things. Only one of them is covered by the
PRD's literal text:

| Call | Failure | Today | PRD says |
| --- | --- | --- | --- |
| `users.GetByID` | `user.ErrNotFound` | `401` + clear | correct, keep (FR-CLASSIFY-8) |
| `users.GetByID` | Postgres error | `401` + clear | *silent* |
| `fleet.Active` | upstream down | `401` + clear | **`503`, keep cookie** |
| `admins.IsAdmin` | Postgres error | `401` + clear | *silent* |

Rows 2 and 4 are the same defect the task exists to fix, one layer over. An
`auth` Postgres blip lasting longer than one request logs out every active
session, for the same reason and through the same code path.

**Recommendation: classify them transient too.** In `newPrincipalResolver`:

```go
u, err := users.GetByID(userID)
if err != nil {
    if errors.Is(err, user.ErrNotFound) {
        return session.Principal{}, err   // permanent: the credential is dead
    }
    return session.Principal{}, fmt.Errorf("%w: user lookup failed", server.ErrServiceUnavailable)
}
```

…and the same wrap on `admins.IsAdmin`'s error. Note the wrap deliberately drops
the driver's text from the chain rather than `%w`-ing it, so a SQLSTATE or table
name cannot ride into the error even in principle; the underlying error is
logged at the point of failure instead.

This does not weaken anything: token validity is decided by `Rotate`, which runs
*before* the resolver, so a `503` here still cannot keep a revoked or expired
session alive (NFR "503 MUST NOT become a way to keep a dead session alive"). And
a permanent DB fault misclassified as transient just means the user retries and
gets another `503` — a working session preserved, which is the correct answer for
a still-valid token.

**This is an extension beyond the PRD's stated scope, flagged deliberately.** It
is confined to one function and two `if` blocks; if it should be cut, cut it here
and the rest of the design is unaffected. It is recommended rather than assumed
because shipping the fix while leaving "auth's own database hiccups log everyone
out" in place would be a strange place to stop.

### D10 — `403` and `429` (PRD Open Questions 4 and 5)

**`429` folds into transient**, as FR-CLASSIFY-2 directs. No passthrough of the
upstream's `Retry-After`: `fleet-service` does not rate-limit its internal
endpoints today, so there is nothing to pass through, and relaying an upstream
retry policy to an unauthenticated caller leaks internal posture for no gain. The
fixed `5` stands.

**`403` stays permanent and needs no new machinery.** The existing permanent-path
log line already carries the status in its message ("…failed with status 403"),
so an operator grepping the resolver-failure log sees `403` distinctly from
`user.ErrNotFound`. A dedicated alarm path is a monitoring concern, not a code
one, and adding a third classification tier to serve a case that has never fired
is speculative.

### D11 — Adjacent hygiene: escape `user_id`

`Active` builds its URL by raw concatenation (`client.go:27`) — and
`FleetMemberIDs`'s comment (`client.go:80-82`) explicitly calls this out as the
habit not to inherit. Since `Active` is being rewritten anyway, it uses
`url.QueryEscape(userID)`. `userID` is an internal UUID from a validated token,
so this is not a live vulnerability; it is one line that stops the file
contradicting its own comment. No behaviour change.

---

## 3. Component design

### `packages/shared-go/server`

**`errors.go`** — add the sentinel and the mapping:

```go
ErrServiceUnavailable = errors.New("service unavailable")  // 503
…
case errors.Is(err, ErrServiceUnavailable):
    return 503
```

plus the `RetryAfter` wrapper from D2. **`server.go`** — `codeFor` gains
`case 503: return "service_unavailable"` (FR-SHARED-2). **`jsonapi.go`** —
`WriteError` gains the four-line header block from D2.

Nothing else moves. The `>= 500` branch keeps `InternalErrorTitle`, keeps
emitting no `detail`, and keeps logging (FR-SHARED-4). Every existing caller is
unaffected: `StatusFor`'s new case is reachable only from the new sentinel, and
`codeFor`'s new case only from status `503`, which nothing previously produced
(FR-SHARED-6).

### `apps/auth-service/internal/membership/client.go`

`Active` gains the timeout (D5), the classification table (D4), and the escape
(D11). `FleetMemberIDs` changes only by the constant rename — its callers do not
need the distinction and it does not get one.

### `apps/auth-service/cmd/main.go`

No change under the PRD's literal scope (D8); two wraps under D9.

### `apps/auth-service/internal/session/resource.go`

The resolver-error branch splits per D3. Adds `errors` to imports and a package
constant:

```go
// refreshRetryAfterSeconds matches fleetLookupTimeout and the restart timescale
// of a fleet-service pod. Advisory: the SPA does not auto-retry.
const refreshRetryAfterSeconds = 5
```

Every other exit — missing token, `Rotate` failure, mint failure — is byte-for-byte
unchanged (FR-REFRESH-7).

**Logging (FR-REFRESH-6).** Transient logs at `Warn` with
`"resolve principal on refresh: upstream unavailable"`; permanent stays at `Error`
with `"resolve principal on refresh"`. Distinct level *and* distinct message, so
an outage is greppable and does not inflate the error rate. `WriteError` also
emits its standard `>= 500` line for the `503` — accepted duplication: that line
is generic by design and carries no idea which handler produced it, and
suppressing it would mean special-casing the shared writer.

### `apps/auth-service/internal/oidc/resource.go`

```go
errServiceUnavailable loginErrorCode = "service_unavailable"
```

and, in the `d.Resolve` branch only (`resource.go:262-267`):

```go
if errors.Is(err, server.ErrServiceUnavailable) {
    log.WithError(err).Warn("resolve principal on callback: upstream unavailable")
    failLogin(w, req, d, errServiceUnavailable)
    return
}
```

Every other `failLogin` call site keeps `errServerError` (FR-CALLBACK-2). The
state cookie handling is not touched — the single unconditional clear at
`resource.go:228` stays exactly where it is, and `failLogin` keeps not clearing
(FR-CALLBACK-3; the comment at `resource.go:95-120` explains why and says not to
"fix" it).

### `packages/shared-ts`

No change (D6).

### `apps/web`

`lib/api/refresh.ts` per D6. `lib/auth/loginError.ts` per D7 — union member,
`CODES` entry, `NOTICES` entry with its own message and `tone: 'danger'`.
`LoginPage.tsx` unchanged: it already renders whatever `noticeFor` returns.

---

## 4. Flows

**Transient — `fleet-service` down, refresh path**

```
SPA ── POST /api/auth/refresh (cookie: old) ─────────────▶ auth-service
                                       Rotate: old consumed, new inserted ✔
                                       resolve → Active → 500 / refused / 5s timeout
                                       wrap: %w ErrServiceUnavailable
                                       ◀── 503 · Retry-After: 5
                                           Set-Cookie: refresh_token=NEW
SPA: refreshAccessToken throws ApiError(503) · access token NOT cleared
     isAuthenticated stays true · RequireAuth does not navigate
     original request rejects with status 503
… fleet-service recovers …
SPA ── POST /api/auth/refresh (cookie: NEW) ──────────────▶ 200, no reuse
```

**Permanent — user row gone** — unchanged: `401`, cookie cleared, SPA clears the
token, `RequireAuth` navigates to `/login`.

**Transient — callback path** — `302` to `…/login#error=service_unavailable`;
`consumeLoginError` reads and strips it; the login page shows the try-again-shortly
message with a "Try again" button. No refresh token was issued, so there is no
cookie question (FR-CALLBACK-4).

---

## 5. Response matrix

| Path | Condition | Status | `Retry-After` | Cookie |
| --- | --- | --- | --- | --- |
| refresh | success | `200` | — | rotated |
| refresh | no token presented | `401` | — | untouched |
| refresh | rotate failed (unknown/expired/reuse) | `401` | — | cleared |
| refresh | resolver: user row gone | `401` | — | cleared |
| refresh | resolver: upstream `4xx` (≠404/429) | `401` | — | cleared |
| refresh | **resolver: upstream unavailable** | **`503`** | **`5`** | **rotated value written** |
| refresh | mint failed | `401` | — | untouched |
| callback | resolver: upstream unavailable | `302` | — | n/a |
| callback | anything else | `302` `#error=server_error` etc. | — | n/a |

---

## 6. Testing

Per the repo memo on vacuous assertions: assert **observable state**, and prove
each new test can fail by reverting the fix before restoring it.

**`shared-go/server`** — extend the existing sentinel→status and status→code
table tests with `503` / `service_unavailable` (FR-SHARED-5). New: `WriteError`
sets `Retry-After: 5` for a `RetryAfter`-wrapped error; omits the header for a
plain error; the `503` body still carries `InternalErrorTitle` and no `detail`.
The header assertion must read `rec.Header().Get("Retry-After")` *after*
`WriteError` returns — that is precisely the assertion that fails if the header
is set after `WriteJSON`.

**`membership.Client.Active`** — table-driven over `httptest.Server`: transport
error (closed listener), timeout (handler sleeps past the deadline — with the
constant temporarily lowered or the server's delay tuned so the test is fast),
`500`, `429`, `403`, `404`, malformed 2xx body, success. Each row asserts
`errors.Is(err, server.ErrServiceUnavailable)` equals the expected classification
— **not merely that an error occurred** — plus the returned `Membership` for the
`404` and success rows.

**`newPrincipalResolver`** — `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError`
must pass **unmodified**. New: classification survives the resolver (D8); and if
D9 lands, a Postgres-style `GetByID` error classifies transient while
`user.ErrNotFound` does not.

**`refreshHandler`** — for both branches assert **status, `Retry-After`, and the
`Set-Cookie` headers**. Specifically, on the transient branch: status `503`,
`Retry-After: 5`, and a `refresh_token` cookie whose value equals the rotated
token with a non-negative `Max-Age` — *not* the `MaxAge=-1` clearing form. And
that the store's new token row is still unconsumed, so a follow-up `Rotate` with
that value succeeds rather than revoking the family (acceptance criterion 4).
Status-only assertions pass while still logging the user out; that is the exact
failure mode the criterion calls out.

**`oidc` callback** — transient resolver error → `Location` ends
`#error=service_unavailable`; permanent → `#error=server_error`; the state cookie
behaviour is identical in both.

**`refresh.ts`** — `503` → `refreshAccessToken` rejects with an `ApiError` of
status `503` **and `getAccessToken()` still returns the prior token**; `401` →
resolves `null` and the token is cleared; `503` → `mintAccessToken` resolves
`null` and does not throw and does not clear; two concurrent calls during a `503`
produce exactly one `fetch`.

**`loginError.ts`** — `service_unavailable` survives `isLoginErrorCode` and maps
to its own message, distinct from `GENERIC_FAILURE`.

**Browser** — `jsdom` cannot see CSS, and two of the acceptance criteria are
user-visible. Against the local stack (`docs/runbooks/local-debugging.md`), with
`fleet-service` scaled to zero: confirm the app does **not** bounce to `/login`
on a refresh `503`, and that a callback during the outage renders the
try-again-shortly notice with the "Try again" button.

**Gates** — `make ci`; both overlays rendered and both server dry-runs green (no
manifest change expected — this is the standing gate); all three reviewer agents
before the PR.

---

## 7. Risks

| Risk | Mitigation |
| --- | --- |
| `Retry-After` set after `WriteHeader` → silently dropped | Header block precedes `WriteJSON` in `WriteError`; a test reads the header off the recorder |
| A future `%v` wrap in the resolver breaks classification silently | D8's regression test asserts `errors.Is` through the resolver |
| `503` used to keep a dead session alive | Unreachable: `Rotate` decides token validity and runs first; the branch is entered only on an upstream/infrastructure error |
| Lost `503` response → next attempt trips reuse | Pre-existing property of rotate-then-respond, identical on the `200` path; documented in D3, not introduced here |
| Timeout test is slow or flaky | Drive it off a short deadline against a deliberately sleeping `httptest` handler, not a 5-second wall-clock wait |
| `CODES` in `loginError.ts` not updated alongside the union | The new code degrades to `server_error` rather than breaking — but a unit test asserts the mapping directly |

## 8. Explicitly out of scope

Retry/backoff/circuit-breaking in `membership.Client`; client-side auto-retry or
"reconnecting…" UI; any `fleet-service` change; the `404 → (Membership{}, nil)`
contract; #14's fail-closed decision; a third `LoginErrorNotice.tone`; a
`retryable` field on `ApiError`; `FleetMemberIDs` classification.
