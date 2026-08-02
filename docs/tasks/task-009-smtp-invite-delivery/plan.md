# SMTP Invite Email Delivery — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a fleet invite is created or resent, the invitee receives an email containing a working accept link, delivered asynchronously through an authenticated SMTP relay.

**Architecture:** fleet-service emits a new `invite.created` event into its transactional outbox on the same tx as the invite write. A **new, independent** consumer in notification-service (`mailconsumer`, its own Kafka group `invite-email`) picks it up, fetches the invite — including its token — from a new network-restricted fleet-service endpoint, renders a `multipart/alternative` message, and hands it to an SMTP relay behind a narrow `mailer.Sender` interface. The token never travels over Kafka and never reaches a log line. The HTTP invite request never waits on the relay.

**Tech Stack:** Go 1.25 (`go.work` workspace), GORM, chi, kafka-go, `net/smtp` + `crypto/tls`, `html/template` + `text/template` with `go:embed`, prometheus `promauto`; React 19 / TypeScript / TanStack Query / vitest; kustomize + Mailpit for local dev.

## Global Constraints

- **The invite token is a bearer credential.** It must not appear in a Kafka payload, a log line at any level (including debug and error paths), an email subject, or any error string. Design §6.6.
- **Config is read through `packages/shared-go/config`** (`Get` / `MustGet` / `GetInt`) — never `os.Getenv` in handlers. FR-CFG-3.
- **No database migrations and no new columns.** PRD §6. `fleet_invites.updated_at` already exists (`entity.go:20`); the ledger reuses `notification.processed_events` with a new consumer name.
- **No test may dial a socket, start a container, or touch a relay.** `SMTP_ENABLED` is unset in CI; every mailer/mailconsumer test uses `mailer.FakeSender`. FR-DEV-4.
- **TLS verification is never disabled in committed config or code.** There is no skip-verify flag. FR-MAIL-2.
- **`SMTP_TLS_MODE` values are exactly `starttls` | `tls` | `none`**, validated against that set at startup. `none` must never render into the `main` overlay.
- **The `main` overlay must render with no PersistentVolumeClaims, no Secrets, no ClusterRole, and no `REPLACE_ME` placeholders.** `tools/check-manifests.sh` enforces this and runs in `make ci`.
- **Rate-limit defaults:** `INVITE_RATE_LIMIT_PER_DAY=20`, `INVITE_RESEND_COOLDOWN_SECONDS=300`.
- **`member.invited` semantics are unchanged.** Do not rename it, do not emit it at creation, do not attach email sending to it. FR-EVT-5.
- **Go tests run from the repo root** against full package paths, e.g.
  `go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestName -v`.
- **Node may not be on `PATH`.** Before any `npm` command:
  `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22`

---

## File Structure

**Created**

| Path | Responsibility |
|---|---|
| `apps/fleet-service/internal/invite/internal.go` | network-restricted `GET /internal/invites/{inviteID}` + `FleetNamer` seam |
| `apps/fleet-service/internal/invite/internal_test.go` | route-tree assertion (FR-INT-4) + internal handler tests |
| `apps/fleet-service/internal/invite/administrator_test.go` | transactional insert / resend rollback + rotation, sqlite |
| `apps/notification-service/internal/mailer/sender.go` | `Sender`, `Message`, `PermanentError`, `Config` |
| `apps/notification-service/internal/mailer/config.go` | `ConfigFromEnv` + startup validation |
| `apps/notification-service/internal/mailer/template.go` | embedded templates, `InviteData`, `RenderInvite` |
| `apps/notification-service/internal/mailer/templates/invite.html.tmpl` | HTML part |
| `apps/notification-service/internal/mailer/templates/invite.txt.tmpl` | plain-text part |
| `apps/notification-service/internal/mailer/compose.go` | `Message` → RFC 5322 `multipart/alternative` bytes |
| `apps/notification-service/internal/mailer/smtp.go` | `smtpSender`: STARTTLS / implicit TLS / plaintext-local |
| `apps/notification-service/internal/mailer/fake.go` | `FakeSender` (exported — `mailconsumer` tests use it) |
| `apps/notification-service/internal/mailer/metrics.go` | `myfleet_invite_emails_total` CounterVec + `RecordOutcome` |
| `apps/notification-service/internal/mailconsumer/consume.go` | one `invite.created` → one email, idempotently |
| `apps/notification-service/internal/mailconsumer/consume_test.go` | idempotency, skips, retry schedule, no-token-in-logs |
| `apps/web/src/lib/utils/clipboard.ts` | `copyToClipboard` with non-secure-context fallback |
| `apps/web/src/lib/utils/clipboard.test.ts` | both clipboard paths |
| `apps/web/src/components/features/settings/InviteList.test.tsx` | copy + resend controls |
| `deploy/k8s/infra-local/mailpit.yaml` | dev-only Mailpit Deployment + Service |

**Modified**

| Path | Change |
|---|---|
| `packages/shared-go/server/errors.go` `server.go` `errors_test.go` | add 429 |
| `packages/dto-go/events/payloads.go` `payloads_test.go` | `InviteCreatedData` |
| `apps/fleet-service/internal/events/emit.go` `emit_test.go` | `EmitInviteCreated` |
| `apps/fleet-service/internal/invite/{model,entity,builder,provider,processor,administrator,resource,rest}.go` | see context.md §2 |
| `apps/fleet-service/internal/invite/processor_test.go` | extend `stubProvider`; validation + limit tests |
| `apps/fleet-service/cmd/main.go` | emit adapter, internal initializer, limit config |
| `apps/notification-service/internal/fleetclient/client.go` | `Invite()`, typed `*statusError` |
| `apps/notification-service/cmd/main.go` | mailer construction + mail consumer goroutine |
| `apps/notification-service/go.mod` `go.sum` `go.work.sum` | prometheus becomes a direct dep |
| `deploy/k8s/base/notification-service/configmap.yaml` | SMTP + `PUBLIC_WEB_URL` |
| `deploy/k8s/secrets.example.yaml` | `SMTP_USERNAME` / `SMTP_PASSWORD` |
| `deploy/k8s/infra-local/kustomization.yaml` | Mailpit resource entry |
| `deploy/k8s/overlays/local/kustomization.yaml` | ConfigMap patch → Mailpit |
| `deploy/compose/docker-compose.yml` | Mailpit service + notification-service env |
| `tools/check-manifests.sh` | `SMTP_TLS_MODE=none` and `mailpit` must not reach `main` |
| `apps/web/src/services/api/InviteService.ts` | `resendInvite` |
| `apps/web/src/lib/hooks/api/invites.ts` | `useResendInvite`, `inviteErrorMessage` |
| `apps/web/src/components/features/settings/InviteList.tsx` | copy-link + resend controls |
| `apps/web/src/components/features/settings/InviteForm.tsx` | 429 message on create |

---

## Task 1: 429 in the shared error taxonomy

`packages/shared-go/server` has no 429 today. Every later rate-limit task depends on this.

**Files:**
- Modify: `packages/shared-go/server/errors.go:5-15` and `:17-40`
- Modify: `packages/shared-go/server/server.go:12-35`
- Test: `packages/shared-go/server/errors_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.ErrTooManyRequests error` — mapped to HTTP 429 by `server.StatusFor`, rendered with JSON:API code `"too_many_requests"` by `server.WriteError`.

- [ ] **Step 1: Add the 429 rows to the existing test tables**

`errors_test.go` already table-tests every status and every code. Add one row to each map.

In `TestStatusFor_mapsDomainErrors`, add to the `cases` map:

```go
		ErrTooManyRequests:       429,
```

In `TestCodeFor_namesEveryMappedStatus`, add to the `cases` map:

```go
		429: "too_many_requests",
```

- [ ] **Step 2: Run the tests to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/packages/shared-go/server/... -run 'TestStatusFor_mapsDomainErrors|TestCodeFor_namesEveryMappedStatus' -v
```

Expected: FAIL to compile — `undefined: ErrTooManyRequests`.

- [ ] **Step 3: Add the sentinel and both mappings**

In `errors.go`, add to the `var (...)` block after `ErrValidation`:

```go
	ErrTooManyRequests       = errors.New("too many requests")        // 429
```

In `errors.go`, add to `StatusFor` before the `default:`:

```go
	case errors.Is(err, ErrTooManyRequests):
		return 429
```

In `server.go`, add to `codeFor` before the `default:`:

```go
	case 429:
		return "too_many_requests"
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/packages/shared-go/server/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add packages/shared-go/server/errors.go packages/shared-go/server/server.go packages/shared-go/server/errors_test.go
git commit -m "feat(shared-go): add ErrTooManyRequests (429) to the error taxonomy"
```

---

## Task 2: `invite.created` event payload and emitter

**Files:**
- Modify: `packages/dto-go/events/payloads.go` (after `MemberInvitedData`, line 39)
- Test: `packages/dto-go/events/payloads_test.go`
- Modify: `apps/fleet-service/internal/events/emit.go` (after `EmitMemberInvited`, line 70)
- Test: `apps/fleet-service/internal/events/emit_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `dtoevents.InviteCreatedData struct { InviteID string \`json:"invite_id"\`; Email string \`json:"email"\`; Role string \`json:"role"\` }`
  - `fleetevents.EmitInviteCreated(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.InviteCreatedData) error`

`InviteCreatedData` is structurally identical to `MemberInvitedData` but stays a **distinct type**:
aliasing them would let a field added for one silently change the other's wire format, and the two
events mean opposite things (FR-EVT-5).

- [ ] **Step 1: Write the failing payload test**

Append to `packages/dto-go/events/payloads_test.go`:

```go
// The token is a bearer credential and must never ride the event bus (FR-EVT-3).
// Pinning the exact JSON is what makes an accidentally-added Token field fail here.
func TestInviteCreatedData_jsonTags(t *testing.T) {
	b, _ := json.Marshal(InviteCreatedData{InviteID: "i1", Email: "a@b.com", Role: "member"})
	if string(b) != `{"invite_id":"i1","email":"a@b.com","role":"member"}` {
		t.Fatalf("unexpected json: %s", b)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/packages/dto-go/events/... -run TestInviteCreatedData_jsonTags -v
```

Expected: FAIL to compile — `undefined: InviteCreatedData`.

- [ ] **Step 3: Add the payload type**

Append to `packages/dto-go/events/payloads.go`, after `MemberInvitedData`:

```go
// InviteCreatedData is the payload of invite.created, emitted when an invite row
// is created or its token is rotated by a resend. Deliberately NOT an alias of
// MemberInvitedData: the two events mean opposite things (created vs accepted),
// and aliasing would let a field added for one change the other's wire format.
//
// It carries NO token. The token is a bearer credential; the email consumer
// fetches it over internal HTTP instead (design §2).
type InviteCreatedData struct {
	InviteID string `json:"invite_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}
```

- [ ] **Step 4: Run it to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/packages/dto-go/events/... -v
```

Expected: PASS.

- [ ] **Step 5: Write the failing emitter test**

Append to `apps/fleet-service/internal/events/emit_test.go`:

```go
// TestEmitInviteCreated mirrors TestEmitVehicleCreated and adds the assertion
// that matters most for this event: the serialized payload contains no "token"
// key, at any nesting level (FR-EVT-3).
func TestEmitInviteCreated(t *testing.T) {
	db := newOutboxDB(t)

	data := dtoevents.InviteCreatedData{InviteID: "inv-1", Email: "a@b.com", Role: "member"}
	err := db.Transaction(func(tx *gorm.DB) error {
		return EmitInviteCreated(tx, "fleet-1", "user-1", "trace-1", data)
	})
	if err != nil {
		t.Fatalf("EmitInviteCreated: %v", err)
	}

	var rows []sharedevents.OutboxRow
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 outbox row, got %d", len(rows))
	}
	row := rows[0]
	if row.Type != "invite.created" {
		t.Fatalf("type=%q want invite.created", row.Type)
	}
	if row.SentAt != nil {
		t.Fatalf("sent_at must be NULL on enqueue, got %v", row.SentAt)
	}

	if strings.Contains(strings.ToLower(string(row.Payload)), "token") {
		t.Fatalf("payload must not mention a token, got %s", row.Payload)
	}

	var env sharedevents.Envelope
	if err := json.Unmarshal(row.Payload, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.FleetID != "fleet-1" || env.ActorUserID != "user-1" || env.TraceID != "trace-1" {
		t.Fatalf("envelope fields mismatch: %+v", env)
	}
	dataBytes, _ := json.Marshal(env.Data)
	var decoded dtoevents.InviteCreatedData
	if err := json.Unmarshal(dataBytes, &decoded); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if decoded != data {
		t.Fatalf("data=%+v want %+v", decoded, data)
	}
}
```

Add `"strings"` to that file's import block.

- [ ] **Step 6: Run it to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/events/... -run TestEmitInviteCreated -v
```

Expected: FAIL to compile — `undefined: EmitInviteCreated`.

- [ ] **Step 7: Add the emitter**

Append to `apps/fleet-service/internal/events/emit.go`:

```go
// EmitInviteCreated enqueues an invite.created event. Emitted when an invite row
// is created AND when a resend rotates its token — a resend produces a fresh
// event_id, which is what lets it past the consumer's (event_id, consumer)
// ledger (FR-EVT-4). Distinct from member.invited, which fires on ACCEPT.
func EmitInviteCreated(tx *gorm.DB, fleetID, actorID, traceID string, d dtoevents.InviteCreatedData) error {
	return enqueue(tx, "invite.created", fleetID, actorID, traceID, d)
}
```

- [ ] **Step 8: Run it to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/events/... -v
```

Expected: PASS.

- [ ] **Step 9: Commit**

```sh
git add packages/dto-go/events/payloads.go packages/dto-go/events/payloads_test.go apps/fleet-service/internal/events/emit.go apps/fleet-service/internal/events/emit_test.go
git commit -m "feat(events): add invite.created payload and emitter"
```

---

## Task 3: Invite email address validation

Today the only check is `attrs.Email == ""` (`resource.go:66`). A CR/LF in an address is a header
injection; a `Name <addr>` form makes the `To:` header carry an attacker-influenced display name.

**Files:**
- Modify: `apps/fleet-service/internal/invite/processor.go`
- Modify: `apps/fleet-service/internal/invite/resource.go:66`
- Test: `apps/fleet-service/internal/invite/processor_test.go`

**Interfaces:**
- Consumes: `server.ErrValidation` (existing).
- Produces: `invite.ValidateInviteEmail(s string) error` — package-level function, returns
  `server.ErrValidation` or nil.

- [ ] **Step 1: Write the failing test**

Append to `apps/fleet-service/internal/invite/processor_test.go`:

```go
func TestValidateInviteEmail(t *testing.T) {
	valid := []string{"b@x.com", "first.last+tag@sub.example.co.uk"}
	for _, s := range valid {
		if err := ValidateInviteEmail(s); err != nil {
			t.Fatalf("ValidateInviteEmail(%q) = %v, want nil", s, err)
		}
	}

	// Every one of these is a header-injection or display-name vector, or is
	// simply unsendable. mail.ParseAddress ACCEPTS "Bob <b@x.com>", which is
	// exactly why the addr-spec equality check below it exists.
	invalid := []string{
		"",
		"b@x.com\r\nBcc: victim@x.com",
		"b@x.com\nBcc: victim@x.com",
		"Bob <b@x.com>",
		"not-an-address",
		"@x.com",
		"b@",
	}
	for _, s := range invalid {
		if err := ValidateInviteEmail(s); !errors.Is(err, server.ErrValidation) {
			t.Fatalf("ValidateInviteEmail(%q) = %v, want ErrValidation", s, err)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestValidateInviteEmail -v
```

Expected: FAIL to compile — `undefined: ValidateInviteEmail`.

- [ ] **Step 3: Implement it**

Append to `apps/fleet-service/internal/invite/processor.go`:

```go
// ValidateInviteEmail enforces that an invite address is a bare addr-spec safe
// to interpolate into a To: header (PRD §8 Security).
//
// CR/LF is checked first and separately so a header-injection attempt fails for
// an unambiguous reason. The a.Address != s comparison is the important half:
// mail.ParseAddress happily accepts "Bob <b@x.com>", whose display name would
// then be attacker-influenced header content. Requiring the input to equal the
// parsed addr-spec makes the header value a closed set of characters.
func ValidateInviteEmail(s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return server.ErrValidation
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return server.ErrValidation
	}
	return nil
}
```

Add `"net/mail"` to that file's import block (`strings` and `server` are already imported).

- [ ] **Step 4: Run it to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestValidateInviteEmail -v
```

Expected: PASS.

- [ ] **Step 5: Wire it into the create handler**

In `apps/fleet-service/internal/invite/resource.go`, replace lines 62-69:

```go
			// Role is copied verbatim onto the membership created at accept
			// time, so an unrecognised value would mint a membership whose
			// role no authz gate understands. Validate against the vocabulary
			// membership owns.
			if attrs.Email == "" || !membership.IsValidRole(attrs.Role) {
				server.WriteError(w, server.ErrValidation)
				return
			}
```

with:

```go
			// Role is copied verbatim onto the membership created at accept
			// time, so an unrecognised value would mint a membership whose
			// role no authz gate understands. Validate against the vocabulary
			// membership owns.
			if !membership.IsValidRole(attrs.Role) {
				server.WriteError(w, server.ErrValidation)
				return
			}
			// A newline in an address must fail HERE, not be discovered by the
			// SMTP layer hours later (PRD §8 Security).
			if err := ValidateInviteEmail(attrs.Email); err != nil {
				server.WriteError(w, err)
				return
			}
```

- [ ] **Step 6: Verify the package still builds and all its tests pass**

```sh
go build github.com/jtumidanski/myfleet/apps/fleet-service/... && go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```sh
git add apps/fleet-service/internal/invite/processor.go apps/fleet-service/internal/invite/processor_test.go apps/fleet-service/internal/invite/resource.go
git commit -m "feat(fleet-service): validate invite email as a bare addr-spec"
```

---

## Task 4: Carry `updated_at` onto the Model, and count invites in a window

Both rate limits derive from persisted state so they hold across replicas. The resend cooldown reads
`updated_at`, which the `Entity` already has (`entity.go:20`) but which is dropped on the way to
`Model`.

> **Fragility recorded deliberately (design §4.4):** `updated_at` means "last write of any kind", not
> "last rotation". It is correct today because the only other writer is `Accept`, which sets
> `accepted_at` and therefore trips the 409 before the cooldown is ever consulted. Any future column
> write on this table silently resets the cooldown. The alternative — deriving the last rotation as
> `expires_at - defaultExpiry` — is exact and equally free of new columns, but breaks retroactively
> if `defaultExpiry` ever changes. `updated_at` wins because its failure mode is a *softened* rate
> limit, whereas the other's can be arbitrarily wrong in either direction.

**Files:**
- Modify: `apps/fleet-service/internal/invite/model.go`
- Modify: `apps/fleet-service/internal/invite/entity.go:28-53`
- Modify: `apps/fleet-service/internal/invite/builder.go`
- Modify: `apps/fleet-service/internal/invite/provider.go`
- Modify: `apps/fleet-service/internal/invite/processor.go`
- Test: `apps/fleet-service/internal/invite/processor_test.go`

