package maintenancerecord

import (
	"errors"
	"io"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newTestProcessor(t *testing.T) (*Processor, string) {
	t.Helper()
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, 0)
	return NewProcessor(log, NewProvider(db), a), id
}

// The data-access layer's ErrNotFound is a package-local sentinel. Handlers
// only understand server.ErrNotFound; the translation is this layer's job and
// is the reason it exists at all for these two methods.
func TestProcessorAttachDocument_translatesNotFound(t *testing.T) {
	pr, _ := newTestProcessor(t)

	if _, err := pr.AttachDocument("no-such-record", "media-1"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("err = %v, want server.ErrNotFound", err)
	}
}

// The cap error is already a server sentinel; it must arrive unchanged so
// StatusFor maps it to 422 rather than being swallowed as a 404 or a 500.
func TestProcessorAttachDocument_passesValidationThrough(t *testing.T) {
	db := newTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	a := NewAdministrator(db)
	id := insertRecord(t, a, "v1", "cat-1", 5, MaxDocuments)
	pr := NewProcessor(log, NewProvider(db), a)

	if _, err := pr.AttachDocument(id, "one-too-many"); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want server.ErrValidation", err)
	}
}

func TestProcessorAttachDocument_returnsTheUpdatedModel(t *testing.T) {
	pr, id := newTestProcessor(t)

	m, err := pr.AttachDocument(id, "media-1")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if got := m.DocumentMediaIDs(); len(got) != 1 || got[0] != "media-1" {
		t.Errorf("DocumentMediaIDs() = %v, want [media-1]", got)
	}
}

func TestProcessorDetachDocument_translatesNotFound(t *testing.T) {
	pr, id := newTestProcessor(t)

	if err := pr.DetachDocument(id, "never-attached"); !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("err = %v, want server.ErrNotFound", err)
	}
}

func TestProcessorDetachDocument_succeedsForALiveAttachment(t *testing.T) {
	pr, id := newTestProcessor(t)
	if _, err := pr.AttachDocument(id, "media-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}

	if err := pr.DetachDocument(id, "media-1"); err != nil {
		t.Fatalf("detach: %v", err)
	}
	m, err := pr.GetByID(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(m.DocumentMediaIDs()) != 0 {
		t.Errorf("DocumentMediaIDs() = %v, want empty", m.DocumentMediaIDs())
	}
}
