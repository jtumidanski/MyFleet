# DDD guideline fixes — `apps/fleet-service/internal/invite`

Fixes for the blocking DOM-* findings in `audit-backend.md`. One commit per
finding; all five are independent and readable on their own.

| Finding | Commit | Subject |
|---|---|---|
| DOM-18 | `529e36c` | `refactor(fleet-service): name the invite create request type` |
| DOM-01 | `aad0e8c` | `refactor(fleet-service): enforce invite invariants in Builder.Build` |
| DOM-10 | `33aa3b4` | `refactor(fleet-service): thread request context through the invite domain` |
| DOM-12/14 | `75c0b4a` | `refactor(fleet-service): move invite business logic behind the processor` |
| DOM-07 | `ad83081` | `fix(fleet-service): carry the correlation id on every invite error log` |

`755f3e7` (`feat(shared-go/server): log 5xx faults…`) is interleaved in the log;
it belongs to the concurrent `packages/shared-go/server` agent, not to this work.

---

## DOM-18 — inline anonymous request struct

**Guideline clause.** `file-responsibilities.md:116` — `rest.go` defines the
request models; `ai-guidance.md:255` — flat named request types, not anonymous
structs decoded inline.

**Changed.**
- `rest.go:27-37` — new `CreateRequest{Email, Role}` with the same JSON tags.
- `resource.go:45` — `server.RegisterInputHandler(func(w, req, attrs CreateRequest))`
  replaces the five-line anonymous struct in the handler signature.

**Behaviour.** None. Field names and JSON tags are identical, so the decoded
wire format is byte-for-byte the same.

**Covering tests.** `resource_test.go` `TestCreateInvite_perFleetLimitIs429`,
`TestCreateInvite_malformedEmailIs422`, `TestCreateInvite_unknownRoleIs422` all
POST a real JSON:API envelope through `RegisterInputHandler`; a tag or shape
change breaks them.

---

## DOM-01 — `Build()` enforced no invariants

**Guideline clause.** `file-responsibilities.md:24` "`Build()` enforces
invariants"; `ai-guidance.md:170` builder checklist item;
`scaffolding-checklist.md:139`.

**Changed.**
- `builder.go:11-28` — doc comment stating each invariant and why it is one.
- `builder.go:47-68` — `Build() (Model, error)`, matching `fleet.Builder` and
  `fuel.Builder`. Rejects a blank `fleetID`, `email`, `role`, `token` or
  `invitedByUserID`, and a zero `expiresAt`, with `server.ErrValidation`.
- `builder.go` — the unexported `setAcceptedAt` / `setUpdatedAt` test-only
  setters are deleted; their last callers are gone and the `unused` linter is
  enabled.
- `processor.go:271-283` (Create) — the only production call site, error checked.

**Choice of invariants.** Every field validated is `NOT NULL` in
`fleet.fleet_invites` (`entity.go:10-21`) and load-bearing at runtime: a blank
token collides with every other blank token on the accept route, a blank
`fleetID` would mint a membership in nothing, a blank role is copied verbatim
onto a membership no authz gate understands.

**What I deliberately did NOT do.** `Build()` does not re-check email *syntax*.
`ValidateInviteEmail` (`processor.go:151-159`) owns the bare-addr-spec rule that
makes the address safe to interpolate into a `To:` header; duplicating it in the
builder would give the domain two sources of truth that can drift, and the
audit's own note asks for exactly this carve-out. `Build` asserts only that an
address is present. `TestBuild_doesNotRevalidateEmailSyntax` pins the split in
both directions.

`Make(Entity)` (`entity.go:28`) is untouched and still does not validate. That
is required, not an oversight: a corrupt row that predates this check must
remain readable so the domain can reject it deliberately via `ErrInviteUnusable`
rather than failing to load at all.

**Test fixtures.** Fixtures that need a corrupt or hand-stamped row now build
through `Make` instead of the builder — which is how such a row reaches the code
under test in production anyway, since `Build` can no longer produce one:
- `processor_test.go:65-67` `mk` → `Make(Entity{...})`
- `processor_test.go:160,165` cooldown inputs → `Make(Entity{UpdatedAt: …})`
- `resource_test.go` `seedInvite` → `Make(Entity{...})`
- `pending_test.go` the accepted/expired pair → `Make(Entity{...})`
- `administrator_test.go` `newInvite` gains `t *testing.T` and fails the test on
  a build error (a fixture that trips an invariant is a bug in the fixture).

**No assertion was changed.** Only construction and call sites.

