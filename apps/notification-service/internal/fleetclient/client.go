// Package fleetclient is the internal HTTP client notification-service uses to
// resolve recipients and the maintenance reminder feed from fleet-service's
// network-restricted (no-JWT) internal endpoints (design D2 — cross-service data
// is fetched via API, never via a cross-service DB join). Mirrors the
// auth-service membership client pattern.
package fleetclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Member is one active fleet member (recipient) returned by
// GET /internal/fleets/{fleetID}/members.
type Member struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

// DueSchedule is one currently upcoming/overdue maintenance schedule returned by
// GET /internal/maintenance/due.
type DueSchedule struct {
	ScheduleID     string `json:"schedule_id"`
	VehicleID      string `json:"vehicle_id"`
	FleetID        string `json:"fleet_id"`
	CategoryID     string `json:"category_id"`
	State          string `json:"state"`
	NextDueDate    string `json:"next_due_date"`
	NextDueMileage int    `json:"next_due_mileage"`
}

// ErrInviteNotFound is returned by Invite when fleet-service reports 404. The
// mail consumer treats it as a PERMANENT condition (mark the ledger, do not
// retry): an invite that has been deleted will never come back, and retrying
// against it four times is pure waste.
var ErrInviteNotFound = errors.New("invite not found")

// Invite is one invite as served by fleet-service's network-restricted
// GET /internal/invites/{inviteID}. It carries the TOKEN, which is why the
// endpoint is internal-only and why this struct is never logged whole.
//
// ExpiresAt/AcceptedAt stay strings on the wire and are parsed with
// time.RFC3339 by the caller, matching how the invite REST layer formats them.
type Invite struct {
	InviteID        string  `json:"invite_id"`
	FleetID         string  `json:"fleet_id"`
	FleetName       string  `json:"fleet_name"`
	Email           string  `json:"email"`
	Role            string  `json:"role"`
	Token           string  `json:"token"`
	ExpiresAt       string  `json:"expires_at"`
	AcceptedAt      *string `json:"accepted_at"`
	InvitedByUserID string  `json:"invited_by_user_id"`
}

// statusError carries the HTTP status of a non-200 internal response so callers
// can classify it. It formats identically to the fmt.Errorf it replaced, so
// ActiveMembers and DueSchedules are unaffected.
type statusError struct {
	url  string
	code int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("fleet internal %s: status %d", e.url, e.code)
}

// Client calls fleet-service's internal endpoints. base is the service base URL
// (e.g. http://fleet-service:8080), from FLEET_INTERNAL_URL.
type Client struct {
	base string
	hc   *http.Client
}

// NewClient returns a Client targeting the given fleet-service base URL.
func NewClient(base string) *Client { return &Client{base: base, hc: http.DefaultClient} }

// ActiveMembers resolves a fleet's active members (recipients).
func (c *Client) ActiveMembers(ctx context.Context, fleetID string) ([]Member, error) {
	url := fmt.Sprintf("%s/internal/fleets/%s/members", c.base, fleetID)
	var out []Member
	if err := c.getJSON(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DueSchedules returns all currently upcoming/overdue schedules across all fleets.
func (c *Client) DueSchedules(ctx context.Context) ([]DueSchedule, error) {
	url := c.base + "/internal/maintenance/due"
	var out []DueSchedule
	if err := c.getJSON(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Invite fetches one invite, including its token, for composing an invite email.
func (c *Client) Invite(ctx context.Context, inviteID string) (Invite, error) {
	url := fmt.Sprintf("%s/internal/invites/%s", c.base, inviteID)
	var out Invite
	if err := c.getJSON(ctx, url, &out); err != nil {
		var se *statusError
		if errors.As(err, &se) && se.code == http.StatusNotFound {
			return Invite{}, ErrInviteNotFound
		}
		return Invite{}, err
	}
	return out, nil
}

func (c *Client) getJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return &statusError{url: url, code: res.StatusCode}
	}
	return json.NewDecoder(res.Body).Decode(dst)
}
