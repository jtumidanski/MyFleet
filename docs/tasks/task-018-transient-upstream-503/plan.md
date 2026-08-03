# Transient Upstream Failures Must Not Log Users Out — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Task: task-018 · PRD: [`prd.md`](./prd.md) · Design: [`design.md`](./design.md) · Issue: [#15](https://github.com/jtumidanski/MyFleet/issues/15)

**Goal:** Make a `fleet-service` (or local-infrastructure) outage answer `POST /auth/refresh` with `503 Retry-After: 5` while preserving the user's session, instead of `401`-and-clear-cookie, which forces the entire user base back through Google sign-in.

**Architecture:** One fact — *"the answer is unknown because something upstream is unavailable"* — is carried by a single sentinel, `server.ErrServiceUnavailable`, from `membership.Client.Active` (where the status code is visible) through the injected `session.PrincipalResolver` to the two handlers that must act on it. `StatusFor` maps that sentinel to `503`; a `RetryAfter` wrapper error carries the header value the same way `Detailed` already carries a `detail`. On the transient branch, the refresh handler writes the **rotated** refresh cookie before responding `503`, because `Rotate` has already consumed the old token and re-presenting it would trip reuse detection and revoke the family — the logout this task exists to prevent. On the SPA side, `refresh.ts` grows an internal outcome type and *rejects* with `ApiError(503)`; the rejection propagates through the existing `onRefresh` channel, so `packages/shared-ts` needs no change at all.

**Tech Stack:** Go 1.25 (chi, logrus, `net/http`, `httptest`), React 19 + TypeScript + Vitest, Tailwind. No schema changes, no new dependencies, no manifest changes.

## Global Constraints

Every task's requirements implicitly include this section.

- **Never break the `404` contract.** `membership.Client.Active` must keep returning `(Membership{}, nil)` for `404`. `apps/auth-service/cmd/main_test.go`'s `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError` must pass **completely unmodified** — do not touch that test.
- **No error message may carry the user id, the fleet id, or the upstream response body.** Status code plus fixed, our-own text only. This applies to every new wrap in this plan.
- **No existing status mapping, code string, or envelope shape may change.** Every current `WriteError` caller must be byte-identical in behaviour.
- **Retry-After value is `5`** everywhere it appears (header, constant, tests).
- **The login error code string is exactly `service_unavailable`** in Go (`loginErrorCode`), in the redirect fragment, and in TypeScript (`LoginErrorCode`, `CODES`, `NOTICES`).
- **The `503` body stays redacted:** `title` is `InternalErrorTitle` ("internal server error"), no `detail` member. Only `status` and `code` differ from a `500`.
- **One attempt, no retries.** No backoff, no circuit breaker, no client-side auto-retry.
- **Prove every new test can fail.** Before committing a task, revert the implementation edit (or comment out the branch), watch the new test go red, restore it, watch it go green. Assert on observable state — headers, cookies, stored tokens, store rows — never on a status code alone.
- **Test commands.** Go: `go test ./... ` from the module dir. Web: Node is not always on `PATH`; run `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` first.
- **Full gate before the PR:** `make ci` (lint-check, vet, test, build, fe-test, fe-build, manifests, carfax-template), both kustomize overlays rendered, both `kubectl apply --dry-run=server` runs green, and all three reviewer agents.

## File Structure

| File | Change | Responsibility |
| --- | --- | --- |
| `packages/shared-go/server/errors.go` | modify | `ErrServiceUnavailable` sentinel, `StatusFor` → 503, `RetryAfter` wrapper error |
| `packages/shared-go/server/server.go` | modify | `codeFor(503)` → `"service_unavailable"` |
| `packages/shared-go/server/jsonapi.go` | modify | `WriteError` emits `Retry-After` before the header block commits |
| `packages/shared-go/server/errors_test.go` | modify | sentinel/code table rows, `RetryAfter` behaviour, `503` envelope redaction |
| `apps/auth-service/internal/membership/client.go` | modify | `Active`: 5s timeout, transient classification, query escaping; shared timeout name |
| `apps/auth-service/internal/membership/client_test.go` | modify | table-driven classification, transport failure, timeout |
| `apps/auth-service/cmd/main.go` | modify | `newPrincipalResolver`: local infra failures classify transient, `ErrNotFound` stays permanent |
| `apps/auth-service/cmd/main_test.go` | modify | classification survives the resolver; local-infra rows |
| `apps/auth-service/internal/session/resource.go` | modify | `refreshHandler` transient branch: rotated cookie + `503` + `Retry-After` + `Warn` log |
| `apps/auth-service/internal/session/resource_test.go` | modify | status + `Retry-After` + `Set-Cookie` + store state, both branches; log levels |
| `apps/auth-service/internal/oidc/resource.go` | modify | `errServiceUnavailable` login code; split the `d.Resolve` failure branch |
| `apps/auth-service/internal/oidc/resource_test.go` | modify | transient callback redirect + `Warn` log + unchanged cookie behaviour |
| `apps/web/src/lib/api/refresh.ts` | modify | internal `RefreshOutcome`; `refreshAccessToken` throws `ApiError(503)` and does not clear |
| `apps/web/src/lib/api/refresh.test.ts` | modify | 503 vs 401 vs success; dedupe under 503; mint unchanged |
| `packages/shared-ts/src/apiClient.test.ts` | modify | test only — the 401→503 propagation (no source change) |
| `apps/web/src/lib/auth/loginError.ts` | modify | `service_unavailable` union member, `CODES` entry, its own `NOTICES` message |
| `apps/web/src/lib/auth/loginError.test.ts` | modify | parses the new code; its copy is distinct from `GENERIC_FAILURE` |

`packages/shared-ts/src/apiClient.ts`, `packages/shared-ts/src/errors.ts`, `apps/web/src/pages/LoginPage.tsx`, `apps/auth-service/internal/membership`'s `FleetMemberIDs` body, `fleet-service`, and `deploy/k8s` are **not** modified.

---

## Task 1: The `503` sentinel in `shared-go/server`

**Files:**
- Modify: `packages/shared-go/server/errors.go:5-16` (var block), `:18-43` (`StatusFor`)
- Modify: `packages/shared-go/server/server.go:12-37` (`codeFor`)
- Test: `packages/shared-go/server/errors_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.ErrServiceUnavailable` (an `error`), mapped by `StatusFor` to `503` and by `codeFor` to `"service_unavailable"`. Every later Go task depends on this exact name.

- [x] **Step 1: Write the failing tests**

In `packages/shared-go/server/errors_test.go`, add `ErrServiceUnavailable: 503` to the `cases` map in `TestStatusFor_mapsDomainErrors` and `503: "service_unavailable"` to the `cases` map in `TestCodeFor_namesEveryMappedStatus`. Then append this new test at the end of the file:

```go
// TestWriteError_503KeepsTheRedactedEnvelope: 503 is the first status above 429
// this package maps, and it is reached from a PUBLIC, unauthenticated endpoint
// (POST /auth/refresh). Only `status` and `code` may differ from a 500 — the
// reason for the outage is upstream-controlled and is not the caller's
// business, so `title` stays the fixed InternalErrorTitle and no `detail` is
// emitted.
func TestWriteError_503KeepsTheRedactedEnvelope(t *testing.T) {
	transient := fmt.Errorf("%w: active membership lookup failed with status 500", ErrServiceUnavailable)

	rec, got := writeErrorBody(t, transient)

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got.Status != "503" || got.Code != "service_unavailable" {
		t.Fatalf("status/code = %q/%q, want 503/service_unavailable", got.Status, got.Code)
	}
	if got.Title != InternalErrorTitle {
		t.Fatalf("title = %q, want %q — a 5xx title must not describe the fault", got.Title, InternalErrorTitle)
	}
	if got.Detail != "" {
		t.Fatalf("detail = %q, want empty on a 5xx", got.Detail)
	}
	for _, secret := range []string{"membership", "lookup", "500"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("503 body leaked %q: %s", secret, rec.Body.String())
		}
	}
}
```

Note: `"500"` in that redaction list is deliberate — the upstream status must not appear anywhere in the body. `fmt` and `strings` are already imported by this file.

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/packages/shared-go
go test ./server/ -run 'TestStatusFor_mapsDomainErrors|TestCodeFor_namesEveryMappedStatus|TestWriteError_503KeepsTheRedactedEnvelope' -v
```

Expected: FAIL — compile error `undefined: ErrServiceUnavailable`.

- [x] **Step 3: Add the sentinel and both mappings**

In `packages/shared-go/server/errors.go`, add the last line of the var block:

```go
var (
	ErrBadRequest            = errors.New("bad request")              // 400
	ErrUnauthorized          = errors.New("unauthorized")             // 401
	ErrForbidden             = errors.New("forbidden")                // 403
	ErrNotFound              = errors.New("not found")                // 404
	ErrConflict              = errors.New("conflict")                 // 409
	ErrGone                  = errors.New("gone")                     // 410
	ErrRequestEntityTooLarge = errors.New("request entity too large") // 413
	ErrUnsupportedMediaType  = errors.New("unsupported media type")   // 415
	ErrValidation            = errors.New("validation")               // 422
	ErrTooManyRequests       = errors.New("too many requests")        // 429
	// ErrServiceUnavailable means the answer is UNKNOWN because something this
	// request depends on is unavailable — not that the request was wrong. It is
	// the carrier for that distinction across package boundaries: a caller that
	// cannot import the failing client still classifies with
	// errors.Is(err, ErrServiceUnavailable).
	ErrServiceUnavailable = errors.New("service unavailable") // 503
)
```

In the same file, add one case to `StatusFor` immediately before `default`:

```go
	case errors.Is(err, ErrServiceUnavailable):
		return 503