**Covering tests.** New `builder_test.go`:
`TestBuild_acceptsACompleteInvite`,
`TestBuild_rejectsAnInviteMissingAnInvariant` (six sub-cases, one per field),
`TestBuild_doesNotRevalidateEmailSyntax`.
The pre-existing blank-email tests
(`TestValidateAccept_rejectsAnInviteWithNoEmail`,
`TestPendingRoute_returnsNothingForAnEmptyEmailClaim`,
`TestAcceptRoute_rendersADistinctDetailPerPrecondition`'s "invite row has no
email" case) still pass unmodified.

---

## DOM-10 — providers held a startup `*gorm.DB` with no `WithContext`

**Guideline clause.** `testing-guide.md:213` "Verify providers use
`db.WithContext(ctx)` not bare `db`"; `file-responsibilities.md:36,82` —
"Always use `p.db.WithContext(p.ctx)`"; SKILL.md Key Principle 3, "Context
Propagation — request context is always passed explicitly".

**Changed.**
- `provider.go:16-32` — `context.Context` is the first parameter of all five
  `Provider` methods, with a comment on why.
- `provider.go:38,52,74,88,100` — every query runs on `p.db.WithContext(ctx)`.
- `provider.go:38-104` — reads adopt `database.Query` / `database.SliceQuery`,
  the lazy wrapper `auth-service/internal/user/provider.go` and
  `media-service/internal/mediavariant/provider.go` already use.
- `provider.go:108-114` — `makeAll` helper replaces the two duplicated
  entity→model loops.
- `administrator.go:14-30` — same treatment on the four write methods;
  `:76,95,158,168` open on `a.db.WithContext(ctx)`.
- `processor.go:26,43,51,60,145` — `ListByFleet`, `ListRedeemableForEmail`,
  `GetByID`, `GetByToken`, `CheckCreateLimit` take and forward `ctx`.
  `ValidateAccept` and `CheckResendCooldown` stay pure and take none.
- `resource.go` (all six handlers) and `internal.go:47` pass `req.Context()`.

**No `context.TODO()` or `context.Background()` appears in the production path** —
verified by `grep -rn "context.TODO\|context.Background" internal/invite/*.go`
matching only `_test.go` files.

**Scope note (deliberate extension).** The audit named only `Provider`. I
included `Administrator` as well: it holds the same startup handle, and a write
transaction outliving its request is the worse of the two failure modes. Both
are inside the assigned scope (`internal/invite`).

**What I did NOT do.** `OwnerChecker.RequireOwnerInFleet` and
`FleetNamer.GetByID` are satisfied by the `membership` and `fleet` packages;
threading `ctx` through them means changing those packages' signatures, which is
outside the assigned scope. They remain context-free and are flagged below.

**Behaviour.** None. Same SQL, same errors, same ordering. Test-only changes are
signature updates: `stubProvider`'s methods take an ignored context, and direct
administrator/provider calls in tests pass `context.Background()`.

**Covering tests.** The whole package — every HTTP test drives a real
`*http.Request`, so a lost or wrong context breaks at compile time; the sqlite
DB tests exercise every `WithContext`ed query and transaction.

---

## DOM-12 / DOM-14 — business logic in the HTTP layer

**Guideline clause.** `file-responsibilities.md:99` "Delegate ALL business logic
to processors"; `:106-108` `resource.go → processor.go → administrator.go`;
`ai-guidance.md:179` "No cross-domain business logic in handlers";
`anti-patterns.md:14`.

**Changed — logic moved into `processor.go`:**

| Was (handler) | Now |
|---|---|
| `membership.IsValidRole` role check | `Processor.Create` (`processor.go:246-249`) |
| `ValidateInviteEmail` | `Processor.Create` (`:252-254`) |
| create rate-limit call | `Processor.Create` (`:257-259`) |
| `generateToken()` | `Processor.Create` / `Processor.Resend`; the function itself moved to `processor.go:340-352` |
| `time.Now().Add(defaultExpiry)` | `Processor.Create` / `Processor.Resend` |
| `NewBuilder()…Build()` | `Processor.Create` (`:266-278`) |
| `inv.AcceptedAt() != nil → 409` | `Processor.Resend` (`:296-299`) |
| cooldown call | `Processor.Resend` (`:300-304`) |
| `adm.Insert` / `adm.Resend` / `adm.Accept` / `adm.Delete` | `Processor.Create/Resend/Accept/Delete` |

`defaultExpiry` and `Limits` moved to `processor.go:19-30` with the logic that
uses them. The administrator is now reachable only through the processor, which
takes it via `WithAdministrator` (`processor.go:53-56`) — `internal.go`'s
read-only processor needs none, the same optional-collaborator shape
`dbAdministrator` uses for its emitters.

**Handlers now.** Authorize → call one domain operation → render. `resource.go`
contains no `time.Now()`, no `NewBuilder`, no `generateToken`, no `adm.*`.

**Behaviour explicitly preserved.**
- **Check order is unchanged in every handler.** Create still validates role
  before address (both return `server.ErrValidation` with identical bodies, so
  the pair is unobservable regardless) and still checks the rate limit BEFORE
  minting a token, so a throttled request costs no entropy and no write.
- **Resend still reads the invite FIRST, before authz.** I considered hoisting
  the authz block above the read — it needs only the path `fleetId` — but that
  would turn "nonexistent invite under a fleet the caller does not hold" from
  404 into 403. Preserving status codes outranks a thinner handler, so the read
  stays first and the resend handler makes two processor calls
  (`GetByID`, then `Resend`) rather than one.
- **Cross-fleet resend is still 404**, byte-identical to a nonexistent id
  (`resource.go:249-252`).
- **Accepted 409 is still raised before the cooldown 429**
  (`processor.go:296-304`).
- **`Resend` still computes ONE `now`**, uses it as the cooldown clock and hands
  it to the administrator explicitly, so the returned `updated_at` is provably
  the persisted value the next `CheckResendCooldown` reads. `now` is computed in
  the processor rather than the handler; the property is between processor and
  administrator and is unchanged.
- **`accepted_at IS NULL` guard, `RowsAffected` check and the single-transaction
  outbox enqueues** in `administrator.go` are untouched.
- **The four accept sentinels** still reach the handler unchanged, so the
  per-precondition 409 detail strings and the per-case log split still work.

**Covering tests.** Nine new processor tests over a stub administrator
(`processor_test.go`): role and address rejection without writing, throttled
create writes nothing, fresh 64-hex token and expiry per invite, the model
handed to the administrator is the one returned,
`TestProcessorResend_reportsAcceptedBeforeCooldown` (updated_at deliberately
fresh, so a wrong order would produce 429 not 409),
`TestProcessorResend_passesOneNowForBothUpdatedAtAndExpiry`,
`TestProcessorResend_insideTheCooldownWritesNothing`,
`TestProcessorAccept_rejectsAMismatchWithoutWriting`,
`TestProcessorAccept_writesWhenThePreconditionsHold`,
`TestProcessorErrors_neverCarryTheToken`.
Two new HTTP tests (`resource_test.go`):
`TestResendInvite_acceptedIs409EvenInsideTheCooldown`,
`TestCreateInvite_unknownRoleIs422` (also asserts no row was written).
All pre-existing HTTP tests — cross-fleet 404 body identity, 429s, 422, the
four accept details, FR-10 disclosure — pass unmodified.

---

## DOM-07 — error logging in `resource.go`

**State on arrival.** The audit predates `b07e970`, which had already added most
of the logging. Three gaps remained, plus one consistency problem.

**Guideline clause.** `file-responsibilities.md:101` "Log errors with context";
`ai-guidance.md:220`.

**Changed.**
- `internal.go:43` — derives `correlationID`; `:57-60` and `:70-74` now carry it.
  This was the audit's explicit complaint (`internal.go:53,63` logged with the
  startup logger and no correlation id).
