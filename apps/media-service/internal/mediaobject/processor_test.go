package mediaobject

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestAuthorizeAccess_404CrossFleet(t *testing.T) {
	obj, err := NewBuilder().
		SetFleetID("fleet-a").
		SetUploadedByUserID("u1").
		SetBucket("media").
		SetObjectKey("fleet-a/o1/file.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("file.jpg").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	// Same fleet → allowed.
	if err := AuthorizeAccess(obj, "fleet-a"); err != nil {
		t.Fatalf("same-fleet access must succeed, got %v", err)
	}

	// Different fleet → 404 (never leak existence).
	if err := AuthorizeAccess(obj, "fleet-b"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("cross-fleet access must be 404, got %v", err)
	}
}

func TestMarkReady_requiresProcessing(t *testing.T) {
	base := NewBuilder().
		SetFleetID("f1").
		SetUploadedByUserID("u1").
		SetBucket("media").
		SetObjectKey("f1/o1/file.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("file.jpg")

	// ready is only valid FROM processing.
	processing, _ := base.Build()
	processing = processing.WithStatus(StatusProcessing)
	got, err := MarkReady(processing)
	if err != nil {
		t.Fatalf("processing→ready must be allowed, got %v", err)
	}
	if got.Status() != StatusReady {
		t.Fatalf("expected status ready, got %q", got.Status())
	}

	// uploaded → ready is invalid (must go through processing first).
	uploaded, _ := base.Build() // defaults to uploaded
	if _, err := MarkReady(uploaded); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("uploaded→ready must be a conflict, got %v", err)
	}
}

func TestMarkProcessing_requiresUploaded(t *testing.T) {
	base := NewBuilder().
		SetFleetID("f1").
		SetUploadedByUserID("u1").
		SetBucket("media").
		SetObjectKey("f1/o1/file.jpg").
		SetContentType("image/jpeg").
		SetOriginalFilename("file.jpg")

	uploaded, _ := base.Build()
	got, err := MarkProcessing(uploaded)
	if err != nil {
		t.Fatalf("uploaded→processing must be allowed, got %v", err)
	}
	if got.Status() != StatusProcessing {
		t.Fatalf("expected status processing, got %q", got.Status())
	}

	// processing → processing is invalid.
	already, _ := base.Build()
	already = already.WithStatus(StatusProcessing)
	if _, err := MarkProcessing(already); !errors.Is(err, server.ErrConflict) {
		t.Fatalf("processing→processing must be a conflict, got %v", err)
	}
}
