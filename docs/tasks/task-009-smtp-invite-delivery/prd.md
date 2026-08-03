# SMTP Invite Email Delivery — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
---

## 1. Overview

A fleet owner can create an invite today, but the invited person is never told. `POST /fleets/{id}/invites`
mints a 32-byte hex token and persists it (`apps/fleet-service/internal/invite/resource.go:71`), the web UI
lists outstanding invites, and `/invites/:token/accept` works end to end — but nothing ever transmits the
token to the invitee, and `InviteList.tsx` never renders it either. The invite feature is therefore
undeliverable by any in-product path; the only way to use it is to read the token out of the `POST`
response body in devtools or out of the database.

This task closes that gap: when an invite is created (or resent), the invitee receives an email containing
a link to the accept page. Delivery goes through an authenticated SMTP relay, driven from
`notification-service`, which already owns the Kafka consumers, the `(event_id, consumer)` idempotency
ledger (`apps/notification-service/internal/inbox/processed.go:20`), and the internal-HTTP client pattern
for pulling cross-service data (`apps/notification-service/internal/fleetclient/client.go`).

Two structural facts shape the design and are easy to get wrong:

- **There is no invite-creation event.** `member.invited` is emitted from `Administrator.Accept`
  (`apps/fleet-service/internal/invite/administrator.go:108`) — it fires when an invite is *accepted*, not
  when it is created. The existing notification "A new member was invited to your fleet"
  (`apps/notification-service/internal/consumer/consume.go:181`) is an accept-time notice to existing
  members. This task adds a **new** `invite.created` event and must not overload `member.invited`.
- **The token is a bearer credential.** Anyone holding it can join the fleet as the invited role, subject
  only to the email match in `ValidateAccept`. It therefore stays out of Kafka payloads, out of logs, and
  out of email subject lines.

## 2. Goals

Primary goals:

- An invitee receives an email with a working accept link within seconds of the invite being created.
- The email is deliverable in practice — authenticated relay, aligned From domain, correct MIME structure —
  not merely "sent without error".
- Sending is idempotent under at-least-once event redelivery: one invite creation produces exactly one email.
- An SMTP outage or a rejected recipient never fails the invite-creation request and never rolls back the
  invite row.
- The owner can resend an invite, and can copy the accept link manually when email fails.
- The invite endpoint cannot be used to relay attacker-chosen mail at volume from the MyFleet domain.
- Local development and CI never touch a real SMTP relay or a real mailbox.

Non-goals:

- DNS setup itself (SPF/DKIM/DMARC record creation) — a documented prerequisite, not code.
- Email for any notification type other than invites. `schedule.overdue`, `maintenance.completed`,
  `fuel.logged`, `vehicle.created` and accept-time `member.invited` stay in-app only.
- An email channel in the `preferences` model. Invite mail is transactional; a user who suppressed it could
  not be invited at all. No `email_enabled` column, no preference UI.
- Per-invite delivery status surfaced in the UI (bounces, "delivery failed" badges). Failures are logged and
  metered; the copy-link fallback (FR-UI-1) is the manual recovery path.
- Bounce/complaint webhook ingestion, suppression lists, unsubscribe management.
- Rich HTML email design beyond a clean, accessible, single-column template.
- Changing invite acceptance semantics. The empty-`email`-claim defect in that path is
  `task-008-refresh-token-email-claim` and is out of scope here.

## 3. User Stories

- As someone invited to a household fleet, I want an email with a link I can click, so that I can join
  without the owner having to hand me a token out of band.
- As a fleet owner, I want to see that the invite was sent and to copy the link myself, so that a spam-filtered
  email doesn't leave me with no way to onboard someone.
- As a fleet owner, I want to resend an invite that expired or never arrived, so that I don't have to delete
  and recreate it.
- As an operator, I want SMTP credentials to live in the out-of-band secret like every other credential, so
  that the `main` overlay keeps rendering with no Secrets and Argo CD's `prune` can't remove them.
