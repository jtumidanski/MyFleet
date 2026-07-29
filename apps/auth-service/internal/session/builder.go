package session

import (
	"time"

	"github.com/google/uuid"
)

// Builder fluently constructs a refresh-token Model with a fresh id.
type Builder struct{ m Model }

// NewBuilder starts a builder with a generated token id.
func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetUserID(s string) *Builder       { b.m.userID = s; return b }
func (b *Builder) SetTokenHash(s string) *Builder    { b.m.tokenHash = s; return b }
func (b *Builder) SetFamilyID(s string) *Builder     { b.m.familyID = s; return b }
func (b *Builder) SetExpiresAt(t time.Time) *Builder { b.m.expiresAt = t; return b }

// Build returns the constructed Model. If no family id was set, the token
// starts its own family (id == family id) per the rotation design.
func (b *Builder) Build() Model {
	if b.m.familyID == "" {
		b.m.familyID = b.m.id
	}
	return b.m
}
