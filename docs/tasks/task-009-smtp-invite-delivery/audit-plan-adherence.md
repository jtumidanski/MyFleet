# Plan Audit — task-009-smtp-invite-delivery

**Plan Path:** docs/tasks/task-009-smtp-invite-delivery/plan.md
**Audit Date:** 2026-08-02
**Branch:** task-009-smtp-invite-delivery
**Base Branch:** main (scope 2ac2455..7452ed6, 19 feature commits)
**Scope of this report:** plan adherence only. Two sibling reviewers cover Go and TS guidelines.

## Executive Summary

All 15 plan tasks were implemented. Every task has direct file:line evidence; nothing was stubbed,
silently skipped, or deferred. All 44 PRD functional requirements in the plan's Requirements
Coverage table are genuinely satisfied by the code the table points at, with the five documented
intentional deviations scored against intent. Builds, vet, lint-check, Go tests, web tests, web
build, manifest invariants and **both** kustomize server dry-runs pass. The single outstanding item
is Task 15 Step 4, the manual Mailpit walkthrough, which requires a human and was not performed.

## Task Completion

| # | Task | Status | Evidence / Notes |
|---|------|--------|------------------|
| 1 | 429 in the shared error taxonomy | DONE | `packages/shared-go/server/errors.go:15` (`ErrTooManyRequests`), `:38-39` (StatusFor→429), `packages/shared-go/server/server.go:32-33` (`too_many_requests`), test rows `errors_test.go:16,39` |
| 2 | `invite.created` payload + emitter | DONE | `packages/dto-go/events/payloads.go:41-53` (`InviteCreatedData`, no Token field), `apps/fleet-service/internal/events/emit.go:72-78` (`EmitInviteCreated`), tests `payloads_test.go:15-22`, `emit_test.go:73-122` (asserts payload contains no `token` at any nesting level) |
| 3 | Invite email address validation | DONE | `apps/fleet-service/internal/invite/processor.go:70-78` (`ValidateInviteEmail`, CR/LF then addr-spec equality), wired at `resource.go:79-84`; test `processor_test.go:86` covers `Bob <b@x.com>`, CR/LF, empty |
| 4 | `updated_at` on Model + windowed count | DONE | `model.go:15,26`, `entity.go:38,53`, `builder.go:27-29` (`setUpdatedAt`), `provider.go:17-20,62-68` (`CountByFleetSince`), `processor.go:82-92` (`CheckCreateLimit`), `:94-104` (`CheckResendCooldown`); tests `processor_test.go:113,134,151` |
| 5 | Transactional insert with `invite.created` | DONE | `administrator.go:71-88` — `tx.Create` + `emitCreated` in one `db.Transaction`, emit failure returns and rolls back. Tests `administrator_test.go:69` (commits together), `:108` (rollback leaves 0 invite AND 0 outbox rows). Wired `resource.go:41-43`, `cmd/main.go:99-102,192` |
| 6 | Resend, token rotation, both rate limits | DONE | `administrator.go:90-125` (`Resend`, UPDATE + fresh emit in one tx), route `resource.go:174-234`, `Limits` `resource.go:27-31`, create-limit check `resource.go:86-91`, main wiring `cmd/main.go:179-188`. Tests `administrator_test.go:127,183`. **Deviation (authorised):** cross-fleet path-pair returns 404 (`resource.go:195-198`) not the plan's literal 403 — matches `authz.RequireSameFleet`'s no-existence-leak convention; landed in 7452ed6 |
| 7 | Internal invite lookup endpoint | DONE | `apps/fleet-service/internal/invite/internal.go:33-74` (no-JWT initializer, `FleetNamer` seam, 404 on unknown, returns accepted rows, degrades to empty fleet name), response shape `rest.go:56-86`, registered off the JWT tree at `cmd/main.go:186`. Tests `internal_test.go:48,86,98,125,147` incl. `TestInternalRouteAbsentFromJWTTree` which *walks* the chi tree |
| 8 | `fleetclient.Invite` + typed status error | DONE | `fleetclient/client.go:36-70` (`ErrInviteNotFound`, `Invite` struct, `statusError`), `:117-129` (`Invite` method), `:141-143` (typed non-200). Tests `client_test.go:11,41,53,67`. **Authorised addition:** bounded `requestTimeout = 10s` and an owned `*http.Client` (`client.go:80,93-95`) |
| 9 | `mailer` — rendering and composition | DONE | `mailer/sender.go:36-68` (`Message`, `Sender`, `PermanentError`), `template.go:12-83` (`go:embed`, `html/template` + `text/template`, empty-fleet-name degradation at `:59-64`), `templates/invite.{html,txt}.tmpl`, `compose.go:24-104` (multipart/alternative, text part first, RFC 2047 subject, `sanitizeHeader`, domain-matched Message-ID), `fake.go:78-95`. Tests: `template_test.go:19,61,80,92`, `compose_test.go:31,91,115,127` |
| 10 | SMTP transport, config, metrics | DONE | `mailer/config.go:47-84` (`ConfigFromEnv`, startup panic on bad TLS mode / missing creds / `SendAttempts<1`, disabled short-circuit at `:48-50`), `smtp.go:20-164` (STARTTLS with downgrade refusal `:107-110`, implicit TLS `:63-84`, `MinVersion TLS12`, no skip-verify anywhere, `classify` `:155-164`), `metrics.go:11-28` (five outcome constants + `RecordOutcome`). prometheus promoted to a direct dep: `apps/notification-service/go.mod:10`. Tests `config_test.go:10,17,46,70,89`, `smtp_test.go:12`. 6c00ec6 additionally closes the leaked implicit-TLS socket and sets a session deadline (`smtp.go:75-78,94-97`) |
| 11 | `mailconsumer` package | DONE | `mailconsumer/consume.go:100-161` (`Handle`: dedupe → disabled short-circuit → fetch → staleness → render → send → mark), `:167-182` (`render`, accept URL from `PublicWebURL`), `:199-231` (`fetchInvite` bounded retry), `:244-280` (`send` bounded retry, permanent short-circuit), `:284-296` (`staleness`). Own group + ledger key `consumerName = "invite-email"` at `:30`. 12 tests `consume_test.go:90-350` incl. `TestHandle_neverLogsTheToken` (`:350`) which scans every log message *and* field, and bans `/invites/` outright |
| 12 | Wire the consumer, ship base config | DONE | `apps/notification-service/cmd/main.go:63-79` (`ConfigFromEnv`, nil sender when disabled, goroutine `Run`), `deploy/k8s/base/notification-service/configmap.yaml:12-31` (all six FR-CFG-1 keys + `SMTP_ENABLED: "false"`), `deploy/k8s/base/fleet-service/configmap.yaml:15-18` (rate-limit knobs), `deploy/k8s/secrets.example.yaml:58-61` (`SMTP_USERNAME`/`SMTP_PASSWORD` = `REPLACE_ME`) |
| 13 | Local development — Mailpit | DONE | `deploy/compose/docker-compose.yml:3-25` (Mailpit + `/mail` Traefik route + published 8025) and `:198-204` (notification-service SMTP env), `deploy/k8s/infra-local/mailpit.yaml` (Deployment + Service, no PVC), `infra-local/kustomization.yaml:20`, `overlays/local/kustomization.yaml:79-103` (ConfigMap patch), `tools/check-manifests.sh:48-65` (two new invariants). **Authorised deviation:** the grep is quote-tolerant (`SMTP_TLS_MODE:[[:space:]]*"?none"?`, `check-manifests.sh:55`). Verified: the plan's literal `'SMTP_TLS_MODE: "none"'` does **not** match the real local render (kustomize emits `SMTP_TLS_MODE: none` unquoted, local render line 135) while the corrected pattern does |
| 14 | Web — copy link, resend, 429 copy | DONE | `apps/web/src/lib/utils/clipboard.ts:11-37` (async API + execCommand fallback, returns false), `services/api/InviteService.ts:59-65` (`resendInvite`), `lib/hooks/api/invites.ts:71-88` (`inviteErrorMessage`, distinct 429 create/resend copy + 409 resend copy) and `:129-148` (`useResendInvite`, invalidates on **settle**), `components/features/settings/InviteList.tsx:42-49,63-88`. Tests `clipboard.test.ts` (3), `InviteList.test.tsx` (5), `members.test.ts:244,265` (real-hook invalidation, success and reject) |
| 15 | Full-branch verification | PARTIAL | Steps 1-3 and 5 satisfied — see Build & Test Results below; both server dry-runs run clean against the `aeon.tumidanski` cluster. **Step 4 (manual local-stack Mailpit walkthrough) was not performed** — it requires a human at a browser. Outstanding, not a defect |

