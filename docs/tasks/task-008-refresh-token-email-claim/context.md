# Refresh Token Email Claim — Implementation Context

Companion to [plan.md](./plan.md). Read this first if you are picking the task up cold: it is the map of the code the plan edits, the decisions already made and closed, and the traps that are easy to fall into.

PRD: [prd.md](./prd.md) · Design: [design.md](./design.md) · Issue: [#6](https://github.com/jtumidanski/myfleet/issues/6)

---

## 1. The defect in one screen

Two call sites mint access tokens. Both build a `session.Principal` and hand it to `session.Processor.MintAccess`, which mechanically maps each field to a JWT claim:

```
oidc/resource.go:115      session.Principal{UserID, Email, ActiveFleetID, Role}   ← complete
session/resource.go:62    session.Principal{UserID,        ActiveFleetID, Role}   ← Email absent
```

Both compile. Go zero-values the omitted field, `MintAccess` writes `"email": ""`, and `auth.JWT` faithfully propagates the empty string into `auth.Identity.Email`. Nothing fails loudly.

`packages/shared-ts/src/apiClient.ts` refreshes on any 401 and access tokens live 15 minutes, so **all but the first few minutes of every session run on a refreshed — and therefore email-less — token.** This is not an edge case.

`MintAccess` is not the defect; it is faithful. The defect is that `Principal` is an exported four-field struct either call site may fill in partially, and one does.

## 2. Blast radius — confirmed by grep, and narrower than it looks

`auth.Identity.Email` has exactly **one** consumer in the whole repository: `apps/fleet-service/internal/invite/resource.go:163`, which passes it to `ValidateAccept`. That consumer fails closed — an empty string never `EqualFold`s a real invite email — so the bug produces a **false rejection, never a false authorization**.

Two consequences worth holding onto:

- Populating the claim **strictly tightens** behaviour. There is no consumer to re-audit for a treated-empty-as-permissive assumption.
- The user-visible symptom is a bare `409 Conflict` on invite acceptance, identical to the 409 for an already-accepted or expired invite. The user is told nothing and the logs say nothing. That is why this task also buys diagnostics, not just the fix.

## 3. Key files

### `apps/auth-service` — the fix

| File | What matters |
|---|---|
| `internal/session/processor.go:20-25` | `Principal` — four string fields. `MintAccess:59-73` maps each to a claim plus `iss`/`aud`/`iat`/`exp`. |
| `internal/session/resource.go:15-18` | `MembershipResolver func(ctx, userID) (fleetID, role string, err error)` — the type being replaced. |
| `internal/session/resource.go:55-67` | The bug: `resolve` returns only fleet+role, so the handler hand-builds a partial `Principal`. |
| `internal/oidc/resource.go:33` | `Dependencies.Resolve session.MembershipResolver` — retyped in Task 4. |
| `internal/oidc/resource.go:108-131` | The complete-`Principal` call site. Also `:141` — the onboarding redirect keys off an empty `fleetID`. |
| `cmd/main.go:45-55` | Decision 1's inline closure. Becomes `newPrincipalResolver`. |
| `internal/user/provider.go:11-26` | Read the doc comment. It explains `GetByID` vs `GetBySub` at length because confusing them was the *first* identity-propagation defect here. |
| `internal/membership/client.go:33-35` | **404 → `(Membership{}, nil)`, not an error.** See §5. |
| `internal/session/processor_test.go:18-74` | `fakeStore` (Provider + Administrator over maps) and `newTestProcessor`. Task 4's new `resource_test.go` reuses both — same package, no new fake infrastructure. |

### `apps/fleet-service` — symptom diagnostics

| File | What matters |
|---|---|
| `internal/invite/processor.go:46-59` | `ValidateAccept` — three preconditions, one indistinguishable `server.ErrConflict`. |
| `internal/invite/resource.go:163-166` | The accept handler's call into it. `errors`, `logrus`, `telemetry`, `server` all already imported. |
| `internal/invite/processor_test.go:46-77` | Four existing tests asserting `errors.Is(err, server.ErrConflict)`. **Leave them alone** — they are the FR-8 regression guard. |
| `internal/invite/builder.go:26` | `setAcceptedAt` — unexported, exists for white-box tests in the package. Task 3's route test uses it. |
| `internal/maintenancecategory/entity_test.go:13-30` | The sqlite-with-`ATTACH DATABASE ':memory:' AS fleet` pattern Task 3's route test copies. Table names are schema-qualified for Postgres; sqlite has no schemas. |

### `packages/shared-go` — cause-class diagnostics and the detail carrier

| File | What matters |
|---|---|
| `server/errors.go:42-49` | `APIError.Detail` exists **and nothing in the repo ever sets it.** This is the gap the PRD's §7 Service Impact did not list. |
| `server/jsonapi.go:28-38` | `WriteError` populates `Status`, `Code`, `Title: err.Error()` only. |
| `auth/middleware.go:13-38` | `JWT` delegates to `jwtWithKeyfunc`, which adds no behaviour and exists only because the tests call it. Folded away in Task 2. |
| `auth/middleware_test.go:25,44` | The two `jwtWithKeyfunc` call sites to migrate onto `JWT`. |
| `telemetry/correlation.go:30` | `CorrelationIDFromContext(ctx) string`. |

## 4. Decisions already made — do not relitigate

From design §9, plus the one plan-time confirmation.

| # | Decision | Why the alternative was rejected |
|---|---|---|
| D1 | `PrincipalResolver` is a **function value** returning `session.Principal` | An interface would work and buy nothing, at the cost of a named type in one of the two packages that must not own it. |
| D2 | Composed in a named `newPrincipalResolver` in `cmd/main.go` | An inline `main()` closure cannot be unit-tested, and it is now the sole guarantor of claim completeness. A dedicated `internal/principal` package would exist to hold one closure. A method on `user.Processor` would make the user domain depend on the membership client — the exact coupling Decision 1 prevents. |
| D3 | Resolver takes the read-only `user.Provider`, reads the local DB **before** the network call | `*user.Processor` is a wider surface with no existing interface seam. Local-first short-circuits the permanent failure without an HTTP round trip. |
| D4 | Membership 404 → empty fleet, **not** an error | Treating it as an error breaks new-user signup on first login. |
| D5 | The resolver does **not** reject an empty `u.Email()` | Rejecting would hard-lock out any legacy row with a blank email — a cosmetic defect turned into total denial of service. PRD §2 makes enforcement a non-goal; the warn log is the chosen response. |
| D6 | Refresh **clears the cookie** on resolver error | Today's path returns 401 without clearing, unlike every other 401 in the handler. A session whose user row is gone should not keep a cookie that will 401 forever. |
| D7 | Parity guarded by a reflection test + an AST test, not a two-path e2e | The e2e needs a Google token-endpoint fake and a forged ID token passing `idtoken.Validate`, and localises failures worse. **Confirmed with the user at plan time** — see plan.md "Deviation". |
| D8 | `server.Detailed` carries the detail *with* the sentinel | `WriteErrorDetail(w, err, detail)` puts the detail at the transport call site and the sentinel at the domain — two places to sync, and the handler must re-switch on the sentinel it just received. A hand-built envelope duplicates `server`'s construction. |
| D9 | `detailedError.Error()` returns the **base** message | `fmt.Errorf("...: %w", ErrConflict)` would make every 409's `title` a sentence — a shape change to every existing 409 body, since `WriteError` sets `Title: err.Error()`. |
| D10 | `auth.JWT` variadic options, nil-logger no-op, `jwtWithKeyfunc` folded in | A separate `JWTWithLogger` constructor means two entry points to keep in step. A discard logger instead of a nil check allocates and reads less clearly than `cfg.log == nil`. |
| D11 | PRD Open Question 1 resolved as out of scope | See §7. |
| D12 | PRD Open Question 2 closed by verification | See §6. |

**One minor mechanical simplification of D3.** Design §2.3 has `main` construct a *second* `user.NewProvider(db)` alongside the one inside `users`. The plan instead hoists a single `userProv := user.NewProvider(db)` and passes it to both `newPrincipalResolver` and `user.NewProcessor`. Same seam, same behaviour, one fewer "why are there two of these?" question. `dbProvider` is a stateless struct over the shared `*gorm.DB`, so either form is free.

## 5. Traps

**The membership 404 is load-bearing.** `membership.Client.Active` maps HTTP 404 to `(Membership{}, nil)` (`client.go:33-35`), so a user with no fleet resolves to an empty `ActiveFleetID`, and the OIDC callback keys its onboarding redirect off exactly that (`oidc/resource.go:141`). Turning "no membership" into a resolver error breaks new-user signup on the first login. Task 4's `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError` exists for this.

**`GetByID`, not `GetBySub`.** The JWT `sub` claim carries our internal user id; Google's subject is a different identifier. `Processor.Rotate` returns `m.UserID()` — the internal id — so `GetByID` is correct. Passing one where the other is expected returns `ErrNotFound` silently rather than failing loudly, and that mistake once made `/auth/me` 404 for every valid token, bouncing users back to login forever. The plan's `fakeUsers.GetBySub` returns a loud error rather than data so a regression is caught, not absorbed.

**Read-your-own-write in the callback.** `ProvisionFromGoogle` inserts/updates the row, then the resolver immediately reads it back by primary key. Safe today: a single primary Postgres, no read replica in `deploy/k8s`, and the write has committed before `ProvisionFromGoogle` returns. This is the first place a read replica would break — recorded rather than assumed.

**`Title` must stay `"conflict"`.** `WriteError` sets `Title: err.Error()`, so the envelope surfaces error strings to the client by default. That is *why* `detailedError.Error()` returns the base message, and *why* the mismatch detail must be a constant with no format verb: there must be no code path that can put an email into a client-visible string.

**Precondition order is a disclosure control, not a style choice.** `accepted → expired → email`. A wrong-account caller presenting an already-accepted invite learns only "already accepted", never "…and it wasn't yours".

**The invite token is a bearer credential.** Anyone holding the URL reaches `POST /invites/{token}/accept`. `"invite was issued to alice@example.com"` would turn a leaked link into an address-disclosure oracle. FR-10 is a hard requirement.

**Do not modify the four existing tests in `invite/processor_test.go`.** Their `errors.Is(err, server.ErrConflict)` assertions passing unmodified is the proof that FR-8 — the status contract — was not violated. New tests tighten each case to its specific sentinel alongside them.

**A test-only package does not break the build.** `apps/auth-service/internal/arch` contains only `arch_test.go`. Verified empirically in this repo: `go build github.com/jtumidanski/myfleet/...` and `go vet` both skip a directory with no non-test Go files, exit 0.

## 6. Open Question 2 — closed

PRD §9.2 asked whether `telemetry.CorrelationID` runs before `authmw.JWT` in every service, since the warn log wants a correlation id. Verified across all four, and re-confirmed against source while writing the plan:

| Service | `Use(telemetry.CorrelationID)` | `pr.Use(authmw.JWT(…))` |
|---|---|---|
| auth-service | `cmd/main.go:85` (router root) | `:93` (route group) |
| fleet-service | `cmd/main.go:176` (router root) | `:183` (route group) |
| media-service | `cmd/main.go:130` (router root) | `:137` (route group) |
| notification-service | `cmd/main.go:70` (router root) | `:73` (route group) |

Correlation is a **root** middleware and JWT is a **route-group** middleware everywhere; chi runs root middlewares first. The correlation id is on the context before `JWT` executes, and before the fleet-service invite handler runs. **No ordering change is required.**

## 7. Open Question 1 — resolved as out of scope, with an accepted consequence

FR-5 collapses "membership unavailable" (transient — fleet-service down) and "user not found" (permanent) into one `401`, so a fleet-service outage logs users out rather than returning `503`.

**Keep the collapse.** The current closure (`cmd/main.go:49-55`) already returns any `fleetClient.Active` error to a handler that answers 401; this change adds a second error source with the same handling and does not make outage behaviour worse. Distinguishing them means classifying `membership.Client` errors by retryability and returning `503` with a `Retry-After` that `apiClient.ts` would have to learn not to treat as a logout — coherent work, wrong task.

**The accepted consequence:** because refresh now also clears the cookie (D6), a fleet-service outage that previously logged users out will additionally require re-authentication rather than a retry. Correct for the permanent case, slightly worse for the transient one.

**Recorded for a follow-up task:** *classify auth-service upstream failures and return 503 rather than 401 for transient membership resolution errors.*

## 8. Dependencies and blast radius of the change itself

**No `go.mod` change in any module.** `shared-go/auth` newly imports `shared-go/telemetry` and `sirupsen/logrus`. `telemetry` imports only `shared-go/config` plus stdlib and OTel — it does not import `auth` or `server`, so no cycle. `logrus` is already a direct dependency (`shared-go/go.mod:12`). `logrus/hooks/test`, used by Task 2's tests, imports only `io`, `sync`, and `logrus` — no testify, no new requirement.

**No schema change, no migration, no config change, no forced re-authentication.** Existing tokens carrying `"email": ""` stay valid until they expire (≤15 min) and are replaced on the next refresh. Every session self-heals within one access-token lifetime.

**Deployment ordering is unconstrained.** `shared-go` is consumed through the Go workspace — each Dockerfile copies `go.work` and `packages/` and builds from source (`apps/auth-service/Dockerfile:3-5`) — so there is no module version to bump and no window where a service builds against a stale `shared-go`. `fleet-service` and `auth-service` can ship in either order: the invite fix is inert until tokens carry an email, and corrected tokens are inert until the invite handler reads them.

**Rollback is a plain revert.** Nothing persists that a previous build cannot read.

**Untouched:** `apps/media-service`, `apps/notification-service` (inherit the corrected token and the compatible middleware signature; deliberately left on `auth.JWT(keyfn)` since neither consumes `Identity.Email`), `apps/web`, `packages/shared-ts` (`apiClient.ts` behaviour is correct as-is), `deploy/k8s`.

## 9. Verification

`make ci` — lint-check, vet, test, build, fe-test, fe-build. Node is not always on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

No deployment manifests change, so no `kustomize` render or server dry-run is required for this task.

Lint is golangci-lint v2 with the `standard` linter group (errcheck, govet, ineffassign, staticcheck, unused) plus gofumpt and goimports formatters, configured once at the repo root in `.golangci.yml`. `goimports` groups intra-repo imports separately under the `github.com/jtumidanski/myfleet` local prefix — match the existing import blocks.

Two steps in the plan (Task 5 Steps 3 and 6) deliberately break the code to prove each new guard fails, then restore it. Do not skip them: a guard that cannot fail is not a guard, and both of these tests are the entire justification for the "second occurrence" work.
