package session

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/apps/auth-service/internal/jwks"
)

const (
	accessTTL  = 15 * time.Minute
	refreshTTL = 30 * 24 * time.Hour
)

type Principal struct {
	UserID        string
	Email         string
	ActiveFleetID string
	Role          string
	// PlatformAdmin is orthogonal to Role: Role is a position inside one fleet,
	// this is a position above all of them. It is stamped at mint time from
	// auth.platform_admins, so revoking it does not take effect until the access
	// token expires or is refreshed — a staleness the console states in plain
	// words and the purge endpoints re-verify away (FR-ADMIN-AUTH-7).
	PlatformAdmin bool
}

// Issued bundles a freshly minted token pair returned to the client. The
// refresh token is the raw value; only its hash is persisted server-side.
type Issued struct {
	Access  string
	Refresh string
}

type Processor struct {
	log    logrus.FieldLogger
	ks     *jwks.KeySet
	issuer string
	aud    string
	p      Provider
	a      Administrator
}

// NewProcessor constructs the minting-only processor (no DB access). Used by
// tests and any caller that only needs MintAccess. Persistence-backed callers
// should attach a provider+administrator via WithStore.
func NewProcessor(log logrus.FieldLogger, ks *jwks.KeySet, issuer, aud string) *Processor {
	return &Processor{log: log, ks: ks, issuer: issuer, aud: aud}
}

// WithStore returns a copy of the processor wired with the read provider and
// write administrator, enabling refresh-token issuance and rotation.
func (p *Processor) WithStore(prov Provider, admin Administrator) *Processor {
	cp := *p
	cp.p = prov
	cp.a = admin
	return &cp
}

func (p *Processor) MintAccess(pr Principal) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"sub":             pr.UserID,
		"email":           pr.Email,
		"active_fleet_id": pr.ActiveFleetID,
		"role":            pr.Role,
		"platform_admin":  pr.PlatformAdmin,
		"iss":             p.issuer,
		"aud":             p.aud,
		"iat":             now.Unix(),
		"exp":             now.Add(accessTTL).Unix(),
	})
	tok.Header["kid"] = p.ks.Kid()
	return tok.SignedString(p.ks.Private())
}

// IssueRefresh mints a brand-new refresh token (new family) for a user,
// persisting only the hash. The raw token is returned to the caller.
func (p *Processor) IssueRefresh(userID string) (string, error) {
	raw := newRawToken()
	m := NewBuilder().
		SetUserID(userID).
		SetTokenHash(HashRefresh(raw)).
		SetExpiresAt(time.Now().Add(refreshTTL)).
		Build()
	if _, err := p.a.Insert(m); err != nil {
		return "", err
	}
	return raw, nil
}

// Rotate validates a presented raw refresh token and, if valid, consumes it and
// issues a new one in the same family. If the token was already consumed or
// revoked, the entire family is revoked (reuse detection) and ErrTokenReuse is
// returned. Returns the new raw refresh token and the owning user id.
func (p *Processor) Rotate(raw string) (newRaw string, userID string, err error) {
	now := time.Now()
	m, err := p.p.FindByHash(HashRefresh(raw))
	if err != nil {
		return "", "", err
	}
	if m.IsConsumed() || m.IsRevoked() {
		// Replay of a spent/revoked token: nuke the family.
		if rerr := p.a.RevokeFamily(m.FamilyID(), now); rerr != nil {
			p.log.WithError(rerr).Error("revoke family on reuse")
		}
		return "", "", ErrTokenReuse
	}
	if m.IsExpired(now) {
		return "", "", ErrTokenExpired
	}

	if err := p.a.Consume(m.ID(), now); err != nil {
		return "", "", err
	}
	newRaw = newRawToken()
	next := NewBuilder().
		SetUserID(m.UserID()).
		SetTokenHash(HashRefresh(newRaw)).
		SetFamilyID(m.FamilyID()).
		SetExpiresAt(now.Add(refreshTTL)).
		Build()
	if _, err := p.a.Insert(next); err != nil {
		return "", "", err
	}
	return newRaw, m.UserID(), nil
}

// Logout revokes the entire family of the presented refresh token. Unknown
// tokens are treated as success (idempotent logout).
func (p *Processor) Logout(raw string) error {
	m, err := p.p.FindByHash(HashRefresh(raw))
	if err != nil {
		if err == ErrNotFound {
			return nil
		}
		return err
	}
	return p.a.RevokeFamily(m.FamilyID(), time.Now())
}

// newRawToken returns an opaque, unguessable refresh token (uuid+uuid).
func newRawToken() string { return uuid.NewString() + uuid.NewString() }

// HashRefresh returns the deterministic sha256 hex digest of a raw refresh
// token. Only this digest is ever persisted.
func HashRefresh(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
