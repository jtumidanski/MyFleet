package membership

import "github.com/google/uuid"

// validRoles is the allowed set of membership roles.
var validRoles = map[string]bool{"owner": true, "member": true, "viewer": true}

// Builder constructs a Membership model. Build() returns Model (no error)
// because callers always supply validated role values from constants; the
// role validation is enforced in the administrator layer before calling Build.
type Builder struct{ m Model }

func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), status: "active"}}
}

func (b *Builder) SetFleetID(id string) *Builder  { b.m.fleetID = id; return b }
func (b *Builder) SetUserID(id string) *Builder   { b.m.userID = id; return b }
func (b *Builder) SetRole(role string) *Builder   { b.m.role = role; return b }
func (b *Builder) SetStatus(s string) *Builder    { b.m.status = s; return b }

// Build returns the Model. Callers supply valid roles; no runtime error needed
// here since all call-sites are internal and use role constants.
func (b *Builder) Build() Model { return b.m }
