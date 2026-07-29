package notification

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Store is the write/dedupe surface the Generate flow needs: a read-only dedupe
// check by key plus an insert. It is intentionally narrow so the generation
// logic is unit-testable with a fake (see processor_test.go). The concrete
// implementation is the Administrator.
type Store interface {
	// ExistsByDedupeKey reports whether a notification with the given dedupe_key
	// already exists. The key is unique, so this is the idempotency guard.
	ExistsByDedupeKey(k string) (bool, error)
	// Insert persists a new notification.
	Insert(n Model) error
}

// Prefs is the per-user/per-type preference surface. Enabled defaults to true
// when the user has no row yet (so users get notifications before customizing).
type Prefs interface {
	Enabled(userID, typ string) (bool, error)
}

// GenerateInput is the request to generate one notification for one user
// (design §8.4, A6). DedupeKey MUST already include the user id so each
// recipient gets their own deduplicated copy.
type GenerateInput struct {
	UserID    string
	Type      string
	DedupeKey string
	Title     string
	Body      string
	VehicleID string
	FleetID   string
}

// Processor contains notification generation + read business logic, injected
// with a Store (write/dedupe) and Prefs (preference checks). The read/mark
// surfaces (Provider, Administrator) are wired separately via WithReads so the
// generation path stays unit-testable with narrow fakes (processor_test.go).
type Processor struct {
	log   logrus.FieldLogger
	store Store
	prefs Prefs
	prov  Provider
	adm   Administrator
}

// NewProcessor constructs a Processor with its collaborators injected.
func NewProcessor(log logrus.FieldLogger, store Store, prefs Prefs) *Processor {
	return &Processor{log: log, store: store, prefs: prefs}
}

// WithReads injects the read provider + write administrator the REST layer needs
// for listing and marking notifications read. Returns the processor for chaining.
func (pr *Processor) WithReads(prov Provider, adm Administrator) *Processor {
	pr.prov = prov
	pr.adm = adm
	return pr
}

// Generate creates one in-app notification idempotently (design A6, FR-NOTIF-2/3):
//
//  1. Preference check: if the user has disabled this type, return nil (no insert).
//  2. Dedupe check: if a notification with this dedupe_key already exists, no-op.
//  3. Otherwise build and insert the notification.
//
// The dedupe_key is the single source of idempotency and is identical whether
// the notification originates from the event-path consumer or the daily reminder
// safety-net, so the two paths cannot double-fire for the same trigger+user.
func (pr *Processor) Generate(in GenerateInput) error {
	enabled, err := pr.prefs.Enabled(in.UserID, in.Type)
	if err != nil {
		return err
	}
	if !enabled {
		return nil // suppressed by user preference
	}

	exists, err := pr.store.ExistsByDedupeKey(in.DedupeKey)
	if err != nil {
		return err
	}
	if exists {
		return nil // already generated (redelivery or reminder safety-net)
	}

	m, err := NewBuilder().
		SetUserID(in.UserID).
		SetType(in.Type).
		SetTitle(in.Title).
		SetBody(in.Body).
		SetDedupeKey(in.DedupeKey).
		SetVehicleID(in.VehicleID).
		SetFleetID(in.FleetID).
		Build()
	if err != nil {
		return err
	}
	if err := pr.store.Insert(m); err != nil {
		// A concurrent insert may win the unique-key race; treat the resulting
		// conflict as a successful dedupe so redelivery does not error.
		if errors.Is(err, ErrDuplicate) {
			return nil
		}
		return err
	}
	return nil
}

// List returns a page of the given user's notifications, optionally filtered by
// read-state and type (design §8.4). Notifications are always scoped to the
// caller's own user id.
func (pr *Processor) List(userID string, filter ListFilter, page server.Page) ([]Model, int, error) {
	return pr.prov.ListByUser(userID, filter, page)
}

// MarkRead stamps a single notification (owned by userID) as read.
func (pr *Processor) MarkRead(userID, id string) error {
	err := pr.adm.MarkRead(userID, id)
	if errors.Is(err, ErrNotFound) {
		return server.ErrNotFound
	}
	return err
}

// MarkAllRead stamps every unread notification owned by userID as read.
func (pr *Processor) MarkAllRead(userID string) error {
	return pr.adm.MarkAllRead(userID)
}