```

In `packages/shared-go/server/server.go`, add one case to `codeFor` immediately before `default`:

```go
	case 503:
		return "service_unavailable"
```

- [x] **Step 4: Run the tests to verify they pass, and the whole package with them**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/packages/shared-go
go test ./... -race
```

Expected: PASS. Every pre-existing test in `errors_test.go` must still pass — they are the regression net for the ~190 existing `WriteError` call sites.

- [x] **Step 5: Prove the new test can fail**

Temporarily change `codeFor`'s new case to `return "internal_error"`, re-run `go test ./server/ -run TestWriteError_503KeepsTheRedactedEnvelope`, confirm it goes red on the code assertion, then restore it.

- [x] **Step 6: Commit**

```bash
git add packages/shared-go/server/errors.go packages/shared-go/server/server.go packages/shared-go/server/errors_test.go
git commit -m "feat(shared-go): add the ErrServiceUnavailable sentinel and its 503 mapping"
```

---

## Task 2: `Retry-After` rides on the error

**Files:**
- Modify: `packages/shared-go/server/errors.go` (append after `detailedError`)
- Modify: `packages/shared-go/server/jsonapi.go:95-116` (`WriteError`)
- Test: `packages/shared-go/server/errors_test.go`

**Interfaces:**
- Consumes: `ErrServiceUnavailable` from Task 1.
- Produces: `server.RetryAfter(base error, seconds int) error`. Task 5 calls it as `server.RetryAfter(err, refreshRetryAfterSeconds)`.

- [x] **Step 1: Write the failing tests**

Append to `packages/shared-go/server/errors_test.go`:

```go
// TestRetryAfter_keepsStatusTitleAndClassification: RetryAfter must be as
// invisible to everything else as Detailed is. It changes one response header
// and nothing about the status, the envelope title, or what errors.Is sees.
func TestRetryAfter_keepsStatusTitleAndClassification(t *testing.T) {
	wrapped := RetryAfter(ErrServiceUnavailable, 5)

	if got := StatusFor(wrapped); got != 503 {
		t.Fatalf("StatusFor = %d, want 503 — StatusFor must follow Unwrap to the base sentinel", got)
	}
	if got := wrapped.Error(); got != ErrServiceUnavailable.Error() {
		t.Fatalf("Error() = %q, want %q — the wrapper must not rewrite the message", got, ErrServiceUnavailable.Error())
	}
	if !errors.Is(wrapped, ErrServiceUnavailable) {
		t.Fatal("errors.Is(RetryAfter(...), ErrServiceUnavailable) must be true")
	}
}

// TestRetryAfter_survivesAnIntermediateWrap is the shape the refresh handler
// actually produces: membership wraps the sentinel with fmt.Errorf, the handler
// wraps THAT with RetryAfter. errors.As has to walk the whole chain for the
// header to survive, and errors.Is has to keep reaching the sentinel for the
// status to survive.
func TestRetryAfter_survivesAnIntermediateWrap(t *testing.T) {
	fromUpstream := fmt.Errorf("%w: active membership lookup failed with status 500", ErrServiceUnavailable)

	rec, got := writeErrorBody(t, RetryAfter(fromUpstream, 5))

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if h := rec.Header().Get("Retry-After"); h != "5" {
		t.Fatalf("Retry-After = %q, want \"5\"", h)
	}
	if got.Title != InternalErrorTitle || got.Detail != "" {
		t.Fatalf("title/detail = %q/%q, want %q/empty", got.Title, got.Detail, InternalErrorTitle)
	}
}

// TestWriteError_setsRetryAfterBeforeCommittingTheHeaderBlock is the whole
// reason the header assignment sits where it does. WriteJSON calls WriteHeader,
// and every header mutation after that is silently discarded — the response
// would still be a 503 and every status-only assertion would still pass. Read
// the header off the recorder AFTER WriteError returns: that is what catches it.
func TestWriteError_setsRetryAfterBeforeCommittingTheHeaderBlock(t *testing.T) {
	rec := httptest.NewRecorder()

	WriteError(rec, RetryAfter(ErrServiceUnavailable, 5))

	if h := rec.Result().Header.Get("Retry-After"); h != "5" {
		t.Fatalf("Retry-After = %q on the committed response, want \"5\" — the header was set after WriteJSON wrote the header block", h)
	}
}

// TestWriteError_omitsRetryAfterWhenThereIsNothingToSay: an absent wrapper is
// the overwhelmingly common case (~190 call sites), and a non-positive value is
// a caller bug — `Retry-After: 0` tells an intermediary to retry immediately,
// which is the opposite of the intent. Both omit the header.
func TestWriteError_omitsRetryAfterWhenThereIsNothingToSay(t *testing.T) {
	cases := map[string]error{
		"plain sentinel":     ErrServiceUnavailable,
		"unrelated 4xx":      ErrConflict,
		"unclassified error": errors.New("boom"),
		"zero seconds":       RetryAfter(ErrServiceUnavailable, 0),
		"negative seconds":   RetryAfter(ErrServiceUnavailable, -1),
	}
	for name, err := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, err)
			if h := rec.Result().Header.Get("Retry-After"); h != "" {
				t.Fatalf("Retry-After = %q, want no header at all", h)
			}
		})
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/packages/shared-go
go test ./server/ -run 'TestRetryAfter|TestWriteError_setsRetryAfter|TestWriteError_omitsRetryAfter' -v
```

Expected: FAIL — compile error `undefined: RetryAfter`.

- [x] **Step 3: Add the wrapper**

Append to `packages/shared-go/server/errors.go`, immediately after the `detailedError` methods:

```go
// RetryAfter wraps a status sentinel with the number of seconds a client should
// wait before retrying, which WriteError emits as the Retry-After header.
//
// The value rides on the ERROR rather than on a second WriteError entry point,
// mirroring Detailed: the delay is a property of the failure, so it can be
// attached once where the failure is understood and survive any number of
// intermediate returns. A parallel WriteErrorWith… would also have to be
// duplicated for every future response concern.
//
// Error() returns the base sentinel's message for the same reason Detailed's
// does — the envelope `title` is err.Error(), and a wrapper must not rewrite it.
// RetryAfter and Detailed compose in either order; errors.As walks the whole
// chain, so neither has to know about the other.
func RetryAfter(base error, seconds int) error {
	return &retryAfterError{base: base, seconds: seconds}
}

type retryAfterError struct {
	base    error
	seconds int
}

func (e *retryAfterError) Error() string   { return e.base.Error() }
func (e *retryAfterError) Unwrap() error   { return e.base }
func (e *retryAfterError) RetryAfter() int { return e.seconds }
```

- [x] **Step 4: Emit the header in `WriteError`**

In `packages/shared-go/server/jsonapi.go`, insert the header block as the first thing after `status := StatusFor(err)`:

```go
func WriteError(w http.ResponseWriter, err error) {
	status := StatusFor(err)
	// BEFORE WriteJSON, which calls WriteHeader: after that point every header
	// mutation is silently discarded, and the response would still be the right
	// status with the header missing. A non-positive value is a caller bug —
	// Retry-After: 0 tells an intermediary to hammer immediately — so it is
	// dropped rather than emitted.
	var ra interface{ RetryAfter() int }
	if errors.As(err, &ra) && ra.RetryAfter() > 0 {
		w.Header().Set("Retry-After", itoa(ra.RetryAfter()))
	}
	apiErr := APIError{
		Status: itoa(status),
		Code:   codeFor(status),
		Title:  InternalErrorTitle,
	}
	// ...rest of the function unchanged...
```

`errors` is already imported by `jsonapi.go`. Nothing else in the function moves.

- [x] **Step 5: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/packages/shared-go
go test ./... -race
```

Expected: PASS, including every pre-existing test.

- [x] **Step 6: Prove the ordering test can fail**

Move the header block to *after* the `WriteJSON(...)` call at the end of `WriteError`. Re-run:

```bash
go test ./server/ -run TestWriteError_setsRetryAfterBeforeCommittingTheHeaderBlock -v
```

Expected: FAIL with `Retry-After = "" on the committed response`. This is the assertion that the whole header ordering rests on. Restore the block to its correct position and re-run to green.

- [x] **Step 7: Commit**

```bash
git add packages/shared-go/server/errors.go packages/shared-go/server/jsonapi.go packages/shared-go/server/errors_test.go
git commit -m "feat(shared-go): carry Retry-After on the error and emit it before the header block commits"
```

---

## Task 3: `membership.Client.Active` classifies and times out

**Files:**
- Modify: `apps/auth-service/internal/membership/client.go:26-60` (`Active`), `:62-65` (timeout constant), `:77` (`FleetMemberIDs`'s use of it)
- Test: `apps/auth-service/internal/membership/client_test.go`

**Interfaces:**
- Consumes: `server.ErrServiceUnavailable` from Task 1.
- Produces: `Active` returns an error satisfying `errors.Is(err, server.ErrServiceUnavailable)` exactly for: transport failure, timeout, status `>= 500`, status `429`, and an unparseable 2xx body. `404` still returns `(Membership{}, nil)`. Every other non-2xx returns a bare (non-transient) error. Package-level `fleetLookupTimeout` replaces `fleetMemberLookupTimeout`.

- [x] **Step 1: Write the failing tests**

Replace nothing; append to `apps/auth-service/internal/membership/client_test.go`. Add `"errors"`, `"time"` and `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to its imports.

