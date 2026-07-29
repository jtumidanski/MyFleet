// Package jwks holds the RS256 signing key and serves the public JWKS (design A3).
package jwks

import (
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"math/big"

	"github.com/golang-jwt/jwt/v5"
)

type JWK struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type JWKSDocument struct {
	Keys []JWK `json:"keys"`
}

type KeySet struct {
	priv *rsa.PrivateKey
	kid  string
}

func NewKeySet(priv *rsa.PrivateKey, kid string) *KeySet { return &KeySet{priv: priv, kid: kid} }

func (k *KeySet) Private() *rsa.PrivateKey { return k.priv }
func (k *KeySet) Kid() string              { return k.kid }

// Keyfunc returns a jwt.Keyfunc that validates tokens against this key set's
// in-memory RSA public key, matching on the token's "kid" header. This lets
// auth-service validate its OWN tokens without HTTP-fetching its own JWKS at
// startup (Decision 2 — avoids a self-fetch deadlock before the server listens).
func (k *KeySet) Keyfunc() jwt.Keyfunc {
	pub := k.priv.Public()
	return func(t *jwt.Token) (any, error) {
		if kid, _ := t.Header["kid"].(string); kid != k.kid {
			return nil, errors.New("jwks: unknown key id")
		}
		return pub, nil
	}
}

func (k *KeySet) PublicJWKS() JWKSDocument {
	pub := k.priv.Public().(*rsa.PublicKey)
	return JWKSDocument{Keys: []JWK{{
		Kty: "RSA", Use: "sig", Kid: k.kid, Alg: "RS256",
		N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}
