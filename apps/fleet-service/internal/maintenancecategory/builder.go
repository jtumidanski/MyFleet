package maintenancecategory

import (
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// maxCategoryNameLen bounds a free-form category name, counted in RUNES (not
// bytes): a 60-character Cyrillic or CJK name is 120-180 bytes, and rejecting
// it on byte length would punish non-Latin scripts for a limit stated in
// characters. It is a UI-legibility limit, not a storage one — the column is
// unbounded text.
const maxCategoryNameLen = 60

// Builder constructs a valid maintenance category Model. Build() returns
// (Model, error) because name, kind, and fleetID are invariants enforced at
// construction time (design §8.2).
//
// Only the fleet-scoped create path builds models; system rows are seeded by
// migration and read back through Make, which bypasses validation by design.
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString()}}
}

func (b *Builder) SetKind(kind Kind) *Builder         { b.m.kind = kind; return b }
func (b *Builder) SetDescription(d string) *Builder   { b.m.description = d; return b }
func (b *Builder) SetSystemDefined(sd bool) *Builder  { b.m.systemDefined = sd; return b }
func (b *Builder) SetFleetID(fleetID string) *Builder { b.m.fleetID = &fleetID; return b }

// SetName sets the display name, trimmed of surrounding whitespace. Trimming
// happens here rather than at the caller so the name that gets validated for
// length is the same one that gets stored.
func (b *Builder) SetName(name string) *Builder {
	b.m.name = strings.TrimSpace(name)
	return b
}

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if err := Validate(b.m); err != nil {
		return Model{}, err
	}
	return b.m, nil
}

// Validate enforces the maintenance category invariants.
//
// The fleetID check is not merely cosmetic: FindByName("", ...) silently
// matches nothing (visibleTo's empty-fleetID branch), but an insert with an
// empty FleetID binds "" into a uuid column and fails at bind time on
// PostgreSQL (SQLSTATE 22P02) — the same hazard documented on visibleTo. A
// caller that bypasses the resource-layer guard must not be able to reach that
// insert, so the model refuses to be built.
func Validate(m Model) error {
	if m.fleetID == nil || *m.fleetID == "" {
		return server.ErrValidation
	}
	if m.name == "" || utf8.RuneCountInString(m.name) > maxCategoryNameLen {
		return server.ErrValidation
	}
	if m.kind != KindMaintenance && m.kind != KindModification {
		return server.ErrValidation
	}
	return nil
}
