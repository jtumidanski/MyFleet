package server

import "testing"

func TestStatusFor_mapsDomainErrors(t *testing.T) {
	cases := map[error]int{
		ErrBadRequest:            400,
		ErrUnauthorized:          401,
		ErrForbidden:             403,
		ErrNotFound:              404,
		ErrConflict:              409,
		ErrGone:                  410,
		ErrRequestEntityTooLarge: 413,
		ErrValidation:            422,
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
		422: "validation_error",
		500: "internal_error",
	}
	for status, want := range cases {
		if got := codeFor(status); got != want {
			t.Fatalf("codeFor(%d)=%q want %q", status, got, want)
		}
	}
}
