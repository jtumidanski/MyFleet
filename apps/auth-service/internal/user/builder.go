package user

import "github.com/google/uuid"

type Builder struct{ m Model }

func NewBuilder() *Builder { return &Builder{m: Model{id: uuid.NewString()}} }

func (b *Builder) SetGoogleSub(s string) *Builder  { b.m.googleSub = s; return b }
func (b *Builder) SetEmail(e string) *Builder       { b.m.email = e; return b }
func (b *Builder) SetDisplayName(n string) *Builder { b.m.displayName = n; return b }
func (b *Builder) SetAvatarURL(a string) *Builder   { b.m.avatarURL = a; return b }
func (b *Builder) Build() Model                     { return b.m }
