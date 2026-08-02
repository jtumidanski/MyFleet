package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
)

// TestMain parks a null logger in the error seam so the tests that exercise the
// 5xx path do not spray the real failures they simulate across the test output.
// Tests that care about the log install their own; the one that covers the
// unwired fallback clears it explicitly.
func TestMain(m *testing.M) {
	quiet, _ := logrustest.NewNullLogger()
	SetErrorLogger(quiet)
	os.Exit(m.Run())
}

// writeErrorBody renders err through WriteError and returns the recorder plus
// the decoded single APIError, failing the test if the envelope is malformed.
func writeErrorBody(t *testing.T, err error) (*httptest.ResponseRecorder, APIError) {
	t.Helper()
	rec := httptest.NewRecorder()
	WriteError(rec, err)
	var body struct {
		Errors []APIError `json:"errors"`
	}
	if decodeErr := json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&body); decodeErr != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), decodeErr)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %d, want 1; body: %s", len(body.Errors), rec.Body.String())
	}
	return rec, body.Errors[0]
}

func TestStatusFor_mapsDomainErrors(t *testing.T) {
	cases := map[error]int{
		ErrBadRequest:            400,
		ErrUnauthorized:          401,
		ErrForbidden:             403,
		ErrNotFound:              404,
		ErrConflict:              409,
		ErrGone:                  410,
		ErrRequestEntityTooLarge: 413,
		ErrUnsupportedMediaType:  415,
		ErrValidation:            422,
		ErrTooManyRequests:       429,
	}
	for err, want := range cases {
		if got := StatusFor(err); got != want {
			t.Fatalf("StatusFor(%v)=%d want %d", err, got, want)
		}
	}
}

// TestCodeFor_namesEveryMappedStatus pins the JSON:API `code` string for every
// status StatusFor can produce. 400 and 413 previously fell through to
// "internal_error", which tells a client nothing about what it did wrong.
func TestCodeFor_namesEveryMappedStatus(t *testing.T) {
	cases := map[int]string{
		400: "bad_request",
		401: "unauthorized",
		403: "forbidden",
		404: "not_found",
		409: "conflict",
		410: "gone",
		413: "payload_too_large",
		415: "unsupported_media_type",
		422: "validation_error",
		429: "too_many_requests",
		500: "internal_error",
	}
	for status, want := range cases {
		if got := codeFor(status); got != want {
			t.Fatalf("codeFor(%d)=%q want %q", status, got, want)
		}
	}
}

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

// TestWriteError_500RedactsTheUnderlyingErrorText is the regression guard for
// SEC-09. Callers hand raw repository errors to WriteError, so before the fix
// the driver's message — schema name, table name, SQLSTATE — was rendered
// verbatim as the client-facing `title`. Nothing distinctive from the error may
// survive anywhere in the body.
func TestWriteError_500RedactsTheUnderlyingErrorText(t *testing.T) {
	leaky := errors.New(`pq: relation "fleet.fleet_invites" does not exist (SQLSTATE 42P01)`)

	rec, got := writeErrorBody(t, leaky)

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got.Status != "500" || got.Code != "internal_error" {
		t.Fatalf("status/code = %q/%q, want 500/internal_error", got.Status, got.Code)
	}
	if got.Title != InternalErrorTitle {
		t.Fatalf("title = %q, want %q", got.Title, InternalErrorTitle)
	}
	body := rec.Body.String()
	for _, secret := range []string{
		"pq:",
		"relation",
		"fleet.fleet_invites",
		"fleet_invites",
		"SQLSTATE",
		"42P01",
		"does not exist",
	} {
		if strings.Contains(body, secret) {
			t.Fatalf("500 body leaked %q: %s", secret, body)
		}
	}
}