```go
// TestActive_classifiesEveryResponseShape is the table the whole task rests on.
// Each row asserts the CLASSIFICATION, not merely that an error occurred: the
// two buckets have opposite correct responses — a transient error becomes a 503
// that preserves the session, a permanent one becomes a 401 that ends it — so a
// test that only checked `err != nil` would pass while logging every user out.
func TestActive_classifiesEveryResponseShape(t *testing.T) {
	cases := []struct {
		name          string
		status        int
		body          string
		wantErr       bool
		wantTransient bool
		wantFleetID   string
	}{
		{name: "success", status: 200, body: `{"fleet_id":"f1","role":"owner"}`, wantFleetID: "f1"},
		// Load-bearing: a user with no fleet is a real state, and the OIDC
		// callback keys its onboarding redirect off the empty fleet id.
		{name: "no membership", status: 404, body: ""},
		{name: "upstream 500", status: 500, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		{name: "upstream 502", status: 502, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		{name: "upstream 503", status: 503, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		{name: "rate limited", status: 429, body: fleetErrorEnvelope, wantErr: true, wantTransient: true},
		// A 4xx that is not 404 or 429 is a contract or authorization fault
		// between the two services. Retrying cannot fix it, so it must NOT keep
		// the session alive on a promise of recovery.
		{name: "bad request", status: 400, body: fleetErrorEnvelope, wantErr: true},
		{name: "unauthorized", status: 401, body: fleetErrorEnvelope, wantErr: true},
		{name: "forbidden", status: 403, body: fleetErrorEnvelope, wantErr: true},
		// A 2xx whose body will not parse is a garbled or truncated response,
		// not a definitive answer.
		{name: "unparseable 2xx body", status: 200, body: `{"fleet_id":`, wantErr: true, wantTransient: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := serving(t, tc.status, tc.body).Active(context.Background(), "u1")

			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if got := errors.Is(err, server.ErrServiceUnavailable); got != tc.wantTransient {
				t.Fatalf("transient = %v, want %v (err = %v) — this classification is the difference "+
					"between a 503 that preserves the session and a 401 that ends it", got, tc.wantTransient, err)
			}
			if tc.wantErr && m != (Membership{}) {
				t.Fatalf("membership = %+v alongside an error, want the zero value", m)
			}
			if !tc.wantErr && m.FleetID != tc.wantFleetID {
				t.Fatalf("FleetID = %q, want %q", m.FleetID, tc.wantFleetID)
			}
		})
	}
}

// TestActive_classifiesATransportFailureAsTransient covers the shape the
// acceptance criteria name second: fleet-service unreachable, connection
// refused. It also pins the disclosure rule on the one path where it is easy to
// break — url.Error's message embeds the request URL, and this request's URL
// carries the user id as a query parameter.
func TestActive_classifiesATransportFailureAsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close() // nothing is listening on that port now

	_, err := NewClient(base).Active(context.Background(), "user-42")

	if err == nil {
		t.Fatal("an unreachable fleet-service must not resolve to a membership")
	}
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("connection refused must classify transient, got %v", err)
	}
	for _, secret := range []string{"user-42", "user_id"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("the error carries %q — the request URL rode into it, and with it the user id: %q", secret, err)
		}
	}
}

// TestActive_boundsAHangAndClassifiesItTransient is the criterion the other
// tests cannot reach: a hang is the most likely outage shape and the worst,
// because Client shares http.DefaultClient, which has no timeout of its own.
// Without the deadline this test does not fail — it never returns.
func TestActive_boundsAHangAndClassifiesItTransient(t *testing.T) {
	prev := fleetLookupTimeout
	t.Cleanup(func() { fleetLookupTimeout = prev })
	fleetLookupTimeout = 20 * time.Millisecond

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	start := time.Now()
	m, err := NewClient(srv.URL).Active(context.Background(), "u1")

	if err == nil {
		t.Fatalf("a hanging fleet-service returned membership %+v with no error", m)
	}
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("a timeout must classify transient — this is what turns a hang into a 503 rather than a logout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Active took %v; the handler was pinned open rather than bounded by fleetLookupTimeout", elapsed)
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./internal/membership/ -v
```

Expected: FAIL — compile error `undefined: fleetLookupTimeout`.

- [x] **Step 3: Rewrite `Active` and rename the timeout**

In `apps/auth-service/internal/membership/client.go`, add `"errors"` and `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to the imports. Move the timeout declaration **above** `Active` (it now serves both calls) and change it from a `const` to a `var`:

```go
// fleetLookupTimeout bounds one auth→fleet hop, for BOTH calls in this file.
// The Client shares http.DefaultClient, which has NO timeout, so without this a
// wedged fleet-service pins an auth-service handler open indefinitely — and a
// pinned refresh handler is exactly the outage shape that used to end as a
// logout.
//
// A var rather than a const so the timeout test can drive the deadline in
// milliseconds instead of parking the suite for five seconds. Nothing in
// production writes it.
var fleetLookupTimeout = 5 * time.Second
```

Replace the body of `Active` with:

```go
func (c *Client) Active(ctx context.Context, userID string) (Membership, error) {
	ctx, cancel := context.WithTimeout(ctx, fleetLookupTimeout)
	defer cancel()

	// QueryEscape even though userID is an internal UUID off a validated token:
	// it costs nothing and stops this file contradicting FleetMemberIDs's own
	// comment, which names Active's raw concatenation as the habit not to
	// inherit.
	endpoint := c.base + "/internal/memberships/active?user_id=" + url.QueryEscape(userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		// Deliberately NOT transient: a base URL that will not parse is our own
		// misconfiguration, and no amount of retrying fixes it.
		return Membership{}, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		// Connection refused, DNS failure, TLS error — and the deadline above,
		// which surfaces here as a *url.Error wrapping context.DeadlineExceeded.
		// All of them mean the answer is UNKNOWN, which is not the same as "this
		// user has no fleet" and must not end the session.
		//
		// url.Error's own message embeds the request URL, and this URL carries
		// the user id as a query parameter, so unwrap to the transport error
		// underneath: "connection refused", "no such host", "context deadline
		// exceeded" — the diagnostic value without the address.
		detail := err
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Err != nil {
			detail = urlErr.Err
		}
		return Membership{}, fmt.Errorf("%w: active membership lookup transport failure: %v",
			server.ErrServiceUnavailable, detail)
	}
	defer func() { _ = res.Body.Close() }()
	// 404 is the one status that is not a failure: a user with no fleet resolves
	// to a zero Membership, and the OIDC callback keys its onboarding redirect
	// off the resulting empty ActiveFleetID. Do not turn this into an error —
	// it would break a brand-new user's first login.
	if res.StatusCode == http.StatusNotFound {
		return Membership{}, nil
	}
	// 5xx and 429 are the upstream saying "not now", not "no". Classifying them
	// transient is what lets the caller answer 503 and keep the session, instead
	// of reading someone else's outage as a dead credential.
	if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests {
		return Membership{}, fmt.Errorf("%w: active membership lookup failed with status %d",
			server.ErrServiceUnavailable, res.StatusCode)
	}
	// Every OTHER non-2xx must be an error, and a PERMANENT one. fleet-service's
	// error envelope is JSON, so without this check a non-2xx decodes cleanly
	// into a zero Membership with err == nil — indistinguishable from the 404
	// above. The caller then mints a valid token claiming the user has no fleet
	// and the SPA offers to create a duplicate one. A 400 or 403 here is a
	// contract or authorization fault between the two services that retrying
	// will not fix, so it stays off the transient path.
	//
	// Status code and a fixed description only: the body is upstream-controlled
	// and could carry anything, and the user id must not ride along in a
	// message that ends up in a log as an address.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return Membership{}, fmt.Errorf("active membership lookup failed with status %d", res.StatusCode)
	}
	var m Membership
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		// Fixed text, and the decoder's error is dropped: its message quotes
		// bytes from an upstream-controlled body.
		return Membership{}, fmt.Errorf("%w: active membership lookup returned an unparseable body",
			server.ErrServiceUnavailable)
	}
	return m, nil
}
```

Then update `FleetMemberIDs` to use the renamed value — change `context.WithTimeout(ctx, fleetMemberLookupTimeout)` to `context.WithTimeout(ctx, fleetLookupTimeout)`, and delete the old `fleetMemberLookupTimeout` declaration block (its doc comment has moved above `Active`). `FleetMemberIDs` gets no classification: its callers do not need the distinction.

- [x] **Step 4: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./internal/membership/ -race -v
```

Expected: PASS, including the four pre-existing `TestActive_*` tests and all four `TestFleetMemberIDs_*` tests. `TestActive_errorDisclosesNeitherBodyNorUser` in particular must still pass — the transient 500 wrap still carries the status and still carries neither body nor user id.

- [x] **Step 5: Prove the new tests can fail**

Three reverts, one at a time, re-running `go test ./internal/membership/` after each and restoring before the next:

1. Change the `>= 500 || 429` branch to fall through to the bare `fmt.Errorf` (drop the `%w` sentinel) → `TestActive_classifiesEveryResponseShape/upstream_500` goes red on the transient assertion.
2. Delete the `context.WithTimeout` lines from `Active` → `TestActive_boundsAHangAndClassifiesItTransient` hangs (kill it after ~10s; a hang here **is** the failure, and it is what the production bug looks like).
3. Change the transport wrap to `fmt.Errorf("%w: %v", server.ErrServiceUnavailable, err)` (using the raw `url.Error`) → `TestActive_classifiesATransportFailureAsTransient` goes red on the `user_id` disclosure assertion.

- [x] **Step 6: Commit**

```bash
git add apps/auth-service/internal/membership/client.go apps/auth-service/internal/membership/client_test.go
git commit -m "feat(auth): classify transient fleet lookup failures and bound Active with a timeout"
```

---

## Task 4: The resolver carries the classification, and classifies its own infrastructure

