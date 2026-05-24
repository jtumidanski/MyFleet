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

// List returns a page of maintenance categories.
func (pr *Processor) List(page server.Page) ([]Model, int, error) {
	return pr.p.List(page)
}