// TestWriteError_500RedactsAWrappedDriverError: the leak survives wrapping too.
// A caller that annotates a driver failure with fmt.Errorf still maps to 500,
// and the driver text still rides along inside err.Error().
func TestWriteError_500RedactsAWrappedDriverError(t *testing.T) {
	wrapped := fmt.Errorf("insert invite for fleet %s: %w", "fleet-42",
		errors.New(`ERROR: duplicate key value violates unique constraint "uniq_invites_token" (SQLSTATE 23505)`))

	rec, got := writeErrorBody(t, wrapped)

	if rec.Code != 500 || got.Title != InternalErrorTitle {
		t.Fatalf("status/title = %d/%q, want 500/%q", rec.Code, got.Title, InternalErrorTitle)
	}
	body := rec.Body.String()
	for _, secret := range []string{"duplicate key", "uniq_invites_token", "SQLSTATE", "23505", "fleet-42", "insert invite"} {
		if strings.Contains(body, secret) {
			t.Fatalf("500 body leaked %q: %s", secret, body)
		}
	}
}

// TestWriteError_500DropsDetail: Detail() is a deliberate client-facing
// sentence on a 4xx. On an arbitrary 5xx chain there is no way to know that,
// so it is suppressed along with the title.
func TestWriteError_500DropsDetail(t *testing.T) {
	rec, got := writeErrorBody(t, Detailed(errors.New("pq: password authentication failed for user \"myfleet\""), "connect to fleet_db as myfleet"))

	if rec.Code != 500 {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got.Detail != "" {
		t.Fatalf("detail = %q, want empty on a 5xx", got.Detail)
	}
	body := rec.Body.String()
	for _, secret := range []string{"password authentication", "myfleet", "fleet_db", "detail"} {
		if strings.Contains(body, secret) {
			t.Fatalf("500 body leaked %q: %s", secret, body)
		}
	}
}

// TestWriteError_nilErrorDoesNotPanic: StatusFor maps nil to 500 like any other
// unrecognised error, and the 500 path must not dereference it.
func TestWriteError_nilErrorDoesNotPanic(t *testing.T) {
	rec, got := writeErrorBody(t, nil)

	if rec.Code != 500 || got.Title != InternalErrorTitle {
		t.Fatalf("status/title = %d/%q, want 500/%q", rec.Code, got.Title, InternalErrorTitle)
	}
}

// TestWriteError_4xxSentinelsKeepTheirMessageAndCode pins the other half of the
// SEC-09 split: the redaction is chosen by mapped status, so every deliberate
// 4xx message written for the client must be byte-identical to what it was.
func TestWriteError_4xxSentinelsKeepTheirMessageAndCode(t *testing.T) {
	cases := []struct {
		err    error
		status string
		code   string
		title  string
	}{
		{ErrBadRequest, "400", "bad_request", "bad request"},
		{ErrUnauthorized, "401", "unauthorized", "unauthorized"},
		{ErrForbidden, "403", "forbidden", "forbidden"},
		{ErrNotFound, "404", "not_found", "not found"},
		{ErrConflict, "409", "conflict", "conflict"},
		{ErrGone, "410", "gone", "gone"},
		{ErrRequestEntityTooLarge, "413", "payload_too_large", "request entity too large"},
		{ErrUnsupportedMediaType, "415", "unsupported_media_type", "unsupported media type"},
		{ErrValidation, "422", "validation_error", "validation"},
		{ErrTooManyRequests, "429", "too_many_requests", "too many requests"},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			_, got := writeErrorBody(t, tc.err)
			if got.Status != tc.status || got.Code != tc.code || got.Title != tc.title {
				t.Fatalf("got %q/%q/%q, want %q/%q/%q — 4xx text is deliberate and must not be redacted",
					got.Status, got.Code, got.Title, tc.status, tc.code, tc.title)
			}
			if got.Detail != "" {
				t.Fatalf("detail = %q, want empty for a plain sentinel", got.Detail)
			}
		})
	}
}

