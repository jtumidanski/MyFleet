package invite

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/fleet-service/internal/membership"
)

// defaultExpiry is how long a freshly minted or freshly rotated invite lives.
const defaultExpiry = 7 * 24 * time.Hour

// Limits carries the abuse-control knobs (FR-RATE-1…4). All three are enforced
// server-side, here in the domain layer; the UI disabling a button is a
// convenience, not the control.
type Limits struct {
	CreatePerWindow int
	CreateWindow    time.Duration
	ResendCooldown  time.Duration
}

// Processor contains invite business logic.
//
// The read path (Provider) is required. The write path (Administrator) and the
// rate-limit knobs are injected through the With* chain because the internal,
// read-only route (internal.go) needs neither — the same optional-collaborator
// shape dbAdministrator uses for its emitters.
type Processor struct {
	log    logrus.FieldLogger
	p      Provider
	adm    Administrator
	limits Limits
}

func NewProcessor(log logrus.FieldLogger, p Provider) *Processor {
	return &Processor{log: log, p: p}
}

// WithAdministrator injects the write path. Required for Create/Resend/Accept/Delete.
func (pr *Processor) WithAdministrator(adm Administrator) *Processor {
	pr.adm = adm
	return pr
}

// WithLimits injects the abuse-control knobs. Required for Create/Resend.
func (pr *Processor) WithLimits(l Limits) *Processor {
	pr.limits = l
	return pr
}

// ListByFleet returns all invites for a fleet.
func (pr *Processor) ListByFleet(ctx context.Context, fleetID string) ([]Model, error) {
	return pr.p.ListByFleetID(ctx, fleetID)
}

// ListRedeemableForEmail returns the invites waiting for authedEmail — the
// discovery path for a user who was invited before they had an account and so
// has no fleet, no link, and nothing to click.
//
// authedEmail must come from the validated token, never from the request: this
// is the only thing scoping the result set, so a caller-supplied address would
// let anyone enumerate anyone else's invites.
//
// A blank email returns nothing rather than querying. A token that validates
// but carries no `email` claim is a real failure mode (see the warning in
// packages/shared-go/auth/middleware.go); folding a blank address through the
// LOWER() comparison would match every blank-address row in the table and hand
// them all to that caller.
func (pr *Processor) ListRedeemableForEmail(ctx context.Context, authedEmail string) ([]Model, error) {
	if authedEmail == "" {
		return []Model{}, nil
	}
	return pr.p.ListRedeemableByEmail(ctx, authedEmail, time.Now())
}

