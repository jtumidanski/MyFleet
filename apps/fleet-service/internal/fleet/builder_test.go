package fleet

import (
	"errors"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestBuilder_rejectsEmptyName(t *testing.T) {
	_, err := NewBuilder().SetCreatedByUserID("u1").Build()
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("empty name must return ErrValidation, got %v", err)
	}
}

func TestBuilder_acceptsValidName(t *testing.T) {
	m, err := NewBuilder().SetName("My Fleet").SetCreatedByUserID("u1").Build()
	if err != nil {
		t.Fatalf("valid fleet build must not error, got %v", err)
	}
	if m.Name() != "My Fleet" {
		t.Fatalf("expected name %q, got %q", "My Fleet", m.Name())
	}
	if m.ID() == "" {
		t.Fatal("fleet ID must be non-empty after build")
	}
}
