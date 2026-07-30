package mediaobject

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// TestClassifyUploadError_mapsMaxBytesTo413 exercises the real
// *http.MaxBytesError produced by http.MaxBytesReader (the same mechanism
// PUT /media/{id}/content uses to bound uploads), rather than a hand-rolled
// stand-in, so the mapping is verified against actual SDK behavior.
func TestClassifyUploadError_mapsMaxBytesTo413(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/media/x/content", strings.NewReader("more bytes than the cap allows"))
	body := http.MaxBytesReader(rec, req.Body, 4)

	_, readErr := io.ReadAll(body)
	if readErr == nil {
		t.Fatal("expected MaxBytesReader to reject a body over its cap")
	}

	mapped := classifyUploadError(readErr)
	if !errors.Is(mapped, server.ErrRequestEntityTooLarge) {
		t.Fatalf("classifyUploadError(%v) = %v, want ErrRequestEntityTooLarge", readErr, mapped)
	}
}

// TestClassifyUploadError_passesOtherErrorsThrough verifies the mapping is
// scoped to the MaxBytesError case only: any other error (404/409/500, ...)
// must come back unchanged so the caller's existing error handling applies.
func TestClassifyUploadError_passesOtherErrorsThrough(t *testing.T) {
	other := errors.New("simulated store failure")
	if got := classifyUploadError(other); got != other {
		t.Fatalf("classifyUploadError(%v) = %v, want passthrough", other, got)
	}

	if got := classifyUploadError(server.ErrNotFound); !errors.Is(got, server.ErrNotFound) {
		t.Fatalf("classifyUploadError(ErrNotFound) = %v, want ErrNotFound unchanged", got)
	}
}
