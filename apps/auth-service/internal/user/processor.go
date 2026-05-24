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

// GetBySub fetches a user by Google sub — delegates to the provider.
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
