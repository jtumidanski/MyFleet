// Package membership calls fleet-service's internal endpoint to resolve a user's
// active fleet/role for token minting (design §8.1; allowed under D2 — API, not join).
package membership

import (
	"context"
	"encoding/json"
	"fmt"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/internal/memberships/active?user_id="+userID, nil)
	if err != nil {
		return Membership{}, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return Membership{}, err
	}
	defer func() { _ = res.Body.Close() }()
	// 404 is the one status that is not a failure: a user with no fleet resolves
	// to a zero Membership, and the OIDC callback keys its onboarding redirect
	// off the resulting empty ActiveFleetID. Do not turn this into an error —
	// it would break a brand-new user's first login.
	if res.StatusCode == http.StatusNotFound {
		return Membership{}, nil
	}
	// Every OTHER non-2xx must be an error. fleet-service's error envelope is
	// JSON, so without this check a 500 decodes cleanly into a zero Membership
	// with err == nil — indistinguishable from the 404 above. The caller then
	// mints a valid token claiming the user has no fleet and the SPA offers to
	// create a duplicate one.
	//
	// Status code and a fixed description only: the body is upstream-controlled
	// and could carry anything, and the user id must not ride along in a
	// message that ends up in a log as an address.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return Membership{}, fmt.Errorf("active membership lookup failed with status %d", res.StatusCode)
	}
	var m Membership
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		return Membership{}, err
	}
	return m, nil
}
