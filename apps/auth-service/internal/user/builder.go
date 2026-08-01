package user

import "github.com/google/uuid"

type Builder struct{ m Model }

// NewBuilder seeds themePreference explicitly rather than leaning on the
// Postgres column default. ProvisionFromGoogle inserts through this builder,
// and whether GORM omits a zero-valued column carrying a `default:` tag —
// then reads the value back via RETURNING — is version- and dialect-dependent
// (design §3.4).
func NewBuilder() *Builder {
	return &Builder{m: Model{id: uuid.NewString(), themePreference: ThemeSystem}}
}

func (b *Builder) SetGoogleSub(s string) *Builder       { b.m.googleSub = s; return b }
func (b *Builder) SetEmail(e string) *Builder           { b.m.email = e; return b }
func (b *Builder) SetDisplayName(n string) *Builder     { b.m.displayName = n; return b }
func (b *Builder) SetAvatarURL(a string) *Builder       { b.m.avatarURL = a; return b }
func (b *Builder) SetThemePreference(p string) *Builder { b.m.themePreference = p; return b }
func (b *Builder) Build() Model                         { return b.m }
