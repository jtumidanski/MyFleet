package session

import (
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when no refresh token matches a hash.
	ErrNotFound = errors.New("refresh token not found")
	// ErrTokenReuse signals a consumed/revoked refresh token was replayed; the
	// caller must revoke the whole family and reject the request (design §8.1).
	ErrTokenReuse = errors.New("refresh token reuse detected")
	// ErrTokenExpired is returned when a refresh token is past its expiry.
	ErrTokenExpired = errors.New("refresh token expired")
)

// Model is the immutable representation of a stored (hashed) refresh token.
// The raw token is never persisted; only TokenHash is stored.
type Model struct {
	id         string
	userID     string
	tokenHash  string
	familyID   string
	expiresAt  time.Time
	revokedAt  *time.Time
	consumedAt *time.Time
}

func (m Model) ID() string            { return m.id }
func (m Model) UserID() string        { return m.userID }
func (m Model) TokenHash() string     { return m.tokenHash }
func (m Model) FamilyID() string      { return m.familyID }
func (m Model) ExpiresAt() time.Time  { return m.expiresAt }
func (m Model) RevokedAt() *time.Time { return m.revokedAt }
func (m Model) ConsumedAt() *time.Time {
	return m.consumedAt
}

// IsRevoked reports whether the token has been revoked.
func (m Model) IsRevoked() bool { return m.revokedAt != nil }

// IsConsumed reports whether the token has already been used (rotated).
func (m Model) IsConsumed() bool { return m.consumedAt != nil }

// IsExpired reports whether the token is past its expiry relative to now.
func (m Model) IsExpired(now time.Time) bool { return now.After(m.expiresAt) }

// WithConsumed returns a copy marked consumed at the given time.
func (m Model) WithConsumed(at time.Time) Model {
	m.consumedAt = &at
	return m
}