**Interfaces:**
- Consumes: `server.ErrTooManyRequests` (Task 1).
- Produces:
  - `invite.Model.UpdatedAt() time.Time`
  - `(*invite.Builder).setUpdatedAt(t time.Time) *Builder` — unexported, white-box tests only
  - `invite.Provider.CountByFleetSince(fleetID string, since time.Time) (int64, error)`
  - `(*invite.Processor).CheckCreateLimit(fleetID string, limit int, window time.Duration, now time.Time) error`
  - `(*invite.Processor).CheckResendCooldown(inv Model, cooldown time.Duration, now time.Time) error`

- [ ] **Step 1: Write the failing tests**

Append to `apps/fleet-service/internal/invite/processor_test.go`:

```go
func TestCheckCreateLimit(t *testing.T) {
	now := time.Now()
	sp := &stubProvider{countByFleet: map[string]int64{"f1": 19}}
	p := NewProcessor(logrus.New(), sp)

	if err := p.CheckCreateLimit("f1", 20, 24*time.Hour, now); err != nil {
		t.Fatalf("19 of 20 must be allowed, got %v", err)
	}

	sp.countByFleet["f1"] = 20
	if err := p.CheckCreateLimit("f1", 20, 24*time.Hour, now); !errors.Is(err, server.ErrTooManyRequests) {
		t.Fatalf("at the limit must be 429, got %v", err)
	}

	// The window boundary is what the provider is asked for; assert we asked for
	// the right one rather than trusting the count alone.
	if got, want := sp.lastSince, now.Add(-24*time.Hour); !got.Equal(want) {
		t.Fatalf("counted since %v, want %v", got, want)
	}
}

func TestCheckResendCooldown(t *testing.T) {
	now := time.Now()
	p := newTestProcessor()

	fresh := NewBuilder().setUpdatedAt(now.Add(-time.Minute)).Build()
	if err := p.CheckResendCooldown(fresh, 5*time.Minute, now); !errors.Is(err, server.ErrTooManyRequests) {
		t.Fatalf("1 minute into a 5 minute cooldown must be 429, got %v", err)
	}

	stale := NewBuilder().setUpdatedAt(now.Add(-6 * time.Minute)).Build()
	if err := p.CheckResendCooldown(stale, 5*time.Minute, now); err != nil {
		t.Fatalf("6 minutes into a 5 minute cooldown must be allowed, got %v", err)
	}
}

// Make must carry updated_at through to the Model, or the cooldown above reads a
// zero time and never fires.
func TestMake_carriesUpdatedAt(t *testing.T) {
	ts := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	m := Make(Entity{ID: "i1", UpdatedAt: ts})
	if !m.UpdatedAt().Equal(ts) {
		t.Fatalf("UpdatedAt()=%v want %v", m.UpdatedAt(), ts)
	}
	if !m.ToEntity().UpdatedAt.Equal(ts) {
		t.Fatalf("ToEntity dropped UpdatedAt")
	}
}
```

Extend the existing `stubProvider` (`processor_test.go:13`) — **required, or the package stops
compiling once `Provider` grows a method**:

```go
// stubProvider satisfies the Provider interface for unit tests.
type stubProvider struct {
	byID         map[string]Model
	byToken      map[string]Model
	byFleet      map[string][]Model
	countByFleet map[string]int64
	lastSince    time.Time
}

func (s *stubProvider) CountByFleetSince(fleetID string, since time.Time) (int64, error) {
	s.lastSince = since
	return s.countByFleet[fleetID], nil
}
```

- [ ] **Step 2: Run them to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run 'TestCheckCreateLimit|TestCheckResendCooldown|TestMake_carriesUpdatedAt' -v
```

Expected: FAIL to compile — `undefined: setUpdatedAt`, `UpdatedAt`, `CheckCreateLimit`, `CheckResendCooldown`.

- [ ] **Step 3: Carry `updatedAt` onto the Model**

In `model.go`, add the field to the struct and the accessor:

```go
type Model struct {
	id              string
	fleetID         string
	email           string
	role            string
	token           string
	expiresAt       time.Time
	acceptedAt      *time.Time
	invitedByUserID string
	updatedAt       time.Time
}
```

```go
func (m Model) UpdatedAt() time.Time    { return m.updatedAt }
```

In `entity.go`, add `updatedAt: e.UpdatedAt,` to `Make` and `UpdatedAt: m.updatedAt,` to `ToEntity`.

> GORM stamps `UpdatedAt` itself when the field is zero on `Create`, so round-tripping it costs
> nothing on insert and is what makes the cooldown readable on a resend.

In `builder.go`, add beside the existing `setAcceptedAt`:

```go
// setUpdatedAt is unexported — used only by white-box tests in package invite,
// which need a hand-stamped updated_at to exercise the resend cooldown.
func (b *Builder) setUpdatedAt(t time.Time) *Builder { b.m.updatedAt = t; return b }
```

- [ ] **Step 4: Add the provider count**

In `provider.go`, add to the `Provider` interface and implement it:

```go
// Provider is the read-only interface for invite data access.
type Provider interface {
	GetByID(id string) (Model, error)
	GetByToken(token string) (Model, error)
	ListByFleetID(fleetID string) ([]Model, error)
	// CountByFleetSince backs the per-fleet creation rate limit. It is a query,
	// not an in-process counter, because fleet-service runs multiple replicas
	// and a per-pod limiter enforces nothing (FR-RATE-1).
	CountByFleetSince(fleetID string, since time.Time) (int64, error)
}
```

```go
func (p *dbProvider) CountByFleetSince(fleetID string, since time.Time) (int64, error) {
	var n int64
	err := p.db.Model(&Entity{}).
		Where("fleet_id = ? AND created_at > ?", fleetID, since).
		Count(&n).Error
	return n, err
}
```

Add `"time"` to that file's import block.

- [ ] **Step 5: Add the two processor checks**

Append to `processor.go`:

```go
// CheckCreateLimit enforces the per-fleet invite creation window (FR-RATE-1).
// Over the limit → server.ErrTooManyRequests (429).
func (pr *Processor) CheckCreateLimit(fleetID string, limit int, window time.Duration, now time.Time) error {
	n, err := pr.p.CountByFleetSince(fleetID, now.Add(-window))
	if err != nil {
		return err
	}
	if n >= int64(limit) {
		return server.ErrTooManyRequests
	}
	return nil
}

// CheckResendCooldown enforces the per-invite resend cooldown (FR-RATE-2),
// derived from updated_at — GORM stamps it on the token rotation, so the limit
// survives a pod restart and holds across replicas. See design §4.4 for why
// updated_at is preferred over deriving the last rotation from expires_at.
func (pr *Processor) CheckResendCooldown(inv Model, cooldown time.Duration, now time.Time) error {
	if now.Sub(inv.UpdatedAt()) < cooldown {
		return server.ErrTooManyRequests
	}
	return nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -v
```

Expected: PASS (all tests in the package, including the pre-existing ones).

- [ ] **Step 7: Commit**

```sh
git add apps/fleet-service/internal/invite/
git commit -m "feat(fleet-service): carry updated_at on the invite model and add rate-limit checks"
```

---

## Task 5: Transactional insert with `invite.created`

`Insert` currently writes outside a transaction (`administrator.go:50`). Wrap it so the invite row
and the outbox row commit atomically, mirroring `Accept`. A failed enqueue must roll back the invite.

**Files:**
- Modify: `apps/fleet-service/internal/invite/administrator.go`
- Modify: `apps/fleet-service/internal/invite/resource.go:32-34, :86`
- Modify: `apps/fleet-service/cmd/main.go:95-98, :186`
- Test: `apps/fleet-service/internal/invite/administrator_test.go` (create)

**Interfaces:**
- Consumes: `fleetevents.EmitInviteCreated` (Task 2).
- Produces:
  - `invite.CreatedEmitter func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error`
  - `(*invite.dbAdministrator).WithCreatedEmitter(emit CreatedEmitter) *dbAdministrator`
  - `invite.Administrator.Insert(m Model, traceID string) (Model, error)` — **signature change**
  - `invite.InitializeRoutes(log, db, ownerCheck, record, emit InvitedEmitter, emitCreated CreatedEmitter, limits Limits) func(chi.Router)` — the `limits` parameter lands in Task 6; for **this** task add only `emitCreated`, so the signature is
    `InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, record ActivityRecorder, emit InvitedEmitter, emitCreated CreatedEmitter) func(chi.Router)`

- [ ] **Step 1: Write the failing test**

Create `apps/fleet-service/internal/invite/administrator_test.go`:

```go
package invite

import (
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"
)

// newInviteDB returns an in-memory sqlite DB with the invite table and the
// shared outbox table migrated. No socket, no container (FR-DEV-4).
func newInviteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := Migration(db); err != nil {
		t.Fatalf("migrate invites: %v", err)
	}
	if err := sharedevents.MigrateOutbox(db); err != nil {
		t.Fatalf("migrate outbox: %v", err)
	}
	return db
}

func newInvite(fleetID, email, token string) Model {
	return NewBuilder().
		SetFleetID(fleetID).
		SetEmail(email).
		SetRole("member").
		SetToken(token).
		SetExpiresAt(time.Now().Add(7 * 24 * time.Hour)).
		SetInvitedByUserID("user-1").
		Build()
}