- `resource.go:99` (list), `:161` (pending), `:186` (delete) — derive
  `correlationID`; the list, delete, delete-owner-check and per-row
  pending-fleet warn lines now carry it (`:107-110`, `:180-183`, `:196-200`,
  `:213-217`, `:224-228`).
- Field key normalised from `trace_id` to `correlation_id` throughout, matching
  `packages/shared-go/auth/middleware.go:59`. Previously a single package used
  both keys, so no one query found all its lines.
- The handler variable is now named `correlationID` and is used for both the log
  field and the outbox envelope's trace id — they are the same value, which is
  what lets a mail that never arrived be walked back to the request.

**Routine 4xx stay unlogged**, per the brief: validation, 403, 404, the
already-accepted/expired 409s and both 429s produce no line. The email-mismatch
`Warn` and the corrupt-row `Error` are kept — they are the two the audit called
out as worth being greppable.

**Value beyond the concurrent `WriteError` change.** `755f3e7` added a generic
5xx record ("request failed; error text redacted from the response body",
`status=500`). These lines say *which* invite, in *which* fleet, on *which*
request — `invite_id`, `fleet_id`, `user_id`, `correlation_id`.

**Token discipline.** No log message or field anywhere in the package carries
the token. `Processor.generateToken`'s doc comment states the rule at the point
it is minted.