**Completion Rate:** 15/15 tasks implemented (100%); 14 fully verified, 1 (Task 15) partial on a
human-only step.
**Skipped without approval:** 0
**Partial implementations:** 1 (Task 15 Step 4 only)

## Requirements Coverage Walk

Every row of the plan's Requirements Coverage table was checked against the code it points at.

| Requirement | Verdict | Evidence |
|---|---|---|
| FR-EVT-1, FR-EVT-2 | SATISFIED | `administrator.go:73-83` — insert and outbox enqueue in one `db.Transaction` |
| FR-EVT-3 | SATISFIED | `payloads.go:47-51` has no token field; `emit_test.go:104-106` fails if the serialized payload contains `token` at any level |
| FR-EVT-4 | SATISFIED | `events/emit.go:40` mints `uuid.NewString()` per enqueue, so each resend emits a distinct `event_id`; `administrator.go:116-118` emits inside the rotation tx |
| FR-EVT-5 | SATISFIED | `member.invited` still emitted only on accept (`administrator.go:179-183`); separate `CreatedEmitter` seam (`administrator.go:37-41`); `consumer.Topics` unchanged (`consumer/consume.go:27-33`) |
| FR-INT-1/2/3 | SATISFIED | `internal.go:37-72`; response fields `rest.go:56-66`; accepted rows returned (`internal.go:44-47`, test `internal_test.go:98`) |
| FR-INT-4 | SATISFIED | `internal_test.go:147` walks the JWT router and fails on any route containing `internal`; belt-and-braces ingress deny at `deploy/k8s/overlays/main/ingressroute.yaml:89` (verified present by `check-manifests.sh` "internal-deny present at priority 200") |
| FR-MAIL-1 | SATISFIED | `sender.go:55-57` + `fake.go:78`; no test dials a socket (grep of the mailer/mailconsumer suites shows only `httptest` in fleetclient) |
| FR-MAIL-2 | SATISFIED | `smtp.go:64,111` set `MinVersion: tls.VersionTLS12` with `ServerName`; no `InsecureSkipVerify` anywhere in the tree; STARTTLS downgrade is an explicit error (`smtp.go:107-110`) |
| FR-MAIL-3 *(amended)* | SATISFIED against intent | Separate package and separate group `invite-email` (`consume.go:30,83`), deliberately absent from `consumer.Topics`. This delivers the requirement's stated goal (SMTP failure cannot re-run the in-app path or vice versa) more completely than the literal wording, and independently decouples offsets |
| FR-MAIL-4 | SATISFIED | `consume.go:145-149` + `staleness` `:284-296`; test `consume_test.go:154` |
| FR-MAIL-5 *(amended)* | SATISFIED against intent | `classify` `smtp.go:155-164` (5xx→permanent); `send` `consume.go:244-280` retries transients on `RetryBase*4^(n-1)` and short-circuits permanents. Ledger IS marked on transient exhaustion (`consume.go:160`) — the documented amendment, since `events.Consume` does not actually redeliver; copy-link is the recovery path. Note the deliberate asymmetry: *lookup* exhaustion leaves the ledger unmarked (`consume.go:138-142`), which is strictly more permissive and is explained in place |
| FR-MAIL-6 | SATISFIED | `resource.go:108-113` returns 201 as soon as the tx commits; no sender is reachable from fleet-service |
| FR-TPL-1 | SATISFIED | `compose.go:42,56-71` — `multipart/alternative`, plain part written first per RFC 2046; test `compose_test.go:31` re-parses the output |
| FR-TPL-2 | SATISFIED | `consume.go:172-173` builds `{PublicWebURL}/invites/{token}/accept` from config only; SPA route confirmed at `apps/web/src/App.tsx:22` (`/invites/:token/accept`); `PUBLIC_WEB_URL` shipped at `configmap.yaml:31` |
| FR-TPL-3 | SATISFIED against intent | Fleet, role and expiry are all in both parts (`templates/invite.txt.tmpl:3-7`); empty fleet name degrades (`template.go:59-64`, test `template_test.go:92`). Inviter name intentionally absent per design §4.5 — auth-service exposes no internal surface for it |
| FR-TPL-4 | SATISFIED | Subject is fixed copy + fleet name (`template.go:60,63`); no token path reaches the subject |
| FR-TPL-5 | SATISFIED | `template.go:21-22` uses `html/template` and `text/template` separately; escaping test `template_test.go:61` |
| FR-TPL-6 | SATISFIED | "you can safely ignore this email" in both templates; no unsubscribe link |
| FR-CFG-1 | SATISFIED | All six named keys present, `configmap.yaml:19-31` |
| FR-CFG-2 | SATISFIED | `secrets.example.yaml:58-61`; `check-manifests.sh` still reports "no Secret" and "no placeholders" for main |
| FR-CFG-3 | SATISFIED | `config.go:52-65` uses `config.Get/MustGet/GetInt`; `cmd/main.go:181-187` uses `config.GetInt`; no `os.Getenv` in either service (grep returns only the comment at `config.go:24`) |
| FR-CFG-4 | SATISFIED | `config.go:48-50` returns a disabled config reading nothing else; `consume.go:116-120` short-circuits before any network call and marks the ledger; `configmap.yaml:25` ships `"false"` |
| FR-CFG-5 | SATISFIED | `config.MustGet` on HOST/FROM_ADDRESS/PUBLIC_WEB_URL (`config.go:54,57,61`); `config_test.go:46` asserts the panic |
| FR-RSND-1 | SATISFIED | `resource.go:174` route, owner gates `:200-208` |
| FR-RSND-2 | SATISFIED | `administrator.go:95-99` rotates token + `expires_at` + `updated_at` in one UPDATE inside the tx |
| FR-RSND-3 | SATISFIED | `resource.go:212-215` returns 409, checked *before* the cooldown |
| FR-RSND-4 | SATISFIED | `resource.go:233` returns `Transform(updated)`, same shape as create |
| FR-RSND-5 | SATISFIED | `resource.go:217-220` |
| FR-RATE-1 | SATISFIED | `provider.go:62-68` DB-backed count; `processor.go:82-92`; wired `resource.go:88` |
| FR-RATE-2 | SATISFIED | `processor.go:94-104` reads persisted `updated_at`; wired `resource.go:217` |
| FR-RATE-3 | SATISFIED | Both checks live in the processor and are called from the handler, not the UI |
| FR-RATE-4 | SATISFIED | `cmd/main.go:184-186` env with the specified defaults (20 / 300); ConfigMap `deploy/k8s/base/fleet-service/configmap.yaml:17-18` |
| FR-UI-1 | SATISFIED | `InviteList.tsx:43` copies `${window.location.origin}/invites/${token}/accept`; toast confirmation `:45`; token already in `Attributes` (`rest.go:14`) |
| FR-UI-2 | SATISFIED | `InviteList.tsx:72-79` + `invites.ts:129-148`; invalidation covered by two real-hook tests |
| FR-UI-3 | SATISFIED | `InviteList.tsx:36` filters on `!acceptedAt`, so accepted invites render no row at all; test `InviteList.test.tsx:85` |
| FR-UI-4 | SATISFIED | `invites.ts:78-86` maps 429 (and 409-on-resend) to human copy; wired into create (`InviteForm.tsx:33`) and resend (`invites.ts:144`) |
| FR-DEV-1 | SATISFIED | compose + `infra-local/mailpit.yaml`; local render contains 10 `mailpit` references |
| FR-DEV-2 | SATISFIED | `check-manifests.sh:48-65`; **proven non-vacuous** — the pattern fires against the rendered local overlay and not against main |
| FR-DEV-3 | SATISFIED against intent | Compose gets the `/mail` Traefik route plus a published 8025; k3s-local is `kubectl port-forward svc/mailpit 8025:8025`, documented at `mailpit.yaml:9-11`. Documented deviation from design §9 |
| FR-DEV-4 | SATISFIED | `env \| grep -c '^SMTP_'` = 0 in the audit environment; full Go + web suites pass with no relay running |
| FR-OBS-1 | SATISFIED | `metrics.go:12-16` defines exactly the five named outcomes; all five are reachable from `consume.go` (`:118,125,135,147,155,213,222,249,258,265,274`) |
| FR-OBS-2 | SATISFIED | `consume.go:101-105,128` attaches `event_id`/`fleet_id`/`trace_id`/`invite_id`; `TestHandle_neverLogsTheToken` (`consume_test.go:350`) mechanically enforces the no-token/no-URL rule across a full failing Handle at debug level |
| FR-OBS-3 | SATISFIED (permissive) | No prohibition breached |
| NFR Security — header injection | SATISFIED | `ValidateInviteEmail` at creation (`processor.go:70`) plus `sanitizeHeader` + QEncoding at compose (`compose.go:34,38,82-84`); test `compose_test.go:91` |
| NFR Security — 429 taxonomy | SATISFIED | Task 1 |
| PRD §6 — no migrations | SATISFIED | Diff contains no migration files and no new columns; `updated_at` was already on `Entity` |

