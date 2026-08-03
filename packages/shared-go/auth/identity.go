package auth

import "context"

type Identity struct {
	UserID        string
	Email         string
	ActiveFleetID string
	Role          string // owner | member | viewer
	// PlatformAdmin is orthogonal to Role and to ActiveFleetID: an admin with no
	// fleet is a normal state, including immediately after a system purge
	// (FR-ADMIN-AUTH-9).
	PlatformAdmin bool
}

type idCtxKey int

const identityKey idCtxKey = iota

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

func IdentityFromContext(ctx context.Context) Identity {
	if v, ok := ctx.Value(identityKey).(Identity); ok {
		return v
	}
	return Identity{}
}
