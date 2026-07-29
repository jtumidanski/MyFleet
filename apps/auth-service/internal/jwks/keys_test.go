package jwks

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

func TestJWKS_exposesPublicKey(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := NewKeySet(priv, "kid-1")
	doc := ks.PublicJWKS()
	if len(doc.Keys) != 1 {
		t.Fatalf("want 1 key, got %d", len(doc.Keys))
	}
	if doc.Keys[0].Kid != "kid-1" || doc.Keys[0].Kty != "RSA" || doc.Keys[0].Use != "sig" {
		t.Fatalf("unexpected jwk: %+v", doc.Keys[0])
	}
}
