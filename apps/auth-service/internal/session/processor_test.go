package session

import (
	"crypto/rand"
	"crypto/rsa"
	"reflect"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
)

// fakeStore implements both Provider and Administrator over in-memory maps so
// tests assert real outcomes (token consumed, family shared, family revoked)
// rather than mock call counts. It mirrors the user domain's fake-based tests.
type fakeStore struct {
	byHash map[string]Model // hash -> stored token
	byID   map[string]Model // id   -> stored token

	revokedFamilies []string // family ids passed to RevokeFamily, in order
	revokeErr       error    // when non-nil, RevokeFamily fails with it
}

func newFakeStore() *fakeStore {
	return &fakeStore{byHash: map[string]Model{}, byID: map[string]Model{}}
}

// seed inserts a token directly (bypassing the processor) for test setup.
func (f *fakeStore) seed(m Model) {
	f.byHash[m.TokenHash()] = m
	f.byID[m.ID()] = m
}

func (f *fakeStore) FindByHash(hash string) (Model, error) {
	if m, ok := f.byHash[hash]; ok {
		return m, nil
	}
	return Model{}, ErrNotFound
}

func (f *fakeStore) Insert(m Model) (Model, error) {
	f.seed(m)
	return m, nil
}

func (f *fakeStore) Consume(id string, at time.Time) error {
	m, ok := f.byID[id]
	if !ok {
		return ErrNotFound
	}
	consumed := m.WithConsumed(at)
	f.byID[id] = consumed
	f.byHash[consumed.TokenHash()] = consumed
	return nil
}

func (f *fakeStore) RevokeFamily(familyID string, at time.Time) error {
	// Recorded BEFORE the injected failure so revokedFamilies remains a call
	// log rather than a success log: a test asserting "no family was revoked"
	// must be able to distinguish a call that failed from a call that never
	// happened.
	f.revokedFamilies = append(f.revokedFamilies, familyID)
	if f.revokeErr != nil {
		return f.revokeErr
	}
	for id, m := range f.byID {
		if m.FamilyID() == familyID && !m.IsRevoked() {
			m.revokedAt = &at
			f.byID[id] = m
			f.byHash[m.TokenHash()] = m
		}
	}
	return nil
}

func newTestProcessor(store *fakeStore) *Processor {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := jwks.NewKeySet(priv, "kid-1")
	return NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet").WithStore(store, store)
}

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

