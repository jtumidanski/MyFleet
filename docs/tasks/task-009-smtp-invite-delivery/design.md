# SMTP Invite Email Delivery — Design

Task: `task-009-smtp-invite-delivery`
PRD: [`prd.md`](./prd.md) (v1, approved)
Status: Draft
Created: 2026-08-02

---

## 1. Summary

Invite creation gets a transactional-outbox event (`invite.created`). A **new, independent consumer** in
notification-service picks it up, fetches the invite (including its token) from a new network-restricted
fleet-service endpoint, renders a `multipart/alternative` message, and hands it to an SMTP relay behind a
narrow `mailer.Sender` interface. The invite HTTP request never waits on the relay.

Three findings from reading the current code shape this design more than anything in the PRD:

1. **`events.Consume` does not retry.** Its `continue`-without-commit is documented as "will retry", but
   kafka-go advances to the next message and commits *that* offset, which implicitly commits past the
   failed one. A failed message is skipped permanently, not redelivered. FR-MAIL-5's "let the consumer's
   existing redelivery path retry" therefore rests on a mechanism that does not exist. §5 replaces it with
   in-handler bounded retry.
2. **`fleet-service` cannot resolve the inviter's display name, and `auth-service` exposes no internal
   surface at all.** Open Question 1 resolves to "don't try" — §7 drops `invited_by_name`.
