# Task 009 — SMTP Invite Delivery — Implementation Context

Companion to [`plan.md`](./plan.md). Read this first if you are picking the task up cold.

---

## 1. What exists today

Invites already work end to end **except** that nothing ever tells the invitee.

| Concern | Where | State |
|---|---|---|
| Create invite | `apps/fleet-service/internal/invite/resource.go:39` | Works; mints 32-byte hex token; `Insert` is **not** transactional |
| List invites | `resource.go:95` | Works; fleet-scoped; response already carries `token` (`rest.go:10`) |
| Revoke | `resource.go:112` | Works |
| Accept | `resource.go:149` | Works; `Accept` **is** transactional and emits `member.invited` |
| Notify invitee | — | **Missing entirely.** This task. |

`member.invited` fires from `Administrator.Accept` (`administrator.go:111`) — it is an
*accept-time* notice to existing members, **not** an invite-creation event. Do not overload it.

---

## 2. Key files, by area

### fleet-service — `apps/fleet-service/internal/invite/`

| File | What you change |
|---|---|
| `model.go` | add `updatedAt` field + `UpdatedAt()` accessor |
| `entity.go` | `Make`/`ToEntity` carry `UpdatedAt` (column already exists, `entity.go:20`) |
| `builder.go` | unexported `setUpdatedAt` (mirrors existing `setAcceptedAt`, `builder.go:25`) |
| `provider.go` | `CountByFleetSince` |
| `processor.go` | `ValidateInviteEmail`, `CheckCreateLimit`, `CheckResendCooldown` |
| `administrator.go` | `Insert` → transactional + `traceID`; new `Resend`; `CreatedEmitter` + `WithCreatedEmitter` |
| `resource.go` | wire validation + limit into create; new resend route |
| `internal.go` *(new)* | `InitializeInternalRoutes`, `FleetNamer` |
| `rest.go` | `InternalResponse` + `TransformInternal` |

`processor_test.go:13` has a `stubProvider` — **every new `Provider` method must be added to it**
or the package stops compiling.

### fleet-service — elsewhere

- `internal/events/emit.go` — add `EmitInviteCreated` beside the five existing emitters.
- `cmd/main.go:95` — emit adapter closures live here; `main.go:178` — internal route initializers;
  `main.go:186` — the JWT-group `invite.InitializeRoutes` call.

### notification-service — `apps/notification-service/internal/`

| Package | State |
|---|---|
| `consumer/` | existing in-app notification consumer. **Do not touch** — the design deliberately puts email in its own package/group (design §3) |
| `inbox/` | `(event_id, consumer)` ledger; `Exists`/`Mark`. Reused as-is with consumer name `invite-email` |
| `fleetclient/` | internal HTTP client; gains `Invite()` + a typed `*statusError` from `getJSON` |
| `mailer/` *(new)* | transport + rendering. Infrastructure, **not** a DDD domain — no Model/Entity/Provider/Administrator |
| `mailconsumer/` *(new)* | one `invite.created` → one email, idempotently |

`cmd/main.go:56` constructs the existing consumer; the mail consumer goroutine goes beside it.

### shared

- `packages/shared-go/server/errors.go` + `server.go` — no 429 exists today; add it.
- `packages/dto-go/events/payloads.go` — `InviteCreatedData`.

### deploy

- `deploy/k8s/base/notification-service/configmap.yaml` — 5 lines today; gains the SMTP block.
- `deploy/k8s/secrets.example.yaml` — `notification-service-secret` stanza (template only,
  referenced by no kustomization, which is why `main` still renders zero Secrets).
- `deploy/k8s/infra-local/` — new `mailpit.yaml` + a `resources:` entry.
- `deploy/k8s/overlays/local/kustomization.yaml` — ConfigMap patch pointing at Mailpit.
- `deploy/compose/docker-compose.yml` — `notification-service` block at line 164.
- `tools/check-manifests.sh` — two new greps (`make ci` runs this via `make manifests`).

### web — `apps/web/src/`

- `components/features/settings/InviteList.tsx` — already filtered to pending (line 27), so FR-UI-3 is free.
- `lib/hooks/api/invites.ts` — `useResendInvite` + `inviteErrorMessage`.
- `services/api/InviteService.ts` — `resendInvite` (needs a raw `apiClient.request`; `BaseService`
  has no bodyless-POST helper — copy the `acceptInvite` shape at line 45).
- `lib/utils/clipboard.ts` *(new)* — sibling of the existing `lib/utils/download.ts`.

---

## 3. Decisions carried in from the design (do not re-litigate)

