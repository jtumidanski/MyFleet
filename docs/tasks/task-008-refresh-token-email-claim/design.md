# Refresh Token Email Claim — Design

Version: v1
Status: Draft
Created: 2026-08-02
PRD: [prd.md](./prd.md)
Issue: [#6](https://github.com/jtumidanski/myfleet/issues/6)

---

## 1. Problem Restated in Structural Terms

Two call sites mint access tokens. Both build a `session.Principal` and hand it to
`session.Processor.MintAccess`, which mechanically maps each `Principal` field to a JWT claim
(`apps/auth-service/internal/session/processor.go:59`). `MintAccess` is not the defect — it is
faithful. The defect is upstream: `Principal` is an exported four-field struct that either call site
may fill in partially, and one of them does.

```
oidc/resource.go:115      session.Principal{UserID, Email, ActiveFleetID, Role}   ← complete
session/resource.go:62    session.Principal{UserID,        ActiveFleetID, Role}   ← Email absent
```

Both compile. Go zero-values the omitted field, `MintAccess` writes `"email": ""`, and the JWT
middleware faithfully propagates the empty string into `auth.Identity.Email`. Nothing anywhere fails
loudly.

The blast radius is narrow and confirmed by grep: `Identity.Email` has exactly **one** consumer in
the entire repository — `apps/fleet-service/internal/invite/resource.go:163`, which passes it to
`ValidateAccept`. That single consumer fails closed (an empty string never `EqualFold`s a real invite
email), so the bug produces a false rejection, never a false authorization. Populating the claim
strictly tightens behaviour; there is no consumer that must be re-audited for a
treated-empty-as-permissive assumption.

The design therefore has three separable concerns, and they are deliberately kept separable:

| Concern | Package | Nature |
|---|---|---|
| A. Remove the affordance that lets a call site build a partial `Principal` | `auth-service` | structural |
| B. Make the *symptom* diagnosable when it recurs | `fleet-service` | observability |
| C. Make the *cause class* diagnosable when it recurs | `shared-go/auth` | observability |

A is the fix. B and C are the "second occurrence" tax — this is the second identity-propagation
defect in this flow (PRD §1), so the work buys detection, not just repair.

---

## 2. Concern A — Single Principal Construction

### 2.1 Shape

`session.MembershipResolver` is deleted and replaced:

```go
// session/resource.go
type PrincipalResolver func(ctx context.Context, userID string) (Principal, error)
```

The type stays a **function value**, not an interface. This is not incidental: Decision 1 (recorded
at `cmd/main.go:45-47`) exists because `session` and `oidc` must not import the concrete
`internal/membership` client — `membership` is an outbound HTTP client to fleet-service, and pulling
it into the token-minting packages couples the two. A function value injected from `main` is the
existing mechanism for that, and widening the return type from `(string, string, error)` to
`(Principal, error)` preserves it exactly. An interface would also work and would buy nothing, at the
cost of a named type in one of the two packages that must not own it.

`oidc.Dependencies.Resolve` is retyped `session.PrincipalResolver` (FR-4).

### 2.2 Where the resolver is composed

In `apps/auth-service/cmd/main.go`, as a named function rather than an inline closure:

```go
// newPrincipalResolver composes the two sources of identity — the local users
// table for email, fleet-service for the active membership — into the single
// construction site for session.Principal. Every access token this service
// mints, on either path, is built here.
func newPrincipalResolver(users user.Provider, fleet *membership.Client) session.PrincipalResolver {
	return func(ctx context.Context, userID string) (session.Principal, error) {
		u, err := users.GetByID(userID)
		if err != nil {
			return session.Principal{}, err
		}
		m, err := fleet.Active(ctx, userID)
		if err != nil {
			return session.Principal{}, err
		}
		return session.Principal{
			UserID:        userID,
			Email:         u.Email(),
			ActiveFleetID: m.FleetID,
			Role:          m.Role,
		}, nil
	}
}
```

Named, not inline, for one reason: a `func main()` closure cannot be unit-tested, and this closure is
now the sole guarantor of claim completeness. As a package-level function it is reachable from a
`package main` test with two fakes.

`main()` shrinks to:

```go
users := user.NewProcessor(log, user.NewProvider(db), user.NewAdministrator(db))
userProv := user.NewProvider(db)   // see §2.3 on why the Provider, not the Processor
fleetClient := membership.NewClient(config.Get("FLEET_SERVICE_URL", "http://fleet-service:8080"))
resolve := newPrincipalResolver(userProv, fleetClient)
```

**Alternatives considered.** A dedicated `internal/principal` package importing `user`, `membership`,
and `session` would be marginally more DDD-orthodox and equally testable. It was rejected because
(a) nothing but `main` would ever import it, so it is a package that exists to hold one closure, and
(b) PRD §10 pins the construction site to `cmd/main.go` as an acceptance criterion. A method on
`user.Processor` was rejected outright: it would make the user domain depend on the membership
client, which is the coupling Decision 1 exists to prevent.

### 2.3 `user.Provider` vs `user.Processor`

The resolver takes the read-only `user.Provider` interface (`internal/user/provider.go:23`), not
`*user.Processor`. `Provider` is already an interface, so the resolver test needs no new seam, and
the resolver has no business reaching write methods. `main` constructs a second `user.NewProvider(db)`
alongside the one inside `users` — a stateless struct wrapping the shared `*gorm.DB`, so this costs
nothing and mirrors the existing precedent at `fleet-service/cmd/main.go:168`, where a second
stateless processor is constructed rather than reshaping an initializer.

`Provider.GetByID` is the correct lookup and its doc comment (`provider.go:11-22`) explains at length
why: the JWT `sub` claim carries our internal user id, not Google's subject. Confusing the two was the
*first* identity-propagation defect in this flow — the one that logged every user out. `Rotate`
returns `m.UserID()`, the internal id, so `GetByID` is right. This is called out here because it is
precisely the mistake this codebase has already made once.

### 2.4 Lookup order and failure semantics

Order is deliberate: **local DB read first, network call second.** A missing user is the permanent,
cheap-to-detect failure; short-circuiting on it avoids an HTTP round trip to fleet-service for a
request that cannot succeed.

| Condition | Resolver behaviour | Rationale |
|---|---|---|
| `users.GetByID` → `user.ErrNotFound` | return error | Refresh token outlives its user row. Fail closed (FR-5). |
| `users.GetByID` → other DB error | return error | Fail closed. |
| `fleet.Active` → transport/decode error | return error | Preserves today's behaviour exactly. |
| `fleet.Active` → HTTP 404 | **not an error** — `Membership{}` zero value | **Load-bearing.** See below. |

The 404 case matters and is easy to break. `membership.Client.Active` maps a 404 to
`(Membership{}, nil)` (`internal/membership/client.go:31`), so a user with no fleet resolves to an
empty `ActiveFleetID`. The OIDC callback keys its onboarding redirect off exactly that
(`oidc/resource.go:141`). Turning "no membership" into a resolver error would break new-user signup
on the first login. The resolver must propagate the zero value untouched.

**Should the resolver reject an empty `u.Email()`?** No. It is tempting — minting a token we can see
is defective is the very bug class in hand — but rejecting would hard-lock out any legacy row with a
blank email, converting a cosmetic defect into a total denial of service for that account, and PRD §2
explicitly makes enforcement a non-goal. Concern C's warn log is the chosen response: loud, not fatal.

### 2.5 Call site changes

**`session/resource.go` (refresh).** Lines 55-67 collapse to one resolver call plus a pass-through:

```go
principal, err := resolve(req.Context(), userID)
if err != nil {
	log.WithError(err).Error("resolve principal on refresh")
	clearRefreshCookie(w, cookieSecure)
	server.WriteError(w, server.ErrUnauthorized)
	return
}
access, err := proc.MintAccess(principal)
```

Note `clearRefreshCookie` on the resolver-error path. Today's code (line 56-60) returns 401 **without**
clearing the cookie, unlike every other 401 in the handler. PRD FR-5 specifies clearing it, so this is
a small deliberate behaviour change, not an oversight: a session whose user row is gone should not keep
a cookie that will 401 forever.

**`oidc/resource.go` (callback).** Lines 108-120 collapse likewise. The handler stops calling
`u.Email()` and reads everything downstream from the resolved principal:

```go
principal, err := d.Resolve(ctx, u.ID())
// ... fail closed: log + 500 "authentication failed" (FR-6, unchanged)
access, err := d.Sessions.MintAccess(principal)
// ...
refresh, err := d.Sessions.IssueRefresh(principal.UserID)
// ...
if principal.ActiveFleetID == "" { dest = d.AppBaseURL + d.OnboardingPath }
```

`u` is still needed — `ProvisionFromGoogle` must run to create-or-update the row before the resolver
reads it — but `u.Email()` is no longer consumed. Only `u.ID()` is, to seed the lookup.

This introduces a **read-your-own-write**: `ProvisionFromGoogle` inserts/updates the row, then the
resolver immediately reads it back by primary key. Safe here — there is a single primary Postgres and
no read replica in `deploy/k8s`, and the write has committed before `ProvisionFromGoogle` returns. Were
a replica ever introduced, this is the first place that would break, so it is recorded rather than
assumed.

Cost: one indexed primary-key read per login, on a path that has already performed an OAuth token
exchange and an ID-token verification against Google. Negligible, and the PRD accepts it (§8) in
exchange for the single construction site.

---

## 3. Guarding Parity Structurally

"Make claim divergence structurally impossible" (PRD §2) cannot be fully achieved by types in Go: a
partial composite literal is always legal. Three layers get progressively closer, and each catches a
different regression.

### Layer 1 — Type flow (prevents today's bug)

After §2, `Principal` values originate only from the resolver, and both call sites pass them to
`MintAccess` unmodified. There is no partially-filled `Principal` anywhere for a reader to copy.

### Layer 2 — Reflection test over `Principal` → claims (catches a *new field*)

The scenario Layer 1 does not cover: someone adds `Principal.TenantID`, wires it in the resolver, and
forgets `MintAccess`. A test in `session` that walks the struct by reflection catches it without
naming fields:

```go
// TestMintAccess_mapsEveryPrincipalField fails when a field is added to
// Principal but not wired into MintAccess's claim map. It deliberately names no
// field: a test that enumerates fields is a test that has to be remembered.
func TestMintAccess_mapsEveryPrincipalField(t *testing.T) {
	p := reflect.New(reflect.TypeOf(Principal{})).Elem()
	want := map[string]string{}
	for i := 0; i < p.NumField(); i++ {
		f := p.Type().Field(i)
		if f.Type.Kind() != reflect.String {
			t.Fatalf("Principal.%s is not a string; extend this test", f.Name)
		}
		v := "sentinel-" + f.Name
		p.Field(i).SetString(v)
		want[f.Name] = v
	}
	// mint, parse claims, assert every sentinel value appears as some claim value
}
```

The `Kind() != String` guard is the point of the `t.Fatalf`: a non-string field means the test's
sentinel scheme no longer holds, and it should say so rather than silently skip.

### Layer 3 — Architecture test forbidding `Principal{…}` outside the resolver (catches a *new call site*)

PRD §10 states as an acceptance criterion that neither `session/resource.go` nor `oidc/resource.go`
contains a `Principal{...}` literal. Verifying that by review is exactly the discipline that failed
the first time. A ~40-line test parses both files with `go/parser` and walks for a
`CompositeLit` whose type is `Principal` or `session.Principal`:

```
apps/auth-service/internal/arch/arch_test.go   (package arch — test-only, no production code)
```

An empty package existing solely to hold an architecture test is unusual but honest; the alternative
placements are worse (a test in `session` reaching into `../oidc/` source, or a test in `oidc`
reaching into `../session/`). AST parsing rather than a string grep because `Principal{` appears in
comments and would produce false failures the first time someone documents the type.

**Cost check.** Layer 3 is the most arguable of the three. It is included because the PRD's framing —
second occurrence, "make a future silent recurrence loud" — makes the marginal 40 lines cheap against
a third occurrence. If it proves brittle in review it can be dropped without touching Layers 1 and 2,
which carry the functional guarantee.

### What is *not* built: a true two-path end-to-end parity test

PRD §10 asks for "a session-level test [that] mints one token via the callback path and one via the
refresh path for the same user and asserts the two claim key sets are identical." Driving the real
callback path in a test requires standing in for Google's token endpoint and forging a verifiable
ID token against `idtoken.Validate` — a large fake for a property that Layers 1-3 already pin more
tightly than the e2e test would. The design substitutes: Layer 3 proves both paths mint from the same
constructor, Layer 2 proves that constructor's output reaches every claim, and a direct unit test of
`newPrincipalResolver` proves the constructor fills every field. Together these imply the parity
property and, unlike the e2e test, they name the failing component when they break.

This is a deliberate deviation from a written acceptance criterion and should be confirmed at plan
time rather than silently absorbed. Manual end-to-end coverage of the user-visible outcome remains
(the last criterion in PRD §10: authenticate, force refresh, accept invite).

---

## 4. Concern B — Invite Acceptance Diagnostics

### 4.1 The gap the PRD did not see

FR-9 requires the 409 body to carry a distinguishing JSON:API `detail`. `server.APIError` has a
`Detail` field (`errors.go:47`) — **and nothing in the repository ever sets it.** `WriteError`
populates `Status`, `Code`, and `Title: err.Error()` only (`jsonapi.go:28-37`). So FR-9 is not
reachable by adding sentinels alone; it needs a way to carry a detail string from a domain error to
the envelope. That is a `shared-go/server` change the PRD's §7 Service Impact does not list.

Three ways to close it:

| Option | Mechanism | Assessment |
|---|---|---|
| A | New `server.WriteErrorDetail(w, err, detail)` | Detail lives at the transport call site, sentinel at the domain — two places to keep in sync, and the handler must re-switch on the sentinel it just received. |
| B | `server.Detailed(err, detail) error`; `WriteError` unwraps it | Detail travels *with* the sentinel. Zero changes at 100+ existing `WriteError` call sites. **Chosen.** |
| C | Handler hand-builds the envelope via `WriteJSON` | Duplicates envelope construction outside `server`. Rejected. |

### 4.2 Option B in detail

```go
// packages/shared-go/server/errors.go

// Detailed wraps a status sentinel with a human-readable JSON:API `detail`.
// Error() returns the base sentinel's message so the envelope `title` stays the
// canonical status word; the detail rides in the `detail` field. errors.Is
// against both the wrapper (identity) and the base sentinel (Unwrap) works.
func Detailed(base error, detail string) error { return &detailedError{base: base, detail: detail} }

type detailedError struct {
	base   error
	detail string
}

func (e *detailedError) Error() string  { return e.base.Error() }
func (e *detailedError) Unwrap() error  { return e.base }
func (e *detailedError) Detail() string { return e.detail }
```

`WriteError` gains four lines:

```go
var d interface{ Detail() string }
if errors.As(err, &d) {
	apiErr.Detail = d.Detail()
}
```

Three properties make this safe to drop into a shared package:

- **`Error()` returns the base message, not a concatenation.** `Title` stays `"conflict"`. Had the
  sentinels been built with `fmt.Errorf("...: %w", ErrConflict)`, `Title` would have become
  `"invite has already been accepted: conflict"` — a shape change to every 409 body.
- **`StatusFor` is unchanged.** It uses `errors.Is`, which follows `Unwrap` to `ErrConflict` → 409.
  FR-8 (status contract unchanged) holds without any mapping code.
- **Sentinels compare by pointer identity.** Two `Detailed(ErrConflict, …)` values with different
  details are distinct errors, so `errors.Is(err, ErrEmailMismatch)` discriminates precisely.

### 4.3 The sentinels

```go
// apps/fleet-service/internal/invite/processor.go
var (
	ErrAlreadyAccepted = server.Detailed(server.ErrConflict, "invite has already been accepted")
	ErrInviteExpired   = server.Detailed(server.ErrConflict, "invite has expired")
	ErrEmailMismatch   = server.Detailed(server.ErrConflict, "invite was issued to a different account")
)
```

`ValidateAccept` returns these in place of bare `server.ErrConflict`. Precondition order is unchanged
(accepted → expired → email), which matters: it means a *wrong-account* caller presenting an
already-accepted invite still learns only "already accepted," never "…and it wasn't yours."

The three existing tests in `processor_test.go` assert `errors.Is(err, server.ErrConflict)` and
**keep passing unmodified** — they are the regression guard that FR-8 was not violated. New tests
tighten each to its specific sentinel.

### 4.4 FR-10 — non-disclosure

The mismatch detail is a compile-time constant with no interpolation. There is no code path that can
put an email into it; enforcement is the absence of a format verb, not a runtime check. A test asserts
the rendered 409 body contains neither the invite address nor the authenticated address.

This is a hard requirement rather than a nicety because the invite token is a bearer credential: any
holder of the URL reaches this endpoint, and "invite was issued to `alice@example.com`" would turn a
leaked invite link into an address-disclosure oracle. Note also that the pre-existing `Title:
err.Error()` behaviour is what makes constant-only details necessary — the envelope surfaces error
strings to the client by default.

### 4.5 Handler wiring

In the accept handler (`invite/resource.go:163`):

```go
if err := proc.ValidateAccept(inv, identity.Email); err != nil {
	if errors.Is(err, ErrEmailMismatch) {
		log.WithFields(logrus.Fields{
			"invite_id":      inv.ID(),
			"correlation_id": telemetry.CorrelationIDFromContext(req.Context()),
		}).Warn("invite accept rejected: email mismatch")
	}
	server.WriteError(w, err)
	return
}
```

Invite id and correlation id only — never `inv.Email()`, never `identity.Email` (PRD §8). The
`invite_id` is sufficient to join to the row in the database, where an operator with DB access can see
the address they already have access to; the log line itself adds no disclosure. `logrus`,
`telemetry`, and `errors` are all already imported by this file.

Only `ErrEmailMismatch` is logged. Already-accepted and expired are ordinary, self-explanatory user
outcomes now visible in the response body; logging them adds noise without adding information.

---

## 5. Concern C — Empty-Claim Observability in `shared-go/auth`

### 5.1 Options API

```go
type Option func(*jwtConfig)

type jwtConfig struct{ log logrus.FieldLogger }

func WithLogger(l logrus.FieldLogger) Option { return func(c *jwtConfig) { c.log = l } }

func JWT(keyfn jwt.Keyfunc, opts ...Option) func(http.Handler) http.Handler
```

Variadic options keep all four existing call sites (`auth-service:93`, `fleet-service:183`,
`media-service:137`, `notification-service:73`) compiling untouched (FR-12). `auth-service` and
`fleet-service` gain `auth.WithLogger(log)`; `media-service` and `notification-service` are left alone
(FR-15) — they have no `Identity.Email` consumer, so a warn from them would be noise about a claim they
never read.

The no-op default (FR-14) is a nil check at the single log site, not a discard logger. One branch, no
allocation, and `cfg.log == nil` reads unambiguously as "not configured."

### 5.2 Folding `jwtWithKeyfunc`

`JWT` currently delegates to an unexported `jwtWithKeyfunc` that adds no behaviour and exists only
because the tests call it. With options in play, keeping both means threading `jwtConfig` through a
pointless hop. `jwtWithKeyfunc` is deleted and the two tests that reference it
(`middleware_test.go:25,43`) call `JWT(keyfn)` directly — which is strictly better, since they then
exercise the exported surface.

### 5.3 The warn

```go
if cfg.log != nil && id.Email == "" {
	cfg.log.WithFields(logrus.Fields{
		"sub":            id.UserID,
		"correlation_id": telemetry.CorrelationIDFromContext(r.Context()),
	}).Warn("access token missing email claim")
}
next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
```

Logged after successful validation and before dispatch. The request proceeds — observability, not
enforcement (FR-13, and PRD §2 non-goal). Never the raw token, never an email.

### 5.4 Resolving Open Question 2 — middleware ordering

PRD §9.2 asks whether `telemetry.CorrelationID` runs before `authmw.JWT` in every service. Verified
across all four:

| Service | `Use(telemetry.CorrelationID)` | `pr.Use(authmw.JWT(…))` |
|---|---|---|
| auth-service | `cmd/main.go:85` (router root) | `:93` (route group) |
| fleet-service | `cmd/main.go:176` (router root) | `:183` (route group) |
| media-service | `cmd/main.go:130` (router root) | `:137` (route group) |
| notification-service | `cmd/main.go:70` (router root) | `:73` (route group) |

In every case correlation is a **root** middleware and JWT is a **route-group** middleware; chi runs
root middlewares first, so the correlation id is on the context before `JWT` executes. The same
ordering guarantees it for the fleet-service invite handler in §4.5. **Open Question 2 is closed: no
ordering change required.**

### 5.5 Import-cycle check

`auth` would newly import `shared-go/telemetry` and `sirupsen/logrus`. `telemetry` imports only
`shared-go/config` (plus stdlib and OTel); it does not import `auth` or `server`. No cycle. `logrus`
is already a direct dependency of `shared-go` (`go.mod:12`). No `go.mod` change in any module.

---

## 6. Open Question 1 — Resolver Error Granularity

PRD §9.1 flags that FR-5 collapses a transient fleet-service outage and a permanent missing-user into
one 401, so a fleet-service outage logs users out.

**Resolved: keep the collapse. Out of scope, and not a regression.**

The current `resolve` closure (`cmd/main.go:49-55`) already returns any `fleetClient.Active` error to a
handler that answers 401. This change adds a second error source (the user lookup) with the same
handling; it does not make outage behaviour worse. Distinguishing transient from permanent would mean
classifying `membership.Client` errors — deciding which HTTP statuses and transport failures are
retryable — and returning 503 with a `Retry-After` that `apiClient.ts` would have to learn not to treat
as a logout. That is a coherent piece of work and a poor fit for a bug fix whose entire risk profile
is "one field is empty."

Recorded for a follow-up task: *classify auth-service upstream failures and return 503 rather than 401
for transient membership resolution errors.*

There is a real, accepted consequence: after this change a fleet-service outage that previously logged
users out on refresh will additionally clear the refresh cookie (§2.5), so recovery requires
re-authentication rather than a retry. Cookie-clearing is correct for the permanent case and slightly
worse for the transient one — which is precisely what the follow-up would fix.

---

## 7. Testing Strategy

| # | Test | Location | Guards |
|---|---|---|---|
| 1 | `newPrincipalResolver` fills all four fields from fakes | `apps/auth-service/cmd/main_test.go` | FR-1, FR-2 |
| 2 | resolver returns error when user lookup misses | same | FR-5 |
| 3 | resolver returns error when membership client errors | same | FR-5 |
| 4 | resolver returns empty fleet/role (no error) on membership 404 | same | §2.4 — onboarding path |
| 5 | reflection test: every `Principal` field reaches a claim | `internal/session/processor_test.go` | claim parity (Layer 2) |
| 6 | AST test: no `Principal{…}` literal in `session`/`oidc` resources | `internal/arch/arch_test.go` | claim parity (Layer 3) |
| 7 | refresh → 401 + cleared cookie + no token when resolver errors | `internal/session/resource_test.go` (new) | FR-5 |
| 8 | refresh happy path, rotation, reuse detection unchanged | `internal/session/processor_test.go` (existing) | no regression |
| 9 | `ValidateAccept` returns each of the three sentinels + success | `internal/invite/processor_test.go` | FR-7 |
| 10 | existing `errors.Is(err, server.ErrConflict)` assertions still pass | same (unmodified) | FR-8 |
| 11 | each sentinel renders 409 with its own `detail` | `internal/invite/resource_test.go` | FR-8, FR-9 |
| 12 | mismatch `detail` contains neither email address | same | FR-10 |
| 13 | `server.Detailed` — `StatusFor` unchanged, `Title` unchanged, `Detail` set, `errors.Is` both ways | `packages/shared-go/server/errors_test.go` | §4.2 |
| 14 | `JWT(keyfn)` compiles and behaves as today | `packages/shared-go/auth/middleware_test.go` | FR-12, FR-14 |
| 15 | `JWT(keyfn, WithLogger(hook))` warns on empty email **and still calls next** | same | FR-13 |
| 16 | manual e2e: login → force refresh → accept invite → 200 | — | PRD §10 |

Test 15 uses `logrus/hooks/test.NewNullLogger()` to assert the entry without writing output; asserting
"still calls next" in the same test is what keeps the observability change from quietly becoming
enforcement.

Test 7 needs a `PrincipalResolver` stub and an `httptest` request carrying a refresh cookie against a
processor wired with the existing `fakeStore` (`session/processor_test.go:18`) — no new fake
infrastructure.

Verification per CLAUDE.md: `make ci`. No deployment manifests change, so no `kustomize` render is
required for this task.

---

## 8. Rollout

No schema change, no migration, no config change, no forced re-authentication. Existing tokens with
`"email": ""` stay valid until they expire (≤15 min) and are replaced on the next refresh; every
session self-heals within one access-token lifetime.

Deployment ordering is unconstrained. `shared-go` is consumed through the Go workspace — each
Dockerfile copies `go.work` and `packages/` and builds from source (`apps/auth-service/Dockerfile:3-5`),
so there is no module version to bump and no window where a service builds against a stale `shared-go`.
`fleet-service` and `auth-service` can ship in either order: the invite fix is inert until tokens carry
an email, and the corrected tokens are inert until the invite handler reads them.

Rollback is a plain revert. Nothing persists that a previous build cannot read.

---

## 9. Summary of Decisions

| # | Decision | Alternative rejected |
|---|---|---|
| D1 | `PrincipalResolver` is a function value returning `session.Principal` | Interface — no benefit, and puts a named type in a package that must not own it |
| D2 | Composed in a named `newPrincipalResolver` in `cmd/main.go` | Inline closure (untestable); `internal/principal` package (exists to hold one closure) |
| D3 | Resolver takes `user.Provider`, reads local DB before the network call | `*user.Processor` — wider surface, no existing interface seam |
| D4 | Membership 404 → empty fleet, not an error | Treating it as an error breaks new-user onboarding |
| D5 | Resolver does not reject an empty email | Enforcement would lock out legacy rows; PRD non-goal |
| D6 | Refresh clears the cookie on resolver error | Today's non-clearing 401 — inconsistent with every other 401 in the handler |
| D7 | Parity guarded by reflection test + AST test, not a two-path e2e | e2e needs a Google token-endpoint fake and localises failures worse |
| D8 | `server.Detailed` carries the detail with the sentinel | `WriteErrorDetail` at the call site (two places to sync); hand-built envelope (duplication) |
| D9 | `Detailed.Error()` returns the base message | `fmt.Errorf("%w")` wrapping — would change `title` on every 409 |
| D10 | `auth.JWT` variadic options, nil-logger no-op, `jwtWithKeyfunc` folded in | Separate `JWTWithLogger` constructor — two entry points to keep in step |
| D11 | Open Question 1 resolved as out of scope; 401 collapse retained | 503 classification — real work, wrong task |
| D12 | Open Question 2 closed by verification; no ordering change | — |