**Files:**
- Modify: `apps/auth-service/cmd/main.go:173-200` (`newPrincipalResolver`) and its import block
- Test: `apps/auth-service/cmd/main_test.go`

**Interfaces:**
- Consumes: `server.ErrServiceUnavailable` (Task 1), `Active`'s classification (Task 3).
- Produces: the `session.PrincipalResolver` closure returns an error satisfying `errors.Is(err, server.ErrServiceUnavailable)` when fleet-service is unavailable **or** when the local `users` / `platform_admins` reads fail for any reason other than `user.ErrNotFound`. `user.ErrNotFound` propagates bare and must never classify transient. Tasks 5 and 6 branch on exactly this.

**Note on scope:** the local-infrastructure classification is design D9, an explicit extension beyond the PRD's literal text, flagged there as "recommended rather than assumed". It is two `if` blocks. Rows 2 and 4 of D9's table are the same defect one layer over — an auth Postgres blip lasting longer than one request logs out every active session through the same code path.

- [x] **Step 1: Write the failing tests**

Append to `apps/auth-service/cmd/main_test.go`. Add `"github.com/jtumidanski/myfleet/packages/shared-go/server"` and `"strings"` to its imports (`errors`, `fmt`, `context`, `net/http` are already there).

```go
// TestNewPrincipalResolver_carriesTheTransientClassificationThrough is a
// regression test, not a test of new code: main.go returns the membership
// error bare today, so errors.Is already reaches through it. The risk is
// tomorrow's edit — someone annotating it with fmt.Errorf("...: %v", err)
// silently breaks classification for BOTH handlers, every transient failure
// becomes a logout again, and nothing else in the repository goes red.
func TestNewPrincipalResolver_carriesTheTransientClassificationThrough(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusInternalServerError, `{"errors":[{"status":"500"}]}`),
		&fakeAdmins{isAdmin: false},
	)

	_, err := resolve(context.Background(), "user-1")

	if err == nil {
		t.Fatal("a 500 from fleet-service must not resolve to a principal")
	}
	if !errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("classification did not survive the resolver: %v — both handlers "+
			"branch on errors.Is, so a %%v wrap here turns every outage back into a logout", err)
	}
}

// TestNewPrincipalResolver_keepsAMissingUserPermanent is the other half. A user
// row that is gone is a DEAD credential: it must reach the 401-and-clear path,
// not the 503 that invites the browser to try again forever.
func TestNewPrincipalResolver_keepsAMissingUserPermanent(t *testing.T) {
	resolve := newPrincipalResolver(
		&fakeUsers{byID: map[string]user.Model{}},
		fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
		&fakeAdmins{isAdmin: false},
	)

	_, err := resolve(context.Background(), "user-1")

	if !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("err = %v, want user.ErrNotFound", err)
	}
	if errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("a missing user row classified as transient: %v — the credential is dead and the session must end", err)
	}
}

// TestNewPrincipalResolver_keepsAPermanentUpstreamFaultPermanent: a 403 from
// fleet-service is a misconfiguration between the two services, not an outage.
// Retrying cannot fix it, so it keeps today's fail-closed behaviour.
func TestNewPrincipalResolver_keepsAPermanentUpstreamFaultPermanent(t *testing.T) {
	resolve := newPrincipalResolver(
		usersWith("user-1", "a@b.com"),
		fleetServing(t, http.StatusForbidden, `{"errors":[{"status":"403"}]}`),
		&fakeAdmins{isAdmin: false},
	)

	_, err := resolve(context.Background(), "user-1")

	if err == nil {
		t.Fatal("a 403 must not resolve to a principal")
	}
	if errors.Is(err, server.ErrServiceUnavailable) {
		t.Fatalf("a 403 classified as transient: %v", err)
	}
}

// TestNewPrincipalResolver_classifiesLocalInfrastructureFailuresAsTransient
// (design D9). The resolver reads three sources; only one of them is
// fleet-service. An auth-Postgres blip lasting longer than one request logs out
// every active session through this same closure, for the same reason. The
// driver's text is deliberately dropped rather than %w-wrapped so a SQLSTATE or
// table name cannot ride into a log line even in principle.
func TestNewPrincipalResolver_classifiesLocalInfrastructureFailuresAsTransient(t *testing.T) {
	dbDown := errors.New(`pq: relation "auth.users" does not exist (SQLSTATE 42P01)`)

	cases := map[string]func() (user.Provider, *fakeAdmins){
		"users read fails": func() (user.Provider, *fakeAdmins) {
			return &fakeUsers{err: dbDown}, &fakeAdmins{isAdmin: false}
		},
		"platform admin read fails": func() (user.Provider, *fakeAdmins) {
			return usersWith("user-1", "a@b.com"), &fakeAdmins{err: dbDown}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			users, admins := setup()
			resolve := newPrincipalResolver(
				users,
				fleetServing(t, http.StatusOK, `{"fleet_id":"fleet-9","role":"owner"}`),
				admins,
			)

			_, err := resolve(context.Background(), "user-1")

			if err == nil {
				t.Fatal("a failed local read must not resolve to a principal")
			}
			if !errors.Is(err, server.ErrServiceUnavailable) {
				t.Fatalf("err = %v, want a transient classification — a database hiccup must not "+
					"log every active session out", err)
			}
			for _, secret := range []string{"SQLSTATE", "42P01", "auth.users", "pq:"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("the error carries %q from the driver: %q", secret, err)
				}
			}
		})
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./cmd/ -run TestNewPrincipalResolver -v
```

Expected: `carriesTheTransientClassificationThrough`, `keepsAMissingUserPermanent` and `keepsAPermanentUpstreamFaultPermanent` PASS already (they guard behaviour that exists); `classifiesLocalInfrastructureFailuresAsTransient` FAILS on `want a transient classification`. That split is the point — three of them are regression nets and one drives new code.

- [x] **Step 3: Classify the local reads**

In `apps/auth-service/cmd/main.go`, add `"errors"` and `"fmt"` to the standard-library import group. Then change the two local-read error branches inside `newPrincipalResolver`:

```go
		u, err := users.GetByID(userID)
		if err != nil {
			// A missing row is the PERMANENT case: the credential names a user
			// who is gone, and the session must end.
			if errors.Is(err, user.ErrNotFound) {
				return session.Principal{}, err
			}
			// Anything else from the local store — a dead pool, a failed-over
			// primary, a connection refused — is the same defect this task fixes
			// one layer up: the answer is unknown, so the session must survive.
			// The driver's error is deliberately NOT %w-wrapped, so a SQLSTATE or
			// table name cannot ride into a log line even in principle.
			return session.Principal{}, fmt.Errorf("%w: user lookup failed", server.ErrServiceUnavailable)
		}
		// A 404 from fleet-service is NOT an error here: membership.Client maps
		// it to a zero Membership, and the OIDC callback keys its onboarding
		// redirect off an empty ActiveFleetID. Turning it into an error would
		// break a new user's first login. A transport or 5xx failure IS an
		// error, and arrives already classified — it passes through bare so
		// errors.Is keeps reaching the sentinel.
		m, err := fleet.Active(ctx, userID)
		if err != nil {
			return session.Principal{}, err
		}
		// Fail closed: a lookup error must not mint a token that silently
		// claims false, because the console's absence would then read as
		// "you are not an admin" rather than "we could not tell". Transient for
		// the same reason as the users read above.
		isAdmin, err := admins.IsAdmin(userID)
		if err != nil {
			return session.Principal{}, fmt.Errorf("%w: platform admin lookup failed", server.ErrServiceUnavailable)
		}
```

The `session.Principal{...}` literal at the end of the closure is unchanged.

- [x] **Step 4: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./cmd/ -race -v
```

Expected: PASS. `TestNewPrincipalResolver_treatsNoMembershipAsEmptyNotError` must be green and **unmodified** — check with `git diff apps/auth-service/cmd/main_test.go` that no line inside it changed.

- [x] **Step 5: Prove the new test can fail**

Replace the `users.GetByID` transient wrap with `return session.Principal{}, err`, re-run `go test ./cmd/ -run TestNewPrincipalResolver_classifiesLocal -v`, confirm the `users read fails` sub-test goes red, then restore.

- [x] **Step 6: Commit**

```bash
git add apps/auth-service/cmd/main.go apps/auth-service/cmd/main_test.go
git commit -m "feat(auth): classify local infrastructure failures in the principal resolver as transient"
```

---

## Task 5: `POST /auth/refresh` answers 503 and keeps the session

This is the task the whole PRD is about. FR-REFRESH-5 — the rotated cookie — is the highest-risk item in it: a `503` that is followed by a family-revoking replay on the user's next attempt satisfies none of the acceptance criteria.

**Files:**
- Modify: `apps/auth-service/internal/session/resource.go:1-13` (imports), add a package constant, `:59-69` (the resolver-error branch)
- Test: `apps/auth-service/internal/session/resource_test.go`

**Interfaces:**
- Consumes: `server.ErrServiceUnavailable` and `server.RetryAfter` (Tasks 1–2), the resolver's classification (Task 4).
- Produces: `POST /auth/refresh` → `503` + `Retry-After: 5` + a `Set-Cookie` carrying the **rotated** refresh token on a transient resolver error; `401` + clearing `Set-Cookie` on a permanent one. Package constant `refreshRetryAfterSeconds = 5`.

- [x] **Step 1: Write the failing tests**

In `apps/auth-service/internal/session/resource_test.go`, first widen the router helper so tests can read what was logged. Replace `newRefreshRouter` with:

```go
// newRefreshRouter mounts the real public routes over a store seeded with one
// valid refresh token, and returns the router, the processor, a hook holding
// everything the handler logged, and that token's raw value.
func newRefreshRouter(t *testing.T, resolve PrincipalResolver) (chi.Router, *Processor, *logrustest.Hook, string) {
	t.Helper()
	store := newFakeStore()
	proc := newTestProcessor(store)

	raw := "valid-refresh-token"
	store.seed(NewBuilder().
		SetUserID("user-1").
		SetTokenHash(HashRefresh(raw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build())

	log, hook := logrustest.NewNullLogger()

	r := chi.NewRouter()
	r.Group(InitializePublicRoutes(log, proc, resolve, false))
	return r, proc, hook, raw
}

// refreshCookieSet returns the non-clearing refresh cookie the response sets,
// or nil. Distinct from refreshCookieCleared: "no cookie at all" and "a cookie
// carrying the rotated token" are different outcomes with different
// consequences on the user's next attempt.
func refreshCookieSet(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == RefreshCookieName && c.Value != "" {
			return c
		}
	}
	return nil
}
```

Update the imports: drop `"io"`, add `"fmt"`, `"strings"`, `logrustest "github.com/sirupsen/logrus/hooks/test"`, and `"github.com/jtumidanski/myfleet/packages/shared-go/server"`. Keep `"github.com/sirupsen/logrus"` (the log-level assertions use it). Update the two existing call sites to the four-value form: `r, proc, _, raw := newRefreshRouter(t, resolve)` in `TestRefresh_mintsAccessTokenCarryingEmailClaim`, and `r, _, hook, raw := newRefreshRouter(t, resolve)` in `TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors`.

Then add these assertions to the end of the existing `TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors`, so the permanent branch is pinned on all three axes too:

```go
	if got := rec.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q on the permanent path; a dead credential is not something to retry", got)
	}
	if !loggedAtLevel(hook, logrus.ErrorLevel, "resolve principal on refresh") {
		t.Fatalf("the permanent failure must still log at error: %+v", hook.AllEntries())
	}
