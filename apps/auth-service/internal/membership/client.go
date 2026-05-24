// Package membership calls fleet-service's internal endpoint to resolve a user's
// active fleet/role for token minting (design §8.1; allowed under D2 — API, not join).
package membership

import (
	"context"
	"encoding/json"
	"net/http"
)

type Membership struct {
	FleetID string `json:"fleet_id"`
	Role    string `json:"role"`
}

type Client struct {
	base string
	hc   *http.Client
}

func NewClient(base string) *Client { return &Client{base: base, hc: http.DefaultClient} }

func (c *Client) Active(ctx context.Context, userID string) (Membership, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/internal/memberships/active?user_id="+userID, nil)
	res, err := c.hc.Do(req)
	if err != nil {
		return Membership{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return Membership{}, nil
	}
	var m Membership
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return Membership{}, err
	}
	return m, nil
}
