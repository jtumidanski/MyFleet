# Refresh Token Email Claim Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Access tokens minted by `POST /auth/refresh` carry the `email` claim, by giving `session.Principal` a single construction site, and make a future recurrence of this defect class loud rather than silent.

**Architecture:** Three separable concerns. (A) `auth-service` replaces `session.MembershipResolver` with a `session.PrincipalResolver` that returns a fully-populated `Principal`, composed once in `cmd/main.go`; both minting call sites pass its result to `MintAccess` unmodified. (B) `fleet-service` splits `ValidateAccept`'s single `server.ErrConflict` into three sentinels carrying distinguishable JSON:API `detail` strings, enabled by a new `server.Detailed` wrapper in `shared-go`. (C) `shared-go/auth.JWT` gains variadic options so `auth-service` and `fleet-service` can attach a logger that warns when a validated token carries an empty `email` claim.

**Tech Stack:** Go 1.25 workspace (`go.work`, six modules), chi v5 router, GORM + Postgres (sqlite in-memory for tests), logrus, `golang-jwt/jwt/v5`, JSON:API envelopes via `packages/shared-go/server`.

## Global Constraints

- **Never log an email address, a raw access token, or a raw refresh token.** Log the `sub`/user id, the invite id, and the correlation id only. (PRD §8, FR-10, FR-11, FR-13.)
- **The `ErrEmailMismatch` detail string must be a compile-time constant with no format verb.** The invite token is a bearer credential; anyone holding the link reaches the accept endpoint, so the response must not disclose who the invite was addressed to. (PRD FR-10.)
- **HTTP status contract does not change.** All three invite sentinels still render `409`. `POST /auth/refresh` and `POST /invites/{token}/accept` keep their request and response shapes. (PRD §5, FR-8.)
- **`auth.JWT(keyfn)` — the existing single-argument form — must keep compiling at all four call sites.** `media-service` and `notification-service` are deliberately left on it. (PRD FR-12, FR-15.)
- **The JWT middleware never rejects a token for an empty `email` claim.** Observability only; the request proceeds. (PRD §2 non-goal, FR-13.)
- **No schema change, no migration, no config change, no `go.mod` change in any module.** (PRD §6, design §5.5, §8.)
- Verification command is `make ci` (lint-check, vet, test, build, fe-test, fe-build) per `CLAUDE.md`. No deployment manifest changes, so no `kustomize` render is required for this task.
- All work happens in the worktree `/home/tumidanski/source/MyFleet/.worktrees/task-008-refresh-token-email-claim` on branch `task-008-refresh-token-email-claim`. Never edit the main checkout.

## File Structure

| File | Task | Responsibility |
|---|---|---|
| `packages/shared-go/server/errors.go` | 1 | Add `Detailed` + unexported `detailedError` carrying a JSON:API `detail` alongside a status sentinel |
| `packages/shared-go/server/jsonapi.go` | 1 | `WriteError` reads the detail off the error chain via `errors.As` |
| `packages/shared-go/server/errors_test.go` | 1 | Pins `StatusFor`, `Title`, `errors.Is` both directions, rendered body with and without a detail |
| `packages/shared-go/auth/middleware.go` | 2 | `Option` / `jwtConfig` / `WithLogger`; `jwtWithKeyfunc` folded into `JWT`; empty-email warn |
| `packages/shared-go/auth/middleware_test.go` | 2 | Existing tests moved onto the exported `JWT`; warn-and-still-call-next; no warn when email present |
| `apps/fleet-service/internal/invite/processor.go` | 3 | Three sentinels; `ValidateAccept` returns them in place of bare `server.ErrConflict` |
| `apps/fleet-service/internal/invite/resource.go` | 3 | Accept handler warn-logs `ErrEmailMismatch` (invite id + correlation id only) |
| `apps/fleet-service/internal/invite/processor_test.go` | 3 | Existing `errors.Is(err, server.ErrConflict)` assertions kept as the FR-8 regression guard; new per-sentinel assertions |
| `apps/fleet-service/internal/invite/resource_test.go` | 3 | **New.** Drives the real accept route; asserts 409 + distinct detail per case + non-disclosure |
| `apps/fleet-service/cmd/main.go` | 3 | `authmw.JWT(keyfn, authmw.WithLogger(log))` |
| `apps/auth-service/internal/session/resource.go` | 4 | `PrincipalResolver` replaces `MembershipResolver`; refresh handler fails closed and clears the cookie |
| `apps/auth-service/internal/oidc/resource.go` | 4 | `Dependencies.Resolve` retyped; callback stops assembling its own `Principal` |
| `apps/auth-service/cmd/main.go` | 4 | `newPrincipalResolver` — the single construction site; JWT logger wiring |
| `apps/auth-service/cmd/main_test.go` | 4 | **New.** Resolver unit tests: all four fields, user-miss, membership error, membership 404 |
| `apps/auth-service/internal/session/resource_test.go` | 4 | **New.** Refresh handler: happy path mints a non-empty email; resolver error → 401 + cleared cookie + no token |
| `apps/auth-service/internal/session/processor_test.go` | 5 | Reflection test: every `Principal` field reaches a claim |
| `apps/auth-service/internal/arch/arch_test.go` | 5 | **New, test-only package.** AST test: no `Principal{…}` literal in either resource file |

**Task ordering is load-bearing.** Task 1 must land before Task 3 (the sentinels call `server.Detailed`). Task 2 must land before Tasks 3 and 4 (both pass `authmw.WithLogger`). Task 4 is a single atomic commit because retyping `MembershipResolver` → `PrincipalResolver` breaks `session`, `oidc`, and `main` simultaneously — the tree does not compile between the call-site edits. Task 5 depends on Task 4 (the arch test asserts the post-Task-4 state).

## Deviation from a written acceptance criterion (confirmed at plan time)

PRD §10 asks for a two-path claim-parity test: mint one token via the callback path and one via the refresh path for the same user and compare claim key sets. Design §3 declines it — driving the real callback in a test requires standing in for Google's token endpoint and forging an ID token that passes `idtoken.Validate`, a large fake for a property three narrower tests pin more tightly. **This deviation was confirmed with the user at plan time.** The substitute coverage is:

- Task 4's `TestNewPrincipalResolver_fillsEveryField` — the constructor fills every field.
- Task 5's `TestMintAccess_mapsEveryPrincipalField` — every field the constructor fills reaches a claim.
- Task 5's `TestNoPrincipalLiteralOutsideResolver` — both minting paths use that one constructor.
- Task 4's `TestRefresh_mintsAccessTokenCarryingEmailClaim` — the direct, user-visible proof at the refresh route.
- Task 6's manual end-to-end check — login, force refresh, accept invite.

---

### Task 1: `server.Detailed` — carry a JSON:API detail with a status sentinel

`server.APIError` has a `Detail` field (`errors.go:47`) and nothing in the repository ever sets it — `WriteError` populates `Status`, `Code`, and `Title` only. FR-9 is therefore not reachable by adding sentinels alone. `Detailed` lets a detail string travel *with* the sentinel, so no existing `WriteError` call site changes.

