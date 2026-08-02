# task-009 — Code Review Record

Consolidated index for the review of `task-009-smtp-invite-delivery`.
Three reviewers audited the branch in parallel after all fifteen plan tasks
landed, following the whole-branch review that ran during execution.

| Report | Reviewer | Verdict as first issued |
| --- | --- | --- |
| [audit-plan-adherence.md](./audit-plan-adherence.md) | `plan-adherence-reviewer` | PASS — 15/15 tasks, all 44 FR-* rows covered, no silent gaps |
| [audit-backend.md](./audit-backend.md) | `backend-guidelines-reviewer` | NEEDS-WORK — 7 blocking, 5 non-blocking |
| [audit-frontend.md](./audit-frontend.md) | `frontend-guidelines-reviewer` | NEEDS-WORK — 1 non-blocking |

An adversarial whole-branch review also ran mid-execution (0 Critical,
3 Important, 4 Minor). Its three Important findings were fixed in `7452ed6`
and confirmed by a scoped re-review:

- `fleetclient` used `http.DefaultClient` (zero timeout) with a
  never-cancelled context — a stalled fleet-service would have wedged the
  `invite-email` consumer permanently, silently, with no metric.
- A transient invite-lookup failure dropped the email with no metric, on the
  premise that `events.Consume` redelivers — which `design.md` §5.1 proves it
  does not.
- The resend handler returned 403 for a cross-fleet invite where a nonexistent
  id returned 404, creating an existence oracle.

An earlier per-task round fixed an unbounded SMTP session (only the initial
connect was bounded, so a relay that completed the handshake then stalled hung
the goroutine) and a socket leak in the implicit-TLS dial path.

## Security posture — passed with evidence

Every SEC-* focus item passed, each citable:

- **Token confinement.** The invite token reaches no log message, log field,
  error string, metric label, or outbox/Kafka payload. `InviteCreatedData`
  carries only `invite_id`/`email`/`role`; `TestEmitInviteCreated` asserts the
  serialized payload contains no `token` substring at any nesting level, and
  `TestHandle_neverLogsTheToken` mechanically scans every log message and field
  at debug level across a full failing `Handle`.
- **The token-serving internal endpoint** is off the JWT tree
  (`TestInternalRouteAbsentFromJWTTree` *walks* the chi tree rather than
  probing one URL) and off the public internet (the existing priority-200 deny
  regex already matches the new path on both entrypoints).
- **Header injection** is blocked at both ends — `ValidateInviteEmail` requires
  the input to equal the parsed addr-spec (rejecting `Bob <b@x.com>` display
  names and CR/LF), and `compose` applies `sanitizeHeader` plus RFC 2047
  encoding to every header carrying user input.
- **TLS** verification is on in both modes with no skip-verify anywhere in code
  or config, a relay that does not advertise STARTTLS is a hard error rather
  than a silent downgrade, and credentials are never sent before the upgrade.
- **`SMTP_TLS_MODE: none`** is structurally unable to reach production: the
  override lives in the local overlay, and `tools/check-manifests.sh` fails the
  build if it renders into `main`.

## Findings requiring a ruling

The backend audit's blocking list mixes defects this branch introduced with
long-standing house patterns the branch merely continued. They are not the same
decision:

**Attributable to this branch** — fixed, see below:

- **C-1** — `Resend` ignores `RowsAffected` and does not condition the UPDATE on
  `accepted_at IS NULL`. A concurrent `Accept` yields an accepted invite
  carrying a fresh live token; a concurrent delete returns 200 with a rotated
  token for a row that no longer exists.
- **DOM-19b** — no `resource_test.go`, so the cross-fleet 404 added by `7452ed6`,
  the 429 rate-limit wiring, and the 422 email-validation wiring have no
  HTTP-layer test.
- **DOM-07** — the resend handler was added to a file with no error logging at
  all, while sibling domains log.
- **Frontend text casing** — "Copy link" should be "Copy Link"; the sibling
  labels added in the same diff are already title case.

**Pre-existing and repo-wide** — deliberately *not* churned on this branch:

- **SEC-09** — `server.WriteError` puts `err.Error()` into the JSON:API response
  `Title` (`packages/shared-go/server/jsonapi.go`), so a raw GORM fault reaches
  the client. This affects every service in the repo, not just invites. Fixing
  it is a shared-package change with blast radius well beyond task-009.
- **DOM-01** — `Build()` validates nothing.
- **DOM-10** — providers use a bare startup `*gorm.DB` rather than
  `db.WithContext(ctx)`.
- **DOM-12/DOM-14** — handlers mint tokens, build models and call the
  administrator directly. The audit itself notes this is the existing house
  pattern (`fuel/resource.go`).

## Accepted tradeoffs, not defects

Recorded so a future reader does not re-litigate them:

- **Mark-after-success** in the mail consumer: a crash between `Send` and `Mark`
  yields one duplicate email. Marking first would silently *drop* mail, which
  was judged strictly worse.
- **Exhausted SEND retries mark the ledger**, permanently dropping that email;
  the copy-link UI is the documented recovery path. The invite-FETCH path
  deliberately differs — it stays unmarked, which is strictly more permissive.
- **The resend cooldown reads `updated_at`**, meaning "last write of any kind".
  Correct today because the only other writer is `Accept`, which trips a 409
  first. Recorded as a known fragility in `design.md` §4.4.
- **`mailer` composes RFC 5322 by hand** rather than taking a third-party
  dependency — the header set is small and fixed.

## Outstanding

**The manual local-stack walkthrough has not been done.** Nothing on this branch
has ever put a byte on a real socket — every mailer and mailconsumer test uses
`FakeSender`, which is what lets `make ci` run with no relay. STARTTLS
negotiation, `PlainAuth` against a real provider, `classify`'s 4xx/5xx split
against real relay responses, and HTML rendering in a real mail client are all
unexercised. See `context.md` §7 and runbook §8.
