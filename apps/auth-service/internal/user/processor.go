package user

import (
	"errors"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type Processor struct {
	log logrus.FieldLogger
	p   Provider
	a   Administrator
	// bootstrapEmails and grantAdmin are the provision-time half of platform
	// admin seeding (FR-ADMIN-AUTH-2). Both nil by default, so every existing
	// construction site compiles and behaves exactly as before.
	bootstrapEmails map[string]bool
	grantAdmin      func(userID string) error
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator) *Processor {
	return &Processor{log: log, p: p, a: a}
}

// WithBootstrapAdmins returns a copy of the processor that grants platform
// admin to any provisioned user whose email is in emails.
//
// It follows the repo's established With… idiom (cf.
// maintenanceschedule.WithOverdueHooks, NewCompletionDeps().WithActivityRecorder)
// so this package never imports platformadmin: the composition root supplies the
// grant as a function value.
func (pr *Processor) WithBootstrapAdmins(emails map[string]bool, grant func(userID string) error) *Processor {
	cp := *pr
	cp.bootstrapEmails = emails
	cp.grantAdmin = grant
	return &cp
}

// GetByID fetches a user by our internal user id — the value the JWT `sub`
// claim carries. This is what GET /auth/me needs; GetBySub is not a substitute.
func (pr *Processor) GetByID(id string) (Model, error) {
	return pr.p.GetByID(id)
}

// No GetBySub wrapper: ProvisionFromGoogle below calls pr.p.GetBySub directly,
// and it is the only code path that holds a Google sub. Exposing a lookup named
// "BySub" on the Processor is what invited the login-loop bug — the JWT's `sub`
// claim is a user id, so the name reads as correct at every call site that has
// an Identity. Anything holding a user id wants GetByID.

// ProvisionFromGoogle upserts a user by google_sub (FR-AUTH-2). Idempotent.
func (pr *Processor) ProvisionFromGoogle(gp GoogleProfile) (Model, error) {
	existing, err := pr.p.GetBySub(gp.Sub)
	if errors.Is(err, ErrNotFound) {
		m := NewBuilder().SetGoogleSub(gp.Sub).SetEmail(gp.Email).SetDisplayName(gp.Name).SetAvatarURL(gp.Avatar).Build()
		m = m.WithLogin(gp.Name, gp.Avatar, time.Now(), gp.EmailVerified)
		created, ierr := pr.a.Insert(m)
		if ierr != nil {
			return Model{}, ierr
		}
		pr.maybeGrantAdmin(created)
		return created, nil
	}
	if err != nil {
		return Model{}, err
	}
	updated, uerr := pr.a.Update(existing.WithLogin(gp.Name, gp.Avatar, time.Now(), gp.EmailVerified))
	if uerr != nil {
		return Model{}, uerr
	}
	pr.maybeGrantAdmin(updated)
	return updated, nil
}

// maybeGrantAdmin grants the platform-admin privilege when the provisioned user
// is on the bootstrap list AND Google's id_token asserted the email is
// verified as of THIS login (m.EmailVerified(), refreshed by WithLogin above
// rather than read off the incoming GoogleProfile directly — the persisted
// value is what platformadmin.SeedFromEmails also reads at boot, so both
// hooks honor exactly the same bit). The unverified case is not an error — it
// is simply not a grant: ordinary login for that user proceeds unaffected,
// only the admin escalation is withheld.
//
// A grant failure is logged, not returned. Refusing the login because a grant
// failed would be a worse outcome than a delayed grant, and the startup seed
// re-runs on every boot — so the failure is transient by construction.
func (pr *Processor) maybeGrantAdmin(m Model) {
	if pr.grantAdmin == nil || !pr.bootstrapEmails[strings.ToLower(m.Email())] {
		return
	}
	if !m.EmailVerified() {
		pr.log.WithField("user_id", m.ID()).
			Warn("bootstrap platform-admin email is on the list but Google has not verified it; refusing the grant")
		return
	}
	if err := pr.grantAdmin(m.ID()); err != nil {
		pr.log.WithError(err).WithField("user_id", m.ID()).
			Warn("bootstrap platform-admin grant failed; the startup seed will retry on the next boot")
	}
}

// UpdateTheme validates, loads, mutates and persists the caller's theme
// preference (PRD §5.2).
//
// Validation runs before the read on purpose: an out-of-range value then costs
// no database round trip and cannot leave a partially-applied state. The three
// error outcomes — ErrInvalidTheme, ErrNotFound, and anything else — are
// distinguishable at the call site, which is what lets the handler render 422,
// 404 and a bare 500 apart.
func (pr *Processor) UpdateTheme(userID string, pref string) (Model, error) {
	if !IsValidTheme(pref) {
		return Model{}, ErrInvalidTheme
	}
	m, err := pr.p.GetByID(userID)
	if err != nil {
		return Model{}, err
	}
	return pr.a.Update(m.WithThemePreference(pref))
}
