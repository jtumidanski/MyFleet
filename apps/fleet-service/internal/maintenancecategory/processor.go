package maintenancecategory

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// maxCategoryNameLen bounds a free-form category name, counted in RUNES (not
// bytes): a 60-character Cyrillic or CJK name is 120-180 bytes, and rejecting
// it on byte length would punish non-Latin scripts for a limit stated in
// characters. It is a UI-legibility limit, not a storage one — the column is
// unbounded text.
const maxCategoryNameLen = 60

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
// fleetID is validated here, not only at the HTTP layer: FindByName("", ...)
// silently matches nothing (visibleTo's empty-fleetID branch), but an insert
// with an empty FleetID binds "" into a uuid column and fails at bind time on
// PostgreSQL (SQLSTATE 22P02) — the same hazard documented on visibleTo. A
// caller invoking Create directly, bypassing the resource-layer guard, must
// not be able to reach that insert.
func (pr *Processor) Create(fleetID, name string, kind Kind) (Model, error) {
	if fleetID == "" {
		return Model{}, server.ErrValidation
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxCategoryNameLen {
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

	created, err := pr.a.Insert(Model{
		id:      uuid.NewString(),
		name:    name,
		kind:    kind,
		fleetID: &fleetID,
	})
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
