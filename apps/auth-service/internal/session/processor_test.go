package session

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
)

func TestMintAccess_setsRequiredClaims(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := jwks.NewKeySet(priv, "kid-1")
	p := NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet")
	tokenStr, err := p.MintAccess(Principal{UserID: "u1", Email: "a@b.com", ActiveFleetID: "f1", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) { return &priv.PublicKey, nil })
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"sub": "u1", "email": "a@b.com", "active_fleet_id": "f1", "role": "owner", "iss": "myfleet-auth"} {
		if claims[k] != want {
			t.Fatalf("claim %s = %v, want %s", k, claims[k], want)
		}
	}
}

func TestHashRefresh_isStableAndNotPlaintext(t *testing.T) {
	h := HashRefresh("secret-token")
	if h == "secret-token" || h != HashRefresh("secret-token") {
		t.Fatal("refresh token must be hashed deterministically and never stored plaintext")
	}
}
