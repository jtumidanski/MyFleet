package server

import (
	"encoding/json"
	"errors"
	"net/http"
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
// A `detail` from Detailed is likewise rendered only on 4xx — a detail is a
// deliberate client-facing sentence, and there is no way to know that an
// arbitrary 5xx error chain's Detail() is one.
//
// The redacted text is NOT logged here: this function has no logger, and the
// server's log lives on Server. Callers that turn an error into a 5xx are
// responsible for logging it before calling WriteError.
func WriteError(w http.ResponseWriter, err error) {
	status := StatusFor(err)
	apiErr := APIError{
		Status: itoa(status),
		Code:   codeFor(status),
		Title:  InternalErrorTitle,
	}
	if status < 500 {
		apiErr.Title = err.Error()
		var d interface{ Detail() string }
		if errors.As(err, &d) {
			apiErr.Detail = d.Detail()
		}
	}
	WriteJSON(w, status, struct {
		Errors []APIError `json:"errors"`
	}{Errors: []APIError{apiErr}})
}
