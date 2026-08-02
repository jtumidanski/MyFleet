# Backend Audit — task-009-smtp-invite-delivery

- **Scope:** Go portion of `2ac2455..7452ed6`
- **Guidelines Source:** `backend-dev-guidelines` skill (`.claude/skills/backend-dev-guidelines/resources/`)
- **Date:** 2026-08-02
- **Build:** PASS
- **Tests:** all targeted packages PASS, 0 failed
- **Overall:** NEEDS-WORK

## Build & Test Results

`go build ./...` from the worktree root reports `pattern ./...: directory prefix . does not contain
modules listed in go.work` (multi-module repo — not a build failure). Per-module verification:

```
go vet ./...   apps/fleet-service          clean
go vet ./...   apps/notification-service   clean
go vet ./...   packages/shared-go          clean

ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/invite            0.021s
ok  github.com/jtumidanski/myfleet/apps/fleet-service/internal/events            0.013s
ok  github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer     0.008s
ok  github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer 0.020s
ok  github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient  0.028s
ok  github.com/jtumidanski/myfleet/packages/shared-go/server                     0.003s
```

## Framework note

The guidelines are written against gorilla/mux + api2go + `server.RegisterHandler(l)(si)(...)` +
`database.Query`. This repo uses chi + `packages/shared-go/server` (`handler.go:42`, `jsonapi.go:9`).
Checks whose *mechanism* does not exist here are marked N/A with the substitute cited; checks whose
*intent* survives the framework change are enforced.

## Domain Checklist Results

