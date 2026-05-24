package fleet

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Processor contains fleet business logic, injected with Provider and Administrator.
type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// GetByID fetches a fleet by ID.
func (pr *Processor) GetByID(id string) (Model, error) {
	m, err := pr.p.GetByID(id)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}

// Rename updates a fleet's name.
func (pr *Processor) Rename(id, name string) (Model, error) {
	m, err := pr.p.GetByID(id)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	if err != nil {
		return Model{}, err
	}
	return pr.a.Update(m.WithName(name))
}
