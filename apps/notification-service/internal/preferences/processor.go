package preferences

import (
	"errors"

	"github.com/sirupsen/logrus"
)

// Processor contains preference business logic, injected with a Provider (reads)
// and Administrator (writes). It satisfies notification.Prefs via Enabled.
type Processor struct {
	log  logrus.FieldLogger
	prov Provider
	adm  Administrator
}

// NewProcessor constructs a Processor with its collaborators injected.
func NewProcessor(log logrus.FieldLogger, prov Provider, adm Administrator) *Processor {
	return &Processor{log: log, prov: prov, adm: adm}
}

// Enabled reports whether the user has the given notification type enabled.
// A missing row defaults to enabled (true) so users get notifications before
// customizing their preferences (design §8.4).
func (pr *Processor) Enabled(userID, typ string) (bool, error) {
	m, err := pr.prov.GetByUserAndType(userID, typ)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return true, nil // default-on
		}
		return false, err
	}
	return m.InAppEnabled(), nil
}

// List returns all of a user's stored preference rows.
func (pr *Processor) List(userID string) ([]Model, error) {
	return pr.prov.ListByUser(userID)
}

// Set upserts the in-app toggle for a (user, type) pair.
func (pr *Processor) Set(userID, typ string, enabled bool) (Model, error) {
	return pr.adm.Upsert(userID, typ, enabled)
}