- As an operator, I want a failed send to be visible in logs and metrics with the invite id but not the
  token, so that I can diagnose delivery without leaking a credential into Loki.
- As a developer, I want mail to land in a local inbox I can open in a browser, so that I can iterate on the
  template without sending real email.

## 4. Functional Requirements

### 4.1 Invite-created event (fleet-service)

- **FR-EVT-1** — A new event type `invite.created` is emitted whenever an invite row is created, enqueued in
  the transactional outbox on the same `tx` as the insert, following the existing pattern in
  `apps/fleet-service/internal/events/emit.go`. A failed enqueue rolls back the invite creation.
- **FR-EVT-2** — `Administrator.Insert` currently writes outside a transaction
  (`apps/fleet-service/internal/invite/administrator.go:50`). It must be wrapped in a transaction so the
  invite row and the outbox row commit atomically, mirroring `Accept`.
- **FR-EVT-3** — Payload `InviteCreatedData` in `packages/dto-go/events/payloads.go` carries
  `{invite_id, email, role}` and **must not** carry `token`. The envelope already carries `fleet_id`,
  `actor_user_id`, `event_id` and `trace_id`.
- **FR-EVT-4** — A resend (FR-RSND-1) emits a fresh `invite.created` with a new `event_id`, which is what
  allows it past the inbox ledger. The ledger key is `(event_id, consumer)`, so a new event id is a new unit
  of work; no ledger change is required.
- **FR-EVT-5** — `member.invited` semantics are unchanged. Do not rename it, do not emit it at creation, and
  do not attach email sending to it.

### 4.2 Internal invite lookup (fleet-service)

- **FR-INT-1** — New network-restricted endpoint `GET /internal/invites/{inviteID}`, registered via an
  `invite.InitializeInternalRoutes` following `membership.InitializeInternalRoutes`
  (`apps/fleet-service/internal/membership/resource.go:89`) and wired in `apps/fleet-service/cmd/main.go`
  alongside the existing internal initializers. No JWT; reachability is governed by the ingress internal-deny
  rule already present in `deploy/k8s/overlays/main/ingressroute.yaml`.
- **FR-INT-2** — Response carries exactly what composing the email needs: `invite_id`, `email`, `role`,
  `token`, `expires_at`, `accepted_at`, `fleet_id`, `fleet_name`, and the inviter's display identity
  (`invited_by_user_id` plus whatever name/email fleet-service can resolve without calling auth-service; if
  it cannot resolve a name, the template falls back to the fleet name alone — see FR-TPL-3).
- **FR-INT-3** — Returns 404 for an unknown id. Returns the row even when `accepted_at` is set; the caller
  decides (FR-MAIL-4).
- **FR-INT-4** — This endpoint returns a bearer token. It must never be exposed through the public router,
  and a test must assert it is absent from the JWT-protected route tree.

### 4.3 SMTP sender (notification-service)

- **FR-MAIL-1** — A new `internal/mailer` package exposes a narrow interface, e.g.
  `Sender interface { Send(ctx context.Context, msg Message) error }`, with an SMTP implementation and an
  in-memory fake. All tests use the fake; no test dials a socket.
- **FR-MAIL-2** — The SMTP implementation authenticates over TLS: STARTTLS on 587 or implicit TLS on 465,
  selected by `SMTP_TLS_MODE`. Certificate verification is on; there is no "skip verify" escape hatch in
  committed config.
- **FR-MAIL-3** — `notification-service` subscribes to `invite.created` (added to `consumer.Topics`) and,
  on receipt, sends the invite email. The send is recorded in the inbox ledger under a **separate consumer
  name** (`invite-email`), distinct from the existing `notification` consumer, so that an SMTP failure
  cannot cause the in-app notification path to be re-run and vice versa.
- **FR-MAIL-4** — The handler skips sending (and marks the ledger) when the invite is already accepted or
  already expired at the time of processing — a delayed redelivery must not mail a dead link.