### `apps/fleet-service/internal/invite` (domain — has `model.go`)

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| DOM-01 | `builder.go` exists with validating `Build()` | **FAIL** | `builder.go:15` `NewBuilder()`, `:17-22` fluent setters — but `:33` `func (b *Builder) Build() Model { return b.m }` enforces no invariants. `ai-guidance.md:170` / `file-responsibilities.md:24` / `scaffolding-checklist.md:139` all require `Build()` validation. |
| DOM-02 | `ToEntity()` on Model | PASS | `entity.go:43` |
| DOM-03 | `Make(Entity)` | PASS | `entity.go:28` (returns `Model`, not `(Model, error)` — house convention; deviation from `file-responsibilities.md:17`) |
| DOM-04 | `Transform` | PASS | `rest.go:21`; internal variant `rest.go:70` |
| DOM-05 | `TransformSlice`, used by list handler | PASS | `rest.go:42`; used at `resource.go:130`, no inline loop |
| DOM-06 | Processor takes `logrus.FieldLogger` | PASS | `processor.go:20` |
| DOM-07 | Handlers pass a request-scoped logger | **FAIL** | No `HandlerDependency` exists here. `resource.go` contains **zero** log calls in 273 lines / 5 handlers (`grep -n "log\.\|Logger()"` → no match). `internal.go:53,63` logs with the startup logger captured at `internal.go:33`, so no correlation ID reaches the line even though `telemetry.CorrelationID` is installed (`cmd/main.go:191`) and read at `resource.go:108`. Sibling domain `fuel/resource.go:53,129,213` does log — invite is the outlier, not the convention. |
| DOM-08 | POST/PATCH use typed input handler | PASS | `resource.go:48` `server.RegisterInputHandler`. The body-less action POSTs (`resource.go:174` resend, `:237` accept) correctly stay bare per `patterns-rest-jsonapi.md:69-72`. |
| DOM-09 | Transform errors handled | PASS (N/A) | `Transform` returns no error in this codebase (`rest.go:21`) |
| DOM-10 | Providers lazy / context-carrying | **FAIL** | No `database.Query`/`SliceQuery` helper exists in this repo, so the literal check is N/A; the surviving requirement — `testing-guide.md:213` "Verify providers use `db.WithContext(ctx)` not bare `db`" — fails. `provider.go:26` captures the raw `*gorm.DB` at startup (`resource.go:42`, `internal.go:34`); `provider.go:30,41,52,64` all use bare `p.db`. New `CountByFleetSince` (`provider.go:62-68`) continues it. |
| DOM-11 | No `os.Getenv()` in handlers | PASS | zero matches in `internal/invite/`; limits injected via `Limits` (`resource.go:27`, `cmd/main.go:185-189`) |
| DOM-12 | No cross-domain logic in handlers | **FAIL** | See DOM-13/14 — token minting (`resource.go:93,222`), expiry computation (`:104,227`), model construction (`:99-106`) and the accepted-invite invariant (`:212`) all execute in the handler. The analogous invariant `ValidateAccept` lives in the processor (`processor.go:49`), so the layering is internally inconsistent. |
| DOM-13 | Handlers don't call providers directly | PASS | handlers call `proc.*` only (`resource.go:88,125,138,179,217,241`) |
| DOM-14 | Writes go processor → administrator | **FAIL** | `resource.go:43` builds the administrator in the route initializer and handlers call it directly: `adm.Insert` `:108`, `adm.Delete` `:163`, `adm.Resend` `:227` (new), `adm.Accept` `:256`. `file-responsibilities.md:106-108` and `anti-patterns.md:14` require `resource.go → processor.go → administrator.go`. Pre-existing house pattern (`fuel/resource.go:211,237`; `membership/resource.go:77`) — the new resend handler adds one more instance. No `db.Create`/`db.Save` appears in `resource.go`. |
| DOM-15 | `administrator.go` exists | PASS | `administrator.go:43-51`, `Resend` at `:90` |
| DOM-16 | Domain error → HTTP status | PASS | `server.StatusFor` (`packages/shared-go/server/errors.go:18`): 422 validation, 404, 409, and new 429 at `errors.go:38`. Handlers return typed sentinels (`processor.go:76,89,100`). |
| DOM-17 | JSON:API interface on REST models | N/A | House convention is `server.Resource{Type,ID,Attributes}` (`packages/shared-go/server/jsonapi.go:9-14`), not api2go `GetName/GetID/SetID`. `rest.go:34-38` conforms. |
| DOM-18 | Flat request models | WARN | Structure is flat (no nested Data/Type/Attributes), but the create request is an **inline anonymous struct** at `resource.go:48-51` instead of a named type in `rest.go` per `file-responsibilities.md:116`. |
| DOM-19 | Table-driven tests | PASS | `processor_test.go:86,113,134`; `mailer/smtp_test.go:13`; `mailconsumer/consume_test.go:154`. Remainder are single-case by design. |
| DOM-19b | REST status mapping tested | **FAIL** | `testing-guide.md:37` "REST — Verify status mapping and JSON:API output". No `resource_test.go` exists in `internal/invite/`. The security-critical cross-fleet 404 added by `7452ed6` (`resource.go:195-198`), the 429 wiring (`:88`) and the 422 email-validation wiring (`:81`) have no HTTP-level test. `internal_test.go` covers only the internal route. |

## Infrastructure Package Results

`mailer`, `mailconsumer` and `fleetclient` are infrastructure, not DDD domains (declared at
`mailer/sender.go:1-8`, `mailconsumer/consume.go:1-10`, plan.md Task 9). DOM-* not applied.

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SUB-01 | Logic not in the handler | PASS | `mailconsumer/consume.go:100` `Handle` orchestrates; collaborators injected at `:58` behind `Inbox`/`Invites`/`mailer.Sender` seams (`:37,43`, `sender.go`) |
| SUB-02 | No direct DB writes | PASS | no `db.Create`/`db.Save` in either package; ledger writes go through the `Inbox` seam (`consume.go:39`) |
| SUB-03 | Typed input handler for POST | N/A | packages expose no HTTP handlers |
| SUB-04 | No manual JSON parsing in handlers | PASS | `json.NewDecoder` at `fleetclient/client.go:144` is an HTTP **client** decode, not a request handler |

## Security Review