// GetByID fetches an invite by ID.
func (pr *Processor) GetByID(ctx context.Context, id string) (Model, error) {
	m, err := pr.p.GetByID(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}

// GetByToken fetches an invite by its token.
func (pr *Processor) GetByToken(ctx context.Context, token string) (Model, error) {
	m, err := pr.p.GetByToken(ctx, token)
	if errors.Is(err, ErrNotFound) {
		return Model{}, server.ErrNotFound
	}
	return m, err
}

// The three preconditions ValidateAccept enforces, each carrying its own
// JSON:API detail so a 409 tells the caller which one failed. All three wrap
// server.ErrConflict, so StatusFor still renders 409 (FR-8) and any existing
// errors.Is(err, server.ErrConflict) check keeps working.
//
// ErrEmailMismatch's detail deliberately names NEITHER address, and is a
// constant with no format verb so no code path can interpolate one. The invite
// token is a bearer credential — anyone holding the link reaches this endpoint
// — so echoing the invited address would turn a leaked link into a
// who-was-this-for oracle (PRD FR-10).
var (
	ErrAlreadyAccepted = server.Detailed(server.ErrConflict, "invite has already been accepted")
	ErrInviteExpired   = server.Detailed(server.ErrConflict, "invite has expired")
	ErrEmailMismatch   = server.Detailed(server.ErrConflict, "invite was issued to a different account")

	// ErrInviteUnusable reports a corrupt row rather than anything the caller
	// did: an invite with no email address cannot be matched against anyone, so
	// it can never legitimately be accepted. It is separate from
	// ErrEmailMismatch so the two are distinguishable in logs — a mismatch is a
	// user outcome, a blank address is a data defect an operator should chase.
	//
	// It renders 409 like the other three (FR-8): the caller's request is
	// well-formed and correctly authenticated, and the response must not tell a
	// bearer-link holder that they found a broken row worth probing.
	ErrInviteUnusable = server.Detailed(server.ErrConflict, "invite cannot be accepted")
)

// ValidateAccept enforces FR-FLEET-3: invite must be for the same email, not
// yet accepted, and not expired. Each violation returns its own sentinel; all
// three render 409.
//
// Order is load-bearing (accepted → expired → email): a wrong-account caller
// presenting an already-accepted invite learns only "already accepted".
func (pr *Processor) ValidateAccept(inv Model, authedEmail string) error {
	if inv.AcceptedAt() != nil {
		return ErrAlreadyAccepted
	}
	if !inv.ExpiresAt().After(time.Now()) {
		return ErrInviteExpired
	}
	// The blank-invite-email guard sits AT the email precondition, not ahead of
	// the two above it, so the disclosure order is undisturbed. Without it
	// strings.EqualFold("", "") below is true and a row with no address would be
	// accepted by any authenticated caller — including one carrying the empty
	// `email` claim this branch fixes. Unreachable today (resource.go rejects a
	// blank email at creation); the guard makes it structurally impossible
	// instead of impossible by another file's validation.
	if inv.Email() == "" {
		return ErrInviteUnusable
	}
	if !strings.EqualFold(inv.Email(), authedEmail) {
		return ErrEmailMismatch
	}
	return nil
}

// ValidateInviteEmail enforces that an invite address is a bare addr-spec safe
// to interpolate into a To: header (PRD §8 Security).
//
// CR/LF is checked first and separately so a header-injection attempt fails for
// an unambiguous reason. The a.Address != s comparison is the important half:
// mail.ParseAddress happily accepts "Bob <b@x.com>", whose display name would
// then be attacker-influenced header content. Requiring the input to equal the
// parsed addr-spec makes the header value a closed set of characters.
func ValidateInviteEmail(s string) error {
	if strings.ContainsAny(s, "\r\n") {
		return server.ErrValidation
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s {
		return server.ErrValidation
	}
	return nil
}

// CheckCreateLimit enforces the per-fleet invite creation window (FR-RATE-1).
// Over the limit → server.ErrTooManyRequests (429).
func (pr *Processor) CheckCreateLimit(ctx context.Context, fleetID string, limit int, window time.Duration, now time.Time) error {
	n, err := pr.p.CountByFleetSince(ctx, fleetID, now.Add(-window))
	if err != nil {
		return err
	}
	if n >= int64(limit) {
		return server.ErrTooManyRequests
	}
	return nil
}

// CheckResendCooldown enforces the per-invite resend cooldown (FR-RATE-2),
// derived from updated_at — GORM stamps it on the token rotation, so the limit
// survives a pod restart and holds across replicas. See design §4.4 for why
// updated_at is preferred over deriving the last rotation from expires_at.
func (pr *Processor) CheckResendCooldown(inv Model, cooldown time.Duration, now time.Time) error {
	if now.Sub(inv.UpdatedAt()) < cooldown {
		return server.ErrTooManyRequests
	}
	return nil
}

// Create mints an invite: it validates the role and the address, enforces the
// per-fleet creation window, generates the token, computes the expiry, builds
// the model and hands it to the administrator, which writes the row and the
// invite.created outbox event in one transaction.
//
// The caller (the HTTP handler) has already established that the requester may
// invite into fleetID; authorization is not this function's job. Everything
// after that is.
//
// Order is preserved from the handler this replaced: role, then address, then
// the rate limit — checked BEFORE minting a token, so a throttled request costs
// no entropy and no database write. Role and address both return
// server.ErrValidation, so their relative order is not observable.
func (pr *Processor) Create(ctx context.Context, fleetID, email, role, invitedByUserID, traceID string) (Model, error) {
	// Role is copied verbatim onto the membership created at accept time, so an
	// unrecognised value would mint a membership whose role no authz gate
	// understands. Validate against the vocabulary membership owns.
	if !membership.IsValidRole(role) {
		return Model{}, server.ErrValidation
	}
	// A newline in an address must fail HERE, not be discovered by the SMTP
	// layer hours later (PRD §8 Security).
	if err := ValidateInviteEmail(email); err != nil {
		return Model{}, err
	}

	now := time.Now()
	if err := pr.CheckCreateLimit(ctx, fleetID, pr.limits.CreatePerWindow, pr.limits.CreateWindow, now); err != nil {
		return Model{}, err
	}

	token, err := generateToken()
	if err != nil {
		return Model{}, err
	}

	m, err := NewBuilder().
		SetFleetID(fleetID).
		SetEmail(email).
		SetRole(role).
		SetToken(token).
		SetExpiresAt(now.Add(defaultExpiry)).
		SetInvitedByUserID(invitedByUserID).
		Build()
	if err != nil {
		return Model{}, err
	}
	return pr.adm.Insert(ctx, m, traceID)
}

// Resend rotates inv's token and resets its expiry (FR-RSND-1…5). Resend is
// used when the previous link never arrived or expired, so invalidating it costs
// nothing and bounds the lifetime of a token that leaked into a mailbox.
//
// inv is supplied by the caller rather than re-read here: the handler must read
// it first anyway to authorize the request against the invite's own fleet, and
// re-reading would double the query for no gain. The administrator's UPDATE is
// guarded by `accepted_at IS NULL` and checks RowsAffected, so the staleness of
// inv cannot be exploited.
//
// now is computed once and used for BOTH the cooldown comparison and the
// persisted updated_at, and is handed to the administrator explicitly. That is
// what makes the updated_at on the returned Model provably the value written —
// which is precisely what the next CheckResendCooldown reads.
func (pr *Processor) Resend(ctx context.Context, inv Model, traceID string) (Model, error) {
	// Accepted BEFORE cooldown, so an accepted invite never reports a cooldown
	// it could never satisfy (FR-RSND-3).
	if inv.AcceptedAt() != nil {
		return Model{}, server.ErrConflict
	}
	now := time.Now()
	if err := pr.CheckResendCooldown(inv, pr.limits.ResendCooldown, now); err != nil {
		return Model{}, err
	}
	token, err := generateToken()
	if err != nil {
		return Model{}, err
	}
	return pr.adm.Resend(ctx, inv, token, now.Add(defaultExpiry), now, traceID)
}

// Accept enforces the accept preconditions and, if they hold, stamps
// accepted_at and mints the membership in one transaction.
//
// The sentinel ValidateAccept returns reaches the caller unchanged, so a handler
// can still tell an ordinary already-accepted/expired outcome apart from an
// email mismatch or a corrupt row.
func (pr *Processor) Accept(ctx context.Context, inv Model, userID, authedEmail, traceID string) (Model, error) {
	if err := pr.ValidateAccept(inv, authedEmail); err != nil {
		return Model{}, err
	}
	return pr.adm.Accept(ctx, inv, userID, traceID)
}

// Delete removes an invite. Authorization is the handler's; the write is not.
func (pr *Processor) Delete(ctx context.Context, id string) error {
	return pr.adm.Delete(ctx, id)
}

// generateToken returns a cryptographically random 32-byte hex string.
//
// The result is a BEARER CREDENTIAL: it is the whole of the authority the accept
// route checks. It must never reach a log message, a log field or an error
// string — see the sentinel comments above, which is why no error returned from
// this file's operations carries one.
func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