- **FR-MAIL-5** — Failure classification:
  - *Transient* (dial failure, timeout, 4xx greylisting, 5xx server error): do not mark the ledger; let the
    consumer's existing redelivery path retry. Retries are bounded by an attempt budget with backoff so a
    permanently unreachable relay does not spin.
  - *Permanent* (relay rejects the recipient address, malformed address): log at error with `invite_id` and
    `fleet_id`, increment the failure metric, **mark the ledger**, and stop. Retrying a rejected mailbox
    forever wedges the partition.
- **FR-MAIL-6** — Send failures are contained within the consumer. `POST /fleets/{id}/invites` returns 201
  as soon as the invite and outbox rows commit, and its latency is unaffected by relay latency.

### 4.4 Email content

- **FR-TPL-1** — The message is `multipart/alternative` with both a `text/plain` and a `text/html` part.
  Headers include `From`, `To`, `Subject`, `Date`, `Message-ID`, and `MIME-Version`. A single-part
  HTML-only body is not acceptable — it materially raises the spam score.
- **FR-TPL-2** — The accept link is `{PUBLIC_WEB_URL}/invites/{token}/accept`, matching the route in
  `apps/web/src/pages/InviteAcceptPage.tsx:3`. `PUBLIC_WEB_URL` is configuration, never derived from an
  inbound request header.
- **FR-TPL-3** — The body states who invited them, which fleet, what role, and when the link expires. If the
  inviter's display name is unresolvable, fall back to the fleet name; never render an empty "invited you"
  fragment or a raw UUID.
- **FR-TPL-4** — The subject line is a fixed, non-templated string plus the fleet name. The token never
  appears in the subject.
- **FR-TPL-5** — Every interpolated value is contextually escaped: `html/template` for the HTML part,
  `text/template` for the text part. The fleet name and inviter name are user-controlled input.
- **FR-TPL-6** — The email states that the recipient can ignore it if unexpected, and does not include a
  one-click unsubscribe (transactional mail; out of scope per §2).

### 4.5 Configuration and secrets

- **FR-CFG-1** — Non-secret keys go in `deploy/k8s/base/notification-service/configmap.yaml`:
  `SMTP_HOST`, `SMTP_PORT`, `SMTP_TLS_MODE`, `SMTP_FROM_ADDRESS`, `SMTP_FROM_NAME`, `PUBLIC_WEB_URL`.
- **FR-CFG-2** — Secret keys `SMTP_USERNAME` and `SMTP_PASSWORD` are added to the
  `notification-service-secret` stanza in `deploy/k8s/secrets.example.yaml` with `REPLACE_ME` placeholders.
  The `main` overlay must still render with **no** Secrets; credentials are applied out of band exactly as
  the file's header describes.
- **FR-CFG-3** — Config is read through `packages/shared-go/config` (`MustGet`/`Get`), never `os.Getenv` in
  handlers.
- **FR-CFG-4** — Email sending is feature-gated by `SMTP_ENABLED` (default `false`). When disabled, the
  consumer logs and marks the ledger without dialing, so a cluster without credentials configured is a
  no-op rather than a crash loop or a retry storm.
- **FR-CFG-5** — When `SMTP_ENABLED` is true, missing `SMTP_HOST`/`SMTP_FROM_ADDRESS`/`PUBLIC_WEB_URL` is a
  startup failure (`MustGet`), not a per-message failure discovered hours later.

### 4.6 Resend

- **FR-RSND-1** — New endpoint `POST /fleets/{fleetId}/invites/{inviteId}/resend`, owner-only, gated by the
  same three-step check every invite mutation uses: `authz.RequireSameFleet`, `authz.RequireOwner` (fast
  path), then `ownerCheck.RequireOwnerInFleet` (authoritative DB recheck, design §9).
- **FR-RSND-2** — Resend **rotates the token** and resets `expires_at` to `now + defaultExpiry`, in one
  transaction with the `invite.created` emit. Rationale: resend is used when the previous link failed to
  arrive or expired, so invalidating the old link costs nothing and bounds the lifetime of any token that
  leaked into a mailbox or a forwarded thread.