func countRows(t *testing.T, db *gorm.DB, model any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(model).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// FR-EVT-1/FR-EVT-2: the invite row and the outbox row are one unit of work.
func TestInsert_commitsInviteAndOutboxTogether(t *testing.T) {
	db := newInviteDB(t)
	var seen struct {
		fleetID, actorID, traceID, inviteID, email, role string
	}
	adm := NewAdministrator(db).WithCreatedEmitter(
		func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error {
			seen.fleetID, seen.actorID, seen.traceID = fleetID, actorID, traceID
			seen.inviteID, seen.email, seen.role = inviteID, email, role
			return sharedevents.Enqueue(tx, sharedevents.Envelope{EventID: "e1", Type: "invite.created", FleetID: fleetID})
		})

	m := newInvite("f1", "a@b.com", "tok-1")
	created, err := adm.Insert(m, "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if created.ID() != m.ID() {
		t.Fatalf("Insert returned id %q want %q", created.ID(), m.ID())
	}
	if countRows(t, db, &Entity{}) != 1 {
		t.Fatal("want 1 invite row")
	}
	if countRows(t, db, &sharedevents.OutboxRow{}) != 1 {
		t.Fatal("want 1 outbox row")
	}
	// The emitter must be handed the invite's own identity, not the builder's
	// inputs — a mismatch here means the email consumer looks up the wrong row.
	if seen.inviteID != m.ID() || seen.email != "a@b.com" || seen.role != "member" {
		t.Fatalf("emitter got %+v", seen)
	}
	if seen.fleetID != "f1" || seen.actorID != "user-1" || seen.traceID != "trace-1" {
		t.Fatalf("emitter envelope args %+v", seen)
	}
}

// FR-EVT-1: "A failed enqueue rolls back the invite creation." Without the
// transaction this test leaves an invite row behind whose email is never sent
// and whose creation nothing downstream ever hears about.
func TestInsert_rollsBackWhenEmitFails(t *testing.T) {
	db := newInviteDB(t)
	boom := errors.New("outbox unavailable")
	adm := NewAdministrator(db).WithCreatedEmitter(
		func(*gorm.DB, string, string, string, string, string, string) error { return boom })

	if _, err := adm.Insert(newInvite("f1", "a@b.com", "tok-1"), "trace-1"); !errors.Is(err, boom) {
		t.Fatalf("Insert err = %v, want %v", err, boom)
	}
	if n := countRows(t, db, &Entity{}); n != 0 {
		t.Fatalf("want 0 invite rows after rollback, got %d", n)
	}
	if n := countRows(t, db, &sharedevents.OutboxRow{}); n != 0 {
		t.Fatalf("want 0 outbox rows after rollback, got %d", n)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestInsert -v
```

Expected: FAIL to compile — `undefined: WithCreatedEmitter`, and `Insert` takes 1 argument.

- [ ] **Step 3: Make `Insert` transactional**

In `administrator.go`, update the interface, add the emitter type and setter, and rewrite `Insert`.

Interface (replace lines 11-19):

```go
// Administrator is the write interface for invite data access.
type Administrator interface {
	// Insert creates the invite row and enqueues an invite.created outbox event
	// in the SAME transaction (FR-EVT-1, FR-EVT-2). A failed enqueue rolls the
	// invite back — an invite nobody is told about is worse than no invite.
	Insert(m Model, traceID string) (Model, error)
	Delete(id string) error
	// Accept stamps accepted_at and creates an active membership in one transaction.
	// Appends a member.invited activity event and enqueues a member.invited
	// outbox event in the SAME transaction.
	Accept(inv Model, userID, traceID string) (Model, error)
}
```

Add beside `InvitedEmitter` (after line 27):

```go
// CreatedEmitter enqueues an invite.created event in the outbox on the supplied
// tx. Injected to avoid coupling, exactly like InvitedEmitter. Satisfied by
// events.EmitInviteCreated. Deliberately a SEPARATE seam from InvitedEmitter:
// member.invited fires on ACCEPT, invite.created fires on CREATE/RESEND.
type CreatedEmitter func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error
```

Add the field to `dbAdministrator`:

```go
type dbAdministrator struct {
	db          *gorm.DB
	record      ActivityRecorder
	emit        InvitedEmitter
	emitCreated CreatedEmitter
}
```

Add the setter after `WithEmitter`:

```go
// WithCreatedEmitter injects the invite.created outbox emitter (FR-EVT-1).
func (a *dbAdministrator) WithCreatedEmitter(emit CreatedEmitter) *dbAdministrator {
	a.emitCreated = emit
	return a
}
```

Replace `Insert` (lines 50-56):

```go
func (a *dbAdministrator) Insert(m Model, traceID string) (Model, error) {
	e := m.ToEntity()
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&e).Error; err != nil {
			return err
		}
		// FATAL: a failed enqueue rolls back the invite row. An invite the
		// invitee is never told about is undeliverable by any in-product path.
		if a.emitCreated != nil {
			return a.emitCreated(tx, e.FleetID, e.InvitedByUserID, traceID, e.ID, e.Email, e.Role)
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return Make(e), nil
}
```

- [ ] **Step 4: Run it to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestInsert -v
```

Expected: PASS.

- [ ] **Step 5: Wire the new parameter through the resource and main**

In `resource.go`, change the signature and construction (lines 32-34):

```go
// InitializeRoutes wires the JWT-protected invite endpoints.
// ownerCheck is injected for the authoritative DB owner recheck on mutations.
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, record ActivityRecorder, emit InvitedEmitter, emitCreated CreatedEmitter) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db).WithActivityRecorder(record).WithEmitter(emit).WithCreatedEmitter(emitCreated)
	proc := NewProcessor(log, prov)
```

In `resource.go`, change the create handler's `Insert` call (line 86) to pass the correlation id —
`telemetry` is already imported (line 16):

```go
			created, err := adm.Insert(m, telemetry.CorrelationIDFromContext(req.Context()))
```

In `apps/fleet-service/cmd/main.go`, add the adapter closure after `emitMemberInvited` (line 98):

```go
	emitInviteCreated := func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error {
		return fleetevents.EmitInviteCreated(tx, fleetID, actorID, traceID,
			dtoevents.InviteCreatedData{InviteID: inviteID, Email: email, Role: role})
	}
```

and pass it at the call site (line 186):

```go
				invite.InitializeRoutes(log, db, membershipProc, activity.Record, emitMemberInvited, emitInviteCreated)(pr)
```

- [ ] **Step 6: Verify the whole service builds and its tests pass**

```sh
go build github.com/jtumidanski/myfleet/... && go test github.com/jtumidanski/myfleet/apps/fleet-service/... -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```sh
git add apps/fleet-service/internal/invite/ apps/fleet-service/cmd/main.go
git commit -m "feat(fleet-service): emit invite.created from a transactional invite insert"
```

---

## Task 6: Resend endpoint, token rotation, and both rate limits

**Files:**
- Modify: `apps/fleet-service/internal/invite/administrator.go`
- Modify: `apps/fleet-service/internal/invite/resource.go`
- Modify: `apps/fleet-service/cmd/main.go`
- Test: `apps/fleet-service/internal/invite/administrator_test.go`

**Interfaces:**
- Consumes: `CheckCreateLimit`, `CheckResendCooldown` (Task 4); `CreatedEmitter` (Task 5).
- Produces:
  - `invite.Administrator.Resend(inv Model, newToken string, expiresAt, now time.Time, traceID string) (Model, error)`
  - `invite.Limits struct { CreatePerWindow int; CreateWindow time.Duration; ResendCooldown time.Duration }`
  - `invite.InitializeRoutes(log, db, ownerCheck, record, emit, emitCreated, limits Limits) func(chi.Router)` — **final signature**
  - Route `POST /fleets/{fleetId}/invites/{inviteId}/resend`

`Resend` takes `now` explicitly rather than calling `time.Now()` internally so the returned Model's
`updatedAt` is exactly the value written — which is what the cooldown then reads.

- [ ] **Step 1: Write the failing rotation test**

Append to `apps/fleet-service/internal/invite/administrator_test.go`:

```go
// FR-RSND-2: rotation is an UPDATE, so token's unique index still holds and the
// old link resolves to no row (404 on accept), which is acceptance criterion 4.
func TestResend_rotatesTokenAndEmitsFreshEvent(t *testing.T) {
	db := newInviteDB(t)
	events := 0
	adm := NewAdministrator(db).WithCreatedEmitter(
		func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error {
			events++
			return sharedevents.Enqueue(tx, sharedevents.Envelope{
				EventID: "e" + strconv.Itoa(events), Type: "invite.created", FleetID: fleetID})
		})

	orig, err := adm.Insert(newInvite("f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	now := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	newExpiry := now.Add(7 * 24 * time.Hour)
	updated, err := adm.Resend(orig, "tok-2", newExpiry, now, "trace-2")
	if err != nil {
		t.Fatalf("Resend: %v", err)
	}

	if updated.Token() != "tok-2" {
		t.Fatalf("token=%q want tok-2", updated.Token())
	}
	if !updated.ExpiresAt().Equal(newExpiry) {
		t.Fatalf("expires_at=%v want %v", updated.ExpiresAt(), newExpiry)
	}
	if !updated.UpdatedAt().Equal(now) {
		t.Fatalf("updated_at=%v want %v", updated.UpdatedAt(), now)
	}

	// The old token must no longer resolve — that is what makes the previously
	// mailed link dead.
	prov := NewProvider(db)
	if _, err := prov.GetByToken("tok-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token still resolves: %v", err)
	}
	if got, err := prov.GetByToken("tok-2"); err != nil || got.ID() != orig.ID() {
		t.Fatalf("new token lookup: %v %+v", err, got)
	}

	// Exactly one row was updated, not inserted, and a SECOND event was emitted
	// with a new event_id — that new id is what lets it past the consumer's
	// (event_id, consumer) ledger (FR-EVT-4).
	if n := countRows(t, db, &Entity{}); n != 1 {
		t.Fatalf("want 1 invite row after resend, got %d", n)
	}
	if n := countRows(t, db, &sharedevents.OutboxRow{}); n != 2 {
		t.Fatalf("want 2 outbox rows after resend, got %d", n)
	}
}

// A failed enqueue must leave the token unrotated — otherwise the previously
// mailed link dies and no new mail is ever sent.
func TestResend_rollsBackWhenEmitFails(t *testing.T) {
	db := newInviteDB(t)
	boom := errors.New("outbox unavailable")
	emitOK := true
	adm := NewAdministrator(db).WithCreatedEmitter(
		func(tx *gorm.DB, fleetID, actorID, traceID, inviteID, email, role string) error {
			if emitOK {
				return sharedevents.Enqueue(tx, sharedevents.Envelope{EventID: "e1", Type: "invite.created", FleetID: fleetID})
			}
			return boom
		})

	orig, err := adm.Insert(newInvite("f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	emitOK = false
	now := time.Now()
	if _, err := adm.Resend(orig, "tok-2", now.Add(time.Hour), now, "trace-2"); !errors.Is(err, boom) {
		t.Fatalf("Resend err = %v, want %v", err, boom)
	}
	if _, err := NewProvider(db).GetByToken("tok-1"); err != nil {
		t.Fatalf("original token must survive a rolled-back resend: %v", err)
	}
}
```

Add `"strconv"` to that file's import block.

- [ ] **Step 2: Run it to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestResend -v
```

Expected: FAIL to compile — `adm.Resend undefined`.

- [ ] **Step 3: Implement `Resend`**

Add to the `Administrator` interface in `administrator.go`, after `Insert`:

```go
	// Resend rotates the token and resets expires_at, enqueuing a fresh
	// invite.created in the SAME transaction (FR-RSND-2). now is passed in
	// rather than read internally so the returned Model's updated_at is exactly
	// the value written — which is what the resend cooldown then reads.
	Resend(inv Model, newToken string, expiresAt, now time.Time, traceID string) (Model, error)
```

Add the implementation after `Insert`:

```go
func (a *dbAdministrator) Resend(inv Model, newToken string, expiresAt, now time.Time, traceID string) (Model, error) {
	var updated Model
	err := a.db.Transaction(func(tx *gorm.DB) error {
		// updated_at is set explicitly rather than left to GORM so the value we
		// return is provably the value persisted.
		res := tx.Model(&Entity{}).Where("id = ?", inv.ID()).Updates(map[string]any{
			"token":      newToken,
			"expires_at": expiresAt,
			"updated_at": now,
		})
		if res.Error != nil {
			return res.Error
		}
		updated = Make(Entity{
			ID:              inv.ID(),
			FleetID:         inv.FleetID(),
			Email:           inv.Email(),
			Role:            inv.Role(),
			Token:           newToken,
			ExpiresAt:       expiresAt,
			AcceptedAt:      inv.AcceptedAt(),
			InvitedByUserID: inv.InvitedByUserID(),
			UpdatedAt:       now,
		})
		// FATAL: a failed enqueue rolls the rotation back. Rotating without
		// emitting would kill the previously mailed link and send nothing new.
		if a.emitCreated != nil {
			return a.emitCreated(tx, inv.FleetID(), inv.InvitedByUserID(), traceID, inv.ID(), inv.Email(), inv.Role())
		}
		return nil
	})
	if err != nil {
		return Model{}, err
	}
	return updated, nil
}
```

- [ ] **Step 4: Run it to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestResend -v
```

Expected: PASS.

- [ ] **Step 5: Add the `Limits` type and the resend route**

In `resource.go`, add after the `defaultExpiry` const (line 22):

```go
// Limits carries the abuse-control knobs (FR-RATE-1…4). Both are enforced
// server-side in the domain layer; the UI disabling a button is a convenience,
// not the control.
type Limits struct {
	CreatePerWindow int
	CreateWindow    time.Duration
	ResendCooldown  time.Duration
}
```

Change the signature and construction (lines 32-35) to the final form:

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, ownerCheck OwnerChecker, record ActivityRecorder, emit InvitedEmitter, emitCreated CreatedEmitter, limits Limits) func(chi.Router) {
	prov := NewProvider(db)
	adm := NewAdministrator(db).WithActivityRecorder(record).WithEmitter(emit).WithCreatedEmitter(emitCreated)
	proc := NewProcessor(log, prov)
```

In the create handler, add the window check immediately after the `ValidateInviteEmail` block from
Task 3 and **before** `generateToken()`:

```go
			// Per-fleet creation window (FR-RATE-1). Checked before minting a
			// token so a throttled request costs no entropy and no DB write.
			if err := proc.CheckCreateLimit(fleetID, limits.CreatePerWindow, limits.CreateWindow, time.Now()); err != nil {
				server.WriteError(w, err)
				return
			}
```

Add the new route inside the returned `func(chi.Router)`, after the DELETE handler:

```go
		// POST /fleets/{fleetId}/invites/{inviteId}/resend — owner-only.
		// Rotates the token and resets expiry (FR-RSND-1…5): resend is used when
		// the previous link never arrived or expired, so invalidating it costs
		// nothing and bounds the lifetime of a token that leaked into a mailbox.
		r.Post("/fleets/{fleetId}/invites/{inviteId}/resend", func(w http.ResponseWriter, req *http.Request) {
			identity := auth.IdentityFromContext(req.Context())
			fleetID := chi.URLParam(req, "fleetId")
			inviteID := chi.URLParam(req, "inviteId")

			inv, err := proc.GetByID(inviteID)
			if err != nil {
				server.WriteError(w, err)
				return
			}

			if err := authz.RequireSameFleet(identity, fleetID); err != nil {
				server.WriteError(w, err)
				return
			}
			// Path-pair mismatch → 403, not 404: the caller proved membership of
			// the fleet they named but named an invite belonging to another one.
			if inv.FleetID() != fleetID {
				server.WriteError(w, server.ErrForbidden)
				return
			}
			// Token-level owner gate (fast path)
			if err := authz.RequireOwner(identity); err != nil {
				server.WriteError(w, err)
				return
			}
			// Authoritative DB check via processor (stale-claim guard, design §9)
			if err := ownerCheck.RequireOwnerInFleet(fleetID, identity.UserID); err != nil {
				server.WriteError(w, err)
				return
			}

			// Accepted BEFORE cooldown, so an accepted invite never reports a
			// cooldown it could never satisfy (FR-RSND-3).
			if inv.AcceptedAt() != nil {
				server.WriteError(w, server.ErrConflict)
				return
			}
			now := time.Now()
			if err := proc.CheckResendCooldown(inv, limits.ResendCooldown, now); err != nil {
				server.WriteError(w, err)
				return
			}

			token, err := generateToken()
			if err != nil {
				server.WriteError(w, err)
				return
			}
			updated, err := adm.Resend(inv, token, now.Add(defaultExpiry), now,
				telemetry.CorrelationIDFromContext(req.Context()))
			if err != nil {
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, server.Document{Data: Transform(updated)})
		})
```

- [ ] **Step 6: Wire the limits in main**

In `apps/fleet-service/cmd/main.go`, add beside the other config reads (near the `mediaClient`
construction, line ~176):

```go
	// Abuse control (FR-RATE-1…4). Twenty invites per fleet per day is roughly
	// 3x the largest plausible household fleet, so it never obstructs real use
	// while capping a compromised account's burn on the domain's sending
	// reputation. Five minutes is longer than any mail delay a user would wait
	// through before hitting resend again.
	inviteLimits := invite.Limits{
		CreatePerWindow: config.GetInt("INVITE_RATE_LIMIT_PER_DAY", 20),
		CreateWindow:    24 * time.Hour,
		ResendCooldown:  time.Duration(config.GetInt("INVITE_RESEND_COOLDOWN_SECONDS", 300)) * time.Second,
	}
```

Update the call site:

```go
				invite.InitializeRoutes(log, db, membershipProc, activity.Record, emitMemberInvited, emitInviteCreated, inviteLimits)(pr)
```

- [ ] **Step 7: Verify the service builds and all tests pass**

```sh
go build github.com/jtumidanski/myfleet/... && go test github.com/jtumidanski/myfleet/apps/fleet-service/... -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add apps/fleet-service/internal/invite/ apps/fleet-service/cmd/main.go
git commit -m "feat(fleet-service): add invite resend with token rotation and rate limits"
```

---

## Task 7: Internal invite lookup endpoint

This endpoint returns a bearer token. It must never be reachable through the public router. The
ingress deny rule already covers it — `PathRegexp((?i)^/+api/+fleet[^/]*/*internal)` in
`deploy/k8s/overlays/main/ingressroute.yaml:89` matches any path under `/api/fleet/internal…` — so
**no manifest change is needed**; what is needed is a test proving the route is absent from the JWT
tree (FR-INT-4).

`invited_by_name` is **not** in the response. auth-service exposes no internal surface at all, so
resolving a display name would mean creating the identity service's first unauthenticated endpoint
plus a third priority-200 deny rule and a `REQUIRED_DENY_SERVICES` entry — a new attack surface on
the identity service for one cosmetic string (design §4.5). The email names the fleet instead.

**Files:**
- Create: `apps/fleet-service/internal/invite/internal.go`
- Create: `apps/fleet-service/internal/invite/internal_test.go`
- Modify: `apps/fleet-service/internal/invite/rest.go`
- Modify: `apps/fleet-service/cmd/main.go:178-179`

**Interfaces:**
- Consumes: `invite.Provider`, `fleet.Model` (read-only).
- Produces:
  - `invite.FleetNamer interface { GetByID(id string) (fleet.Model, error) }`
  - `invite.InternalResponse struct` with json tags `invite_id`, `fleet_id`, `fleet_name`, `email`, `role`, `token`, `expires_at`, `accepted_at`, `invited_by_user_id`
  - `invite.TransformInternal(m Model, fleetName string) InternalResponse`
  - `invite.InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB, fleets FleetNamer) func(chi.Router)`
  - Route `GET /internal/invites/{inviteID}` — plain JSON, **not** JSON:API, matching the existing internal convention (`membership/resource.go:108`)

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/invite/internal_test.go`:

```go
package invite

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

type stubFleetNamer struct {
	names map[string]string
	err   error
}

func (s stubFleetNamer) GetByID(id string) (fleet.Model, error) {
	if s.err != nil {
		return fleet.Model{}, s.err
	}
	name, ok := s.names[id]
	if !ok {
		return fleet.Model{}, fleet.ErrNotFound
	}
	return fleet.NewBuilder().SetName(name).Build(), nil
}

func internalRouter(t *testing.T, db *gorm.DB, namer FleetNamer) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	InitializeInternalRoutes(logrus.New(), db, namer)(r)
	return r
}

func TestInternalGetInvite_returnsTokenAndFleetName(t *testing.T) {
	db := newInviteDB(t)
	adm := NewAdministrator(db)
	created, err := adm.Insert(newInvite("f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	r := internalRouter(t, db, stubFleetNamer{names: map[string]string{"f1": "The Smiths"}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/"+created.ID(), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got InternalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.InviteID != created.ID() || got.FleetID != "f1" || got.FleetName != "The Smiths" {
		t.Fatalf("identity fields wrong: %+v", got)
	}
	// The whole point of this endpoint: the consumer cannot compose the email
	// without the token, and the token is deliberately absent from the event.
	if got.Token != "tok-1" {
		t.Fatalf("token=%q want tok-1", got.Token)
	}
	if got.Email != "a@b.com" || got.Role != "member" || got.InvitedByUserID != "user-1" {
		t.Fatalf("payload fields wrong: %+v", got)
	}
	if got.AcceptedAt != nil {
		t.Fatalf("accepted_at should be null, got %v", *got.AcceptedAt)
	}
	if _, err := time.Parse(time.RFC3339, got.ExpiresAt); err != nil {
		t.Fatalf("expires_at %q is not RFC3339: %v", got.ExpiresAt, err)
	}
}

func TestInternalGetInvite_unknownIDIs404(t *testing.T) {
	r := internalRouter(t, newInviteDB(t), stubFleetNamer{names: map[string]string{}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

// FR-INT-3: the row is returned even when accepted; the CALLER decides whether
// to send. Returning 404 here would make a stale redelivery indistinguishable
// from a deleted invite and cost the consumer four pointless retries.
func TestInternalGetInvite_returnsAcceptedInvites(t *testing.T) {
	db := newInviteDB(t)
	adm := NewAdministrator(db)
	created, err := adm.Insert(newInvite("f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	accepted := time.Now().UTC()
	if err := db.Model(&Entity{}).Where("id = ?", created.ID()).Update("accepted_at", &accepted).Error; err != nil {
		t.Fatalf("stamp accepted_at: %v", err)
	}

	r := internalRouter(t, db, stubFleetNamer{names: map[string]string{"f1": "The Smiths"}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/"+created.ID(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	var got InternalResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.AcceptedAt == nil {
		t.Fatal("accepted_at must be populated")
	}
}

// A missing fleet row degrades to an empty name rather than a 500 — the email
// then falls back to a generic subject (design §4.5).
func TestInternalGetInvite_missingFleetDegradesToEmptyName(t *testing.T) {
	db := newInviteDB(t)
	created, err := NewAdministrator(db).Insert(newInvite("f1", "a@b.com", "tok-1"), "trace-1")
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	r := internalRouter(t, db, stubFleetNamer{names: map[string]string{}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/internal/invites/"+created.ID(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	var got InternalResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.FleetName != "" {
		t.Fatalf("fleet_name=%q want empty", got.FleetName)
	}
}

// FR-INT-4. The tree is WALKED rather than probed with one URL, so a future
// internal route registered on the wrong initializer also fails here.
func TestInternalRouteAbsentFromJWTTree(t *testing.T) {
	db := newInviteDB(t)
	r := chi.NewRouter()
	InitializeRoutes(logrus.New(), db, stubOwnerChecker{}, nil, nil, nil, Limits{})(r)

	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.Contains(route, "internal") {
			t.Errorf("JWT-protected tree registers an internal route: %s %s", method, route)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

type stubOwnerChecker struct{}

func (stubOwnerChecker) RequireOwnerInFleet(string, string) error { return nil }
```

- [ ] **Step 2: Run them to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -run TestInternal -v
```

Expected: FAIL to compile — `undefined: InternalResponse`, `InitializeInternalRoutes`, `FleetNamer`.

- [ ] **Step 3: Add the internal response shape**

Append to `apps/fleet-service/internal/invite/rest.go`:

```go
// InternalResponse is the plain-JSON (NOT JSON:API) body of
// GET /internal/invites/{inviteID}, matching the convention the other internal
// endpoints use (membership/resource.go:108).
//
// This is the ONLY place outside the fleet-scoped list where the token is
// served. The endpoint is network-restricted; see design §4.5.
type InternalResponse struct {
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

// TransformInternal builds the internal response. fleetName may be empty when
// the fleet row is unresolvable; the email degrades to a generic subject.
func TransformInternal(m Model, fleetName string) InternalResponse {
	out := InternalResponse{
		InviteID:        m.ID(),
		FleetID:         m.FleetID(),
		FleetName:       fleetName,
		Email:           m.Email(),
		Role:            m.Role(),
		Token:           m.Token(),
		ExpiresAt:       m.ExpiresAt().Format(time.RFC3339),
		InvitedByUserID: m.InvitedByUserID(),
	}
	if m.AcceptedAt() != nil {
		s := m.AcceptedAt().Format(time.RFC3339)
		out.AcceptedAt = &s
	}
	return out
}
```

Add `"time"` to that file's import block.

- [ ] **Step 4: Add the internal routes file**

Create `apps/fleet-service/internal/invite/internal.go`:

```go
package invite

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/fleet"
)

// FleetNamer resolves a fleet's display name for the invite email's subject and
// body. A narrow read-only seam rather than the fleet processor, mirroring how
// OwnerChecker keeps this package decoupled. Satisfied by fleet.Provider.
type FleetNamer interface {
	GetByID(id string) (fleet.Model, error)
}

// InitializeInternalRoutes wires the network-restricted internal endpoint (no JWT).
// Register this initializer WITHOUT JWT middleware.
//
// GET /internal/invites/{inviteID} → InternalResponse or 404.
//
// SECURITY: this endpoint serves the invite TOKEN, a bearer credential. It is
// kept off the public internet by the priority-200 internal-deny rule in the
// main overlay's ingressroute, which already matches every path under
// /api/fleet/internal. TestInternalRouteAbsentFromJWTTree proves it is not
// reachable through the JWT router either (FR-INT-4).
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB, fleets FleetNamer) func(chi.Router) {
	prov := NewProvider(db)
	proc := NewProcessor(log, prov)
	return func(r chi.Router) {
		r.Get("/internal/invites/{inviteID}", func(w http.ResponseWriter, req *http.Request) {
			inviteID := chi.URLParam(req, "inviteID")
			if inviteID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			// Returned even when accepted or expired (FR-INT-3) — the consumer
			// decides whether to send, and needs to tell "stale" apart from
			// "deleted" to avoid pointless retries.
			inv, err := proc.GetByID(inviteID)
			if err != nil {
				if errors.Is(err, server.ErrNotFound) {
					server.WriteError(w, server.ErrNotFound)
					return
				}
				log.WithError(err).WithField("invite_id", inviteID).Error("internal get invite")
				server.WriteError(w, err)
				return
			}

			// An unresolvable fleet degrades to an empty name rather than a 500;
			// the email then uses the generic subject.
			var fleetName string
			f, err := fleets.GetByID(inv.FleetID())
			if err != nil {
				log.WithError(err).WithFields(logrus.Fields{
					"invite_id": inviteID,
					"fleet_id":  inv.FleetID(),
				}).Warn("could not resolve fleet name for invite email")
			} else {
				fleetName = f.Name()
			}

			server.WriteJSON(w, http.StatusOK, TransformInternal(inv, fleetName))
		})
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite/... -v
```

Expected: PASS. If `fleet.NewBuilder().SetName(...)` or `fleet.ErrNotFound` do not exist with those
names, read `apps/fleet-service/internal/fleet/builder.go` and `provider.go` and adjust the stub in
`internal_test.go` to match — the production code above depends only on `fleet.Model` and `Name()`.

- [ ] **Step 6: Register the initializer in main**

In `apps/fleet-service/cmd/main.go`, add to the internal-routes block (after line 179):

```go
		AddRouteInitializer(invite.InitializeInternalRoutes(log, db, fleet.NewProvider(db))).
```

- [ ] **Step 7: Verify the service builds and all tests pass**

```sh
go build github.com/jtumidanski/myfleet/... && go test github.com/jtumidanski/myfleet/apps/fleet-service/... -v
```

Expected: PASS.

- [ ] **Step 8: Commit**

```sh
git add apps/fleet-service/internal/invite/ apps/fleet-service/cmd/main.go
git commit -m "feat(fleet-service): add network-restricted internal invite lookup"
```

---

## Task 8: `fleetclient.Invite` with a typed status error

`getJSON` returns a bare `fmt.Errorf` on non-200 (`client.go:75`), which the mail consumer cannot
tell apart from a transient blip. A deleted invite must be a **permanent** skip — otherwise four
retries burn against a row that will never exist.

**Files:**
- Modify: `apps/notification-service/internal/fleetclient/client.go`
- Test: `apps/notification-service/internal/fleetclient/client_test.go` (create)

**Interfaces:**
- Consumes: `GET /internal/invites/{inviteID}` (Task 7).
- Produces:
  - `fleetclient.Invite struct` — fields `InviteID`, `FleetID`, `FleetName`, `Email`, `Role`, `Token`, `ExpiresAt string`, `AcceptedAt *string`, `InvitedByUserID`
  - `fleetclient.ErrInviteNotFound error`
  - `(*fleetclient.Client).Invite(ctx context.Context, inviteID string) (Invite, error)`

`ExpiresAt`/`AcceptedAt` stay strings on the wire and are parsed with `time.Parse(time.RFC3339, …)`
by the consumer, matching how `TransformInternal` formats them.

- [ ] **Step 1: Write the failing tests**

Create `apps/notification-service/internal/fleetclient/client_test.go`:

```go
package fleetclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvite_decodesTheInternalResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/invites/inv-1" {
			t.Errorf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"invite_id":"inv-1","fleet_id":"f1","fleet_name":"The Smiths",
			"email":"a@b.com","role":"member","token":"tok-1",
			"expires_at":"2026-08-09T12:00:00Z","accepted_at":null,
			"invited_by_user_id":"u1"}`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL).Invite(context.Background(), "inv-1")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if got.InviteID != "inv-1" || got.FleetID != "f1" || got.FleetName != "The Smiths" {
		t.Fatalf("identity fields wrong: %+v", got)
	}
	if got.Token != "tok-1" || got.Email != "a@b.com" || got.Role != "member" {
		t.Fatalf("payload fields wrong: %+v", got)
	}
	if got.ExpiresAt != "2026-08-09T12:00:00Z" || got.AcceptedAt != nil {
		t.Fatalf("time fields wrong: %+v", got)
	}
}

// A 404 must be distinguishable from a blip, or the consumer retries four times
// against a row that will never exist.
func TestInvite_404IsASentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).Invite(context.Background(), "gone"); !errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("err=%v want ErrInviteNotFound", err)
	}
}

// Any other non-200 stays a plain error, i.e. transient, and is retried.
func TestInvite_500IsNotTheSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewClient(srv.URL).Invite(context.Background(), "inv-1")
	if err == nil || errors.Is(err, ErrInviteNotFound) {
		t.Fatalf("err=%v want a non-sentinel error", err)
	}
}

// The existing callers must keep behaving identically: *statusError still
// satisfies error and formats the same way.
func TestActiveMembers_nonOKStillErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL).ActiveMembers(context.Background(), "f1"); err == nil {
		t.Fatal("want an error for a 502")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient/... -v
```

Expected: FAIL to compile — `undefined: ErrInviteNotFound`, `c.Invite undefined`.

- [ ] **Step 3: Add the typed status error and the `Invite` method**

In `client.go`, add to the imports: `"errors"`, `"time"` is **not** needed.

Add after the `DueSchedule` type:

```go
// ErrInviteNotFound is returned by Invite when fleet-service reports 404. The
// mail consumer treats it as a PERMANENT condition (mark the ledger, do not
// retry): an invite that has been deleted will never come back, and retrying
// against it four times is pure waste.
var ErrInviteNotFound = errors.New("invite not found")

// Invite is one invite as served by fleet-service's network-restricted
// GET /internal/invites/{inviteID}. It carries the TOKEN, which is why the
// endpoint is internal-only and why this struct is never logged whole.
//
// ExpiresAt/AcceptedAt stay strings on the wire and are parsed with
// time.RFC3339 by the caller, matching how the invite REST layer formats them.
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

// statusError carries the HTTP status of a non-200 internal response so callers
// can classify it. It formats identically to the fmt.Errorf it replaced, so
// ActiveMembers and DueSchedules are unaffected.
type statusError struct {
	url  string
	code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("fleet internal %s: status %d", e.url, e.code)
}
```

Add the method after `DueSchedules`:

```go
// Invite fetches one invite, including its token, for composing an invite email.
func (c *Client) Invite(ctx context.Context, inviteID string) (Invite, error) {
	url := fmt.Sprintf("%s/internal/invites/%s", c.base, inviteID)
	var out Invite
	if err := c.getJSON(ctx, url, &out); err != nil {
		var se *statusError
		if errors.As(err, &se) && se.code == http.StatusNotFound {
			return Invite{}, ErrInviteNotFound
		}
		return Invite{}, err
	}
	return out, nil
}
```

Replace the non-200 branch of `getJSON` (line 74-76):

```go
	if res.StatusCode != http.StatusOK {
		return &statusError{url: url, code: res.StatusCode}
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient/... -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add apps/notification-service/internal/fleetclient/
git commit -m "feat(notification-service): add internal invite lookup to fleetclient"
```

---

## Task 9: The `mailer` package — rendering and composition

`mailer` is **infrastructure, not a domain**: no Model/Entity/Provider/Administrator, matching the
existing `internal/inbox` and `internal/fleetclient` packages. It knows nothing about invites beyond
a struct of already-resolved fields. Called out here so the backend guidelines reviewer scores it
against SUB-* rather than DOM-*.

Rendering lives **above** `Send`, so `Sender` stays a pure transport seam: `RenderInvite` is tested
directly on its output strings with no sender at all.

**Files:**
- Create: `apps/notification-service/internal/mailer/sender.go`
- Create: `apps/notification-service/internal/mailer/template.go`
- Create: `apps/notification-service/internal/mailer/templates/invite.html.tmpl`
- Create: `apps/notification-service/internal/mailer/templates/invite.txt.tmpl`
- Create: `apps/notification-service/internal/mailer/compose.go`
- Create: `apps/notification-service/internal/mailer/fake.go`
- Test: `apps/notification-service/internal/mailer/template_test.go`
- Test: `apps/notification-service/internal/mailer/compose_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `mailer.Message struct { To, Subject, HTML, Text string }`
  - `mailer.Sender interface { Send(ctx context.Context, msg Message) error }`
  - `mailer.PermanentError struct { Err error }` with `Error() string` and `Unwrap() error` — pointer receiver, so `errors.As(err, &target)` with `target *PermanentError`
  - `mailer.InviteData struct { To, FleetName, Role, AcceptURL string; ExpiresAt time.Time }`
  - `mailer.RenderInvite(d InviteData) (Message, error)`
  - `mailer.FakeSender struct { Sent []Message; Errs []error; Err error }` with `Send`, exported because `mailconsumer` (a different package) uses it
  - unexported `compose(fromName, fromAddress string, msg Message, date time.Time, messageID, boundary string) ([]byte, error)`, `newBoundary() (string, error)`, `newMessageID(fromAddress string) (string, error)`

- [ ] **Step 1: Write the failing rendering tests**

Create `apps/notification-service/internal/mailer/template_test.go`:

```go
package mailer

import (
	"strings"
	"testing"
	"time"
)

func testInviteData() InviteData {
	return InviteData{
		To:        "a@b.com",
		FleetName: "The Smiths",
		Role:      "member",
		AcceptURL: "https://myfleet.example.com/invites/deadbeef/accept",
		ExpiresAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
	}
}

func TestRenderInvite(t *testing.T) {
	msg, err := RenderInvite(testInviteData())
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if msg.To != "a@b.com" {
		t.Fatalf("To=%q", msg.To)
	}
	// FR-TPL-4: fixed string plus the fleet name, and NEVER the token.
	if msg.Subject != "You're invited to The Smiths on MyFleet" {
		t.Fatalf("Subject=%q", msg.Subject)
	}
	if strings.Contains(msg.Subject, "deadbeef") {
		t.Fatal("the token must never appear in the subject")
	}
	// FR-TPL-1: both parts exist and both are usable on their own.
	for name, part := range map[string]string{"html": msg.HTML, "text": msg.Text} {
		if part == "" {
			t.Fatalf("%s part is empty", name)
		}
		if !strings.Contains(part, "https://myfleet.example.com/invites/deadbeef/accept") {
			t.Fatalf("%s part is missing the accept URL: %s", name, part)
		}
		if !strings.Contains(part, "The Smiths") {
			t.Fatalf("%s part is missing the fleet name", name)
		}
		if !strings.Contains(part, "member") {
			t.Fatalf("%s part is missing the role", name)
		}
		// FR-TPL-3: the expiry must be legible, not a raw timestamp.
		if !strings.Contains(part, "August 9, 2026") {
			t.Fatalf("%s part is missing a legible expiry: %s", name, part)
		}
		// FR-TPL-6.
		if !strings.Contains(strings.ToLower(part), "ignore") {
			t.Fatalf("%s part is missing the 'ignore if unexpected' line", name)
		}
	}
}

// FR-TPL-5: the fleet name is user-controlled input. html/template must escape
// it contextually; the raw script tag must not survive into the HTML part.
func TestRenderInvite_escapesUserControlledFleetName(t *testing.T) {
	d := testInviteData()
	d.FleetName = `<script>alert(1)</script>`
	msg, err := RenderInvite(d)
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if strings.Contains(msg.HTML, "<script>") {
		t.Fatalf("unescaped script tag survived into the HTML part: %s", msg.HTML)
	}
	if !strings.Contains(msg.HTML, "&lt;script&gt;") {
		t.Fatalf("expected an escaped fleet name, got: %s", msg.HTML)
	}
}

// The accept URL is the one value that must NOT be escaped into uselessness.
// html/template treats href as a URL context and passes an https:// URL through
// intact — but would rewrite a javascript: scheme, which is the protection we
// want to keep.
func TestRenderInvite_keepsTheAcceptURLUsableInHref(t *testing.T) {
	msg, err := RenderInvite(testInviteData())
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if !strings.Contains(msg.HTML, `href="https://myfleet.example.com/invites/deadbeef/accept"`) {
		t.Fatalf("href was mangled: %s", msg.HTML)
	}
}

// An empty fleet name degrades to a generic subject rather than rendering
// "invited to  on MyFleet" (FR-TPL-3).
func TestRenderInvite_emptyFleetNameDegradesGracefully(t *testing.T) {
	d := testInviteData()
	d.FleetName = ""
	msg, err := RenderInvite(d)
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if msg.Subject != "You're invited to a fleet on MyFleet" {
		t.Fatalf("Subject=%q", msg.Subject)
	}
	if strings.Contains(msg.Text, "  ") {
		t.Fatalf("empty fleet name left a hole in the text part: %s", msg.Text)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -v
```

Expected: FAIL — no such package.

- [ ] **Step 3: Write the sender types**

Create `apps/notification-service/internal/mailer/sender.go`:

```go
// Package mailer composes and delivers transactional email. It is
// INFRASTRUCTURE, not a DDD domain — no Model/Entity/Provider/Administrator,
// matching internal/inbox and internal/fleetclient.
//
// It knows nothing about invites beyond a struct of already-resolved fields;
// mailconsumer knows nothing about SMTP. That split is what lets every test run
// without a socket (FR-MAIL-1, FR-DEV-4).
package mailer

import "context"

// Message is one already-rendered email. Both parts are required: a single-part
// HTML-only body materially raises the spam score (FR-TPL-1).
//
// SECURITY: HTML/Text contain the accept URL, which contains the invite token.
// This type deliberately has NO String() or LogValue() method, and no code path
// passes a Message to a logger (FR-OBS-2, design §6.6).
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Sender is the transport seam. The SMTP implementation and the in-memory fake
// both satisfy it; every test uses the fake.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// PermanentError marks a failure that retrying cannot fix: a relay rejecting
// the recipient address, or a malformed address caught before any dial. The
// consumer marks the ledger and stops — retrying a rejected mailbox forever is
// what wedges a partition (FR-MAIL-5).
//
// It wraps the SMTP error only, never the message body.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }
```

- [ ] **Step 4: Write the templates**

Create `apps/notification-service/internal/mailer/templates/invite.txt.tmpl`:

```
{{.Greeting}}

You've been invited to join {{.FleetLabel}} on MyFleet as a {{.Role}}.

Accept your invitation:
{{.AcceptURL}}

This link expires on {{.Expires}}.

If you weren't expecting this invitation, you can safely ignore this email.

-- MyFleet
```

Create `apps/notification-service/internal/mailer/templates/invite.html.tmpl`:

```html
<!doctype html>
<html lang="en">
  <body style="margin:0;padding:24px;background:#f6f7f9;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2328;">
    <table role="presentation" cellpadding="0" cellspacing="0" width="100%" style="max-width:560px;margin:0 auto;background:#ffffff;border-radius:8px;padding:32px;">
      <tr>
        <td>
          <h1 style="margin:0 0 16px;font-size:20px;line-height:1.3;">{{.Greeting}}</h1>
          <p style="margin:0 0 16px;font-size:15px;line-height:1.6;">
            You've been invited to join <strong>{{.FleetLabel}}</strong> on MyFleet as a <strong>{{.Role}}</strong>.
          </p>
          <p style="margin:0 0 24px;">
            <a href="{{.AcceptURL}}" style="display:inline-block;background:#1f6feb;color:#ffffff;text-decoration:none;padding:12px 20px;border-radius:6px;font-size:15px;font-weight:600;">Accept invitation</a>
          </p>
          <p style="margin:0 0 16px;font-size:13px;line-height:1.6;color:#57606a;">
            Or paste this link into your browser:<br />
            <a href="{{.AcceptURL}}" style="color:#1f6feb;word-break:break-all;">{{.AcceptURL}}</a>
          </p>
          <p style="margin:0 0 16px;font-size:13px;line-height:1.6;color:#57606a;">
            This link expires on {{.Expires}}.
          </p>
          <p style="margin:0;font-size:13px;line-height:1.6;color:#57606a;">
            If you weren't expecting this invitation, you can safely ignore this email.
          </p>
        </td>
      </tr>
    </table>
  </body>
</html>
```

- [ ] **Step 5: Write the renderer**

Create `apps/notification-service/internal/mailer/template.go`:

```go
package mailer

import (
	"bytes"
	_ "embed"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed templates/invite.html.tmpl
var inviteHTMLSource string

//go:embed templates/invite.txt.tmpl
var inviteTextSource string

// Parsed once at package init. template.Must is correct here: an unparseable
// embedded template is a build-time mistake, not a runtime condition.
var (
	inviteHTML = htmltemplate.Must(htmltemplate.New("invite.html").Parse(inviteHTMLSource))
	inviteText = texttemplate.Must(texttemplate.New("invite.txt").Parse(inviteTextSource))
)

// InviteData is the render input. Every field is already resolved — mailer does
// no lookups. FleetName may be empty; the copy degrades rather than rendering a
// hole (FR-TPL-3).
//
// SECURITY: AcceptURL contains the token. This struct is never logged whole.
type InviteData struct {
	To        string
	FleetName string
	Role      string
	AcceptURL string
	ExpiresAt time.Time
}

// viewModel is what the templates actually see: pre-resolved copy fragments, so
// neither template contains a conditional and both stay trivially reviewable.
type viewModel struct {
	Greeting   string
	FleetLabel string
	Role       string
	AcceptURL  htmltemplate.URL
	Expires    string
}

// RenderInvite produces both MIME parts plus the subject.
//
// Escaping is contextual by construction (FR-TPL-5): html/template for the HTML
// part, text/template for the plain part. FleetName and Role are user-influenced
// input and are escaped by the template engine, not by hand.
//
// AcceptURL is typed htmltemplate.URL so html/template passes an https:// URL
// through an href intact instead of mangling it. That is safe because
// PUBLIC_WEB_URL comes from config and never from an inbound request header
// (FR-TPL-2), so the scheme is trusted by construction.
func RenderInvite(d InviteData) (Message, error) {
	label := strings.TrimSpace(d.FleetName)
	subject := "You're invited to " + label + " on MyFleet"
	if label == "" {
		label = "a fleet"
		subject = "You're invited to a fleet on MyFleet"
	}

	vm := viewModel{
		Greeting:   "You've been invited to MyFleet",
		FleetLabel: label,
		Role:       d.Role,
		AcceptURL:  htmltemplate.URL(d.AcceptURL),
		Expires:    d.ExpiresAt.UTC().Format("January 2, 2006"),
	}

	var html, text bytes.Buffer
	if err := inviteHTML.Execute(&html, vm); err != nil {
		return Message{}, err
	}
	if err := inviteText.Execute(&text, vm); err != nil {
		return Message{}, err
	}

	return Message{To: d.To, Subject: subject, HTML: html.String(), Text: text.String()}, nil
}
```

> The text template receives `AcceptURL` as `htmltemplate.URL`, which
> `text/template` prints as its underlying string — no escaping, which is what
> the plain part needs.

- [ ] **Step 6: Run the rendering tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -run TestRenderInvite -v
```

Expected: PASS.

- [ ] **Step 7: Write the failing composition tests**

Create `apps/notification-service/internal/mailer/compose_test.go`:

```go
package mailer

import (
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func composeFixture(t *testing.T, msg Message) *mail.Message {
	t.Helper()
	raw, err := compose("MyFleet", "invites@myfleet.example.com", msg,
		time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
		"<abc-123@myfleet.example.com>", "BOUNDARY123")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("the composed message does not parse as RFC 5322: %v\n%s", err, raw)
	}
	return m
}

// FR-TPL-1: multipart/alternative with BOTH parts and all six required headers.
// Asserted by parsing the output back rather than by string matching, so a
// malformed-but-grep-passing message cannot slip through.
func TestCompose_isParseableMultipartAlternative(t *testing.T) {
	m := composeFixture(t, Message{
		To: "a@b.com", Subject: "You're invited to The Smiths on MyFleet",
		HTML: "<p>hello html</p>", Text: "hello text",
	})

	for _, h := range []string{"From", "To", "Subject", "Date", "Message-ID", "MIME-Version"} {
		if m.Header.Get(h) == "" {
			t.Errorf("missing required header %s", h)
		}
	}
	if got := m.Header.Get("From"); got != "MyFleet <invites@myfleet.example.com>" {
		t.Errorf("From=%q", got)
	}
	if got := m.Header.Get("To"); got != "a@b.com" {
		t.Errorf("To=%q", got)
	}
	if got := m.Header.Get("MIME-Version"); got != "1.0" {
		t.Errorf("MIME-Version=%q", got)
	}

	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type: %v", err)
	}
	if mediaType != "multipart/alternative" {
		t.Fatalf("mediaType=%q want multipart/alternative", mediaType)
	}

	types := map[string]string{}
	mr := multipart.NewReader(m.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		body, _ := io.ReadAll(p)
		mt, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		types[mt] = string(body)
	}

	if !strings.Contains(types["text/plain"], "hello text") {
		t.Errorf("text/plain part = %q", types["text/plain"])
	}
	if !strings.Contains(types["text/html"], "hello html") {
		t.Errorf("text/html part = %q", types["text/html"])
	}
	// Order matters: least-capable first, so a text-only client shows the
	// plain part rather than raw markup.
	if len(types) != 2 {
		t.Fatalf("want exactly 2 parts, got %d: %v", len(types), types)
	}
}

// The fleet name is user-controlled and reaches the Subject header. A non-ASCII
// name must be RFC 2047 encoded, and a CR/LF must not survive into headers even
// if one predates the invite-creation validation (design §6.3).
func TestCompose_encodesSubjectAndStripsHeaderInjection(t *testing.T) {
	m := composeFixture(t, Message{
		To: "a@b.com", Subject: "You're invited to Håkon's Garage\r\nBcc: victim@x.com on MyFleet",
		HTML: "<p>h</p>", Text: "t",
	})

	if m.Header.Get("Bcc") != "" {
		t.Fatal("header injection succeeded: a Bcc header was created")
	}
	raw := m.Header.Get("Subject")
	if strings.Contains(raw, "\r") || strings.Contains(raw, "\n") {
		t.Fatalf("raw CR/LF survived into the Subject header: %q", raw)
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(raw)
	if err != nil {
		t.Fatalf("decode subject: %v", err)
	}
	if !strings.Contains(decoded, "Håkon's Garage") {
		t.Fatalf("decoded subject lost the fleet name: %q", decoded)
	}
}

// Message-ID's domain half must match the From domain — a mismatch is a spam
// signal (design §6.3).
func TestNewMessageID_usesTheFromDomain(t *testing.T) {
	id, err := newMessageID("invites@myfleet.example.com")
	if err != nil {
		t.Fatalf("newMessageID: %v", err)
	}
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@myfleet.example.com>") {
		t.Fatalf("Message-ID=%q", id)
	}
}

// The boundary must be random per message so it cannot collide with body
// content, which would truncate the message at the collision.
func TestNewBoundary_isRandom(t *testing.T) {
	a, err := newBoundary()
	if err != nil {
		t.Fatalf("newBoundary: %v", err)
	}
	b, err := newBoundary()
	if err != nil {
		t.Fatalf("newBoundary: %v", err)
	}
	if a == b {
		t.Fatal("boundaries must differ between messages")
	}
	if len(a) < 16 {
		t.Fatalf("boundary %q is too short to be collision-resistant", a)
	}
}
```

- [ ] **Step 8: Run them to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -run 'TestCompose|TestNew' -v
```

Expected: FAIL to compile — `undefined: compose`, `newMessageID`, `newBoundary`.

- [ ] **Step 9: Write the composer**

Create `apps/notification-service/internal/mailer/compose.go`:

```go
package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"
)

// compose renders msg as an RFC 5322 multipart/alternative message.
//
// Hand-composed rather than pulled from a third-party mail library: the header
// set is small and fixed, and mime/multipart plus net/textproto cover the rest.
//
// date, messageID and boundary are parameters rather than generated inside, so
// the output is deterministic under test.
func compose(fromName, fromAddress string, msg Message, date time.Time, messageID, boundary string) ([]byte, error) {
	var buf bytes.Buffer

	from := fromAddress
	if n := sanitizeHeader(fromName); n != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", n), fromAddress)
	}

	headers := [][2]string{
		{"From", from},
		{"To", sanitizeHeader(msg.To)},
		// The fleet name reaches the Subject and is user-controlled, so it is
		// RFC 2047 encoded. QEncoding also neutralises any control character
		// that reached the database before invite-creation validation existed.
		{"Subject", mime.QEncoding.Encode("utf-8", sanitizeHeader(msg.Subject))},
		{"Date", date.Format(time.RFC1123Z)},
		{"Message-ID", sanitizeHeader(messageID)},
		{"MIME-Version", "1.0"},
		{"Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", boundary)},
	}
	for _, h := range headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", h[0], h[1])
	}
	buf.WriteString("\r\n")

	w := multipart.NewWriter(&buf)
	if err := w.SetBoundary(boundary); err != nil {
		return nil, err
	}

	// Least-capable part first, per RFC 2046: a text-only client renders the
	// plain part rather than raw markup.
	parts := []struct{ contentType, body string }{
		{`text/plain; charset="utf-8"`, msg.Text},
		{`text/html; charset="utf-8"`, msg.HTML},
	}
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", p.contentType)
		h.Set("Content-Transfer-Encoding", "8bit")
		pw, err := w.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := pw.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sanitizeHeader strips CR and LF from a header value. Invite-creation already
// rejects them (ValidateInviteEmail), and QEncoding would encode them anyway;
// this is the belt to that pair of braces, and it covers rows written before
// the validation existed.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// newBoundary returns a random MIME boundary. It must not collide with body
// content — a collision truncates the message at the collision point.
func newBoundary() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "myfleet-" + hex.EncodeToString(b), nil
}

// newMessageID builds a Message-ID whose domain half matches the From domain.
// A Message-ID whose domain does not match From is a spam signal.
func newMessageID(fromAddress string) (string, error) {
	at := strings.LastIndex(fromAddress, "@")
	if at < 0 || at == len(fromAddress)-1 {
		return "", fmt.Errorf("from address %q has no domain", fromAddress)
	}
	return fmt.Sprintf("<%s@%s>", uuid.NewString(), fromAddress[at+1:]), nil
}
```

- [ ] **Step 10: Write the fake sender**

Create `apps/notification-service/internal/mailer/fake.go`:

```go
package mailer

import "context"

// FakeSender records what would have been sent and can be programmed to fail.
// Exported because mailconsumer's tests live in a different package.
//
// Errs is consumed one entry per call and takes precedence over Err, which lets
// a test express "fail twice, then succeed" — the shape the retry test needs.
type FakeSender struct {
	Sent []Message
	Err  error
	Errs []error
}

func (f *FakeSender) Send(_ context.Context, msg Message) error {
	f.Sent = append(f.Sent, msg)
	if len(f.Errs) > 0 {
		err := f.Errs[0]
		f.Errs = f.Errs[1:]
		return err
	}
	return f.Err
}

// Calls reports how many Send attempts were made, including failed ones.
func (f *FakeSender) Calls() int { return len(f.Sent) }
```

- [ ] **Step 11: Run the whole package to verify it passes**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -v
```

Expected: PASS. If `github.com/google/uuid` is not yet a direct dependency of notification-service,
run `(cd apps/notification-service && go mod tidy)` first.

- [ ] **Step 12: Commit**

```sh
git add apps/notification-service/internal/mailer/ apps/notification-service/go.mod apps/notification-service/go.sum go.work.sum
git commit -m "feat(notification-service): add the mailer package with invite rendering and MIME composition"
```

---

## Task 10: SMTP transport, configuration, and metrics

**Files:**
- Create: `apps/notification-service/internal/mailer/config.go`
- Create: `apps/notification-service/internal/mailer/smtp.go`
- Create: `apps/notification-service/internal/mailer/metrics.go`
- Test: `apps/notification-service/internal/mailer/config_test.go`
- Test: `apps/notification-service/internal/mailer/smtp_test.go`

**Interfaces:**
- Consumes: `Message`, `PermanentError`, `compose`, `newBoundary`, `newMessageID` (Task 9).
- Produces:
  - `mailer.Config struct { Enabled bool; Host string; Port int; TLSMode string; FromAddress, FromName, Username, Password, PublicWebURL string; Timeout time.Duration; SendAttempts int; RetryBase time.Duration }`
  - `mailer.ConfigFromEnv() Config` — panics at startup on a bad or missing required value
  - `mailer.NewSMTPSender(cfg Config) Sender`
  - `mailer.TLSModeStartTLS = "starttls"`, `TLSModeTLS = "tls"`, `TLSModeNone = "none"`
  - `mailer.OutcomeSent/OutcomeFailedTransient/OutcomeFailedPermanent/OutcomeSkippedDisabled/OutcomeSkippedStale` string constants
  - `mailer.RecordOutcome(outcome string)`

No test in this task dials a socket. `NewSMTPSender` is exercised only through `ConfigFromEnv`
validation and the error-classification helper; live delivery is verified by hand against Mailpit
(PRD §10).

- [ ] **Step 1: Write the failing config tests**

Create `apps/notification-service/internal/mailer/config_test.go`:

```go
package mailer

import (
	"testing"
	"time"
)

// FR-CFG-4: disabled is the default, and a disabled config reads nothing else.
// A cluster with no relay credentials must be a no-op, not a crash loop.
func TestConfigFromEnv_disabledByDefault(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.Enabled {
		t.Fatal("SMTP must default to disabled")
	}
}

func TestConfigFromEnv_enabledReadsEverything(t *testing.T) {
	t.Setenv("SMTP_ENABLED", "true")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_TLS_MODE", "starttls")
	t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
	t.Setenv("SMTP_FROM_NAME", "MyFleet")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "pass")
	t.Setenv("PUBLIC_WEB_URL", "https://example.com")

	cfg := ConfigFromEnv()
	if !cfg.Enabled || cfg.Host != "smtp.example.com" || cfg.Port != 587 {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.TLSMode != TLSModeStartTLS || cfg.FromAddress != "invites@example.com" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.PublicWebURL != "https://example.com" {
		t.Fatalf("cfg=%+v", cfg)
	}
	// Defaults that keep a black-holed relay from hanging the consumer.
	if cfg.Timeout != 10*time.Second || cfg.SendAttempts != 4 || cfg.RetryBase != 2*time.Second {
		t.Fatalf("retry defaults wrong: %+v", cfg)
	}
}

// FR-CFG-5: a missing required key is a STARTUP failure, not a per-message
// failure discovered hours later.
func TestConfigFromEnv_missingRequiredKeyPanics(t *testing.T) {
	for _, missing := range []string{"SMTP_HOST", "SMTP_FROM_ADDRESS", "PUBLIC_WEB_URL"} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("SMTP_ENABLED", "true")
			t.Setenv("SMTP_HOST", "smtp.example.com")
			t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
			t.Setenv("PUBLIC_WEB_URL", "https://example.com")
			t.Setenv("SMTP_TLS_MODE", "starttls")
			t.Setenv("SMTP_USERNAME", "user")
			t.Setenv("SMTP_PASSWORD", "pass")
			t.Setenv(missing, "")

			defer func() {
				if recover() == nil {
					t.Fatalf("missing %s must panic at startup", missing)
				}
			}()
			ConfigFromEnv()
		})
	}
}

// Open Q3: the mode set is closed. An unknown value is a startup panic, not a
// runtime surprise on the first invite.
func TestConfigFromEnv_rejectsUnknownTLSMode(t *testing.T) {
	t.Setenv("SMTP_ENABLED", "true")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
	t.Setenv("PUBLIC_WEB_URL", "https://example.com")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "pass")
	t.Setenv("SMTP_TLS_MODE", "ssl")

	defer func() {
		if recover() == nil {
			t.Fatal("an unknown SMTP_TLS_MODE must panic at startup")
		}
	}()
	ConfigFromEnv()
}

// Empty credentials against a real relay mean every message is rejected. Fail
// at startup instead. They are legal only for the plaintext local relay.
func TestConfigFromEnv_credentialsRequiredUnlessModeIsNone(t *testing.T) {
	base := func(t *testing.T, mode string) {
		t.Setenv("SMTP_ENABLED", "true")
		t.Setenv("SMTP_HOST", "smtp.example.com")
		t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
		t.Setenv("PUBLIC_WEB_URL", "https://example.com")
		t.Setenv("SMTP_TLS_MODE", mode)
		t.Setenv("SMTP_USERNAME", "")
		t.Setenv("SMTP_PASSWORD", "")
	}

	t.Run("starttls without credentials panics", func(t *testing.T) {
		base(t, TLSModeStartTLS)
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()
		ConfigFromEnv()
	})

	t.Run("none without credentials is legal", func(t *testing.T) {
		base(t, TLSModeNone)
		cfg := ConfigFromEnv()
		if cfg.Username != "" || cfg.TLSMode != TLSModeNone {
			t.Fatalf("cfg=%+v", cfg)
		}
	})
}
```

- [ ] **Step 2: Run them to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -run TestConfigFromEnv -v
```

Expected: FAIL to compile — `undefined: ConfigFromEnv`, `TLSModeStartTLS`.

- [ ] **Step 3: Write the config**

Create `apps/notification-service/internal/mailer/config.go`:

```go
package mailer

import (
	"fmt"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

// The closed set of TLS modes (Open Q3, design §6.2).
//
//	starttls — plaintext connect, STARTTLS, FAIL if the server does not offer it
//	tls      — implicit TLS from the first byte
//	none     — plaintext; the ONLY mode that permits an unauthenticated session,
//	           and legal only for a local relay (Mailpit). tools/check-manifests.sh
//	           keeps it out of the main overlay (FR-DEV-2).
const (
	TLSModeStartTLS = "starttls"
	TLSModeTLS      = "tls"
	TLSModeNone     = "none"
)

// Config is the mail transport configuration. Every value is read through
// packages/shared-go/config; nothing here calls os.Getenv (FR-CFG-3).
type Config struct {
	Enabled      bool
	Host         string
	Port         int
	TLSMode      string
	FromAddress  string
	FromName     string
	Username     string
	Password     string
	PublicWebURL string
	Timeout      time.Duration
	SendAttempts int
	RetryBase    time.Duration
}

// ConfigFromEnv builds the transport config, failing at STARTUP on anything
// misconfigured rather than surfacing it as a per-message failure hours later
// (FR-CFG-5).
//
// When SMTP_ENABLED is false it reads nothing else and returns a disabled
// config: a cluster without relay credentials is a documented no-op, not a
// crash loop or a retry storm (FR-CFG-4).
func ConfigFromEnv() Config {
	if config.Get("SMTP_ENABLED", "false") != "true" {
		return Config{Enabled: false}
	}

	cfg := Config{
		Enabled:      true,
		Host:         config.MustGet("SMTP_HOST"),
		Port:         config.GetInt("SMTP_PORT", 587),
		TLSMode:      config.Get("SMTP_TLS_MODE", TLSModeStartTLS),
		FromAddress:  config.MustGet("SMTP_FROM_ADDRESS"),
		FromName:     config.Get("SMTP_FROM_NAME", "MyFleet"),
		Username:     config.Get("SMTP_USERNAME", ""),
		Password:     config.Get("SMTP_PASSWORD", ""),
		PublicWebURL: config.MustGet("PUBLIC_WEB_URL"),
		Timeout:      time.Duration(config.GetInt("SMTP_TIMEOUT_SECONDS", 10)) * time.Second,
		SendAttempts: config.GetInt("SMTP_SEND_ATTEMPTS", 4),
		RetryBase:    time.Duration(config.GetInt("SMTP_RETRY_BASE_SECONDS", 2)) * time.Second,
	}

	switch cfg.TLSMode {
	case TLSModeStartTLS, TLSModeTLS, TLSModeNone:
	default:
		panic(fmt.Sprintf("SMTP_TLS_MODE %q must be one of %q, %q, %q",
			cfg.TLSMode, TLSModeStartTLS, TLSModeTLS, TLSModeNone))
	}

	// Unauthenticated submission to a real relay is rejected on every message.
	// Fail now rather than emitting one permanent failure per invite forever.
	if cfg.TLSMode != TLSModeNone && (cfg.Username == "" || cfg.Password == "") {
		panic("SMTP_USERNAME and SMTP_PASSWORD are required unless SMTP_TLS_MODE is \"none\"")
	}
	if cfg.SendAttempts < 1 {
		panic("SMTP_SEND_ATTEMPTS must be at least 1")
	}

	return cfg
}
```

- [ ] **Step 4: Run the config tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -run TestConfigFromEnv -v
```

Expected: PASS.

- [ ] **Step 5: Write the failing classification test**

Create `apps/notification-service/internal/mailer/smtp_test.go`:

```go
package mailer

import (
	"errors"
	"net/textproto"
	"testing"
)

// FR-MAIL-5. Classification decides whether the consumer burns four attempts or
// gives up after one, so it is asserted directly rather than inferred from
// consumer behaviour.
func TestClassify(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		permanent bool
	}{
		{"5xx recipient rejected", &textproto.Error{Code: 550, Msg: "no such user"}, true},
		{"5xx syntax", &textproto.Error{Code: 501, Msg: "bad address"}, true},
		{"4xx greylisting", &textproto.Error{Code: 451, Msg: "try again later"}, false},
		{"4xx mailbox busy", &textproto.Error{Code: 450, Msg: "busy"}, false},
		{"dial failure", errors.New("dial tcp: connection refused"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classify(c.err)
			if c.err == nil {
				if got != nil {
					t.Fatalf("classify(nil)=%v want nil", got)
				}
				return
			}
			var perm *PermanentError
			if isPerm := errors.As(got, &perm); isPerm != c.permanent {
				t.Fatalf("classify(%v) permanent=%v want %v", c.err, isPerm, c.permanent)
			}
			// The original error must remain inspectable either way.
			if !errors.Is(got, c.err) {
				t.Fatalf("classify lost the underlying error: %v", got)
			}
		})
	}
}
```

- [ ] **Step 6: Run it to verify it fails**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -run TestClassify -v
```

Expected: FAIL to compile — `undefined: classify`.

- [ ] **Step 7: Write the SMTP sender**

Create `apps/notification-service/internal/mailer/smtp.go`:

```go
package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"net/textproto"
	"time"
)

type smtpSender struct{ cfg Config }

// NewSMTPSender returns a Sender that delivers over SMTP.
//
// TLS verification is ON in both TLS modes and there is no skip-verify escape
// hatch in config or code (FR-MAIL-2).
func NewSMTPSender(cfg Config) Sender { return &smtpSender{cfg: cfg} }

// Send composes and delivers one message. Errors are classified so the caller
// can tell a retryable blip from a rejected mailbox (FR-MAIL-5); the message
// body is never attached to an error (FR-OBS-2).
func (s *smtpSender) Send(ctx context.Context, msg Message) error {
	boundary, err := newBoundary()
	if err != nil {
		return err
	}
	messageID, err := newMessageID(s.cfg.FromAddress)
	if err != nil {
		// A From address with no domain is a configuration error that no retry
		// fixes.
		return &PermanentError{Err: err}
	}
	raw, err := compose(s.cfg.FromName, s.cfg.FromAddress, msg, time.Now(), messageID, boundary)
	if err != nil {
		return &PermanentError{Err: err}
	}

	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	c, err := s.dial(ctx)
	if err != nil {
		return classify(err)
	}
	defer func() { _ = c.Close() }()

	if err := s.authenticate(c); err != nil {
		return classify(err)
	}
	return classify(deliver(c, s.cfg.FromAddress, msg.To, raw))
}

// dial opens a connection appropriate to the configured TLS mode.
func (s *smtpSender) dial(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	// Timeout on the dialer as well as the context: tls.DialWithDialer takes no
	// context, so without this a black-holed relay hangs the implicit-TLS path.
	d := &net.Dialer{Timeout: s.cfg.Timeout}

	if s.cfg.TLSMode == TLSModeTLS {
		conn, err := tls.DialWithDialer(d, "tcp", addr, &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return nil, err
		}
		return smtp.NewClient(conn, s.cfg.Host)
	}

	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	if s.cfg.TLSMode == TLSModeStartTLS {
		// Erroring when the server does not advertise STARTTLS is the whole
		// point: silently continuing in plaintext is the classic downgrade.
		if ok, _ := c.Extension("STARTTLS"); !ok {
			_ = c.Close()
			return nil, errors.New("relay does not offer STARTTLS and SMTP_TLS_MODE is starttls")
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
			_ = c.Close()
			return nil, err
		}
	}
	return c, nil
}

// authenticate is a no-op when no credentials are configured, which is legal
// only for the plaintext local relay (ConfigFromEnv enforces that).
func (s *smtpSender) authenticate(c *smtp.Client) error {
	if s.cfg.Username == "" {
		return nil
	}
	return c.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host))
}

func deliver(c *smtp.Client, from, to string, raw []byte) error {
	if err := c.Mail(from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// classify turns a transport error into the taxonomy the consumer branches on
// (FR-MAIL-5):
//
//	5xx on MAIL FROM / RCPT TO / DATA → PermanentError (rejected recipient,
//	  malformed address). Retrying a rejected mailbox forever wedges a partition.
//	4xx (greylisting, mailbox busy), dial failures, TLS handshake failures and
//	  timeouts → returned bare, i.e. transient.
func classify(err error) error {
	if err == nil {
		return nil
	}
	var te *textproto.Error
	if errors.As(err, &te) && te.Code >= 500 && te.Code < 600 {
		return &PermanentError{Err: err}
	}
	return err
}
```

- [ ] **Step 8: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer/... -v
```

Expected: PASS.

- [ ] **Step 9: Add the metric**

This is the **first custom Prometheus metric in the repo** — only `promhttp.Handler()` exists today
(`packages/shared-go/health`). Declaring it with `promauto` on the default registry means the
existing `/metrics` route (`notification-service/cmd/main.go:83`) exposes it with no extra wiring.

Create `apps/notification-service/internal/mailer/metrics.go`:

```go
package mailer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Outcomes for myfleet_invite_emails_total (FR-OBS-1). Constants rather than
// bare strings so a typo in a rarely-hit branch cannot silently create a new
// time series that no dashboard ever shows.
const (
	OutcomeSent            = "sent"
	OutcomeFailedTransient = "failed_transient"
	OutcomeFailedPermanent = "failed_permanent"
	OutcomeSkippedDisabled = "skipped_disabled"
	OutcomeSkippedStale    = "skipped_stale"
)

// Registered on the default registry, which the existing /metrics route already
// serves — no wiring needed in main.
var inviteEmails = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "myfleet_invite_emails_total",
	Help: "Invite emails by outcome.",
}, []string{"outcome"})

// RecordOutcome increments the invite-email counter. Exported so mailconsumer,
// which owns the outcome decisions, does not import prometheus itself.
func RecordOutcome(outcome string) { inviteEmails.WithLabelValues(outcome).Inc() }
```

- [ ] **Step 10: Promote prometheus to a direct dependency and verify**

```sh
(cd apps/notification-service && go mod tidy)
go build github.com/jtumidanski/myfleet/... && go test github.com/jtumidanski/myfleet/apps/notification-service/... -v
```

Expected: PASS, and `apps/notification-service/go.mod` no longer marks
`github.com/prometheus/client_golang` `// indirect`.

- [ ] **Step 11: Commit**

```sh
git add apps/notification-service/internal/mailer/ apps/notification-service/go.mod apps/notification-service/go.sum go.work.sum
git commit -m "feat(notification-service): add SMTP transport, config validation and the invite-email metric"
```

---

## Task 11: The `mailconsumer` package

**This is a deliberate deviation from FR-MAIL-3's literal wording**, satisfying its stated intent
("an SMTP failure cannot cause the in-app notification path to be re-run and vice versa") more
completely. FR-MAIL-3 says to add `invite.created` to `consumer.Topics` with a distinct ledger name;
taken literally that wires email into `*consumer.Consumer`, which resolves fleet recipients and
generates in-app notifications — none of which an invite email wants. Here the **Kafka group** is
separate too (`invite-email`), so offsets are independent and a stalled email consumer cannot hold
back in-app notification offsets. Flagged so the plan-adherence reviewer scores it against intent.
Design §3.

**Retry exists here and not in `events.Consume` because `Consume` does not retry.** Its
`continue`-without-commit is documented as "will retry", but kafka-go advances to the next message
and commits *that* offset, implicitly committing past the failed one. Fixing `Consume` would change
semantics for all four existing topics. Design §5.1.

**Files:**
- Create: `apps/notification-service/internal/mailconsumer/consume.go`
- Test: `apps/notification-service/internal/mailconsumer/consume_test.go`

**Interfaces:**
- Consumes: `inbox.Store` (`Exists`/`Mark`), `fleetclient.Client.Invite` + `ErrInviteNotFound` (Task 8), `mailer.{Config, Sender, RenderInvite, InviteData, PermanentError, RecordOutcome, Outcome*}` (Tasks 9–10).
- Produces:
  - `mailconsumer.Topic = "invite.created"`
  - `mailconsumer.Inbox`, `mailconsumer.Invites` interfaces
  - `mailconsumer.NewConsumer(log logrus.FieldLogger, inbox Inbox, invites Invites, sender mailer.Sender, cfg mailer.Config) *Consumer`
  - `(*Consumer).WithSleep(fn func(context.Context, time.Duration) error) *Consumer`
  - `(*Consumer).Run(ctx context.Context, brokers []string)`
  - `(*Consumer).Handle(ctx context.Context, e events.Envelope) error`

**Backoff — a documented refinement of design §5.2.** The design lists "2s, 8s, 30s"; this plan uses
the single formula `RetryBase × 4^(attempt-1)` → **2s, 8s, 32s** (≈42s total, which is the "~40s"
design §5.2 calls for). One formula is assertable as a schedule; three hand-written constants are
not.

- [ ] **Step 1: Write the failing tests**

Create `apps/notification-service/internal/mailconsumer/consume_test.go`:

```go
package mailconsumer

import (
	"context"
	"errors"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"

	"github.com/jtumidanski/myfleet/packages/shared-go/events"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer"
)

const testToken = "deadbeefcafe0123"

type fakeInbox struct{ seen map[string]bool }

func (f *fakeInbox) Exists(eventID, consumer string) (bool, error) {
	return f.seen[eventID+":"+consumer], nil
}

func (f *fakeInbox) Mark(eventID, consumer string) error {
	f.seen[eventID+":"+consumer] = true
	return nil
}

type fakeInvites struct {
	inv   fleetclient.Invite
	err   error
	calls int
}

func (f *fakeInvites) Invite(context.Context, string) (fleetclient.Invite, error) {
	f.calls++
	return f.inv, f.err
}

func liveInvite() fleetclient.Invite {
	return fleetclient.Invite{
		InviteID: "inv-1", FleetID: "f1", FleetName: "The Smiths",
		Email: "a@b.com", Role: "member", Token: testToken,
		ExpiresAt: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		InvitedByUserID: "u1",
	}
}

func enabledConfig() mailer.Config {
	return mailer.Config{
		Enabled: true, PublicWebURL: "https://myfleet.example.com",
		SendAttempts: 4, RetryBase: 2 * time.Second, FromAddress: "invites@myfleet.example.com",
	}
}

func envelope() events.Envelope {
	return events.Envelope{
		EventID: "evt-1", Type: "invite.created", FleetID: "f1", TraceID: "trace-1",
		Data: map[string]any{"invite_id": "inv-1", "email": "a@b.com", "role": "member"},
	}
}

// newTestConsumer wires a consumer whose sleep is instant, so a backoff test
// asserts the SCHEDULE in microseconds instead of sleeping 42 seconds in CI.
func newTestConsumer(t *testing.T, inv *fakeInvites, s mailer.Sender, cfg mailer.Config) (*Consumer, *fakeInbox, *[]time.Duration) {
	t.Helper()
	ib := &fakeInbox{seen: map[string]bool{}}
	var slept []time.Duration
	c := NewConsumer(logrus.New(), ib, inv, s, cfg).
		WithSleep(func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			return nil
		})
	return c, ib, &slept
}

func TestHandle_sendsOneEmailWithAWorkingAcceptLink(t *testing.T) {
	sender := &mailer.FakeSender{}
	c, ib, _ := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(sender.Sent) != 1 {
		t.Fatalf("want 1 send, got %d", len(sender.Sent))
	}
	got := sender.Sent[0]
	if got.To != "a@b.com" {
		t.Fatalf("To=%q", got.To)
	}
	// FR-TPL-2: the link is built from PUBLIC_WEB_URL and matches the SPA route
	// at apps/web/src/pages/InviteAcceptPage.tsx.
	want := "https://myfleet.example.com/invites/" + testToken + "/accept"
	if !strings.Contains(got.Text, want) || !strings.Contains(got.HTML, want) {
		t.Fatalf("accept URL missing from one of the parts:\n%s\n%s", got.Text, got.HTML)
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("the ledger must be marked after a successful send")
	}
}

// Acceptance criterion 3.
func TestHandle_isIdempotentByEventID(t *testing.T) {
	sender := &mailer.FakeSender{}
	c, _, _ := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	for i := 0; i < 2; i++ {
		if err := c.Handle(context.Background(), envelope()); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	if len(sender.Sent) != 1 {
		t.Fatalf("replaying one event_id must send exactly once, got %d", len(sender.Sent))
	}
}

// FR-CFG-4: the disabled check comes BEFORE the fleet-service fetch, so a
// cluster with no relay configured makes no network calls at all.
func TestHandle_disabledMakesNoCalls(t *testing.T) {
	sender := &mailer.FakeSender{}
	invites := &fakeInvites{inv: liveInvite()}
	cfg := enabledConfig()
	cfg.Enabled = false
	c, ib, _ := newTestConsumer(t, invites, sender, cfg)

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if len(sender.Sent) != 0 {
		t.Fatal("a disabled mailer must not send")
	}
	if invites.calls != 0 {
		t.Fatal("a disabled mailer must not call fleet-service")
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("a skipped event must still be marked, or it is reprocessed forever")
	}
}

// FR-MAIL-4: a delayed redelivery must not mail a dead link.
func TestHandle_skipsAcceptedAndExpired(t *testing.T) {
	accepted := time.Now().UTC().Format(time.RFC3339)
	stale := []struct {
		name string
		inv  fleetclient.Invite
	}{
		{"accepted", func() fleetclient.Invite { i := liveInvite(); i.AcceptedAt = &accepted; return i }()},
		{"expired", func() fleetclient.Invite {
			i := liveInvite()
			i.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
			return i
		}()},
	}
	for _, tc := range stale {
		t.Run(tc.name, func(t *testing.T) {
			sender := &mailer.FakeSender{}
			c, ib, _ := newTestConsumer(t, &fakeInvites{inv: tc.inv}, sender, enabledConfig())
			if err := c.Handle(context.Background(), envelope()); err != nil {
				t.Fatalf("Handle: %v", err)
			}
			if len(sender.Sent) != 0 {
				t.Fatal("a stale invite must not be mailed")
			}
			if !ib.seen["evt-1:"+consumerName] {
				t.Fatal("a skipped event must still be marked")
			}
		})
	}
}

// FR-MAIL-5 transient half + design §5.2: bounded attempts with backoff, then
// mark. Marking on exhaustion is the uncomfortable half — see the plan's note.
func TestHandle_transientFailureRetriesThenGivesUp(t *testing.T) {
	sender := &mailer.FakeSender{Err: errors.New("dial tcp: connection refused")}
	c, ib, slept := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle must not return an error after a bounded give-up: %v", err)
	}
	if sender.Calls() != 4 {
		t.Fatalf("want 4 attempts, got %d", sender.Calls())
	}
	want := []time.Duration{2 * time.Second, 8 * time.Second, 32 * time.Second}
	if len(*slept) != len(want) {
		t.Fatalf("want %d backoffs, got %v", len(want), *slept)
	}
	for i, d := range want {
		if (*slept)[i] != d {
			t.Fatalf("backoff[%d]=%v want %v (schedule %v)", i, (*slept)[i], d, *slept)
		}
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("exhausted retries must mark the ledger; see design §5.2")
	}
}

// A transient failure that clears must not burn the whole budget.
func TestHandle_transientFailureThatRecoversSendsOnce(t *testing.T) {
	sender := &mailer.FakeSender{Errs: []error{errors.New("timeout"), nil}}
	c, _, slept := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if sender.Calls() != 2 {
		t.Fatalf("want 2 attempts, got %d", sender.Calls())
	}
	if len(*slept) != 1 {
		t.Fatalf("want 1 backoff, got %v", *slept)
	}
}

// FR-MAIL-5 permanent half: exactly ONE attempt. Retrying a rejected mailbox is
// what wedges a partition.
func TestHandle_permanentFailureAttemptsOnce(t *testing.T) {
	sender := &mailer.FakeSender{Err: &mailer.PermanentError{Err: &textproto.Error{Code: 550, Msg: "no such user"}}}
	c, ib, slept := newTestConsumer(t, &fakeInvites{inv: liveInvite()}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if sender.Calls() != 1 {
		t.Fatalf("want exactly 1 attempt, got %d", sender.Calls())
	}
	if len(*slept) != 0 {
		t.Fatalf("a permanent failure must not back off, got %v", *slept)
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("a permanent failure must mark the ledger")
	}
}

// A deleted invite will never come back; four lookups against it are waste.
func TestHandle_inviteNotFoundIsPermanent(t *testing.T) {
	sender := &mailer.FakeSender{}
	invites := &fakeInvites{err: fleetclient.ErrInviteNotFound}
	c, ib, _ := newTestConsumer(t, invites, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if invites.calls != 1 {
		t.Fatalf("want 1 lookup, got %d", invites.calls)
	}
	if len(sender.Sent) != 0 {
		t.Fatal("nothing to send")
	}
	if !ib.seen["evt-1:"+consumerName] {
		t.Fatal("a 404 must mark the ledger")
	}
}

// Any other fleet-service failure is transient: return the error UNMARKED so a
// redelivery can still do the work.
func TestHandle_lookupFailureReturnsErrorUnmarked(t *testing.T) {
	sender := &mailer.FakeSender{}
	c, ib, _ := newTestConsumer(t, &fakeInvites{err: errors.New("502")}, sender, enabledConfig())

	if err := c.Handle(context.Background(), envelope()); err == nil {
		t.Fatal("a transient lookup failure must surface as an error")
	}
	if ib.seen["evt-1:"+consumerName] {
		t.Fatal("a transient lookup failure must NOT mark the ledger")
	}
}

// FR-OBS-2 / acceptance criterion 9, made mechanical rather than a hand grep:
// the token appears in no log entry — message OR field — across a full failing
// Handle, which is the path most likely to dump a message.
func TestHandle_neverLogsTheToken(t *testing.T) {
	log, hook := logrustest.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)

	ib := &fakeInbox{seen: map[string]bool{}}
	sender := &mailer.FakeSender{Err: errors.New("dial tcp: connection refused")}
	c := NewConsumer(log, ib, &fakeInvites{inv: liveInvite()}, sender, enabledConfig()).
		WithSleep(func(context.Context, time.Duration) error { return nil })

	if err := c.Handle(context.Background(), envelope()); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	if len(hook.AllEntries()) == 0 {
		t.Fatal("expected the failure path to log something")
	}
	for _, e := range hook.AllEntries() {
		if strings.Contains(e.Message, testToken) {
			t.Fatalf("token leaked into a log message: %s", e.Message)
		}
		for k, v := range e.Data {
			if strings.Contains(strings.ToLower(fmtValue(v)), testToken) {
				t.Fatalf("token leaked into log field %q: %v", k, v)
			}
		}
		// The accept URL contains the token, so it is banned outright.
		if strings.Contains(e.Message, "/invites/") {
			t.Fatalf("an accept URL leaked into a log message: %s", e.Message)
		}
	}
}

func fmtValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if err, ok := v.(error); ok {
		return err.Error()
	}
	return ""
}
```

- [ ] **Step 2: Run them to verify they fail**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer/... -v
```

Expected: FAIL — no such package.

- [ ] **Step 3: Write the consumer**

Create `apps/notification-service/internal/mailconsumer/consume.go`:

```go
// Package mailconsumer turns one invite.created event into exactly one invite
// email, idempotently.
//
// It is deliberately SEPARATE from internal/consumer, and not merely a second
// ledger name on it (design §3). The Kafka consumer group is separate too, so
// offsets are independent: a stalled email consumer cannot hold back in-app
// notification offsets, and an SMTP failure cannot cause the in-app
// notification path to be re-run or vice versa — which is FR-MAIL-3's stated
// intent, satisfied more completely than its literal wording.
package mailconsumer

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/events"

	"github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer"
)

// consumerName is BOTH the Kafka consumer group and the ledger key, so the two
// cannot drift apart.
const consumerName = "invite-email"

// Topic is the single topic this consumer subscribes to. Deliberately NOT added
// to consumer.Topics: the in-app consumer would only mark-and-skip it.
const Topic = "invite.created"

// Inbox is the idempotency ledger surface (satisfied by *inbox.Store).
type Inbox interface {
	Exists(eventID, consumer string) (bool, error)
	Mark(eventID, consumer string) error
}

// Invites fetches one invite including its token (satisfied by *fleetclient.Client).
type Invites interface {
	Invite(ctx context.Context, inviteID string) (fleetclient.Invite, error)
}

// Consumer sends one invite email per invite.created event.
type Consumer struct {
	log     logrus.FieldLogger
	inbox   Inbox
	invites Invites
	sender  mailer.Sender
	cfg     mailer.Config
	sleep   func(context.Context, time.Duration) error
}

// NewConsumer constructs a Consumer with its collaborators injected.
func NewConsumer(log logrus.FieldLogger, inbox Inbox, invites Invites, sender mailer.Sender, cfg mailer.Config) *Consumer {
	return &Consumer{log: log, inbox: inbox, invites: invites, sender: sender, cfg: cfg, sleep: sleepCtx}
}

// WithSleep replaces the backoff sleep. Tests inject an instant one so the retry
// test asserts the SCHEDULE in microseconds rather than sleeping ~42s in CI.
func (c *Consumer) WithSleep(fn func(context.Context, time.Duration) error) *Consumer {
	c.sleep = fn
	return c
}

// sleepCtx waits d, or returns early when ctx is cancelled so shutdown is prompt.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Run blocks consuming Topic in the "invite-email" group until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, brokers []string) {
	events.Consume(ctx, c.log, brokers, consumerName, Topic, c.Handle)
}

// Handle processes one invite.created event.
//
//  1. Read-only dedupe check on (event_id, "invite-email").
//  2. Skip if sending is disabled — BEFORE any network call (FR-CFG-4).
//  3. Fetch the invite, including the token, over internal HTTP. The token is
//     deliberately absent from the event; fetching it also means a delayed or
//     duplicated delivery mails the CURRENT token after a resend rotation.
//  4. Skip if already accepted or already expired (FR-MAIL-4).
//  5. Send with bounded backoff.
//  6. Mark the ledger.
//
// Mark-after-success means a crash between a successful Send and Mark produces
// one duplicate email on the next delivery. PRD §8 accepts that explicitly:
// marking first would silently DROP mail, which is strictly worse.
func (c *Consumer) Handle(ctx context.Context, e events.Envelope) error {
	log := c.log.WithFields(logrus.Fields{
		"event_id": e.EventID,
		"fleet_id": e.FleetID,
		"trace_id": e.TraceID,
	})

	seen, err := c.inbox.Exists(e.EventID, consumerName)
	if err != nil {
		return fmt.Errorf("dedupe check: %w", err)
	}
	if seen {
		log.Debug("invite email already sent for this event; skipping")
		return nil
	}

	if !c.cfg.Enabled {
		log.Debug("SMTP disabled; recording and skipping")
		mailer.RecordOutcome(mailer.OutcomeSkippedDisabled)
		return c.inbox.Mark(e.EventID, consumerName)
	}

	inviteID, _ := e.Data["invite_id"].(string)
	if inviteID == "" {
		log.Warn("invite.created carries no invite_id; recording and skipping")
		mailer.RecordOutcome(mailer.OutcomeSkippedStale)
		return c.inbox.Mark(e.EventID, consumerName)
	}
	log = log.WithField("invite_id", inviteID)

	inv, err := c.invites.Invite(ctx, inviteID)
	if err != nil {
		// A deleted invite will never come back — retrying is pure waste.
		if errors.Is(err, fleetclient.ErrInviteNotFound) {
			log.Warn("invite no longer exists; recording and skipping")
			mailer.RecordOutcome(mailer.OutcomeSkippedStale)
			return c.inbox.Mark(e.EventID, consumerName)
		}
		// Anything else is transient: leave UNMARKED so a redelivery retries.
		return fmt.Errorf("fetch invite %s: %w", inviteID, err)
	}

	if stale, reason := staleness(inv, time.Now()); stale {
		log.WithField("reason", reason).Info("invite is no longer live; not mailing a dead link")
		mailer.RecordOutcome(mailer.OutcomeSkippedStale)
		return c.inbox.Mark(e.EventID, consumerName)
	}

	msg, err := c.render(inv)
	if err != nil {
		// A render failure is deterministic; retrying reproduces it exactly.
		log.WithError(err).Error("could not render the invite email")
		mailer.RecordOutcome(mailer.OutcomeFailedPermanent)
		return c.inbox.Mark(e.EventID, consumerName)
	}

	c.send(ctx, log, msg)
	return c.inbox.Mark(e.EventID, consumerName)
}

// render builds the message. The accept URL matches the SPA route at
// apps/web/src/pages/InviteAcceptPage.tsx and is built from PUBLIC_WEB_URL,
// never from an inbound request header (FR-TPL-2). PathEscape is a no-op for
// today's 64-hex-char token and a guard if generation ever changes.
func (c *Consumer) render(inv fleetclient.Invite) (mailer.Message, error) {
	expires, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		return mailer.Message{}, fmt.Errorf("parse expires_at: %w", err)
	}
	acceptURL := fmt.Sprintf("%s/invites/%s/accept",
		strings.TrimRight(c.cfg.PublicWebURL, "/"), url.PathEscape(inv.Token))

	return mailer.RenderInvite(mailer.InviteData{
		To:        inv.Email,
		FleetName: inv.FleetName,
		Role:      inv.Role,
		AcceptURL: acceptURL,
		ExpiresAt: expires,
	})
}

