package user

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("user not found")

// ErrInvalidTheme is returned by Processor.UpdateTheme for a value outside the
// allow-list. It is deliberately distinguishable from ErrNotFound so the
// transport layer can render 422 and 404 apart.
var ErrInvalidTheme = errors.New("invalid theme preference")

// Model is immutable; state changes return new instances (design §6).
type Model struct {
	id              string
	googleSub       string
	email           string
	displayName     string
	avatarURL       string
	themePreference string
	lastLoginAt     time.Time
}

func (m Model) ID() string              { return m.id }
func (m Model) GoogleSub() string       { return m.googleSub }
func (m Model) Email() string           { return m.email }
func (m Model) DisplayName() string     { return m.displayName }
func (m Model) AvatarURL() string       { return m.avatarURL }
func (m Model) ThemePreference() string { return m.themePreference }

// WithLogin returns a copy with login metadata refreshed.
func (m Model) WithLogin(name, avatar string, at time.Time) Model {
	m.displayName, m.avatarURL, m.lastLoginAt = name, avatar, at
	return m
}

// WithThemePreference returns a copy carrying the new preference. It does NOT
// validate: validation is the processor's job (PRD §6.2), so an out-of-range
// value produces a typed domain error at the call site rather than a mutation
// that silently does nothing.
func (m Model) WithThemePreference(pref string) Model {
	m.themePreference = pref
	return m
}

type GoogleProfile struct {
	Sub    string
	Email  string
	Name   string
	Avatar string
}
