package invite

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("invite not found")

// Provider is the read-only interface for invite data access.
type Provider interface {
	GetByID(id string) (Model, error)
	GetByToken(token string) (Model, error)
	ListByFleetID(fleetID string) ([]Model, error)
	ListRedeemableByEmail(email string, now time.Time) ([]Model, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) GetByID(id string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

func (p *dbProvider) GetByToken(token string) (Model, error) {
	var e Entity
	if err := p.db.First(&e, "token = ?", token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Model{}, ErrNotFound
		}
		return Model{}, err
	}
	return Make(e), nil
}

// ListRedeemableByEmail returns the invites addressed to email that an accept
// call would actually honour: not yet accepted, not yet expired.
//
// The comparison is LOWER()-folded to match Processor.ValidateAccept, which
// uses strings.EqualFold. A case-sensitive lookup here would hide an invite the
// accept route would gladly redeem, telling the invitee nothing is waiting.
//
// A blank email is rejected by the caller (Processor.ListRedeemableForEmail),
// not here, so this stays a plain query.
func (p *dbProvider) ListRedeemableByEmail(email string, now time.Time) ([]Model, error) {
	var es []Entity
	err := p.db.
		Where("LOWER(email) = LOWER(?)", email).
		Where("accepted_at IS NULL").
		Where("expires_at > ?", now).
		Find(&es).Error
	if err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}

func (p *dbProvider) ListByFleetID(fleetID string) ([]Model, error) {
	var es []Entity
	if err := p.db.Where("fleet_id = ?", fleetID).Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}