```

And append the new tests:

```go
// loggedAtLevel reports whether the handler wrote exactly this message at
// exactly this level. Level AND message, because FR-REFRESH-6 is about an
// outage not reading as a wave of authentication failures — same text at a
// different level, or a different text at the same level, both fail that.
func loggedAtLevel(hook *logrustest.Hook, level logrus.Level, msg string) bool {
	for _, e := range hook.AllEntries() {
		if e.Level == level && e.Message == msg {
			return true
		}
	}
	return false
}

// transientResolver stands in for a fleet-service outage, in the exact shape
// membership.Client produces.
func transientResolver() PrincipalResolver {
	return func(context.Context, string) (Principal, error) {
		return Principal{}, fmt.Errorf("%w: active membership lookup failed with status 500",
			server.ErrServiceUnavailable)
	}
}

// TestRefresh_transientResolverFailureKeepsTheSessionAlive is the whole task in
// one test. Asserting the status ALONE would pass while still logging the user
// out — which is precisely the failure mode the acceptance criteria call out —
// so this asserts the three things that actually decide the outcome: the
// status, the Retry-After, and the cookie the browser is left holding.
func TestRefresh_transientResolverFailureKeepsTheSessionAlive(t *testing.T) {
	r, proc, hook, raw := newRefreshRouter(t, transientResolver())

	rec := postRefresh(r, raw)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("Retry-After = %q, want \"5\"", got)
	}
	if refreshCookieCleared(rec) {
		t.Fatal("the refresh cookie was cleared — someone else's outage must not end this session")
	}

	// FR-REFRESH-5, the highest-risk item in the task. Rotate has ALREADY
	// consumed the presented token and committed a new one to the store. If the
	// browser is left holding the old value, its next attempt is a replay, and
	// Processor.Rotate answers a replay by revoking the whole family — the exact
	// logout this branch exists to prevent. So the rotated value must be written.
	c := refreshCookieSet(rec)
	if c == nil {
		t.Fatal("no refresh cookie was set: the browser would keep the token Rotate already consumed " +
			"and trip reuse detection on its next attempt")
	}
	if c.Value == raw {
		t.Fatalf("the cookie still carries the CONSUMED token %q", raw)
	}
	if c.MaxAge < 0 {
		t.Fatalf("MaxAge = %d — this is the clearing form of the cookie", c.MaxAge)
	}

	// Acceptance criterion 4, asserted against stored state rather than against
	// the response: the value the browser now holds must still rotate cleanly
	// once fleet-service recovers.
	if _, userID, err := proc.Rotate(c.Value); err != nil {
		t.Fatalf("re-presenting the cookie written alongside the 503 failed: %v — "+
			"the retry after recovery would log the user out", err)
	} else if userID != "user-1" {
		t.Fatalf("userID = %q, want user-1", userID)
	}

	// FR-REFRESH-6: an outage is greppable as one and does not inflate the
	// error rate.
	if !loggedAtLevel(hook, logrus.WarnLevel, "resolve principal on refresh: upstream unavailable") {
		t.Fatalf("the transient failure must log at warn with its own message: %+v", hook.AllEntries())
	}
	if loggedAtLevel(hook, logrus.ErrorLevel, "resolve principal on refresh") {
		t.Fatal("the transient failure also logged the permanent path's error line")
	}
}