| ID | Check | Status | Evidence |
|----|-------|--------|----------|
| SEC-01 | Internal endpoint off the JWT tree | PASS | `internal.go:33` registered outside the JWT group (`cmd/main.go:182` vs `:199`); `internal_test.go:147` walks the JWT router tree and fails on any route containing `internal` |
| SEC-01b | Internal endpoint off the public internet | PASS | `deploy/k8s/overlays/main/ingressroute.yaml:88-96` priority-200 `PathRegexp((?i)^/+api/+fleet[^/]*/*internal)` + `internal-deny` ipAllowList `255.255.255.255/32` (`:16-23`), mirrored to :443 by kustomize `replacements` |
| SEC-02 | Token absent from the event payload | PASS | `packages/dto-go/events/payloads.go:47-51` — `InviteCreatedData` carries `invite_id`/`email`/`role` only; asserted by `events/emit_test.go:77` |
| SEC-02b | Token absent from logs/errors/metrics | PASS | `grep` over `invite/*.go`, `mailer/*.go`, `mailconsumer/consume.go` finds no token in any log or error path; `mailconsumer/consume_test.go:350` `TestHandle_neverLogsTheToken` asserts it mechanically across a full failing `Handle`, message **and** fields, and bans `/invites/` outright. Metric labels are a closed constant set (`mailer/metrics.go:11-17`). |
| SEC-03 | No open redirect | PASS | accept URL built from `cfg.PublicWebURL` only (`mailconsumer/consume.go:172-173`), sourced from `config.MustGet("PUBLIC_WEB_URL")` (`mailer/config.go:61`) — never from an inbound header; `url.PathEscape` on the token |
| SEC-04 | No hardcoded secrets | PASS | `mailer/config.go:52-65` reads everything through `packages/shared-go/config`; no literal credentials anywhere in the diff |
| SEC-05 | Header injection via email / fleet name | PASS | `processor.go:70-79` `ValidateInviteEmail` rejects CR/LF first, then requires `a.Address == s` so a display-name form cannot smuggle header content; `compose.go:82` `sanitizeHeader` strips CR/LF at compose time; `compose.go:38` RFC-2047-encodes the user-controlled Subject; asserted by `mailer/compose_test.go:91` |
| SEC-06 | TLS verification on, no downgrade | PASS | no `InsecureSkipVerify` anywhere in `apps/` or `packages/`; `MinVersion: tls.VersionTLS12` at `smtp.go:64` and `:111`; `smtp.go:107-110` **errors** rather than continuing in plaintext when STARTTLS is not advertised; `TLSModeNone` blocked from the main overlay by `tools/check-manifests.sh:55-56` |
| SEC-07 | Credentials not sent before TLS upgrade | PASS | `smtp.go:50` `authenticate` runs after `dial` (`:44`), and `dial` completes STARTTLS at `:111` before returning; `net/smtp`'s `PlainAuth` additionally refuses an unencrypted non-localhost connection |
| SEC-08 | Resend authorization / no cross-fleet leak | PASS (untested) | `resource.go:185` same-fleet → `:195-198` path-pair mismatch → 404 → `:200` owner → `:205` authoritative DB owner recheck. A guessed cross-fleet invite ID and a nonexistent one both yield 404. **No test covers this ordering** — see DOM-19b. |
| SEC-09 | Error text disclosure | **FAIL** | `packages/shared-go/server/jsonapi.go:35` puts `err.Error()` into the response `Title` for **any** error, and `errors.go:41` maps unrecognised errors to 500. `resource.go:89-91` (rate-limit `COUNT`), `:110-112` (`Insert`) and `:229-232` (`Resend`) pass raw GORM/driver errors straight in, so a DB failure returns driver text — table names, SQLSTATE — to the caller. Shared-package behaviour, but newly exercised by this task's paths, and with DOM-07 the same failure is invisible in the logs. |
| SEC-10 | Token in URL path | WARN | `resource.go:237` `POST /invites/{token}/accept` and the emailed link `mailconsumer/consume.go:172` place a bearer credential in a URL path — browser history and same-origin `Referer` on the accept POST at minimum. The route pre-dates this task, but this task is what mails the URL. No access logging is configured in-repo, so a log-based leak is not demonstrable from this tree. |

## Correctness Findings (outside the checklists)

