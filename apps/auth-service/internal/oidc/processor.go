// Package oidc implements the Google login/callback flow: it builds the OAuth2
// consent URL, exchanges the auth code, verifies the returned id_token against
// Google's keys, and maps the verified claims to a user.GoogleProfile
// (design §8.1).
package oidc

import (
	"context"
	"errors"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/idtoken"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/user"
)

// ErrNoIDToken is returned when Google's token response lacks an id_token.
var ErrNoIDToken = errors.New("oidc: token response missing id_token")

// Processor encapsulates the OAuth2 config and id_token verification.
type Processor struct {
	cfg      *oauth2.Config
	clientID string
}

// NewProcessor builds the Google OAuth2 config from credentials.
func NewProcessor(clientID, clientSecret, redirectURL string) *Processor {
	return &Processor{
		clientID: clientID,
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     google.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
		},
	}
}

// AuthCodeURL returns the Google consent URL carrying the given state and nonce.
func (p *Processor) AuthCodeURL(state, nonce string) string {
	return p.cfg.AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce))
}

// Exchange swaps an authorization code for a token and returns the raw id_token.
func (p *Processor) Exchange(ctx context.Context, code string) (string, error) {
	tok, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return "", err
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok || raw == "" {
		return "", ErrNoIDToken
	}
	return raw, nil
}

// Verify validates a raw id_token against Google's keys and our client id,
// returning the mapped profile.
func (p *Processor) Verify(ctx context.Context, rawIDToken string) (user.GoogleProfile, error) {
	payload, err := idtoken.Validate(ctx, rawIDToken, p.clientID)
	if err != nil {
		return user.GoogleProfile{}, err
	}
	return profileFromClaims(payload.Claims), nil
}

// profileFromClaims maps verified id_token claims to a user.GoogleProfile.
func profileFromClaims(claims map[string]any) user.GoogleProfile {
	return user.GoogleProfile{
		Sub:    str(claims["sub"]),
		Email:  str(claims["email"]),
		Name:   str(claims["name"]),
		Avatar: str(claims["picture"]),
	}
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