// TestWriteError_4xxKeepsACallerAuthoredWrap: auth-service renders its
// themePreference validation message by wrapping ErrValidation, and that text
// reaching the client is the point of the test over there. Redacting by status
// must leave it alone.
func TestWriteError_4xxKeepsACallerAuthoredWrap(t *testing.T) {
	wrapped := fmt.Errorf("themePreference must be one of light, dark, system: %w", ErrValidation)

	rec, got := writeErrorBody(t, wrapped)

	if rec.Code != 422 {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if got.Title != wrapped.Error() {
		t.Fatalf("title = %q, want %q", got.Title, wrapped.Error())
	}
}

// TestWriteError_4xxDetailSurvivesTheRedaction: the invite-accept preconditions
// ride in `detail` on a 409. The SEC-09 fix must not take them with it.
func TestWriteError_4xxDetailSurvivesTheRedaction(t *testing.T) {
	for _, detail := range []string{
		"invite has already been accepted",
		"invite has expired",
		"invite was issued to a different account",
		"invite cannot be accepted",
	} {
		rec, got := writeErrorBody(t, Detailed(ErrConflict, detail))
		if rec.Code != 409 || got.Title != "conflict" || got.Code != "conflict" {
			t.Fatalf("status/title/code = %d/%q/%q, want 409/conflict/conflict", rec.Code, got.Title, got.Code)
		}
		if got.Detail != detail {
			t.Fatalf("detail = %q, want %q", got.Detail, detail)
		}
	}
}

// installErrorLogger swaps in a capturing logger for the duration of one test
// and restores whatever was there before, so tests that assert on the log do
// not leak a logger into the tests that assert nothing was logged.
func installErrorLogger(t *testing.T) *logrustest.Hook {
	t.Helper()
	prev := errorLog.Load()
	t.Cleanup(func() { errorLog.Store(prev) })

	log, hook := logrustest.NewNullLogger()
	log.SetLevel(logrus.DebugLevel)
	SetErrorLogger(log)
	return hook
}

// TestWriteError_500LogsTheErrorItRedacts is the point of the whole exercise:
// the log and the response body must DIVERGE. Redacting the body (SEC-09) took
// away the only copy of the failure anyone could see, so both halves are
// asserted together — a change that re-leaks the body, or one that stops
// logging, fails right here.
func TestWriteError_500LogsTheErrorItRedacts(t *testing.T) {
	hook := installErrorLogger(t)
	leaky := errors.New(`pq: relation "fleet.fleet_invites" does not exist (SQLSTATE 42P01)`)

	rec, got := writeErrorBody(t, leaky)

	// Half one: the operator can see everything.
	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want exactly 1", len(entries))
	}
	entry := entries[0]
	if entry.Level != logrus.ErrorLevel {
		t.Fatalf("level = %v, want error", entry.Level)
	}
	logged, ok := entry.Data[logrus.ErrorKey].(error)
	if !ok || logged == nil {
		t.Fatalf("log entry carries no error field: %+v", entry.Data)
	}
	if logged.Error() != leaky.Error() {
		t.Fatalf("logged error = %q, want the full underlying text %q", logged.Error(), leaky.Error())
	}
	if entry.Data["status"] != 500 {
		t.Fatalf("log status field = %v, want 500", entry.Data["status"])
	}

	// Half two: the client can see none of it.
	if rec.Code != 500 || got.Title != InternalErrorTitle {
		t.Fatalf("status/title = %d/%q, want 500/%q", rec.Code, got.Title, InternalErrorTitle)
	}
	body := rec.Body.String()
	for _, secret := range []string{"pq:", "relation", "fleet.fleet_invites", "SQLSTATE", "42P01"} {
		if strings.Contains(body, secret) {
			t.Fatalf("500 body leaked %q despite the redaction: %s", secret, body)
		}
	}
}