**Files:**
- Modify: `packages/shared-go/server/errors.go` (append after the `StatusFor` function, before `APIError`)
- Modify: `packages/shared-go/server/jsonapi.go:28-38`
- Test: `packages/shared-go/server/errors_test.go` (append)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func server.Detailed(base error, detail string) error`. The returned error satisfies `Error() string` (returns the *base* sentinel's message), `Unwrap() error` (returns `base`), and `Detail() string`. `errors.Is(Detailed(X, …), X)` is true; two `Detailed` values built from the same base with different details are distinct under `errors.Is`. Task 3 builds its three sentinels with it.

- [ ] **Step 1: Write the failing tests**

Append to `packages/shared-go/server/errors_test.go`, and add `"encoding/json"`, `"errors"`, `"net/http/httptest"`, `"strings"` to that file's imports:

```go
// TestDetailed_keepsStatusAndTitleWhileCarryingDetail pins the three properties
// that make Detailed safe to drop into a package 100+ call sites already use:
// the status mapping is unchanged (errors.Is follows Unwrap), the envelope
// `title` stays the canonical status word rather than becoming a sentence, and
// two sentinels sharing a base are still distinguishable by errors.Is.
func TestDetailed_keepsStatusAndTitleWhileCarryingDetail(t *testing.T) {
	expired := Detailed(ErrConflict, "invite has expired")
	accepted := Detailed(ErrConflict, "invite has already been accepted")

	if got := StatusFor(expired); got != 409 {
		t.Fatalf("StatusFor = %d, want 409 — StatusFor must follow Unwrap to the base sentinel", got)
	}
	if got := expired.Error(); got != ErrConflict.Error() {
		t.Fatalf("Error() = %q, want %q — the envelope title must stay the canonical status word", got, ErrConflict.Error())
	}
	if !errors.Is(expired, ErrConflict) {
		t.Fatal("errors.Is(detailed, ErrConflict) must be true")
	}
	if !errors.Is(expired, expired) {
		t.Fatal("errors.Is(detailed, itself) must be true so a handler can discriminate one sentinel")
	}
	if errors.Is(expired, accepted) {
		t.Fatal("two Detailed sentinels over the same base must not compare equal")
	}
}

