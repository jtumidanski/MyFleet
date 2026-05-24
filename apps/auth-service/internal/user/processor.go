package user

import (
	"errors"
	"time"

	"github.com/sirupsen/logrus"
)

type Processor struct {
	log logrus.FieldLogger
	p   Provider
}

func NewProcessor(log logrus.FieldLogger, p Provider) *Processor { return &Processor{log: log, p: p} }

// ProvisionFromGoogle upserts a user by google_sub (FR-AUTH-2). Idempotent.
func (pr *Processor) ProvisionFromGoogle(w Writer, gp GoogleProfile) (Model, error) {
	existing, err := pr.p.GetBySub(gp.Sub)
	if errors.Is(err, ErrNotFound) {
		m := NewBuilder().SetGoogleSub(gp.Sub).SetEmail(gp.Email).SetDisplayName(gp.Name).SetAvatarURL(gp.Avatar).Build()
		m = m.WithLogin(gp.Name, gp.Avatar, time.Now())
		return w.Insert(m)
	}
	if err != nil {
		return Model{}, err
	}
	return w.Update(existing.WithLogin(gp.Name, gp.Avatar, time.Now()))
}