// TestRefresh_transientFailureMintsNothingAndDisclosesNothing: failing closed is
// unchanged — a token with incomplete identity is never minted — and the body
// of a 503 on a PUBLIC endpoint tells an unauthenticated caller only that
// something is unavailable.
func TestRefresh_transientFailureMintsNothingAndDisclosesNothing(t *testing.T) {
	r, _, _, raw := newRefreshRouter(t, transientResolver())

	rec := postRefresh(r, raw)

	var body struct {
		Data   any `json:"data"`
		Errors []struct {
			Status string `json:"status"`
			Code   string `json:"code"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data != nil {
		t.Fatalf("a 503 carried a data member: %v — no token may be minted on this path", body.Data)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %d, want 1", len(body.Errors))
	}
	if body.Errors[0].Status != "503" || body.Errors[0].Code != "service_unavailable" {
		t.Fatalf("status/code = %q/%q, want 503/service_unavailable — the SPA keys on the code",
			body.Errors[0].Status, body.Errors[0].Code)
	}
	if body.Errors[0].Detail != "" {
		t.Fatalf("detail = %q, want empty", body.Errors[0].Detail)
	}
	for _, secret := range []string{"membership", "lookup", "user-1"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("the 503 body leaked %q: %s", secret, rec.Body.String())
		}
	}
}

// TestRefresh_transientFailureCannotReviveARevokedToken: a 503 must never
// become a way to keep a dead session alive. Rotate decides token validity and
// runs BEFORE the resolver, so a revoked cookie still ends at 401-and-clear
// however unavailable fleet-service is.
func TestRefresh_transientFailureCannotReviveARevokedToken(t *testing.T) {
	r, _, _, raw := newRefreshRouter(t, transientResolver())

	// Spend the token first: the second presentation is a replay.
	postRefresh(r, raw)
	rec := postRefresh(r, raw)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — a replayed token must not reach the transient branch", rec.Code)
	}
	if !refreshCookieCleared(rec) {
		t.Fatalf("a replayed token must still clear the cookie; cookies = %v", rec.Result().Cookies())
	}
}
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./internal/session/ -run TestRefresh -v
```

Expected: FAIL — `status = 401, want 503`.

- [x] **Step 3: Split the resolver-error branch**

In `apps/auth-service/internal/session/resource.go`, add `"errors"` to the standard-library imports. Add this constant just below `RefreshCookieName`:

```go
// refreshRetryAfterSeconds is the advisory Retry-After on a 503 from this
// handler. It matches the auth→fleet lookup timeout and the restart timescale
// of a fleet-service pod. Advisory only: the SPA does not auto-retry, so the
// header is for correctness, intermediaries, and any future client.
const refreshRetryAfterSeconds = 5
```

Replace the resolver-error block in `refreshHandler` with:

```go
		principal, err := resolve(req.Context(), userID)
		if err != nil {
			if errors.Is(err, server.ErrServiceUnavailable) {
				// Someone else's outage is not an authentication failure. The
				// refresh token is still good, so the session survives — but the
				// cookie must carry the ROTATED value. Rotate already consumed
				// the presented token and committed the new one to the store, so
				// leaving the browser with the old value makes its next attempt
				// indistinguishable from a replay, and Processor.Rotate answers a
				// replay by revoking the whole family. That revocation is exactly
				// the logout this branch exists to prevent.
				//
				// Warn with its own message, not Error: an outage must read as an
				// outage rather than as a wave of authentication failures.
				log.WithError(err).Warn("resolve principal on refresh: upstream unavailable")
				SetRefreshCookie(w, newRaw, cookieSecure)
				server.WriteError(w, server.RetryAfter(err, refreshRetryAfterSeconds))
				return
			}
			// Fail closed (FR-5): a token with incomplete identity is never
			// minted. Clear the cookie too — unlike this path's previous bare
			// 401 — so a session whose user row is gone stops re-presenting a
			// credential that can only 401.
			log.WithError(err).Error("resolve principal on refresh")
			clearRefreshCookie(w, cookieSecure)
			server.WriteError(w, server.ErrUnauthorized)
			return
		}
```

No access token is minted and no token document is written on the transient branch — `WriteError` writes the whole response. Every other exit in the handler (missing token, `Rotate` failure, mint failure, success) is untouched.

- [x] **Step 4: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./internal/session/ -race -v
```

Expected: PASS, including `TestRefresh_mintsAccessTokenCarryingEmailClaim` and the (extended) `TestRefresh_failsClosedAndClearsCookieWhenTheResolverErrors`.

- [x] **Step 5: Prove the cookie assertion can fail**

Delete the `SetRefreshCookie(w, newRaw, cookieSecure)` line — this is design D3's explicitly rejected variant, "clear nothing at all", which satisfies FR-REFRESH-2's letter while guaranteeing the family-revoking replay. Re-run:

```bash
go test ./internal/session/ -run TestRefresh_transientResolverFailureKeepsTheSessionAlive -v
```

Expected: FAIL on `no refresh cookie was set`. Restore the line. Then separately change `Warn` to `Error` and confirm the log assertion goes red, and restore that too.

- [x] **Step 6: Commit**

```bash
git add apps/auth-service/internal/session/resource.go apps/auth-service/internal/session/resource_test.go
git commit -m "feat(auth): answer refresh with 503 and the rotated cookie when an upstream is unavailable"
```

---

## Task 6: The OIDC callback distinguishes an outage

**Files:**
- Modify: `apps/auth-service/internal/oidc/resource.go:1-21` (imports), `:88-93` (`loginErrorCode` constants), `:262-267` (the `d.Resolve` failure branch)
- Test: `apps/auth-service/internal/oidc/resource_test.go`

**Interfaces:**
- Consumes: `server.ErrServiceUnavailable` (Task 1), the resolver's classification (Task 4).
- Produces: `GET /auth/callback` redirects to `<AppBaseURL><LoginPath>#error=service_unavailable` on a transient resolver failure. Task 8's SPA code consumes that exact string.

**Correction carried from the PRD:** issue #15 says the callback answers `500`. It does not — it `302`-redirects with an `#error=` fragment, so "distinguishing transient here" means a new `loginErrorCode`, not a status.

- [x] **Step 1: Write the failing test**

Append to `apps/auth-service/internal/oidc/resource_test.go` (add `"fmt"` and `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to its imports; `strings` and `session` are already there):

```go
// TestCallback_transientResolverFailureRedirectsWithServiceUnavailable: a user
// signing in during a fleet-service outage is told to try again shortly, rather
// than being shown a generic failure that implies their account is broken. The
// permanent case in TestCallback_failureRedirectsToLogin ("principal resolution
// fails", plain error → server_error) must stay green beside this: that pairing
// is what proves the branch actually split rather than moved.
func TestCallback_transientResolverFailureRedirectsWithServiceUnavailable(t *testing.T) {
	d := okDeps()
	d.Resolve = func(context.Context, string) (session.Principal, error) {
		return session.Principal{}, fmt.Errorf("%w: active membership lookup failed with status 500",
			server.ErrServiceUnavailable)
	}

	rec, hook := callback(t, d, "/auth/callback?code=abc&state=s-1", stateCookie(t, "s-1", "nonce-1"))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	want := "http://app.test/login#error=service_unavailable"
	if got := rec.Header().Get("Location"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
	// FR-REFRESH-6's callback-side twin: an outage must not read as a wave of
	// authentication failures.
	if !loggedAt(hook, logrus.WarnLevel, "resolve principal on callback: upstream unavailable") {
		t.Errorf("the transient failure must log at warn with its own message: %+v", hook.AllEntries())
	}
	// FR-CALLBACK-3: the state cookie behaviour is identical to every other
	// post-verification exit. failLogin's deliberate non-clearing and the single
	// unconditional clear after verifyStateCookie are a reviewed departure from
	// task-010's FR-ERR-8 — do not disturb them.
	if !clearsStateCookie(rec) {
		t.Error("this exit is after state verification, so the abandoned attempt's cookie must still be cleared")
	}
	// FR-CALLBACK-4: no refresh token has been issued on this path, so there is
	// no cookie-preservation question — and no half-session may be left behind.
	for _, c := range rec.Result().Cookies() {
		if c.Name == session.RefreshCookieName && c.Value != "" {
			t.Errorf("a failed callback set a refresh cookie: %q", c.Value)
		}
	}
	if strings.Contains(rec.Header().Get("Location"), "access_token") {
		t.Error("a failed callback carried an access token in the fragment")
	}
}
```

- [x] **Step 2: Run the test to verify it fails**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./internal/oidc/ -run TestCallback_transientResolverFailure -v
```

Expected: FAIL — `Location = "http://app.test/login#error=server_error", want ".../login#error=service_unavailable"`.

- [x] **Step 3: Add the code and split the branch**

In `apps/auth-service/internal/oidc/resource.go`, add `"errors"` to the standard-library imports and `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to the project import group. Add the fifth constant:

```go
const (
	errCancelled    loginErrorCode = "cancelled"
	errInvalidState loginErrorCode = "invalid_state"
	errAuthFailed   loginErrorCode = "auth_failed"
	errServerError  loginErrorCode = "server_error"
	// The one failure whose ADVICE differs: wait and retry, rather than try a
	// different Google account. It is emitted from exactly one call site — the
	// resolver's transient branch — and every other failLogin keeps its code.
	errServiceUnavailable loginErrorCode = "service_unavailable"
)
```

Replace the `d.Resolve` failure branch:

```go
		principal, err := d.Resolve(ctx, u.ID())
		if err != nil {
			// A fleet-service or database outage is not a broken account. Warn,
			// not Error, and its own login code, so the page can say "try again
			// in a moment" instead of implying the user's account is at fault.
			if errors.Is(err, server.ErrServiceUnavailable) {
				log.WithError(err).Warn("resolve principal on callback: upstream unavailable")
				failLogin(w, req, d, errServiceUnavailable)
				return
			}
			log.WithError(err).Error("resolve principal on callback")
			failLogin(w, req, d, errServerError)
			return
		}
```

Nothing else in the handler changes — in particular, do not touch `failLogin` or the single unconditional `clearStateCookie` call.

- [x] **Step 4: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503/apps/auth-service
go test ./internal/oidc/ -race -v
```

Expected: PASS, and specifically `TestCallback_failureRedirectsToLogin/principal_resolution_fails` must still assert `server_error` at `ErrorLevel`.

- [x] **Step 5: Prove the test can fail**

Change `failLogin(w, req, d, errServiceUnavailable)` to `errServerError`, re-run, confirm the `Location` assertion goes red, restore.

- [x] **Step 6: Commit**

```bash
git add apps/auth-service/internal/oidc/resource.go apps/auth-service/internal/oidc/resource_test.go
git commit -m "feat(auth): redirect the OIDC callback with service_unavailable during an upstream outage"
```

---

## Task 7: The SPA stops treating a `503` refresh as a dead session

FR-SPA-3 names the actual logout mechanism: `refresh.ts:26` returns `null` for every non-`ok` response, `refreshAccessToken` then calls `clearAccessToken()`, `useAuth().isAuthenticated` goes false, and `RequireAuth` navigates to `/login`. This task breaks that chain for `503` only.

**Files:**
- Modify: `apps/web/src/lib/api/refresh.ts` (whole file)
- Test: `apps/web/src/lib/api/refresh.test.ts`, `packages/shared-ts/src/apiClient.test.ts`

**Interfaces:**
- Consumes: the `503` + `service_unavailable` response from Task 5; `ApiError` from `@myfleet/shared-ts`.
- Produces: `mintAccessToken(): Promise<string | null>` — **signature and behaviour unchanged**, never clears, never throws. `refreshAccessToken(): Promise<string | null>` — resolves the token on success, resolves `null` **and clears** on a dead session, and **rejects** with `new ApiError(503, 'service_unavailable', ...)` **without clearing** on a `503`. `packages/shared-ts` source is not modified: the rejection travels out through the existing `onRefresh: () => Promise<string | null>` channel.

- [x] **Step 1: Write the failing tests**

In `apps/web/src/lib/api/refresh.test.ts`, add `ApiError` to the imports (`import { ApiError } from '@myfleet/shared-ts';`) and add this helper next to `jsonResponse`:

```ts
/** A response with a real `status`, which `jsonResponse` deliberately omits. */
function statusResponse(status: number, body: unknown = null): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response;
}
```

Append to the `refreshAccessToken` describe block:

```ts
  // The point of the whole task. A 503 says auth-service could not reach what
  // it needed to answer — it says nothing about whether this session is still
  // good. Clearing the token here is what navigates the user to /login.
  it('keeps the stored token and rejects with a 503 when the service is unavailable', async () => {
    setAccessToken('still-valid');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(503)));

    await expect(refreshAccessToken()).rejects.toBeInstanceOf(ApiError);
    // The stored token is the state that decides isAuthenticated, and therefore
    // whether RequireAuth navigates away. Assert it, not just the rejection.
    expect(getAccessToken()).toBe('still-valid');
  });

  it('rejects with status 503 so the original request does not surface as a 401', async () => {
    setAccessToken('still-valid');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(503)));

    await expect(refreshAccessToken()).rejects.toMatchObject({
      status: 503,
      code: 'service_unavailable',
    });
  });

  it('still clears the stored token on a 401', async () => {
    setAccessToken('dead');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(401)));

    await expect(refreshAccessToken()).resolves.toBeNull();
    expect(getAccessToken()).toBeNull();
  });

  // The dedupe is load-bearing regardless of outcome: two POSTs carrying the
  // same rotating cookie are reuse, and auth-service answers reuse by revoking
  // the whole family — the logout this task is preventing.
  it('still dedupes concurrent callers onto one request during an outage', async () => {
    const fetchMock = vi.fn().mockResolvedValue(statusResponse(503));
    vi.stubGlobal('fetch', fetchMock);

    const results = await Promise.allSettled([refreshAccessToken(), refreshAccessToken()]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(results.map((r) => r.status)).toEqual(['rejected', 'rejected']);
  });
```

And to the `mintAccessToken` describe block:

```ts
  // FR-SPA-7: mintAccessToken's contract is unchanged. Its callers
  // (invites.ts, members.ts) run it on a HEALTHY session to pick up fresher
  // claims, so it must neither throw at them nor discard a working token.
  it('resolves null without throwing or clearing when the service is unavailable', async () => {
    setAccessToken('still-valid');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse(503)));

    await expect(mintAccessToken()).resolves.toBeNull();
    expect(getAccessToken()).toBe('still-valid');
  });
```

Then, in `packages/shared-ts/src/apiClient.test.ts`, append a new describe block. This is a **test-only** change — it pins FR-SPA-5 and FR-SPA-6, which hold by construction:

```ts
describe('ApiClient refresh failures', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  // FR-SPA-6. When the original request 401s and the refresh answers 503, the
  // caller must see the 503. A 401 would be a lie — the caller's credentials
  // were never the problem — and downstream code keys on status. This works
  // with no code in this package: a rejection from onRefresh propagates
  // straight out of fetchAuthenticated, which is exactly why refresh.ts throws
  // instead of widening the onRefresh return type.
  it('propagates a rejecting onRefresh instead of surfacing the original 401', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ errors: [{ status: '401', code: 'unauthorized', title: 'unauthorized' }] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => {
        throw new ApiError(503, 'service_unavailable', 'Service temporarily unavailable');
      },
    });

    await expect(client.request('/api/fleet/vehicles')).rejects.toMatchObject({ status: 503 });
    // FR-SPA-5: one-shot, no retry loop. The original request was attempted
    // once and never retried.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  // The other half: an onRefresh that resolves null keeps today's behaviour
  // exactly — the original 401 surfaces, still one-shot.
  it('surfaces the original 401 when onRefresh resolves null', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({ errors: [{ status: '401', code: 'unauthorized', title: 'unauthorized' }] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({
      baseUrl: '',
      getAccessToken: () => 'tok-123',
      onRefresh: async () => null,
    });

    await expect(client.request('/api/fleet/vehicles')).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/api/refresh.test.ts
npm run -w packages/shared-ts test
```

Expected: `refresh.test.ts` FAILS (`refreshAccessToken` resolves `null` instead of rejecting, and the token is cleared). The `shared-ts` block PASSES already — it documents a property that holds by construction, and it is the guard against a future edit to `fetchAuthenticated` swallowing the rejection.

- [x] **Step 3: Rewrite `refresh.ts`**

Replace the whole of `apps/web/src/lib/api/refresh.ts` with:

```ts
import { ApiError } from '@myfleet/shared-ts';
import { setAccessToken, clearAccessToken } from './token';

interface RefreshResponse {
  data?: { attributes?: { accessToken?: string } };
}

/**
 * What one POST to /auth/refresh concluded.
 *
 * `unavailable` is the distinction the whole type exists for. auth-service
 * answers 503 when it could not reach what it needed to resolve the session —
 * a fleet-service outage, a database blip — which says nothing about whether
 * the session is still good. Collapsing it into `dead`, as a bare `null` did,
 * turns someone else's brief outage into a forced sign-in through Google.
 */
type RefreshOutcome =
  | { status: 'ok'; token: string }
  | { status: 'dead' }
  | { status: 'unavailable' };

/**
 * In-flight refresh, shared by every caller.
 *
 * auth-service rotates the refresh token on use and treats a replay as reuse,
 * revoking the whole token family — so two concurrent POSTs to /auth/refresh
 * with the same cookie log the user out everywhere. Callers are deduped onto
 * one request rather than being trusted to serialize themselves.
 */
let inflight: Promise<RefreshOutcome> | null = null;

async function requestToken(): Promise<RefreshOutcome> {
  try {
    const res = await fetch('/api/auth/refresh', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/vnd.api+json' },
    });
    // Checked before `ok` so it can never fall into the dead bucket.
    if (res.status === 503) return { status: 'unavailable' };
    if (!res.ok) return { status: 'dead' };
    const body = (await res.json().catch(() => null)) as RefreshResponse | null;
    const token = body?.data?.attributes?.accessToken;
    if (!token) return { status: 'dead' };
    setAccessToken(token);
    return { status: 'ok', token };
  } catch {
    return { status: 'dead' };
  }
}

/**
 * The shared request, unchanged in kind: the promise held in `inflight` still
 * RESOLVES — never rejects — so concurrent callers collapse onto one POST and
 * `.finally` frees the slot exactly as before. That is why the outcome travels
 * as a value rather than as a rejection at this layer: a shared promise that
 * rejects is where unhandled-rejection and double-settle bugs live. Callers
 * translate the outcome into their own contract.
 */
function refreshOnce(): Promise<RefreshOutcome> {
  if (!inflight) {
    inflight = requestToken().finally(() => {
      inflight = null;
    });
  }
  return inflight;
}

/**
 * Exchanges the HttpOnly refresh cookie for a new access token and stores it,
 * returning null if that fails. A failure leaves the existing token ALONE, and
 * never throws.
 *
 * Use this when the goal is fresher claims — e.g. after accepting an invite,
 * where `active_fleet_id` and `role` were fixed into the JWT at mint time and
 * only move when a new token is issued. The current token is still valid in
 * that case, so discarding it on a transient failure would log the user out of
 * a working session — and for the same reason a 503 is just another null here,
 * not something to surface to a caller running on a healthy session.
 */
export async function mintAccessToken(): Promise<string | null> {
  const outcome = await refreshOnce();
  return outcome.status === 'ok' ? outcome.token : null;
}

/**
 * mintAccessToken plus clear-on-failure, for the API client's one-shot 401
 * retry. There the request already came back unauthorized, so a failed mint
 * usually means the session is genuinely dead and the stale token should go.
 *
 * The exception is a 503: auth-service is telling us it could not answer, not
 * that the answer is no. Rejecting rather than resolving null is what carries
 * that distinction outward with no change to packages/shared-ts — the rejection
 * propagates through `onRefresh` and out of `fetchAuthenticated`, so the
 * original request fails with this 503 instead of the 401 that was never the
 * real problem.
 */
export async function refreshAccessToken(): Promise<string | null> {
  const outcome = await refreshOnce();
  if (outcome.status === 'ok') return outcome.token;
  if (outcome.status === 'unavailable') {
    throw new ApiError(503, 'service_unavailable', 'Service temporarily unavailable');
  }
  clearAccessToken();
  return null;
}
```

Note on `mintAccessToken`: it now `await`s rather than returning `inflight` directly, but `refreshOnce()` is still evaluated synchronously on entry, so the dedupe behaviour is identical — the existing `dedupes concurrent callers onto one request` test proves it.

- [x] **Step 4: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/api
npm run -w packages/shared-ts test
```

Expected: PASS, including every pre-existing `refresh.test.ts` case. Those use `jsonResponse`, whose fake carries no `status` field, so `res.status === 503` is `undefined === 503` → `false` → `dead` — exactly what they already assert.

- [x] **Step 5: Prove the new tests can fail**

Change `if (res.status === 503) return { status: 'unavailable' };` to `if (res.status === 500) …`. Re-run `npm run -w apps/web test -- src/lib/api/refresh.test.ts` and confirm `keeps the stored token and rejects with a 503` goes red on `getAccessToken()` — the token assertion, not just the rejection. Restore.

- [x] **Step 6: Commit**

```bash
git add apps/web/src/lib/api/refresh.ts apps/web/src/lib/api/refresh.test.ts packages/shared-ts/src/apiClient.test.ts
git commit -m "feat(web): keep the session on a 503 refresh and surface it as a retryable error"
```

---

## Task 8: The login page says "try again in a moment"

**Files:**
- Modify: `apps/web/src/lib/auth/loginError.ts:1-25`
- Test: `apps/web/src/lib/auth/loginError.test.ts`

**Interfaces:**
- Consumes: the `#error=service_unavailable` fragment from Task 6.
- Produces: `LoginErrorCode` gains `'service_unavailable'`; `noticeFor('service_unavailable')` returns `{ tone: 'danger', message: 'Sign-in is temporarily unavailable. Nothing was saved — try again in a moment.' }`. `LoginPage.tsx` is not modified — it already renders whatever `noticeFor` returns.

**On the tone (design D7):** `tone` is not purely cosmetic in `LoginPage.tsx:44` — `const failed = notice?.tone === 'danger'` drives `role="alert"`, the danger band, and the primary button's label (`'Try again'` vs `'Continue with Google'`). Two of those three are exactly right for an outage: the user should be told sign-in failed, and the button should say "Try again". `'neutral'` is reserved for `cancelled`, a deliberate choice with no retry implied. Accepted cost: the band is red for something that is not the user's fault; a third tone is a design-system change the PRD puts out of remit.

- [x] **Step 1: Write the failing tests**

In `apps/web/src/lib/auth/loginError.test.ts`, add `'service_unavailable'` to the `it.each<LoginErrorCode>([...])` list in the `parses the %s code` test. Then append to the `noticeFor` describe block:

```ts
  // The one failure whose ADVICE differs: wait and retry, rather than try a
  // different Google account. That is why it does not reuse GENERIC_FAILURE —
  // unlike the invalid_state / auth_failed / server_error split, which exists
  // for log correlation and which the reader cannot act on differently.
  it('tells the user an outage is temporary rather than reusing the generic copy', async () => {
    const { noticeFor } = await freshModule('/login');

    expect(noticeFor('service_unavailable')).toEqual({
      tone: 'danger',
      message: 'Sign-in is temporarily unavailable. Nothing was saved — try again in a moment.',
    });
    // Distinct from the generic failure, which advises a different Google
    // account — the wrong thing to do during an outage.
    expect(noticeFor('service_unavailable').message).not.toBe(noticeFor('server_error').message);
  });

  // The union member alone is not enough: CODES is a hand-maintained
  // readonly string[], and isLoginErrorCode silently degrades anything missing
  // from it to server_error (loginError.ts:58). That degradation is what makes
  // shipping the backend ahead of the frontend safe — and what would hide this
  // mistake in review.
  it('accepts service_unavailable through the CODES allowlist', async () => {
    const { consumeLoginError } = await freshModule('/login#error=service_unavailable');
    expect(consumeLoginError()).toBe('service_unavailable');
  });
```

- [x] **Step 2: Run the tests to verify they fail**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/auth/loginError.test.ts
```

Expected: FAIL — a TypeScript error on `noticeFor('service_unavailable')` (not assignable to `LoginErrorCode`), and `expect(consumeLoginError()).toBe('service_unavailable')` receiving `'server_error'`.

- [x] **Step 3: Add the code, the allowlist entry, and the message**

In `apps/web/src/lib/auth/loginError.ts`:

```ts
/** The closed set auth-service redirects with (FR-ERR-4). Nothing else is valid. */
export type LoginErrorCode =
  | 'cancelled'
  | 'invalid_state'
  | 'auth_failed'
  | 'server_error'
  | 'service_unavailable';

export interface LoginErrorNotice {
  tone: 'neutral' | 'danger';
  message: string;
}

const CODES: readonly string[] = [
  'cancelled',
  'invalid_state',
  'auth_failed',
  'server_error',
  'service_unavailable',
];
```

and add the fifth `NOTICES` entry:

```ts
const NOTICES: Record<LoginErrorCode, LoginErrorNotice> = {
  // Cancelling is a choice, not a fault: neutral tone, no alarm (FR-STATE-5).
  cancelled: { tone: 'neutral', message: 'Sign-in cancelled.' },
  invalid_state: { tone: 'danger', message: GENERIC_FAILURE },
  auth_failed: { tone: 'danger', message: GENERIC_FAILURE },
  server_error: { tone: 'danger', message: GENERIC_FAILURE },
  // The one code that does NOT reuse GENERIC_FAILURE. Every other failure
  // advises trying a different Google account; during an outage that is the
  // wrong advice, and nothing about the user's account is at fault.
  //
  // tone: 'danger' rather than 'neutral' because LoginPage keys more than
  // colour off it — role="alert" and the "Try again" button label both hang on
  // tone === 'danger', and both are right here. 'neutral' is reserved for the
  // deliberate cancellation, and would leave an outage rendering as muted body
  // text under a "Continue with Google" button.
  service_unavailable: {
    tone: 'danger',
    message: 'Sign-in is temporarily unavailable. Nothing was saved — try again in a moment.',
  },
};
```

`NOTICES` is keyed on `LoginErrorCode`, so the compiler demands this entry the moment the union member is added — but `CODES` is hand-maintained and gets no such help, which is why the allowlist test above exists.

- [x] **Step 4: Run the tests to verify they pass**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- src/lib/auth/loginError.test.ts
npm run -w apps/web build
```

Expected: PASS, and a clean type-check.

- [x] **Step 5: Prove the allowlist test can fail**

Remove `'service_unavailable'` from `CODES` (leave the union member and the `NOTICES` entry). Re-run — `accepts service_unavailable through the CODES allowlist` goes red with `'server_error'`, and the parse test goes red too. This is exactly the silent degradation the risk table names. Restore.

- [x] **Step 6: Commit**

```bash
git add apps/web/src/lib/auth/loginError.ts apps/web/src/lib/auth/loginError.test.ts
git commit -m "feat(web): give a sign-in outage its own try-again-shortly notice"
```

---

## Task 9: Full verification, browser confirmation, and code review

No code is written in this task unless something below fails.

**Files:**
- Modify: `docs/tasks/task-018-transient-upstream-503/plan.md` (check off completed steps), and `audit.md` is produced by the reviewer agents.

- [x] **Step 1: Run the full CI gate**

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: PASS on every target — `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, `carfax-template`. Paste the failing output into the report if any target fails; do not proceed past a red gate.

- [x] **Step 2: Render both overlays and run both server dry-runs**

No manifest change is expected in this task — this is the standing gate from `CLAUDE.md`, and the `local` overlay is not exempt.

```bash
cd /home/tumidanski/source/MyFleet/.worktrees/task-018-transient-upstream-503
kustomize build deploy/k8s/overlays/main
kustomize build deploy/k8s/overlays/local
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Expected: both renders succeed; `main` shows no PersistentVolumeClaim, no Secret, no ClusterRole and no placeholder values; both dry-runs report only `(server dry run)` lines and no errors.

- [x] **Step 3: Confirm the two user-visible behaviours in a real browser**

`jsdom` cannot evaluate CSS, and both of these are things a unit test cannot see. Follow `docs/runbooks/local-debugging.md` to bring the stack up and drive real Chromium via the Playwright container.

With the stack up and a signed-in session, scale `fleet-service` to zero:

```bash
kubectl -n myfleet scale deploy/fleet-service --replicas=0
```

1. **Refresh path.** With the SPA open on an authenticated route, wait for (or force) an access-token refresh. Confirm: the app does **not** navigate to `/login`, the stored `access_token` in `localStorage` is still present, and the network tab shows `POST /api/auth/refresh` → `503` carrying `Retry-After: 5` and a `Set-Cookie: refresh_token=…` that is **not** the clearing form.
2. **Callback path.** Sign out, then sign in through Google. Confirm the browser lands on `/login#error=service_unavailable` (fragment stripped on arrival), the notice reads "Sign-in is temporarily unavailable. Nothing was saved — try again in a moment." in the danger band, and the primary button reads **Try again**.
3. **Recovery.** Scale `fleet-service` back up (`kubectl -n myfleet scale deploy/fleet-service --replicas=1`) and confirm the next refresh succeeds — the session was never revoked and the user was never signed out. This is acceptance criterion 4 confirmed end to end.

Record what was observed (including screenshots if the runbook's flow produces them) in the task folder.

- [x] **Step 4: Run the three reviewer agents**

Per `CLAUDE.md`, code review runs before the PR and is not skipped even when the plan looks complete. Invoke `superpowers:requesting-code-review`, which dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer` (Go changed) and `frontend-guidelines-reviewer` (TS changed) in parallel. Findings land in `docs/tasks/task-018-transient-upstream-503/audit.md`.

Address every finding, or record an explicit adjudication for any that is declined.

- [x] **Step 5: Walk the acceptance criteria and check them off**

Open `prd.md` §10 and confirm each box against real evidence — a test name, a command's output, or a browser observation. The two that are easiest to fool yourself about:

- "does not clear the refresh cookie" — proven by `TestRefresh_transientResolverFailureKeepsTheSessionAlive`'s `refreshCookieSet` assertions, not by the status code.
- "does not trip reuse detection" — proven by the `proc.Rotate(c.Value)` call at the end of that same test, which asserts the store's own state.

- [x] **Step 6: Commit the verification record**

```bash
git add docs/tasks/task-018-transient-upstream-503/
git commit -m "docs(task-018): record the review findings and browser verification"
```

---

## Self-Review

**Spec coverage.** Every PRD requirement maps to a task:

| Requirement | Task |
| --- | --- |
| FR-CLASSIFY-1/2/3/5 | 3 |
| FR-CLASSIFY-4 (`404` contract, guard test unmodified) | 3, 4 (Global Constraints) |
| FR-CLASSIFY-6 (no widening) | 3 (transport unwrap, fixed texts), 4 (driver text dropped) |
| FR-CLASSIFY-7 (`errors.Is` through the resolver) | 4 |
| FR-CLASSIFY-8 (`ErrNotFound` stays permanent) | 4 |
| FR-TIMEOUT-1/2 | 3 |
| FR-SHARED-1/2 | 1 |
| FR-SHARED-3 | 2 |
| FR-SHARED-4 | 1, 2, 5 |
| FR-SHARED-5 | 1 |
| FR-SHARED-6 (nothing existing changes) | 1, 2 (full-package runs) |
| FR-REFRESH-1/2/3/5/6 | 5 |
| FR-REFRESH-4/7 (other paths unchanged) | 5 |
| FR-CALLBACK-1/2/3/4 | 6 |
| FR-SPA-1/2 | 8 |
| FR-SPA-3/4/7 | 7 |
| FR-SPA-5/6 | 7 (`apiClient.test.ts`, no source change) |
| Design D9 (local infra) | 4 |
| Design D11 (query escaping) | 3 |
| Acceptance criteria, `make ci`, overlays, browser, review | 9 |

**Design decisions where the plan is more specific than the design, and why:**

1. **D5 says `const`; the plan uses `var fleetLookupTimeout`.** The design's own testing section permits "the constant temporarily lowered" — a `var` is what makes that possible without a build tag or a five-second test. Still one literal `5 * time.Second` in the package, still unexported, never written in production. Documented in the declaration's comment.
2. **D4's transport wrap.** The design notes the `hc.Do` path is where an upstream-influenced string enters the chain and accepts it as pre-existing. The plan unwraps `*url.Error` to the transport error underneath first, because this request's URL carries `user_id` as a query parameter and FR-CLASSIFY-6 forbids the user id riding into a log line. Four lines, keeps the diagnostic ("connection refused", "no such host", "context deadline exceeded"), drops the address. Test asserts it.
3. **D9's dropped driver text.** Followed exactly as written — `%w` on the sentinel, our own fixed text, the driver error not in the chain. Accepted cost, stated here so it is not rediscovered in review: a local DB fault reaches the log as "service unavailable: user lookup failed" without the SQLSTATE. The database layer and the pod's own logs carry the detail.