func TestWriteError_rendersDetailWhenTheErrorCarriesOne(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, Detailed(ErrConflict, "invite was issued to a different account"))

	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body struct {
		Errors []APIError `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(body.Errors))
	}
	got := body.Errors[0]
	if got.Status != "409" || got.Code != "conflict" {
		t.Fatalf("status/code = %q/%q, want 409/conflict", got.Status, got.Code)
	}
	if got.Title != "conflict" {
		t.Fatalf("title = %q, want %q — the detail must not leak into the title", got.Title, "conflict")
	}
	if got.Detail != "invite was issued to a different account" {
		t.Fatalf("detail = %q, want the detail passed to Detailed", got.Detail)
	}
}

// TestWriteError_omitsDetailForAPlainSentinel is the regression guard for the
// 100+ existing WriteError call sites: their response bodies must be
// byte-identical to what they were before Detailed existed.
func TestWriteError_omitsDetailForAPlainSentinel(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, ErrConflict)

	if strings.Contains(rec.Body.String(), "detail") {
		t.Fatalf("plain sentinel rendered a detail key: %s", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/packages/shared-go/server -run 'Detailed|WriteError' -v
```

Expected: compile failure — `undefined: Detailed`.

- [ ] **Step 3: Add `Detailed` to `errors.go`**

Insert between the closing brace of `StatusFor` and the `// APIError is one entry…` comment:

```go
// Detailed wraps a status sentinel with a human-readable JSON:API `detail`.
//
// Error() returns the BASE sentinel's message, not a concatenation: the
// envelope's `title` is `err.Error()`, so wrapping with fmt.Errorf("...: %w")
// would turn every 409 title into a sentence and change the response shape for
// every existing caller. The detail rides in the `detail` field instead.
//
// errors.Is matches both the wrapper (pointer identity, so a handler can tell
// two sentinels over the same base apart) and the base sentinel (via Unwrap, so
// StatusFor keeps mapping it to the right status with no new mapping code).
func Detailed(base error, detail string) error {
	return &detailedError{base: base, detail: detail}
}

type detailedError struct {
	base   error
	detail string
}

func (e *detailedError) Error() string  { return e.base.Error() }
func (e *detailedError) Unwrap() error  { return e.base }
func (e *detailedError) Detail() string { return e.detail }
```

- [ ] **Step 4: Teach `WriteError` to read the detail**

Replace `packages/shared-go/server/jsonapi.go` lines 28-38 with:

```go
// WriteError renders the standard envelope for a domain/HTTP error. If the
// error chain carries a detail (see Detailed), it is rendered in the JSON:API
// `detail` field; otherwise the field is omitted and the body is unchanged from
// what every existing caller already produces.
func WriteError(w http.ResponseWriter, err error) {
	status := StatusFor(err)
	apiErr := APIError{
		Status: itoa(status),
		Code:   codeFor(status),
		Title:  err.Error(),
	}
	var d interface{ Detail() string }
	if errors.As(err, &d) {
		apiErr.Detail = d.Detail()
	}
	WriteJSON(w, status, struct {
		Errors []APIError `json:"errors"`
	}{Errors: []APIError{apiErr}})
}
```

Add `"errors"` to that file's import block (it currently imports `"encoding/json"` and `"net/http"`).

- [ ] **Step 5: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/packages/shared-go/... -race
```

Expected: PASS, including the pre-existing `TestStatusFor_mapsDomainErrors` and `TestCodeFor_namesEveryMappedStatus`.

- [ ] **Step 6: Verify nothing else in the workspace broke**

```sh
go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
```

Expected: no output, exit 0.

- [ ] **Step 7: Commit**

```sh
git add packages/shared-go/server/errors.go packages/shared-go/server/jsonapi.go packages/shared-go/server/errors_test.go
git commit -m "feat(shared-go): carry a JSON:API detail alongside a status sentinel"
```

---

### Task 2: `auth.JWT` variadic options and the empty-email warn

Makes the *cause class* diagnosable when it recurs. A token that validates but carries no email is the signature of a minting path that built a partial principal.

**Files:**
- Modify: `packages/shared-go/auth/middleware.go:12-38`
- Test: `packages/shared-go/auth/middleware_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `type auth.Option func(*jwtConfig)`; `func auth.WithLogger(l logrus.FieldLogger) auth.Option`; `func auth.JWT(keyfn jwt.Keyfunc, opts ...auth.Option) func(http.Handler) http.Handler`. The unexported `jwtWithKeyfunc` is **deleted**. Tasks 3 and 4 call `authmw.JWT(keyfn, authmw.WithLogger(log))`.

- [ ] **Step 1: Write the failing tests**

In `packages/shared-go/auth/middleware_test.go`, first change the two existing call sites so they exercise the exported surface — line 25 and line 44, `jwtWithKeyfunc(` → `JWT(`:

```go
	mw := JWT(func(*jwt.Token) (any, error) { return nil, nil })
```

```go
	mw := JWT(func(*jwt.Token) (any, error) { return &key.PublicKey, nil })
```

Then append these two tests, adding `"strings"`, `"github.com/sirupsen/logrus"`, and `"github.com/sirupsen/logrus/hooks/test"` to the imports:

```go
// TestJWT_warnsOnEmptyEmailClaimAndStillCallsNext is the guard that keeps this
// observability change from quietly becoming enforcement. The warn exists so a
// regression of the empty-claim class surfaces in the logs before a user
// reports a rejected invite; it must never cost the request.
func TestJWT_warnsOnEmptyEmailClaimAndStillCallsNext(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":             "user-1",
		"email":           "",
		"active_fleet_id": "fleet-9",
		"role":            "owner",
		"exp":             time.Now().Add(time.Hour).Unix(),
	})

	log, hook := test.NewNullLogger()
	called := false
	mw := JWT(func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, WithLogger(log))
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("an empty email claim must not block the request — this is observability, not enforcement")
	}
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("status = %d, want the request to proceed", rec.Code)
	}

	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want exactly 1", len(entries))
	}
	e := entries[0]
	if e.Level != logrus.WarnLevel {
		t.Fatalf("level = %v, want warn", e.Level)
	}
	if e.Data["sub"] != "user-1" {
		t.Fatalf("sub field = %v, want user-1", e.Data["sub"])
	}
	if _, ok := e.Data["correlation_id"]; !ok {
		t.Fatal("warn must carry a correlation_id field so the line joins to the request")
	}
	// Never the raw token (Global Constraints, PRD §8).
	if strings.Contains(e.Message, tokenStr) {
		t.Fatal("log message must not contain the raw token")
	}
	for k, v := range e.Data {
		if s, ok := v.(string); ok && strings.Contains(s, tokenStr) {
			t.Fatalf("log field %q must not contain the raw token", k)
		}
	}
}

func TestJWT_doesNotWarnWhenEmailIsPresent(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	tokenStr := signTestToken(t, key, jwt.MapClaims{
		"sub":   "user-1",
		"email": "a@b.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	log, hook := test.NewNullLogger()
	mw := JWT(func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, WithLogger(log))
	h := mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	h.ServeHTTP(httptest.NewRecorder(), req)

	if n := len(hook.AllEntries()); n != 0 {
		t.Fatalf("log entries = %d, want 0 for a healthy token", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/packages/shared-go/auth -v
```

Expected: compile failure — `undefined: WithLogger`.

- [ ] **Step 3: Rewrite `middleware.go` lines 1-38**

Replace everything from the `package auth` line through the closing brace of `jwtWithKeyfunc` with:

```go
package auth

import (
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
	"github.com/jtumidanski/myfleet/packages/shared-go/telemetry"
)

// Option configures the JWT middleware. Options are variadic so every existing
// single-argument JWT(keyfn) call site keeps compiling untouched; services with
// no Identity.Email consumer are deliberately left on that form.
type Option func(*jwtConfig)

type jwtConfig struct{ log logrus.FieldLogger }

// WithLogger attaches a logger so the middleware can report an identity claim
// that validated but arrived empty. With no logger the middleware logs nothing
// and behaves exactly as it did before options existed.
func WithLogger(l logrus.FieldLogger) Option { return func(c *jwtConfig) { c.log = l } }

// JWT validates RS256 tokens via JWKS and puts an Identity on context (design §9).
func JWT(keyfn jwt.Keyfunc, opts ...Option) func(http.Handler) http.Handler {
	var cfg jwtConfig
	for _, o := range opts {
		o(&cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" || raw == r.Header.Get("Authorization") {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			claims := jwt.MapClaims{}
			tok, err := jwt.ParseWithClaims(raw, claims, keyfn, jwt.WithValidMethods([]string{"RS256"}))
			if err != nil || !tok.Valid {
				server.WriteError(w, server.ErrUnauthorized)
				return
			}
			id := Identity{
				UserID:        str(claims["sub"]),
				Email:         str(claims["email"]),
				ActiveFleetID: str(claims["active_fleet_id"]),
				Role:          str(claims["role"]),
			}
			// A token that validates but carries no email is the signature of a
			// minting path that built a partial principal — the defect this task
			// fixed, which previously surfaced only as an unexplained 409 on
			// invite acceptance. Log it and proceed: observability, not
			// enforcement. Never the raw token, never an email address.
			if cfg.log != nil && id.Email == "" {
				cfg.log.WithFields(logrus.Fields{
					"sub":            id.UserID,
					"correlation_id": telemetry.CorrelationIDFromContext(r.Context()),
				}).Warn("access token missing email claim")
			}
			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}
```

`RequireRole` and `str` below it are unchanged. `jwtWithKeyfunc` is gone — it added no behaviour and existed only because the tests called it.

- [ ] **Step 4: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/packages/shared-go/... -race -v -run JWT
```

Expected: PASS for `TestJWT_rejectsMissingToken`, `TestJWT_parsesIdentityFromValidToken`, `TestJWT_warnsOnEmptyEmailClaimAndStillCallsNext`, `TestJWT_doesNotWarnWhenEmailIsPresent`.

- [ ] **Step 5: Verify the four existing call sites still compile untouched**

```sh
go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
```

Expected: no output, exit 0. This is the FR-12 check — `auth-service:93`, `fleet-service:183`, `media-service:137`, and `notification-service:73` all still pass a single argument at this point.

- [ ] **Step 6: Commit**

```sh
git add packages/shared-go/auth/middleware.go packages/shared-go/auth/middleware_test.go
git commit -m "feat(shared-go): warn when a validated token carries an empty email claim"
```

---

### Task 3: Invite acceptance diagnostics in `fleet-service`

Makes the *symptom* diagnosable. Today all three accept preconditions return a bare `409` with `"title": "conflict"` and nothing else, so a user told "conflict" cannot tell "already accepted" from "wrong account", and neither can an operator reading the logs.

**Files:**
- Modify: `apps/fleet-service/internal/invite/processor.go:46-59`
- Modify: `apps/fleet-service/internal/invite/resource.go:163-166`
- Modify: `apps/fleet-service/cmd/main.go:183`
- Test: `apps/fleet-service/internal/invite/processor_test.go` (append)
- Test: `apps/fleet-service/internal/invite/resource_test.go` (**new**)

**Interfaces:**
- Consumes: `server.Detailed(base error, detail string) error` from Task 1; `authmw.WithLogger(logrus.FieldLogger) authmw.Option` from Task 2.
- Produces: package-level `invite.ErrAlreadyAccepted`, `invite.ErrInviteExpired`, `invite.ErrEmailMismatch` (all `error`). `Processor.ValidateAccept(inv Model, authedEmail string) error` keeps its signature and returns one of those three or `nil`.

- [ ] **Step 1: Write the failing processor tests**

Append to `apps/fleet-service/internal/invite/processor_test.go`. **Do not modify the four existing tests** — their `errors.Is(err, server.ErrConflict)` assertions are the regression guard proving FR-8 (the status contract) was not violated.

```go
// The existing four tests above assert only errors.Is(err, server.ErrConflict)
// and must keep passing unmodified: they are the guard that this change did not
// alter the HTTP status contract. These tighten each case to its own sentinel.

func TestValidateAccept_returnsDistinctSentinelPerPrecondition(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()

	cases := []struct {
		name string
		inv  Model
		as   string
		want error
	}{
		{"already accepted", mk("a@b.com", now.Add(time.Hour), &now), "a@b.com", ErrAlreadyAccepted},
		{"expired", mk("a@b.com", now.Add(-time.Hour), nil), "a@b.com", ErrInviteExpired},
		{"email mismatch", mk("invited@b.com", now.Add(time.Hour), nil), "other@b.com", ErrEmailMismatch},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := p.ValidateAccept(c.inv, c.as)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
			if !errors.Is(err, server.ErrConflict) {
				t.Fatal("every sentinel must still map to 409 (FR-8)")
			}
		})
	}
}

// TestValidateAccept_sentinelsAreMutuallyExclusive proves the three sentinels
// are actually distinguishable. Without it, three errors that all satisfy
// errors.Is(err, server.ErrConflict) could still be the same value and the
// per-case test above would pass vacuously.
func TestValidateAccept_sentinelsAreMutuallyExclusive(t *testing.T) {
	all := []error{ErrAlreadyAccepted, ErrInviteExpired, ErrEmailMismatch}
	for i, a := range all {
		for j, b := range all {
			if i != j && errors.Is(a, b) {
				t.Fatalf("sentinel %d and %d are not distinguishable", i, j)
			}
		}
	}
}