// TestRotate_happyPath asserts that rotating a valid token consumes the old
// token, inserts a new token in the SAME family, and returns a fresh raw token
// plus a real (parseable, distinct) access token.
func TestRotate_happyPath(t *testing.T) {
	store := newFakeStore()
	p := newTestProcessor(store)

	oldRaw := "old-refresh-token"
	old := NewBuilder().
		SetUserID("u1").
		SetTokenHash(HashRefresh(oldRaw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build()
	store.seed(old)

	newRaw, userID, err := p.Rotate(oldRaw)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if userID != "u1" {
		t.Fatalf("userID = %q, want u1", userID)
	}
	if newRaw == "" || newRaw == oldRaw {
		t.Fatalf("expected a fresh raw refresh token, got %q (old %q)", newRaw, oldRaw)
	}

	// Old token must now be consumed.
	stored, err := store.FindByHash(HashRefresh(oldRaw))
	if err != nil {
		t.Fatalf("find old: %v", err)
	}
	if !stored.IsConsumed() {
		t.Fatal("old token was not consumed after rotation")
	}

	// New token must exist and carry the SAME family id as the old token.
	next, err := store.FindByHash(HashRefresh(newRaw))
	if err != nil {
		t.Fatalf("find new: %v", err)
	}
	if next.FamilyID() != old.FamilyID() {
		t.Fatalf("new token family = %q, want %q (same family)", next.FamilyID(), old.FamilyID())
	}
	if next.IsConsumed() || next.IsRevoked() {
		t.Fatal("new token must be fresh (not consumed/revoked)")
	}
	if next.UserID() != "u1" {
		t.Fatalf("new token user = %q, want u1", next.UserID())
	}

	// A fresh access token must be mintable and parseable for the rotated user.
	access, err := p.MintAccess(Principal{UserID: userID})
	if err != nil {
		t.Fatalf("mint access: %v", err)
	}
	claims := jwt.MapClaims{}
	if _, perr := jwt.ParseWithClaims(access, claims, func(*jwt.Token) (any, error) {
		return p.ks.Private().Public(), nil
	}); perr != nil {
		t.Fatalf("parse minted access: %v", perr)
	}
	if claims["sub"] != "u1" {
		t.Fatalf("access sub = %v, want u1", claims["sub"])
	}
}

// TestRotate_reuseDetection asserts that replaying an already-consumed token
// revokes the whole family and returns ErrTokenReuse (the 401 the handler maps).
func TestRotate_reuseDetection(t *testing.T) {
	store := newFakeStore()
	p := newTestProcessor(store)

	raw := "spent-refresh-token"
	consumedAt := time.Now().Add(-time.Minute)
	spent := NewBuilder().
		SetUserID("u2").
		SetTokenHash(HashRefresh(raw)).
		SetFamilyID("fam-2").
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build().
		WithConsumed(consumedAt)
	store.seed(spent)

	_, _, err := p.Rotate(raw)
	if err != ErrTokenReuse {
		t.Fatalf("err = %v, want ErrTokenReuse", err)
	}
	if len(store.revokedFamilies) != 1 || store.revokedFamilies[0] != "fam-2" {
		t.Fatalf("RevokeFamily calls = %v, want [fam-2]", store.revokedFamilies)
	}
	// The replayed token's family must actually be revoked in the store.
	after, ferr := store.FindByHash(HashRefresh(raw))
	if ferr != nil {
		t.Fatalf("find: %v", ferr)
	}
	if !after.IsRevoked() {
		t.Fatal("family token should be revoked after reuse detection")
	}
}

// TestLogout_revokesFamily asserts logout revokes the presented token's family,
// and that an unknown token is treated as success (idempotent).
func TestLogout_revokesFamily(t *testing.T) {
	store := newFakeStore()
	p := newTestProcessor(store)

	raw := "active-refresh-token"
	active := NewBuilder().
		SetUserID("u3").
		SetTokenHash(HashRefresh(raw)).
		SetFamilyID("fam-3").
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build()
	store.seed(active)

	if err := p.Logout(raw); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if len(store.revokedFamilies) != 1 || store.revokedFamilies[0] != "fam-3" {
		t.Fatalf("RevokeFamily calls = %v, want [fam-3]", store.revokedFamilies)
	}
	revoked, err := store.FindByHash(HashRefresh(raw))
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !revoked.IsRevoked() {
		t.Fatal("token family should be revoked after logout")
	}

	// Unknown token: idempotent success, no extra revoke.
	if err := p.Logout("never-issued-token"); err != nil {
		t.Fatalf("logout unknown token should be nil, got %v", err)
	}
	if len(store.revokedFamilies) != 1 {
		t.Fatalf("unknown-token logout must not revoke any family; calls = %v", store.revokedFamilies)
	}
}

// TestMintAccess_mapsEveryPrincipalField fails when a field is added to
// Principal but not wired into MintAccess's claim map. It deliberately names no
// field: TestMintAccess_setsRequiredClaims above enumerates the four we have
// today, and an enumerating test is one somebody has to remember to update.
//
// The scenario: someone adds Principal.TenantID, wires it in
// newPrincipalResolver, and forgets MintAccess. Every other test stays green
// and the claim is silently absent — the same shape as the defect this task
// fixed.
func TestMintAccess_mapsEveryPrincipalField(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := jwks.NewKeySet(priv, "kid-1")
	p := NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet")

	v := reflect.New(reflect.TypeOf(Principal{})).Elem()
	wantStrings := map[string]string{}
	var boolFields []string
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		switch f.Type.Kind() {
		case reflect.String:
			sentinel := "sentinel-" + f.Name
			v.Field(i).SetString(sentinel)
			wantStrings[f.Name] = sentinel
		case reflect.Bool:
			// A bool carries only two values, so it cannot hold a per-field
			// sentinel. Exactly ONE bool field is still attributable — set it
			// true and require a true-valued claim. A second would be
			// ambiguous, so fail loudly and make whoever adds it extend this.
			boolFields = append(boolFields, f.Name)
			if len(boolFields) > 1 {
				t.Fatalf("Principal now has %d bool fields (%v) — a true-valued claim can no "+
					"longer be attributed to one of them; extend this test's sentinel scheme",
					len(boolFields), boolFields)
			}
			v.Field(i).SetBool(true)
		default:
			t.Fatalf("Principal.%s is a %s — extend this test's sentinel scheme", f.Name, f.Type.Kind())
		}
	}

	tokenStr, err := p.MintAccess(v.Interface().(Principal))
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	if _, perr := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
		return &priv.PublicKey, nil
	}); perr != nil {
		t.Fatal(perr)
	}

	gotStrings := map[string]bool{}
	gotTrue := 0
	for _, cv := range claims {
		switch typed := cv.(type) {
		case string:
			gotStrings[typed] = true
		case bool:
			if typed {
				gotTrue++
			}
		}
	}
	for field, sentinel := range wantStrings {
		if !gotStrings[sentinel] {
			t.Errorf("Principal.%s never reaches a claim — MintAccess drops it. "+
				"Every Principal field must appear in MintAccess's claim map.", field)
		}
	}
	if len(boolFields) == 1 && gotTrue == 0 {
		t.Errorf("Principal.%s never reaches a claim — MintAccess drops it. "+
			"Every Principal field must appear in MintAccess's claim map.", boolFields[0])
	}
}

// FR-ADMIN-AUTH-4: the claim is emitted on every mint, and it is a JSON boolean
// rather than a string — the shared middleware parses it with a boolean
// accessor and would read "true" as false.
func TestMintAccess_setsPlatformAdminClaimAsBoolean(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	ks := jwks.NewKeySet(priv, "kid-1")
	p := NewProcessor(logrus.New(), ks, "myfleet-auth", "myfleet")

	for _, want := range []bool{true, false} {
		tokenStr, err := p.MintAccess(Principal{UserID: "u1", Email: "e@x", PlatformAdmin: want})
		if err != nil {
			t.Fatal(err)
		}
		claims := jwt.MapClaims{}
		if _, perr := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
			return &priv.PublicKey, nil
		}); perr != nil {
			t.Fatal(perr)
		}
		got, ok := claims["platform_admin"].(bool)
		if !ok {
			t.Fatalf("platform_admin must be a JSON boolean, got %T (%v)",
				claims["platform_admin"], claims["platform_admin"])
		}
		if got != want {
			t.Errorf("platform_admin = %v, want %v", got, want)
		}
	}
}
