package membership

import (
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// OwnerCounter is satisfied by both Provider (real) and stubProvider (test).
type OwnerCounter interface{ CountOwners(fleetID string) (int, error) }

// Processor contains membership business logic.
type Processor struct {
	log logrus.FieldLogger
	p   OwnerCounter
}

func NewProcessor(log logrus.FieldLogger, p OwnerCounter) *Processor {
	return &Processor{log: log, p: p}
}

// ValidateRemoval enforces FR-FLEET-4: an owner cannot remove themselves if they
// are the only owner.
func (pr *Processor) ValidateRemoval(fleetID, actorUserID, targetUserID, targetRole string) error {
	if actorUserID == targetUserID && targetRole == "owner" {
		n, err := pr.p.CountOwners(fleetID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return server.ErrConflict
		}
	}
	return nil
}