// TestValidateAccept_reportsAlreadyAcceptedBeforeEmailMismatch pins the
// precondition order. It matters for disclosure: a wrong-account caller holding
// a leaked, already-accepted invite learns only "already accepted", never
// "...and it wasn't yours".
func TestValidateAccept_reportsAlreadyAcceptedBeforeEmailMismatch(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()
	inv := mk("invited@b.com", now.Add(time.Hour), &now)

	if err := p.ValidateAccept(inv, "other@b.com"); !errors.Is(err, ErrAlreadyAccepted) {
		t.Fatalf("err = %v, want ErrAlreadyAccepted (order: accepted → expired → email)", err)
	}
}
```

- [ ] **Step 2: Run the processor tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite -run ValidateAccept -v
```

Expected: compile failure — `undefined: ErrAlreadyAccepted`.

- [ ] **Step 3: Add the sentinels and return them from `ValidateAccept`**

Replace `apps/fleet-service/internal/invite/processor.go` lines 46-59 with:

```go
// The three preconditions ValidateAccept enforces, each carrying its own
// JSON:API detail so a 409 tells the caller which one failed. All three wrap
// server.ErrConflict, so StatusFor still renders 409 (FR-8) and any existing
// errors.Is(err, server.ErrConflict) check keeps working.
//
// ErrEmailMismatch's detail deliberately names NEITHER address, and is a
// constant with no format verb so no code path can interpolate one. The invite
// token is a bearer credential — anyone holding the link reaches this endpoint
// — so echoing the invited address would turn a leaked link into a
// who-was-this-for oracle (PRD FR-10).
var (
	ErrAlreadyAccepted = server.Detailed(server.ErrConflict, "invite has already been accepted")
	ErrInviteExpired   = server.Detailed(server.ErrConflict, "invite has expired")
	ErrEmailMismatch   = server.Detailed(server.ErrConflict, "invite was issued to a different account")
)

// ValidateAccept enforces FR-FLEET-3: invite must be for the same email, not
// yet accepted, and not expired. Each violation returns its own sentinel; all
// three render 409.
//
// Order is load-bearing (accepted → expired → email): a wrong-account caller
// presenting an already-accepted invite learns only "already accepted".
func (pr *Processor) ValidateAccept(inv Model, authedEmail string) error {
	if inv.AcceptedAt() != nil {
		return ErrAlreadyAccepted
	}
	if !inv.ExpiresAt().After(time.Now()) {
		return ErrInviteExpired
	}
	if !strings.EqualFold(inv.Email(), authedEmail) {
		return ErrEmailMismatch
	}
	return nil
}
```

- [ ] **Step 4: Run the processor tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite -race -v
```

Expected: PASS — the four pre-existing tests and the three new ones.

- [ ] **Step 5: Write the failing route test**

Create `apps/fleet-service/internal/invite/resource_test.go`:

```go
package invite

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
)

// stubOwnerChecker satisfies OwnerChecker. The accept route never consults it —
// the invite token is what authorizes acceptance — but InitializeRoutes wires
// it for the create/delete routes.
type stubOwnerChecker struct{}

func (stubOwnerChecker) RequireOwnerInFleet(string, string) error { return nil }

func newInviteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (fleet.fleet_invites) for Postgres. SQLite
	// has no schemas, so attach an in-memory database aliased "fleet" so the
	// qualified name resolves.
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	if err := Migration(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// newAcceptRouter builds the real chi router over a seeded database. The
// activity recorder and outbox emitter are nil: every case here fails
// validation before Administrator.Accept runs, so neither is reached.
func newAcceptRouter(t *testing.T, inv Model) chi.Router {
	t.Helper()
	db := newInviteTestDB(t)
	if _, err := NewAdministrator(db).Insert(inv); err != nil {
		t.Fatalf("seed invite: %v", err)
	}

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, stubOwnerChecker{}, nil, nil))
	return r
}