- **FR-RSND-3** — Resending an already-accepted invite returns `409 Conflict` and does not rotate anything.
- **FR-RSND-4** — Response is the updated invite resource, same shape as create.
- **FR-RSND-5** — Resend is subject to its own cooldown (FR-RATE-2).

### 4.7 Abuse control

- **FR-RATE-1** — Invite creation is limited per fleet over a rolling window (proposed: 20 invites per fleet
  per 24h). Exceeding it returns `429 Too Many Requests`. The count is derived from `fleet_invites.created_at`
  by query, not from an in-process counter — `fleet-service` runs multiple replicas and a per-pod limiter
  enforces nothing.
- **FR-RATE-2** — Resend has a per-invite cooldown (proposed: 1 per 5 minutes), also derived from persisted
  state (`updated_at`, which GORM stamps on the token rotation). Violation returns `429`.
- **FR-RATE-3** — Both limits are enforced server-side in the domain layer, not in the UI. The UI may disable
  the button, but that is a convenience, not the control.
- **FR-RATE-4** — Limits are configurable via env with the defaults above.

### 4.8 Web UI

- **FR-UI-1** — `InviteList.tsx` shows a copy-link control per pending invite that copies
  `{origin}/invites/{token}/accept`. This requires the token in the list response — it is already present in
  the invite resource (`apps/fleet-service/internal/invite/rest.go:10`) and the list endpoint is fleet-scoped,
  so no API change is needed. Copy uses the existing clipboard approach in the codebase, with a visible
  confirmation.
- **FR-UI-2** — A resend control per pending invite, calling FR-RSND-1 through a React Query mutation that
  invalidates the invite list on success, following the patterns in `apps/web/src/lib/hooks/api/invites.ts`.
