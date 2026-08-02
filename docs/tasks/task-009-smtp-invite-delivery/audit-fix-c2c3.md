# Fix: C-2 and C-3 non-blocking correctness findings (notification-service)

Scope: `apps/notification-service/internal/mailconsumer`,
`apps/notification-service/internal/mailer` only.

## C-2 — `fmtValue` did not cover every log field type

**File**: `apps/notification-service/internal/mailconsumer/consume_test.go`

Before, `fmtValue` returned `""` for anything that was not a `string` or an
`error`:

```go
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

`TestHandle_neverLogsTheToken` (consume_test.go:350-380) scans every logged
field through `fmtValue` looking for the token substring. Any field that was
a struct, a `fmt.Stringer`, a `[]byte`, etc. rendered as `""` and would never
trip the assertion — the guard was blind to exactly the kind of leak most
likely to be introduced (e.g. someone adding `.WithField("invite", inv)` for
debugging).

**Fix** (consume_test.go:382-388): render every value faithfully with
`fmt.Sprint`, added `"fmt"` to imports:

```go
// fmtValue renders ANY log field value faithfully so the leak scan in
// TestHandle_neverLogsTheToken covers every field type, not just string and
// error — a token embedded in a struct, a fmt.Stringer, a []byte, or any
// other type must still be caught.
func fmtValue(v any) string {
	return fmt.Sprint(v)
}
```

The existing ban on the accept-URL substring (`/invites/`) in log messages
(consume_test.go:376-378) was left untouched.

### Mutation-test evidence (the guard was observed to actually fail)

1. Confirmed the real, unmodified `consume.go` still passes with the new
   `fmtValue`:

   ```
   $ go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer/... -run TestHandle_neverLogsTheToken -v
   === RUN   TestHandle_neverLogsTheToken
   --- PASS: TestHandle_neverLogsTheToken (0.00s)
   PASS
   ok      github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer        0.008s
   ```

2. Temporarily injected a log field carrying the whole
   `fleetclient.Invite` (which embeds `Token`) into `consume.go`, right
   before the staleness check in `Handle`:

   ```go
   // TEMPORARY MUTATION for C-2 guard verification — DO NOT COMMIT.
   log = log.WithField("debug_invite_MUTATION", inv)
   ```

3. Re-ran the same test — it FAILED, catching the leak via the struct field
   (not a string, not an error — exactly the class of leak the old
   `fmtValue` was blind to):

   ```
   $ go test github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer/... -run TestHandle_neverLogsTheToken -v
   === RUN   TestHandle_neverLogsTheToken
       consume_test.go:373: token leaked into log field "debug_invite_MUTATION": {inv-1 f1 The Smiths a@b.com member deadbeefcafe0123 2026-08-04T19:37:16Z <nil> u1}
   --- FAIL: TestHandle_neverLogsTheToken (0.00s)
   FAIL
   FAIL    github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer        0.008s
   FAIL
   ```

4. Reverted the mutation. `git diff apps/notification-service/internal/mailconsumer/consume.go`
   is empty — `consume.go` is unchanged from the pre-existing committed
   state. Full mailconsumer suite re-run afterward, all green (see
   Verification section).

**Conclusion**: the guard is now known to actually guard something — it was
observed to fail on the exact class of leak (a non-string/non-error field
carrying the token) it previously could not detect, and passes cleanly
against the real code.

## C-3 — `TLSModeNone` did not reject configured credentials

**File**: `apps/notification-service/internal/mailer/config.go:76-88`

Before, `ConfigFromEnv` only validated one direction: credentials required
when TLS mode is *not* `none`. It never rejected credentials being present
when the mode *is* `none`. Consequence (per the finding): `net/smtp`'s
`PlainAuth` refuses to transmit credentials over an unencrypted connection,
so every send fails; `classify` marks that TRANSIENT, so all
`SMTP_SEND_ATTEMPTS` attempts are burned and the mail is dropped under the
exhaustion policy — a confusing, expensive way to discover a config mistake
that should have been caught at startup.

**Fix** (config.go:79-88): added a second validation branch, panicking at
startup, symmetric with the existing one:

```go
// The reverse combination is just as broken: net/smtp's PlainAuth refuses to
// transmit credentials over an unencrypted connection, so every send would
// fail. That failure classifies as TRANSIENT, burning the full retry budget
// before the mail is permanently dropped — a confusing, expensive way to
// discover a config mistake. Fail now instead. Empty credentials with
// TLSModeNone stay legal: that is exactly how compose and the k3s-local
// overlay point at the unauthenticated Mailpit sink (FR-DEV-2).
if cfg.TLSMode == TLSModeNone && (cfg.Username != "" || cfg.Password != "") {
	panic("SMTP_USERNAME/SMTP_PASSWORD cannot be set when SMTP_TLS_MODE is \"none\": " +
		"net/smtp refuses to send credentials over an unencrypted connection, so every message would fail")
}
```

`classify`, TLS handling, and the retry schedule were not touched.

### Confirmed the local-dev path stays legal before making the change

- `deploy/compose/docker-compose.yml:198-203` sets `SMTP_ENABLED=true`,
  `SMTP_HOST=mailpit`, `SMTP_TLS_MODE=none`, and does **not** set
  `SMTP_USERNAME`/`SMTP_PASSWORD` anywhere in the file.
- `deploy/k8s/overlays/local/kustomization.yaml:80-107` patches the
  `notification-service-config` ConfigMap to `SMTP_TLS_MODE=none` pointed at
  the bundled Mailpit sink, and does not set `SMTP_USERNAME`/`SMTP_PASSWORD`.
- `grep -rn -i 'SMTP_USERNAME\|SMTP_PASSWORD' deploy/k8s/base/ deploy/k8s/overlays/ deploy/compose/`
  returned no matches — credentials are never set anywhere in the deploy
  tree, so `config.Get("SMTP_USERNAME", "")` / `config.Get("SMTP_PASSWORD", "")`
  resolve to empty strings for both compose and the k3s-local overlay, which
  remains the legal, tested case.

### New test

`apps/notification-service/internal/mailer/config_test.go` —
`TestConfigFromEnv_credentialsWithModeNonePanics`, placed alongside the
existing `TestConfigFromEnv_credentialsRequiredUnlessModeIsNone`, following
the same `t.Setenv` + `defer recover()` idiom:

- `username set panics`
- `password set panics`
- `both set panics`
- `neither set is legal` (asserts `cfg.TLSMode == TLSModeNone`,
  `cfg.Username == ""`, `cfg.Password == ""` — the compose/k3s-local case)

No test dials a socket, starts a container, or touches a relay — all
`ConfigFromEnv` tests exercise only environment variables and panics.

## Verification

```
$ go build github.com/jtumidanski/myfleet/...
(no output — success)

