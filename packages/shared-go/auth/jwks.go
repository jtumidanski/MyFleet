package auth

import (
	"context"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// NewJWKSKeyfunc fetches+caches auth-service's JWKS (design A3).
func NewJWKSKeyfunc(ctx context.Context, jwksURL string) (jwt.Keyfunc, error) {
	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, err
	}
	return k.Keyfunc, nil
}
