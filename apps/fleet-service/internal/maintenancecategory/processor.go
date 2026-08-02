package maintenancecategory

import (
	"errors"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor holds maintenance category business logic: thin reads plus
// fleet-scoped category creation.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
}

// NewProcessor constructs a Processor with the given logger, provider, and
// administrator.
func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
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
//
// Every invariant — including the non-empty fleetID that keeps a bypassing
// caller away from a uuid bind failure — lives in Build(), so the candidate is
// constructed before anything touches the database. Lookup uses the built
// model's name so the value matched against is the same trimmed value stored.
func (pr *Processor) Create(fleetID, name string, kind Kind) (Model, error) {
	candidate, err := NewBuilder().
		SetFleetID(fleetID).
		SetName(name).
		SetKind(kind).
		Build()
	if err != nil {
		return Model{}, err
	}

	existing, found, err := pr.p.FindByName(fleetID, candidate.Name(), kind)
	if err != nil {
		return Model{}, err
	}
	if found {
		return existing, nil
	}

	created, err := pr.a.Insert(candidate)
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// Lost a race: another request inserted the identical
			// (fleet_id, name, kind) row between our FindByName and this
			// Insert. Re-resolve rather than surfacing a 500 — the winner
			// is now visible and this call should be just as idempotent as
			// the found==true path above. gorm.ErrDuplicatedKey requires
			// the connection to be opened with TranslateError: true (see
			// Administrator.Insert's doc comment).
			existing, found, ferr := pr.p.FindByName(fleetID, name, kind)
			if ferr != nil {
				return Model{}, ferr
			}
			if found {
				return existing, nil
			}
		}
		return Model{}, err
	}
	return created, nil
}
