package maintenancecategory

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Kind discriminates repair/service work from modifications. It lives on the
// category rather than on the record so a record's kind cannot disagree with
// its category's — the category a record points at is the single source of
// truth, and a record stores no kind at all (design D1).
type Kind string

const (
	KindMaintenance  Kind = "maintenance"
	KindModification Kind = "modification"
)

// ParseKind maps a ?kind= query-parameter value to a Kind. The empty string
// means "no filter" and yields ("", nil); anything else unrecognised is a
// validation error, never a silent empty result (PRD FR-KIND-4).
func ParseKind(s string) (Kind, error) {
	switch Kind(s) {
	case "":
		return "", nil
	case KindMaintenance:
		return KindMaintenance, nil
	case KindModification:
		return KindModification, nil
	default:
		return "", server.ErrValidation
	}
}

// Model is the immutable maintenance category domain model. Categories are
// global/system data (not fleet-scoped); see design §8.2.
type Model struct {
	id            string
	name          string
	description   string
	systemDefined bool
	kind          Kind
}

func (m Model) ID() string          { return m.id }
func (m Model) Name() string        { return m.name }
func (m Model) Description() string { return m.description }
func (m Model) SystemDefined() bool { return m.systemDefined }
func (m Model) Kind() Kind          { return m.kind }
