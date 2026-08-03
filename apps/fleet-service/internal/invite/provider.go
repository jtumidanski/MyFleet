package invite

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/database"
)

var ErrNotFound = errors.New("invite not found")

// Provider is the read-only interface for invite data access.
//
// Every method takes the caller's context as its first argument and runs its
// query on db.WithContext(ctx). The *gorm.DB the provider holds is captured once
// at startup; without WithContext every query would run on that bare connection,
// so a client that disconnected mid-request would leave the query running, and
// no query would carry a deadline or join the request's trace.
type Provider interface {
	GetByID(ctx context.Context, id string) (Model, error)
	GetByToken(ctx context.Context, token string) (Model, error)
	ListByFleetID(ctx context.Context, fleetID string) ([]Model, error)
	ListRedeemableByEmail(ctx context.Context, email string, now time.Time) ([]Model, error)
	// CountByFleetSince backs the per-fleet creation rate limit. It is a query,
	// not an in-process counter, because fleet-service runs multiple replicas
	// and a per-pod limiter enforces nothing (FR-RATE-1).
	CountByFleetSince(ctx context.Context, fleetID string, since time.Time) (int64, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

// Every read below filters deleted_at. main's refactor to context-aware
// database.Query wrappers did not carry that predicate, and without it an
// invite soft-deleted by an admin purge stays visible and redeemable for the
// whole recovery window — a purged fleet would still hand out memberships
// (FR-ADMIN-DATA-1/2).
func (p *dbProvider) GetByID(ctx context.Context, id string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := p.db.WithContext(ctx).First(&e, "id = ? AND deleted_at IS NULL", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}

func (p *dbProvider) GetByToken(ctx context.Context, token string) (Model, error) {
	return database.Query(func() (Model, error) {
		var e Entity
		if err := p.db.WithContext(ctx).First(&e, "token = ? AND deleted_at IS NULL", token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return Model{}, ErrNotFound
			}
			return Model{}, err
		}
		return Make(e), nil
	})()
}

// ListRedeemableByEmail returns the invites addressed to email that an accept
// call would actually honour: not yet accepted, not yet expired, not purged.
//
// The comparison is LOWER()-folded to match Processor.ValidateAccept, which
// uses strings.EqualFold. A case-sensitive lookup here would hide an invite the
// accept route would gladly redeem, telling the invitee nothing is waiting.
//
// A blank email is rejected by the caller (Processor.ListRedeemableForEmail),
// not here, so this stays a plain query.
func (p *dbProvider) ListRedeemableByEmail(ctx context.Context, email string, now time.Time) ([]Model, error) {
	return database.SliceQuery(func() ([]Model, error) {
		var es []Entity
		err := p.db.WithContext(ctx).
			Where("LOWER(email) = LOWER(?)", email).
			Where("accepted_at IS NULL").
			Where("expires_at > ?", now).
			Where("deleted_at IS NULL").
			Find(&es).Error
		if err != nil {
			return nil, err
		}
		return makeAll(es), nil
	})()
}

func (p *dbProvider) ListByFleetID(ctx context.Context, fleetID string) ([]Model, error) {
	return database.SliceQuery(func() ([]Model, error) {
		var es []Entity
		if err := p.db.WithContext(ctx).
			Where("fleet_id = ? AND deleted_at IS NULL", fleetID).Find(&es).Error; err != nil {
			return nil, err
		}
		return makeAll(es), nil
	})()
}

// CountByFleetSince backs the invite rate limit. It excludes purged rows too:
// counting invites an admin purge removed would rate-limit an operator who has
// just restored a fleet, for invites that no longer exist.
func (p *dbProvider) CountByFleetSince(ctx context.Context, fleetID string, since time.Time) (int64, error) {
	return database.Query(func() (int64, error) {
		var n int64
		err := p.db.WithContext(ctx).Model(&Entity{}).
			Where("fleet_id = ? AND created_at > ? AND deleted_at IS NULL", fleetID, since).
			Count(&n).Error
		return n, err
	})()
}

// makeAll converts a slice of rows to Models, always returning a non-nil slice
// so a caller marshalling the result renders [] rather than null.
func makeAll(es []Entity) []Model {
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out
}