- **FR-UI-3** — Accepted invites show neither control.
- **FR-UI-4** — `429` from create or resend surfaces as a human-readable message ("You've sent too many
  invites today"), not a raw error toast.

### 4.9 Local development

- **FR-DEV-1** — Mailpit is added to `deploy/compose/docker-compose.yml` and to `deploy/k8s/infra-local/`
  (manifest plus a `kustomization.yaml` entry), with `notification-service`'s local config pointing at it
  (`SMTP_HOST=mailpit`, `SMTP_PORT=1025`, `SMTP_TLS_MODE=none`, `SMTP_ENABLED=true`).
- **FR-DEV-2** — `SMTP_TLS_MODE=none` is permitted **only** for a plaintext local relay. The value must not
  appear in the `main` overlay, and a manifest check should assert that.
- **FR-DEV-3** — The Mailpit web UI is reachable locally so a developer can read rendered mail.
- **FR-DEV-4** — `make ci` must not require a running relay. The default for `SMTP_ENABLED` in test
  environments is `false`, and all mailer tests use the fake.

### 4.10 Observability

- **FR-OBS-1** — Counter metric for invite emails by outcome (`sent`, `failed_transient`, `failed_permanent`,
  `skipped_disabled`, `skipped_stale`).
- **FR-OBS-2** — Every log line about a send carries `invite_id`, `fleet_id`, and the correlation id from the
  event's `trace_id`. **No log line contains the token or the full accept URL**, at any level, including
  debug and including error paths that dump the message.
- **FR-OBS-3** — The recipient address may be logged, as it is already stored in `fleet_invites.email` and
  present in existing event payloads.

## 5. API Surface

### 5.1 `POST /api/fleet/fleets/{fleetId}/invites/{inviteId}/resend` (new, JWT, owner-only)

Request: no body.

Response `200 OK` — JSON:API document, same resource shape as invite create:

```json
{ "data": { "type": "invites", "id": "<uuid>",
  "attributes": { "email": "…", "role": "member", "token": "<rotated>",
                  "expires_at": "…", "accepted_at": null, "invited_by_user_id": "<uuid>" } } }
```

Errors: `401` unauthenticated · `403` not an owner of the fleet, or fleet mismatch · `404` unknown invite ·
`409` already accepted · `429` cooldown not elapsed.

### 5.2 `POST /api/fleet/fleets/{fleetId}/invites` (existing, modified)

Adds `429 Too Many Requests` when the per-fleet window limit is exceeded. All other behavior unchanged.

### 5.3 `GET /internal/invites/{inviteID}` (new, internal-only, no JWT)

Response `200 OK`:

```json
{ "invite_id": "<uuid>", "fleet_id": "<uuid>", "fleet_name": "…",
  "email": "…", "role": "member", "token": "…",
  "expires_at": "2026-08-09T12:00:00Z", "accepted_at": null,
  "invited_by_user_id": "<uuid>", "invited_by_name": "…" }
```

Errors: `404` unknown invite. Plain JSON (not JSON:API), matching the existing internal endpoints'
convention (`apps/fleet-service/internal/membership/resource.go:114`).

### 5.4 Event: `invite.created`

Standard envelope (`packages/shared-go/events/envelope.go:9`), `version: 1`, with
`data: {"invite_id": "<uuid>", "email": "…", "role": "member"}`. Published to topic `invite.created` by the
existing outbox relay; auto-created by Redpanda as with every other topic.

## 6. Data Model

No new tables and no new columns.

- `fleet.fleet_invites` is unchanged. Resend writes `token`, `expires_at`, and `updated_at` on an existing row.
- Rate limiting reads `created_at` (creation window) and `updated_at` (resend cooldown). An index on
  `(fleet_id, created_at)` should be considered if the invite table ever grows; at expected scale the
  existing `fleet_id` index suffices, and adding one is a judgment call for the design phase.
- `notification.processed_events` is unchanged. The email path uses the existing composite key with the new
  consumer name `invite-email`; no migration.

Migration notes: none. The only schema-adjacent risk is FR-EVT-2 wrapping `Insert` in a transaction, which
changes write semantics but not the schema.

## 7. Service Impact

**fleet-service** — `internal/invite`: transactional `Insert` + `invite.created` emit; resend endpoint with
token rotation; rate limit and cooldown checks; new internal routes file. `internal/events`:
`EmitInviteCreated`. `cmd/main.go`: wire `invite.InitializeInternalRoutes` and the new emitter.

**notification-service** — new `internal/mailer` (interface, SMTP impl, fake, templates); `internal/consumer`:
subscribe to `invite.created`, new handler with the `invite-email` ledger consumer name, stale/accepted skip,
failure classification; `internal/fleetclient`: `Invite(ctx, inviteID)` method; `cmd/main.go`: construct the
sender from config and inject it.

**packages/dto-go** — `InviteCreatedData` in `events/payloads.go`.

**deploy** — `base/notification-service/configmap.yaml` (SMTP + `PUBLIC_WEB_URL`); `secrets.example.yaml`
(`SMTP_USERNAME`/`SMTP_PASSWORD`); `infra-local` Mailpit manifest + kustomization entry; local overlay config
patch; `deploy/compose/docker-compose.yml` Mailpit service.

**apps/web** — `InviteList.tsx` copy-link and resend controls; `InviteService.ts` + `lib/hooks/api/invites.ts`
resend mutation; 429 message handling.

**Out-of-repo prerequisite** — a sending domain with SPF, DKIM and DMARC records published, and relay
credentials issued. Reference setup: Resend free tier (3,000/month, 100/day) sending from
`myfleet.tumidanski.com` via `smtp.resend.com:587`, username `resend`, password = API key. The config surface
is deliberately provider-generic, so moving to SES or another relay later is a secret edit and a restart, not
a code change.

## 8. Non-Functional Requirements

**Security**

- The token is a bearer credential: absent from Kafka payloads, absent from logs, absent from the subject
  line, and served only by the internal endpoint and the fleet-scoped list.
- TLS is mandatory for any non-loopback relay; certificate verification is never disabled in committed config.
- The invite endpoints let an authenticated user cause mail to be sent to an arbitrary address from the
  MyFleet domain. FR-RATE-1/2 bound that; without them a compromised account can burn the domain's sending
  reputation.
- Header injection: the recipient address and any interpolated header value must reject CR/LF. A newline in
  an email address must be a validation failure at invite creation, not something the SMTP layer discovers.
- Invite creation currently validates only that `email` is non-empty
  (`apps/fleet-service/internal/invite/resource.go:66`). It should validate the address shape, which is also
  what makes FR-MAIL-5's "permanent failure" class rare.

**Performance**

- `POST /fleets/{id}/invites` p95 must not regress; the send is asynchronous by construction.
- One email per invite creation, one internal lookup per email. No fan-out.

**Reliability**

- At-least-once event delivery plus the `invite-email` ledger yields exactly-one-email in the normal case and
  at-most-one-extra in a crash-between-send-and-mark window. That window is acknowledged and accepted: a
  duplicate invite email is a minor annoyance, whereas marking before sending would silently drop mail.

**Observability** — per §4.10.

## 9. Open Questions

1. **Inviter display name.** Can fleet-service resolve a human name for `invited_by_user_id` without calling
   auth-service? If not, the options are a cross-service call (new coupling), rendering the inviter's email,
   or falling back to fleet name only (current FR-TPL-3 default). Design phase should settle this.
2. **Rate-limit numbers.** 20/fleet/24h and 1 resend/5min are proposed, not measured. Reasonable for a
   household fleet; confirm before implementation.
3. **`SMTP_TLS_MODE` values.** Proposed `starttls` | `tls` | `none`. Confirm naming before it becomes a
   deployed config key.
4. **Interaction with `task-008`.** That branch changes the accept path's 409 semantics. If it lands first,
   FR-RSND-3's 409 should be consistent with whatever error taxonomy it establishes.
5. **Index on `(fleet_id, created_at)`** for the rate-limit query — needed now or deferred?

## 10. Acceptance Criteria

- [ ] Creating an invite in the local stack produces an email visible in Mailpit within seconds, with a
      working accept link, correct fleet name, correct role, and correct expiry.
- [ ] The email is `multipart/alternative`; both the text and HTML parts render the link and are legible.
- [ ] Replaying the same `invite.created` event (same `event_id`) produces exactly one email.
- [ ] A resend produces a second email with a **different** token; the previous link returns 404/409 on accept.
- [ ] Resending an accepted invite returns 409 and rotates nothing.
- [ ] With the relay unreachable, `POST /fleets/{id}/invites` still returns 201, the invite row exists, the
      failure is logged and metered, and retries are bounded rather than unbounded.
- [ ] A relay rejection of the recipient marks the ledger and does not retry forever.
- [ ] With `SMTP_ENABLED=false`, no dial is attempted and the consumer does not error.
- [ ] `grep` of service logs from a full invite-create-and-send cycle contains no token and no accept URL.
- [ ] Exceeding the invite window returns 429; the UI shows a human-readable message.
- [ ] The resend cooldown returns 429 before it elapses and succeeds after.
- [ ] `GET /internal/invites/{id}` is unreachable through the public router; a test asserts it.
- [ ] `InviteList.tsx` copy-link produces a URL that accepts successfully in a fresh session.
- [ ] `make ci` passes with no relay running and no network egress.
- [ ] `kustomize build deploy/k8s/overlays/main` renders with no Secrets, no PVCs, no ClusterRole, and no
      placeholder values; `SMTP_TLS_MODE=none` appears nowhere in it.
- [ ] `kustomize build deploy/k8s/overlays/local` renders and includes Mailpit.
- [ ] Both server dry-runs pass against the bee cluster:
      `kustomize build deploy/k8s/overlays/main | kubectl apply --dry-run=server -f -` and the same for
      `local`.
