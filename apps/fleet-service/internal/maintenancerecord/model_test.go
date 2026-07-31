package maintenancerecord

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func validBuilder() *Builder {
	return NewBuilder().
		SetVehicleID("v1").
		SetCategoryID("c1").
		SetPerformedAt(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
}

func TestBuild_requiresVehicleCategoryAndDate(t *testing.T) {
	if _, err := NewBuilder().SetCategoryID("c1").SetPerformedAt(time.Now()).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing vehicleID err = %v, want ErrValidation", err)
	}
	if _, err := NewBuilder().SetVehicleID("v1").SetPerformedAt(time.Now()).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing categoryID err = %v, want ErrValidation", err)
	}
	// A maintenance log with a silently-guessed date is worse than one that
	// refuses to save (PRD FR-REC-5).
	if _, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("zero performedAt err = %v, want ErrValidation", err)
	}
}

// The limit is 200 RUNES, not bytes: a 200-character limit that rejects 60
// emoji is a bug, not a security control (design D4).
func TestValidate_descriptionLimitIsCountedInRunes(t *testing.T) {
	okMultibyte := strings.Repeat("é", MaxDescriptionRunes)
	if _, err := validBuilder().SetDescription(okMultibyte).Build(); err != nil {
		t.Fatalf("200 multi-byte runes rejected: %v", err)
	}

	tooLong := strings.Repeat("a", MaxDescriptionRunes+1)
	m, err := validBuilder().SetDescription(tooLong).Build()
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("201 runes err = %v, want ErrValidation", err)
	}
	if m.Description() != "" {
		t.Fatal("an over-length description must be rejected, never truncated")
	}
}

// Surrounding whitespace is trimmed at the setter so measurement and storage
// agree, matching the client's z.string().trim().
func TestSetDescription_trims(t *testing.T) {
	m, err := validBuilder().SetDescription("  Cat-back exhaust  ").Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.Description() != "Cat-back exhaust" {
		t.Fatalf("Description() = %q, want trimmed", m.Description())
	}
	// A description of only whitespace is empty, not a 1-rune value.
	padded := strings.Repeat(" ", 50) + strings.Repeat("a", MaxDescriptionRunes) + strings.Repeat(" ", 50)
	if _, err := validBuilder().SetDescription(padded).Build(); err != nil {
		t.Fatalf("whitespace-padded 200 runes rejected: %v", err)
	}
}

// The cap bounds the ids= query string on media-service's internal endpoint,
// the per-record fan-out when an attachment list is expanded, and the InsertTx
// document loop (design D9).
func TestValidate_capsAttachments(t *testing.T) {
	ids := make([]string, MaxDocuments)
	for i := range ids {
		ids[i] = "m" + string(rune('a'+i))
	}
	if _, err := validBuilder().SetDocumentMediaIDs(ids).Build(); err != nil {
		t.Fatalf("%d attachments rejected: %v", MaxDocuments, err)
	}
	if _, err := validBuilder().SetDocumentMediaIDs(append(ids, "one-too-many")).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("%d attachments err = %v, want ErrValidation", MaxDocuments+1, err)
	}
}

// Validate is called from both write paths, so PATCH is guarded too — the
// builder alone would leave Processor.Update unchecked (design D4).
func TestValidate_rejectsAnOverLongUpdate(t *testing.T) {
	m, err := validBuilder().Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := Validate(m.WithDescription(strings.Repeat("a", MaxDescriptionRunes+1))); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("Validate(over-long update) = %v, want ErrValidation", err)
	}
}