// postAccept drives POST /invites/{token}/accept with a validated Identity on
// context, standing in for the JWT middleware the real router mounts upstream.
func postAccept(r chi.Router, token, authedEmail string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/invites/"+token+"/accept", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        "user-1",
		Email:         authedEmail,
		ActiveFleetID: "fleet-1",
		Role:          "member",
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeDetail(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Errors []struct {
			Status string `json:"status"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(body.Errors))
	}
	if body.Errors[0].Status != "409" {
		t.Fatalf("status field = %q, want 409", body.Errors[0].Status)
	}
	if body.Errors[0].Title != "conflict" {
		t.Fatalf("title = %q, want conflict — the status contract must not change", body.Errors[0].Title)
	}
	return body.Errors[0].Detail
}

// seedInvite uses the builder's unexported setAcceptedAt, which exists for
// exactly this purpose: production code stamps accepted_at inside
// Administrator.Accept's transaction, never by hand.
func seedInvite(email string, expires time.Time, accepted *time.Time) Model {
	return NewBuilder().
		SetFleetID("fleet-1").
		SetEmail(email).
		SetRole("member").
		SetToken("tok-" + email).
		SetExpiresAt(expires).
		SetInvitedByUserID("owner-1").
		setAcceptedAt(accepted).
		Build()
}

// TestAcceptRoute_rendersADistinctDetailPerPrecondition is the user-facing half
// of this task: before it, all three of these returned a body a caller could
// not tell apart, so an invite rejected because the session had refreshed
// looked exactly like one already accepted.
func TestAcceptRoute_rendersADistinctDetailPerPrecondition(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		inv        Model
		as         string
		wantDetail string
	}{
		{"already accepted", seedInvite("a@b.com", now.Add(time.Hour), &now), "a@b.com", "invite has already been accepted"},
		{"expired", seedInvite("a@b.com", now.Add(-time.Hour), nil), "a@b.com", "invite has expired"},
		{"email mismatch", seedInvite("invited@b.com", now.Add(time.Hour), nil), "other@b.com", "invite was issued to a different account"},
	}
	seen := map[string]bool{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newAcceptRouter(t, c.inv)
			rec := postAccept(r, c.inv.Token(), c.as)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409 (the status contract does not change)", rec.Code)
			}
			got := decodeDetail(t, rec)
			if got != c.wantDetail {
				t.Fatalf("detail = %q, want %q", got, c.wantDetail)
			}
			if seen[got] {
				t.Fatalf("detail %q is not unique across preconditions", got)
			}
			seen[got] = true
		})
	}
}

// TestAcceptRoute_mismatchDetailDisclosesNeitherAddress enforces FR-10. The
// invite token is the only credential needed to reach this endpoint, so naming
// the invited address here would turn a leaked invite link into an
// address-disclosure oracle.
func TestAcceptRoute_mismatchDetailDisclosesNeitherAddress(t *testing.T) {
	inv := seedInvite("invited@example.com", time.Now().Add(time.Hour), nil)
	r := newAcceptRouter(t, inv)
	rec := postAccept(r, inv.Token(), "attacker@example.com")

	body := rec.Body.String()
	if strings.Contains(body, "invited@example.com") {
		t.Fatalf("409 body discloses the invited address: %s", body)
	}
	if strings.Contains(body, "attacker@example.com") {
		t.Fatalf("409 body echoes the authenticated address: %s", body)
	}
}
```

- [ ] **Step 6: Run the route test and confirm it exercises the route**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite -run AcceptRoute -v
```

Expected: PASS — Step 3 already made the detail assertions true. Confirm the test is live rather than vacuous: every subtest must report 409 with its own detail. `status = 404` means the seeded token is not being found — check the `ATTACH DATABASE` alias and `Migration`. `status = 500` means `Administrator.Accept` was reached with nil recorder/emitter, which would mean validation did not reject as expected.

- [ ] **Step 7: Add the mismatch warn log to the accept handler**

Replace `apps/fleet-service/internal/invite/resource.go` lines 163-166 with:

```go
			if err := proc.ValidateAccept(inv, identity.Email); err != nil {
				// Only the mismatch is logged. Already-accepted and expired are
				// ordinary user outcomes the response body now explains;
				// logging them adds noise. A mismatch is either a genuine
				// wrong-account attempt or a regression of the empty-email-claim
				// defect — worth being greppable. Invite id and correlation id
				// only: never inv.Email(), never identity.Email (PRD FR-10/§8).
				// The invite id joins to the row for an operator who already has
				// database access, so the line itself discloses nothing.
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

`errors`, `logrus`, `telemetry`, and `server` are all already imported by this file. `telemetry.CorrelationIDFromContext` is safe here: `fleet-service` mounts `telemetry.CorrelationID` as a **root** middleware (`cmd/main.go:176`) and `authmw.JWT` as a **route-group** middleware (`:183`); chi runs root middlewares first, so the correlation id is on the context by the time this handler runs.

- [ ] **Step 8: Wire the logger into fleet-service's JWT middleware**

`apps/fleet-service/cmd/main.go:183` — change:

```go
				pr.Use(authmw.JWT(keyfn, authmw.WithLogger(log)))
```

- [ ] **Step 9: Run the full fleet-service suite**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/... -race
go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
```

Expected: PASS, no build or vet output.

- [ ] **Step 10: Commit**

```sh
git add apps/fleet-service/internal/invite/processor.go apps/fleet-service/internal/invite/processor_test.go apps/fleet-service/internal/invite/resource.go apps/fleet-service/internal/invite/resource_test.go apps/fleet-service/cmd/main.go
git commit -m "feat(fleet-service): distinguish the three invite-accept 409 causes"
```

---

### Task 4: Single `Principal` construction site in `auth-service`

The fix itself. `Principal` is an exported four-field struct that either minting call site may fill in partially, and the refresh path does — omitting `Email`, so every refreshed access token carries `"email": ""`. This task removes the affordance rather than patching the one call site.

This is **one atomic commit**: retyping `MembershipResolver` → `PrincipalResolver` breaks `session`, `oidc`, and `cmd/main.go` at once, so the tree does not compile part-way through. Write all three edits, then run the tests.

**Files:**
- Modify: `apps/auth-service/internal/session/resource.go:15-18, 28, 35, 55-67`
- Modify: `apps/auth-service/internal/oidc/resource.go:33, 108-131, 139-143`
- Modify: `apps/auth-service/cmd/main.go:45-55, 65, 93`
- Test: `apps/auth-service/cmd/main_test.go` (**new**)
- Test: `apps/auth-service/internal/session/resource_test.go` (**new**)

**Interfaces:**
- Consumes: `authmw.WithLogger(logrus.FieldLogger) authmw.Option` from Task 2.
- Produces: `type session.PrincipalResolver func(ctx context.Context, userID string) (session.Principal, error)`, replacing `session.MembershipResolver` (**deleted**). `session.InitializePublicRoutes(log logrus.FieldLogger, proc *Processor, resolve PrincipalResolver, cookieSecure bool) func(chi.Router)` — same shape, third parameter retyped. `oidc.Dependencies.Resolve` is now `session.PrincipalResolver`. New in `package main`: `func newPrincipalResolver(users user.Provider, fleet *membership.Client) session.PrincipalResolver`. Task 5's arch test asserts no `Principal{…}` literal survives in either resource file.

- [ ] **Step 1: Write the failing resolver tests**

Create `apps/auth-service/cmd/main_test.go`:

```go
package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/membership"
	"github.com/jtumidanski/myfleet/apps/auth-service/internal/user"
)

// fakeUsers is a user.Provider over a map. GetBySub returns a loud error rather
// than data: the resolver must look users up by our internal id, and calling
// GetBySub with a JWT `sub` was the FIRST identity-propagation defect in this
// flow — it silently returned ErrNotFound and logged every user back out.
type fakeUsers struct {
	byID map[string]user.Model
	err  error
}

func (f *fakeUsers) GetByID(id string) (user.Model, error) {
	if f.err != nil {
		return user.Model{}, f.err
	}
	if m, ok := f.byID[id]; ok {
		return m, nil
	}
	return user.Model{}, user.ErrNotFound
}

func (f *fakeUsers) GetBySub(string) (user.Model, error) {
	return user.Model{}, errors.New("resolver must look users up by internal id, not google_sub")
}

func usersWith(id, email string) *fakeUsers {
	return &fakeUsers{byID: map[string]user.Model{
		id: user.NewBuilder().SetEmail(email).Build(),
	}}
}

// fleetServing stands up a fake fleet-service and returns a membership.Client
// pointed at it, so the real HTTP client — including its 404 handling — is
// under test rather than a stand-in for it.
func fleetServing(t *testing.T, status int, body string) *membership.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return membership.NewClient(srv.URL)
}

// TestNewPrincipalResolver_fillsEveryField is the core guarantee: this closure
// is the only place a session.Principal is built, so if it fills every field,
// every token this service mints on either path carries a complete claim set.
func TestNewPrincipalResolver_fillsEveryField(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
	)

	p, err := resolve(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if p.UserID != "user-1" {
		t.Fatalf("UserID = %q, want user-1", p.UserID)
	}
	if p.Email != "a@b.com" {
		t.Fatalf("Email = %q, want a@b.com — this is the field the refresh path used to drop", p.Email)
	}
	if p.ActiveFleetID != "fleet-9" {
		t.Fatalf("ActiveFleetID = %q, want fleet-9", p.ActiveFleetID)
	}
	if p.Role != "owner" {
		t.Fatalf("Role = %q, want owner", p.Role)
	}
}

// TestNewPrincipalResolver_failsClosedWhenTheUserIsGone covers FR-5: a refresh
// token can outlive its user row. Returning an error here is what makes the
// handler answer 401 instead of minting a token with absent identity.
func TestNewPrincipalResolver_failsClosedWhenTheUserIsGone(t *testing.T) {
	resolve := newPrincipalResolver(
		&fakeUsers{byID: map[string]user.Model{}},
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
	)

	if _, err := resolve(context.Background(), "ghost"); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err = %v, want user.ErrNotFound", err)
	}
}

func TestNewPrincipalResolver_failsClosedWhenTheMembershipCallFails(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusOK, `not json`),
	)

	if _, err := resolve(context.Background(), "user-1"); err == nil {
		t.Fatal("a membership decode failure must fail closed, not mint a partial principal")
	}
}

// TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError is load-bearing
// and easy to break. membership.Client maps a 404 to a zero Membership with no
// error, and the OIDC callback keys its onboarding redirect off an empty
// ActiveFleetID. Turning "no membership" into a resolver error would break
// signup on a brand-new user's first login.
func TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusNotFound, ``),
	)

	p, err := resolve(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("a user with no fleet must resolve cleanly, got %v", err)
	}
	if p.ActiveFleetID != "" || p.Role != "" {
		t.Fatalf("fleet/role = %q/%q, want empty so the callback redirects to onboarding", p.ActiveFleetID, p.Role)
	}
	if p.Email != "a@b.com" {
		t.Fatalf("Email = %q, want a@b.com even with no membership", p.Email)
	}
}
```

- [ ] **Step 2: Write the failing refresh-handler tests**

Create `apps/auth-service/internal/session/resource_test.go`. It reuses `fakeStore`, `newFakeStore`, and `newTestProcessor` from `processor_test.go` — same package, no new fake infrastructure.

```go
package session

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

