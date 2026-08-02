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

// WriteError renders the standard envelope for a domain/HTTP error. If the
// error chain carries a detail (see Detailed), it is rendered in the JSON:API
// `detail` field; otherwise the field is omitted and the body is unchanged from
// what every existing caller already produces.
func WriteError(w http.ResponseWriter, err error) {
	status := StatusFor(err)
	apiErr := APIError{
		Status: itoa(status),
		Code:   codeFor(status),
		Title:  err.Error(),
	}
	var d interface{ Detail() string }
	if errors.As(err, &d) {
		apiErr.Detail = d.Detail()
	}
	WriteJSON(w, status, struct {
		Errors []APIError `json:"errors"`
	}{Errors: []APIError{apiErr}})
}