// TestWriteError_4xxLogsNothing: a 404 or a 422 is the client being wrong, not
// the server failing. At ~190 WriteError call sites, logging those at Error
// would bury the 5xx entries the test above depends on.
func TestWriteError_4xxLogsNothing(t *testing.T) {
	for _, err := range []error{
		ErrBadRequest, ErrUnauthorized, ErrForbidden, ErrNotFound, ErrConflict,
		ErrGone, ErrRequestEntityTooLarge, ErrUnsupportedMediaType, ErrValidation,
		ErrTooManyRequests, Detailed(ErrConflict, "invite has expired"),
	} {
		t.Run(err.Error(), func(t *testing.T) {
			hook := installErrorLogger(t)
			writeErrorBody(t, err)
			if n := len(hook.AllEntries()); n != 0 {
				t.Fatalf("a %d response wrote %d log entries, want 0: %+v",
					StatusFor(err), n, hook.AllEntries())
			}
		})
	}
}

// TestWriteError_withNoLoggerInstalledDoesNotPanic: a nil logger on the error
// path would turn every 500 into a crash, which is strictly worse than the
// disclosure this package is fixing. The fallback must also still surface the
// error rather than dropping it, so the standard logger's output is captured
// rather than merely silenced.
func TestWriteError_withNoLoggerInstalledDoesNotPanic(t *testing.T) {
	prev := errorLog.Load()
	t.Cleanup(func() { errorLog.Store(prev) })
	errorLog.Store(nil)

	std := logrus.StandardLogger()
	prevOut, prevFmt := std.Out, std.Formatter
	t.Cleanup(func() { std.Out, std.Formatter = prevOut, prevFmt })
	var captured bytes.Buffer
	std.Out = &captured
	std.Formatter = &logrus.TextFormatter{DisableColors: true}

	rec, got := writeErrorBody(t, errors.New("boom from an unwired service"))

	if rec.Code != 500 || got.Title != InternalErrorTitle {
		t.Fatalf("status/title = %d/%q, want 500/%q", rec.Code, got.Title, InternalErrorTitle)
	}
	if !strings.Contains(captured.String(), "boom from an unwired service") {
		t.Fatalf("the fallback logger discarded the error instead of surfacing it: %q", captured.String())
	}
	if strings.Contains(rec.Body.String(), "boom from an unwired service") {
		t.Fatalf("the fallback path leaked the error into the body: %s", rec.Body.String())
	}
}

// TestNew_installsTheErrorLogger pins the zero-caller-churn property: the four
// service mains each call server.New(log) once and nothing else, so if New
// stops installing the logger every 5xx across every service silently falls
// back to stderr and the per-service log fields vanish.
func TestNew_installsTheErrorLogger(t *testing.T) {
	prev := errorLog.Load()
	t.Cleanup(func() { errorLog.Store(prev) })
	errorLog.Store(nil)

	log, hook := logrustest.NewNullLogger()
	_ = New(log)

	writeErrorBody(t, errors.New("failure after bootstrap"))

	if n := len(hook.AllEntries()); n != 1 {
		t.Fatalf("entries = %d, want 1 — New must install its logger for WriteError", n)
	}
}

// TestSetErrorLogger_isSafeUnderConcurrentUse is the test that fails under
// -race if errorLog is ever demoted from atomic.Pointer to a plain global: a
// startup/reconfiguration write racing the per-request reads is exactly the
// access pattern of the real thing. Verified to fail that way before being
// committed — see the report.
func TestSetErrorLogger_isSafeUnderConcurrentUse(t *testing.T) {
	prev := errorLog.Load()
	t.Cleanup(func() { errorLog.Store(prev) })

	quiet, _ := logrustest.NewNullLogger()
	SetErrorLogger(quiet)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				WriteError(httptest.NewRecorder(), errors.New("concurrent failure"))
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				next, _ := logrustest.NewNullLogger()
				SetErrorLogger(next)
			}
		}()
	}
	wg.Wait()
}