// send attempts delivery with bounded backoff (design §5.2). It never returns
// an error: on exhaustion the caller marks the ledger regardless.
//
// Marking after exhausted retries means a relay outage longer than the budget
// drops that invite's email permanently. That is chosen because the alternative
// under the events.Consume defect (design §5.1) is not "retry later" — it is
// "return an error into a loop that discards the message anyway", i.e. the same
// mail loss plus an unbounded error rate. FR-UI-1's copy-link is the documented
// recovery path.
//
// The backoff schedule is RetryBase * 4^(attempt-1) → 2s, 8s, 32s by default.
func (c *Consumer) send(ctx context.Context, log logrus.FieldLogger, msg mailer.Message) {
	backoff := c.cfg.RetryBase
	for attempt := 1; attempt <= c.cfg.SendAttempts; attempt++ {
		err := c.sender.Send(ctx, msg)
		if err == nil {
			mailer.RecordOutcome(mailer.OutcomeSent)
			log.WithField("attempts", attempt).Info("invite email sent")
			return
		}

		// A rejected recipient is not retryable; retrying it forever wedges a
		// partition (FR-MAIL-5).
		var perm *mailer.PermanentError
		if errors.As(err, &perm) {
			mailer.RecordOutcome(mailer.OutcomeFailedPermanent)
			log.WithError(err).WithField("attempts", attempt).
				Error("invite email permanently rejected; not retrying")
			return
		}

		if attempt == c.cfg.SendAttempts {
			mailer.RecordOutcome(mailer.OutcomeFailedTransient)
			log.WithError(err).WithField("attempts", attempt).
				Error("invite email gave up after exhausting retries")
			return
		}

		log.WithError(err).WithFields(logrus.Fields{"attempt": attempt, "retry_in": backoff.String()}).
			Warn("invite email send failed; retrying")
		if err := c.sleep(ctx, backoff); err != nil {
			mailer.RecordOutcome(mailer.OutcomeFailedTransient)
			log.WithError(err).Warn("shutting down mid-backoff; invite email abandoned")
			return
		}
		backoff *= 4
	}
}

