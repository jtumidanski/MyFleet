// Package fleetclient is the internal HTTP client notification-service uses to
// resolve recipients and the maintenance reminder feed from fleet-service's
// network-restricted (no-JWT) internal endpoints (design D2 — cross-service data
// is fetched via API, never via a cross-service DB join). Mirrors the
// auth-service membership client pattern.
package fleetclient

import (
	"context"
	"encoding/json"
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
		return fmt.Errorf("fleet internal %s: status %d", url, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(dst)
}