**Covering tests.** Two new mechanical guards in `resource_test.go`:

- `TestInviteHandlers_neverLogTheTokenAndAlwaysCarryTheCorrelationID` — mounts
  every route (public + internal) behind `telemetry.CorrelationID`, mints a real
  invite, drops the table out from under the handlers to force the
  unexpected-error branches, drives all six routes plus accept-with-the-real-
  token-in-the-URL, then asserts the token appears nowhere in the captured
  output and that EVERY emitted line carries a non-empty `correlation_id`.

  The capture is scanned as serialized JSON **text**, not field-by-field. This is
  deliberate and is the fix for the flaw the audit recorded as C-2 in
  `mailconsumer/consume_test.go:382`: a field-by-field assertion has to
  stringify each value, and anything it does not know how to render silently
  reads as empty, so a token embedded in a struct or a `fmt.Stringer` passes.
  Bytes cannot be fooled that way.

- `TestAcceptRoute_mismatchLogDisclosesNoAddressButKeepsTheCorrelationID` — pins
  the rejection line most likely to grow an address: it must be greppable, must
  carry `invite_id` and `correlation_id`, and must name neither the invited nor
  the authenticated address (PRD FR-10/§8).

**Both guards were confirmed non-vacuous.** Reintroducing the leak
(`.WithField("leak", token)` on the accept branch) fails the first with "the
invite token reached the logs"; renaming the list handler's `correlation_id` key
to `cid` fails it with "log line carries no correlation_id". Both were reverted
immediately after the check.

---

## Verification

All commands run from
`/home/tumidanski/source/MyFleet/.worktrees/task-009-smtp-invite-delivery`.

```
$ go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
BUILD+VET OK          # both silent, exit 0
```

```
$ go test github.com/jtumidanski/myfleet/apps/fleet-service/... -race -count=1
ok  github.com/jtumidanski/myfleet/apps/fleet-service/cmd                              1.102s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/activity                1.021s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/authz                   1.014s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/dashboard               1.021s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/events                  1.031s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet                   1.024s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/fuel                    1.022s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite                  1.083s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancecategory     1.057s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenancerecord       1.034s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/maintenanceschedule     1.028s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/mediaclient             1.034s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership              1.019s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/mileage                 1.019s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/status                  1.107s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle                 1.123s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehiclemedia            1.113s
```

```
$ go test .../internal/invite/... -race -count=1 -v | grep -c '^--- PASS'
55                    # 0 failures, 0 skips
```

```
$ export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && make ci
… lint-check: 0 issues (×6 modules)
… go vet, go test, go build: clean
… fe-test, fe-build: pass (vite built in 3.64s)
… manifest checks passed
… carfax template check passed
make ci EXIT=0
```

No test dials a socket or starts a container; the DB tests use in-memory sqlite
with the existing `ATTACH DATABASE ':memory:' AS fleet` workaround.

---

## Things I chose not to do

1. **`OwnerChecker` / `FleetNamer` remain context-free.** Threading `ctx` into
   `RequireOwnerInFleet` and `fleet.Provider.GetByID` means changing the
   `membership` and `fleet` packages, outside the assigned scope. Those two
   calls still run on a bare connection. Worth a follow-up.

2. **Resend still makes two processor calls.** See DOM-12 above — hoisting authz
   above the read would change a 404 to a 403 for one input combination, and
   preserving status codes was ranked higher.

3. **`Make` still returns `Model`, not `(Model, error)`** (audit DOM-03,
   non-blocking). Changing it would touch every read path in the package for no
   behavioural gain, and the current shape is what lets the corrupt-row guard be
   testable at all.

4. **`server.WriteError`'s raw-error echo (SEC-09) not touched** — that is
   `packages/shared-go/server`, being changed concurrently by another agent.
   Note that `755f3e7` on this branch already redacts error text from the
   response body, which addresses it.

5. **SEC-10 (token in the URL path) not touched** — non-blocking, and a route
   redesign well outside these findings.

6. **No assertion in any existing test was changed**, and no test was weakened,
   skipped or deleted. The only edits to existing tests are call-site signature
   updates and fixture construction moved from `Builder` to `Make`. I found no
   assertion I believe is now wrong.

7. **No migrations, no new columns.**
