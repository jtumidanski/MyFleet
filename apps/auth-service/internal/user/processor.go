package user

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"
)

type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// GetByID fetches a user by our internal user id — the value the JWT `sub`
// claim carries. This is what GET /auth/me needs; GetBySub is not a substitute.
func (pr *Processor) GetByID(id string) (Model, error) {
	return pr.p.GetByID(id)
}

// GetBySub fetches a user by Google sub — delegates to the provider. Only the
// OAuth callback path has a Google sub; everything downstream of token issue
// holds a user id and wants GetByID.
func (pr *Processor) GetBySub(sub string) (Model, error) {
	return pr.p.GetBySub(sub)
}

// ProvisionFromGoogle upserts a user by google_sub (FR-AUTH-2). Idempotent.
func (pr *Processor) ProvisionFromGoogle(gp GoogleProfile) (Model, error) {
	existing, err := pr.p.GetBySub(gp.Sub)
	if errors.Is(err, ErrNotFound) {
		m := NewBuilder().SetGoogleSub(gp.Sub).SetEmail(gp.Email).SetDisplayName(gp.Name).SetAvatarURL(gp.Avatar).Build()
		m = m.WithLogin(gp.Name, gp.Avatar, time.Now())
		return pr.a.Insert(m)
	}
	if err != nil {
		return Model{}, err
	}
	return pr.a.Update(existing.WithLogin(gp.Name, gp.Avatar, time.Now()))
}