// staleness reports whether the invite is no longer worth mailing, and why. The
// reason is a fixed label, never interpolated data.
func staleness(inv fleetclient.Invite, now time.Time) (bool, string) {
	if inv.AcceptedAt != nil && *inv.AcceptedAt != "" {
		return true, "already_accepted"
	}
	expires, err := time.Parse(time.RFC3339, inv.ExpiresAt)
	if err != nil {
		return true, "unparseable_expiry"
	}
	if !expires.After(now) {
		return true, "expired"
	}
	return false, ""
}
```

> **Why the log never leaks the token, structurally rather than by discipline:**
> `mailer.Message` has no `String()`/`LogValue()`, no code path passes a `Message`
> or an `InviteData` to a logger, every log call above takes explicit fields, and
> `PermanentError` wraps the SMTP error only — never the body.
> `TestHandle_neverLogsTheToken` is what keeps that true.

- [ ] **Step 4: Run the tests to verify they pass**

```sh
go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer/... -v
```

Expected: PASS. `logrus/hooks/test` ships with logrus, which is already a dependency.

- [ ] **Step 5: Commit**

```sh
git add apps/notification-service/internal/mailconsumer/
git commit -m "feat(notification-service): add the invite-email consumer with bounded retry"
```

---

## Task 12: Wire the mail consumer and ship the base configuration

**Files:**
- Modify: `apps/notification-service/cmd/main.go`
- Modify: `deploy/k8s/base/notification-service/configmap.yaml`
- Modify: `deploy/k8s/base/fleet-service/configmap.yaml`
- Modify: `deploy/k8s/secrets.example.yaml`

**Interfaces:**
- Consumes: `mailer.ConfigFromEnv`, `mailer.NewSMTPSender` (Task 10); `mailconsumer.NewConsumer`, `.Run` (Task 11).
- Produces: a running `invite-email` consumer goroutine; the deployed config surface.

`SMTP_ENABLED` ships **`"false"`** in the base ConfigMap. The out-of-repo prerequisite (a verified
sending domain with SPF/DKIM/DMARC and issued relay credentials, design §13) must land before it is
flipped. Until then the service is a documented no-op: invites are created, events emitted and
consumed, `skipped_disabled` increments, and nothing dials.

- [ ] **Step 1: Wire the consumer in main**

In `apps/notification-service/cmd/main.go`, add to the import block:

```go
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer"
	"github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer"
