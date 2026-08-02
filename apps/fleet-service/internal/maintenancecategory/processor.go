package maintenancecategory

import (
	"strings"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// maxCategoryNameLen bounds a free-form category name. It is a UI-legibility
// limit, not a storage one — the column is unbounded text.
const maxCategoryNameLen = 60

// Processor holds maintenance category business logic (thin reads only —
// categories are global/system data with no fleet-scoped mutations).
type Processor struct {
	log logrus.FieldLogger
	p   Provider
}

// NewProcessor constructs a Processor with the given logger and provider.
func NewProcessor(log logrus.FieldLogger, p Provider) *Processor {
	return &Processor{log: log, p: p}
}

// List returns a page of categories visible to the given fleet.
func (pr *Processor) List(kind Kind, fleetID string, page server.Page) ([]Model, int, error) {
	return pr.p.List(kind, fleetID, page)
}

// IDsByKind returns every category ID of a kind visible to the given fleet.
// It is what satisfies maintenancerecord.CategoryAccessor, so the record list
// can filter by kind without importing this package's data access.
func (pr *Processor) IDsByKind(kind Kind, fleetID string) ([]string, error) {
	return pr.p.IDsByKind(kind, fleetID)
}

// Create resolves a free-form category name to a category, creating a
// fleet-scoped one only when nothing visible already matches.
//
// Matching is case-insensitive against system rows AND the fleet's own, so a
// user typing "oil change" gets the seeded "Oil Change" instead of a
// near-duplicate that would split their history in two (design §10.1).
func (pr *Processor) Create(fleetID, name string, kind Kind) (Model, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxCategoryNameLen {
		return Model{}, server.ErrValidation
	}
	if kind != KindMaintenance && kind != KindModification {
		return Model{}, server.ErrValidation
	}

	existing, found, err := pr.p.FindByName(fleetID, name, kind)
	if err != nil {
		return Model{}, err
	}
	if found {
		return existing, nil
	}

	return pr.p.Create(Entity{
		ID:            uuid.NewString(),
		Name:          name,
		SystemDefined: false,
		Kind:          string(kind),
		FleetID:       &fleetID,
	})
}