$ go vet github.com/jtumidanski/myfleet/...
(no output — success)

$ go test github.com/jtumidanski/myfleet/apps/notification-service/... -v
...
--- PASS: TestHandle_neverLogsTheToken (0.00s)
...
--- PASS: TestConfigFromEnv_credentialsRequiredUnlessModeIsNone (0.00s)
    --- PASS: TestConfigFromEnv_credentialsRequiredUnlessModeIsNone/starttls_without_credentials_panics (0.00s)
    --- PASS: TestConfigFromEnv_credentialsRequiredUnlessModeIsNone/none_without_credentials_is_legal (0.00s)
--- PASS: TestConfigFromEnv_credentialsWithModeNonePanics (0.00s)
    --- PASS: TestConfigFromEnv_credentialsWithModeNonePanics/username_set_panics (0.00s)
    --- PASS: TestConfigFromEnv_credentialsWithModeNonePanics/password_set_panics (0.00s)
    --- PASS: TestConfigFromEnv_credentialsWithModeNonePanics/both_set_panics (0.00s)
    --- PASS: TestConfigFromEnv_credentialsWithModeNonePanics/neither_set_is_legal (0.00s)
...
PASS
ok      github.com/jtumidanski/myfleet/apps/notification-service/cmd
ok      github.com/jtumidanski/myfleet/apps/notification-service/internal/consumer
ok      github.com/jtumidanski/myfleet/apps/notification-service/internal/fleetclient
ok      github.com/jtumidanski/myfleet/apps/notification-service/internal/mailconsumer
ok      github.com/jtumidanski/myfleet/apps/notification-service/internal/mailer
ok      github.com/jtumidanski/myfleet/apps/notification-service/internal/notification
```

All packages under `apps/notification-service/...` pass, including the
unrelated `smtp_test.go` suite (no socket/relay tests were touched or
affected).

## Files changed

- `apps/notification-service/internal/mailconsumer/consume_test.go` — `fmtValue` fix (C-2)
- `apps/notification-service/internal/mailer/config.go` — new validation branch (C-3)
- `apps/notification-service/internal/mailer/config_test.go` — new test (C-3)

Not modified (owned by concurrent agents, left untouched):
`packages/shared-go/server/*`, `apps/fleet-service/internal/invite/*`.