// newRefreshRouter mounts the real public routes over a store seeded with one
// valid refresh token, and returns the router plus that token's raw value.
func newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, string) {
	t.Helper()
	store := newFakeStore()
	proc := newTestProcessor(store)

	raw := "valid-refresh-token"
	store.seed(NewBuilder().
		SetUserID("user-1").
		SetTokenHash(HashRefresh(raw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build())

	log := logrus.New()
	log.SetOutput(io.Discard)

	r := chi.NewRouter()
	r.Group(InitializePublicRoutes(log, proc, resolve, false))
	return r, proc, raw
}

func postRefresh(r chi.Router, raw string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: RefreshCookieName, Value: raw})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func refreshCookieCleared(rec *httptest.ResponseRecorder) bool {
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName && c.Value == "" && c.MaxAge < 0 {
			return true
		}
	}
	return false
}

// TestRefresh_mintsAccessTokenCarryingEmailClaim is the direct, user-visible
// proof of the fix. Before it, this route minted `"email": ""` on every call,
// and since the SPA refreshes on any 401 and access tokens live 15 minutes, all
// but the first few minutes of every session ran on an email-less token.
func TestRefresh_mintsAccessTokenCarryingEmailClaim(t *testing.T) {
	resolve := func(context.Context, string) (Principal, error) {
		return Principal{UserID: "user-1", Email: "a@b.com", ActiveFleetID: "fleet-9", Role: "owner"}, nil
	}
	r, proc, raw := newRefreshRouter(t, resolve)

	rec := postRefresh(r, raw)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data struct {
			Attributes struct {
				AccessToken  string `json:"accessToken"`
				RefreshToken string `json:"refreshToken"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	claims := jwt.MapClaims{}
	if _, err := jwt.ParseWithClaims(body.Data.Attributes.AccessToken, claims, func(*jwt.Token) (any, error) {
		return proc.ks.Private().Public(), nil
	}); err != nil {
		t.Fatalf("parse minted access token: %v", err)
	}
	for claim, want := range map[string]string{
		"sub": "user-1", "email": "a@b.com", "active_fleet_id": "fleet-9", "role": "owner",
	} {
		if claims[claim] != want {
			t.Fatalf("claim %s = %v, want %q", claim, claims[claim], want)
		}
	}
	if body.Data.Attributes.RefreshToken == raw {
		t.Fatal("refresh token must rotate")
	}
}

// TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors covers FR-5. The
// cookie clearing is a deliberate behaviour change: today's resolver-error path
// returns 401 WITHOUT clearing, unlike every other 401 in this handler, so a
// session whose user row is gone keeps re-presenting a credential that will 401
// forever.
func TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors(t *testing.T) {
	resolve := func(context.Context, string) (Principal, error) {
		return Principal{}, errors.New("user row is gone")
	}
	r, _, raw := newRefreshRouter(t, resolve)

	rec := postRefresh(r, raw)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("refresh cookie must be cleared on resolver error; cookies = %v", rec.Result().Cookies())
	}

	// No token of any kind may be issued: an incomplete identity must never be
	// minted, and no new refresh cookie may replace the cleared one.
	var body struct {
		Data any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data != nil {
		t.Fatalf("response carried a data member: %v", body.Data)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName && c.Value != "" {
			t.Fatalf("a refresh cookie was still set: %q", c.Value)
		}
	}
}
```

- [ ] **Step 3: Run both new test files to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/auth-service/... -v 2>&1 | head -30
```

Expected: compile failures — `undefined: newPrincipalResolver` in `package main`, and `cannot use resolve ... as MembershipResolver` in `session`.

- [ ] **Step 4: Retype the resolver in `session/resource.go`**

Replace lines 15-18:

```go
// PrincipalResolver resolves the COMPLETE session.Principal for a user — email
// from the local users table, active fleet and role from fleet-service.
//
// It is injected (Decision 1) so neither session nor oidc imports the concrete
// membership client, and it is the single construction site for Principal: a
// call site that assembles its own can omit a field and mint a token missing a
// claim, which is exactly what the refresh path did with Email.
type PrincipalResolver func(ctx context.Context, userID string) (Principal, error)
```

Change the two signatures (lines 28 and 35) from `resolve MembershipResolver` to `resolve PrincipalResolver`.

Replace lines 55-67 with:

```go
		principal, err := resolve(req.Context(), userID)
		if err != nil {
			// Fail closed (FR-5): a token with incomplete identity is never
			// minted. Clear the cookie too — unlike this path's previous bare
			// 401 — so a session whose user row is gone stops re-presenting a
			// credential that can only 401.
			log.WithError(err).Error("resolve principal on refresh")
			clearRefreshCookie(w, cookieSecure)
			server.WriteError(w, server.ErrUnauthorized)
			return
		}

		access, err := proc.MintAccess(principal)
		if err != nil {
			log.WithError(err).Error("mint access on refresh")
			server.WriteError(w, server.ErrUnauthorized)
			return
		}
```

- [ ] **Step 5: Retype and simplify the OIDC callback**

`apps/auth-service/internal/oidc/resource.go` line 33:

```go
	Resolve     session.PrincipalResolver
```

Update the `Dependencies` doc comment above it (lines 26-28):

```go
// Dependencies bundles everything the callback orchestration needs. The
// principal resolver is injected (Decision 1) so this package never imports the
// concrete membership client, and so this handler never constructs a Principal
// of its own.
```

Replace lines 108-131 with:

```go
		// The resolver reads the row ProvisionFromGoogle just wrote, by primary
		// key. Safe: there is a single primary Postgres and no read replica in
		// deploy/k8s, and the write has committed by the time ProvisionFromGoogle
		// returns. This is the first place a read replica would break.
		principal, err := d.Resolve(ctx, u.ID())
		if err != nil {
			log.WithError(err).Error("resolve principal on callback")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}

		access, err := d.Sessions.MintAccess(principal)
		if err != nil {
			log.WithError(err).Error("mint access on callback")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}
		refresh, err := d.Sessions.IssueRefresh(principal.UserID)
		if err != nil {
			log.WithError(err).Error("issue refresh on callback")
			http.Error(w, "authentication failed", http.StatusInternalServerError)
			return
		}
```

Replace lines 139-143 with:

```go
		// New users without a fleet go to onboarding; everyone else lands home.
		// membership.Client maps fleet-service's 404 to an empty fleet id, so
		// this is how a brand-new user is recognised.
		dest := d.AppBaseURL + d.HomePath
		if principal.ActiveFleetID == "" {
			dest = d.AppBaseURL + d.OnboardingPath
		}
```

`u` is still needed — `ProvisionFromGoogle` must create-or-update the row before the resolver reads it — but only `u.ID()` is consumed now; `u.Email()` is gone.

- [ ] **Step 6: Compose the resolver in `cmd/main.go`**

Replace lines 45-55 with:

```go
	// Decision 1: build the concrete membership client and compose it with the
	// local user provider into the PrincipalResolver function value injected
	// into session/oidc, so neither package imports the concrete client (no
	// import cycle).
	fleetClient := membership.NewClient(config.Get("FLEET_SERVICE_URL", "http://fleet-service:8080"))
	userProv := user.NewProvider(db)
	resolve := newPrincipalResolver(userProv, fleetClient)
```

Change line 65 to reuse that provider rather than construct a second one:

```go
	users := user.NewProcessor(log, userProv, user.NewAdministrator(db))
```

Change line 93 to wire the logger:

```go
				pr.Use(authmw.JWT(ks.Keyfunc(), authmw.WithLogger(log)))
```

Add the resolver constructor at the bottom of the file, above `loadKeySet`:

```go
// newPrincipalResolver composes the two sources of identity — the local users
// table for email, fleet-service for the active membership — into the single
// construction site for session.Principal. Every access token this service
// mints, on either path, is built here (FR-1, FR-2).
//
// It is a package-level function rather than an inline closure in main() so it
// can be unit-tested: it is now the sole guarantor that a minted token carries
// a complete claim set.
//
// The local read comes first. A missing user is the permanent, cheap-to-detect
// failure, so short-circuiting on it avoids an HTTP round trip to fleet-service
// for a request that cannot succeed.
//
// GetByID, NOT GetBySub: the JWT `sub` claim and Processor.Rotate both carry our
// internal user id, while Google's subject is a different identifier. Confusing
// the two is the mistake this service has already made once — see the doc
// comment on user.Provider.
func newPrincipalResolver(users user.Provider, fleet *membership.Client) session.PrincipalResolver {
	return func(ctx context.Context, userID string) (session.Principal, error) {
		u, err := users.GetByID(userID)
		if err != nil {
			return session.Principal{}, err
		}
		// A 404 from fleet-service is NOT an error here: membership.Client maps
		// it to a zero Membership, and the OIDC callback keys its onboarding
		// redirect off an empty ActiveFleetID. Turning it into an error would
		// break a new user's first login.
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

The `context` import is already present and is still used by this function.

- [ ] **Step 7: Run the auth-service suite**

```sh
go test github.com/jtumidanski/myfleet/apps/auth-service/... -race -v
```

Expected: PASS for the four `TestNewPrincipalResolver_*` tests, both `TestRefresh_*` tests, and every pre-existing test in `session`, `oidc`, and `user`.

- [ ] **Step 8: Verify `MembershipResolver` is gone from the codebase**

```sh
grep -rn "MembershipResolver" apps packages docs/tasks/task-008-refresh-token-email-claim/plan.md
```

Expected: no hits outside this plan file. If `apps/` or `packages/` matches, a reference was missed.

- [ ] **Step 9: Verify no `Principal` literal survives in either resource file**

```sh
grep -n "Principal{" apps/auth-service/internal/session/resource.go apps/auth-service/internal/oidc/resource.go
```

Expected: no output. (Task 5 automates this check.)

- [ ] **Step 10: Build and vet the whole workspace**

```sh
go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
```

Expected: no output, exit 0.

- [ ] **Step 11: Commit**

```sh
git add apps/auth-service/internal/session/resource.go apps/auth-service/internal/session/resource_test.go apps/auth-service/internal/oidc/resource.go apps/auth-service/cmd/main.go apps/auth-service/cmd/main_test.go
git commit -m "fix(auth-service): mint refreshed access tokens with the email claim"
```

---

### Task 5: Structural parity guards

Task 4 removes today's bug. These two tests catch the two ways it can come back: a new `Principal` field that never reaches a claim, and a new call site that builds its own `Principal`. Neither is caught by any test that names a field, because a test that enumerates fields is a test somebody has to remember to update.

**Files:**
- Modify: `apps/auth-service/internal/session/processor_test.go` (append)
- Test: `apps/auth-service/internal/arch/arch_test.go` (**new, test-only package**)

**Interfaces:**
- Consumes: `session.Principal` and `session.Processor.MintAccess` (unchanged by this task); the post-Task-4 state of `session/resource.go` and `oidc/resource.go`.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the reflection test**

Append to `apps/auth-service/internal/session/processor_test.go`, adding `"reflect"` to its imports:

```go
// TestMintAccess_mapsEveryPrincipalField fails when a field is added to
// Principal but not wired into MintAccess's claim map. It deliberately names no
// field: TestMintAccess_setsRequiredClaims above enumerates the four we have
// today, and an enumerating test is one somebody has to remember to update.
//
// The scenario: someone adds Principal.TenantID, wires it in
// newPrincipalResolver, and forgets MintAccess. Every other test stays green
// and the claim is silently absent — the same shape as the defect this task
// fixed.
func TestMintAccess_mapsEveryPrincipalField(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := jwks.NewKeySet(priv, "kid-1")
	p := NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet")

	v := reflect.New(reflect.TypeOf(Principal{})).Elem()
	want := map[string]string{}
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.Type.Kind() != reflect.String {
			// Not a skip: a non-string field means this test's sentinel scheme
			// no longer holds, and it must say so rather than quietly cover less.
			t.Fatalf("Principal.%s is a %s, not a string — extend this test's sentinel scheme", f.Name, f.Type.Kind())
		}
		sentinel := "sentinel-" + f.Name
		v.Field(i).SetString(sentinel)
		want[f.Name] = sentinel
	}

	tokenStr, err := p.MintAccess(v.Interface().(Principal))
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	if _, perr := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
		return &priv.PublicKey, nil
	}); perr != nil {
		t.Fatal(perr)
	}

	got := map[string]bool{}
	for _, cv := range claims {
		if s, ok := cv.(string); ok {
			got[s] = true
		}
	}
	for field, sentinel := range want {
		if !got[sentinel] {
			t.Errorf("Principal.%s never reaches a claim — MintAccess drops it. "+
				"Every Principal field must appear in MintAccess's claim map.", field)
		}
	}
}
```

- [ ] **Step 2: Run it and confirm it passes against the current four fields**

```sh
go test github.com/jtumidanski/myfleet/apps/auth-service/internal/session -run MapsEveryPrincipalField -v
```

Expected: PASS.

- [ ] **Step 3: Prove the reflection test actually bites**

Temporarily delete the `"email"` line from `MintAccess`'s claim map in `apps/auth-service/internal/session/processor.go:63`, then:

```sh
go test github.com/jtumidanski/myfleet/apps/auth-service/internal/session -run MapsEveryPrincipalField -v
```

Expected: FAIL with `Principal.Email never reaches a claim`. **Restore the line immediately** and re-run to confirm PASS. A guard that cannot fail is not a guard.

- [ ] **Step 4: Write the architecture test**

Create `apps/auth-service/internal/arch/arch_test.go`:

```go
// Package arch holds architecture tests. It deliberately contains no production
// code: the invariants here span packages, and putting them in session or oidc
// would mean one package reaching into the other's source directory. `go build
// ./...` and `go vet ./...` both skip a directory with no non-test Go files, so
// a test-only package costs nothing in the build.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestNoPrincipalLiteralOutsideResolver enforces the acceptance criterion that
// session.Principal is constructed in exactly one place —
// newPrincipalResolver in cmd/main.go — so the two token-minting paths cannot
// diverge on a claim.
//
// A composite literal in either resource file means a call site has started
// building its own principal again, which is precisely the defect this task
// fixed: the refresh path built Principal{UserID, ActiveFleetID, Role}, Go
// zero-valued the omitted Email, and every refreshed token carried
// `"email": ""` with nothing anywhere failing loudly.
//
// This parses the files rather than grepping them: `Principal{` appears in
// comments and would produce a false failure the first time someone documents
// the type.
func TestNoPrincipalLiteralOutsideResolver(t *testing.T) {
	files := []string{
		"../session/resource.go",
		"../oidc/resource.go",
	}
	for _, path := range files {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			// A rename or move must fail this test, not silently skip the file.
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if name, ok := literalTypeName(lit.Type); ok && name == "Principal" {
				t.Errorf("%s:%d: session.Principal literal outside newPrincipalResolver — "+
					"obtain the principal from the injected PrincipalResolver instead, "+
					"so this call site cannot omit a claim",
					path, fset.Position(lit.Pos()).Line)
			}
			return true
		})
	}
}

// literalTypeName returns the bare type name of a composite literal, handling
// both the same-package form `Principal{…}` and the qualified form
// `session.Principal{…}`.
func literalTypeName(e ast.Expr) (string, bool) {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name, true
	case *ast.SelectorExpr:
		return t.Sel.Name, true
	}
	return "", false
}
```

- [ ] **Step 5: Run the architecture test**

```sh
go test github.com/jtumidanski/myfleet/apps/auth-service/internal/arch -v
```

Expected: PASS.

- [ ] **Step 6: Prove the architecture test actually bites**

Temporarily add a throwaway line inside `refreshHandler` in `apps/auth-service/internal/session/resource.go`, just after the `principal, err := resolve(...)` block:

```go
			_ = Principal{UserID: userID}
```

Then:

```sh
go test github.com/jtumidanski/myfleet/apps/auth-service/internal/arch -v
```

Expected: FAIL naming `../session/resource.go` and the line number. **Remove the line immediately** and re-run to confirm PASS.

- [ ] **Step 7: Confirm the test-only package does not break the build**

```sh
go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
```

Expected: no output, exit 0.

- [ ] **Step 8: Commit**

```sh
git add apps/auth-service/internal/session/processor_test.go apps/auth-service/internal/arch/arch_test.go
git commit -m "test(auth-service): guard claim parity by reflection and AST"
```

---

### Task 6: Full verification and end-to-end confirmation

**Files:** none modified. This task produces evidence, not code.

**Interfaces:**
- Consumes: everything from Tasks 1-5.
- Produces: nothing.

- [ ] **Step 1: Run the full CI gate**

Node is not always on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: PASS for `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`. No deployment manifests changed, so no `kustomize` render is required.

- [ ] **Step 2: Confirm the acceptance criteria that are greppable**

```sh
# MembershipResolver is gone from the codebase.
grep -rn "MembershipResolver" apps packages

# Neither resource file constructs a Principal.
grep -n "Principal{" apps/auth-service/internal/session/resource.go apps/auth-service/internal/oidc/resource.go

# media-service and notification-service are still on the single-argument form.
grep -n "authmw.JWT" apps/*/cmd/main.go
```

Expected: the first two produce no output. The third shows `authmw.WithLogger` on `auth-service` and `fleet-service` only.

- [ ] **Step 3: Manual end-to-end check**

This is the one criterion no unit test covers, and it is the user-visible outcome the whole task exists for. On `main` step (e) returns `409`; after this branch it returns `200`.

```sh
make up   # docker compose, deploy/compose/docker-compose.yml
```

Then:

a. Sign in through Google as user A (a fleet owner) and create an invite for user B's address.
b. Sign out; sign in as user B.
c. Force a refresh: wait past the 15-minute access-token TTL, or clear the SPA's in-memory access token so the next API call 401s and `apiClient.ts` refreshes.
d. Confirm the new access token's `email` claim is non-empty — decode the token from the SPA, or check that `auth-service`'s logs contain no `access token missing email claim` warn.
e. Accept the invite. Expect `200` and a membership created.

Also confirm the diagnostic path, which is what makes a recurrence visible:

f. Accept the same invite a second time. Expect `409` with `"detail": "invite has already been accepted"`.
g. As a third user, attempt to accept an invite addressed to someone else. Expect `409` with `"detail": "invite was issued to a different account"`, a response body containing neither address, and a `invite accept rejected: email mismatch` warn in `fleet-service`'s logs carrying `invite_id` and `correlation_id` and no address.

```sh
make down
```

- [ ] **Step 4: Run the code review**

Per `CLAUDE.md`, always run the code-review step before opening a PR:

```
/audit-plan task-008
```

Or invoke `superpowers:requesting-code-review`, which dispatches `plan-adherence-reviewer` and `backend-guidelines-reviewer` (Go files changed; no frontend files changed, so `frontend-guidelines-reviewer` is not needed).

- [ ] **Step 5: Commit any review fixes and open the PR**

```sh
git log --oneline main..HEAD
```

Expected: five commits, one per Task 1-5.

---

## Self-Review

**Spec coverage.** Every functional requirement maps to a task:

| Requirement | Task | Where |
|---|---|---|
| FR-1 `PrincipalResolver` type | 4 | `session/resource.go` Step 4 |
| FR-2 constructed once in `cmd/main.go` | 4 | Step 6 |
| FR-3 both call sites pass it through | 4 | Steps 4, 5 |
| FR-4 `oidc.Dependencies.Resolve` retyped | 4 | Step 5 |
| FR-5 refresh fails closed, clears cookie | 4 | Step 4; test Step 2 |
| FR-6 callback fails closed with 500 | 4 | Step 5 |
| FR-7 three sentinels | 3 | Step 3 |
| FR-8 all three still 409 | 3 | Steps 1, 5 (existing tests unmodified) |
| FR-9 distinguishable detail | 1 + 3 | `server.Detailed`; sentinels |
| FR-10 no address in the detail | 3 | Step 3 comment; test Step 5 |
| FR-11 warn-log the mismatch | 3 | Step 8 |
| FR-12 variadic options, old form compiles | 2 | Steps 3, 5 |
| FR-13 warn on empty email, proceed | 2 | Steps 1, 3 |
| FR-14 no-op without a logger | 2 | `cfg.log != nil` guard; `TestJWT_doesNotWarnWhenEmailIsPresent` and the two migrated tests |
| FR-15 wire logger in auth + fleet only | 3 Step 9, 4 Step 6 | verified in 6 Step 2 |
| Design §2.4 membership 404 → empty, not error | 4 | test Step 1 |
| Design §3 Layer 2 reflection test | 5 | Steps 1-3 |
| Design §3 Layer 3 AST test | 5 | Steps 4-6 |
| Design §4.2 `server.Detailed` properties | 1 | Steps 1, 3, 4 |
| PRD §10 e2e | 6 | Step 3 |

The one gap is deliberate and confirmed: the two-path claim-parity test (see "Deviation" above).

**Not covered, and out of scope by design.** Design §6 resolves PRD Open Question 1 (transient-vs-permanent resolver errors both collapse to `401`, so a fleet-service outage logs users out and now also clears the cookie) as a follow-up task: *classify auth-service upstream failures and return 503 rather than 401 for transient membership resolution errors.* PRD Open Question 2 (middleware ordering) was closed by verification in design §5.4 and re-confirmed against `fleet-service/cmd/main.go:176,183` while writing this plan — no ordering change is required.

Also uncovered: the accept route's **success** path has no route-level test, before or after this change. Reaching `Administrator.Accept` needs the membership entity migrated plus activity-recorder and outbox-emitter stubs, and `Accept` is untouched by this task. Task 6 Step 3(e) covers it manually.

**Placeholder scan.** No "TBD", no "add appropriate error handling", no "similar to Task N". Every code step carries the literal code to write. Every test step carries the command to run and the expected result.

**Type consistency.** `server.Detailed(base error, detail string) error` (Task 1) is called with exactly that signature in Task 3 Step 3. `authmw.WithLogger(logrus.FieldLogger) Option` (Task 2) is called as `authmw.WithLogger(log)` in Task 3 Step 9 and Task 4 Step 6, where `log` is the `logrus.FieldLogger` returned by `telemetry.NewLogger()` in both `main.go` files. `session.PrincipalResolver` (Task 4 Step 4) is the return type of `newPrincipalResolver` (Step 6), the parameter type in `InitializePublicRoutes` (Step 4), and the type of `oidc.Dependencies.Resolve` (Step 5). `user.Provider` has exactly two methods, `GetByID(string) (Model, error)` and `GetBySub(string) (Model, error)`; `fakeUsers` in Task 4 Step 1 implements both. `membership.Client.Active(ctx, userID) (Membership, error)` returns a struct with `FleetID` and `Role`, which is what the resolver reads. `Builder.setAcceptedAt(*time.Time) *Builder` already exists (`invite/builder.go:26`) and is unexported, so Task 3's `seedInvite` can call it from `resource_test.go` in the same package — no production-file change is needed for that seam. `Processor.ValidateAccept(Model, string) error` keeps its exact signature through Task 3, so the four pre-existing tests compile unmodified.