| # | Decision | Why |
|---|---|---|
| 1 | Email lives in its **own package + Kafka group** (`mailconsumer`, group `invite-email`), not in `consumer.Topics` | An SMTP stall must not hold back in-app notification offsets. Deviation from FR-MAIL-3's literal wording, satisfying its intent (design §3) |
| 2 | Token travels over **internal HTTP**, never over Kafka | Redpanda persists messages unencrypted; also means a delayed event mails the *current* token after a rotation |
| 3 | **Bounded in-handler retry**, then mark the ledger | `events.Consume`'s `continue`-without-commit does **not** redeliver — the next message's commit implicitly commits past the failed one (design §5.1). Durable retry would need a table; PRD §6 says no migrations |
| 4 | `invited_by_name` is **dropped** | auth-service has no internal surface; adding one means a new unauthenticated endpoint on the identity service for one cosmetic string (design §4.5) |
| 5 | Resend **rotates** the token and resets expiry | Bounds the lifetime of a token that leaked into a forwarded mailbox |
| 6 | Rate limits derive from **persisted state** (`created_at`, `updated_at`) | fleet-service runs multiple replicas; a per-pod counter enforces nothing |
| 7 | **No new index** on `(fleet_id, created_at)` | `fleet_id` is already indexed (`entity.go:13`); a household fleet holds tens of rows |
| 8 | `SMTP_TLS_MODE` ∈ `{starttls, tls, none}`, validated at startup | Open Q3 |
| 9 | `20` invites/fleet/24h, `300s` resend cooldown, both env-configurable | Open Q2 |

**Known fragility, recorded deliberately:** the resend cooldown reads `updated_at`, which means
"last write of any kind". It is correct today because the only other writer is `Accept`, which trips
the 409 first. Any future column write on `fleet_invites` silently resets the cooldown.

---

## 4. Deviations this plan introduces beyond the design

Both are flagged inline in `plan.md`; listed here so the audit does not read them as drift.

1. **Backoff schedule is `base × 4^(n-1)` → 2s / 8s / 32s**, not the design's literal "2s / 8s / 30s".
   A single formula is testable as a schedule; three hand-written constants are not. Total ≈42s,
   which is what design §5.2 actually calls for ("~40s").
2. **Mailpit in k3s-local is reached by `kubectl port-forward`**, not by a Traefik `/mail` route.
   Mailpit's UI is a SPA that breaks under a stripped prefix; docker-compose gets the design's
   `/mail` route via `MP_WEBROOT` (which needs no stripprefix) *and* publishes `8025`, so FR-DEV-3
   holds in the environment developers actually use.

---

## 5. Sequencing and dependencies

```
T1 (429 error)  ─┐
T2 (event+DTO)  ─┼─→ T5 (transactional Insert) ─→ T6 (resend + limits)
T3 (email valid)─┤                                      │
T4 (updatedAt)  ─┘                                      │
                                                        ↓
T7 (internal endpoint) ─→ T8 (fleetclient.Invite) ─→ T11 (mailconsumer) ─→ T12 (wiring)
                          T9 (render) ─→ T10 (smtp+config) ─┘                    │
                                                                                 ↓
                                                                     T13 (deploy) ─→ T14 (web)
```

T1–T4 are independent of one another and can be done in any order. T9/T10 need nothing from
fleet-service and can proceed in parallel with T5–T7.

---

## 6. Verification

Per-task commands are in `plan.md`. The branch gate is:

```sh
make ci        # lint-check, vet, test, build, fe-test, fe-build, manifests, carfax-template
```

`npm` may not be on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

Manifests must also pass **both** server dry-runs against the bee cluster — rendering alone does not
catch namespace or cross-resource errors, and the local overlay is not exempt:

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Go tests run from the repo root against full package paths (it is a `go.work` workspace):

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestName -v
```

**No test may dial a socket, start a container, or touch a relay.** `SMTP_ENABLED` is unset in CI,
`ConfigFromEnv` returns a disabled config, and every mailer/mailconsumer test uses `FakeSender`.

---

## 7. Manual verification not covered by tests

Per PRD §10 — do these by hand against the local stack before opening the PR:

- Create an invite locally → mail appears in Mailpit within seconds, link works, fleet name / role /
  expiry correct.
- Resend → second mail with a **different** token; the first link 404s on accept.
- HTML part renders legibly in a real client.
- Real-relay deliverability (SPF/DKIM/DMARC alignment) — blocked on the out-of-repo prerequisite
  below, so `SMTP_ENABLED` ships `"false"` in `main`.

---

## 8. Out-of-repo prerequisite

Before flipping `SMTP_ENABLED: "true"` in `main`: a verified sending domain with SPF, DKIM and DMARC
published for `myfleet.tumidanski.com`, plus relay credentials applied out of band.

Reference: Resend free tier (3,000/month, 100/day), `smtp.resend.com:587`, username literally
`resend`, password = API key. The config surface is provider-generic, so moving to SES is a secret
edit and a pod restart, not a code change.

Until then the service is a documented no-op: invites are created, events emitted and consumed,
`skipped_disabled` increments, and nothing dials.