```

Add after the existing consumer loop (after line 59):

```go
	// Invite email delivery (design §3). A SEPARATE consumer group from the
	// in-app "notification" group, so a stalled relay cannot hold back in-app
	// notification offsets and vice versa.
	//
	// ConfigFromEnv fails at startup on a misconfiguration (FR-CFG-5); when
	// SMTP_ENABLED is false it reads nothing else and the consumer short-circuits
	// before any network call, so a cluster with no relay credentials is a
	// documented no-op rather than a crash loop (FR-CFG-4).
	mailCfg := mailer.ConfigFromEnv()
	var mailSender mailer.Sender
	if mailCfg.Enabled {
		mailSender = mailer.NewSMTPSender(mailCfg)
	}
	go mailconsumer.NewConsumer(log, inboxStore, fleetClient, mailSender, mailCfg).Run(ctx, brokers)
	if !mailCfg.Enabled {
		log.Warn("SMTP is disabled; invite emails will be recorded and skipped")
	}
```

> `mailSender` stays nil when disabled — the consumer never reaches `Send` in that
> branch, and constructing a sender against an unconfigured relay would be
> meaningless. `mailconsumer`'s disabled test covers exactly this path.

- [ ] **Step 2: Verify it builds**

```sh
go build github.com/jtumidanski/myfleet/... && go vet github.com/jtumidanski/myfleet/...
```

Expected: no output.

- [ ] **Step 3: Add the non-secret config keys (FR-CFG-1)**

Replace `deploy/k8s/base/notification-service/configmap.yaml` with:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-service-config
data:
  PORT: "8080"
  LOG_LEVEL: "info"
  JWKS_URL: "http://auth-service:8080/.well-known/jwks.json"
  KAFKA_BROKERS: "redpanda:9092"
  FLEET_INTERNAL_URL: "http://fleet-service:8080"

  # Invite email delivery (task-009).
  #
  # Ships DISABLED. Flip to "true" only after the out-of-repo prerequisite is
  # done: a verified sending domain with SPF, DKIM and DMARC published for
  # myfleet.tumidanski.com, and relay credentials applied out of band into
  # notification-service-secret. Until then this is a deliberate no-op —
  # invites are created and consumed, and skipped_disabled increments.
  SMTP_ENABLED: "false"
  # Provider-generic on purpose: moving from Resend to SES is a secret edit and
  # a pod restart, not a code change.
  SMTP_HOST: "smtp.resend.com"
  SMTP_PORT: "587"
  # One of starttls | tls | none. "none" is LOCAL-ONLY (Mailpit) and
  # tools/check-manifests.sh fails the build if it ever renders into main.
  SMTP_TLS_MODE: "starttls"
  SMTP_FROM_ADDRESS: "invites@myfleet.tumidanski.com"
  SMTP_FROM_NAME: "MyFleet"
  # The accept link's origin. Configuration, never derived from an inbound
  # request header (FR-TPL-2).
  PUBLIC_WEB_URL: "https://myfleet.tumidanski.com"
```

