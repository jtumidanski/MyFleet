# Refresh Token Email Claim — Product Requirements Document

Version: v1
Status: Draft
Created: 2026-08-02
Issue: [#6](https://github.com/jtumidanski/myfleet/issues/6)
---

## 1. Overview

`auth-service` mints access tokens on two paths: the Google OIDC callback and the refresh-token
rotation endpoint. The callback builds a complete `session.Principal` including `Email`
(`apps/auth-service/internal/oidc/resource.go:115`). The refresh path builds a partial one, silently
omitting `Email` (`apps/auth-service/internal/session/resource.go:62`). Every access token minted by
a refresh therefore carries `"email": ""`.

This is not an edge case. `packages/shared-ts/src/apiClient.ts` refreshes automatically on any 401
and access tokens expire after 15 minutes, so all but the first few minutes of every session run on a
refreshed — and therefore email-less — token. The `email` claim flows through
`auth.JWT` middleware into `auth.Identity.Email` (`packages/shared-go/auth/middleware.go:31`), which
is consumed across services.

The visible failure is invite acceptance. `fleet-service` gates `POST /invites/{token}/accept` on
`strings.EqualFold(inv.Email(), authedEmail)` (`apps/fleet-service/internal/invite/processor.go:55`).
Once a session has refreshed, `identity.Email` is `""`, the comparison fails, and acceptance returns a
bare `409 Conflict` with no indication of why — the same status the endpoint returns for an already-accepted
or expired invite. The user is told nothing, and the logs say nothing.

This is the second identity-propagation defect in this flow. The fix is therefore not only to populate
the missing field, but to remove the structural affordance that let one call site build a partial
`Principal` while another built a complete one, and to make a future silent recurrence loud.

## 2. Goals

Primary goals:

- Access tokens minted by `POST /auth/refresh` carry the same `email` claim as callback-minted tokens.
- Make claim divergence between the two minting paths structurally impossible rather than
  test-detectable-in-principle, by giving `Principal` a single construction site.
- Regression coverage that generalises past `email`: a refreshed token must carry every claim a
  callback-minted token does.
- A future empty-email token is diagnosable — the invite 409 distinguishes its cause, and the JWT
  middleware records the anomaly.

Non-goals:

- Changing refresh-token rotation, reuse detection, or family-revocation semantics.
- Changing the `/auth/me` flow or the user-provisioning path.
- Adding new claims to the access token, or changing token TTLs.
- Making the JWT middleware *reject* tokens with an empty `email` claim (log only — see §4.4).
- Any frontend change. `apiClient.ts` behaviour is correct and stays as-is.
- Migrating or invalidating existing sessions (see §6).

## 3. User Stories

- As a household member who was invited to a fleet, I want to accept my invite at any point in my
  session so that I can join without having to guess that logging out and back in might help.
- As an operator debugging a rejected invite, I want the 409 to tell me which precondition failed so
  that I can distinguish "already accepted" from "wrong account" without reading the database.
- As an engineer adding a claim to the access token, I want one place to construct a `Principal` so
  that I cannot populate it on one minting path and forget the other.
- As an operator, I want an access token with a missing identity claim to appear in the logs so that a
  regression of this class surfaces before a user reports it.

## 4. Functional Requirements

### 4.1 Single principal construction (`auth-service`)

**FR-1.** `session.MembershipResolver` is replaced by a `session.PrincipalResolver`:

```go
type PrincipalResolver func(ctx context.Context, userID string) (Principal, error)
```

It returns a fully-populated `Principal` — `UserID`, `Email`, `ActiveFleetID`, `Role`.

**FR-2.** The resolver is constructed once, in `apps/auth-service/cmd/main.go`, composing the existing
`membership.Client` (for fleet + role) with `user.Provider.GetByID` (for email). It continues to be
injected as a function value so `session` and `oidc` never import the concrete membership client — the
existing no-import-cycle constraint (Decision 1) is preserved.

**FR-3.** Both minting call sites obtain their `Principal` exclusively from the resolver and pass it to
`MintAccess` unmodified:

- `apps/auth-service/internal/session/resource.go` (refresh)
- `apps/auth-service/internal/oidc/resource.go` (callback)

The callback stops hand-assembling a `Principal` from `u.Email()`. This costs one extra user lookup per
login — a rare, already-DB-bound operation — and buys the guarantee that no call site can construct a
partial `Principal`.

**FR-4.** `oidc.Dependencies.Resolve` changes type to `session.PrincipalResolver` accordingly.

### 4.2 Failure handling

**FR-5.** Refresh fails closed. If the resolver returns an error — including a user lookup that finds no
record — `POST /auth/refresh` logs the error and returns `401 Unauthorized`, clearing the refresh cookie,
matching the existing behaviour when membership resolution fails. A token with incomplete identity is
never minted.

**FR-6.** The OIDC callback fails closed on resolver error, preserving its current behaviour: log and
redirect/respond `500` (`authentication failed`). Callback and refresh keep their respective existing
failure responses; only the resolver's return type changes.

### 4.3 Invite acceptance diagnostics (`fleet-service`)

**FR-7.** `invite.Processor.ValidateAccept` returns a distinct sentinel error per precondition instead of
a bare `server.ErrConflict`:

| Precondition | Sentinel |
|---|---|
| `inv.AcceptedAt() != nil` | `ErrAlreadyAccepted` |
| `!inv.ExpiresAt().After(now)` | `ErrInviteExpired` |
| `!strings.EqualFold(inv.Email(), authedEmail)` | `ErrEmailMismatch` |

**FR-8.** All three continue to map to HTTP `409 Conflict`. The status contract does not change; only the
error `detail` differentiates them.

**FR-9.** The 409 response body carries a distinguishable JSON:API error `detail` per case, e.g.
`"invite has already been accepted"`, `"invite has expired"`, `"invite was issued to a different account"`.

**FR-10.** The `detail` for `ErrEmailMismatch` MUST NOT echo the invite's email address or the
authenticated email. An invite token is bearer-ish: anyone holding it can reach this endpoint, and the
response must not disclose who it was addressed to.

**FR-11.** The accept handler logs `ErrEmailMismatch` at warn with the invite id and correlation id
(never the email addresses), so the failure is greppable.

### 4.4 Empty-claim observability (`shared-go`)

**FR-12.** `auth.JWT` accepts optional functional options: `auth.JWT(keyfn, opts ...Option)`, with
`auth.WithLogger(logrus.FieldLogger)`. The existing single-argument form keeps compiling — all four
current call sites are unaffected unless updated.

**FR-13.** When a token validates but its `email` claim is empty, the middleware logs at warn — including
the `sub` claim and correlation id, never the token itself. The request proceeds normally; this is
observability, not enforcement.

**FR-14.** With no logger supplied, the middleware uses a no-op and behaves exactly as it does today.

**FR-15.** `auth-service` and `fleet-service` wire a logger into `authmw.JWT`. `media-service` and
`notification-service` may be left on the single-argument form.

## 5. API Surface

No endpoint is added, removed, or changed in shape.

**`POST /auth/refresh`** (`auth-service`) — unchanged request and response shape. Behavioural change: the
returned access token's `email` claim is now populated. New failure mode: `401` when the user record
backing the refresh token cannot be resolved (previously would have minted a partial token).

**`POST /invites/{token}/accept`** (`fleet-service`) — unchanged request shape, unchanged `200` response,
unchanged `409` status. Changed: the `409` body's error `detail` now distinguishes three previously
indistinguishable causes.

```json
{
  "errors": [
    { "status": "409", "detail": "invite was issued to a different account" }
  ]
}
```

Access token claim set (unchanged in shape; now identical across both minting paths):

```json
{
  "sub": "...", "email": "...", "active_fleet_id": "...", "role": "...",
  "iss": "myfleet-auth", "aud": "myfleet", "iat": 0, "exp": 0
}
```

## 6. Data Model

No schema change. No migration.

Existing access tokens carrying `"email": ""` remain valid until they expire (15 minutes) and are replaced
by a correctly-populated token on the next refresh. Sessions self-heal within one token lifetime of
deploy; no forced re-authentication and no refresh-token invalidation is required.

## 7. Service Impact

**`apps/auth-service`** — primary.
- `internal/session/resource.go` — `PrincipalResolver` type; refresh handler passes the resolved
  `Principal` straight through; fail-closed on resolver error.
- `internal/oidc/resource.go` — `Dependencies.Resolve` retyped; callback stops assembling its own
  `Principal`.
- `cmd/main.go` — resolver now composes `membership.Client` with `user.Provider.GetByID`; wires a logger
  into `authmw.JWT`.
- `internal/user` — no change; `Provider.GetByID` already exists (`provider.go:34`) and is used as-is.

**`packages/shared-go/auth`** — `JWT` gains variadic options and `WithLogger`; empty-email warn log.
Backward compatible.

**`apps/fleet-service`** — `internal/invite/processor.go` sentinel errors; `internal/invite/resource.go`
error mapping and warn log; `cmd/main.go` wires a logger into `authmw.JWT`.

**`apps/media-service`, `apps/notification-service`** — no change required; they inherit the corrected
token and the compatible middleware signature.

**`apps/web`, `packages/shared-ts`** — no change.

## 8. Non-Functional Requirements

**Performance.** The OIDC callback gains one indexed primary-key lookup per login — negligible against
the OAuth round trip it already performs. `POST /auth/refresh` gains one indexed primary-key lookup per
refresh (roughly once per 15 minutes per active session), on a path that already performs a refresh-token
read, a write, and a cross-service membership call.

**Security.**
- Fail-closed on resolver error (FR-5) ensures no token is ever minted with unverified or absent identity.
- No log statement added by this work may contain an email address, a raw access token, or a raw refresh
  token. Log the `sub`/user id and correlation id only.
- FR-10's non-disclosure rule is a hard requirement, not a nicety: the invite token is the only credential
  needed to reach the accept endpoint.
- Populating `email` on refresh restores an authorization input that is currently empty. Any consumer that
  treated empty-email as a pass rather than a fail must be checked; `ValidateAccept` fails closed today
  (empty never matches a real invite email), so this fix strictly tightens behaviour.

**Observability.** FR-11 and FR-13 make both the symptom (invite mismatch) and the root cause class
(missing identity claim) greppable. Neither adds a metric; log-level detection is sufficient for a defect
of this frequency.

## 9. Open Questions

1. **Resolver error granularity.** FR-5 collapses "membership unavailable" (likely transient — fleet-service
   down) and "user not found" (permanent) into a single `401`. A transient fleet-service outage will
   therefore log users out rather than returning `503`. This preserves today's behaviour and is the safe
   default, but a follow-up may want to distinguish them. Out of scope here — flagged for design.
2. **`WithLogger` vs. correlation-scoped logging.** FR-13 wants a correlation id in the warn log; the
   middleware has access to the request context, so `telemetry.CorrelationIDFromContext` should work, but
   the ordering of `telemetry.CorrelationID` and `authmw.JWT` in each service's middleware chain needs
   confirming at design time. `auth-service` applies `telemetry.CorrelationID` first (`cmd/main.go:86`);
   the other services need checking.

## 10. Acceptance Criteria

- [ ] `session.PrincipalResolver` exists and returns a fully-populated `Principal`; `MembershipResolver`
      is gone from the codebase.
- [ ] `Principal` is constructed in exactly one place (`cmd/main.go`'s resolver); neither
      `session/resource.go` nor `oidc/resource.go` contains a `Principal{...}` literal.
- [ ] An access token minted by `POST /auth/refresh` carries a non-empty, correct `email` claim.
- [ ] **Claim-parity test:** a session-level test mints one token via the callback path and one via the
      refresh path for the same user and asserts the two claim *key sets* are identical and that every
      identity claim (`sub`, `email`, `active_fleet_id`, `role`) is equal. The test fails if a future claim
      is added to one path only.
- [ ] A test asserts `POST /auth/refresh` returns `401` and clears the refresh cookie when the resolver
      errors, and that no token is issued.
- [ ] A test asserts the existing refresh happy path, rotation, and reuse-detection behaviour is unchanged.
- [ ] `ValidateAccept` returns three distinct sentinel errors; a test covers each, plus the success case.
- [ ] All three sentinels map to HTTP `409` with distinct `detail` strings; a test asserts the mismatch
      `detail` contains neither the invite email nor the authenticated email.
- [ ] `auth.JWT(keyfn)` still compiles at every existing call site; `auth.JWT(keyfn, auth.WithLogger(log))`
      logs a warn when the `email` claim is empty and does not reject the request. Both covered by tests.
- [ ] An end-to-end check: authenticate, force a refresh, then accept a pending invite — the invite is
      accepted (`200`), where on `main` it returns `409`.
- [ ] `go build` and `go test` pass across `apps/auth-service`, `apps/fleet-service`, and
      `packages/shared-go`.
