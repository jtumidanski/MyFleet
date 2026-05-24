package invite

import (
	"errors"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains invite business logic.
type Processor struct {
	log logrus.FieldLogger
}

func NewProcessor(log logrus.FieldLogger) *Processor {
	return &Processor{log: log}
}

// ValidateAccept enforces FR-FLEET-3: invite must be for the same email, not
// yet accepted, and not expired. Any violation → server.ErrConflict (409).
func (pr *Processor) ValidateAccept(inv Model, authedEmail string) error {
	if inv.AcceptedAt() != nil {
		return server.ErrConflict
	}
	if !inv.ExpiresAt().After(time.Now()) {
		return server.ErrConflict
	}
	if !strings.EqualFold(inv.Email(), authedEmail) {
		return server.ErrConflict
	}
	return nil
}

// GetByID fetches an invite by ID.
func (pr *Processor) GetByID(p Provider, id string) (Model, error) {
	m, err := p.GetByID(id)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}