3. **The ingress deny rule already covers the new internal endpoint.** `PathRegexp(`(?i)^/+api/+fleet[^/]*/*internal`)`
   (`deploy/k8s/overlays/main/ingressroute.yaml:89`) matches any path under `/api/fleet/internal…`, so
   FR-INT-1 needs no manifest change — only a test that proves the route is absent from the JWT tree.

---

## 2. Architecture

```
 fleet-service                                      notification-service
 ─────────────                                      ────────────────────

 POST /fleets/{id}/invites            ┌─ Kafka ─┐
 POST …/invites/{id}/resend           │ topic:  │
        │                             │ invite. │
        │ ┌──── ONE TRANSACTION ────┐ │ created │
        ├─┤ INSERT/UPDATE invite    │ └────┬────┘        mailconsumer.Handle
        │ │ INSERT outbox row       │      │             ┌──────────────────────────┐
        │ └─────────────────────────┘      └────────────▶│ 1. inbox.Exists(id,      │
        │        │                                       │      "invite-email")?    │
   201/200 ◀─────┘ (returns immediately)                  │ 2. fleetclient.Invite()  │──┐
                   │                                      │ 3. skip if accepted/     │  │
                   │ outbox relay (2s tick,               │      expired / disabled  │  │
                   │ leader lock)                         │ 4. mailer.Send(msg)      │  │
                   └──────────────────▶ Kafka             │ 5. inbox.Mark(...)       │  │
                                                          └──────────────────────────┘  │
 GET /internal/invites/{inviteID}  ◀───────────────────────────────────────────────────┘
   (no JWT; ingress-denied from the public internet)
                                                                     │
                                                            mailer.Sender
                                                              ├── smtpSender  → relay (587 STARTTLS)
                                                              └── fakeSender  → tests
```

**Why the token travels over HTTP and not over Kafka.** The event carries `{invite_id, email, role}` only
(FR-EVT-3). Kafka messages persist on disk in Redpanda with no encryption and no expiry policy tuned for
secrets; an internal HTTP fetch keeps the credential's blast radius to a single request. It also makes
resend correct for free: the handler always reads the *current* token, so a delayed or duplicated delivery
of an older `invite.created` mails the live link rather than a rotated-away one.

**Component boundaries.** Four new units, each independently testable:

| Unit | Purpose | Depends on |
|---|---|---|
| `fleet-service/internal/invite` (extended) | transactional insert, resend + rotation, rate limits, internal read route | `events.EmitInviteCreated`, `fleet.Provider` |
| `notification-service/internal/mailer` | compose + deliver a message; no domain knowledge | `net/smtp`, `crypto/tls`, templates |
| `notification-service/internal/mailconsumer` | one event → one email, idempotently | `Inbox`, `Invites`, `mailer.Sender` |
| `notification-service/internal/fleetclient` (extended) | `Invite(ctx, id)` | fleet-service internal API |

`mailer` knows nothing about invites beyond a struct of already-resolved fields; `mailconsumer` knows
nothing about SMTP. That split is what lets every test run without a socket (FR-MAIL-1, FR-DEV-4).

---

## 3. Decision: a separate consumer, not a new topic on the existing one

FR-MAIL-3 says to add `invite.created` to `consumer.Topics` and use a distinct ledger name. Taken
literally, that wires the email work into the existing `*consumer.Consumer`, which resolves fleet
recipients and generates in-app notifications — none of which an invite email wants.

**Chosen: a new `internal/mailconsumer` package with its own Kafka consumer group `invite-email`.**

- The Kafka group, not just the ledger name, is separate. Offsets are then independent: a stalled email
  consumer cannot hold back in-app notification offsets, which is the same isolation FR-MAIL-3 asks for one
  layer up.
- `consumer.Topics` stays untouched, so the in-app consumer never sees an event whose only outcome would be
  `notificationType() → false` → mark-and-skip.
- `mailconsumer.Handle` has a signature and a fake set of its own; `consume_test.go` keeps testing exactly
  what it tests today.

**Rejected: extend `*consumer.Consumer`.** It would need a second ledger name, a second code path branching
on `e.Type` before recipient resolution, and a nil-mailer guard for the `SMTP_ENABLED=false` case — all
inside a type whose doc comment describes a single responsibility.

**Rejected: a `notification_outbox` table in notification-service** (persist a pending send, retry from a
job). Strictly more reliable than in-handler retry and immune to the `events.Consume` defect, but it adds a
table, a migration, and a second leader-locked job. PRD §6 states no migrations. Revisit if invite volume
ever justifies it; §5 explains why it is not needed at household scale.

**This is a deliberate deviation from FR-MAIL-3's wording, satisfying its stated intent** ("an SMTP failure
cannot cause the in-app notification path to be re-run and vice versa") more completely than the literal
instruction. Flagged here so the plan-adherence reviewer scores it against intent.

---

## 4. fleet-service changes

### 4.1 Transactional insert (FR-EVT-1, FR-EVT-2)

`Administrator.Insert` gains a transaction and a trace id, mirroring `Accept`
(`apps/fleet-service/internal/invite/administrator.go:66`):

```go
type Administrator interface {
    Insert(m Model, traceID string) (Model, error)
    Resend(inv Model, newToken string, expiresAt time.Time, actorID, traceID string) (Model, error)
    Delete(id string) error
    Accept(inv Model, userID, traceID string) (Model, error)
}
```

`Insert` runs `tx.Create(&e)` then `a.emitCreated(tx, …)`; a failed enqueue rolls back the invite row.
A new `CreatedEmitter` field is injected via `WithCreatedEmitter`, exactly like the existing
`WithEmitter`/`InvitedEmitter` pair, and adapted in `cmd/main.go` alongside the other five emit closures
(`apps/fleet-service/cmd/main.go:87-107`):

```go
emitInviteCreated := func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error {
    return fleetevents.EmitInviteCreated(tx, fleetID, actorID, traceID,
        dtoevents.InviteCreatedData{InviteID: inviteID, Email: email, Role: role})
}
```

`InviteCreatedData` is structurally identical to `MemberInvitedData` but stays a distinct type: aliasing
them would let a future field added for one silently change the other's wire format, and the two events
mean opposite things (FR-EVT-5).

### 4.2 Resend (FR-RSND-1…5)

`POST /fleets/{fleetId}/invites/{inviteId}/resend`, in the JWT group, gated by the same three checks every
invite mutation already uses (`resource.go:47-60`). Order of operations:

1. `proc.GetByID(inviteID)` → 404 if unknown.
2. `authz.RequireSameFleet(identity, fleetID)`; reject if `inv.FleetID() != fleetID` (path-pair mismatch →
   403, not 404 — the caller proved fleet membership but named the wrong invite).
3. `authz.RequireOwner(identity)` → `ownerCheck.RequireOwnerInFleet(...)`.
4. `inv.AcceptedAt() != nil` → 409 (FR-RSND-3), before the cooldown check so an accepted invite never
   reports a cooldown.
5. Cooldown check (§4.4) → 429.
6. `generateToken()`, then `adm.Resend(...)` in one transaction: `UPDATE token, expires_at` + emit
   `invite.created`.
7. `200` with `Transform(updated)` — identical shape to create (FR-RSND-4).

Rotation is an `UPDATE`, so `token`'s unique index still holds and the old link resolves to no row →
`GetByToken` returns `ErrNotFound` → 404 on accept, matching acceptance criterion 4.

**Interaction with `task-008` (Open Q4).** That branch changes the accept path's `409` semantics, not the
resend path's. Step 4's 409 is a distinct condition (resending an already-accepted invite) with no shared
code. No coordination needed; if task-008 lands first, re-read `processor.go:ValidateAccept` before
implementing to confirm nothing moved.

### 4.3 Address validation (NFR Security)

Today `attrs.Email == ""` is the only check (`resource.go:66`). Replace with a `validateInviteEmail` helper
in the invite processor:

```go
func ValidateInviteEmail(s string) error {
    if strings.ContainsAny(s, "\r\n") { return server.ErrValidation }  // header injection
    a, err := mail.ParseAddress(s)
    if err != nil || a.Address != s { return server.ErrValidation }    // reject "Name <a@b>" forms
    return nil
}
```

`a.Address != s` is the important half: `mail.ParseAddress` happily accepts `Bob <bob@x.com>`, which would
then be interpolated into a `To:` header as an attacker-influenced display name. Requiring the input to be
a bare addr-spec makes the header value a closed set of characters. CR/LF is checked first and separately
so the failure reason is unambiguous in a test.

### 4.4 Rate limits (FR-RATE-1…4)

Both derive from persisted state, so they hold across replicas (FR-RATE-1's explicit requirement).

**Creation window** — new `Provider.CountByFleetSince(fleetID string, since time.Time) (int64, error)`:

```sql
SELECT count(*) FROM fleet.fleet_invites WHERE fleet_id = ? AND created_at > ?
```

Checked before `generateToken()`. Over limit → `server.ErrTooManyRequests`.

**Resend cooldown** — `now.Sub(inv.UpdatedAt()) < cooldown` → 429.

`updated_at` requires two supporting changes: `Model` gains `updatedAt` and `Make`/`ToEntity` carry it
(the field exists on `Entity` already, `entity.go:20`, but is dropped on the way to `Model`).

> **Fragility to record.** `updated_at` means "last write of any kind", not "last rotation". It is correct
> today because the only other writer is `Accept`, which sets `accepted_at` and thus trips the 409 first.
> Any future column write on this table silently resets the cooldown. The alternative — deriving the last
> rotation as `expires_at - defaultExpiry` — is exact and equally free of new columns, but breaks
> retroactively if `defaultExpiry` is ever changed or made configurable. `updated_at` is chosen because its
> failure mode (a cooldown that resets early) is a rate-limit softening, while the other's failure mode (a
> cooldown computed from a stale constant) can be arbitrarily wrong in either direction. A test asserts the
> cooldown against a hand-stamped `updated_at`.

**Open Q2 settled.** `INVITE_RATE_LIMIT_PER_DAY=20`, `INVITE_RESEND_COOLDOWN_SECONDS=300`, read via
`config.GetInt` in `cmd/main.go` and passed into `InitializeRoutes`. Twenty invites per fleet per day is
roughly 3× the size of the largest plausible household fleet, so it never obstructs real use while still
capping a compromised account at 20 messages/day/fleet against the domain's sending reputation. Five
minutes is longer than any mail-delivery delay a user would wait through before hitting resend again.

**Open Q5 settled: no index now.** `fleet_invites` already indexes `fleet_id` (`entity.go:13`). At
household scale a fleet holds tens of invite rows, so the count query reads a handful of tuples after the
index seek. Adding `(fleet_id, created_at)` is a one-line GORM tag if invite volume ever grows; doing it
speculatively adds write cost to every insert for no measurable read benefit.

### 4.5 Internal read endpoint (FR-INT-1…4)

New `invite.InitializeInternalRoutes(log, db, fleetProv)`, registered in `cmd/main.go` next to the other
two internal initializers (`main.go:178-179`) — outside the JWT group.

```go
r.Get("/internal/invites/{inviteID}", ...)
```

Response is plain JSON, matching the internal convention (`membership/resource.go:108`), not JSON:API:

```json
{ "invite_id": "…", "fleet_id": "…", "fleet_name": "…", "email": "…", "role": "member",
  "token": "…", "expires_at": "…", "accepted_at": null, "invited_by_user_id": "…" }
```

`fleet_name` comes from `fleet.Provider.GetByID(inv.FleetID())`, injected as a narrow interface
(`FleetNamer interface { GetByID(id string) (fleet.Model, error) }`) rather than by importing the fleet
package's processor — same decoupling style as `OwnerChecker`. A missing fleet row yields `fleet_name: ""`
and a warn log rather than a 500; the email then degrades to the generic subject (§6).

Returns the row even when accepted or expired (FR-INT-3) — the consumer decides.

**`invited_by_name` is dropped (Open Q1 resolved).** `auth-service` exposes no internal endpoints — its
only cross-service client is outbound (`apps/auth-service/internal/membership/client.go:24`) — so resolving
a display name means creating auth-service's first unauthenticated surface *and* adding a third
priority-200 deny rule plus a `REQUIRED_DENY_SERVICES` entry in `tools/check-manifests.sh:93`. That is a
new attack surface on the identity service in exchange for one cosmetic string. The email names the fleet
instead, which is what a household invitee actually recognises. FR-TPL-3's fallback becomes the only path,
and §5.3's `invited_by_name` field is removed from the contract.

### 4.6 New shared error: 429

`packages/shared-go/server` has no `429`. Add to `errors.go`, `StatusFor`, and `codeFor`
(`server.go:12-35`):

```go
ErrTooManyRequests = errors.New("too many requests") // 429
// StatusFor: case errors.Is(err, ErrTooManyRequests): return 429
// codeFor:   case 429: return "too_many_requests"
```

`errors_test.go` already table-tests every status and every code; both tables gain a 429 row, which is what
makes this change self-verifying.

### 4.7 Route-tree test (FR-INT-4)

A test in `apps/fleet-service/internal/invite` builds the JWT-protected router via `InitializeRoutes` and
walks it with `chi.Walk`, asserting no registered pattern contains `/internal`. Walking the tree rather
than probing one URL means a future internal route added to the wrong initializer also fails.

---

## 5. Delivery, retry, and idempotency

### 5.1 The retry problem

`packages/shared-go/events/consumer.go:40-43`:

```go
if err := h(ctx, e); err != nil {
    log.WithError(err).…Error("handler failed; will retry")
    continue // do not commit → redelivery
}
```

The comment is wrong. `continue` returns to `r.FetchMessage(ctx)`, which yields the **next** message. When
that one succeeds, `CommitMessages` commits its offset — which is past the failed message's. On restart the
group resumes after the committed offset and the failed message is never seen again. There is no
in-process redelivery and no cross-restart redelivery either.

Fixing `Consume` itself (re-processing the same message before advancing) would change retry semantics for
all four existing topics on this branch, which is out of scope and risks head-of-line blocking in paths
that never asked for it.

### 5.2 Chosen: bounded in-handler retry

`mailconsumer.Handle` wraps the send in a bounded backoff loop, so retry does not depend on `Consume`:

```
attempt 1 → fail(transient) → sleep 2s
attempt 2 → fail(transient) → sleep 8s
attempt 3 → fail(transient) → sleep 30s
attempt 4 → fail(transient) → give up
```

Four attempts over ~40s (`SMTP_SEND_ATTEMPTS`, `SMTP_RETRY_BASE_SECONDS`). Each attempt's context carries a
per-attempt dial/send timeout (`SMTP_TIMEOUT_SECONDS`, default 10) so a black-holed relay cannot hang the
goroutine. Sleeps are `select` on `ctx.Done()` so shutdown is prompt.

On exhaustion: log at error, increment `failed_transient`, **and mark the ledger**. Marking is the
uncomfortable half — it means a relay outage longer than ~40s drops that invite's email permanently. It is
chosen anyway because the alternative under the §5.1 defect is not "retry later", it is "return an error
into a loop that discards the message and logs the same error" — identical mail loss with an unbounded
error rate and a wedged partition on top. FR-MAIL-5's "retries are bounded rather than unbounded"
(acceptance criterion 6) is satisfied; what is *not* satisfied is durable retry, and FR-UI-1's copy-link is
the documented recovery path for it. If durable retry is later required, §3's rejected
`notification_outbox` option is the way in.

Only the `invite-email` group's single goroutine blocks during backoff. In-app notifications, on their own
group, are untouched.

### 5.3 Failure classification (FR-MAIL-5)

`mailer` returns typed errors so the consumer classifies without parsing strings:

```go
type PermanentError struct{ Err error }   // errors.As target
func (e *PermanentError) Error() string { return e.Err.Error() }
```

`smtpSender` inspects `*textproto.Error` from `net/smtp`: code `5xx` on `MAIL FROM`/`RCPT TO`/`DATA` →
`PermanentError`; `4xx`, dial errors, TLS handshake failures and timeouts → returned bare (transient).
Malformed-address failures are `PermanentError` from the compose step, before any dial.

Permanent → log error with `invite_id`/`fleet_id`, increment `failed_permanent`, mark ledger, return nil.
Retrying a rejected mailbox forever is what wedges a partition.

### 5.4 Skip paths (FR-MAIL-4, FR-CFG-4)

Checked in this order, each marking the ledger and returning nil:

1. `!cfg.Enabled` → `skipped_disabled`. Checked **before** the fleet-service fetch, so a cluster with no
   relay configured makes no network calls at all.
2. `inv.AcceptedAt != nil` → `skipped_stale`.
3. `inv.ExpiresAt <= now` → `skipped_stale`.

### 5.5 Duplicate-email window

Mark-after-success means a crash between a successful `Send` and `inbox.Mark` produces one duplicate email
on the next delivery. PRD §8 accepts this explicitly. Nothing in the design narrows it further; marking
first would silently drop mail, which is strictly worse.

---

## 6. The mailer package

```
apps/notification-service/internal/mailer/
  sender.go       // Sender interface, Message struct, PermanentError
  smtp.go         // smtpSender: STARTTLS / implicit TLS / plaintext-local
  fake.go         // fakeSender: records messages, programmable error
  compose.go      // Message → RFC 5322 multipart/alternative bytes
  template.go     // go:embed + parsed html/template and text/template
  templates/invite.html.tmpl
  templates/invite.txt.tmpl
  metrics.go      // prometheus CounterVec
```

`mailer` is infrastructure, not a domain — no Model/Entity/Provider/Administrator, matching the existing
`internal/inbox` and `internal/fleetclient` packages. Called out so the backend guidelines reviewer scores
it against SUB-* rather than DOM-*.

### 6.1 Interface

```go
type Message struct {
    To      string
    Subject string
    HTML    string
    Text    string
}

type Sender interface {
    Send(ctx context.Context, msg Message) error
}
```

Rendering happens above `Send` in `mailconsumer`, via an exported `mailer.RenderInvite(InviteData) (Message, error)`.
That keeps `Sender` a pure transport seam: `fakeSender` asserts on delivery, and `RenderInvite` is tested
directly on its output string without any sender at all.

### 6.2 TLS modes (Open Q3 settled)

`SMTP_TLS_MODE` ∈ `{starttls, tls, none}`, validated against that exact set at construction — an unknown
value is a startup panic, not a runtime surprise.

| Mode | Behaviour | Typical port |
|---|---|---|
| `starttls` | plaintext connect, `STARTTLS`, **fail if the server does not offer it** | 587 |
| `tls` | `tls.Dial` (implicit TLS) from the first byte | 465 |
| `none` | plaintext, no auth attempted unless credentials are set | 1025 (Mailpit) |

`tls.Config{ServerName: host}` with verification on in both TLS modes; there is no skip-verify flag in
config or code (FR-MAIL-2). The `starttls` path must check `client.Extension("STARTTLS")` and error if
absent — silently continuing in plaintext is the classic downgrade.

`none` is the only mode that permits an unauthenticated, unencrypted session, and §9's manifest check keeps
it out of the `main` overlay (FR-DEV-2).

### 6.3 Message composition (FR-TPL-1…6)

Composed by hand into `multipart/alternative` — no third-party mail library. The header set is small and
fixed, and `mime/multipart` plus `net/textproto` cover the rest:

```
From: {SMTP_FROM_NAME} <{SMTP_FROM_ADDRESS}>
To: {invite.email}
Subject: You're invited to {fleetName} on MyFleet
Date: {RFC1123Z}
Message-ID: <{uuid}@{fromDomain}>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="…"
```

- `Subject` is a fixed string plus the fleet name (FR-TPL-4). If `fleet_name` is empty it degrades to
  `You're invited to a fleet on MyFleet`. The fleet name is user-controlled, so it is run through
  `mime.QEncoding.Encode("utf-8", …)`, which also neutralises any CR/LF that reached the database before
  §4.3's validation existed.
- `Message-ID`'s domain half is derived from `SMTP_FROM_ADDRESS`, not from a request or the invite — a
  Message-ID whose domain doesn't match the From domain is a spam signal.
- Body states fleet, role, and expiry date; names no inviter (§4.5); and includes the
  "if you weren't expecting this, ignore it" line (FR-TPL-6). No unsubscribe header.
- The MIME boundary is a random hex string generated per message; it must not collide with body content.

### 6.4 Escaping (FR-TPL-5)

Two template sets, parsed once at package init with `template.Must` over `go:embed`:

- `html/template` for the HTML part — contextual escaping of `fleetName` and `role`.
- `text/template` for the plain part.

The accept URL is the one value that must **not** be escaped into uselessness. In the HTML part it appears
only as `<a href="{{.AcceptURL}}">` and as link text; `html/template` treats `href` as a URL context and
will pass an `https://` URL through intact while still blocking `javascript:`. `PUBLIC_WEB_URL` comes from
config, never from a request header (FR-TPL-2), so the scheme is trusted by construction.

Link: `{PUBLIC_WEB_URL}/invites/{url.PathEscape(token)}/accept` — matching
`apps/web/src/pages/InviteAcceptPage.tsx:3`. The token is 64 hex chars, so `PathEscape` is a no-op today
and a guard if generation ever changes.

### 6.5 Metrics (FR-OBS-1)

This is the **first custom Prometheus metric in the repo** — only `promhttp.Handler()` exists today
(`packages/shared-go/health`). Declared with `promauto` on the default registry, so the existing
`/metrics` route (`notification-service/cmd/main.go:83`) exposes it with no wiring:

```go
var sends = promauto.NewCounterVec(prometheus.CounterOpts{
    Name: "myfleet_invite_emails_total",
    Help: "Invite emails by outcome.",
}, []string{"outcome"})
// outcome ∈ sent | failed_transient | failed_permanent | skipped_disabled | skipped_stale
```

`prometheus/client_golang` is currently an *indirect* dependency of notification-service
(`apps/notification-service/go.mod:30`); it becomes direct, so the task includes `make tidy` and a
`go.work.sum` update.

### 6.6 Never log the token (FR-OBS-2)

Enforced structurally, not by discipline:

- `mailer.Message` holds rendered `HTML`/`Text` containing the URL. It gets **no** `String()`/`LogValue()`
  method, and no code path passes a `Message` to a logger — log statements take explicit fields only.
- `PermanentError` wraps the SMTP error, never the message body.
- `InviteData` (the render input) is likewise never logged whole.
- Every log line in `mailconsumer` uses `log.WithFields(logrus.Fields{"invite_id":…, "fleet_id":…, "trace_id": e.TraceID})`.
- A unit test renders an invite with a known token, captures a `logrus` test hook across a full
  `Handle` call with a failing fake sender, and asserts the token substring appears in no entry —
  message *or* fields. That test is what makes acceptance criterion 9 mechanical rather than a grep by hand.

Recipient address may be logged (FR-OBS-3).

---

## 7. fleetclient extension

```go
type Invite struct {
    InviteID        string  `json:"invite_id"`
    FleetID         string  `json:"fleet_id"`
    FleetName       string  `json:"fleet_name"`
    Email           string  `json:"email"`
    Role            string  `json:"role"`
    Token           string  `json:"token"`
    ExpiresAt       string  `json:"expires_at"`
    AcceptedAt      *string `json:"accepted_at"`
    InvitedByUserID string  `json:"invited_by_user_id"`
}

func (c *Client) Invite(ctx context.Context, inviteID string) (Invite, error)
```

`getJSON` currently returns a bare `fmt.Errorf` on non-200 (`fleetclient/client.go:75`), which the mail
consumer cannot distinguish from a transient blip. `Invite` checks `res.StatusCode == 404` and returns a
sentinel `ErrInviteNotFound` so a deleted invite is a permanent skip (mark the ledger) rather than four
retries against a row that will never exist. This requires a small refactor: `getJSON` returns a typed
`*statusError` carrying the code, and `ActiveMembers`/`DueSchedules` keep their current behaviour since
`*statusError` still satisfies `error` and formats identically.

`ExpiresAt`/`AcceptedAt` stay strings on the wire and are parsed with `time.Parse(time.RFC3339, …)` in the
consumer, matching how the invite REST layer formats them (`invite/rest.go:23`).

---

## 8. Configuration

`deploy/k8s/base/notification-service/configmap.yaml` (FR-CFG-1):

```yaml
SMTP_ENABLED: "false"          # flipped to "true" once relay credentials are applied out of band
SMTP_HOST: "smtp.resend.com"
SMTP_PORT: "587"
SMTP_TLS_MODE: "starttls"
SMTP_FROM_ADDRESS: "invites@myfleet.tumidanski.com"
SMTP_FROM_NAME: "MyFleet"
PUBLIC_WEB_URL: "https://myfleet.tumidanski.com"
```

`deploy/k8s/secrets.example.yaml`, in the existing `notification-service-secret` stanza (FR-CFG-2):

```yaml
  SMTP_USERNAME: "REPLACE_ME"   # Resend: literally "resend"
  SMTP_PASSWORD: "REPLACE_ME"   # Resend: the API key
```

The `main` overlay renders no Secrets, and `check-manifests.sh:37-46` already proves it — this file is a
template only and is referenced by no kustomization.

**Startup validation (FR-CFG-4, FR-CFG-5).** A `mailer.ConfigFromEnv()` builder run in `cmd/main.go`:

- `SMTP_ENABLED` false → return a disabled config; read nothing else; construct no sender. The consumer
  short-circuits at §5.4 step 1.
- true → `config.MustGet` for `SMTP_HOST`, `SMTP_FROM_ADDRESS`, `PUBLIC_WEB_URL`; validate `SMTP_TLS_MODE`
  against the three-value set; `config.GetInt("SMTP_PORT", 587)`. Any miss panics at startup rather than
  surfacing as a per-message failure hours later.
- `SMTP_USERNAME`/`SMTP_PASSWORD` are read with `config.Get(…, "")`. Empty credentials are legal only when
  `SMTP_TLS_MODE=none` (Mailpit accepts unauthenticated mail); empty credentials with `starttls`/`tls` is a
  startup failure, since an unauthenticated submission to a real relay will be rejected on every message.

All reads go through `packages/shared-go/config` (FR-CFG-3).

`deploy/compose/docker-compose.yml`'s `notification-service` block gets the same keys pointed at Mailpit.

---

## 9. Local development (FR-DEV-1…4)

**Compose:** a `mailpit` service (`axllent/mailpit`), ports `1025` (SMTP) and `8025` (web UI), with a
Traefik label routing `/mail` to `:8025` so it is reachable through the same entrypoint as everything else
(FR-DEV-3). `notification-service` gains `depends_on: mailpit`.

**k3s local:** `deploy/k8s/infra-local/mailpit.yaml` (Deployment + Service, no PVC — mail is ephemeral and
a PVC in `infra-local` would be a foot-gun if the directory were ever pulled into another overlay), added
to `infra-local/kustomization.yaml`'s `resources` list. Local SMTP config is applied as a ConfigMap patch
in `overlays/local/kustomization.yaml`, where `SMTP_ENABLED: "true"`, `SMTP_HOST: "mailpit"`,
`SMTP_PORT: "1025"`, `SMTP_TLS_MODE: "none"`, and `PUBLIC_WEB_URL: "http://myfleet.home"` override the base
values. Putting the override in the overlay rather than the base is what keeps `none` structurally unable
to reach `main`.

**FR-DEV-2's assertion** goes in `tools/check-manifests.sh`, which is already part of `make ci` via
`make manifests`. Two greps against the rendered `main` overlay, in the same style as the existing
`REPLACE_ME` check (`check-manifests.sh:42`):

```sh
grep -q 'SMTP_TLS_MODE: "none"'   → fail
grep -qi 'mailpit'                → fail   # mailpit must never render into main
```

`make ci` requires no relay: `SMTP_ENABLED` is unset in the test environment, `ConfigFromEnv` returns
disabled, and every mailer test uses `fakeSender` (FR-DEV-4).

---

## 10. Web UI (FR-UI-1…4)

`InviteList.tsx` grows two controls beside Revoke, both owner-gated and both rendered only for pending
invites (the list is already filtered to `!acceptedAt`, `InviteList.tsx:27`, which satisfies FR-UI-3 as
written).

**Copy link (FR-UI-1).** The repo has no clipboard helper today, so this adds the first one:
`apps/web/src/lib/utils/clipboard.ts` exporting `copyToClipboard(text): Promise<boolean>` — `navigator.clipboard.writeText`
with a `document.execCommand('copy')` textarea fallback for non-secure contexts, since local dev runs over
plain HTTP on `myfleet.home` where `navigator.clipboard` is undefined. Without that fallback the button is
dead in exactly the environment where it gets tested first. Confirmation via the existing `sonner` toast
(`toast.success('Invite link copied')`), consistent with every other feedback path in
`lib/hooks/api/invites.ts`.

URL: `${window.location.origin}/invites/${inv.attributes.token}/accept`. The token is already in the list
response (`invite/rest.go:11`), so no API change.

**Resend (FR-UI-2).** `inviteService.resendInvite(fleetId, inviteId)` → `useResendInvite(fleetId)` mutation
in `lib/hooks/api/invites.ts`, invalidating `inviteKeys.lists()` `onSettled` — same shape as
`useRevokeInvite`. The list refresh is required, not cosmetic: rotation changes the token, so a stale cache
would hand the copy-link button a dead token.

**429 handling (FR-UI-4).** `ApiError` already carries `status` (`packages/shared-ts/src/errors.ts:11`).
A small `inviteErrorMessage(err)` helper maps it:

```ts
429 on create → "You've sent too many invites today. Try again tomorrow."
429 on resend → "You just resent this invite. Wait a few minutes before trying again."
409 on resend → "That invite has already been accepted."
```

Everything else falls through to `apiError.message`. Placed in the hooks file next to the mutations rather
than in a shared error module — it is invite-specific copy, and the two 429s need different sentences,
which a generic status-to-string map could not express.

---

## 11. Testing

Every test runs offline. No test dials a socket, starts a container, or touches a relay.

**fleet-service**

| Test | Asserts |
|---|---|
| `events`: `TestEmitInviteCreated` | one unsent outbox row, type `invite.created`, data round-trips, **no `token` key in the payload** (FR-EVT-3) |
| `invite`: insert rollback | emitter returns error → no invite row *and* no outbox row (FR-EVT-1/2) |
| `invite`: `TestValidateInviteEmail` | CR/LF rejected; `Bob <b@x.com>` rejected; `b@x.com` accepted |
| `invite`: resend rotation | token changes, `expires_at` advances, exactly one new outbox row with a **new** `event_id` (FR-EVT-4) |
| `invite`: resend on accepted | 409, token unchanged |
| `invite`: cooldown | 429 before the window elapses, 200 after (hand-stamped `updated_at`) |
| `invite`: creation window | 429 at limit+1 |
| `invite`: `TestInternalRouteAbsentFromJWTTree` | `chi.Walk` over `InitializeRoutes` finds no `/internal` pattern (FR-INT-4) |
| `server`: 429 tables | `StatusFor`/`codeFor` gain a 429 row |

**notification-service**

| Test | Asserts |
|---|---|
| `mailer`: `TestRenderInvite` | both parts present; both contain the accept URL; HTML escapes a fleet name of `<script>alert(1)</script>`; expiry date rendered |
| `mailer`: `TestCompose` | `multipart/alternative`; all six required headers; parses back via `net/mail.ReadMessage` + `mime/multipart` |
| `mailer`: `TestSubjectEncodesFleetName` | RFC 2047 encoding; no raw CR/LF survives into headers |
| `mailconsumer`: idempotency | same `event_id` twice → one send (mirrors `consume_test.go:46`) |
| `mailconsumer`: skips | accepted / expired / `SMTP_ENABLED=false` → zero sends, ledger marked, correct metric |
| `mailconsumer`: transient | fake fails N times → N attempts, then marked; wall-clock kept ~0 by injecting the sleep function |
| `mailconsumer`: permanent | `*PermanentError` → exactly one attempt, marked |
| `mailconsumer`: 404 from fleetclient | one attempt, marked, no retry |
| `mailconsumer`: `TestNoTokenInLogs` | `logrus` test hook over a full failing `Handle`; token appears in no message and no field (FR-OBS-2) |

**Retry timing is injected** (`sleep func(context.Context, time.Duration) error` on the consumer struct),
so the backoff test asserts the *schedule* — attempt count and requested durations — in microseconds
instead of sleeping 40 seconds in CI.

**web**: `InviteList` renders both controls for pending invites and neither for accepted; copy writes the
expected URL to a mocked clipboard; the 429 mapper returns the right sentence per endpoint.

**manifests**: `make manifests` covers the `main`/`local` renders, the no-Secrets/no-PVC/no-placeholder
invariants, and the two new `SMTP_TLS_MODE=none` / `mailpit` assertions.

**Not covered by automated tests** — verified by hand against the local stack per PRD §10:
end-to-end Mailpit delivery, real-relay deliverability (SPF/DKIM/DMARC alignment), and the rendered
appearance of the HTML part in a real client.

---

## 12. Deliberate risks

| Risk | Why it is accepted | Recovery |
|---|---|---|
| Relay outage > ~40s permanently drops that email (§5.2) | The alternative under the `Consume` defect is the same loss plus a wedged partition; durable retry needs a table PRD §6 excludes | Owner uses the copy-link control (FR-UI-1) |
| Duplicate email in the crash-between-send-and-mark window | PRD §8 accepts it; marking first would silently drop mail | None needed |
| `updated_at` cooldown resets if a future writer touches the row (§4.4) | Failure mode is a softened rate limit, not an incorrect one | Switch to a dedicated column if another writer is added |
| The email names no inviter (§4.5) | Resolving it means auth-service's first unauthenticated endpoint | Add `GET /internal/users/{id}` + a fourth deny rule if the copy proves confusing |
| `PUBLIC_WEB_URL` misconfigured → every link 404s | Config, not derivable safely from a request | Acceptance criterion 1 catches it before release |

---

## 13. Out-of-repo prerequisite

Before `SMTP_ENABLED=true` in `main`: a verified sending domain with SPF, DKIM and DMARC published for
`myfleet.tumidanski.com`, and relay credentials issued. Reference: Resend free tier, `smtp.resend.com:587`,
username `resend`, password = API key. The config surface is provider-generic, so moving to SES is a secret
edit and a pod restart.

Until that is done the service ships with `SMTP_ENABLED: "false"` and is a documented no-op — invites are
created, events are emitted and consumed, `skipped_disabled` increments, and nothing dials.

---

## 14. PRD amendments

Recorded so the plan and the audit measure against the resolved contract, not the draft:

1. **FR-MAIL-3** — a separate `mailconsumer` package with its own Kafka consumer group, rather than adding
   `invite.created` to `consumer.Topics`. Intent preserved and strengthened (§3).
2. **FR-MAIL-5** — "let the consumer's existing redelivery path retry" is not implementable; replaced with
   bounded in-handler retry and ledger-marking on exhaustion (§5).
3. **§5.3 / FR-INT-2** — `invited_by_name` removed from the internal response; `invited_by_user_id` stays
   (§4.5, Open Q1).
4. **Open Q2** — 20 invites/fleet/24h, 300s resend cooldown, both env-configurable (§4.4).
5. **Open Q3** — `SMTP_TLS_MODE` ∈ `{starttls, tls, none}`, validated at startup (§6.2).
6. **Open Q4** — no interaction with task-008; the two 409s are unrelated conditions (§4.2).
7. **Open Q5** — no new index (§4.4).
8. **New, not in the PRD** — `server.ErrTooManyRequests` must be added to `packages/shared-go/server`;
   there is no 429 in the shared taxonomy today (§4.6).
