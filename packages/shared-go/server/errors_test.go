package server

import "testing"

func TestStatusFor_mapsDomainErrors(t *testing.T) {
	cases := map[error]int{
		ErrUnauthorized:          401,
		ErrForbidden:             403,
		ErrNotFound:              404,
		ErrConflict:              409,
		ErrGone:                  410,
		ErrRequestEntityTooLarge: 413,
		ErrUnsupportedMediaType:  415,
		ErrValidation:            422,
	}
	for err, want := range cases {
		if got := StatusFor(err); got != want {
			t.Fatalf("StatusFor(%v)=%d want %d", err, got, want)
		}
	}
}
