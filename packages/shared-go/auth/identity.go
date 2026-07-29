package auth

import "context"

type Identity struct {
	UserID        string
	Email         string
	ActiveFleetID string
	Role          string // owner | member | viewer
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
