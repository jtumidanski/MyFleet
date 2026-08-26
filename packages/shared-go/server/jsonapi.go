package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"

	"github.com/sirupsen/logrus"
)

// Resource is the JSON:API resource object (design §12, PRD §5.1).
type Resource struct {
	Type          string         `json:"type"`
	ID            string         `json:"id"`
	Attributes    any            `json:"attributes"`
	Relationships map[string]any `json:"relationships,omitempty"`
}

type Document struct {
	Data  any            `json:"data,omitempty"`
	Meta  any            `json:"meta,omitempty"`
	Links map[string]any `json:"links,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// InternalErrorTitle is the only `title` a 5xx response ever carries. See
// WriteError for why the real error text never reaches the client.
const InternalErrorTitle = "internal server error"

// errorLog is the logger WriteError writes 5xx faults to. Redacting the
// response body (SEC-09) removes the client's copy of the error, so the server
// has to keep one or the failure becomes invisible.
//
// atomic.Pointer, not a plain global with a mutex: the value is written once at
// startup by New and read on every erroring request, so a lock would sit on the
// hot path to guard a write that happens once. The atomic load is race-free
// with no contention. It holds a *logrus.FieldLogger rather than the interface
// because atomic.Pointer needs a pointer type.
var errorLog atomic.Pointer[logrus.FieldLogger]

// SetErrorLogger installs the logger WriteError uses for 5xx responses. New
// calls it with the server's logger, so every service gets this without any
// per-service wiring; call it directly only when there is no Server (a test, or
// a handler mounted outside the shared bootstrap).
//
// Safe to call concurrently with in-flight requests. Passing nil restores the
// logrus standard-logger fallback.
func SetErrorLogger(log logrus.FieldLogger) {
	if log == nil {
		errorLog.Store(nil)
		return
	}
	errorLog.Store(&log)
}

// errorLogger never returns nil. A misconfigured or absent logger must not turn
// an error response into a panic, and it must not silently discard the fault
// either — the logrus standard logger writes to stderr, which is where a
// container's logs already go.
func errorLogger() logrus.FieldLogger {
	if p := errorLog.Load(); p != nil && *p != nil {
		return *p
	}
	return logrus.StandardLogger()
}

// WriteError renders the standard envelope for a domain/HTTP error.
//
// The title is chosen by the MAPPED STATUS, not by inspecting the message:
//
//   - 4xx — err.Error(). Every 4xx comes from a sentinel in errors.go (or a
//     caller-authored wrap of one), whose message was written to be shown to
//     the client. Those bodies are unchanged.
//   - 5xx — the fixed InternalErrorTitle. StatusFor maps anything it does not
//     recognise to 500, and callers pass raw repository errors straight in, so
//     err.Error() here is whatever GORM or the pq driver produced: table names,
//     column names, SQLSTATE codes, sometimes parameter values. None of that is
//     the client's business (SEC-09).
//
// A `detail` from Detailed is rendered on every 4xx and on a 503, and on
// nothing else.
//
// A 500 is an ARBITRARY error chain — a raw repository error, whatever GORM or
// the driver produced — and a Detailed wrapper around one is no evidence its
// sentence was written for a client; TestWriteError_500DropsDetail exhibits one
// that names a database and a database user. So a 500 stays wholly redacted.
//
// A 503 is different in kind: ErrServiceUnavailable is a sentinel a caller
// reaches for deliberately, exactly as ErrConflict is, and Detailed over it is
// as authored as Detailed over a 4xx. It is also the one 5xx where the client
// needs a sentence — "the transfer was rolled back" is the difference between
// an operator who knows the write did not land and one who has to go and check.
// The title stays the fixed InternalErrorTitle either way: only the authored
// sentence survives, never err.Error().
//
// The 503 case matches the CONCRETE Detailed wrapper rather than an
// `interface{ Detail() string }`, so a driver or third-party error that happens
// to carry a Detail() method cannot publish its own text through this path.
//
// The text redacted from a 5xx body is written to the error logger instead, so
// the fault stays diagnosable server-side; see SetErrorLogger. Only the error
// value is logged — never the request body, headers or credentials. Sub-500
// responses log nothing: they are routine client mistakes and logging them at
// ~190 call sites would be pure noise.
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
	if status >= 500 {
		errorLogger().WithError(err).WithField("status", status).
			Error("request failed; error text redacted from the response body")
	}
	if status < 500 {
		apiErr.Title = err.Error()
		var d interface{ Detail() string }
		if errors.As(err, &d) {
			apiErr.Detail = d.Detail()
		}
	} else if status == 503 {
		if de := (*detailedError)(nil); errors.As(err, &de) {
			// Title stays redacted; only the authored sentence survives.
			apiErr.Detail = de.detail
		}
	}
	WriteJSON(w, status, struct {
		Errors []APIError `json:"errors"`
	}{Errors: []APIError{apiErr}})
}