## Skipped / Deferred Tasks

1. **Task 15 Step 4 — manual local-stack walkthrough via the Mailpit UI.** Not performed; requires
   a human to bring up `make up`, create and resend an invite, read the rendered mail, and confirm
   the old link 404s. Impact: the end-to-end wiring (compose SMTP env → Mailpit → rendered message →
   accept link) is covered only by unit-level evidence. Every individual link in that chain is
   tested, but their composition in a live stack is not. Recommend running it before merge.

## Bookkeeping Observation

`plan.md` still shows **0 of 114 step checkboxes ticked** (`- [ ]` throughout). The work is done;
the plan file was simply never updated. This is a documentation gap, not an implementation gap, but
it means the plan file on its own is misleading about the branch's state.

## Build & Test Results

| Target | Build | Tests | Vet | Notes |
|---|---|---|---|---|
| fleet-service | PASS | PASS | PASS | 16 packages ok, `internal/invite` 0.017s |
| notification-service | PASS | PASS | PASS | 5 packages ok incl. `mailconsumer`, `mailer`, `fleetclient` |
| packages/shared-go | PASS | PASS | PASS | 9 packages ok |
| packages/dto-go | PASS | PASS | PASS | `events` ok |
| apps/web | PASS | PASS | n/a | 31 files, 188 tests passed; `vite build` clean (pre-existing 500 kB chunk warning only) |
| lint-check | PASS | — | — | golangci `0 issues` across all modules; prettier clean; eslint `--max-warnings 0` clean |
| manifests | PASS | — | — | `manifest checks passed`, incl. `no plaintext SMTP mode` and `no mailpit` |
| kubectl dry-run (main) | PASS | — | — | every resource `(server dry run)`, exit 0 |
| kubectl dry-run (local) | PASS | — | — | every resource `(server dry run)`, exit 0 |

No `TODO`/`FIXME`/`XXX`/unimplemented markers were introduced anywhere in the branch diff.

## Overall Assessment

- **Plan Adherence:** FULL
- **Recommendation:** READY_TO_MERGE (pending the manual walkthrough, and subject to the two sibling
  guideline reviews)

## Action Items

1. Run Task 15 Step 4 by hand: `make up`, create an invite, read it in Mailpit at
   `http://localhost:8025`, resend and confirm a different token, confirm the old link 404s on
   accept, and confirm the log grep for `/invites/[0-9a-f]{16}` returns 0.
2. Tick the completed checkboxes in `plan.md` (or note in the PR that the plan file was not
   maintained) so the committed plan reflects the branch's actual state.
3. Merge the findings of `backend-guidelines-reviewer` and `frontend-guidelines-reviewer` before
   opening the PR, per CLAUDE.md's code-review-before-PR rule.
