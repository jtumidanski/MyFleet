package user

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("user not found")

// Model is immutable; state changes return new instances (design §6).
type Model struct {
	id          string
	googleSub   string
	email       string
	displayName string
	avatarURL   string
	lastLoginAt time.Time
}

func (m Model) ID() string          { return m.id }
func (m Model) GoogleSub() string   { return m.googleSub }
func (m Model) Email() string       { return m.email }
func (m Model) DisplayName() string { return m.displayName }
func (m Model) AvatarURL() string   { return m.avatarURL }

// WithLogin returns a copy with login metadata refreshed.
func (m Model) WithLogin(name, avatar string, at time.Time) Model {
	m.displayName, m.avatarURL, m.lastLoginAt = name, avatar, at
	return m
}

type GoogleProfile struct {
	Sub    string
	Email  string
	Name   string
	Avatar string
}