- [ ] **Step 4: Add the rate-limit knobs to fleet-service (FR-RATE-4)**

Append to `deploy/k8s/base/fleet-service/configmap.yaml`'s `data:` block:

```yaml
  # Invite abuse control (task-009, FR-RATE-1/2). Both are enforced in the
  # domain layer against persisted state, so they hold across replicas.
  INVITE_RATE_LIMIT_PER_DAY: "20"
  INVITE_RESEND_COOLDOWN_SECONDS: "300"
```

- [ ] **Step 5: Add the secret keys (FR-CFG-2)**

In `deploy/k8s/secrets.example.yaml`, extend the `notification-service-secret` stanza:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: notification-service-secret
  namespace: myfleet
type: Opaque
stringData:
  DATABASE_URL: "postgres://myfleet:REPLACE_ME@postgres.home:5432/myfleet?sslmode=disable&search_path=notification"
  # SMTP relay credentials for invite email (task-009).
  # Resend: username is literally "resend", password is the API key.
  SMTP_USERNAME: "REPLACE_ME"
  SMTP_PASSWORD: "REPLACE_ME"
```

> This file is a **template only** — referenced by no kustomization, applied out
> of band — which is why `main` still renders zero Secrets and why Argo CD's
> `prune` cannot remove the real ones. `check-manifests.sh:37-46` proves both.

- [ ] **Step 6: Verify the overlays still render clean**

```sh
kustomize build deploy/k8s/overlays/main > /dev/null && kustomize build deploy/k8s/overlays/local > /dev/null && ./tools/check-manifests.sh
```

Expected: `manifest checks passed`.

- [ ] **Step 7: Commit**

```sh
git add apps/notification-service/cmd/main.go deploy/k8s/base/notification-service/configmap.yaml deploy/k8s/base/fleet-service/configmap.yaml deploy/k8s/secrets.example.yaml
git commit -m "feat(deploy): wire the invite-email consumer and ship SMTP configuration"
```

---

## Task 13: Local development — Mailpit

`make ci` must not require a running relay, so nothing here is on the test path. This exists so a
developer can open rendered mail in a browser (FR-DEV-1…4).

> **Documented deviation from design §9.** The design routes Mailpit's UI at `/mail` through Traefik
> in *both* compose and k3s-local. Compose gets exactly that, using `MP_WEBROOT` so the SPA's asset
> paths match and no stripprefix is needed, **plus** a published `8025` so it works even if Traefik
> is down. For k3s-local, Mailpit is a ClusterIP Service reached with
> `kubectl port-forward svc/mailpit 8025:8025` — adding an ingress route for a dev-only tool buys
> nothing and is one more thing that can break the local render, which CLAUDE.md notes has already
> slipped through ten reviews once.

**Files:**
- Modify: `deploy/compose/docker-compose.yml`
- Create: `deploy/k8s/infra-local/mailpit.yaml`
- Modify: `deploy/k8s/infra-local/kustomization.yaml`
- Modify: `deploy/k8s/overlays/local/kustomization.yaml`
- Modify: `tools/check-manifests.sh`

**Interfaces:**
- Consumes: the config keys from Task 12.
- Produces: a local SMTP sink at `mailpit:1025` with a web UI on `:8025`; two new manifest invariants.

- [ ] **Step 1: Add Mailpit to docker-compose**

In `deploy/compose/docker-compose.yml`, add a service before `notification-service`:

```yaml
  mailpit:
    image: axllent/mailpit:latest
    environment:
      # Serve the UI under /mail so the Traefik route below needs no
      # stripprefix — a stripped prefix breaks the SPA's asset paths.
      MP_WEBROOT: mail
      MP_SMTP_AUTH_ACCEPT_ANY: "true"
      MP_SMTP_AUTH_ALLOW_INSECURE: "true"
    ports:
      # Published as well as routed, so the inbox is reachable even when
      # Traefik is not up — this is the first thing a developer checks.
      - "8025:8025"
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://localhost:8025/mail/ >/dev/null 2>&1 || exit 1"]
      interval: 5s
      timeout: 3s
      retries: 10
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.mailpit.rule=PathPrefix(`/mail`)"
      - "traefik.http.routers.mailpit.entrypoints=web"
      - "traefik.http.services.mailpit.loadbalancer.server.port=8025"
```

In the same file, extend `notification-service`'s `environment:` block:

```yaml
      SMTP_ENABLED: "true"
      SMTP_HOST: mailpit
      SMTP_PORT: "1025"
      SMTP_TLS_MODE: none
      SMTP_FROM_ADDRESS: invites@myfleet.local
      SMTP_FROM_NAME: MyFleet
      PUBLIC_WEB_URL: http://localhost
```

and add to its `depends_on:`:

```yaml
      mailpit:
        condition: service_healthy
```

- [ ] **Step 2: Add the Mailpit manifest**

Create `deploy/k8s/infra-local/mailpit.yaml`:

```yaml
---
# Mailpit — LOCAL-ONLY SMTP sink and web inbox (task-009, FR-DEV-1/3).
#
# Deliberately has NO PersistentVolumeClaim: captured mail is ephemeral, and a
# PVC in infra-local would be a foot-gun if this directory were ever pulled into
# another overlay. tools/check-manifests.sh fails the build if "mailpit" ever
# renders into the main overlay.
#
# Reach the inbox with:
#   kubectl port-forward -n myfleet svc/mailpit 8025:8025
# then open http://localhost:8025
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mailpit
  labels:
    app: mailpit
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mailpit
  template:
    metadata:
      labels:
        app: mailpit
    spec:
      containers:
        - name: mailpit
          image: axllent/mailpit:latest
          env:
            - name: MP_SMTP_AUTH_ACCEPT_ANY
              value: "true"
            - name: MP_SMTP_AUTH_ALLOW_INSECURE
              value: "true"
          ports:
            - containerPort: 1025
              name: smtp
            - containerPort: 8025
              name: http
          readinessProbe:
            httpGet:
              path: /
              port: 8025
            initialDelaySeconds: 3
            periodSeconds: 10
          resources:
            requests:
              memory: "32Mi"
              cpu: "10m"
            limits:
              memory: "128Mi"
              cpu: "200m"

---
apiVersion: v1
kind: Service
metadata:
  name: mailpit
  labels:
    app: mailpit
spec:
  selector:
    app: mailpit
  ports:
    - name: smtp
      port: 1025
      targetPort: 1025
    - name: http
      port: 8025
      targetPort: 8025
```

Add to `deploy/k8s/infra-local/kustomization.yaml`'s `resources:` list:

```yaml
  - mailpit.yaml
```

- [ ] **Step 3: Point the local overlay at Mailpit**

Add to `deploy/k8s/overlays/local/kustomization.yaml`'s `patches:` list:

```yaml
  # Local mail goes to the bundled Mailpit sink. Putting this override in the
  # OVERLAY rather than the base is what keeps SMTP_TLS_MODE=none structurally
  # unable to reach main (FR-DEV-2).
  - target:
      kind: ConfigMap
      name: notification-service-config
    patch: |-
      - op: replace
        path: /data/SMTP_ENABLED
        value: "true"
      - op: replace
        path: /data/SMTP_HOST
        value: "mailpit"
      - op: replace
        path: /data/SMTP_PORT
        value: "1025"
      - op: replace
        path: /data/SMTP_TLS_MODE
        value: "none"
      - op: replace
        path: /data/SMTP_FROM_ADDRESS
        value: "invites@myfleet.home"
      - op: replace
        path: /data/PUBLIC_WEB_URL
        value: "http://myfleet.home"
```

- [ ] **Step 4: Add the two manifest invariants (FR-DEV-2)**

In `tools/check-manifests.sh`, insert after the `REPLACE_ME` block (after line 46):

```sh
echo "==> main overlay must carry no local-only mail configuration"
# SMTP_TLS_MODE=none permits an unauthenticated, unencrypted session. It is
# legal ONLY for a plaintext local relay. The override lives in overlays/local,
# so its appearance in main means the base was edited by mistake.
if grep -q 'SMTP_TLS_MODE: "none"' /tmp/myfleet-main.yaml; then
  bad "main renders SMTP_TLS_MODE=none (plaintext relay is local-only)"
else
  note "no plaintext SMTP mode"
fi
# Mailpit lives in infra-local, which overlays/main must never include.
if grep -qi 'mailpit' /tmp/myfleet-main.yaml; then
  bad "main renders mailpit (local-only dev mail sink)"
else
  note "no mailpit"
