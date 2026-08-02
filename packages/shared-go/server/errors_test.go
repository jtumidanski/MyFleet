package server

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

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