| Ref | Finding | Evidence |
|-----|---------|----------|
| C-1 | `Resend` never checks `RowsAffected` | `administrator.go:95-101` — the UPDATE is only checked for `res.Error`. If the invite is deleted between `proc.GetByID` (`resource.go:179`) and `adm.Resend` (`:227`), the transaction updates 0 rows, `Make(...)` at `:103-113` fabricates the "updated" model from the stale in-memory copy, the handler returns **200 with a rotated token** (`:233`) and an `invite.created` event is emitted for a row that does not exist. The consumer then 404s and records `skipped_stale` (`mailconsumer/consume.go:133-136`). The same TOCTOU lets a concurrent `Accept` and `Resend` interleave into an accepted invite carrying a fresh live token; the UPDATE is not conditioned on `accepted_at IS NULL`. |
| C-2 | Non-string, non-error log fields escape the token-leak test | `mailconsumer/consume_test.go:382-390` — `fmtValue` returns `""` for anything that is not a `string` or `error`, so a token embedded in a struct or `fmt.Stringer` log field would pass `TestHandle_neverLogsTheToken` (`:350`) silently. The assertion is sound for today's code; it is not sound as a regression guard. |
| C-3 | `TLSModeNone` does not reject configured credentials | `mailer/config.go:76` requires credentials when the mode is **not** `none`, but never rejects credentials when it **is** `none`. `smtp.go:121-125` would then attempt PLAIN over plaintext; `net/smtp` refuses (so no credential leak), but the refusal is classified transient at `smtp.go:163`, burning all four send attempts and permanently dropping the mail per the documented exhaustion policy. |

## Summary

### Blocking (must fix)

- **DOM-07** — `apps/fleet-service/internal/invite/resource.go`: zero error logging across 5 handlers and ~15 error branches; `internal.go:53,63` logs without the correlation ID. Sibling `fuel/resource.go:53,129,213` shows this is not the house convention.
- **SEC-09** — `resource.go:89,110,229` feed raw GORM errors into `server.WriteError`, which echoes `err.Error()` to the client (`jsonapi.go:35`). Combined with DOM-07 the failure is visible to the caller and nowhere else.
- **C-1** — `administrator.go:95-101`: `Resend` ignores `RowsAffected` and is not conditioned on `accepted_at IS NULL`; a deleted or concurrently-accepted invite still returns 200 + rotated token + a `invite.created` event.
- **DOM-19b** — no `resource_test.go` in `internal/invite/`: the cross-fleet 404 added by `7452ed6` (`resource.go:195-198`), the 429 and the 422 wiring are untested at the HTTP layer.
- **DOM-01** — `builder.go:33`: `Build()` enforces no invariants.
- **DOM-10** — `provider.go:26,30,41,52,64`: providers hold a startup `*gorm.DB` with no `WithContext`; new `CountByFleetSince` (`:62`) continues it.
- **DOM-12 / DOM-14** — `resource.go:43,93,99-106,108,163,212,222,227,256`: handlers construct models, mint tokens, compute expiry, enforce an invariant, and call the administrator directly. Pre-existing house pattern; the new resend handler extends it.

### Non-Blocking (should fix)

- **SEC-10** — bearer token in the URL path (`resource.go:237`, `mailconsumer/consume.go:172`).
- **C-2** — `consume_test.go:382` `fmtValue` blinds the token-leak assertion to non-string fields.
- **C-3** — `mailer/config.go:76` permits credentials with `SMTP_TLS_MODE=none`.
- **DOM-18** — `resource.go:48-51` inline anonymous request struct instead of a named type in `rest.go`.
- **DOM-03** — `entity.go:28` `Make` returns `Model`, not `(Model, error)` (`file-responsibilities.md:17`).

### Explicitly Cleared

Documented design decisions evaluated as implemented and **not** reported as defects: mark-after-success
(`consume.go:98-99,160`), ledger marking on exhausted send retries (`consume.go:236-241`) versus the
deliberately-unmarked fetch path (`consume.go:138-142`), `updated_at` as the cooldown clock
(`processor.go:94-103`), and hand-composed RFC 5322 (`compose.go:17-23`).
