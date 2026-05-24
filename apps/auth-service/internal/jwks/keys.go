// Package jwks holds the RS256 signing key and serves the public JWKS (design A3).
package jwks

import (
	"crypto/rsa"
	"encoding/base64"
	"math/big"
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

func (k *KeySet) PublicJWKS() JWKSDocument {
	pub := k.priv.Public().(*rsa.PublicKey)
	return JWKSDocument{Keys: []JWK{{
		Kty: "RSA", Use: "sig", Kid: k.kid, Alg: "RS256",
		N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
}