fi
```

- [ ] **Step 5: Verify both overlays render and the new checks fire**

```sh
./tools/check-manifests.sh
```

Expected: `manifest checks passed`, including `no plaintext SMTP mode` and `no mailpit`.

Then prove the local overlay actually contains Mailpit and the override took:

```sh
kustomize build deploy/k8s/overlays/local | grep -c 'name: mailpit'
kustomize build deploy/k8s/overlays/local | grep 'SMTP_TLS_MODE'
```

Expected: a non-zero count, and `SMTP_TLS_MODE: none`.

- [ ] **Step 6: Run both server dry-runs**

Rendering alone does not catch namespace or cross-resource-reference errors, and the local overlay
is **not** exempt — a missing `namespace:` in `infra-local` once broke it and slipped through ten
reviews because only the `main` dry-run was ever run.

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Expected: every resource reports `(server dry run)`, no errors. If no cluster is reachable, record
that these were not run and say so in the PR — do not claim they passed.

- [ ] **Step 7: Commit**

```sh
git add deploy/compose/docker-compose.yml deploy/k8s/infra-local/ deploy/k8s/overlays/local/kustomization.yaml tools/check-manifests.sh
git commit -m "feat(deploy): add Mailpit for local invite-email development"
```

---

## Task 14: Web — copy link, resend, and 429 copy

`InviteList` is already filtered to pending invites (`InviteList.tsx:27`), which satisfies FR-UI-3
for free: an accepted invite never renders, so neither control can appear on one.

**Files:**
- Create: `apps/web/src/lib/utils/clipboard.ts`
- Test: `apps/web/src/lib/utils/clipboard.test.ts`
- Modify: `apps/web/src/services/api/InviteService.ts`
- Modify: `apps/web/src/lib/hooks/api/invites.ts`
- Modify: `apps/web/src/components/features/settings/InviteList.tsx`
- Modify: `apps/web/src/components/features/settings/InviteForm.tsx`
- Test: `apps/web/src/components/features/settings/InviteList.test.tsx`

**Interfaces:**
- Consumes: `POST /api/fleet/fleets/{fleetId}/invites/{inviteId}/resend` (Task 6); `InviteAttributes.token`, already present in the list response.
- Produces:
  - `copyToClipboard(text: string): Promise<boolean>`
  - `inviteService.resendInvite(fleetId: string, inviteId: string): Promise<Invite>`
  - `useResendInvite(fleetId: string)` — a React Query mutation taking `inviteId: string`
  - `inviteErrorMessage(err: unknown, action: 'create' | 'resend'): string`

- [ ] **Step 1: Write the failing clipboard test**

Create `apps/web/src/lib/utils/clipboard.test.ts`:

```ts
import { describe, it, expect, vi, afterEach } from 'vitest';
import { copyToClipboard } from './clipboard';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('copyToClipboard', () => {
  it('uses the async clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });

    await expect(copyToClipboard('hello')).resolves.toBe(true);
    expect(writeText).toHaveBeenCalledWith('hello');
  });

  // Local dev runs over plain HTTP on myfleet.home, where navigator.clipboard
  // is undefined. Without this fallback the button is dead in exactly the
  // environment where it gets tested first.
  it('falls back to execCommand in a non-secure context', async () => {
    vi.stubGlobal('navigator', {});
    const exec = vi.fn().mockReturnValue(true);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = exec;

    await expect(copyToClipboard('hello')).resolves.toBe(true);
    expect(exec).toHaveBeenCalledWith('copy');
    // The scratch textarea must not survive, or it accumulates in the DOM.
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('reports failure rather than throwing when both paths fail', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('denied')) } });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (document as any).execCommand = vi.fn().mockReturnValue(false);

    await expect(copyToClipboard('hello')).resolves.toBe(false);
  });
});
```

- [ ] **Step 2: Run it to verify it fails**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
npm run -w apps/web test -- clipboard
```

Expected: FAIL — cannot resolve `./clipboard`.

- [ ] **Step 3: Write the clipboard helper**

Create `apps/web/src/lib/utils/clipboard.ts`:

```ts
/**
 * Clipboard helper (task-009, FR-UI-1).
 *
 * navigator.clipboard is undefined outside a secure context, and local dev runs
 * over plain HTTP on myfleet.home — the exact environment where the copy-link
 * button gets tested first. The execCommand fallback is deprecated but is the
 * only thing that works there.
 *
 * Returns false rather than throwing: the caller decides what to tell the user.
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (navigator?.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Permission denied or a non-secure context that still exposes the API —
      // fall through to the legacy path rather than giving up.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  // Keep it out of the viewport so focusing it does not scroll the page.
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  textarea.setAttribute('readonly', '');
  document.body.appendChild(textarea);
  try {
    textarea.select();
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}
```

- [ ] **Step 4: Run it to verify it passes**

```sh
npm run -w apps/web test -- clipboard
```

Expected: PASS.

- [ ] **Step 5: Add the resend call and hook**

In `apps/web/src/services/api/InviteService.ts`, update the doc comment's route list and add the
method (there is no bodyless-POST helper on `BaseService`, so this mirrors `acceptInvite`):

```ts
  /**
   * POST /api/fleet/fleets/{fleetId}/invites/{inviteId}/resend
   * No body required. Rotates the token, so the response carries a NEW token
   * and any previously copied link is dead.
   */
  async resendInvite(fleetId: string, inviteId: string): Promise<Invite> {
    const doc = await apiClient.request<JsonApiDocument<JsonApiResource<InviteAttributes>>>(
      `/api/fleet/fleets/${fleetId}/invites/${inviteId}/resend`,
      { method: 'POST' },
    );
    return doc.data;
  }
```

In `apps/web/src/lib/hooks/api/invites.ts`, add the mutation and the error mapper:

```ts
/**
 * Maps invite API failures to copy a person can act on (FR-UI-4).
 *
 * Kept here rather than in a shared error module: this is invite-specific copy,
 * and the two 429s need different sentences, which a generic status-to-string
 * map could not express.
 */
export function inviteErrorMessage(err: unknown, action: 'create' | 'resend'): string {
  const apiError = createErrorFromUnknown(err);
  if (apiError.status === 429) {
    return action === 'create'
      ? "You've sent too many invites today. Try again tomorrow."
      : 'You just resent this invite. Wait a few minutes before trying again.';
  }
  if (apiError.status === 409 && action === 'resend') {
    return 'That invite has already been accepted.';
  }
  return apiError.message || `Could not ${action} invite`;
}

/**
 * POST /api/fleet/fleets/{fleetId}/invites/{inviteId}/resend — owner-only.
 *
 * Invalidating the list is REQUIRED, not cosmetic: resend rotates the token, so
 * a stale cache would hand the copy-link button a dead token.
 */
export function useResendInvite(fleetId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (inviteId: string) => inviteService.resendInvite(fleetId, inviteId),
    onSuccess: () => {
      toast.success('Invite resent');
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: inviteKeys.lists() });
    },
    onError: (err) => {
      toast.error(inviteErrorMessage(err, 'resend'));
    },
  });
}
```

In `apps/web/src/components/features/settings/InviteForm.tsx`, replace the `catch` body so a 429 on
create reads as English rather than a raw error:

```tsx
    } catch (err) {
      toast.error(inviteErrorMessage(err, 'create'));
    }
```

and update its import:

```tsx
import { useCreateInvite, inviteErrorMessage } from '../../../lib/hooks/api/invites';
```

`createErrorFromUnknown` is now unused in `InviteForm.tsx` — remove that import line or `lint-check`
fails.

- [ ] **Step 6: Write the failing component test**

Create `apps/web/src/components/features/settings/InviteList.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { InviteList } from './InviteList';

const pendingInvite = {
  id: 'inv-1',
  type: 'invites',
  attributes: {
    fleetId: 'f1',
    email: 'a@b.com',
    role: 'member',
    token: 'deadbeef',
    expiresAt: '2026-08-09T12:00:00Z',
    invitedByUserId: 'u1',
  },
};

const resendMutate = vi.fn();
const revokeMutate = vi.fn();
let invites = [pendingInvite];

vi.mock('../../../lib/hooks/api/invites', () => ({
  useInvites: () => ({ data: invites, isLoading: false }),
  useRevokeInvite: () => ({ mutate: revokeMutate, isPending: false }),
  useResendInvite: () => ({ mutate: resendMutate, isPending: false }),
}));

const copyToClipboard = vi.fn().mockResolvedValue(true);
vi.mock('../../../lib/utils/clipboard', () => ({
  copyToClipboard: (text: string) => copyToClipboard(text),
}));

const toastSuccess = vi.fn();
const toastError = vi.fn();
vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));

describe('InviteList', () => {
  beforeEach(() => {
    invites = [pendingInvite];
    resendMutate.mockReset();
    revokeMutate.mockReset();
    copyToClipboard.mockClear();
    toastSuccess.mockReset();
    toastError.mockReset();
  });

  // FR-UI-1: the copied URL must be the one the SPA's accept route serves.
  it('copies the accept link for a pending invite', async () => {
    render(<InviteList fleetId="f1" isOwner />);
    await userEvent.click(screen.getByRole('button', { name: /copy link/i }));

    await waitFor(() => {
      expect(copyToClipboard).toHaveBeenCalledWith(
        `${window.location.origin}/invites/deadbeef/accept`,
      );
    });
    expect(toastSuccess).toHaveBeenCalledWith('Invite link copied');
  });

  it('tells the user when the copy fails instead of silently doing nothing', async () => {
    copyToClipboard.mockResolvedValueOnce(false);
    render(<InviteList fleetId="f1" isOwner />);
    await userEvent.click(screen.getByRole('button', { name: /copy link/i }));

    await waitFor(() => expect(toastError).toHaveBeenCalled());
    expect(toastSuccess).not.toHaveBeenCalled();
  });

  // FR-UI-2.
  it('resends a pending invite by id', async () => {
    render(<InviteList fleetId="f1" isOwner />);
    await userEvent.click(screen.getByRole('button', { name: /resend/i }));
    expect(resendMutate).toHaveBeenCalledWith('inv-1');
  });

  // FR-UI-3: an accepted invite is filtered out entirely, so neither control
  // can render on one.
  it('renders no controls for an accepted invite', () => {
    invites = [{ ...pendingInvite, attributes: { ...pendingInvite.attributes, acceptedAt: '2026-08-03T00:00:00Z' } }];
    render(<InviteList fleetId="f1" isOwner />);

    expect(screen.queryByRole('button', { name: /copy link/i })).toBeNull();
    expect(screen.queryByRole('button', { name: /resend/i })).toBeNull();
    expect(screen.getByText(/no pending invites/i)).toBeInTheDocument();
  });

  // The controls are owner-gated, matching Revoke.
  it('renders no controls for a non-owner', () => {
    render(<InviteList fleetId="f1" isOwner={false} />);
    expect(screen.queryByRole('button', { name: /copy link/i })).toBeNull();
    expect(screen.queryByRole('button', { name: /resend/i })).toBeNull();
  });
});
```

- [ ] **Step 7: Run it to verify it fails**

```sh
npm run -w apps/web test -- InviteList
```

Expected: FAIL — `useResendInvite` is not exported / the buttons do not exist.

- [ ] **Step 8: Add the controls**

Replace `apps/web/src/components/features/settings/InviteList.tsx` with:

```tsx
/**
 * InviteList — pending fleet invites with copy-link, resend and revoke
 * (owner-only).
 *
 * The copy-link control is the documented recovery path when an invite email is
 * spam-filtered or a relay outage drops it (design §5.2). The token is already
 * in the fleet-scoped list response, so this needs no API change.
 */
import { toast } from 'sonner';
import { Skeleton } from '../../ui/skeleton';
import { Button } from '../../ui/button';
import { useInvites, useResendInvite, useRevokeInvite } from '../../../lib/hooks/api/invites';
import { copyToClipboard } from '../../../lib/utils/clipboard';

interface InviteListProps {
  fleetId: string;
  isOwner: boolean;
}

export function InviteList({ fleetId, isOwner }: InviteListProps) {
  const { data: invites, isLoading } = useInvites(fleetId);
  const revokeInvite = useRevokeInvite();
  const resendInvite = useResendInvite(fleetId);

  if (isLoading) {
    return (
      <div className="space-y-2">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    );
  }

  // Show only pending invites (not yet accepted). This is also what satisfies
  // FR-UI-3: an accepted invite never renders, so no control can appear on one.
  const pending = (invites ?? []).filter((inv) => !inv.attributes.acceptedAt);

  if (pending.length === 0) {
    return <p className="text-sm text-muted-foreground">No pending invites.</p>;
  }

  const handleCopy = async (token: string) => {
    const ok = await copyToClipboard(`${window.location.origin}/invites/${token}/accept`);
    if (ok) {
      toast.success('Invite link copied');
    } else {
      toast.error('Could not copy the link. Select and copy it manually.');
    }
  };

  return (
    <ul className="divide-y">
      {pending.map((inv) => (
        <li key={inv.id} className="flex flex-wrap items-center justify-between gap-2 py-3">
          <div className="space-y-0.5">
            <div className="text-sm font-medium">{inv.attributes.email}</div>
            <div className="text-xs text-muted-foreground">
              Role: <span className="capitalize">{inv.attributes.role}</span>
              {' · '}
              Expires {new Date(inv.attributes.expiresAt).toLocaleDateString()}
            </div>
          </div>
          {isOwner && (
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" onClick={() => void handleCopy(inv.attributes.token)}>
                Copy link
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={resendInvite.isPending}
                onClick={() => resendInvite.mutate(inv.id)}
              >
                Resend
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={revokeInvite.isPending}
                onClick={() => revokeInvite.mutate(inv.id)}
              >
                Revoke
              </Button>
            </div>
          )}
        </li>
      ))}
    </ul>
  );
}
```

- [ ] **Step 9: Run the web tests and the type/lint gates**

```sh
npm run -w apps/web test
npm run -w apps/web build
```

Expected: PASS, and a clean type-check. If `@testing-library/user-event` is not a dependency, use
`fireEvent.click` from `@testing-library/react` instead — check `apps/web/package.json` first.

- [ ] **Step 10: Commit**

```sh
git add apps/web/src/
git commit -m "feat(web): add copy-link and resend controls to the invite list"
```

---

## Task 15: Full-branch verification

Nothing new is written here. This is the gate CLAUDE.md requires before the branch is called done,
run as one task so a green result covers the whole change rather than a per-task slice of it.

**Files:** none.

**Interfaces:**
- Consumes: every preceding task.
- Produces: evidence.

- [ ] **Step 1: Run the full CI gate**

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests` and
`carfax-template` all pass. Paste the tail of the output into the PR — do not assert success without
it.

- [ ] **Step 2: Prove `make ci` needed no relay**

Acceptance criterion 14. `SMTP_ENABLED` must be unset in the environment `make ci` ran in:

```sh
env | grep -c '^SMTP_' || true
```

Expected: `0`.

- [ ] **Step 3: Run both server dry-runs**

```sh
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Expected: every resource reports `(server dry run)`. If no cluster is reachable, say so explicitly
in the PR rather than omitting it.

- [ ] **Step 4: Verify the local stack end to end by hand**

Not covered by automated tests (PRD §10, design §11). Bring the stack up and walk the flow:

```sh
make up
```

Then, in the app:

1. Create an invite → open `http://localhost:8025` (Mailpit) → a message arrives within seconds.
   Check: correct fleet name in the subject, correct role, legible expiry, both a text and an HTML
   part, and a working accept link.
2. Click **Resend** → a second message arrives with a **different** token; the first link 404s on
   accept.
3. Copy-link → paste in a fresh session → the invite accepts.
4. Confirm no token and no accept URL appear in the service logs:

```sh
docker compose -f deploy/compose/docker-compose.yml logs notification-service fleet-service | grep -Ec '/invites/[0-9a-f]{16}' || true
```

Expected: `0`. (Acceptance criterion 9.)

- [ ] **Step 5: Run the code review**

CLAUDE.md requires this before a PR and says not to skip it even when the plan looks complete.

```
superpowers:requesting-code-review
```

It dispatches `plan-adherence-reviewer`, `backend-guidelines-reviewer` and
`frontend-guidelines-reviewer` (all three apply — Go and TS both changed) and writes findings to
`docs/tasks/task-009-smtp-invite-delivery/audit.md`.

Point the reviewers at the two intentional deviations so they are scored against intent, not read as
drift: the separate `mailconsumer` package/group (design §3, PRD amendment 1) and bounded in-handler
retry with ledger-marking on exhaustion (design §5, PRD amendment 2).

- [ ] **Step 6: Commit any review fixes and open the PR**

```sh
git add -A && git commit -m "fix(task-009): address code review findings"
```

---

## Requirements Coverage

Every PRD functional requirement, mapped to the task that implements it. PRD §14 amendments are the
resolved contract; the design's resolutions supersede the draft wording where they differ.

| Requirement | Task |
|---|---|
| FR-EVT-1, FR-EVT-2 | 5 |
| FR-EVT-3 | 2 (payload) + 2 (emit test asserts no token) |
| FR-EVT-4 | 6 (resend emits a fresh `event_id`) |
| FR-EVT-5 | 2 (distinct type), 5 (separate emitter seam) |
| FR-INT-1, FR-INT-2, FR-INT-3 | 7 |
| FR-INT-4 | 7 (`TestInternalRouteAbsentFromJWTTree`) |
| FR-MAIL-1 | 9 (`Sender` + `FakeSender`), 10 (`smtpSender`) |
| FR-MAIL-2 | 10 |
| FR-MAIL-3 *(amended — separate package + group)* | 11 |
| FR-MAIL-4 | 11 (`staleness`) |
| FR-MAIL-5 *(amended — bounded in-handler retry)* | 10 (`classify`), 11 (`send`) |
| FR-MAIL-6 | 5 (send is asynchronous by construction) |
| FR-TPL-1 | 9 (`compose`) |
| FR-TPL-2 | 11 (`render`) + 12 (`PUBLIC_WEB_URL`) |
| FR-TPL-3 | 9 (empty-fleet-name degradation; no inviter name per design §4.5) |
| FR-TPL-4 | 9 |
| FR-TPL-5 | 9 (`html/template` + `text/template`) |
| FR-TPL-6 | 9 (templates) |
| FR-CFG-1 | 12 |
| FR-CFG-2 | 12 |
| FR-CFG-3 | 10 (`ConfigFromEnv`), 6 (`config.GetInt` in main) |
| FR-CFG-4 | 10 (disabled config), 11 (short-circuit), 12 (`SMTP_ENABLED: "false"`) |
| FR-CFG-5 | 10 |
| FR-RSND-1 | 6 |
| FR-RSND-2 | 6 |
| FR-RSND-3 | 6 |
| FR-RSND-4 | 6 (`Transform(updated)`) |
| FR-RSND-5 | 6 |
| FR-RATE-1 | 4 (`CountByFleetSince`, `CheckCreateLimit`), 6 (wired) |
| FR-RATE-2 | 4 (`CheckResendCooldown`), 6 (wired) |
| FR-RATE-3 | 4, 6 (domain layer, not UI) |
| FR-RATE-4 | 6 (env + defaults), 12 (ConfigMap) |
| FR-UI-1 | 14 |
| FR-UI-2 | 14 |
| FR-UI-3 | 14 (list is pre-filtered to pending) |
| FR-UI-4 | 14 (`inviteErrorMessage`) |
| FR-DEV-1 | 13 |
| FR-DEV-2 | 13 (`check-manifests.sh`) |
| FR-DEV-3 | 13 |
| FR-DEV-4 | 9, 10, 11 (all tests use `FakeSender`), 15 Step 2 |
| FR-OBS-1 | 10 (`RecordOutcome`), 11 (all five outcomes) |
| FR-OBS-2 | 11 (`TestHandle_neverLogsTheToken`) |
| FR-OBS-3 | 11 (recipient may be logged) |
| NFR Security — address validation, header injection | 3, 9 (`sanitizeHeader`) |
| NFR Security — 429 taxonomy *(design §4.6, new)* | 1 |
| PRD §6 — no migrations | none needed; `updated_at` already exists |
