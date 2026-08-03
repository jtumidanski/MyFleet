// Package membership calls fleet-service's internal endpoint to resolve a user's
// active fleet/role for token minting (design §8.1; allowed under D2 — API, not join).
package membership

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
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
	// QueryEscape, not raw concatenation: an unescaped `&` in userID would end
	// this parameter and inject the rest as its own. The value is a
	// server-generated UUID today, so this is defence in depth rather than a
	// live fix — the same reasoning the fleet roster call below already applies.
	endpoint := c.base + "/internal/memberships/active?user_id=" + url.QueryEscape(userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

// fleetMemberLookupTimeout bounds one auth→fleet hop. The Client shares
// http.DefaultClient, which has NO timeout, so without this a wedged
// fleet-service pins an auth-service handler open indefinitely.
const fleetMemberLookupTimeout = 5 * time.Second

// FleetMemberIDs returns the user ids of a fleet's active members via
// fleet-service's EXISTING internal endpoint (GET /internal/fleets/{id}/members).
// No new fleet-service route is introduced, and the call direction is auth→fleet,
// the direction that already exists — so no import or dependency cycle.
//
// Unlike Active, 404 is an ERROR here, not a sentinel. Active maps 404 to a zero
// value because "this user has no fleet" is a real state; the fleet id passed
// here comes from a validated token, so its absence is a fault. Letting it mean
// "no members" would silently blank every name in the member list.
func (c *Client) FleetMemberIDs(ctx context.Context, fleetID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, fleetMemberLookupTimeout)
	defer cancel()

	// PathEscape even though fleetID is a validated JWT claim and so is not
	// attacker-shaped: it costs nothing, and escaping is now what both calls in
	// this file do.
	endpoint := c.base + "/internal/fleets/" + url.PathEscape(fleetID) + "/members"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	// Every non-2xx is an error. fleet-service's error envelope is JSON, so
	// without this a 500 decodes cleanly into an empty slice with err == nil —
	// indistinguishable from a fleet that really has no members.
	//
	// Status code and a fixed description only: the body is upstream-controlled
	// and the fleet id must not ride along into a log line.
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("fleet member lookup failed with status %d", res.StatusCode)
	}

	var rows []struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.UserID)
	}
	return out, nil
}
