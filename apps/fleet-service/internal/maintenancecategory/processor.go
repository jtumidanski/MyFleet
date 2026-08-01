package maintenancecategory

import (
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

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

// List returns a page of maintenance categories, optionally filtered by kind.
func (pr *Processor) List(kind Kind, page server.Page) ([]Model, int, error) {
	return pr.p.List(kind, page)
}

// IDsByKind returns every category ID of a kind. It is what satisfies
// maintenancerecord.CategoryAccessor, so the record list can filter by kind
// without importing this package's data access.
func (pr *Processor) IDsByKind(kind Kind) ([]string, error) {
	return pr.p.IDsByKind(kind)
}
